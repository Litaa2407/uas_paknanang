package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
	"github.com/graphql-go/graphql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type ctxKey string

const userKey ctxKey = "user"

const (
	userTokenIssuer      = "crowdfunding-user"
	userTokenAudience    = "crowdfunding-api"
	serviceTokenIssuer   = "crowdfunding-oauth"
	serviceTokenAudience = "crowdfunding-services"
	serviceTokenScope    = "service:proxy"
	serviceTokenTTL      = 15 * time.Minute
)

type User struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type Campaign struct {
	ID               int64           `json:"id"`
	Title            string          `json:"title"`
	Description      string          `json:"description"`
	Target           int64           `json:"target"`
	Collected        int64           `json:"collected"`
	Status           string          `json:"status"`
	SubmittedByName  string          `json:"submittedByName"`
	SubmittedByEmail string          `json:"submittedByEmail"`
	CreatedAt        string          `json:"createdAt"`
	Media            []CampaignMedia `json:"-"`
}
type CampaignMedia struct {
	ID          string `json:"id"`
	MediaType   string `json:"mediaType"`
	ContentType string `json:"contentType"`
}

var (
	db                   *pgxpool.Pool
	jwtSecret            []byte
	paymentWebhookSecret []byte
	oauthClientID        string
	oauthClientSecret    string
	mtlsEnabled          bool
)

func main() {
	jwtSecret = []byte(os.Getenv("JWT_SECRET"))
	paymentWebhookSecret = []byte(os.Getenv("PAYMENT_WEBHOOK_SECRET"))
	oauthClientID = strings.TrimSpace(os.Getenv("OAUTH_CLIENT_ID"))
	oauthClientSecret = os.Getenv("OAUTH_CLIENT_SECRET")
	mtlsEnabled = strings.EqualFold(os.Getenv("MTLS_ENABLED"), "true")

	if len(jwtSecret) < 32 {
		log.Fatal("JWT_SECRET wajib berisi minimal 32 karakter")
	}
	if len(paymentWebhookSecret) < 32 {
		log.Fatal("PAYMENT_WEBHOOK_SECRET wajib berisi minimal 32 karakter")
	}
	if oauthClientID == "" || len(oauthClientSecret) < 32 {
		log.Fatal("OAUTH_CLIENT_ID wajib diisi dan OAUTH_CLIENT_SECRET minimal 32 karakter")
	}
	if strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		log.Fatal("DATABASE_URL wajib diisi")
	}
	if strings.TrimSpace(os.Getenv("RABBITMQ_URL")) == "" {
		log.Fatal("RABBITMQ_URL wajib diisi")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	poolConfig, err := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("DATABASE_URL tidak valid: %v", err)
	}
	poolConfig.MaxConns = 20
	poolConfig.MinConns = 1
	poolConfig.MaxConnLifetime = 30 * time.Minute
	poolConfig.MaxConnIdleTime = 5 * time.Minute

	db, err = pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		log.Fatalf("gagal membuat koneksi database: %v", err)
	}
	defer db.Close()

	pingCtx, cancelPing := context.WithTimeout(ctx, 10*time.Second)
	defer cancelPing()
	if err := db.Ping(pingCtx); err != nil {
		log.Fatalf("database tidak dapat diakses: %v", err)
	}
	if err := ensureGoogleOAuthSchema(ctx, db); err != nil {
		log.Fatalf("gagal menyiapkan skema Google OAuth: %v", err)
	}
	if err := ensureOutboxSchema(ctx, db); err != nil {
		log.Fatalf("gagal menyiapkan tabel outbox: %v", err)
	}
	if err := ensureCampaignEngagementSchema(ctx, db); err != nil {
		log.Fatalf("gagal menyiapkan tabel pertanyaan dan update dana: %v", err)
	}
	if err := ensureMidtransSchema(ctx, db); err != nil {
		log.Fatalf("gagal menyiapkan skema pembayaran: %v", err)
	}

	var workerWG sync.WaitGroup
	workerWG.Add(1)
	go func() {
		defer workerWG.Done()
		runOutboxPublisher(ctx, db, os.Getenv("RABBITMQ_URL"))
	}()

	schema := buildSchema()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("POST /oauth/token", oauthToken)
	mux.HandleFunc("POST /auth/register", register)
	mux.HandleFunc("POST /auth/login", login)
	mux.HandleFunc("GET /auth/google", googleLogin)
	mux.HandleFunc("GET /auth/google/callback", googleCallback)
	mux.Handle("POST /campaigns", authMiddleware(http.HandlerFunc(createCampaignWithMedia)))
	mux.Handle("POST /campaigns/{campaignID}/media", authMiddleware(http.HandlerFunc(uploadCampaignMedia)))
	mux.Handle("POST /campaigns/{campaignID}/fund-updates", authMiddleware(http.HandlerFunc(createCampaignFundUpdate)))
	mux.Handle("GET /campaigns/{campaignID}/fund-updates/{updateID}/evidence", authMiddleware(http.HandlerFunc(serveCampaignFundEvidence)))
	mux.Handle("POST /donations", authMiddleware(http.HandlerFunc(createMidtransDonation)))
	mux.Handle("GET /donations/mine", authMiddleware(http.HandlerFunc(listDonorDonationProgress)))
	mux.Handle("GET /donations/pending", authMiddleware(http.HandlerFunc(listPendingCampaignDonations)))
	mux.Handle("POST /donations/{reference}/cancel", authMiddleware(http.HandlerFunc(cancelMidtransDonation)))
	mux.Handle("GET /donations/{reference}/status", authMiddleware(http.HandlerFunc(getMidtransDonationStatus)))
	mux.Handle("POST /donations/simulate", authMiddleware(http.HandlerFunc(simulateDonation)))
	mux.HandleFunc("POST /webhook/midtrans", midtransPaymentWebhook)
	mux.Handle("GET /campaigns/{campaignID}/media/{mediaID}", authMiddleware(http.HandlerFunc(serveCampaignMedia)))
	mux.Handle("/graphql", graphQLHandler(schema))
	mux.HandleFunc("POST /webhook/payment", paymentWebhook)

	handler := securityHeaders(requireMTLS(internalAuthMiddleware(mux)))
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8443"
	}

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	if mtlsEnabled {
		server.TLSConfig, err = serverTLSConfig()
		if err != nil {
			log.Fatalf("konfigurasi mTLS tidak valid: %v", err)
		}
	}

	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("Campaign Service listening on :%s (mTLS=%t)", port, mtlsEnabled)
		if mtlsEnabled {
			serverErrors <- server.ListenAndServeTLS(os.Getenv("TLS_CERT_FILE"), os.Getenv("TLS_KEY_FILE"))
			return
		}
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Campaign Service berhenti: %v", err)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown HTTP server: %v", err)
		}
		workerWG.Wait()
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := db.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unhealthy", "service": "campaign", "database": "down"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "campaign", "database": "up"})
}

