package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type CampaignQuestion struct {
	ID             int64
	Question       string
	AskedByName    string
	CreatedAt      string
	Answer         string
	AnsweredByName string
	AnsweredAt     string
}

type CampaignFundUpdate struct {
	ID                  int64
	Amount              int64
	Purpose             string
	Description         string
	CreatedByName       string
	CreatedAt           string
	HasEvidence         bool
	EvidenceContentType string
}

type CampaignDonor struct {
	Name      string
	Amount    int64
	CreatedAt string
}

func listCampaignQuestions(ctx context.Context, campaignID int64) ([]CampaignQuestion, error) {
	rows, err := db.Query(ctx, `
		SELECT q.id,q.question,q.created_at::text,COALESCE(q.answer,''),
			COALESCE(a.name,''),COALESCE(q.answered_at::text,'')
		FROM campaign_questions q
		LEFT JOIN users a ON a.id=q.answered_by
		WHERE q.campaign_id=$1
		ORDER BY q.created_at DESC,q.id DESC
	`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	questions := []CampaignQuestion{}
	for rows.Next() {
		var question CampaignQuestion
		if err := rows.Scan(
			&question.ID,
			&question.Question,
			&question.CreatedAt,
			&question.Answer,
			&question.AnsweredByName,
			&question.AnsweredAt,
		); err != nil {
			return nil, err
		}
		question.AskedByName = "Donatur"
		questions = append(questions, question)
	}
	return questions, rows.Err()
}

func listCampaignFundUpdates(ctx context.Context, campaignID int64) ([]CampaignFundUpdate, error) {
	rows, err := db.Query(ctx, `
		SELECT u.id,u.amount,u.purpose,u.description,COALESCE(a.name,''),
			u.created_at::text,u.evidence_data IS NOT NULL,COALESCE(u.evidence_content_type,'')
		FROM campaign_fund_updates u
		LEFT JOIN users a ON a.id=u.created_by
		WHERE u.campaign_id=$1
		ORDER BY u.created_at DESC,u.id DESC
	`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	updates := []CampaignFundUpdate{}
	for rows.Next() {
		var update CampaignFundUpdate
		if err := rows.Scan(
			&update.ID,
			&update.Amount,
			&update.Purpose,
			&update.Description,
			&update.CreatedByName,
			&update.CreatedAt,
			&update.HasEvidence,
			&update.EvidenceContentType,
		); err != nil {
			return nil, err
		}
		updates = append(updates, update)
	}
	return updates, rows.Err()
}

func listCampaignDonors(ctx context.Context, campaignID int64) ([]CampaignDonor, error) {
	rows, err := db.Query(ctx, `
		SELECT amount,created_at::text
		FROM donations
		WHERE campaign_id=$1 AND status='SETTLEMENT'
		ORDER BY created_at DESC,id DESC
		LIMIT 100
	`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	donors := []CampaignDonor{}
	for rows.Next() {
		var donor CampaignDonor
		donor.Name = "Donatur"
		if err := rows.Scan(&donor.Amount, &donor.CreatedAt); err != nil {
			return nil, err
		}
		donors = append(donors, donor)
	}
	return donors, rows.Err()
}

func countCampaignDonors(ctx context.Context, campaignID int64) (int64, error) {
	var count int64
	err := db.QueryRow(ctx, "SELECT COUNT(*) FROM donations WHERE campaign_id=$1 AND status='SETTLEMENT'", campaignID).Scan(&count)
	return count, err
}

func ensureCampaignEngagementSchema(ctx context.Context, pool *pgxpool.Pool) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS campaign_questions (
			id BIGSERIAL PRIMARY KEY,
			campaign_id BIGINT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
			user_id BIGINT NOT NULL REFERENCES users(id),
			question TEXT NOT NULL CHECK (char_length(question) BETWEEN 10 AND 2000),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			answer TEXT,
			answered_by BIGINT REFERENCES users(id),
			answered_at TIMESTAMPTZ,
			CHECK ((answer IS NULL AND answered_by IS NULL AND answered_at IS NULL) OR
				(answer IS NOT NULL AND answered_by IS NOT NULL AND answered_at IS NOT NULL))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_campaign_questions_campaign ON campaign_questions(campaign_id, created_at DESC, id DESC)`,
		`CREATE TABLE IF NOT EXISTS campaign_fund_updates (
			id BIGSERIAL PRIMARY KEY,
			campaign_id BIGINT NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
			created_by BIGINT NOT NULL REFERENCES users(id),
			amount BIGINT NOT NULL CHECK (amount > 0),
			purpose VARCHAR(120) NOT NULL,
			description TEXT NOT NULL,
			evidence_content_type VARCHAR(50),
			evidence_data BYTEA,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CHECK ((evidence_content_type IS NULL AND evidence_data IS NULL) OR
				(evidence_content_type IN ('image/jpeg','image/png','image/webp') AND evidence_data IS NOT NULL))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_campaign_fund_updates_campaign ON campaign_fund_updates(campaign_id, created_at DESC, id DESC)`,
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func createCampaignFundUpdate(w http.ResponseWriter, r *http.Request) {
	u, ok := r.Context().Value(userKey).(User)
	if !ok {
		http.Error(w, "autentikasi diperlukan", http.StatusUnauthorized)
		return
	}
	if u.Role != "ADMIN" && u.Role != "PENGELOLA" {
		http.Error(w, "hanya pengelola campaign atau Admin yang dapat mencatat penggunaan dana", http.StatusForbidden)
		return
	}
	campaignID, err := strconv.ParseInt(r.PathValue("campaignID"), 10, 64)
	if err != nil || campaignID <= 0 {
		http.NotFound(w, r)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 6<<20)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "total bukti dan form update dana maksimal 6 MB", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "form update dana tidak valid", http.StatusBadRequest)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	amount, err := strconv.ParseInt(strings.TrimSpace(r.FormValue("amount")), 10, 64)
	purpose := strings.TrimSpace(r.FormValue("purpose"))
	description := strings.TrimSpace(r.FormValue("description"))
	if err != nil || amount <= 0 || utf8.RuneCountInString(purpose) < 3 || utf8.RuneCountInString(purpose) > 120 ||
		utf8.RuneCountInString(description) < 5 || utf8.RuneCountInString(description) > 2000 {
		http.Error(w, "nominal, keperluan, atau keterangan penggunaan dana tidak valid", http.StatusBadRequest)
		return
	}

	var evidence uploadedCampaignMedia
	var hasEvidence bool
	if files := r.MultipartForm.File["evidence"]; len(files) > 0 {
		if len(files) != 1 {
			http.Error(w, "hanya satu foto bukti yang dapat diunggah", http.StatusBadRequest)
			return
		}
		evidence, err = readCampaignMedia(files[0], "IMAGE", maxCampaignPhotoSize)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if evidence.contentType != "image/jpeg" && evidence.contentType != "image/png" && evidence.contentType != "image/webp" {
			http.Error(w, "bukti harus berupa foto JPG, PNG, atau WebP", http.StatusBadRequest)
			return
		}
		hasEvidence = true
	}

	tx, err := db.Begin(r.Context())
	if err != nil {
		http.Error(w, "gagal memulai penyimpanan update dana", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	var ownerID sql.NullInt64
	var status string
	var collected int64
	err = tx.QueryRow(r.Context(),
		"SELECT owner_id,status,collected FROM campaigns WHERE id=$1 FOR UPDATE",
		campaignID,
	).Scan(&ownerID, &status, &collected)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "gagal memeriksa campaign", http.StatusInternalServerError)
		return
	}
	if u.Role != "ADMIN" && (u.Role != "PENGELOLA" || !ownerID.Valid || ownerID.Int64 != u.ID) {
		http.Error(w, "hanya pengelola campaign atau Admin yang dapat mencatat penggunaan dana", http.StatusForbidden)
		return
	}
	if status != "AKTIF" {
		http.Error(w, "update penggunaan dana hanya dapat ditambahkan pada campaign aktif", http.StatusConflict)
		return
	}

	var spent int64
	if err := tx.QueryRow(r.Context(),
		"SELECT COALESCE(SUM(amount),0) FROM campaign_fund_updates WHERE campaign_id=$1",
		campaignID,
	).Scan(&spent); err != nil {
		http.Error(w, "gagal memeriksa penggunaan dana sebelumnya", http.StatusInternalServerError)
		return
	}
	if amount > collected-spent {
		http.Error(w, "nominal update melebihi dana terkumpul yang belum dilaporkan penggunaannya", http.StatusConflict)
		return
	}

	update := CampaignFundUpdate{Amount: amount, Purpose: purpose, Description: description, CreatedByName: u.Name, HasEvidence: hasEvidence}
	var evidenceType interface{}
	var evidenceData interface{}
	if hasEvidence {
		evidenceType = evidence.contentType
		evidenceData = evidence.data
		update.EvidenceContentType = evidence.contentType
	}
	if err := tx.QueryRow(r.Context(), `
		INSERT INTO campaign_fund_updates(campaign_id,created_by,amount,purpose,description,evidence_content_type,evidence_data)
		VALUES($1,$2,$3,$4,$5,$6,$7)
		RETURNING id,created_at::text
	`, campaignID, u.ID, amount, purpose, description, evidenceType, evidenceData).Scan(&update.ID, &update.CreatedAt); err != nil {
		http.Error(w, "gagal menyimpan update penggunaan dana", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "gagal menyelesaikan penyimpanan update dana", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, update)
}

func serveCampaignFundEvidence(w http.ResponseWriter, r *http.Request) {
	campaignID, err := strconv.ParseInt(r.PathValue("campaignID"), 10, 64)
	if err != nil || campaignID <= 0 {
		http.NotFound(w, r)
		return
	}
	updateID, err := strconv.ParseInt(r.PathValue("updateID"), 10, 64)
	if err != nil || updateID <= 0 {
		http.NotFound(w, r)
		return
	}

	var status string
	var ownerID sql.NullInt64
	var contentType string
	var data []byte
	err = db.QueryRow(r.Context(), `
		SELECT c.status,c.owner_id,u.evidence_content_type,u.evidence_data
		FROM campaigns c
		JOIN campaign_fund_updates u ON u.campaign_id=c.id
		WHERE c.id=$1 AND u.id=$2 AND u.evidence_data IS NOT NULL
	`, campaignID, updateID).Scan(&status, &ownerID, &contentType, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "gagal mengambil bukti penggunaan dana", http.StatusInternalServerError)
		return
	}

	u, authenticated := r.Context().Value(userKey).(User)
	if status != "AKTIF" && status != "SELESAI" && (!authenticated || (u.Role != "ADMIN" && (u.Role != "PENGELOLA" || !ownerID.Valid || u.ID != ownerID.Int64))) {
		http.NotFound(w, r)
		return
	}
	if status == "AKTIF" || status == "SELESAI" {
		w.Header().Set("Cache-Control", "public, max-age=300")
	} else {
		w.Header().Set("Cache-Control", "private, no-store")
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", "inline")
	if _, err := w.Write(data); err != nil {
		log.Printf("gagal mengirim bukti penggunaan dana: %v", err)
	}
}

func campaignCanBeManaged(ctx context.Context, campaignID int64, user User) (bool, error) {
	if user.Role == "ADMIN" {
		return true, nil
	}
	if user.Role != "PENGELOLA" {
		return false, nil
	}
	var ownerID sql.NullInt64
	err := db.QueryRow(ctx, "SELECT owner_id FROM campaigns WHERE id=$1 AND status='AKTIF'", campaignID).Scan(&ownerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return ownerID.Valid && ownerID.Int64 == user.ID, err
}

func campaignCanBeAnswered(ctx context.Context, campaignID int64, user User) (bool, error) {
	if user.Role == "ADMIN" {
		return true, nil
	}
	var ownerID sql.NullInt64
	err := db.QueryRow(ctx, "SELECT owner_id FROM campaigns WHERE id=$1 AND status='AKTIF'", campaignID).Scan(&ownerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return canAnswerCampaignQuestions(user, ownerID), err
}

func canAnswerCampaignQuestions(user User, ownerID sql.NullInt64) bool {
	return user.Role == "ADMIN" || (ownerID.Valid && ownerID.Int64 == user.ID)
}
