package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRegistryRoundRobinUsesHealthyTargets(t *testing.T) {
	firstServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer firstServer.Close()

	secondServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer secondServer.Close()

	registry := NewRegistry(firstServer.URL + "," + secondServer.URL)
	registry.Check(&http.Client{Timeout: time.Second})

	first := registry.Pick()
	second := registry.Pick()
	third := registry.Pick()
	if first == nil || second == nil || third == nil {
		t.Fatal("expected all healthy targets to be selectable")
	}
	if first.String() == second.String() {
		t.Fatalf("round robin selected the same target twice: %s", first.String())
	}
	if first.String() != third.String() {
		t.Fatalf("round robin did not cycle back to first target: got %s, want %s", third.String(), first.String())
	}
}

func TestRegistrySkipsUnhealthyTargets(t *testing.T) {
	healthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthyServer.Close()

	unhealthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer unhealthyServer.Close()

	registry := NewRegistry(unhealthyServer.URL + "," + healthyServer.URL)
	registry.Check(&http.Client{Timeout: time.Second})

	for i := 0; i < 5; i++ {
		target := registry.Pick()
		if target == nil {
			t.Fatal("expected one healthy target")
		}
		if target.String() != healthyServer.URL {
			t.Fatalf("selected unhealthy target %s", target.String())
		}
	}
}