func buildSchema() graphql.Schema {
	campaignMediaType := graphql.NewObject(graphql.ObjectConfig{Name: "CampaignMedia", Fields: graphql.Fields{
		"id":          &graphql.Field{Type: graphql.ID},
		"mediaType":   &graphql.Field{Type: graphql.String},
		"contentType": &graphql.Field{Type: graphql.String},
	}})
	questionType := graphql.NewObject(graphql.ObjectConfig{Name: "CampaignQuestion", Fields: graphql.Fields{
		"id":       &graphql.Field{Type: graphql.Int, Resolve: func(p graphql.ResolveParams) (interface{}, error) { return p.Source.(CampaignQuestion).ID, nil }},
		"question": &graphql.Field{Type: graphql.String, Resolve: func(p graphql.ResolveParams) (interface{}, error) { return p.Source.(CampaignQuestion).Question, nil }},
		"askedByName": &graphql.Field{Type: graphql.String, Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return p.Source.(CampaignQuestion).AskedByName, nil
		}},
		"createdAt": &graphql.Field{Type: graphql.String, Resolve: func(p graphql.ResolveParams) (interface{}, error) { return p.Source.(CampaignQuestion).CreatedAt, nil }},
		"answer":    &graphql.Field{Type: graphql.String, Resolve: func(p graphql.ResolveParams) (interface{}, error) { return p.Source.(CampaignQuestion).Answer, nil }},
		"answeredByName": &graphql.Field{Type: graphql.String, Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return p.Source.(CampaignQuestion).AnsweredByName, nil
		}},
		"answeredAt": &graphql.Field{Type: graphql.String, Resolve: func(p graphql.ResolveParams) (interface{}, error) { return p.Source.(CampaignQuestion).AnsweredAt, nil }},
	}})
	fundUpdateType := graphql.NewObject(graphql.ObjectConfig{Name: "CampaignFundUpdate", Fields: graphql.Fields{
		"id":      &graphql.Field{Type: graphql.Int, Resolve: func(p graphql.ResolveParams) (interface{}, error) { return p.Source.(CampaignFundUpdate).ID, nil }},
		"amount":  &graphql.Field{Type: graphql.Int, Resolve: func(p graphql.ResolveParams) (interface{}, error) { return p.Source.(CampaignFundUpdate).Amount, nil }},
		"purpose": &graphql.Field{Type: graphql.String, Resolve: func(p graphql.ResolveParams) (interface{}, error) { return p.Source.(CampaignFundUpdate).Purpose, nil }},
		"description": &graphql.Field{Type: graphql.String, Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return p.Source.(CampaignFundUpdate).Description, nil
		}},
		"createdByName": &graphql.Field{Type: graphql.String, Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return p.Source.(CampaignFundUpdate).CreatedByName, nil
		}},
		"createdAt": &graphql.Field{Type: graphql.String, Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return p.Source.(CampaignFundUpdate).CreatedAt, nil
		}},
		"hasEvidence": &graphql.Field{Type: graphql.Boolean, Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return p.Source.(CampaignFundUpdate).HasEvidence, nil
		}},
		"evidenceContentType": &graphql.Field{Type: graphql.String, Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return p.Source.(CampaignFundUpdate).EvidenceContentType, nil
		}},
	}})
	donorType := graphql.NewObject(graphql.ObjectConfig{Name: "CampaignDonor", Fields: graphql.Fields{
		"name":      &graphql.Field{Type: graphql.String, Resolve: func(p graphql.ResolveParams) (interface{}, error) { return p.Source.(CampaignDonor).Name, nil }},
		"amount":    &graphql.Field{Type: graphql.Int, Resolve: func(p graphql.ResolveParams) (interface{}, error) { return p.Source.(CampaignDonor).Amount, nil }},
		"createdAt": &graphql.Field{Type: graphql.String, Resolve: func(p graphql.ResolveParams) (interface{}, error) { return p.Source.(CampaignDonor).CreatedAt, nil }},
	}})
	campaignType := graphql.NewObject(graphql.ObjectConfig{Name: "Campaign", Fields: graphql.Fields{
		"id":          &graphql.Field{Type: graphql.Int},
		"title":       &graphql.Field{Type: graphql.String},
		"description": &graphql.Field{Type: graphql.String},
		"target":      &graphql.Field{Type: graphql.Int},
		"collected":   &graphql.Field{Type: graphql.Int},
		"status":      &graphql.Field{Type: graphql.String},
		"submittedByName": &graphql.Field{Type: graphql.String, Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			campaign, ok := p.Source.(Campaign)
			if !ok {
				return "", nil
			}
			return campaign.SubmittedByName, nil
		}},
		"submittedByEmail": &graphql.Field{Type: graphql.String, Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			campaign, ok := p.Source.(Campaign)
			if !ok {
				return "", nil
			}
			return campaign.SubmittedByEmail, nil
		}},
		"createdAt": &graphql.Field{Type: graphql.String, Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			campaign, ok := p.Source.(Campaign)
			if !ok {
				return "", nil
			}
			return campaign.CreatedAt, nil
		}},
		"media": &graphql.Field{Type: graphql.NewList(campaignMediaType), Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			campaign, ok := p.Source.(Campaign)
			if !ok {
				return nil, nil
			}
			if campaign.Media != nil {
				return campaign.Media, nil
			}
			rows, err := db.Query(p.Context, "SELECT id,media_type,content_type FROM campaign_media WHERE campaign_id=$1 ORDER BY id", campaign.ID)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			media := []CampaignMedia{}
			for rows.Next() {
				var item CampaignMedia
				var id int64
				if err = rows.Scan(&id, &item.MediaType, &item.ContentType); err != nil {
					return nil, err
				}
				item.ID = strconv.FormatInt(id, 10)
				media = append(media, item)
			}
			return media, rows.Err()
		}},
		"canManage": &graphql.Field{Type: graphql.Boolean, Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			campaign, ok := p.Source.(Campaign)
			if !ok {
				return false, nil
			}
			user, authenticated := currentUser(p)
			if !authenticated {
				return false, nil
			}
			return campaignCanBeManaged(p.Context, campaign.ID, user)
		}},
		"canAnswerQuestions": &graphql.Field{Type: graphql.Boolean, Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			campaign, ok := p.Source.(Campaign)
			if !ok {
				return false, nil
			}
			user, authenticated := currentUser(p)
			if !authenticated {
				return false, nil
			}
			return campaignCanBeAnswered(p.Context, campaign.ID, user)
		}},
		"questions": &graphql.Field{Type: graphql.NewList(questionType), Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			campaign, ok := p.Source.(Campaign)
			if !ok {
				return nil, nil
			}
			return listCampaignQuestions(p.Context, campaign.ID)
		}},
		"fundUpdates": &graphql.Field{Type: graphql.NewList(fundUpdateType), Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			campaign, ok := p.Source.(Campaign)
			if !ok {
				return nil, nil
			}
			return listCampaignFundUpdates(p.Context, campaign.ID)
		}},
		"donors": &graphql.Field{Type: graphql.NewList(donorType), Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			campaign, ok := p.Source.(Campaign)
			if !ok {
				return nil, nil
			}
			return listCampaignDonors(p.Context, campaign.ID)
		}},
		"donorCount": &graphql.Field{Type: graphql.Int, Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			campaign, ok := p.Source.(Campaign)
			if !ok {
				return 0, nil
			}
			return countCampaignDonors(p.Context, campaign.ID)
		}},
	}})

	query := graphql.NewObject(graphql.ObjectConfig{Name: "Query", Fields: graphql.Fields{
		"campaigns": &graphql.Field{Type: graphql.NewList(campaignType), Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			rows, err := db.Query(p.Context, "SELECT id,title,description,target,collected,status FROM campaigns WHERE status IN ('AKTIF','SELESAI') ORDER BY id DESC")
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			out := []Campaign{}
			for rows.Next() {
				var c Campaign
				if err := rows.Scan(&c.ID, &c.Title, &c.Description, &c.Target, &c.Collected, &c.Status); err != nil {
					return nil, err
				}
				out = append(out, c)
			}
			if err := rows.Err(); err != nil {
				return nil, err
			}
			return loadCampaignMedia(p.Context, out)
		}},
		"campaign": &graphql.Field{Type: campaignType, Args: graphql.FieldConfigArgument{
			"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.Int)},
		}, Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			var c Campaign
			err := db.QueryRow(p.Context, "SELECT id,title,description,target,collected,status FROM campaigns WHERE id=$1 AND status IN ('AKTIF','SELESAI')", p.Args["id"]).Scan(&c.ID, &c.Title, &c.Description, &c.Target, &c.Collected, &c.Status)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, nil
			}
			return c, err
		}},
		"pendingCampaigns": &graphql.Field{Type: graphql.NewList(campaignType), Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			if _, err := requireRole(p, "ADMIN"); err != nil {
				return nil, err
			}
			rows, err := db.Query(p.Context, `
				SELECT c.id,c.title,c.description,c.target,c.collected,c.status,
					COALESCE(u.name,''),COALESCE(u.email,''),c.created_at::text
				FROM campaigns c LEFT JOIN users u ON u.id=c.owner_id
				WHERE c.status='MENUNGGU' ORDER BY c.created_at,c.id
			`)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			out := []Campaign{}
			for rows.Next() {
				var c Campaign
				if err := rows.Scan(&c.ID, &c.Title, &c.Description, &c.Target, &c.Collected, &c.Status, &c.SubmittedByName, &c.SubmittedByEmail, &c.CreatedAt); err != nil {
					return nil, err
				}
				out = append(out, c)
			}
			if err := rows.Err(); err != nil {
				return nil, err
			}
			return loadCampaignMedia(p.Context, out)
		}},
		"pendingCampaign": &graphql.Field{Type: campaignType, Args: graphql.FieldConfigArgument{
			"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.Int)},
		}, Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			if _, err := requireRole(p, "ADMIN"); err != nil {
				return nil, err
			}
			var c Campaign
			err := db.QueryRow(p.Context, `
				SELECT c.id,c.title,c.description,c.target,c.collected,c.status,
					COALESCE(u.name,''),COALESCE(u.email,''),c.created_at::text
				FROM campaigns c LEFT JOIN users u ON u.id=c.owner_id
				WHERE c.id=$1 AND c.status='MENUNGGU'
			`, p.Args["id"]).Scan(&c.ID, &c.Title, &c.Description, &c.Target, &c.Collected, &c.Status, &c.SubmittedByName, &c.SubmittedByEmail, &c.CreatedAt)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, nil
			}
			return c, err
		}},
	}})

	mutation := graphql.NewObject(graphql.ObjectConfig{Name: "Mutation", Fields: graphql.Fields{
		"createCampaign": &graphql.Field{Type: campaignType, Args: graphql.FieldConfigArgument{
			"title":       &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
			"description": &graphql.ArgumentConfig{Type: graphql.String},
			"target":      &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.Int)},
		}, Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			u, ok := currentUser(p)
			if !ok {
				return nil, errors.New("autentikasi diperlukan")
			}
			title := strings.TrimSpace(p.Args["title"].(string))
			target := int64(p.Args["target"].(int))
			description, _ := p.Args["description"].(string)
			description = strings.TrimSpace(description)
			if len(title) < 3 || len(title) > 100 || target <= 0 || len(description) > 5000 {
				return nil, errors.New("judul, deskripsi, atau target tidak valid")
			}

			tx, err := db.Begin(p.Context)
			if err != nil {
				return nil, err
			}
			defer tx.Rollback(p.Context)

			var c Campaign
			err = tx.QueryRow(p.Context, "INSERT INTO campaigns(title,description,target,status,owner_id) VALUES($1,$2,$3,'MENUNGGU',$4) RETURNING id,title,description,target,collected,status", title, description, target, u.ID).Scan(&c.ID, &c.Title, &c.Description, &c.Target, &c.Collected, &c.Status)
			if err != nil {
				return nil, err
			}
			if err := addOutboxEvent(p.Context, tx, "campaign.created", map[string]interface{}{"event": "CAMPAIGN_CREATED", "campaign": c}); err != nil {
				return nil, err
			}
			if err := tx.Commit(p.Context); err != nil {
				return nil, err
			}
			return c, nil
		}},
		"approveCampaign": &graphql.Field{Type: campaignType, Args: graphql.FieldConfigArgument{
			"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.Int)},
		}, Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			if _, err := requireRole(p, "ADMIN"); err != nil {
				return nil, err
			}
			tx, err := db.Begin(p.Context)
			if err != nil {
				return nil, err
			}
			defer tx.Rollback(p.Context)

			var c Campaign
			err = tx.QueryRow(p.Context, "UPDATE campaigns SET status='AKTIF' WHERE id=$1 AND status='MENUNGGU' RETURNING id,title,description,target,collected,status", p.Args["id"]).Scan(&c.ID, &c.Title, &c.Description, &c.Target, &c.Collected, &c.Status)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, errors.New("campaign tidak ditemukan atau sudah diproses")
			}
			if err != nil {
				return nil, err
			}
			if err := addOutboxEvent(p.Context, tx, "campaign.approved", map[string]interface{}{"event": "CAMPAIGN_APPROVED", "campaign": c}); err != nil {
				return nil, err
			}
			if err := tx.Commit(p.Context); err != nil {
				return nil, err
			}
			return c, nil
		}},
		"askCampaignQuestion": &graphql.Field{Type: questionType, Args: graphql.FieldConfigArgument{
			"campaignID": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.Int)},
			"question":   &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
		}, Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			user, err := requireRole(p, "DONATUR")
			if err != nil {
				return nil, err
			}
			campaignID := int64(p.Args["campaignID"].(int))
			questionText := strings.TrimSpace(p.Args["question"].(string))
			if utf8.RuneCountInString(questionText) < 10 || utf8.RuneCountInString(questionText) > 2000 {
				return nil, errors.New("pertanyaan harus berisi 10 sampai 2000 karakter")
			}
			var question CampaignQuestion
			question.AskedByName = user.Name
			err = db.QueryRow(p.Context, `
				INSERT INTO campaign_questions(campaign_id,user_id,question)
				SELECT id,$2,$3 FROM campaigns WHERE id=$1 AND status='AKTIF'
				RETURNING id,question,created_at::text
			`, campaignID, user.ID, questionText).Scan(&question.ID, &question.Question, &question.CreatedAt)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, errors.New("campaign tidak ditemukan atau tidak aktif")
			}
			return question, err
		}},
		"answerCampaignQuestion": &graphql.Field{Type: questionType, Args: graphql.FieldConfigArgument{
			"questionID": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.Int)},
			"answer":     &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
		}, Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			user, ok := currentUser(p)
			if !ok {
				return nil, errors.New("autentikasi diperlukan")
			}
			questionID := int64(p.Args["questionID"].(int))
			answer := strings.TrimSpace(p.Args["answer"].(string))
			if utf8.RuneCountInString(answer) < 2 || utf8.RuneCountInString(answer) > 2000 {
				return nil, errors.New("jawaban harus berisi 2 sampai 2000 karakter")
			}
			var campaignID int64
			err := db.QueryRow(p.Context, `
				SELECT q.campaign_id FROM campaign_questions q
				JOIN campaigns c ON c.id=q.campaign_id
				WHERE q.id=$1 AND c.status='AKTIF'
			`, questionID).Scan(&campaignID)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, errors.New("pertanyaan tidak ditemukan atau campaign tidak aktif")
			}
			if err != nil {
				return nil, err
			}
			canAnswer, err := campaignCanBeAnswered(p.Context, campaignID, user)
			if err != nil {
				return nil, err
			}
			if !canAnswer {
				return nil, errors.New("hanya pemilik campaign atau Admin yang dapat menjawab")
			}
			var question CampaignQuestion
			question.AnsweredByName = user.Name
			err = db.QueryRow(p.Context, `
				UPDATE campaign_questions
				SET answer=$2,answered_by=$3,answered_at=NOW()
				WHERE id=$1 AND answer IS NULL
				RETURNING id,question,created_at::text,answer,answered_at::text
			`, questionID, answer, user.ID).Scan(
				&question.ID,
				&question.Question,
				&question.CreatedAt,
				&question.Answer,
				&question.AnsweredAt,
			)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, errors.New("pertanyaan ini sudah dijawab")
			}
			return question, err
		}},
	}})

	schema, err := graphql.NewSchema(graphql.SchemaConfig{Query: query, Mutation: mutation})
	if err != nil {
		log.Fatalf("gagal membangun GraphQL schema: %v", err)
	}
	return schema
}

