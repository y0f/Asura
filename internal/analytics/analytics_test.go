package analytics

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/y0f/Asura/internal/storage"
)

func testStore(t *testing.T) *storage.SQLiteStore {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "asura-analytics-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	store, err := storage.NewSQLiteStore(tmpFile.Name(), 2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestComputeMetrics(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	mon := &storage.Monitor{
		Name:             "Analytics Test",
		Type:             "http",
		Target:           "https://example.com",
		Interval:         60,
		Timeout:          10,
		Enabled:          true,
		FailureThreshold: 3,
		SuccessThreshold: 1,
	}
	if err := store.CreateMonitor(ctx, mon); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	from := now.Add(-1 * time.Hour)
	to := now.Add(1 * time.Hour)

	checks := []struct {
		status       string
		responseTime int64
	}{
		{"up", 50},
		{"up", 100},
		{"up", 150},
		{"up", 200},
		{"up", 250},
		{"up", 300},
		{"up", 350},
		{"down", 0},
		{"degraded", 400},
		{"up", 100},
	}

	for _, c := range checks {
		cr := &storage.CheckResult{
			MonitorID:    mon.ID,
			Status:       c.status,
			ResponseTime: c.responseTime,
		}
		if err := store.InsertCheckResult(ctx, cr); err != nil {
			t.Fatal(err)
		}
	}

	metrics, err := ComputeMetrics(ctx, store, mon.ID, from, to)
	if err != nil {
		t.Fatal(err)
	}

	if metrics.MonitorID != mon.ID {
		t.Fatalf("expected monitor_id %d, got %d", mon.ID, metrics.MonitorID)
	}

	if metrics.TotalChecks != 10 {
		t.Fatalf("expected 10 total checks, got %d", metrics.TotalChecks)
	}

	if metrics.UpChecks != 8 {
		t.Fatalf("expected 8 up checks, got %d", metrics.UpChecks)
	}

	if metrics.DownChecks != 1 {
		t.Fatalf("expected 1 down check, got %d", metrics.DownChecks)
	}

	if metrics.DegradedChecks != 1 {
		t.Fatalf("expected 1 degraded check, got %d", metrics.DegradedChecks)
	}

	// Uptime should be ~80% (8 up out of 10)
	if metrics.UptimePct < 79 || metrics.UptimePct > 81 {
		t.Fatalf("expected uptime ~80%%, got %.2f%%", metrics.UptimePct)
	}

	if metrics.P50 <= 0 {
		t.Fatal("expected P50 > 0")
	}
	if metrics.P95 <= 0 {
		t.Fatal("expected P95 > 0")
	}
	if metrics.P99 <= 0 {
		t.Fatal("expected P99 > 0")
	}

	if metrics.P50 > metrics.P95 {
		t.Fatalf("expected P50 <= P95, got P50=%v P95=%v", metrics.P50, metrics.P95)
	}
	if metrics.P95 > metrics.P99 {
		t.Fatalf("expected P95 <= P99, got P95=%v P99=%v", metrics.P95, metrics.P99)
	}
}
