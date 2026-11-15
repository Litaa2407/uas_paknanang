package main

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