func loadCampaignMedia(ctx context.Context, campaigns []Campaign) ([]Campaign, error) {
	if len(campaigns) == 0 {
		return campaigns, nil
	}

	indexByID := make(map[int64]int, len(campaigns))
	ids := make([]int64, len(campaigns))
	for index := range campaigns {
		campaigns[index].Media = []CampaignMedia{}
		indexByID[campaigns[index].ID] = index
		ids[index] = campaigns[index].ID
	}

	rows, err := db.Query(ctx, `
		SELECT campaign_id,id,media_type,content_type
		FROM campaign_media
		WHERE campaign_id = ANY($1)
		ORDER BY campaign_id,id
	`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var campaignID, mediaID int64
		var media CampaignMedia
		if err := rows.Scan(&campaignID, &mediaID, &media.MediaType, &media.ContentType); err != nil {
			return nil, err
		}
		if index, exists := indexByID[campaignID]; exists {
			media.ID = strconv.FormatInt(mediaID, 10)
			campaigns[index].Media = append(campaigns[index].Media, media)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return campaigns, nil
}

const (
	maxCampaignRequestSize = 52 << 20
	maxCampaignPhotoSize   = 5 << 20
	maxCampaignVideoSize   = 25 << 20
	maxCampaignPhotos      = 5
)

type uploadedCampaignMedia struct {
	mediaType   string
	contentType string
	data        []byte
}

func createCampaignWithMedia(w http.ResponseWriter, r *http.Request) {
	u, ok := r.Context().Value(userKey).(User)
	if !ok {
		http.Error(w, "autentikasi diperlukan", http.StatusUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxCampaignRequestSize)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "total upload maksimal 50 MB", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "form upload tidak valid", http.StatusBadRequest)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	title := strings.TrimSpace(r.FormValue("title"))
	description := strings.TrimSpace(r.FormValue("description"))
	target, err := strconv.ParseInt(r.FormValue("target"), 10, 64)
	if len(title) < 3 || len(title) > 100 || len(description) > 10000 || err != nil || target <= 0 {
		http.Error(w, "judul, deskripsi, atau target tidak valid", http.StatusBadRequest)
		return
	}

	photoHeaders := r.MultipartForm.File["photos"]
	if len(photoHeaders) == 0 {
		http.Error(w, "minimal 1 foto campaign wajib diunggah", http.StatusBadRequest)
		return
	}
	if len(photoHeaders) > maxCampaignPhotos {
		http.Error(w, "maksimal 5 foto dapat diunggah", http.StatusBadRequest)
		return
	}
	videoHeaders := r.MultipartForm.File["video"]
	if len(videoHeaders) > 1 {
		http.Error(w, "maksimal 1 video dapat diunggah", http.StatusBadRequest)
		return
	}

	media := make([]uploadedCampaignMedia, 0, len(photoHeaders)+len(videoHeaders))
	for _, fileHeader := range photoHeaders {
		item, err := readCampaignMedia(fileHeader, "IMAGE", maxCampaignPhotoSize)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if item.contentType != "image/jpeg" && item.contentType != "image/png" && item.contentType != "image/webp" {
			http.Error(w, "format foto harus JPG, PNG, atau WebP", http.StatusBadRequest)
			return
		}
		media = append(media, item)
	}
	for _, fileHeader := range videoHeaders {
		item, err := readCampaignMedia(fileHeader, "VIDEO", maxCampaignVideoSize)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if item.contentType != "video/mp4" {
			http.Error(w, "format video harus MP4", http.StatusBadRequest)
			return
		}
		media = append(media, item)
	}

	tx, err := db.Begin(r.Context())
	if err != nil {
		http.Error(w, "gagal memulai penyimpanan campaign", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	var campaign Campaign
	err = tx.QueryRow(r.Context(),
		"INSERT INTO campaigns(title,description,target,status,owner_id) VALUES($1,$2,$3,'MENUNGGU',$4) RETURNING id,title,description,target,collected,status",
		title, description, target, u.ID,
	).Scan(&campaign.ID, &campaign.Title, &campaign.Description, &campaign.Target, &campaign.Collected, &campaign.Status)
	if err != nil {
		http.Error(w, "gagal menyimpan campaign", http.StatusInternalServerError)
		return
	}

	for _, item := range media {
		if _, err = tx.Exec(r.Context(),
			"INSERT INTO campaign_media(campaign_id,media_type,content_type,file_data) VALUES($1,$2,$3,$4)",
			campaign.ID, item.mediaType, item.contentType, item.data,
		); err != nil {
			http.Error(w, "gagal menyimpan media campaign", http.StatusInternalServerError)
			return
		}
	}
	if err := addOutboxEvent(r.Context(), tx, "campaign.created", map[string]interface{}{
		"event":    "CAMPAIGN_CREATED",
		"campaign": campaign,
	}); err != nil {
		http.Error(w, "gagal membuat event campaign", http.StatusInternalServerError)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		http.Error(w, "gagal menyelesaikan penyimpanan campaign", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(campaign); err != nil {
		log.Printf("gagal menulis response campaign: %v", err)
	}
}

func uploadCampaignMedia(w http.ResponseWriter, r *http.Request) {
	u, ok := r.Context().Value(userKey).(User)
	if !ok {
		http.Error(w, "autentikasi diperlukan", http.StatusUnauthorized)
		return
	}

	campaignID, err := strconv.ParseInt(r.PathValue("campaignID"), 10, 64)
	if err != nil || campaignID <= 0 {
		http.NotFound(w, r)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxCampaignRequestSize)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, "total upload maksimal 50 MB", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "form upload tidak valid", http.StatusBadRequest)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	photoHeaders := r.MultipartForm.File["photos"]
	videoHeaders := r.MultipartForm.File["video"]
	if len(photoHeaders) == 0 {
		http.Error(w, "minimal 1 foto campaign wajib diunggah", http.StatusBadRequest)
		return
	}
	if len(photoHeaders) > maxCampaignPhotos {
		http.Error(w, "maksimal 5 foto dapat diunggah", http.StatusBadRequest)
		return
	}
	if len(videoHeaders) > 1 {
		http.Error(w, "maksimal 1 video dapat diunggah", http.StatusBadRequest)
		return
	}

	media := make([]uploadedCampaignMedia, 0, len(photoHeaders)+len(videoHeaders))
	for _, fileHeader := range photoHeaders {
		item, err := readCampaignMedia(fileHeader, "IMAGE", maxCampaignPhotoSize)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if item.contentType != "image/jpeg" && item.contentType != "image/png" && item.contentType != "image/webp" {
			http.Error(w, "format foto harus JPG, PNG, atau WebP", http.StatusBadRequest)
			return
		}
		media = append(media, item)
	}
	for _, fileHeader := range videoHeaders {
		item, err := readCampaignMedia(fileHeader, "VIDEO", maxCampaignVideoSize)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if item.contentType != "video/mp4" {
			http.Error(w, "format video harus MP4", http.StatusBadRequest)
			return
		}
		media = append(media, item)
	}

	tx, err := db.Begin(r.Context())
	if err != nil {
		http.Error(w, "gagal memulai penyimpanan media campaign", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	var ownerID sql.NullInt64
	var status string
	err = tx.QueryRow(r.Context(), "SELECT owner_id,status FROM campaigns WHERE id=$1 FOR UPDATE", campaignID).Scan(&ownerID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "gagal memeriksa campaign", http.StatusInternalServerError)
		return
	}
	if u.Role != "ADMIN" && (!ownerID.Valid || ownerID.Int64 != u.ID) {
		http.Error(w, "akses ditolak", http.StatusForbidden)
		return
	}
	if status != "MENUNGGU" {
		http.Error(w, "media hanya dapat ditambahkan sebelum campaign disetujui", http.StatusConflict)
		return
	}

	var existingPhotos, existingVideos int64
	err = tx.QueryRow(r.Context(),
		"SELECT COUNT(*) FILTER (WHERE media_type='IMAGE'), COUNT(*) FILTER (WHERE media_type='VIDEO') FROM campaign_media WHERE campaign_id=$1",
		campaignID,
	).Scan(&existingPhotos, &existingVideos)
	if err != nil {
		http.Error(w, "gagal memeriksa media campaign", http.StatusInternalServerError)
		return
	}
	if existingPhotos+int64(len(photoHeaders)) > maxCampaignPhotos || existingVideos+int64(len(videoHeaders)) > 1 {
		http.Error(w, "campaign dapat memiliki maksimal 5 foto dan 1 video", http.StatusConflict)
		return
	}

	for _, item := range media {
		if _, err := tx.Exec(r.Context(),
			"INSERT INTO campaign_media(campaign_id,media_type,content_type,file_data) VALUES($1,$2,$3,$4)",
			campaignID, item.mediaType, item.contentType, item.data,
		); err != nil {
			http.Error(w, "gagal menyimpan media campaign", http.StatusInternalServerError)
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "gagal menyelesaikan penyimpanan media campaign", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]int{"uploaded": len(media)})
}

func readCampaignMedia(fileHeader *multipart.FileHeader, mediaType string, maxSize int64) (uploadedCampaignMedia, error) {
	if fileHeader.Size > maxSize {
		return uploadedCampaignMedia{}, errors.New("ukuran file melebihi batas")
	}
	file, err := fileHeader.Open()
	if err != nil {
		return uploadedCampaignMedia{}, errors.New("file tidak dapat dibaca")
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxSize+1))
	if err != nil {
		return uploadedCampaignMedia{}, errors.New("file tidak dapat dibaca")
	}
	if len(data) == 0 || int64(len(data)) > maxSize {
		return uploadedCampaignMedia{}, errors.New("ukuran file kosong atau melebihi batas")
	}

	contentType := detectCampaignMediaType(data)
	if contentType == "" {
		return uploadedCampaignMedia{}, errors.New("format file tidak didukung atau isi file tidak valid")
	}
	return uploadedCampaignMedia{mediaType: mediaType, contentType: contentType, data: data}, nil
}

func detectCampaignMediaType(data []byte) string {
	if len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP" {
		return "image/webp"
	}
	if len(data) >= 12 && string(data[4:8]) == "ftyp" {
		return "video/mp4"
	}
	detected := http.DetectContentType(data[:min(len(data), 512)])
	if detected == "image/jpeg" || detected == "image/png" {
		return detected
	}
	return ""
}

func serveCampaignMedia(w http.ResponseWriter, r *http.Request) {
	campaignID, err := strconv.ParseInt(r.PathValue("campaignID"), 10, 64)
	if err != nil || campaignID <= 0 {
		http.NotFound(w, r)
		return
	}
	mediaID, err := strconv.ParseInt(r.PathValue("mediaID"), 10, 64)
	if err != nil || mediaID <= 0 {
		http.NotFound(w, r)
		return
	}

	var status string
	var ownerID sql.NullInt64
	var contentType string
	var data []byte
	err = db.QueryRow(r.Context(),
		"SELECT c.status,c.owner_id,m.content_type,m.file_data FROM campaigns c JOIN campaign_media m ON m.campaign_id=c.id WHERE c.id=$1 AND m.id=$2",
		campaignID, mediaID,
	).Scan(&status, &ownerID, &contentType, &data)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "gagal mengambil media campaign", http.StatusInternalServerError)
		return
	}

	u, authenticated := r.Context().Value(userKey).(User)
	if status != "AKTIF" && status != "SELESAI" && (!authenticated || (u.Role != "ADMIN" && (!ownerID.Valid || u.ID != ownerID.Int64))) {
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
	w.Write(data)
}

func currentUser(p graphql.ResolveParams) (User, bool) {
	u, ok := p.Context.Value(userKey).(User)
	return u, ok
}

func graphQLHandler(schema graphql.Schema) http.Handler {
	return authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Query         string                 `json:"query"`
			Variables     map[string]interface{} `json:"variables"`
			OperationName string                 `json:"operationName"`
		}

		switch r.Method {
		case http.MethodPost:
			decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
			if err := decoder.Decode(&request); err != nil {
				http.Error(w, "request GraphQL tidak valid", http.StatusBadRequest)
				return
			}
		case http.MethodGet:
			query := r.URL.Query()
			request.Query = query.Get("query")
			request.OperationName = query.Get("operationName")
			if rawVariables := query.Get("variables"); rawVariables != "" {
				if err := json.Unmarshal([]byte(rawVariables), &request.Variables); err != nil {
					http.Error(w, "variables GraphQL tidak valid", http.StatusBadRequest)
					return
				}
			}
			if isGraphQLMutation(request.Query) {
				http.Error(w, "mutation GraphQL harus menggunakan POST", http.StatusMethodNotAllowed)
				return
			}
		default:
			w.Header().Set("Allow", "GET, POST")
			http.Error(w, "method tidak diizinkan", http.StatusMethodNotAllowed)
			return
		}

		if strings.TrimSpace(request.Query) == "" {
			http.Error(w, "query GraphQL wajib diisi", http.StatusBadRequest)
			return
		}
		if strings.Count(request.Query, "{") > 12 {
			http.Error(w, "query terlalu kompleks", http.StatusBadRequest)
			return
		}

		result := graphql.Do(graphql.Params{
			Schema:         schema,
			RequestString:  request.Query,
			VariableValues: request.Variables,
			OperationName:  request.OperationName,
			Context:        r.Context(),
		})
		writeJSON(w, http.StatusOK, result)
	}))
}

