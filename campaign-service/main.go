package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/graphql-go/graphql"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type ctxKey string

const userKey ctxKey = "user"

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

var db *pgxpool.Pool
var jwtSecret = []byte(os.Getenv("JWT_SECRET"))
var notifURL = os.Getenv("NOTIF_URL")
var notifyClient = &http.Client{Timeout: 2 * time.Second}

func notifyEvent(event map[string]interface{}) {
	if notifURL == "" {
		return
	}
	body, err := json.Marshal(event)
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(notifURL, "/")+"/internal/broadcast", strings.NewReader(string(body)))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := notifyClient.Do(req)
	if err == nil {
		resp.Body.Close()
	}
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
	for _, r := range roles {
		if u.Role == r {
			return u, nil
		}
	}
	return User{}, errors.New("akses ditolak: role tidak memiliki izin")
}
func main() {
	if len(jwtSecret) < 32 {
		log.Fatal("JWT_SECRET wajib minimal 32 karakter")
	}
	var err error
	db, err = pgxpool.New(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err = db.Ping(context.Background()); err != nil {
		log.Fatal(err)
	}
	schema := buildSchema()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "campaign"})
	})
	mux.HandleFunc("POST /auth/register", register)
	mux.HandleFunc("POST /auth/login", login)
	mux.Handle("/graphql", authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query     string                 `json:"query"`
			Variables map[string]interface{} `json:"variables"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			http.Error(w, "request tidak valid", 400)
			return
		}
		if strings.Count(req.Query, "{") > 12 {
			http.Error(w, "query terlalu kompleks", 400)
			return
		}
		result := graphql.Do(graphql.Params{Schema: schema, RequestString: req.Query, VariableValues: req.Variables, Context: r.Context()})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})))
	mux.HandleFunc("POST /webhook/payment", paymentWebhook)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8001"
	}
	log.Printf("Campaign Service listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, securityHeaders(mux)))
}
func buildSchema() graphql.Schema {
	campaignType := graphql.NewObject(graphql.ObjectConfig{Name: "Campaign", Fields: graphql.Fields{
		"id": &graphql.Field{Type: graphql.Int}, "title": &graphql.Field{Type: graphql.String}, "description": &graphql.Field{Type: graphql.String}, "target": &graphql.Field{Type: graphql.Int}, "collected": &graphql.Field{Type: graphql.Int}, "status": &graphql.Field{Type: graphql.String},
	}})
	query := graphql.NewObject(graphql.ObjectConfig{Name: "Query", Fields: graphql.Fields{
		"campaigns": &graphql.Field{Type: graphql.NewList(campaignType), Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			rows, e := db.Query(p.Context, "SELECT id,title,description,target,collected,status FROM campaigns WHERE status='AKTIF' ORDER BY id DESC")
			if e != nil {
				return nil, e
			}
			defer rows.Close()
			out := []Campaign{}
			for rows.Next() {
				var c Campaign
				if e = rows.Scan(&c.ID, &c.Title, &c.Description, &c.Target, &c.Collected, &c.Status); e != nil {
					return nil, e
				}
				out = append(out, c)
			}
			return out, rows.Err()
		}},
		"campaign": &graphql.Field{Type: campaignType, Args: graphql.FieldConfigArgument{"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.Int)}}, Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			var c Campaign
			e := db.QueryRow(p.Context, "SELECT id,title,description,target,collected,status FROM campaigns WHERE id=$1", p.Args["id"]).Scan(&c.ID, &c.Title, &c.Description, &c.Target, &c.Collected, &c.Status)
			if e == sql.ErrNoRows {
				return nil, nil
			}
			return c, e
		}},
	}})
	mutation := graphql.NewObject(graphql.ObjectConfig{Name: "Mutation", Fields: graphql.Fields{
		"createCampaign": &graphql.Field{Type: campaignType, Args: graphql.FieldConfigArgument{"title": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}, "description": &graphql.ArgumentConfig{Type: graphql.String}, "target": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.Int)}}, Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			u, e := requireRole(p, "PENGELOLA", "ADMIN")
			if e != nil {
				return nil, e
			}
			title := strings.TrimSpace(p.Args["title"].(string))
			target := p.Args["target"].(int)
			if len(title) < 3 || len(title) > 100 || target <= 0 {
				return nil, errors.New("judul atau target tidak valid")
			}
			desc, _ := p.Args["description"].(string)
			var c Campaign
			e = db.QueryRow(p.Context, "INSERT INTO campaigns(title,description,target,status,owner_id) VALUES($1,$2,$3,'MENUNGGU',$4) RETURNING id,title,description,target,collected,status", title, desc, target, u.ID).Scan(&c.ID, &c.Title, &c.Description, &c.Target, &c.Collected, &c.Status)
			if e == nil {
				notifyEvent(map[string]interface{}{"event": "CAMPAIGN_CREATED", "campaign": c})
			}
			return c, e
		}},
		"approveCampaign": &graphql.Field{Type: campaignType, Args: graphql.FieldConfigArgument{"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.Int)}}, Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			if _, e := requireRole(p, "ADMIN"); e != nil {
				return nil, e
			}
			var c Campaign
			e := db.QueryRow(p.Context, "UPDATE campaigns SET status='AKTIF' WHERE id=$1 AND status='MENUNGGU' RETURNING id,title,description,target,collected,status", p.Args["id"]).Scan(&c.ID, &c.Title, &c.Description, &c.Target, &c.Collected, &c.Status)
			if e == nil {
				notifyEvent(map[string]interface{}{"event": "CAMPAIGN_APPROVED", "campaign": c})
			}
			return c, e
		}},
	}})
	s, e := graphql.NewSchema(graphql.SchemaConfig{Query: query, Mutation: mutation})
	if e != nil {
		log.Fatal(e)
	}
	return s
}
func register(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in) != nil {
		http.Error(w, "data tidak valid", 400)
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if len(in.Name) < 2 || len(in.Email) > 150 || !strings.Contains(in.Email, "@") || len(in.Password) < 10 || len(in.Password) > 72 {
		http.Error(w, "nama/email/password tidak memenuhi aturan", 400)
		return
	}
	in.Role = "DONATUR" // role istimewa tidak boleh dipilih sendiri saat registrasi
	hash, e := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if e != nil {
		http.Error(w, "gagal membuat akun", 500)
		return
	}
	var u User
	e = db.QueryRow(r.Context(), "INSERT INTO users(name,email,password_hash,role) VALUES($1,$2,$3,$4) RETURNING id,name,email,role", in.Name, in.Email, string(hash), in.Role).Scan(&u.ID, &u.Name, &u.Email, &u.Role)
	if e != nil {
		http.Error(w, "email sudah digunakan atau data tidak valid", 409)
		return
	}
	respondToken(w, u)
}
func login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in) != nil {
		http.Error(w, "data tidak valid", 400)
		return
	}
	var u User
	var hash string
	e := db.QueryRow(r.Context(), "SELECT id,name,email,role,password_hash FROM users WHERE email=$1", strings.ToLower(strings.TrimSpace(in.Email))).Scan(&u.ID, &u.Name, &u.Email, &u.Role, &hash)
	if e != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) != nil {
		http.Error(w, "email atau password salah", 401)
		return
	}
	respondToken(w, u)
}
func respondToken(w http.ResponseWriter, u User) {
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": u.ID, "name": u.Name, "email": u.Email, "role": u.Role, "exp": time.Now().Add(12 * time.Hour).Unix()})
	s, e := t.SignedString(jwtSecret)
	if e != nil {
		http.Error(w, "gagal membuat token", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"token": s, "user": u})
}
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if h != "" {
			t, e := jwt.Parse(h, func(t *jwt.Token) (interface{}, error) {
				if t.Method != jwt.SigningMethodHS256 {
					return nil, errors.New("algoritma tidak valid")
				}
				return jwtSecret, nil
			})
			if e == nil && t.Valid {
				if c, ok := t.Claims.(jwt.MapClaims); ok {
					id, _ := c["sub"].(float64)
					u := User{ID: int64(id), Name: str(c["name"]), Email: str(c["email"]), Role: str(c["role"])}
					r = r.WithContext(context.WithValue(r.Context(), userKey, u))
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
func str(v interface{}) string { s, _ := v.(string); return s }
func paymentWebhook(w http.ResponseWriter, r *http.Request) {
	body, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if e != nil {
		http.Error(w, "request tidak valid", 400)
		return
	}
	secret := []byte(os.Getenv("PAYMENT_WEBHOOK_SECRET"))
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(strings.ToLower(r.Header.Get("X-Signature"))), []byte(expected)) {
		http.Error(w, "signature tidak valid", 401)
		return
	}
	var p struct {
		CampaignID int64  `json:"campaign_id"`
		DonorName  string `json:"donor_name"`
		Amount     int64  `json:"amount"`
		Reference  string `json:"reference"`
		Status     string `json:"status"`
	}
	if json.Unmarshal(body, &p) != nil || p.CampaignID <= 0 || p.Amount <= 0 || p.Reference == "" || len(p.DonorName) < 1 || len(p.DonorName) > 60 || (p.Status != "PENDING" && p.Status != "SETTLEMENT" && p.Status != "FAILED") {
		http.Error(w, "payload tidak valid", 400)
		return
	}
	tx, e := db.Begin(r.Context())
	if e != nil {
		http.Error(w, "gagal memproses", 500)
		return
	}
	defer tx.Rollback(r.Context())
	_, e = tx.Exec(r.Context(), "INSERT INTO donations(campaign_id,donor_name,amount,payment_reference,status) VALUES($1,$2,$3,$4,$5)", p.CampaignID, p.DonorName, p.Amount, p.Reference, p.Status)
	if e != nil {
		http.Error(w, "transaksi duplikat atau tidak valid", 409)
		return
	}
	if p.Status == "SETTLEMENT" {
		_, e = tx.Exec(r.Context(), "UPDATE campaigns SET collected=collected+$1 WHERE id=$2", p.Amount, p.CampaignID)
		if e != nil {
			http.Error(w, "campaign tidak ditemukan", 404)
			return
		}
	}
	if tx.Commit(r.Context()) != nil {
		http.Error(w, "gagal menyimpan transaksi", 500)
		return
	}
	notifyEvent(map[string]interface{}{"event": "NEW_DONATION", "campaign_id": p.CampaignID, "donor_name": p.DonorName, "amount": p.Amount})
	w.WriteHeader(202)
	json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
