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

	checker := &HTTPChecker{AllowPrivate: true}
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

	checker := &HTTPChecker{AllowPrivate: true}
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
	checker := &HTTPChecker{AllowPrivate: true}
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

func TestHTTPCheckerExpectedStatus(t *testing.T) {
	tests := []struct {
		name           string
		serverStatus   int
		expectedStatus int
		wantStatus     string
		wantMessage    string
	}{
		{
			name:           "matching expected status",
			serverStatus:   200,
			expectedStatus: 200,
			wantStatus:     "up",
		},
		{
			name:           "mismatched expected status",
			serverStatus:   502,
			expectedStatus: 200,
			wantStatus:     "down",
			wantMessage:    "expected status 200, got 502",
		},
		{
			name:           "no expected status set",
			serverStatus:   502,
			expectedStatus: 0,
			wantStatus:     "up",
		},
		{
			name:           "expected non-200 status matches",
			serverStatus:   201,
			expectedStatus: 201,
			wantStatus:     "up",
		},
		{
			name:           "expected non-200 status mismatches",
			serverStatus:   200,
			expectedStatus: 201,
			wantStatus:     "down",
			wantMessage:    "expected status 201, got 200",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.serverStatus)
			}))
			defer server.Close()

			settings, _ := json.Marshal(storage.HTTPSettings{
				ExpectedStatus: tt.expectedStatus,
			})

			c := &HTTPChecker{AllowPrivate: true}
			monitor := &storage.Monitor{
				Target:   server.URL,
				Timeout:  5,
				Settings: settings,
			}

			result, err := c.Check(context.Background(), monitor)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q (message: %s)", result.Status, tt.wantStatus, result.Message)
			}
			if tt.wantMessage != "" && result.Message != tt.wantMessage {
				t.Errorf("message = %q, want %q", result.Message, tt.wantMessage)
			}
			if result.StatusCode != tt.serverStatus {
				t.Errorf("status_code = %d, want %d", result.StatusCode, tt.serverStatus)
			}
		})
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
	r := DefaultRegistry(nil, false)
	types := []string{"http", "tcp", "dns", "icmp", "tls", "websocket", "command"}
	for _, typ := range types {
		if _, err := r.Get(typ); err != nil {
			t.Fatalf("expected %s checker, got error: %v", typ, err)
		}
	}
}