func isGraphQLMutation(query string) bool {
	var lines []string
	for _, line := range strings.Split(query, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	// Reject any mutation operation in a GET document, including multi-operation documents.
	return strings.Contains(strings.Join(lines, " "), "mutation")
}

func requireRole(p graphql.ResolveParams, roles ...string) (User, error) {
	u, ok := currentUser(p)
	if !ok {
		return User{}, errors.New("autentikasi diperlukan")
	}
	for _, role := range roles {
		if u.Role == role {
			return u, nil
		}
	}
	return User{}, errors.New("akses ditolak: role tidak memiliki izin")
}

func register(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		http.Error(w, "data tidak valid", http.StatusBadRequest)
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	parsedEmail, emailErr := mail.ParseAddress(in.Email)
	if len(in.Name) < 2 || len(in.Name) > 100 || len(in.Email) > 150 || emailErr != nil || parsedEmail.Address != in.Email || len(in.Password) < 10 || len(in.Password) > 72 {
		http.Error(w, "nama/email/password tidak memenuhi aturan", http.StatusBadRequest)
		return
	}
	in.Role = "DONATUR" // Role publik selalu DONATUR; role istimewa ditetapkan admin database.

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "gagal membuat akun", http.StatusInternalServerError)
		return
	}
	var u User
	err = db.QueryRow(r.Context(), "INSERT INTO users(name,email,password_hash,role) VALUES($1,$2,$3,$4) RETURNING id,name,email,role", in.Name, in.Email, string(hash), in.Role).Scan(&u.ID, &u.Name, &u.Email, &u.Role)
	if err != nil {
		var databaseError *pgconn.PgError
		if errors.As(err, &databaseError) && databaseError.Code == "23505" {
			http.Error(w, "email sudah digunakan", http.StatusConflict)
			return
		}
		log.Printf("registrasi gagal: %v", err)
		http.Error(w, "gagal membuat akun", http.StatusInternalServerError)
		return
	}
	respondToken(w, u)
}

