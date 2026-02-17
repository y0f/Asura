package storage

import (
	"context"
	"os"
	"testing"
	"time"
)

func testStore(t *testing.T) *SQLiteStore {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "asura-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()
	t.Cleanup(func() { os.Remove(tmpFile.Name()) })

	store, err := NewSQLiteStore(tmpFile.Name(), 2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestMonitorCRUD(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	// Create
	m := &Monitor{
		Name:             "Test HTTP",
		Type:             "http",
		Target:           "https://example.com",
		Interval:         60,
		Timeout:          10,
		Enabled:          true,
		Tags:             []string{"prod"},
		FailureThreshold: 3,
		SuccessThreshold: 1,
	}
	err := store.CreateMonitor(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	if m.ID == 0 {
		t.Fatal("expected non-zero ID")
	}

	// Get
	got, err := store.GetMonitor(ctx, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Test HTTP" {
		t.Fatalf("expected 'Test HTTP', got %q", got.Name)
	}
	if got.Status != "pending" {
		t.Fatalf("expected status 'pending', got %q", got.Status)
	}

	// List
	result, err := store.ListMonitors(ctx, Pagination{Page: 1, PerPage: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 {
		t.Fatalf("expected 1 monitor, got %d", result.Total)
	}

	// Update
	m.Name = "Updated HTTP"
	m.Tags = []string{"prod", "web"}
	err = store.UpdateMonitor(ctx, m)
	if err != nil {
		t.Fatal(err)
	}

	got, _ = store.GetMonitor(ctx, m.ID)
	if got.Name != "Updated HTTP" {
		t.Fatalf("expected 'Updated HTTP', got %q", got.Name)
	}

	// Pause/Resume
	err = store.SetMonitorEnabled(ctx, m.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	got, _ = store.GetMonitor(ctx, m.ID)
	if got.Enabled {
		t.Fatal("expected disabled")
	}

	// Delete
	err = store.DeleteMonitor(ctx, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.GetMonitor(ctx, m.ID)
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestCheckResults(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	m := &Monitor{Name: "Test", Type: "http", Target: "https://example.com", Interval: 60, Timeout: 10, Enabled: true, Tags: []string{}, FailureThreshold: 3, SuccessThreshold: 1}
	store.CreateMonitor(ctx, m)

	cr := &CheckResult{
		MonitorID:    m.ID,
		Status:       "up",
		ResponseTime: 150,
		StatusCode:   200,
		Message:      "OK",
		BodyHash:     "abc123",
	}
	err := store.InsertCheckResult(ctx, cr)
	if err != nil {
		t.Fatal(err)
	}

	result, err := store.ListCheckResults(ctx, m.ID, Pagination{Page: 1, PerPage: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 {
		t.Fatalf("expected 1 result, got %d", result.Total)
	}

	latest, err := store.GetLatestCheckResult(ctx, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", latest.StatusCode)
	}
}

func TestIncidents(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	m := &Monitor{Name: "Test", Type: "http", Target: "https://example.com", Interval: 60, Timeout: 10, Enabled: true, Tags: []string{}, FailureThreshold: 3, SuccessThreshold: 1}
	store.CreateMonitor(ctx, m)

	inc := &Incident{MonitorID: m.ID, Status: "open", Cause: "timeout"}
	err := store.CreateIncident(ctx, inc)
	if err != nil {
		t.Fatal(err)
	}

	open, err := store.GetOpenIncident(ctx, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if open.ID != inc.ID {
		t.Fatal("wrong open incident")
	}

	// Resolve
	now := time.Now().UTC()
	inc.Status = "resolved"
	inc.ResolvedAt = &now
	inc.ResolvedBy = "test"
	err = store.UpdateIncident(ctx, inc)
	if err != nil {
		t.Fatal(err)
	}

	result, err := store.ListIncidents(ctx, 0, "", Pagination{Page: 1, PerPage: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 {
		t.Fatalf("expected 1 incident, got %d", result.Total)
	}
}

func TestAnalytics(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	m := &Monitor{Name: "Test", Type: "http", Target: "https://example.com", Interval: 60, Timeout: 10, Enabled: true, Tags: []string{}, FailureThreshold: 3, SuccessThreshold: 1}
	store.CreateMonitor(ctx, m)

	for i := 0; i < 10; i++ {
		status := "up"
		if i == 5 {
			status = "down"
		}
		store.InsertCheckResult(ctx, &CheckResult{
			MonitorID: m.ID, Status: status, ResponseTime: int64(100 + i*10), StatusCode: 200,
		})
	}

	from := time.Now().Add(-1 * time.Hour)
	to := time.Now().Add(1 * time.Hour)

	uptime, err := store.GetUptimePercent(ctx, m.ID, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if uptime != 90 {
		t.Fatalf("expected 90%% uptime, got %f", uptime)
	}

	p50, p95, p99, err := store.GetResponseTimePercentiles(ctx, m.ID, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if p50 == 0 || p95 == 0 || p99 == 0 {
		t.Fatalf("expected non-zero percentiles: p50=%f p95=%f p99=%f", p50, p95, p99)
	}
}

func createTestHeartbeat(t *testing.T) (*SQLiteStore, context.Context, *Monitor) {
	t.Helper()
	store := testStore(t)
	ctx := context.Background()
	m := &Monitor{Name: "Cron Job", Type: "heartbeat", Target: "heartbeat", Interval: 300, Timeout: 10, Enabled: true, Tags: []string{}, FailureThreshold: 1, SuccessThreshold: 1}
	if err := store.CreateMonitor(ctx, m); err != nil {
		t.Fatal(err)
	}
	hb := &Heartbeat{
		MonitorID: m.ID,
		Token:     "abc123def456",
		Grace:     60,
		Status:    "pending",
	}
	if err := store.CreateHeartbeat(ctx, hb); err != nil {
		t.Fatal(err)
	}
	if hb.ID == 0 {
		t.Fatal("expected non-zero heartbeat ID")
	}
	return store, ctx, m
}

func TestHeartbeatCRUD(t *testing.T) {
	store, ctx, m := createTestHeartbeat(t)

	t.Run("GetByToken", func(t *testing.T) {
		got, err := store.GetHeartbeatByToken(ctx, "abc123def456")
		if err != nil {
			t.Fatal(err)
		}
		if got.MonitorID != m.ID {
			t.Fatalf("expected monitor_id %d, got %d", m.ID, got.MonitorID)
		}
		if got.Grace != 60 {
			t.Fatalf("expected grace 60, got %d", got.Grace)
		}
	})

	t.Run("GetByMonitorID", func(t *testing.T) {
		got, err := store.GetHeartbeatByMonitorID(ctx, m.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Token != "abc123def456" {
			t.Fatalf("expected token abc123def456, got %s", got.Token)
		}
	})

	t.Run("UpdatePing", func(t *testing.T) {
		if err := store.UpdateHeartbeatPing(ctx, "abc123def456"); err != nil {
			t.Fatal(err)
		}
		got, _ := store.GetHeartbeatByToken(ctx, "abc123def456")
		if got.Status != "up" {
			t.Fatalf("expected status up after ping, got %s", got.Status)
		}
		if got.LastPingAt == nil {
			t.Fatal("expected last_ping_at to be set")
		}
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		if err := store.UpdateHeartbeatStatus(ctx, m.ID, "down"); err != nil {
			t.Fatal(err)
		}
		got, _ := store.GetHeartbeatByToken(ctx, "abc123def456")
		if got.Status != "down" {
			t.Fatalf("expected status down, got %s", got.Status)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		if err := store.DeleteHeartbeat(ctx, m.ID); err != nil {
			t.Fatal(err)
		}
		_, err := store.GetHeartbeatByToken(ctx, "abc123def456")
		if err == nil {
			t.Fatal("expected error after delete")
		}
	})
}

func TestMonitorPublicFlag(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	m := &Monitor{Name: "Public", Type: "http", Target: "https://example.com", Interval: 60, Timeout: 10, Enabled: true, Tags: []string{}, FailureThreshold: 3, SuccessThreshold: 1, Public: true}
	err := store.CreateMonitor(ctx, m)
	if err != nil {
		t.Fatal(err)
	}

	got, err := store.GetMonitor(ctx, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Public {
		t.Fatal("expected public=true")
	}

	// Update to private
	m.Public = false
	err = store.UpdateMonitor(ctx, m)
	if err != nil {
		t.Fatal(err)
	}
	got, _ = store.GetMonitor(ctx, m.ID)
	if got.Public {
		t.Fatal("expected public=false after update")
	}
}

func TestTags(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	store.CreateMonitor(ctx, &Monitor{Name: "A", Type: "http", Target: "https://a.com", Interval: 60, Timeout: 10, Enabled: true, Tags: []string{"web", "prod"}, FailureThreshold: 3, SuccessThreshold: 1})
	store.CreateMonitor(ctx, &Monitor{Name: "B", Type: "http", Target: "https://b.com", Interval: 60, Timeout: 10, Enabled: true, Tags: []string{"api", "prod"}, FailureThreshold: 3, SuccessThreshold: 1})

	tags, err := store.ListTags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 3 {
		t.Fatalf("expected 3 tags, got %d: %v", len(tags), tags)
	}
}
