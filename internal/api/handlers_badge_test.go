package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/y0f/Asura/internal/storage"
)

func createMonitorOnStatusPage(t *testing.T, srv *Server, adminKey string) int64 {
	t.Helper()

	ctx := context.Background()

	mon := &storage.Monitor{
		Name:             "Badge Test",
		Type:             "http",
		Target:           "https://example.com",
		Interval:         30,
		Timeout:          5,
		FailureThreshold: 3,
		SuccessThreshold: 1,
		Enabled:          true,
	}
	if err := srv.store.CreateMonitor(ctx, mon); err != nil {
		t.Fatal(err)
	}

	sp := &storage.StatusPage{
		Title:   "Test Page",
		Slug:    "test",
		Enabled: true,
	}
	if err := srv.store.CreateStatusPage(ctx, sp); err != nil {
		t.Fatal(err)
	}

	if err := srv.store.SetStatusPageMonitors(ctx, sp.ID, []storage.StatusPageMonitor{
		{PageID: sp.ID, MonitorID: mon.ID, SortOrder: 0},
	}); err != nil {
		t.Fatal(err)
	}

	return mon.ID
}

func TestBadgeStatus(t *testing.T) {
	srv, adminKey := testServer(t)

	t.Run("not found for missing monitor", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/badge/999/status", nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "not found") {
			t.Error("expected 'not found' in badge SVG")
		}
	})

	t.Run("returns SVG for visible monitor", func(t *testing.T) {
		createMonitorOnStatusPage(t, srv, adminKey)

		req := httptest.NewRequest("GET", "/api/v1/badge/1/status", nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		if ct := w.Header().Get("Content-Type"); ct != "image/svg+xml" {
			t.Errorf("content-type = %q, want image/svg+xml", ct)
		}
		if !strings.Contains(w.Body.String(), "<svg") {
			t.Error("expected SVG content")
		}
	})

	t.Run("not found for monitor not on status page", func(t *testing.T) {
		ctx := context.Background()
		mon := &storage.Monitor{
			Name:             "Hidden Monitor",
			Type:             "tcp",
			Target:           "example.com:443",
			Interval:         30,
			Timeout:          5,
			FailureThreshold: 3,
			SuccessThreshold: 1,
			Enabled:          true,
		}
		srv.store.CreateMonitor(ctx, mon)

		req := httptest.NewRequest("GET", "/api/v1/badge/2/status", nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		if !strings.Contains(w.Body.String(), "not found") {
			t.Error("expected 'not found' for monitor not on any status page")
		}
	})
}

func TestBadgeUptime(t *testing.T) {
	srv, adminKey := testServer(t)
	createMonitorOnStatusPage(t, srv, adminKey)

	req := httptest.NewRequest("GET", "/api/v1/badge/1/uptime", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/svg+xml" {
		t.Errorf("content-type = %q, want image/svg+xml", ct)
	}
}

func TestBadgeResponseTime(t *testing.T) {
	srv, adminKey := testServer(t)
	createMonitorOnStatusPage(t, srv, adminKey)

	req := httptest.NewRequest("GET", "/api/v1/badge/1/response", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "image/svg+xml" {
		t.Errorf("content-type = %q, want image/svg+xml", ct)
	}
}

func TestBadgeInvalidID(t *testing.T) {
	srv, _ := testServer(t)

	for _, path := range []string{"/api/v1/badge/abc/status", "/api/v1/badge/abc/uptime", "/api/v1/badge/abc/response"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()
			srv.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", w.Code)
			}
			if !strings.Contains(w.Body.String(), "error") {
				t.Error("expected 'error' in badge SVG for invalid ID")
			}
		})
	}
}
