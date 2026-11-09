package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
)
import "github.com/gorilla/websocket"

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool {
	return r.Header.Get("Origin") == "http://localhost:5173" || r.Header.Get("Origin") == "http://localhost:8000"
}}

type Hub struct {
	mu      sync.Mutex
	clients map[*websocket.Conn]bool
}

var hub = Hub{clients: map[*websocket.Conn]bool{}}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","service":"notification"}`))
	})
	mux.HandleFunc("POST /internal/broadcast", func(w http.ResponseWriter, r *http.Request) {
		var msg map[string]interface{}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&msg) != nil {
			http.Error(w, "payload tidak valid", 400)
			return
		}
		hub.mu.Lock()
		defer hub.mu.Unlock()
		for c := range hub.clients {
			if e := c.WriteJSON(msg); e != nil {
				c.Close()
				delete(hub.clients, c)
			}
		}
		w.WriteHeader(202)
	})
	mux.HandleFunc("/ws/donasi", func(w http.ResponseWriter, r *http.Request) {
		c, e := upgrader.Upgrade(w, r, nil)
		if e != nil {
			return
		}
		hub.mu.Lock()
		hub.clients[c] = true
		hub.mu.Unlock()
		defer func() { hub.mu.Lock(); delete(hub.clients, c); hub.mu.Unlock(); c.Close() }()
		for {
			if _, _, e = c.ReadMessage(); e != nil {
				return
			}
		}
	})
	p := os.Getenv("PORT")
	if p == "" {
		p = "8002"
	}
	log.Fatal(http.ListenAndServe(":"+p, mux))
}
