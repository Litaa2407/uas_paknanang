
package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Registry struct {
	mu      sync.RWMutex
	targets []*url.URL
	healthy []bool
	next    uint64
}

func NewRegistry(raw string) *Registry {
	r := &Registry{}

	for _, s := range strings.Split(raw, ",") {
		if u, e := url.Parse(strings.TrimSpace(s)); e == nil && u.Host != "" {
			r.targets = append(r.targets, u)
			r.healthy = append(r.healthy, true)
		}
	}

	return r
}

func (r *Registry) Pick() *url.URL {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.targets) == 0 {
		return nil
	}

	available := []int{}

	for i, ok := range r.healthy {
		if ok {
			available = append(available, i)
		}
	}

	if len(available) == 0 {
		return nil
	}

	i := available[(atomic.AddUint64(&r.next, 1)-1)%uint64(len(available))]

	return r.targets[i]
}

func (r *Registry) Check() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, u := range r.targets {
		c := http.Client{Timeout: 2 * time.Second}

		resp, e := c.Get(strings.TrimRight(u.String(), "/") + "/health")

		r.healthy[i] = e == nil && resp.StatusCode == http.StatusOK

		if resp != nil {
			resp.Body.Close()
		}
	}
}

func (r *Registry) Status() map[string]bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	m := map[string]bool{}

	for i, u := range r.targets {
		m[u.String()] = r.healthy[i]
	}

	return m
}

// Reverse Proxy dengan Round Robin Load Balancing
func proxy(reg *Registry) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := reg.Pick()

		if u == nil {
			http.Error(w, "service tidak tersedia", http.StatusServiceUnavailable)
			return
		}

		// Menampilkan instance yang menerima request
		w.Header().Set("X-Served-By", u.String())

		// Mencatat proses load balancing ke terminal
		log.Printf(
			"Load Balancing: %s %s -> %s",
			r.Method,
			r.URL.Path,
			u.String(),
		)

		p := httputil.NewSingleHostReverseProxy(u)

		p.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
			log.Printf("Proxy error ke %s: %v", u.String(), e)
			http.Error(w, "upstream service tidak tersedia", http.StatusServiceUnavailable)
		}

		r.Header.Set("X-Forwarded-Proto", "http")

		p.ServeHTTP(w, r)
	})
}

func main() {
	port := os.Getenv("PORT")

	if port == "" {
		port = "8000"
	}

	campaign := NewRegistry(os.Getenv("CAMPAIGN_SERVICES"))
	notif := NewRegistry(os.Getenv("NOTIFICATION_SERVICE"))

	// Pemeriksaan kesehatan service setiap 5 detik
	go func() {
		for {
			campaign.Check()
			notif.Check()
			time.Sleep(5 * time.Second)
		}
	}()

	mux := http.NewServeMux()

	// Health check Gateway
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","service":"gateway"}`))
	})

	// Status service dalam Registry
	mux.HandleFunc("GET /registry", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		w.Write([]byte(
			`{"campaign":` +
				toJSON(campaign.Status()) +
				`,"notification":` +
				toJSON(notif.Status()) +
				`}`,
		))
	})

	// Routing ke Campaign Service
	mux.Handle("/auth/", proxy(campaign))
	mux.Handle("/graphql", proxy(campaign))
	mux.Handle("/webhook/", proxy(campaign))

	// Routing ke Notification Service
	mux.Handle("/ws/", proxy(notif))

	// Halaman utama Gateway
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Crowdfunding API Gateway - Kelompok E", http.StatusOK)
	})

	log.Printf("API Gateway :%s", port)

	log.Fatal(http.ListenAndServe(":"+port, cors(mux)))
}

func toJSON(v map[string]bool) string {
	var b strings.Builder

	b.WriteByte('{')

	i := 0

	for k, v := range v {
		if i > 0 {
			b.WriteByte(',')
		}

		b.WriteString(`"` + k + `":`)

		if v {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}

		i++
	}

	b.WriteByte('}')

	return b.String()
}

// CORS Middleware
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		if origin == "http://localhost:5173" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}

		w.Header().Set(
			"Access-Control-Allow-Headers",
			"Authorization, Content-Type, X-Signature",
		)

		w.Header().Set(
			"Access-Control-Allow-Methods",
			"GET, POST, OPTIONS",
		)

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
