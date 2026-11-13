package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/mail"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

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
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Target      int64  `json:"target"`
	Collected   int64  `json:"collected"`
	Status      string `json:"status"`
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
	if err := ensureOutboxSchema(ctx, db); err != nil {
		log.Fatalf("gagal menyiapkan tabel outbox: %v", err)
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
	mux.Handle("POST /graphql", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query         string                 `json:"query"`
			Variables     map[string]interface{} `json:"variables"`
			OperationName string                 `json:"operationName"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		if err := decoder.Decode(&req); err != nil || strings.TrimSpace(req.Query) == "" {
			http.Error(w, "request GraphQL tidak valid", http.StatusBadRequest)
			return
		}
		if strings.Count(req.Query, "{") > 12 {
			http.Error(w, "query terlalu kompleks", http.StatusBadRequest)
			return
		}
		result := graphql.Do(graphql.Params{
			Schema:         schema,
			RequestString:  req.Query,
			VariableValues: req.Variables,
			OperationName:  req.OperationName,
			Context:        r.Context(),
		})
		writeJSON(w, http.StatusOK, result)
	})))
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
	campaignType := graphql.NewObject(graphql.ObjectConfig{Name: "Campaign", Fields: graphql.Fields{
		"id":          &graphql.Field{Type: graphql.Int},
		"title":       &graphql.Field{Type: graphql.String},
		"description": &graphql.Field{Type: graphql.String},
		"target":      &graphql.Field{Type: graphql.Int},
		"collected":   &graphql.Field{Type: graphql.Int},
		"status":      &graphql.Field{Type: graphql.String},
	}})

	query := graphql.NewObject(graphql.ObjectConfig{Name: "Query", Fields: graphql.Fields{
		"campaigns": &graphql.Field{Type: graphql.NewList(campaignType), Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			rows, err := db.Query(p.Context, "SELECT id,title,description,target,collected,status FROM campaigns WHERE status='AKTIF' ORDER BY id DESC")
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
			return out, rows.Err()
		}},
		"campaign": &graphql.Field{Type: campaignType, Args: graphql.FieldConfigArgument{
			"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.Int)},
		}, Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			var c Campaign
			err := db.QueryRow(p.Context, "SELECT id,title,description,target,collected,status FROM campaigns WHERE id=$1 AND status='AKTIF'", p.Args["id"]).Scan(&c.ID, &c.Title, &c.Description, &c.Target, &c.Collected, &c.Status)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, nil
			}
			return c, err
		}},
		"pendingCampaigns": &graphql.Field{Type: graphql.NewList(campaignType), Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			if _, err := requireRole(p, "ADMIN"); err != nil {
				return nil, err
			}
			rows, err := db.Query(p.Context, "SELECT id,title,description,target,collected,status FROM campaigns WHERE status='MENUNGGU' ORDER BY id")
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
			return out, rows.Err()
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
	}})

	schema, err := graphql.NewSchema(graphql.SchemaConfig{Query: query, Mutation: mutation})
	if err != nil {
		log.Fatalf("gagal membangun GraphQL schema: %v", err)
	}
	return schema
}

func currentUser(p graphql.ResolveParams) (User, bool) {
	u, ok := p.Context.Value(userKey).(User)
	return u, ok
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

func respondToken(w http.ResponseWriter, u User) {
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
		http.Error(w, "gagal membuat token", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"token": signed, "token_type": "Bearer", "expires_in": 43200, "user": u})
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
		result, updateErr := tx.Exec(r.Context(), "UPDATE campaigns SET collected=collected+$1 WHERE id=$2", payload.Amount, payload.CampaignID)
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
