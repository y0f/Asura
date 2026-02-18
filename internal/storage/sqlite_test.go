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

func TestSessionCRUD(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	t.Run("CreateAndGet", func(t *testing.T) {
		sess := &Session{
			TokenHash:  "abc123hash",
			APIKeyName: "admin",
			IPAddress:  "192.168.1.1",
			ExpiresAt:  time.Now().Add(24 * time.Hour),
		}
		if err := store.CreateSession(ctx, sess); err != nil {
			t.Fatal(err)
		}
		if sess.ID == 0 {
			t.Fatal("expected non-zero ID")
		}

		got, err := store.GetSessionByTokenHash(ctx, "abc123hash")
		if err != nil {
			t.Fatal(err)
		}
		if got.APIKeyName != "admin" {
			t.Fatalf("expected api_key_name 'admin', got %q", got.APIKeyName)
		}
		if got.IPAddress != "192.168.1.1" {
			t.Fatalf("expected ip '192.168.1.1', got %q", got.IPAddress)
		}
	})

	t.Run("GetNotFound", func(t *testing.T) {
		_, err := store.GetSessionByTokenHash(ctx, "nonexistent")
		if err == nil {
			t.Fatal("expected error for nonexistent session")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		sess := &Session{
			TokenHash:  "deleteme",
			APIKeyName: "admin",
			ExpiresAt:  time.Now().Add(24 * time.Hour),
		}
		store.CreateSession(ctx, sess)

		if err := store.DeleteSession(ctx, "deleteme"); err != nil {
			t.Fatal(err)
		}
		_, err := store.GetSessionByTokenHash(ctx, "deleteme")
		if err == nil {
			t.Fatal("expected error after delete")
		}
	})

	t.Run("DeleteExpired", func(t *testing.T) {
		// Create an already-expired session
		expired := &Session{
			TokenHash:  "expired_token",
			APIKeyName: "admin",
			ExpiresAt:  time.Now().Add(-1 * time.Hour),
		}
		store.CreateSession(ctx, expired)

		// Create a valid session
		valid := &Session{
			TokenHash:  "valid_token",
			APIKeyName: "admin",
			ExpiresAt:  time.Now().Add(24 * time.Hour),
		}
		store.CreateSession(ctx, valid)

		deleted, err := store.DeleteExpiredSessions(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if deleted == 0 {
			t.Fatal("expected at least 1 expired session deleted")
		}

		// Expired session should be gone
		_, err = store.GetSessionByTokenHash(ctx, "expired_token")
		if err == nil {
			t.Fatal("expected expired session to be deleted")
		}

		// Valid session should still exist
		got, err := store.GetSessionByTokenHash(ctx, "valid_token")
		if err != nil {
			t.Fatal(err)
		}
		if got.TokenHash != "valid_token" {
			t.Fatal("valid session should still exist")
		}
	})

	t.Run("DeleteByAPIKeyName", func(t *testing.T) {
		s2 := testStore(t)
		s2.CreateSession(ctx, &Session{TokenHash: "key1_sess1", APIKeyName: "key1", ExpiresAt: time.Now().Add(24 * time.Hour)})
		s2.CreateSession(ctx, &Session{TokenHash: "key1_sess2", APIKeyName: "key1", ExpiresAt: time.Now().Add(24 * time.Hour)})
		s2.CreateSession(ctx, &Session{TokenHash: "key2_sess1", APIKeyName: "key2", ExpiresAt: time.Now().Add(24 * time.Hour)})

		deleted, err := s2.DeleteSessionsByAPIKeyName(ctx, "key1")
		if err != nil {
			t.Fatal(err)
		}
		if deleted != 2 {
			t.Fatalf("expected 2 deleted, got %d", deleted)
		}

		_, err = s2.GetSessionByTokenHash(ctx, "key1_sess1")
		if err == nil {
			t.Fatal("expected key1 session to be deleted")
		}
		got, err := s2.GetSessionByTokenHash(ctx, "key2_sess1")
		if err != nil {
			t.Fatal(err)
		}
		if got.APIKeyName != "key2" {
			t.Fatal("key2 session should still exist")
		}
	})

	t.Run("DeleteExceptKeyNames", func(t *testing.T) {
		s3 := testStore(t)
		s3.CreateSession(ctx, &Session{TokenHash: "admin_tok", APIKeyName: "admin", ExpiresAt: time.Now().Add(24 * time.Hour)})
		s3.CreateSession(ctx, &Session{TokenHash: "readonly_tok", APIKeyName: "readonly", ExpiresAt: time.Now().Add(24 * time.Hour)})
		s3.CreateSession(ctx, &Session{TokenHash: "removed_tok", APIKeyName: "removed", ExpiresAt: time.Now().Add(24 * time.Hour)})

		deleted, err := s3.DeleteSessionsExceptKeyNames(ctx, []string{"admin", "readonly"})
		if err != nil {
			t.Fatal(err)
		}
		if deleted != 1 {
			t.Fatalf("expected 1 deleted, got %d", deleted)
		}

		_, err = s3.GetSessionByTokenHash(ctx, "removed_tok")
		if err == nil {
			t.Fatal("expected removed key session to be deleted")
		}
		if _, err := s3.GetSessionByTokenHash(ctx, "admin_tok"); err != nil {
			t.Fatal("admin session should still exist")
		}
		if _, err := s3.GetSessionByTokenHash(ctx, "readonly_tok"); err != nil {
			t.Fatal("readonly session should still exist")
		}
	})

	t.Run("DeleteExceptKeyNamesEmpty", func(t *testing.T) {
		s4 := testStore(t)
		s4.CreateSession(ctx, &Session{TokenHash: "tok1", APIKeyName: "any", ExpiresAt: time.Now().Add(24 * time.Hour)})

		deleted, err := s4.DeleteSessionsExceptKeyNames(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if deleted != 1 {
			t.Fatalf("expected 1 deleted, got %d", deleted)
		}
	})

	t.Run("KeyHashStored", func(t *testing.T) {
		s5 := testStore(t)
		s5.CreateSession(ctx, &Session{
			TokenHash:  "hash_test_tok",
			APIKeyName: "admin",
			KeyHash:    "abcdef123456",
			ExpiresAt:  time.Now().Add(24 * time.Hour),
		})

		got, err := s5.GetSessionByTokenHash(ctx, "hash_test_tok")
		if err != nil {
			t.Fatal(err)
		}
		if got.KeyHash != "abcdef123456" {
			t.Fatalf("expected key_hash 'abcdef123456', got %q", got.KeyHash)
		}
	})
}

func TestRequestLogBatchInsertAndList(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	now := time.Now().UTC()
	mid := int64(1)
	logs := []*RequestLog{
		{Method: "GET", Path: "/", StatusCode: 200, LatencyMs: 5, ClientIP: "aaa", UserAgent: "Mozilla/5.0", RouteGroup: "web", CreatedAt: now},
		{Method: "GET", Path: "/api/v1/monitors", StatusCode: 200, LatencyMs: 12, ClientIP: "bbb", RouteGroup: "api", CreatedAt: now},
		{Method: "GET", Path: "/api/v1/badge/1/status", StatusCode: 200, LatencyMs: 3, ClientIP: "aaa", MonitorID: &mid, RouteGroup: "badge", CreatedAt: now},
		{Method: "POST", Path: "/login", StatusCode: 303, LatencyMs: 80, ClientIP: "ccc", RouteGroup: "auth", CreatedAt: now},
	}

	if err := store.InsertRequestLogBatch(ctx, logs); err != nil {
		t.Fatal(err)
	}

	// List all
	result, err := store.ListRequestLogs(ctx, RequestLogFilter{}, Pagination{Page: 1, PerPage: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 4 {
		t.Fatalf("expected 4 logs, got %d", result.Total)
	}
	entries := result.Data.([]*RequestLog)
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	// Filter by route group
	result, err = store.ListRequestLogs(ctx, RequestLogFilter{RouteGroup: "api"}, Pagination{Page: 1, PerPage: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 {
		t.Fatalf("expected 1 api log, got %d", result.Total)
	}

	// Filter by method
	result, err = store.ListRequestLogs(ctx, RequestLogFilter{Method: "POST"}, Pagination{Page: 1, PerPage: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 {
		t.Fatalf("expected 1 POST log, got %d", result.Total)
	}

	// Filter by monitor_id
	result, err = store.ListRequestLogs(ctx, RequestLogFilter{MonitorID: &mid}, Pagination{Page: 1, PerPage: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 {
		t.Fatalf("expected 1 monitor-linked log, got %d", result.Total)
	}
	entry := result.Data.([]*RequestLog)[0]
	if entry.MonitorID == nil || *entry.MonitorID != 1 {
		t.Fatal("expected monitor_id=1")
	}
}

func TestRequestLogStats(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	now := time.Now().UTC()
	logs := []*RequestLog{
		{Method: "GET", Path: "/", StatusCode: 200, LatencyMs: 10, ClientIP: "aaa", RouteGroup: "web", CreatedAt: now},
		{Method: "GET", Path: "/monitors", StatusCode: 200, LatencyMs: 20, ClientIP: "aaa", RouteGroup: "web", CreatedAt: now},
		{Method: "GET", Path: "/api/v1/monitors", StatusCode: 200, LatencyMs: 30, ClientIP: "bbb", RouteGroup: "api", CreatedAt: now},
	}
	if err := store.InsertRequestLogBatch(ctx, logs); err != nil {
		t.Fatal(err)
	}

	stats, err := store.GetRequestLogStats(ctx, now.Add(-1*time.Hour), now.Add(1*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalRequests != 3 {
		t.Fatalf("expected 3 total requests, got %d", stats.TotalRequests)
	}
	if stats.UniqueVisitors != 2 {
		t.Fatalf("expected 2 unique visitors, got %d", stats.UniqueVisitors)
	}
	if stats.AvgLatencyMs != 20 {
		t.Fatalf("expected avg latency 20, got %d", stats.AvgLatencyMs)
	}
	if len(stats.TopPaths) < 1 {
		t.Fatal("expected at least 1 top path")
	}
}

func TestRequestLogRollup(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	date := "2025-01-15"
	ts, _ := time.Parse("2006-01-02T15:04:05Z", date+"T12:00:00Z")
	logs := []*RequestLog{
		{Method: "GET", Path: "/", StatusCode: 200, LatencyMs: 10, ClientIP: "aaa", RouteGroup: "web", CreatedAt: ts},
		{Method: "GET", Path: "/", StatusCode: 200, LatencyMs: 20, ClientIP: "bbb", RouteGroup: "web", CreatedAt: ts},
		{Method: "GET", Path: "/api/v1/health", StatusCode: 200, LatencyMs: 5, ClientIP: "aaa", RouteGroup: "api", CreatedAt: ts},
	}
	if err := store.InsertRequestLogBatch(ctx, logs); err != nil {
		t.Fatal(err)
	}

	if err := store.RollupRequestLogs(ctx, date); err != nil {
		t.Fatal(err)
	}

	// Verify rollup data exists
	var count int
	err := store.readDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM request_log_rollups WHERE date=?", date).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count < 1 {
		t.Fatal("expected at least 1 rollup row")
	}

	// Running rollup again should not error (INSERT OR REPLACE)
	if err := store.RollupRequestLogs(ctx, date); err != nil {
		t.Fatal("second rollup should not error:", err)
	}
}

func TestRequestLogPurge(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	old := time.Now().UTC().AddDate(0, 0, -60)
	recent := time.Now().UTC()

	logs := []*RequestLog{
		{Method: "GET", Path: "/old", StatusCode: 200, LatencyMs: 5, ClientIP: "aaa", RouteGroup: "web", CreatedAt: old},
		{Method: "GET", Path: "/new", StatusCode: 200, LatencyMs: 5, ClientIP: "bbb", RouteGroup: "web", CreatedAt: recent},
	}
	if err := store.InsertRequestLogBatch(ctx, logs); err != nil {
		t.Fatal(err)
	}

	deleted, err := store.PurgeOldRequestLogs(ctx, time.Now().UTC().AddDate(0, 0, -30))
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted, got %d", deleted)
	}

	result, err := store.ListRequestLogs(ctx, RequestLogFilter{}, Pagination{Page: 1, PerPage: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 {
		t.Fatalf("expected 1 remaining log, got %d", result.Total)
	}
}

func TestInsertRequestLogBatchEmpty(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()

	if err := store.InsertRequestLogBatch(ctx, nil); err != nil {
		t.Fatal("empty batch should not error:", err)
	}
	if err := store.InsertRequestLogBatch(ctx, []*RequestLog{}); err != nil {
		t.Fatal("empty slice should not error:", err)
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
