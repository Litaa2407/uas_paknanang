package main

import (
	"context"
	"crypto/rand"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const midtransSandboxBaseURL = "https://app.sandbox.midtrans.com"
const midtransSandboxAPIBaseURL = "https://api.sandbox.midtrans.com"
const maxDonationAmount = int64(100_000_000_000)

type midtransTransactionStatus struct {
	OrderID           string `json:"order_id"`
	GrossAmount       string `json:"gross_amount"`
	TransactionStatus string `json:"transaction_status"`
	FraudStatus       string `json:"fraud_status"`
}

type midtransDonationStatusResponse struct {
	Reference string `json:"reference"`
	Status    string `json:"status"`
	Amount    int64  `json:"amount"`
}

type pendingCampaignDonation struct {
	Reference   string `json:"reference"`
	Amount      int64  `json:"amount"`
	RedirectURL string `json:"redirect_url"`
}

type donorDonationProgress struct {
	Reference     string `json:"reference"`
	CampaignID    int64  `json:"campaign_id"`
	CampaignTitle string `json:"campaign_title"`
	Amount        int64  `json:"amount"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
	Target        int64  `json:"target"`
	Collected     int64  `json:"collected"`
}

func ensureMidtransSchema(ctx context.Context, pool *pgxpool.Pool) error {
	statements := []string{
		"ALTER TABLE donations ADD COLUMN IF NOT EXISTS donor_user_id BIGINT REFERENCES users(id)",
		"ALTER TABLE donations ADD COLUMN IF NOT EXISTS donor_email VARCHAR(150)",
		"ALTER TABLE donations ADD COLUMN IF NOT EXISTS donor_phone VARCHAR(30)",
		"ALTER TABLE donations ADD COLUMN IF NOT EXISTS payment_url TEXT",
		"CREATE INDEX IF NOT EXISTS idx_donations_donor_created ON donations(donor_user_id,created_at DESC,id DESC) WHERE donor_user_id IS NOT NULL",
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement); err != nil {
			return err
		}
	}
	if _, err := pool.Exec(ctx, `
		UPDATE campaigns
		SET status='SELESAI'
		WHERE status='AKTIF' AND target > 0 AND collected >= target
	`); err != nil {
		return err
	}
	if _, err := pool.Exec(ctx, `
		UPDATE donations d
		SET donor_user_id=u.id
		FROM users u
		WHERE d.donor_user_id IS NULL
			AND d.donor_email IS NOT NULL
			AND LOWER(BTRIM(d.donor_email))=LOWER(BTRIM(u.email))
	`); err != nil {
		return err
	}
	return nil
}

func createMidtransDonation(w http.ResponseWriter, r *http.Request) {
	serverKey := strings.TrimSpace(os.Getenv("MIDTRANS_SERVER_KEY"))
	if serverKey == "" {
		http.Error(w, "Pembayaran belum dikonfigurasi. Admin perlu mengisi MIDTRANS_SERVER_KEY.", http.StatusServiceUnavailable)
		return
	}

	user, ok := r.Context().Value(userKey).(User)
	if !ok {
		http.Error(w, "autentikasi diperlukan", http.StatusUnauthorized)
		return
	}
	if user.Role != "DONATUR" {
		http.Error(w, "hanya akun Donatur yang dapat melakukan pembayaran", http.StatusForbidden)
		return
	}

	var in struct {
		CampaignID int64  `json:"campaign_id"`
		Amount     int64  `json:"amount"`
		DonorName  string `json:"donor_name"`
		DonorEmail string `json:"donor_email"`
		DonorPhone string `json:"donor_phone"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		http.Error(w, "data donasi tidak valid", http.StatusBadRequest)
		return
	}
	in.DonorName = strings.TrimSpace(in.DonorName)
	in.DonorEmail = strings.TrimSpace(in.DonorEmail)
	in.DonorPhone = strings.TrimSpace(in.DonorPhone)
	if in.CampaignID <= 0 || in.Amount < 1000 || in.Amount > maxDonationAmount ||
		len(in.DonorName) < 2 || len(in.DonorName) > 100 ||
		!validDonorEmail(in.DonorEmail) || !validDonorPhone(in.DonorPhone) {
		http.Error(w, "data donasi tidak valid. Periksa nama, email, nomor telepon, dan nominal minimal Rp 1.000.", http.StatusBadRequest)
		return
	}

	var referenceBytes [16]byte
	if _, err := rand.Read(referenceBytes[:]); err != nil {
		http.Error(w, "gagal membuat referensi pembayaran", http.StatusInternalServerError)
		return
	}
	reference := "DON-" + hex.EncodeToString(referenceBytes[:])

	tx, err := db.Begin(r.Context())
	if err != nil {
		http.Error(w, "gagal memulai pembayaran", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	var campaignTitle string
	var campaignTarget, campaignCollected int64
	err = tx.QueryRow(r.Context(),
		"SELECT title,target,collected FROM campaigns WHERE id=$1 AND status='AKTIF' FOR UPDATE",
		in.CampaignID,
	).Scan(&campaignTitle, &campaignTarget, &campaignCollected)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "campaign tidak ditemukan atau tidak aktif", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "gagal memeriksa campaign", http.StatusInternalServerError)
		return
	}
	var pendingAmount int64
	if err := tx.QueryRow(r.Context(), `
		SELECT COALESCE(SUM(amount),0)
		FROM donations
		WHERE campaign_id=$1 AND status='PENDING'
			AND created_at > NOW() - INTERVAL '24 hours'
	`, in.CampaignID).Scan(&pendingAmount); err != nil {
		http.Error(w, "gagal memeriksa donasi tertunda", http.StatusInternalServerError)
		return
	}
	remainingAmount := availableDonationAmount(campaignTarget, campaignCollected, pendingAmount)
	if in.Amount > remainingAmount {
		http.Error(w, fmt.Sprintf("Nominal donasi melebihi sisa kebutuhan campaign sebesar Rp %s.", formatRupiah(remainingAmount)), http.StatusBadRequest)
		return
	}
	_, err = tx.Exec(r.Context(), `
		INSERT INTO donations(campaign_id,donor_user_id,donor_name,donor_email,donor_phone,amount,payment_reference,status)
		VALUES($1,$2,$3,$4,$5,$6,$7,'PENDING')
	`, in.CampaignID, user.ID, in.DonorName, in.DonorEmail, in.DonorPhone, in.Amount, reference)
	if err != nil {
		log.Printf("gagal mencatat pembayaran pending: %v", err)
		http.Error(w, "gagal mencatat pembayaran", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "gagal menyiapkan pembayaran", http.StatusInternalServerError)
		return
	}

	finishURL := strings.TrimRight(strings.TrimSpace(os.Getenv("FRONTEND_URL")), "/")
	if finishURL == "" {
		finishURL = "http://localhost:5173"
	}
	callbackURL := fmt.Sprintf("%s/campaign/%d/donate?order_id=%s", finishURL, in.CampaignID, url.QueryEscape(reference))
	payload := map[string]interface{}{
		"transaction_details": map[string]interface{}{
			"order_id":     reference,
			"gross_amount": in.Amount,
		},
		"item_details": []map[string]interface{}{{
			"id":       strconv.FormatInt(in.CampaignID, 10),
			"price":    in.Amount,
			"quantity": 1,
			"name":     campaignTitle,
		}},
		"customer_details": map[string]string{
			"first_name": in.DonorName,
			"email":      in.DonorEmail,
			"phone":      in.DonorPhone,
		},
		"callbacks": map[string]string{"finish": callbackURL},
	}
	var response struct {
		RedirectURL string `json:"redirect_url"`
	}
	if err := callMidtrans(r.Context(), http.MethodPost, midtransSandboxBaseURL+"/snap/v1/transactions", serverKey, payload, &response); err != nil {
		log.Printf("gagal membuat transaksi Midtrans sandbox (%s): %v", reference, err)
		markPendingDonationFailed(r.Context(), reference)
		http.Error(w, "pembayaran belum dapat dibuat. Silakan coba lagi.", http.StatusBadGateway)
		return
	}
	if !validMidtransRedirectURL(response.RedirectURL) {
		log.Printf("Midtrans mengembalikan redirect URL yang tidak valid (%s)", reference)
		markPendingDonationFailed(r.Context(), reference)
		http.Error(w, "provider pembayaran mengembalikan alamat yang tidak valid", http.StatusBadGateway)
		return
	}
	if _, err := db.Exec(r.Context(), `
		UPDATE donations
		SET payment_url=$2
		WHERE payment_reference=$1 AND status='PENDING'
	`, reference, response.RedirectURL); err != nil {
		log.Printf("gagal menyimpan tautan pembayaran (%s): %v", reference, err)
		http.Error(w, "gagal menyimpan tautan pembayaran", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{
		"status":       "PENDING",
		"reference":    reference,
		"redirect_url": response.RedirectURL,
	})
}

func availableDonationAmount(target, collected, pending int64) int64 {
	remaining := target - collected - pending
	if remaining < 0 {
		return 0
	}
	return remaining
}

func markPendingDonationFailed(ctx context.Context, reference string) {
	if _, err := db.Exec(ctx, "UPDATE donations SET status='FAILED' WHERE payment_reference=$1 AND status='PENDING'", reference); err != nil {
		log.Printf("gagal membatalkan pembayaran yang tidak berhasil dibuat (%s): %v", reference, err)
	}
}

func formatRupiah(amount int64) string {
	raw := strconv.FormatInt(amount, 10)
	for i := len(raw) - 3; i > 0; i -= 3 {
		raw = raw[:i] + "." + raw[i:]
	}
	return raw
}

func listPendingCampaignDonations(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(userKey).(User)
	if !ok {
		http.Error(w, "autentikasi diperlukan", http.StatusUnauthorized)
		return
	}
	if user.Role != "DONATUR" {
		http.Error(w, "hanya akun Donatur yang dapat melanjutkan pembayaran", http.StatusForbidden)
		return
	}
	campaignID, err := strconv.ParseInt(r.URL.Query().Get("campaign_id"), 10, 64)
	if err != nil || campaignID <= 0 {
		http.Error(w, "campaign_id tidak valid", http.StatusBadRequest)
		return
	}
	serverKey := strings.TrimSpace(os.Getenv("MIDTRANS_SERVER_KEY"))
	if serverKey == "" {
		http.Error(w, "pembayaran belum dikonfigurasi", http.StatusServiceUnavailable)
		return
	}
	rows, err := db.Query(r.Context(), `
		SELECT payment_reference,amount,payment_url
		FROM donations
		WHERE campaign_id=$1 AND donor_user_id=$2 AND status='PENDING'
			AND payment_url IS NOT NULL
			AND created_at > NOW() - INTERVAL '24 hours'
		ORDER BY created_at DESC,id DESC
		LIMIT 5
	`, campaignID, user.ID)
	if err != nil {
		http.Error(w, "gagal memuat pembayaran tertunda", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	candidates := []pendingCampaignDonation{}
	for rows.Next() {
		var reference, paymentURL string
		var amount int64
		if err := rows.Scan(&reference, &amount, &paymentURL); err != nil {
			http.Error(w, "gagal membaca pembayaran tertunda", http.StatusInternalServerError)
			return
		}
		if !validMidtransRedirectURL(paymentURL) {
			log.Printf("tautan pembayaran tersimpan tidak valid (%s)", reference)
			continue
		}
		candidates = append(candidates, pendingCampaignDonation{
			Reference: reference, Amount: amount, RedirectURL: paymentURL,
		})
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "gagal membaca pembayaran tertunda", http.StatusInternalServerError)
		return
	}
	rows.Close()

	payments := make([]pendingCampaignDonation, 0, len(candidates))
	for _, payment := range candidates {
		providerStatus, err := getMidtransStatus(r.Context(), serverKey, payment.Reference)
		if err != nil {
			log.Printf("gagal memeriksa pembayaran tertunda (%s): %v", payment.Reference, err)
			http.Error(w, "status pembayaran tertunda belum dapat diperiksa. Coba muat ulang.", http.StatusBadGateway)
			return
		}
		if providerStatus.OrderID != payment.Reference || !midtransAmountMatches(providerStatus.GrossAmount, payment.Amount) {
			log.Printf("data pembayaran tertunda tidak cocok (%s)", payment.Reference)
			http.Error(w, "data pembayaran tertunda tidak cocok", http.StatusBadGateway)
			return
		}
		status := mapMidtransStatus(providerStatus.TransactionStatus, providerStatus.FraudStatus)
		if err := applyMidtransDonationStatus(r.Context(), payment.Reference, status); err != nil {
			log.Printf("gagal memperbarui pembayaran tertunda (%s): %v", payment.Reference, err)
			http.Error(w, "gagal memperbarui status pembayaran tertunda", http.StatusInternalServerError)
			return
		}
		if status == "PENDING" {
			payments = append(payments, payment)
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"payments": payments})
}

func listDonorDonationProgress(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(userKey).(User)
	if !ok {
		http.Error(w, "autentikasi diperlukan", http.StatusUnauthorized)
		return
	}
	if user.Role != "DONATUR" {
		http.Error(w, "halaman donasi hanya tersedia untuk akun Donatur", http.StatusForbidden)
		return
	}

	rows, err := db.Query(r.Context(), `
		SELECT d.payment_reference,d.campaign_id,c.title,d.amount,d.status,
			d.created_at::text,c.target,c.collected
		FROM donations d
		JOIN campaigns c ON c.id=d.campaign_id
		WHERE d.donor_user_id=$1
			OR (d.donor_user_id IS NULL AND LOWER(BTRIM(COALESCE(d.donor_email,'')))=LOWER(BTRIM($2)))
		ORDER BY d.created_at DESC,d.id DESC
	`, user.ID, user.Email)
	if err != nil {
		http.Error(w, "gagal memuat riwayat donasi", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	donations := []donorDonationProgress{}
	for rows.Next() {
		var donation donorDonationProgress
		if err := rows.Scan(
			&donation.Reference,
			&donation.CampaignID,
			&donation.CampaignTitle,
			&donation.Amount,
			&donation.Status,
			&donation.CreatedAt,
			&donation.Target,
			&donation.Collected,
		); err != nil {
			http.Error(w, "gagal membaca riwayat donasi", http.StatusInternalServerError)
			return
		}
		donations = append(donations, donation)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "gagal membaca riwayat donasi", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"donations": donations})
}

func validMidtransRedirectURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	return err == nil && parsed.Scheme == "https" && parsed.Host == "app.sandbox.midtrans.com"
}

func cancelMidtransDonation(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(userKey).(User)
	if !ok {
		http.Error(w, "autentikasi diperlukan", http.StatusUnauthorized)
		return
	}
	if user.Role != "DONATUR" {
		http.Error(w, "hanya akun Donatur yang dapat membatalkan pembayaran", http.StatusForbidden)
		return
	}
	serverKey := strings.TrimSpace(os.Getenv("MIDTRANS_SERVER_KEY"))
	if serverKey == "" {
		http.Error(w, "pembayaran belum dikonfigurasi", http.StatusServiceUnavailable)
		return
	}
	reference := strings.TrimSpace(r.PathValue("reference"))
	if len(reference) > 50 || reference == "" {
		http.Error(w, "referensi pembayaran tidak valid", http.StatusBadRequest)
		return
	}

	var amount int64
	var currentStatus string
	err := db.QueryRow(r.Context(), `
		SELECT amount,status
		FROM donations
		WHERE payment_reference=$1 AND donor_user_id=$2
	`, reference, user.ID).Scan(&amount, &currentStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "gagal memeriksa pembayaran", http.StatusInternalServerError)
		return
	}
	if currentStatus == "SETTLEMENT" || currentStatus == "FAILED" {
		writeJSON(w, http.StatusOK, midtransDonationStatusResponse{
			Reference: reference, Status: currentStatus, Amount: amount,
		})
		return
	}

	providerStatus, err := getMidtransStatus(r.Context(), serverKey, reference)
	if err != nil {
		log.Printf("gagal memeriksa pembayaran sebelum pembatalan (%s): %v", reference, err)
		http.Error(w, "status pembayaran belum dapat diperiksa. Coba lagi.", http.StatusBadGateway)
		return
	}
	if providerStatus.OrderID != reference || !midtransAmountMatches(providerStatus.GrossAmount, amount) {
		log.Printf("data transaksi Midtrans tidak cocok saat pembatalan (%s)", reference)
		http.Error(w, "data transaksi pembayaran tidak cocok", http.StatusBadGateway)
		return
	}
	status := mapMidtransStatus(providerStatus.TransactionStatus, providerStatus.FraudStatus)
	if status != "PENDING" {
		if err := applyMidtransDonationStatus(r.Context(), reference, status); err != nil {
			log.Printf("gagal menyimpan status pembayaran saat pembatalan (%s): %v", reference, err)
			http.Error(w, "gagal memperbarui status pembayaran", http.StatusInternalServerError)
			return
		}
		status, err = readMidtransDonationStatus(r.Context(), reference)
		if err != nil {
			http.Error(w, "gagal membaca status pembayaran terbaru", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, midtransDonationStatusResponse{
			Reference: reference, Status: status, Amount: amount,
		})
		return
	}

	var cancelResponse map[string]json.RawMessage
	cancelErr := callMidtrans(
		r.Context(),
		http.MethodPost,
		midtransSandboxAPIBaseURL+"/v2/"+url.PathEscape(reference)+"/cancel",
		serverKey,
		nil,
		&cancelResponse,
	)
	finalProviderStatus, statusErr := getMidtransStatus(r.Context(), serverKey, reference)
	if statusErr != nil {
		log.Printf("gagal memverifikasi pembatalan Midtrans (%s): %v", reference, statusErr)
		http.Error(w, "pembatalan belum dapat diverifikasi. Periksa kembali status pembayaran.", http.StatusBadGateway)
		return
	}
	if finalProviderStatus.OrderID != reference || !midtransAmountMatches(finalProviderStatus.GrossAmount, amount) {
		log.Printf("data transaksi Midtrans tidak cocok setelah pembatalan (%s)", reference)
		http.Error(w, "data transaksi pembayaran tidak cocok", http.StatusBadGateway)
		return
	}
	status = mapMidtransStatus(finalProviderStatus.TransactionStatus, finalProviderStatus.FraudStatus)
	if status == "PENDING" {
		if cancelErr != nil {
			log.Printf("Midtrans menolak pembatalan transaksi (%s): %v", reference, cancelErr)
		}
		http.Error(w, "pembayaran masih tertunda dan belum berhasil dibatalkan. Coba lagi atau periksa status transaksi.", http.StatusBadGateway)
		return
	}
	if err := applyMidtransDonationStatus(r.Context(), reference, status); err != nil {
		log.Printf("gagal menyimpan hasil pembatalan pembayaran (%s): %v", reference, err)
		http.Error(w, "gagal memperbarui status pembayaran", http.StatusInternalServerError)
		return
	}
	status, err = readMidtransDonationStatus(r.Context(), reference)
	if err != nil {
		http.Error(w, "gagal membaca status pembayaran terbaru", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, midtransDonationStatusResponse{
		Reference: reference, Status: status, Amount: amount,
	})
}

func readMidtransDonationStatus(ctx context.Context, reference string) (string, error) {
	var status string
	err := db.QueryRow(ctx, "SELECT status FROM donations WHERE payment_reference=$1", reference).Scan(&status)
	return status, err
}

func getMidtransDonationStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := r.Context().Value(userKey).(User)
	if !ok {
		http.Error(w, "autentikasi diperlukan", http.StatusUnauthorized)
		return
	}
	if user.Role != "DONATUR" {
		http.Error(w, "hanya akun Donatur yang dapat memeriksa pembayaran", http.StatusForbidden)
		return
	}
	serverKey := strings.TrimSpace(os.Getenv("MIDTRANS_SERVER_KEY"))
	if serverKey == "" {
		http.Error(w, "pembayaran belum dikonfigurasi", http.StatusServiceUnavailable)
		return
	}
	reference := strings.TrimSpace(r.PathValue("reference"))
	if len(reference) > 50 || reference == "" {
		http.Error(w, "referensi pembayaran tidak valid", http.StatusBadRequest)
		return
	}

	var amount int64
	var currentStatus string
	err := db.QueryRow(r.Context(), `
		SELECT amount,status
		FROM donations
		WHERE payment_reference=$1 AND donor_user_id=$2
	`, reference, user.ID).Scan(&amount, &currentStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "gagal memeriksa pembayaran", http.StatusInternalServerError)
		return
	}
	if currentStatus == "SETTLEMENT" || currentStatus == "FAILED" {
		writeJSON(w, http.StatusOK, midtransDonationStatusResponse{
			Reference: reference, Status: currentStatus, Amount: amount,
		})
		return
	}

	providerStatus, err := getMidtransStatus(r.Context(), serverKey, reference)
	if err != nil {
		log.Printf("gagal memeriksa status Midtrans (%s): %v", reference, err)
		http.Error(w, "status pembayaran belum dapat diperiksa. Coba lagi.", http.StatusBadGateway)
		return
	}
	if providerStatus.OrderID != reference || !midtransAmountMatches(providerStatus.GrossAmount, amount) {
		log.Printf("data transaksi Midtrans tidak cocok (%s)", reference)
		http.Error(w, "data transaksi pembayaran tidak cocok", http.StatusBadGateway)
		return
	}
	status := mapMidtransStatus(providerStatus.TransactionStatus, providerStatus.FraudStatus)
	if err := applyMidtransDonationStatus(r.Context(), reference, status); err != nil {
		log.Printf("gagal memperbarui status donasi (%s): %v", reference, err)
		http.Error(w, "gagal memperbarui status pembayaran", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, midtransDonationStatusResponse{
		Reference: reference, Status: status, Amount: amount,
	})
}

func midtransPaymentWebhook(w http.ResponseWriter, r *http.Request) {
	serverKey := strings.TrimSpace(os.Getenv("MIDTRANS_SERVER_KEY"))
	if serverKey == "" {
		http.Error(w, "payment provider belum dikonfigurasi", http.StatusServiceUnavailable)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var notification struct {
		OrderID           string `json:"order_id"`
		StatusCode        string `json:"status_code"`
		GrossAmount       string `json:"gross_amount"`
		SignatureKey      string `json:"signature_key"`
		TransactionStatus string `json:"transaction_status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&notification); err != nil {
		http.Error(w, "notifikasi tidak valid", http.StatusBadRequest)
		return
	}
	expected := midtransNotificationSignature(notification.OrderID, notification.StatusCode, notification.GrossAmount, serverKey)
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(notification.SignatureKey)), []byte(expected)) != 1 {
		http.Error(w, "signature notifikasi tidak valid", http.StatusUnauthorized)
		return
	}

	providerStatus, err := getMidtransStatus(r.Context(), serverKey, notification.OrderID)
	if err != nil {
		log.Printf("gagal memverifikasi notifikasi Midtrans (%s): %v", notification.OrderID, err)
		http.Error(w, "status transaksi belum dapat diverifikasi", http.StatusBadGateway)
		return
	}
	var amount int64
	err = db.QueryRow(r.Context(),
		"SELECT amount FROM donations WHERE payment_reference=$1",
		notification.OrderID,
	).Scan(&amount)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "gagal memeriksa transaksi", http.StatusInternalServerError)
		return
	}
	if providerStatus.OrderID != notification.OrderID || !midtransAmountMatches(providerStatus.GrossAmount, amount) {
		http.Error(w, "data transaksi tidak cocok", http.StatusBadRequest)
		return
	}
	status := mapMidtransStatus(providerStatus.TransactionStatus, providerStatus.FraudStatus)
	if err := applyMidtransDonationStatus(r.Context(), notification.OrderID, status); err != nil {
		log.Printf("gagal menyimpan notifikasi Midtrans (%s): %v", notification.OrderID, err)
		http.Error(w, "gagal menyimpan status transaksi", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func midtransNotificationSignature(orderID, statusCode, grossAmount, serverKey string) string {
	sum := sha512.Sum512([]byte(orderID + statusCode + grossAmount + serverKey))
	return hex.EncodeToString(sum[:])
}

func getMidtransStatus(ctx context.Context, serverKey, reference string) (midtransTransactionStatus, error) {
	var status midtransTransactionStatus
	endpoint := midtransSandboxAPIBaseURL + "/v2/" + url.PathEscape(reference) + "/status"
	err := callMidtrans(ctx, http.MethodGet, endpoint, serverKey, nil, &status)
	return status, err
}

func callMidtrans(ctx context.Context, method, endpoint, serverKey string, payload interface{}, result interface{}) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = strings.NewReader(string(encoded))
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	request.SetBasicAuth(serverKey, "")
	request.Header.Set("Accept", "application/json")
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 15 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("provider merespons HTTP %d", response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(result); err != nil {
		return fmt.Errorf("response provider tidak valid: %w", err)
	}
	return nil
}

func midtransAmountMatches(rawAmount string, expectedAmount int64) bool {
	amount, err := strconv.ParseFloat(rawAmount, 64)
	return err == nil && !math.IsNaN(amount) && !math.IsInf(amount, 0) && amount == float64(expectedAmount)
}

func mapMidtransStatus(transactionStatus, fraudStatus string) string {
	switch transactionStatus {
	case "settlement":
		return "SETTLEMENT"
	case "capture":
		if fraudStatus == "accept" {
			return "SETTLEMENT"
		}
	case "expire", "cancel", "deny":
		return "FAILED"
	}
	return "PENDING"
}

func applyMidtransDonationStatus(ctx context.Context, reference, nextStatus string) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var campaignID, amount int64
	var currentStatus string
	if err := tx.QueryRow(ctx, `
		SELECT campaign_id,amount,status
		FROM donations
		WHERE payment_reference=$1
		FOR UPDATE
	`, reference).Scan(&campaignID, &amount, &currentStatus); err != nil {
		return err
	}
	if currentStatus == "SETTLEMENT" || (currentStatus == "FAILED" && nextStatus != "SETTLEMENT") ||
		(nextStatus == "PENDING" && currentStatus != "PENDING") {
		return tx.Commit(ctx)
	}
	if nextStatus == "SETTLEMENT" {
		if _, err := tx.Exec(ctx, "UPDATE donations SET status='SETTLEMENT' WHERE payment_reference=$1", reference); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE campaigns
			SET collected=collected+$1,
				status=CASE WHEN collected+$1 >= target THEN 'SELESAI' ELSE status END
			WHERE id=$2
		`, amount, campaignID); err != nil {
			return err
		}
		if err := addOutboxEvent(ctx, tx, "donation.created", map[string]interface{}{
			"event":       "NEW_DONATION",
			"campaign_id": campaignID,
			"donor_name":  "Donatur",
			"amount":      amount,
		}); err != nil {
			return err
		}
	} else if nextStatus == "FAILED" && currentStatus == "PENDING" {
		if _, err := tx.Exec(ctx, "UPDATE donations SET status='FAILED' WHERE payment_reference=$1", reference); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
