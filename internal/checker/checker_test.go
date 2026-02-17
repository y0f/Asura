package checker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/y0f/Asura/internal/storage"
)

func TestHTTPChecker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	checker := &HTTPChecker{}
	monitor := &storage.Monitor{
		Target:  server.URL,
		Timeout: 5,
	}

	result, err := checker.Check(context.Background(), monitor)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "up" {
		t.Fatalf("expected up, got %s: %s", result.Status, result.Message)
	}
	if result.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", result.StatusCode)
	}
	if result.Body != `{"status":"ok"}` {
		t.Fatalf("unexpected body: %s", result.Body)
	}
	if result.BodyHash == "" {
		t.Fatal("expected body hash")
	}
}

func TestHTTPCheckerWithSettings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("X-Custom") != "test" {
			t.Fatal("expected custom header")
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	settings, _ := json.Marshal(storage.HTTPSettings{
		Method:  "POST",
		Headers: map[string]string{"X-Custom": "test"},
	})

	checker := &HTTPChecker{}
	monitor := &storage.Monitor{
		Target:   server.URL,
		Timeout:  5,
		Settings: settings,
	}

	result, err := checker.Check(context.Background(), monitor)
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != 201 {
		t.Fatalf("expected 201, got %d", result.StatusCode)
	}
}

func TestHTTPCheckerDown(t *testing.T) {
	checker := &HTTPChecker{}
	monitor := &storage.Monitor{
		Target:  "http://192.0.2.1:1", // non-routable, will timeout
		Timeout: 1,
	}

	result, err := checker.Check(context.Background(), monitor)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "down" {
		t.Fatalf("expected down, got %s", result.Status)
	}
}

func TestRegistryGetUnregistered(t *testing.T) {
	r := NewRegistry()
	_, err := r.Get("unknown")
	if err == nil {
		t.Fatal("expected error for unregistered type")
	}
}

func TestDefaultRegistryHasAllTypes(t *testing.T) {
	r := DefaultRegistry(nil)
	types := []string{"http", "tcp", "dns", "icmp", "tls", "websocket", "command"}
	for _, typ := range types {
		if _, err := r.Get(typ); err != nil {
			t.Fatalf("expected %s checker, got error: %v", typ, err)
		}
	}
}
