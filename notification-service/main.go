package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	serviceTokenIssuer   = "crowdfunding-oauth"
	serviceTokenAudience = "crowdfunding-services"
	serviceTokenScope    = "service:proxy"
	eventExchange        = "crowdfunding.events"
	eventQueue           = "notification.events"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 54 * time.Second
)

type client struct {
	connection *websocket.Conn
	send       chan []byte
}

type Hub struct {
	mu      sync.Mutex
	clients map[*client]struct{}
}

var (
	jwtSecret     []byte
	mtlsEnabled   bool
	allowedOrigin map[string]struct{}
	hub           = Hub{clients: make(map[*client]struct{})}
	upgrader      websocket.Upgrader
)

func main() {
	jwtSecret = []byte(os.Getenv("JWT_SECRET"))
	mtlsEnabled = strings.EqualFold(os.Getenv("MTLS_ENABLED"), "true")
	if len(jwtSecret) < 32 {
		log.Fatal("JWT_SECRET wajib berisi minimal 32 karakter")
	}
	if strings.TrimSpace(os.Getenv("OAUTH_CLIENT_ID")) == "" {
		log.Fatal("OAUTH_CLIENT_ID wajib diisi")
	}
	rabbitURL := strings.TrimSpace(os.Getenv("RABBITMQ_URL"))
	if rabbitURL == "" {
		log.Fatal("RABBITMQ_URL wajib diisi")
	}
	allowedOrigin = parseAllowedOrigins(os.Getenv("CORS_ALLOWED_ORIGINS"))
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			_, ok := allowedOrigin[strings.TrimSpace(r.Header.Get("Origin"))]
			return ok
		},
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var consumerWG sync.WaitGroup
	consumerWG.Add(1)
	go func() {
		defer consumerWG.Done()
		consumeEvents(ctx, rabbitURL)
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "notification"})
	})
	mux.HandleFunc("GET /ws/donasi", websocketHandler)

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8443"
	}
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           requireMTLS(internalAuthMiddleware(mux)),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	if mtlsEnabled {
		tlsConfig, err := serverTLSConfig()
		if err != nil {
			log.Fatalf("konfigurasi mTLS tidak valid: %v", err)
		}
		server.TLSConfig = tlsConfig
	}

	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("Notification Service listening on :%s (mTLS=%t)", port, mtlsEnabled)
		if mtlsEnabled {
			serverErrors <- server.ListenAndServeTLS(os.Getenv("TLS_CERT_FILE"), os.Getenv("TLS_KEY_FILE"))
			return
		}
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Notification Service berhenti: %v", err)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown Notification Service: %v", err)
		}
		consumerWG.Wait()
		hub.closeAll()
	}
}

func (h *Hub) register(connection *websocket.Conn) *client {
	newClient := &client{connection: connection, send: make(chan []byte, 32)}
	h.mu.Lock()
	h.clients[newClient] = struct{}{}
	h.mu.Unlock()
	return newClient
}

func (h *Hub) unregister(c *client) {
	h.mu.Lock()
	if _, exists := h.clients[c]; exists {
		delete(h.clients, c)
		close(c.send)
	}
	h.mu.Unlock()
}

func (h *Hub) broadcast(message []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		copyOfMessage := append([]byte(nil), message...)
		select {
		case c.send <- copyOfMessage:
		default:
			// Client yang terlalu lambat dikeluarkan agar tidak menahan consumer MQ.
			delete(h.clients, c)
			close(c.send)
			_ = c.connection.Close()
		}
	}
}

func (h *Hub) closeAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		delete(h.clients, c)
		close(c.send)
		_ = c.connection.Close()
	}
}

func websocketHandler(w http.ResponseWriter, r *http.Request) {
	connection, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := hub.register(connection)
	go c.writePump()
	c.readPump()
}

func (c *client) readPump() {
	defer func() {
		hub.unregister(c)
		_ = c.connection.Close()
	}()
	c.connection.SetReadLimit(4096)
	_ = c.connection.SetReadDeadline(time.Now().Add(pongWait))
	c.connection.SetPongHandler(func(string) error {
		return c.connection.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		if _, _, err := c.connection.ReadMessage(); err != nil {
			return
		}
	}
}

func (c *client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		hub.unregister(c)
		_ = c.connection.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			_ = c.connection.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.connection.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.connection.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.connection.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.connection.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func consumeEvents(ctx context.Context, rabbitURL string) {
	backoff := time.Second
	for ctx.Err() == nil {
		connection, err := amqp.DialConfig(rabbitURL, amqp.Config{Heartbeat: 10 * time.Second, Locale: "en_US", Dial: amqp.DefaultDial(5 * time.Second)})
		if err != nil {
			log.Printf("notification MQ: koneksi RabbitMQ gagal: %v", err)
			if !sleepContext(ctx, backoff) {
				return
			}
			backoff = minDuration(backoff*2, 30*time.Second)
			continue
		}

		channel, err := connection.Channel()
		if err == nil {
			err = channel.ExchangeDeclare(eventExchange, "topic", true, false, false, false, nil)
		}
		var queue amqp.Queue
		if err == nil {
			queue, err = channel.QueueDeclare(eventQueue, true, false, false, false, nil)
		}
		if err == nil {
			err = channel.QueueBind(queue.Name, "#", eventExchange, false, nil)
		}
		var deliveries <-chan amqp.Delivery
		if err == nil {
			err = channel.Qos(20, 0, false)
		}
		if err == nil {
			deliveries, err = channel.Consume(queue.Name, "notification-service", false, false, false, false, nil)
		}
		if err != nil {
			log.Printf("notification MQ: gagal menyiapkan consumer: %v", err)
			if channel != nil {
				_ = channel.Close()
			}
			_ = connection.Close()
			if !sleepContext(ctx, backoff) {
				return
			}
			backoff = minDuration(backoff*2, 30*time.Second)
			continue
		}

		log.Printf("notification MQ: consumer aktif pada queue %s", eventQueue)
		backoff = time.Second
		for {
			select {
			case <-ctx.Done():
				_ = channel.Close()
				_ = connection.Close()
				return
			case delivery, ok := <-deliveries:
				if !ok {
					log.Printf("notification MQ: channel consumer ditutup; mencoba reconnect")
					_ = channel.Close()
					_ = connection.Close()
					goto reconnect
				}
				if !json.Valid(delivery.Body) {
					_ = delivery.Nack(false, false)
					continue
				}
				hub.broadcast(delivery.Body)
				if err := delivery.Ack(false); err != nil {
					log.Printf("notification MQ: gagal ACK message: %v", err)
					_ = channel.Close()
					_ = connection.Close()
					goto reconnect
				}
			}
		}

	reconnect:
		if !sleepContext(ctx, backoff) {
			return
		}
		backoff = minDuration(backoff*2, 30*time.Second)
	}
}

func internalAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
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
	scope, _ := claims["scope"].(string)
	subject, _ := claims["sub"].(string)
	return scope == serviceTokenScope && subject == strings.TrimSpace(os.Getenv("OAUTH_CLIENT_ID"))
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
	return &tls.Config{MinVersion: tls.VersionTLS12, ClientCAs: clientCAs, ClientAuth: tls.VerifyClientCertIfGiven}, nil
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

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("gagal menulis JSON response: %v", err)
	}
}

func sleepContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
