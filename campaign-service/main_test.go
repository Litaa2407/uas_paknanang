package main

import (
	"bytes"
	"context"
	"crypto/sha512"
	"database/sql"
	"encoding/hex"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestGoogleLoginRedirect(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_ID", "test-google-client-id")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test-google-client-secret")
	t.Setenv("GOOGLE_REDIRECT_URL", "http://localhost:5173/auth/google/callback")
	t.Setenv("GOOGLE_COOKIE_SECURE", "false")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/auth/google", nil)
	googleLogin(recorder, request)

	if recorder.Code != http.StatusFound {
		t.Fatalf("googleLogin status = %d, want %d", recorder.Code, http.StatusFound)
	}
	location, err := url.Parse(recorder.Header().Get("Location"))
	if err != nil {
		t.Fatalf("invalid Google redirect URL: %v", err)
	}
	if location.Host != "accounts.google.com" {
		t.Fatalf("redirect host = %q, want accounts.google.com", location.Host)
	}
	if location.Query().Get("client_id") != "test-google-client-id" || location.Query().Get("response_type") != "code" {
		t.Fatal("Google authorization request is missing required parameters")
	}
	if location.Query().Get("state") == "" {
		t.Fatal("Google authorization request must include state")
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != "google_oauth_state" || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatal("Google OAuth state cookie is missing required security attributes")
	}
}

func TestGraphQLGetRejectsMutation(t *testing.T) {
	handler := graphQLHandler(buildSchema())
	request := httptest.NewRequest(http.MethodGet, "/graphql?query=mutation%20%7B%20createCampaign%20%7D", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET mutation status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
}

func TestGraphQLGetSupportsQuery(t *testing.T) {
	handler := graphQLHandler(buildSchema())
	request := httptest.NewRequest(http.MethodGet, "/graphql?query=%7B__typename%7D", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET query status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), "Query") {
		t.Fatalf("GET query response does not contain GraphQL root type: %s", recorder.Body.String())
	}
}