func login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
		http.Error(w, "data tidak valid", http.StatusBadRequest)
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if in.Email == "" || len(in.Email) > 150 || in.Password == "" || len(in.Password) > 72 {
		http.Error(w, "email atau password salah", http.StatusUnauthorized)
		return
	}

	var u User
	var passwordHash string
	err := db.QueryRow(r.Context(), "SELECT id,name,email,role,password_hash FROM users WHERE email=$1", in.Email).Scan(&u.ID, &u.Name, &u.Email, &u.Role, &passwordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "email atau password salah", http.StatusUnauthorized)
		return
	}
	if err != nil {
		log.Printf("login gagal karena database error: %v", err)
		http.Error(w, "gagal memproses login", http.StatusInternalServerError)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(in.Password)) != nil {
		http.Error(w, "email atau password salah", http.StatusUnauthorized)
		return
	}
	respondToken(w, u)
}

type googleUserProfile struct {
	Sub           string `json:"sub"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
}

func googleLogin(w http.ResponseWriter, r *http.Request) {
	clientID := strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID"))
	redirectURL := strings.TrimSpace(os.Getenv("GOOGLE_REDIRECT_URL"))
	if clientID == "" || strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_SECRET")) == "" || redirectURL == "" {
		redirectGoogleError(w, r)
		return
	}

	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		http.Error(w, "tidak dapat memulai autentikasi Google", http.StatusInternalServerError)
		return
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)
	http.SetCookie(w, &http.Cookie{
		Name:     "google_oauth_state",
		Value:    state,
		Path:     "/auth/google/callback",
		MaxAge:   600,
		HttpOnly: true,
		Secure:   strings.EqualFold(os.Getenv("GOOGLE_COOKIE_SECURE"), "true"),
		SameSite: http.SameSiteLaxMode,
	})

	params := url.Values{}
	params.Set("client_id", clientID)
	params.Set("redirect_uri", redirectURL)
	params.Set("response_type", "code")
	params.Set("scope", "openid email profile")
	params.Set("state", state)
	params.Set("prompt", "select_account")
	params.Set("access_type", "online")
	http.Redirect(w, r, "https://accounts.google.com/o/oauth2/v2/auth?"+params.Encode(), http.StatusFound)
}

func googleCallback(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("error") != "" {
		clearGoogleStateCookie(w)
		redirectGoogleError(w, r)
		return
	}

	stateCookie, err := r.Cookie("google_oauth_state")
	state := r.URL.Query().Get("state")
	if err != nil || state == "" || subtle.ConstantTimeCompare([]byte(stateCookie.Value), []byte(state)) != 1 {
		clearGoogleStateCookie(w)
		redirectGoogleError(w, r)
		return
	}
	clearGoogleStateCookie(w)

	code := strings.TrimSpace(r.URL.Query().Get("code"))
	clientID := strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID"))
	clientSecret := strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_SECRET"))
	redirectURL := strings.TrimSpace(os.Getenv("GOOGLE_REDIRECT_URL"))
	if code == "" || clientID == "" || clientSecret == "" || redirectURL == "" {
		redirectGoogleError(w, r)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	accessToken, err := exchangeGoogleCode(ctx, code, clientID, clientSecret, redirectURL)
	if err != nil {
		log.Printf("Google OAuth code exchange gagal: %v", err)
		redirectGoogleError(w, r)
		return
	}

	profile, err := fetchGoogleUserProfile(ctx, accessToken)
	if err != nil || !profile.EmailVerified || strings.TrimSpace(profile.Sub) == "" {
		log.Printf("verifikasi profil Google gagal")
		redirectGoogleError(w, r)
		return
	}

	user, err := findOrCreateGoogleUser(ctx, profile)
	if err != nil {
		log.Printf("pembuatan akun Google gagal: %v", err)
		redirectGoogleError(w, r)
		return
	}

	signedToken, err := createUserToken(user)
	if err != nil {
		log.Printf("pembuatan token login Google gagal: %v", err)
		redirectGoogleError(w, r)
		return
	}
	redirectGoogleToken(w, r, signedToken)
}

func exchangeGoogleCode(ctx context.Context, code, clientID, clientSecret, redirectURL string) (string, error) {
	values := url.Values{}
	values.Set("code", code)
	values.Set("client_id", clientID)
	values.Set("client_secret", clientSecret)
	values.Set("redirect_uri", redirectURL)
	values.Set("grant_type", "authorization_code")

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(values.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", errors.New("Google token endpoint menolak authorization code")
	}
	var tokenResponse struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&tokenResponse); err != nil {
		return "", err
	}
	if tokenResponse.AccessToken == "" || !strings.EqualFold(tokenResponse.TokenType, "Bearer") {
		return "", errors.New("respons token Google tidak valid")
	}
	return tokenResponse.AccessToken, nil
}

func fetchGoogleUserProfile(ctx context.Context, accessToken string) (googleUserProfile, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://openidconnect.googleapis.com/v1/userinfo", nil)
	if err != nil {
		return googleUserProfile{}, err
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		return googleUserProfile{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return googleUserProfile{}, errors.New("Google UserInfo endpoint menolak access token")
	}
	var profile googleUserProfile
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&profile); err != nil {
		return googleUserProfile{}, err
	}
	return profile, nil
}

func findOrCreateGoogleUser(ctx context.Context, profile googleUserProfile) (User, error) {
	email := strings.ToLower(strings.TrimSpace(profile.Email))
	parsedEmail, emailErr := mail.ParseAddress(email)
	if strings.TrimSpace(profile.Sub) == "" || len(email) > 150 || emailErr != nil || parsedEmail.Address != email || !profile.EmailVerified {
		return User{}, errors.New("identitas Google tidak valid atau email belum terverifikasi")
	}

	var user User
	var existingGoogleSub string
	err := db.QueryRow(ctx, `SELECT id,name,email,role,COALESCE(google_sub,'') FROM users WHERE google_sub=$1`, profile.Sub).
		Scan(&user.ID, &user.Name, &user.Email, &user.Role, &existingGoogleSub)
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return User{}, err
	}

	err = db.QueryRow(ctx, `SELECT id,name,email,role,COALESCE(google_sub,'') FROM users WHERE email=$1`, email).
		Scan(&user.ID, &user.Name, &user.Email, &user.Role, &existingGoogleSub)
	if err == nil {
		if existingGoogleSub != "" && existingGoogleSub != profile.Sub {
			return User{}, errors.New("email sudah terhubung ke akun Google lain")
		}
		if existingGoogleSub == "" {
			err = db.QueryRow(ctx, `UPDATE users SET google_sub=$1 WHERE id=$2 AND (google_sub IS NULL OR google_sub='') RETURNING id,name,email,role`, profile.Sub, user.ID).
				Scan(&user.ID, &user.Name, &user.Email, &user.Role)
			if err != nil {
				return User{}, err
			}
		}
		return user, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return User{}, err
	}

	name := strings.TrimSpace(profile.Name)
	if name == "" {
		name = strings.SplitN(email, "@", 2)[0]
	}
	if runes := []rune(name); len(runes) > 100 {
		name = string(runes[:100])
	}
	passwordBytes := make([]byte, 48)
	if _, err := rand.Read(passwordBytes); err != nil {
		return User{}, err
	}
	passwordHash, err := bcrypt.GenerateFromPassword(passwordBytes, bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}
	err = db.QueryRow(ctx, `INSERT INTO users(name,email,password_hash,role,google_sub) VALUES($1,$2,$3,'DONATUR',$4) RETURNING id,name,email,role`, name, email, string(passwordHash), profile.Sub).
		Scan(&user.ID, &user.Name, &user.Email, &user.Role)
	if err == nil {
		return user, nil
	}

	// Handle a concurrent first login without creating duplicate accounts.
	if userErr := db.QueryRow(ctx, `SELECT id,name,email,role FROM users WHERE google_sub=$1`, profile.Sub).
		Scan(&user.ID, &user.Name, &user.Email, &user.Role); userErr == nil {
		return user, nil
	}
	return User{}, err
}

func clearGoogleStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "google_oauth_state",
		Value:    "",
		Path:     "/auth/google/callback",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   strings.EqualFold(os.Getenv("GOOGLE_COOKIE_SECURE"), "true"),
		SameSite: http.SameSiteLaxMode,
	})
}

func redirectGoogleToken(w http.ResponseWriter, r *http.Request, token string) {
	target, err := url.Parse(strings.TrimSpace(os.Getenv("FRONTEND_URL")))
	if err != nil || target.Scheme == "" || target.Host == "" {
		target, _ = url.Parse("http://localhost:5173")
	}
	target.Fragment = "oauth_token=" + token
	http.Redirect(w, r, target.String(), http.StatusSeeOther)
}

func redirectGoogleError(w http.ResponseWriter, r *http.Request) {
	target, err := url.Parse(strings.TrimSpace(os.Getenv("FRONTEND_URL")))
	if err != nil || target.Scheme == "" || target.Host == "" {
		target, _ = url.Parse("http://localhost:5173")
	}
	target.Fragment = "oauth_error=google_login_failed"
	http.Redirect(w, r, target.String(), http.StatusSeeOther)
}

func respondToken(w http.ResponseWriter, u User) {
	signed, err := createUserToken(u)
	if err != nil {
		http.Error(w, "gagal membuat token", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"token": signed, "token_type": "Bearer", "expires_in": 43200, "user": u})
}

func createUserToken(u User) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iss":   userTokenIssuer,
		"aud":   []string{userTokenAudience},
		"sub":   strconv.FormatInt(u.ID, 10),
		"name":  u.Name,
		"email": u.Email,
		"role":  u.Role,
		"iat":   now.Unix(),
		"exp":   now.Add(12 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(jwtSecret)
	if err != nil {
		return "", err
	}
	return signed, nil
}

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		parts := strings.SplitN(header, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") && strings.TrimSpace(parts[1]) != "" {
			claims := jwt.MapClaims{}
			token, err := jwt.ParseWithClaims(parts[1], claims, func(token *jwt.Token) (interface{}, error) {
				if token.Method != jwt.SigningMethodHS256 {
					return nil, errors.New("algoritma token tidak valid")
				}
				return jwtSecret, nil
			}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(userTokenIssuer), jwt.WithAudience(userTokenAudience))
			if err == nil && token.Valid {
				id, parseErr := strconv.ParseInt(str(claims["sub"]), 10, 64)
				if parseErr == nil && id > 0 {
					u := User{ID: id, Name: str(claims["name"]), Email: str(claims["email"]), Role: str(claims["role"])}
					r = r.WithContext(context.WithValue(r.Context(), userKey, u))
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func oauthToken(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "form tidak valid", http.StatusBadRequest)
		return
	}
	clientID, clientSecret, ok := r.BasicAuth()
	if !ok || subtle.ConstantTimeCompare([]byte(clientID), []byte(oauthClientID)) != 1 || subtle.ConstantTimeCompare([]byte(clientSecret), []byte(oauthClientSecret)) != 1 {
		w.Header().Set("WWW-Authenticate", `Basic realm="crowdfunding-internal"`)
		http.Error(w, "client authentication failed", http.StatusUnauthorized)
		return
	}
	if r.Form.Get("grant_type") != "client_credentials" {
		http.Error(w, "grant_type harus client_credentials", http.StatusBadRequest)
		return
	}
	if requestedScope := r.Form.Get("scope"); requestedScope != "" && requestedScope != serviceTokenScope {
		http.Error(w, "scope tidak diizinkan", http.StatusBadRequest)
		return
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"iss":   serviceTokenIssuer,
		"aud":   []string{serviceTokenAudience},
		"sub":   oauthClientID,
		"scope": serviceTokenScope,
		"iat":   now.Unix(),
		"exp":   now.Add(serviceTokenTTL).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(jwtSecret)
	if err != nil {
		http.Error(w, "gagal menerbitkan access token", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token": signed,
		"token_type":   "Bearer",
		"expires_in":   int(serviceTokenTTL.Seconds()),
		"scope":        serviceTokenScope,
	})
}

func internalAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" || r.URL.Path == "/oauth/token" {
			next.ServeHTTP(w, r)
			return
		}
		if !validServiceToken(r.Header.Get("X-Internal-Token")) {
			http.Error(w, "service access token tidak valid", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func validServiceToken(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("algoritma token tidak valid")
		}
		return jwtSecret, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer(serviceTokenIssuer), jwt.WithAudience(serviceTokenAudience))
	if err != nil || !token.Valid {
		return false
	}
	return str(claims["scope"]) == serviceTokenScope && str(claims["sub"]) == oauthClientID
}

func requireMTLS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !mtlsEnabled || r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.PeerCertificates) == 0 {
			http.Error(w, "mutual TLS client certificate required", http.StatusUnauthorized)
			return
		}
		if r.TLS.PeerCertificates[0].Subject.CommonName != "gateway" {
			http.Error(w, "client certificate tidak diizinkan", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func paymentWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "request tidak valid", http.StatusBadRequest)
		return
	}
	signature, err := hex.DecodeString(strings.TrimSpace(r.Header.Get("X-Signature")))
	if err != nil {
		http.Error(w, "signature tidak valid", http.StatusUnauthorized)
		return
	}
	mac := hmac.New(sha256.New, paymentWebhookSecret)
	_, _ = mac.Write(body)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		http.Error(w, "signature tidak valid", http.StatusUnauthorized)
		return
	}

	var payload struct {
		CampaignID int64  `json:"campaign_id"`
		DonorName  string `json:"donor_name"`
		Amount     int64  `json:"amount"`
		Reference  string `json:"reference"`
		Status     string `json:"status"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.CampaignID <= 0 || payload.Amount <= 0 || strings.TrimSpace(payload.Reference) == "" || len(payload.Reference) > 120 || len(strings.TrimSpace(payload.DonorName)) < 1 || len(payload.DonorName) > 60 || (payload.Status != "PENDING" && payload.Status != "SETTLEMENT" && payload.Status != "FAILED") {
		http.Error(w, "payload tidak valid", http.StatusBadRequest)
		return
	}
	payload.DonorName = strings.TrimSpace(payload.DonorName)

	tx, err := db.Begin(r.Context())
	if err != nil {
		http.Error(w, "gagal memproses transaksi", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	_, err = tx.Exec(r.Context(), "INSERT INTO donations(campaign_id,donor_name,amount,payment_reference,status) VALUES($1,$2,$3,$4,$5)", payload.CampaignID, payload.DonorName, payload.Amount, payload.Reference, payload.Status)
	if err != nil {
		var databaseError *pgconn.PgError
		if errors.As(err, &databaseError) {
			switch databaseError.Code {
			case "23505":
				http.Error(w, "payment reference sudah pernah diproses", http.StatusConflict)
				return
			case "23503":
				http.Error(w, "campaign tidak ditemukan", http.StatusNotFound)
				return
			case "23502", "23514":
				http.Error(w, "payload transaksi tidak memenuhi aturan database", http.StatusBadRequest)
				return
			}
		}
		log.Printf("gagal menyimpan donasi: %v", err)
		http.Error(w, "gagal menyimpan transaksi", http.StatusInternalServerError)
		return
	}
	if payload.Status == "SETTLEMENT" {
		result, updateErr := tx.Exec(r.Context(), `
			UPDATE campaigns
			SET collected=collected+$1,
				status=CASE WHEN collected+$1 >= target THEN 'SELESAI' ELSE status END
			WHERE id=$2
		`, payload.Amount, payload.CampaignID)
		if updateErr != nil {
			http.Error(w, "gagal memperbarui campaign", http.StatusInternalServerError)
			return
		}
		if result.RowsAffected() == 0 {
			http.Error(w, "campaign tidak ditemukan", http.StatusNotFound)
			return
		}
	}
	if err := addOutboxEvent(r.Context(), tx, "donation.created", map[string]interface{}{
		"event":       "NEW_DONATION",
		"campaign_id": payload.CampaignID,
		"donor_name":  payload.DonorName,
		"amount":      payload.Amount,
	}); err != nil {
		http.Error(w, "gagal membuat event transaksi", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "gagal menyimpan transaksi", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted", "message": "event akan diproses secara asinkron"})
}

func serverTLSConfig() (*tls.Config, error) {
	caPath := strings.TrimSpace(os.Getenv("TLS_CA_FILE"))
	certPath := strings.TrimSpace(os.Getenv("TLS_CERT_FILE"))
	keyPath := strings.TrimSpace(os.Getenv("TLS_KEY_FILE"))
	if caPath == "" || certPath == "" || keyPath == "" {
		return nil, errors.New("TLS_CA_FILE, TLS_CERT_FILE, dan TLS_KEY_FILE wajib diisi saat MTLS_ENABLED=true")
	}
	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return nil, err
	}
	clientCAs := x509.NewCertPool()
	if !clientCAs.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("TLS_CA_FILE tidak berisi CA certificate yang valid")
	}
	if _, err := tls.LoadX509KeyPair(certPath, keyPath); err != nil {
		return nil, err
	}
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		ClientCAs:  clientCAs,
		// Middleware memaksa sertifikat terverifikasi pada semua endpoint selain /health.
		ClientAuth: tls.VerifyClientCertIfGiven,
	}, nil
}

func simulateDonation(w http.ResponseWriter, r *http.Request) {
	u, ok := r.Context().Value(userKey).(User)
	if !ok {
		http.Error(w, "autentikasi diperlukan", http.StatusUnauthorized)
		return
	}
	if u.Role != "DONATUR" {
		http.Error(w, "hanya donatur yang dapat memberi dukungan simulasi", http.StatusForbidden)
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
	if in.CampaignID <= 0 || in.Amount < 1000 || len(in.DonorName) < 2 || len(in.DonorName) > 100 ||
		!validDonorEmail(in.DonorEmail) || !validDonorPhone(in.DonorPhone) {
		http.Error(w, "data donasi tidak valid. Periksa nama, email, nomor telepon, dan nominal minimal Rp 1.000", http.StatusBadRequest)
		return
	}

	var referenceBytes [16]byte
	if _, err := rand.Read(referenceBytes[:]); err != nil {
		http.Error(w, "gagal membuat referensi donasi", http.StatusInternalServerError)
		return
	}
	reference := "SIM-" + hex.EncodeToString(referenceBytes[:])

	tx, err := db.Begin(r.Context())
	if err != nil {
		http.Error(w, "gagal memulai donasi simulasi", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())

	var campaignID int64
	err = tx.QueryRow(r.Context(),
		"SELECT id FROM campaigns WHERE id=$1 AND status='AKTIF' FOR UPDATE",
		in.CampaignID,
	).Scan(&campaignID)
	if err == sql.ErrNoRows {
		http.Error(w, "campaign tidak ditemukan atau tidak aktif", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "gagal memeriksa campaign", http.StatusInternalServerError)
		return
	}

	if _, err = tx.Exec(r.Context(),
		"INSERT INTO donations(campaign_id,donor_user_id,donor_name,donor_email,donor_phone,amount,payment_reference,status) VALUES($1,$2,$3,$4,$5,$6,$7,'SETTLEMENT')",
		campaignID, u.ID, in.DonorName, in.DonorEmail, in.DonorPhone, in.Amount, reference,
	); err != nil {
		http.Error(w, "gagal menyimpan donasi simulasi", http.StatusInternalServerError)
		return
	}
	if _, err = tx.Exec(r.Context(), `
		UPDATE campaigns
		SET collected=collected+$1,
			status=CASE WHEN collected+$1 >= target THEN 'SELESAI' ELSE status END
		WHERE id=$2
	`,
		in.Amount, campaignID,
	); err != nil {
		http.Error(w, "gagal memperbarui progres campaign", http.StatusInternalServerError)
		return
	}
	if err := addOutboxEvent(r.Context(), tx, "donation.created", map[string]interface{}{
		"event":       "SIMULATED_DONATION",
		"campaign_id": campaignID,
		"donor_name":  "Donatur",
		"amount":      in.Amount,
	}); err != nil {
		http.Error(w, "gagal membuat event donasi", http.StatusInternalServerError)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		http.Error(w, "gagal menyelesaikan donasi simulasi", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "simulated", "reference": reference})
}

func validDonorEmail(email string) bool {
	if len(email) > 150 {
		return false
	}
	address, err := mail.ParseAddress(email)
	return err == nil && address.Address == email && strings.Contains(email, "@")
}

func validDonorPhone(phone string) bool {
	if phone == "" {
		return false
	}
	digits := 0
	for index, char := range phone {
		switch {
		case char >= '0' && char <= '9':
			digits++
		case char == '+' && index == 0:
		case char == ' ' || char == '-' || char == '(' || char == ')':
		default:
			return false
		}
	}
	return digits >= 8 && digits <= 15
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func str(value interface{}) string {
	text, _ := value.(string)
	return text
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("gagal menulis JSON response: %v", err)
	}
}
