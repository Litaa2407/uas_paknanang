package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type Registry struct {
	mu      sync.RWMutex
	targets []*url.URL
	healthy []bool
	next    uint64
}

func NewRegistry(raw string) *Registry {
	registry := &Registry{}
	for _, item := range strings.Split(raw, ",") {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		target, err := url.Parse(value)
		if err != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") {
			log.Printf("mengabaikan target service tidak valid: %q", value)
			continue
		}
		registry.targets = append(registry.targets, target)
		registry.healthy = append(registry.healthy, false)
	}
	return registry
}

func (r *Registry) Targets() []*url.URL {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*url.URL, len(r.targets))
	copy(result, r.targets)
	return result
}

func (r *Registry) Pick() *url.URL {
	r.mu.RLock()
	defer r.mu.RUnlock()
	available := make([]int, 0, len(r.targets))
	for index, healthy := range r.healthy {
		if healthy {
			available = append(available, index)
		}
	}
	if len(available) == 0 {
		return nil
	}
	index := available[(atomic.AddUint64(&r.next, 1)-1)%uint64(len(available))]
	return r.targets[index]
}

func (r *Registry) Check(client *http.Client) {
	targets := r.Targets()
	results := make([]bool, len(targets))
	for index, target := range targets {
		requestURL := strings.TrimRight(target.String(), "/") + "/health"
		response, err := client.Get(requestURL)
		if err == nil {
			results[index] = response.StatusCode == http.StatusOK
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for index := range results {
		if index < len(r.healthy) {
			r.healthy[index] = results[index]
		}
	}
}

func (r *Registry) Status() map[string]bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	status := make(map[string]bool, len(r.targets))
	for index, target := range r.targets {
		status[target.String()] = r.healthy[index]
	}
	return status
}

type oauthTokenProvider struct {
	mu          sync.Mutex
	endpoints   []*url.URL
	client      *http.Client
	clientID    string
	secret      string
	accessToken string
	expiresAt   time.Time
}

type oauthTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

func (p *oauthTokenProvider) Token(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.accessToken != "" && time.Until(p.expiresAt) > 60*time.Second {
		return p.accessToken, nil
	}

	var lastErr error
	for _, baseURL := range p.endpoints {
		endpoint := strings.TrimRight(baseURL.String(), "/") + "/oauth/token"
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader("grant_type=client_credentials&scope=service%3Aproxy"))
		if err != nil {
			lastErr = err
			continue
		}
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.SetBasicAuth(p.clientID, p.secret)

		response, err := p.client.Do(request)
		if err != nil {
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		response.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if response.StatusCode != http.StatusOK {
			lastErr = errors.New("OAuth2 token endpoint menolak client credentials")
			continue
		}

		var tokenResponse oauthTokenResponse
		if err := json.Unmarshal(body, &tokenResponse); err != nil || tokenResponse.AccessToken == "" || tokenResponse.ExpiresIn <= 0 || !strings.EqualFold(tokenResponse.TokenType, "Bearer") {
			lastErr = errors.New("respons OAuth2 token endpoint tidak valid")
			continue
		}
		p.accessToken = tokenResponse.AccessToken
		p.expiresAt = time.Now().Add(time.Duration(tokenResponse.ExpiresIn) * time.Second)
		return p.accessToken, nil
	}
	if lastErr == nil {
		lastErr = errors.New("tidak ada Campaign Service yang terdaftar untuk OAuth2")
	}
	return "", lastErr
}

func proxy(registry *Registry, transport http.RoundTripper, tokens *oauthTokenProvider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := registry.Pick()
		if target == nil {
			http.Error(w, "service tidak tersedia", http.StatusServiceUnavailable)
			return
		}
		internalToken, err := tokens.Token(r.Context())
		if err != nil {
			log.Printf("gagal memperoleh OAuth2 service token: %v", err)
			http.Error(w, "service authentication tidak tersedia", http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("X-Served-By", target.String())
		reverseProxy := httputil.NewSingleHostReverseProxy(target)
		reverseProxy.Transport = transport
		reverseProxy.FlushInterval = -1
		originalDirector := reverseProxy.Director
		reverseProxy.Director = func(request *http.Request) {
			originalDirector(request)
			request.Header.Set("X-Internal-Token", internalToken)
			if r.TLS != nil {
				request.Header.Set("X-Forwarded-Proto", "https")
			} else {
				request.Header.Set("X-Forwarded-Proto", "http")
			}
		}
		reverseProxy.ErrorHandler = func(response http.ResponseWriter, request *http.Request, proxyErr error) {
			log.Printf("proxy error ke %s: %v", target.String(), proxyErr)
			http.Error(response, "upstream service tidak tersedia", http.StatusServiceUnavailable)
		}
		reverseProxy.ServeHTTP(w, r)
	})
}

func newUpstreamTransport(mtlsEnabled bool) (*http.Transport, error) {
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if mtlsEnabled {
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
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(caPEM) {
			return nil, errors.New("TLS_CA_FILE tidak berisi CA certificate yang valid")
		}
		certificate, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, err
		}
		tlsConfig.RootCAs = roots
		tlsConfig.Certificates = []tls.Certificate{certificate}
	}

	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: time.Second,
		TLSClientConfig:       tlsConfig,
	}, nil
}