func TestMapMidtransStatus(t *testing.T) {
	tests := []struct {
		name        string
		transaction string
		fraud       string
		want        string
	}{
		{name: "settlement", transaction: "settlement", want: "SETTLEMENT"},
		{name: "accepted capture", transaction: "capture", fraud: "accept", want: "SETTLEMENT"},
		{name: "challenge capture stays pending", transaction: "capture", fraud: "challenge", want: "PENDING"},
		{name: "pending", transaction: "pending", want: "PENDING"},
		{name: "expired", transaction: "expire", want: "FAILED"},
		{name: "cancelled", transaction: "cancel", want: "FAILED"},
		{name: "denied", transaction: "deny", want: "FAILED"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := mapMidtransStatus(test.transaction, test.fraud); got != test.want {
				t.Fatalf("mapMidtransStatus() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestMidtransAmountMatches(t *testing.T) {
	tests := []struct {
		amount string
		want   bool
	}{
		{amount: "25000.00", want: true},
		{amount: "25000.01", want: false},
		{amount: "NaN", want: false},
		{amount: "invalid", want: false},
	}
	for _, test := range tests {
		if got := midtransAmountMatches(test.amount, 25000); got != test.want {
			t.Errorf("midtransAmountMatches(%q) = %v, want %v", test.amount, got, test.want)
		}
	}
}

func TestAvailableDonationAmount(t *testing.T) {
	tests := []struct {
		name      string
		target    int64
		collected int64
		pending   int64
		want      int64
	}{
		{name: "remaining target", target: 500000, collected: 300000, want: 200000},
		{name: "subtract pending donations", target: 500000, collected: 300000, pending: 100000, want: 100000},
		{name: "target already met", target: 500000, collected: 500000, want: 0},
		{name: "pending donations exceed remaining target", target: 500000, collected: 300000, pending: 250000, want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := availableDonationAmount(test.target, test.collected, test.pending); got != test.want {
				t.Fatalf("availableDonationAmount() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestFormatRupiah(t *testing.T) {
	if got := formatRupiah(85000); got != "85.000" {
		t.Fatalf("formatRupiah() = %q, want %q", got, "85.000")
	}
}

func TestMidtransNotificationSignature(t *testing.T) {
	sum := sha512.Sum512([]byte("ORDER12320010000.00server-key"))
	want := hex.EncodeToString(sum[:])
	if got := midtransNotificationSignature("ORDER123", "200", "10000.00", "server-key"); got != want {
		t.Fatalf("midtransNotificationSignature() = %q, want %q", got, want)
	}
}

func TestValidMidtransRedirectURL(t *testing.T) {
	if !validMidtransRedirectURL("https://app.sandbox.midtrans.com/snap/v2/vtweb/example") {
		t.Fatal("sandbox Snap redirect should be accepted")
	}
	for _, rawURL := range []string{
		"http://app.sandbox.midtrans.com/snap",
		"https://app.midtrans.com/snap",
		"https://app.sandbox.midtrans.com.attacker.example/snap",
	} {
		if validMidtransRedirectURL(rawURL) {
			t.Errorf("validMidtransRedirectURL(%q) = true, want false", rawURL)
		}
	}
}

func TestCanAnswerCampaignQuestions(t *testing.T) {
	tests := []struct {
		name    string
		user    User
		ownerID sql.NullInt64
		want    bool
	}{
		{name: "campaign owner with donor role", user: User{ID: 7, Role: "DONATUR"}, ownerID: sql.NullInt64{Int64: 7, Valid: true}, want: true},
		{name: "campaign owner with manager role", user: User{ID: 7, Role: "PENGELOLA"}, ownerID: sql.NullInt64{Int64: 7, Valid: true}, want: true},
		{name: "admin", user: User{ID: 9, Role: "ADMIN"}, ownerID: sql.NullInt64{Int64: 7, Valid: true}, want: true},
		{name: "other user", user: User{ID: 8, Role: "DONATUR"}, ownerID: sql.NullInt64{Int64: 7, Valid: true}, want: false},
		{name: "campaign has no owner", user: User{ID: 7, Role: "DONATUR"}, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := canAnswerCampaignQuestions(test.user, test.ownerID); got != test.want {
				t.Fatalf("canAnswerCampaignQuestions() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestDetectCampaignMediaType(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{
			name: "jpeg",
			data: []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00},
			want: "image/jpeg",
		},
		{
			name: "png",
			data: []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a},
			want: "image/png",
		},
		{
			name: "webp",
			data: []byte("RIFF\x00\x00\x00\x00WEBP"),
			want: "image/webp",
		},
		{
			name: "mp4",
			data: []byte("\x00\x00\x00\x18ftypisom"),
			want: "video/mp4",
		},
		{
			name: "unsupported",
			data: []byte("not a supported image or video"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := detectCampaignMediaType(test.data); got != test.want {
				t.Fatalf("detectCampaignMediaType() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCreateCampaignRequiresPhoto(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range map[string]string{
		"title":       "Campaign test",
		"description": "Deskripsi test",
		"target":      "1000",
	} {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/campaigns", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request = request.WithContext(context.WithValue(request.Context(), userKey, User{ID: 1}))
	response := httptest.NewRecorder()

	createCampaignWithMedia(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if !strings.Contains(response.Body.String(), "minimal 1 foto") {
		t.Fatalf("response = %q, want required-photo message", response.Body.String())
	}
}

func TestUploadCampaignMediaRequiresPhoto(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/campaigns/1/media", &body)
	request.SetPathValue("campaignID", "1")
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request = request.WithContext(context.WithValue(request.Context(), userKey, User{ID: 1, Role: "DONATUR"}))
	response := httptest.NewRecorder()

	uploadCampaignMedia(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if !strings.Contains(response.Body.String(), "minimal 1 foto") {
		t.Fatalf("response = %q, want required-photo message", response.Body.String())
	}
}

func TestCreateCampaignFundUpdateRequiresManagerRole(t *testing.T) {
	tests := []struct {
		name       string
		user       interface{}
		wantStatus int
	}{
		{name: "unauthenticated", wantStatus: http.StatusUnauthorized},
		{name: "donor", user: User{ID: 1, Role: "DONATUR"}, wantStatus: http.StatusForbidden},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/campaigns/1/fund-updates", strings.NewReader(""))
			request.SetPathValue("campaignID", "1")
			if test.user != nil {
				request = request.WithContext(context.WithValue(request.Context(), userKey, test.user))
			}
			response := httptest.NewRecorder()

			createCampaignFundUpdate(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
		})
	}
}

func TestSimulateDonationRequiresDonorRole(t *testing.T) {
	tests := []struct {
		name       string
		user       interface{}
		wantStatus int
	}{
		{name: "unauthenticated", wantStatus: http.StatusUnauthorized},
		{name: "admin", user: User{ID: 1, Role: "ADMIN"}, wantStatus: http.StatusForbidden},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/donations/simulate", strings.NewReader(`{"campaign_id":1,"amount":1000}`))
			if test.user != nil {
				request = request.WithContext(context.WithValue(request.Context(), userKey, test.user))
			}
			response := httptest.NewRecorder()

			simulateDonation(response, request)

			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
		})
	}
}

func TestSimulateDonationRejectsInvalidAmount(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/donations/simulate", strings.NewReader(`{"campaign_id":1,"amount":0,"donor_name":"Donatur Tes","donor_email":"donatur@example.com","donor_phone":"+62812345678"}`))
	request = request.WithContext(context.WithValue(request.Context(), userKey, User{ID: 1, Role: "DONATUR"}))
	response := httptest.NewRecorder()

	simulateDonation(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestDonorContactValidation(t *testing.T) {
	tests := []struct {
		name      string
		email     string
		phone     string
		wantValid bool
	}{
		{name: "valid contact", email: "donatur@example.com", phone: "+62 (812) 3456-7890", wantValid: true},
		{name: "email with display name", email: "Donatur <donatur@example.com>", phone: "081234567890"},
		{name: "invalid email", email: "bukan-email", phone: "081234567890"},
		{name: "too short phone", email: "donatur@example.com", phone: "1234567"},
		{name: "phone with letters", email: "donatur@example.com", phone: "0812abc"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			valid := validDonorEmail(test.email) && validDonorPhone(test.phone)
			if valid != test.wantValid {
				t.Fatalf("contact validation = %v, want %v", valid, test.wantValid)
			}
		})
	}
}

func TestSimulateDonationRejectsInvalidContact(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/donations/simulate", strings.NewReader(`{"campaign_id":1,"amount":10000,"donor_name":"Donatur Tes","donor_email":"bukan-email","donor_phone":"081234567890"}`))
	request = request.WithContext(context.WithValue(request.Context(), userKey, User{ID: 1, Role: "DONATUR"}))
	response := httptest.NewRecorder()

	simulateDonation(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}