func cors(allowedOrigins map[string]struct{}, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		_, allowed := allowedOrigins[origin]
		if origin != "" && allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Signature")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			if origin != "" && !allowed {
				http.Error(w, "origin tidak diizinkan", http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func parseAllowedOrigins(raw string) map[string]struct{} {
	origins := make(map[string]struct{})
	for _, item := range strings.Split(raw, ",") {
		origin := strings.TrimSpace(item)
		if origin != "" {
			origins[origin] = struct{}{}
		}
	}
	if len(origins) == 0 {
		origins["http://localhost:5173"] = struct{}{}
	}
	return origins
}

func main() {
	mtlsEnabled := strings.EqualFold(os.Getenv("MTLS_ENABLED"), "true")
	transport, err := newUpstreamTransport(mtlsEnabled)
	if err != nil {
		log.Fatalf("konfigurasi koneksi upstream tidak valid: %v", err)
	}
	defer transport.CloseIdleConnections()

	campaign := NewRegistry(os.Getenv("CAMPAIGN_SERVICES"))
	notification := NewRegistry(os.Getenv("NOTIFICATION_SERVICE"))
	if len(campaign.Targets()) == 0 || len(notification.Targets()) == 0 {
		log.Fatal("CAMPAIGN_SERVICES dan NOTIFICATION_SERVICE wajib berisi URL service yang valid")
	}

	client := &http.Client{Transport: transport, Timeout: 4 * time.Second}
	tokens := &oauthTokenProvider{
		endpoints: campaign.Targets(),
		client:    client,
		clientID:  strings.TrimSpace(os.Getenv("OAUTH_CLIENT_ID")),
		secret:    os.Getenv("OAUTH_CLIENT_SECRET"),
	}
	if tokens.clientID == "" || len(tokens.secret) < 32 {
		log.Fatal("OAUTH_CLIENT_ID wajib diisi dan OAUTH_CLIENT_SECRET minimal 32 karakter")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	checkRegistries := func() {
		campaign.Check(client)
		notification.Check(client)
	}
	checkRegistries()
	warmupCtx, cancelWarmup := context.WithTimeout(ctx, 10*time.Second)
	if _, err := tokens.Token(warmupCtx); err != nil {
		cancelWarmup()
		log.Fatalf("OAuth2/mTLS upstream belum siap: %v", err)
	}
	cancelWarmup()
	log.Printf("OAuth2 Client Credentials berhasil diperoleh dari Campaign Service")
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				checkRegistries()
			}
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "gateway"})
	})
	mux.HandleFunc("GET /registry", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"campaign":     campaign.Status(),
			"notification": notification.Status(),
		})
	})
	campaignProxy := proxy(campaign, transport, tokens)
	notificationProxy := proxy(notification, transport, tokens)
	mux.Handle("/auth/", campaignProxy)
	mux.Handle("/graphql", campaignProxy)
	mux.Handle("/webhook/", campaignProxy)
	mux.Handle("/ws/", notificationProxy)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"service": "Crowdfunding API Gateway", "group": "Kelompok E"})
	})

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8000"
	}
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           cors(parseAllowedOrigins(os.Getenv("CORS_ALLOWED_ORIGINS")), mux),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("API Gateway listening on :%s (upstream mTLS=%t)", port, mtlsEnabled)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("API Gateway berhenti: %v", err)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown API Gateway: %v", err)
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("gagal menulis JSON response: %v", err)
	}
}
