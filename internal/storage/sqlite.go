package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"runtime"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteStore implements Store using SQLite with WAL mode.
type SQLiteStore struct {
	readDB  *sql.DB
	writeDB *sql.DB
}

// NewSQLiteStore opens the database with separate read and write pools.
func NewSQLiteStore(path string, maxReadConns int) (*SQLiteStore, error) {
	if maxReadConns <= 0 {
		maxReadConns = runtime.NumCPU()
	}

	// Write connection: single connection, WAL mode
	writeDB, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_foreign_keys=ON")
	if err != nil {
		return nil, fmt.Errorf("open write db: %w", err)
	}
	writeDB.SetMaxOpenConns(1)
	writeDB.SetMaxIdleConns(1)

	// Read pool: multiple connections
	readDB, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_foreign_keys=ON&mode=ro")
	if err != nil {
		writeDB.Close()
		return nil, fmt.Errorf("open read db: %w", err)
	}
	readDB.SetMaxOpenConns(maxReadConns)
	readDB.SetMaxIdleConns(maxReadConns)

	// Run migrations on write connection
	if err := runMigrations(writeDB); err != nil {
		readDB.Close()
		writeDB.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return &SQLiteStore{readDB: readDB, writeDB: writeDB}, nil
}

func runMigrations(db *sql.DB) error {
	_, err := db.Exec(schema)
	if err != nil {
		return err
	}

	var currentVersion int
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM schema_version").Scan(&count)
	if err != nil {
		return err
	}
	if count == 0 {
		_, err = db.Exec("INSERT INTO schema_version (version) VALUES (?)", schemaVersion)
		return err
	}

	err = db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&currentVersion)
	if err != nil {
		return err
	}

	for _, m := range migrations {
		if m.version > currentVersion {
			if _, err := db.Exec(m.sql); err != nil {
				return fmt.Errorf("migration v%d: %w", m.version, err)
			}
			if _, err := db.Exec("UPDATE schema_version SET version=?", m.version); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *SQLiteStore) Close() error {
	s.readDB.Close()
	return s.writeDB.Close()
}

// timeFormat is the format used for storing timestamps in SQLite.
const timeFormat = "2006-01-02T15:04:05Z"

func formatTime(t time.Time) string {
	return t.UTC().Format(timeFormat)
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(timeFormat, s)
	return t
}

func parseTimePtr(s sql.NullString) *time.Time {
	if !s.Valid || s.String == "" {
		return nil
	}
	t := parseTime(s.String)
	return &t
}

// --- Monitors ---

func (s *SQLiteStore) CreateMonitor(ctx context.Context, m *Monitor) error {
	tags, _ := json.Marshal(m.Tags)
	if m.Settings == nil {
		m.Settings = json.RawMessage("{}")
	}
	if m.Assertions == nil {
		m.Assertions = json.RawMessage("[]")
	}
	now := formatTime(time.Now())
	res, err := s.writeDB.ExecContext(ctx,
		`INSERT INTO monitors (name, type, target, interval_secs, timeout_secs, enabled, tags, settings, assertions, track_changes, failure_threshold, success_threshold, public, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.Name, m.Type, m.Target, m.Interval, m.Timeout, boolToInt(m.Enabled),
		string(tags), string(m.Settings), string(m.Assertions), boolToInt(m.TrackChanges),
		m.FailureThreshold, m.SuccessThreshold, boolToInt(m.Public), now, now,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	m.ID = id
	m.CreatedAt = parseTime(now)
	m.UpdatedAt = parseTime(now)

	_, err = s.writeDB.ExecContext(ctx,
		`INSERT INTO monitor_status (monitor_id, status) VALUES (?, 'pending')`, id)
	return err
}

func (s *SQLiteStore) GetMonitor(ctx context.Context, id int64) (*Monitor, error) {
	row := s.readDB.QueryRowContext(ctx,
		`SELECT m.id, m.name, m.type, m.target, m.interval_secs, m.timeout_secs, m.enabled,
		        m.tags, m.settings, m.assertions, m.track_changes, m.failure_threshold, m.success_threshold,
		        m.public, m.created_at, m.updated_at,
		        COALESCE(ms.status, 'pending'), ms.last_check_at, COALESCE(ms.consec_fails, 0), COALESCE(ms.consec_successes, 0)
		 FROM monitors m
		 LEFT JOIN monitor_status ms ON ms.monitor_id = m.id
		 WHERE m.id = ?`, id)
	return scanMonitor(row)
}

func (s *SQLiteStore) ListMonitors(ctx context.Context, p Pagination) (*PaginatedResult, error) {
	var total int64
	err := s.readDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM monitors").Scan(&total)
	if err != nil {
		return nil, err
	}

	offset := (p.Page - 1) * p.PerPage
	rows, err := s.readDB.QueryContext(ctx,
		`SELECT m.id, m.name, m.type, m.target, m.interval_secs, m.timeout_secs, m.enabled,
		        m.tags, m.settings, m.assertions, m.track_changes, m.failure_threshold, m.success_threshold,
		        m.public, m.created_at, m.updated_at,
		        COALESCE(ms.status, 'pending'), ms.last_check_at, COALESCE(ms.consec_fails, 0), COALESCE(ms.consec_successes, 0)
		 FROM monitors m
		 LEFT JOIN monitor_status ms ON ms.monitor_id = m.id
		 ORDER BY m.id DESC
		 LIMIT ? OFFSET ?`, p.PerPage, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var monitors []*Monitor
	for rows.Next() {
		m, err := scanMonitor(rows)
		if err != nil {
			return nil, err
		}
		monitors = append(monitors, m)
	}
	if monitors == nil {
		monitors = []*Monitor{}
	}

	return &PaginatedResult{
		Data:       monitors,
		Total:      total,
		Page:       p.Page,
		PerPage:    p.PerPage,
		TotalPages: int(math.Ceil(float64(total) / float64(p.PerPage))),
	}, nil
}

func (s *SQLiteStore) UpdateMonitor(ctx context.Context, m *Monitor) error {
	tags, _ := json.Marshal(m.Tags)
	now := formatTime(time.Now())
	_, err := s.writeDB.ExecContext(ctx,
		`UPDATE monitors SET name=?, type=?, target=?, interval_secs=?, timeout_secs=?, enabled=?,
		 tags=?, settings=?, assertions=?, track_changes=?, failure_threshold=?, success_threshold=?, public=?, updated_at=?
		 WHERE id=?`,
		m.Name, m.Type, m.Target, m.Interval, m.Timeout, boolToInt(m.Enabled),
		string(tags), string(m.Settings), string(m.Assertions), boolToInt(m.TrackChanges),
		m.FailureThreshold, m.SuccessThreshold, boolToInt(m.Public), now, m.ID,
	)
	return err
}

func (s *SQLiteStore) DeleteMonitor(ctx context.Context, id int64) error {
	_, err := s.writeDB.ExecContext(ctx, "DELETE FROM monitors WHERE id=?", id)
	return err
}

func (s *SQLiteStore) SetMonitorEnabled(ctx context.Context, id int64, enabled bool) error {
	now := formatTime(time.Now())
	_, err := s.writeDB.ExecContext(ctx,
		"UPDATE monitors SET enabled=?, updated_at=? WHERE id=?",
		boolToInt(enabled), now, id)
	return err
}

func (s *SQLiteStore) GetAllEnabledMonitors(ctx context.Context) ([]*Monitor, error) {
	rows, err := s.readDB.QueryContext(ctx,
		`SELECT m.id, m.name, m.type, m.target, m.interval_secs, m.timeout_secs, m.enabled,
		        m.tags, m.settings, m.assertions, m.track_changes, m.failure_threshold, m.success_threshold,
		        m.public, m.created_at, m.updated_at,
		        COALESCE(ms.status, 'pending'), ms.last_check_at, COALESCE(ms.consec_fails, 0), COALESCE(ms.consec_successes, 0)
		 FROM monitors m
		 LEFT JOIN monitor_status ms ON ms.monitor_id = m.id
		 WHERE m.enabled = 1
		 ORDER BY m.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var monitors []*Monitor
	for rows.Next() {
		m, err := scanMonitor(rows)
		if err != nil {
			return nil, err
		}
		monitors = append(monitors, m)
	}
	return monitors, nil
}

// --- Monitor Status ---

func (s *SQLiteStore) GetMonitorStatus(ctx context.Context, monitorID int64) (*MonitorStatus, error) {
	var ms MonitorStatus
	var lastCheck sql.NullString
	err := s.readDB.QueryRowContext(ctx,
		`SELECT monitor_id, status, last_check_at, consec_fails, consec_successes, last_body_hash
		 FROM monitor_status WHERE monitor_id=?`, monitorID).
		Scan(&ms.MonitorID, &ms.Status, &lastCheck, &ms.ConsecFails, &ms.ConsecSuccesses, &ms.LastBodyHash)
	if err != nil {
		return nil, err
	}
	ms.LastCheckAt = parseTimePtr(lastCheck)
	return &ms, nil
}

func (s *SQLiteStore) UpsertMonitorStatus(ctx context.Context, st *MonitorStatus) error {
	var lastCheck string
	if st.LastCheckAt != nil {
		lastCheck = formatTime(*st.LastCheckAt)
	}
	_, err := s.writeDB.ExecContext(ctx,
		`INSERT INTO monitor_status (monitor_id, status, last_check_at, consec_fails, consec_successes, last_body_hash)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(monitor_id) DO UPDATE SET
		   status=excluded.status, last_check_at=excluded.last_check_at,
		   consec_fails=excluded.consec_fails, consec_successes=excluded.consec_successes,
		   last_body_hash=excluded.last_body_hash`,
		st.MonitorID, st.Status, nullStr(lastCheck), st.ConsecFails, st.ConsecSuccesses, st.LastBodyHash)
	return err
}

// --- Check Results ---

func (s *SQLiteStore) InsertCheckResult(ctx context.Context, r *CheckResult) error {
	var certExpiry string
	if r.CertExpiry != nil {
		certExpiry = formatTime(*r.CertExpiry)
	}
	now := formatTime(time.Now())
	res, err := s.writeDB.ExecContext(ctx,
		`INSERT INTO check_results (monitor_id, status, response_time, status_code, message, headers, body, body_hash, cert_expiry, dns_records, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.MonitorID, r.Status, r.ResponseTime, r.StatusCode, r.Message, r.Headers,
		r.Body, r.BodyHash, nullStr(certExpiry), r.DNSRecords, now)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	r.ID = id
	r.CreatedAt = parseTime(now)
	return nil
}

func (s *SQLiteStore) ListCheckResults(ctx context.Context, monitorID int64, p Pagination) (*PaginatedResult, error) {
	var total int64
	err := s.readDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM check_results WHERE monitor_id=?", monitorID).Scan(&total)
	if err != nil {
		return nil, err
	}

	offset := (p.Page - 1) * p.PerPage
	rows, err := s.readDB.QueryContext(ctx,
		`SELECT id, monitor_id, status, response_time, status_code, message, body_hash, cert_expiry, dns_records, created_at
		 FROM check_results WHERE monitor_id=? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		monitorID, p.PerPage, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*CheckResult
	for rows.Next() {
		var r CheckResult
		var certExp sql.NullString
		var createdAt string
		err := rows.Scan(&r.ID, &r.MonitorID, &r.Status, &r.ResponseTime, &r.StatusCode,
			&r.Message, &r.BodyHash, &certExp, &r.DNSRecords, &createdAt)
		if err != nil {
			return nil, err
		}
		r.CreatedAt = parseTime(createdAt)
		r.CertExpiry = parseTimePtr(certExp)
		results = append(results, &r)
	}
	if results == nil {
		results = []*CheckResult{}
	}

	return &PaginatedResult{
		Data:       results,
		Total:      total,
		Page:       p.Page,
		PerPage:    p.PerPage,
		TotalPages: int(math.Ceil(float64(total) / float64(p.PerPage))),
	}, nil
}

func (s *SQLiteStore) GetLatestCheckResult(ctx context.Context, monitorID int64) (*CheckResult, error) {
	var r CheckResult
	var certExp sql.NullString
	var createdAt string
	err := s.readDB.QueryRowContext(ctx,
		`SELECT id, monitor_id, status, response_time, status_code, message, body_hash, cert_expiry, dns_records, created_at
		 FROM check_results WHERE monitor_id=? ORDER BY created_at DESC LIMIT 1`, monitorID).
		Scan(&r.ID, &r.MonitorID, &r.Status, &r.ResponseTime, &r.StatusCode,
			&r.Message, &r.BodyHash, &certExp, &r.DNSRecords, &createdAt)
	if err != nil {
		return nil, err
	}
	r.CreatedAt = parseTime(createdAt)
	r.CertExpiry = parseTimePtr(certExp)
	return &r, nil
}

// --- Incidents ---

func (s *SQLiteStore) CreateIncident(ctx context.Context, inc *Incident) error {
	now := formatTime(time.Now())
	res, err := s.writeDB.ExecContext(ctx,
		`INSERT INTO incidents (monitor_id, status, cause, started_at) VALUES (?, ?, ?, ?)`,
		inc.MonitorID, inc.Status, inc.Cause, now)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	inc.ID = id
	inc.StartedAt = parseTime(now)
	return nil
}

func (s *SQLiteStore) GetIncident(ctx context.Context, id int64) (*Incident, error) {
	var inc Incident
	var startedAt string
	var ackAt, resAt sql.NullString
	err := s.readDB.QueryRowContext(ctx,
		`SELECT i.id, i.monitor_id, i.status, i.cause, i.started_at,
		        i.acknowledged_at, i.acknowledged_by, i.resolved_at, i.resolved_by,
		        COALESCE(m.name, '')
		 FROM incidents i
		 LEFT JOIN monitors m ON m.id = i.monitor_id
		 WHERE i.id=?`, id).
		Scan(&inc.ID, &inc.MonitorID, &inc.Status, &inc.Cause, &startedAt,
			&ackAt, &inc.AcknowledgedBy, &resAt, &inc.ResolvedBy, &inc.MonitorName)
	if err != nil {
		return nil, err
	}
	inc.StartedAt = parseTime(startedAt)
	inc.AcknowledgedAt = parseTimePtr(ackAt)
	inc.ResolvedAt = parseTimePtr(resAt)
	return &inc, nil
}

func (s *SQLiteStore) ListIncidents(ctx context.Context, monitorID int64, status string, p Pagination) (*PaginatedResult, error) {
	where := "1=1"
	args := []interface{}{}
	if monitorID > 0 {
		where += " AND i.monitor_id=?"
		args = append(args, monitorID)
	}
	if status != "" {
		where += " AND i.status=?"
		args = append(args, status)
	}

	var total int64
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	err := s.readDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM incidents i WHERE "+where, countArgs...).Scan(&total)
	if err != nil {
		return nil, err
	}

	offset := (p.Page - 1) * p.PerPage
	args = append(args, p.PerPage, offset)
	rows, err := s.readDB.QueryContext(ctx,
		`SELECT i.id, i.monitor_id, i.status, i.cause, i.started_at,
		        i.acknowledged_at, i.acknowledged_by, i.resolved_at, i.resolved_by,
		        COALESCE(m.name, '')
		 FROM incidents i
		 LEFT JOIN monitors m ON m.id = i.monitor_id
		 WHERE `+where+` ORDER BY i.started_at DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var incidents []*Incident
	for rows.Next() {
		var inc Incident
		var startedAt string
		var ackAt, resAt sql.NullString
		err := rows.Scan(&inc.ID, &inc.MonitorID, &inc.Status, &inc.Cause, &startedAt,
			&ackAt, &inc.AcknowledgedBy, &resAt, &inc.ResolvedBy, &inc.MonitorName)
		if err != nil {
			return nil, err
		}
		inc.StartedAt = parseTime(startedAt)
		inc.AcknowledgedAt = parseTimePtr(ackAt)
		inc.ResolvedAt = parseTimePtr(resAt)
		incidents = append(incidents, &inc)
	}
	if incidents == nil {
		incidents = []*Incident{}
	}

	return &PaginatedResult{
		Data:       incidents,
		Total:      total,
		Page:       p.Page,
		PerPage:    p.PerPage,
		TotalPages: int(math.Ceil(float64(total) / float64(p.PerPage))),
	}, nil
}

func (s *SQLiteStore) UpdateIncident(ctx context.Context, inc *Incident) error {
	var ackAt, resAt interface{}
	if inc.AcknowledgedAt != nil {
		ackAt = formatTime(*inc.AcknowledgedAt)
	}
	if inc.ResolvedAt != nil {
		resAt = formatTime(*inc.ResolvedAt)
	}
	_, err := s.writeDB.ExecContext(ctx,
		`UPDATE incidents SET status=?, cause=?, acknowledged_at=?, acknowledged_by=?, resolved_at=?, resolved_by=? WHERE id=?`,
		inc.Status, inc.Cause, ackAt, inc.AcknowledgedBy, resAt, inc.ResolvedBy, inc.ID)
	return err
}

func (s *SQLiteStore) GetOpenIncident(ctx context.Context, monitorID int64) (*Incident, error) {
	var inc Incident
	var startedAt string
	var ackAt, resAt sql.NullString
	err := s.readDB.QueryRowContext(ctx,
		`SELECT id, monitor_id, status, cause, started_at, acknowledged_at, acknowledged_by, resolved_at, resolved_by
		 FROM incidents WHERE monitor_id=? AND status IN ('open','acknowledged') ORDER BY started_at DESC LIMIT 1`,
		monitorID).
		Scan(&inc.ID, &inc.MonitorID, &inc.Status, &inc.Cause, &startedAt,
			&ackAt, &inc.AcknowledgedBy, &resAt, &inc.ResolvedBy)
	if err != nil {
		return nil, err
	}
	inc.StartedAt = parseTime(startedAt)
	inc.AcknowledgedAt = parseTimePtr(ackAt)
	inc.ResolvedAt = parseTimePtr(resAt)
	return &inc, nil
}

// --- Incident Events ---

func (s *SQLiteStore) InsertIncidentEvent(ctx context.Context, e *IncidentEvent) error {
	now := formatTime(time.Now())
	res, err := s.writeDB.ExecContext(ctx,
		`INSERT INTO incident_events (incident_id, type, message, created_at) VALUES (?, ?, ?, ?)`,
		e.IncidentID, e.Type, e.Message, now)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	e.ID = id
	e.CreatedAt = parseTime(now)
	return nil
}

func (s *SQLiteStore) ListIncidentEvents(ctx context.Context, incidentID int64) ([]*IncidentEvent, error) {
	rows, err := s.readDB.QueryContext(ctx,
		`SELECT id, incident_id, type, message, created_at
		 FROM incident_events WHERE incident_id=? ORDER BY created_at`, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*IncidentEvent
	for rows.Next() {
		var e IncidentEvent
		var createdAt string
		if err := rows.Scan(&e.ID, &e.IncidentID, &e.Type, &e.Message, &createdAt); err != nil {
			return nil, err
		}
		e.CreatedAt = parseTime(createdAt)
		events = append(events, &e)
	}
	if events == nil {
		events = []*IncidentEvent{}
	}
	return events, nil
}

// --- Notification Channels ---

func (s *SQLiteStore) CreateNotificationChannel(ctx context.Context, ch *NotificationChannel) error {
	events, _ := json.Marshal(ch.Events)
	now := formatTime(time.Now())
	res, err := s.writeDB.ExecContext(ctx,
		`INSERT INTO notification_channels (name, type, enabled, settings, events, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		ch.Name, ch.Type, boolToInt(ch.Enabled), string(ch.Settings), string(events), now, now)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	ch.ID = id
	ch.CreatedAt = parseTime(now)
	ch.UpdatedAt = parseTime(now)
	return nil
}

func (s *SQLiteStore) GetNotificationChannel(ctx context.Context, id int64) (*NotificationChannel, error) {
	var ch NotificationChannel
	var eventsStr, createdAt, updatedAt string
	err := s.readDB.QueryRowContext(ctx,
		`SELECT id, name, type, enabled, settings, events, created_at, updated_at
		 FROM notification_channels WHERE id=?`, id).
		Scan(&ch.ID, &ch.Name, &ch.Type, &ch.Enabled, &ch.Settings, &eventsStr, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	ch.CreatedAt = parseTime(createdAt)
	ch.UpdatedAt = parseTime(updatedAt)
	json.Unmarshal([]byte(eventsStr), &ch.Events)
	return &ch, nil
}

func (s *SQLiteStore) ListNotificationChannels(ctx context.Context) ([]*NotificationChannel, error) {
	rows, err := s.readDB.QueryContext(ctx,
		`SELECT id, name, type, enabled, settings, events, created_at, updated_at
		 FROM notification_channels ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []*NotificationChannel
	for rows.Next() {
		var ch NotificationChannel
		var eventsStr, createdAt, updatedAt string
		if err := rows.Scan(&ch.ID, &ch.Name, &ch.Type, &ch.Enabled, &ch.Settings, &eventsStr, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		ch.CreatedAt = parseTime(createdAt)
		ch.UpdatedAt = parseTime(updatedAt)
		json.Unmarshal([]byte(eventsStr), &ch.Events)
		channels = append(channels, &ch)
	}
	if channels == nil {
		channels = []*NotificationChannel{}
	}
	return channels, nil
}

func (s *SQLiteStore) UpdateNotificationChannel(ctx context.Context, ch *NotificationChannel) error {
	events, _ := json.Marshal(ch.Events)
	now := formatTime(time.Now())
	_, err := s.writeDB.ExecContext(ctx,
		`UPDATE notification_channels SET name=?, type=?, enabled=?, settings=?, events=?, updated_at=? WHERE id=?`,
		ch.Name, ch.Type, boolToInt(ch.Enabled), string(ch.Settings), string(events), now, ch.ID)
	return err
}

func (s *SQLiteStore) DeleteNotificationChannel(ctx context.Context, id int64) error {
	_, err := s.writeDB.ExecContext(ctx, "DELETE FROM notification_channels WHERE id=?", id)
	return err
}

// --- Maintenance Windows ---

func (s *SQLiteStore) CreateMaintenanceWindow(ctx context.Context, mw *MaintenanceWindow) error {
	monitorIDs, _ := json.Marshal(mw.MonitorIDs)
	now := formatTime(time.Now())
	res, err := s.writeDB.ExecContext(ctx,
		`INSERT INTO maintenance_windows (name, monitor_ids, start_time, end_time, recurring, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		mw.Name, string(monitorIDs), formatTime(mw.StartTime), formatTime(mw.EndTime), mw.Recurring, now, now)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	mw.ID = id
	mw.CreatedAt = parseTime(now)
	mw.UpdatedAt = parseTime(now)
	return nil
}

func (s *SQLiteStore) GetMaintenanceWindow(ctx context.Context, id int64) (*MaintenanceWindow, error) {
	var mw MaintenanceWindow
	var monitorIDsStr, startTime, endTime, createdAt, updatedAt string
	err := s.readDB.QueryRowContext(ctx,
		`SELECT id, name, monitor_ids, start_time, end_time, recurring, created_at, updated_at
		 FROM maintenance_windows WHERE id=?`, id).
		Scan(&mw.ID, &mw.Name, &monitorIDsStr, &startTime, &endTime, &mw.Recurring, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	mw.StartTime = parseTime(startTime)
	mw.EndTime = parseTime(endTime)
	mw.CreatedAt = parseTime(createdAt)
	mw.UpdatedAt = parseTime(updatedAt)
	json.Unmarshal([]byte(monitorIDsStr), &mw.MonitorIDs)
	return &mw, nil
}

func (s *SQLiteStore) ListMaintenanceWindows(ctx context.Context) ([]*MaintenanceWindow, error) {
	rows, err := s.readDB.QueryContext(ctx,
		`SELECT id, name, monitor_ids, start_time, end_time, recurring, created_at, updated_at
		 FROM maintenance_windows ORDER BY start_time DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var windows []*MaintenanceWindow
	for rows.Next() {
		var mw MaintenanceWindow
		var monitorIDsStr, startTime, endTime, createdAt, updatedAt string
		if err := rows.Scan(&mw.ID, &mw.Name, &monitorIDsStr, &startTime, &endTime, &mw.Recurring, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		mw.StartTime = parseTime(startTime)
		mw.EndTime = parseTime(endTime)
		mw.CreatedAt = parseTime(createdAt)
		mw.UpdatedAt = parseTime(updatedAt)
		json.Unmarshal([]byte(monitorIDsStr), &mw.MonitorIDs)
		windows = append(windows, &mw)
	}
	if windows == nil {
		windows = []*MaintenanceWindow{}
	}
	return windows, nil
}

func (s *SQLiteStore) UpdateMaintenanceWindow(ctx context.Context, mw *MaintenanceWindow) error {
	monitorIDs, _ := json.Marshal(mw.MonitorIDs)
	now := formatTime(time.Now())
	_, err := s.writeDB.ExecContext(ctx,
		`UPDATE maintenance_windows SET name=?, monitor_ids=?, start_time=?, end_time=?, recurring=?, updated_at=? WHERE id=?`,
		mw.Name, string(monitorIDs), formatTime(mw.StartTime), formatTime(mw.EndTime), mw.Recurring, now, mw.ID)
	return err
}

func (s *SQLiteStore) DeleteMaintenanceWindow(ctx context.Context, id int64) error {
	_, err := s.writeDB.ExecContext(ctx, "DELETE FROM maintenance_windows WHERE id=?", id)
	return err
}

func (s *SQLiteStore) IsMonitorInMaintenance(ctx context.Context, monitorID int64, at time.Time) (bool, error) {
	windows, err := s.ListMaintenanceWindows(ctx)
	if err != nil {
		return false, err
	}

	for _, mw := range windows {
		// Check if monitor is covered
		if len(mw.MonitorIDs) > 0 {
			found := false
			for _, id := range mw.MonitorIDs {
				if id == monitorID {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		if isInWindow(mw, at) {
			return true, nil
		}
	}
	return false, nil
}

func isInWindow(mw *MaintenanceWindow, at time.Time) bool {
	if mw.Recurring == "" {
		return !at.Before(mw.StartTime) && at.Before(mw.EndTime)
	}

	duration := mw.EndTime.Sub(mw.StartTime)
	switch mw.Recurring {
	case "daily":
		// Check if current time-of-day falls within the window
		startSec := mw.StartTime.Hour()*3600 + mw.StartTime.Minute()*60 + mw.StartTime.Second()
		atSec := at.Hour()*3600 + at.Minute()*60 + at.Second()
		endSec := startSec + int(duration.Seconds())
		if endSec > 86400 {
			return atSec >= startSec || atSec < (endSec-86400)
		}
		return atSec >= startSec && atSec < endSec
	case "weekly":
		startDay := mw.StartTime.Weekday()
		atDay := at.Weekday()
		if startDay == atDay {
			startSec := mw.StartTime.Hour()*3600 + mw.StartTime.Minute()*60 + mw.StartTime.Second()
			atSec := at.Hour()*3600 + at.Minute()*60 + at.Second()
			return atSec >= startSec && atSec < startSec+int(duration.Seconds())
		}
	case "monthly":
		if mw.StartTime.Day() == at.Day() {
			startSec := mw.StartTime.Hour()*3600 + mw.StartTime.Minute()*60 + mw.StartTime.Second()
			atSec := at.Hour()*3600 + at.Minute()*60 + at.Second()
			return atSec >= startSec && atSec < startSec+int(duration.Seconds())
		}
	}
	return false
}

// --- Content Changes ---

func (s *SQLiteStore) InsertContentChange(ctx context.Context, c *ContentChange) error {
	now := formatTime(time.Now())
	res, err := s.writeDB.ExecContext(ctx,
		`INSERT INTO content_changes (monitor_id, old_hash, new_hash, diff, old_body, new_body, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.MonitorID, c.OldHash, c.NewHash, c.Diff, c.OldBody, c.NewBody, now)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	c.ID = id
	c.CreatedAt = parseTime(now)
	return nil
}

func (s *SQLiteStore) ListContentChanges(ctx context.Context, monitorID int64, p Pagination) (*PaginatedResult, error) {
	var total int64
	err := s.readDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM content_changes WHERE monitor_id=?", monitorID).Scan(&total)
	if err != nil {
		return nil, err
	}

	offset := (p.Page - 1) * p.PerPage
	rows, err := s.readDB.QueryContext(ctx,
		`SELECT id, monitor_id, old_hash, new_hash, diff, created_at
		 FROM content_changes WHERE monitor_id=? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		monitorID, p.PerPage, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var changes []*ContentChange
	for rows.Next() {
		var c ContentChange
		var createdAt string
		if err := rows.Scan(&c.ID, &c.MonitorID, &c.OldHash, &c.NewHash, &c.Diff, &createdAt); err != nil {
			return nil, err
		}
		c.CreatedAt = parseTime(createdAt)
		changes = append(changes, &c)
	}
	if changes == nil {
		changes = []*ContentChange{}
	}

	return &PaginatedResult{
		Data:       changes,
		Total:      total,
		Page:       p.Page,
		PerPage:    p.PerPage,
		TotalPages: int(math.Ceil(float64(total) / float64(p.PerPage))),
	}, nil
}

// --- Analytics ---

func (s *SQLiteStore) GetUptimePercent(ctx context.Context, monitorID int64, from, to time.Time) (float64, error) {
	var total, up int64
	err := s.readDB.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(CASE WHEN status='up' THEN 1 ELSE 0 END), 0)
		 FROM check_results WHERE monitor_id=? AND created_at >= ? AND created_at < ?`,
		monitorID, formatTime(from), formatTime(to)).Scan(&total, &up)
	if err != nil {
		return 0, err
	}
	if total == 0 {
		return 100, nil
	}
	return float64(up) / float64(total) * 100, nil
}

func (s *SQLiteStore) GetResponseTimePercentiles(ctx context.Context, monitorID int64, from, to time.Time) (p50, p95, p99 float64, err error) {
	rows, err := s.readDB.QueryContext(ctx,
		`SELECT response_time FROM check_results
		 WHERE monitor_id=? AND created_at >= ? AND created_at < ? AND status='up'
		 ORDER BY response_time`,
		monitorID, formatTime(from), formatTime(to))
	if err != nil {
		return 0, 0, 0, err
	}
	defer rows.Close()

	var times []float64
	for rows.Next() {
		var rt float64
		if err := rows.Scan(&rt); err != nil {
			return 0, 0, 0, err
		}
		times = append(times, rt)
	}

	if len(times) == 0 {
		return 0, 0, 0, nil
	}

	sort.Float64s(times)
	p50 = percentile(times, 0.50)
	p95 = percentile(times, 0.95)
	p99 = percentile(times, 0.99)
	return p50, p95, p99, nil
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := p * float64(len(sorted)-1)
	lower := int(math.Floor(idx))
	upper := int(math.Ceil(idx))
	if lower == upper {
		return sorted[lower]
	}
	frac := idx - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}

func (s *SQLiteStore) GetCheckCounts(ctx context.Context, monitorID int64, from, to time.Time) (total, up, down, degraded int64, err error) {
	err = s.readDB.QueryRowContext(ctx,
		`SELECT COUNT(*),
		        COALESCE(SUM(CASE WHEN status='up' THEN 1 ELSE 0 END), 0),
		        COALESCE(SUM(CASE WHEN status='down' THEN 1 ELSE 0 END), 0),
		        COALESCE(SUM(CASE WHEN status='degraded' THEN 1 ELSE 0 END), 0)
		 FROM check_results WHERE monitor_id=? AND created_at >= ? AND created_at < ?`,
		monitorID, formatTime(from), formatTime(to)).Scan(&total, &up, &down, &degraded)
	return
}

func (s *SQLiteStore) CountMonitorsByStatus(ctx context.Context) (up, down, degraded, paused int64, err error) {
	err = s.readDB.QueryRowContext(ctx,
		`SELECT
		   COALESCE(SUM(CASE WHEN m.enabled=1 AND ms.status='up' THEN 1 ELSE 0 END), 0),
		   COALESCE(SUM(CASE WHEN m.enabled=1 AND ms.status='down' THEN 1 ELSE 0 END), 0),
		   COALESCE(SUM(CASE WHEN m.enabled=1 AND ms.status='degraded' THEN 1 ELSE 0 END), 0),
		   COALESCE(SUM(CASE WHEN m.enabled=0 THEN 1 ELSE 0 END), 0)
		 FROM monitors m
		 LEFT JOIN monitor_status ms ON ms.monitor_id = m.id`).
		Scan(&up, &down, &degraded, &paused)
	return
}

// --- Tags ---

func (s *SQLiteStore) ListTags(ctx context.Context) ([]string, error) {
	rows, err := s.readDB.QueryContext(ctx, "SELECT tags FROM monitors")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tagSet := map[string]struct{}{}
	for rows.Next() {
		var tagsStr string
		if err := rows.Scan(&tagsStr); err != nil {
			return nil, err
		}
		var tags []string
		json.Unmarshal([]byte(tagsStr), &tags)
		for _, t := range tags {
			tagSet[t] = struct{}{}
		}
	}

	result := make([]string, 0, len(tagSet))
	for t := range tagSet {
		result = append(result, t)
	}
	sort.Strings(result)
	return result, nil
}

// --- Audit ---

func (s *SQLiteStore) InsertAudit(ctx context.Context, entry *AuditEntry) error {
	now := formatTime(time.Now())
	_, err := s.writeDB.ExecContext(ctx,
		`INSERT INTO audit_log (action, entity, entity_id, api_key_name, detail, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		entry.Action, entry.Entity, entry.EntityID, entry.APIKeyName, entry.Detail, now)
	return err
}

// --- Retention ---

func (s *SQLiteStore) PurgeOldData(ctx context.Context, before time.Time) (int64, error) {
	ts := formatTime(before)
	var totalDeleted int64

	res, err := s.writeDB.ExecContext(ctx, "DELETE FROM check_results WHERE created_at < ?", ts)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	totalDeleted += n

	res, err = s.writeDB.ExecContext(ctx, "DELETE FROM incident_events WHERE created_at < ?", ts)
	if err != nil {
		return totalDeleted, err
	}
	n, _ = res.RowsAffected()
	totalDeleted += n

	res, err = s.writeDB.ExecContext(ctx, "DELETE FROM incidents WHERE status='resolved' AND resolved_at < ?", ts)
	if err != nil {
		return totalDeleted, err
	}
	n, _ = res.RowsAffected()
	totalDeleted += n

	res, err = s.writeDB.ExecContext(ctx, "DELETE FROM content_changes WHERE created_at < ?", ts)
	if err != nil {
		return totalDeleted, err
	}
	n, _ = res.RowsAffected()
	totalDeleted += n

	res, err = s.writeDB.ExecContext(ctx, "DELETE FROM audit_log WHERE created_at < ?", ts)
	if err != nil {
		return totalDeleted, err
	}
	n, _ = res.RowsAffected()
	totalDeleted += n

	return totalDeleted, nil
}

// --- Heartbeats ---

func (s *SQLiteStore) CreateHeartbeat(ctx context.Context, h *Heartbeat) error {
	res, err := s.writeDB.ExecContext(ctx,
		`INSERT INTO heartbeats (monitor_id, token, grace, status) VALUES (?, ?, ?, ?)`,
		h.MonitorID, h.Token, h.Grace, h.Status)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	h.ID = id
	return nil
}

func (s *SQLiteStore) GetHeartbeatByToken(ctx context.Context, token string) (*Heartbeat, error) {
	var h Heartbeat
	var lastPing sql.NullString
	err := s.readDB.QueryRowContext(ctx,
		`SELECT id, monitor_id, token, grace, last_ping_at, status FROM heartbeats WHERE token=?`, token).
		Scan(&h.ID, &h.MonitorID, &h.Token, &h.Grace, &lastPing, &h.Status)
	if err != nil {
		return nil, err
	}
	h.LastPingAt = parseTimePtr(lastPing)
	return &h, nil
}

func (s *SQLiteStore) GetHeartbeatByMonitorID(ctx context.Context, monitorID int64) (*Heartbeat, error) {
	var h Heartbeat
	var lastPing sql.NullString
	err := s.readDB.QueryRowContext(ctx,
		`SELECT id, monitor_id, token, grace, last_ping_at, status FROM heartbeats WHERE monitor_id=?`, monitorID).
		Scan(&h.ID, &h.MonitorID, &h.Token, &h.Grace, &lastPing, &h.Status)
	if err != nil {
		return nil, err
	}
	h.LastPingAt = parseTimePtr(lastPing)
	return &h, nil
}

func (s *SQLiteStore) UpdateHeartbeatPing(ctx context.Context, token string) error {
	now := formatTime(time.Now())
	_, err := s.writeDB.ExecContext(ctx,
		`UPDATE heartbeats SET last_ping_at=?, status='up' WHERE token=?`, now, token)
	return err
}

func (s *SQLiteStore) ListExpiredHeartbeats(ctx context.Context) ([]*Heartbeat, error) {
	rows, err := s.readDB.QueryContext(ctx,
		`SELECT h.id, h.monitor_id, h.token, h.grace, h.last_ping_at, h.status
		 FROM heartbeats h
		 JOIN monitors m ON m.id = h.monitor_id
		 WHERE m.enabled = 1
		   AND h.last_ping_at IS NOT NULL
		   AND datetime(h.last_ping_at, '+' || (m.interval_secs + h.grace) || ' seconds') < datetime('now')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var heartbeats []*Heartbeat
	for rows.Next() {
		var h Heartbeat
		var lastPing sql.NullString
		if err := rows.Scan(&h.ID, &h.MonitorID, &h.Token, &h.Grace, &lastPing, &h.Status); err != nil {
			return nil, err
		}
		h.LastPingAt = parseTimePtr(lastPing)
		heartbeats = append(heartbeats, &h)
	}
	return heartbeats, nil
}

func (s *SQLiteStore) UpdateHeartbeatStatus(ctx context.Context, monitorID int64, status string) error {
	_, err := s.writeDB.ExecContext(ctx,
		`UPDATE heartbeats SET status=? WHERE monitor_id=?`, status, monitorID)
	return err
}

func (s *SQLiteStore) DeleteHeartbeat(ctx context.Context, monitorID int64) error {
	_, err := s.writeDB.ExecContext(ctx, "DELETE FROM heartbeats WHERE monitor_id=?", monitorID)
	return err
}

// --- Helpers ---

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanMonitor(row scanner) (*Monitor, error) {
	var m Monitor
	var tagsStr, settingsStr, assertionsStr string
	var createdAt, updatedAt string
	var lastCheck sql.NullString
	err := row.Scan(&m.ID, &m.Name, &m.Type, &m.Target, &m.Interval, &m.Timeout, &m.Enabled,
		&tagsStr, &settingsStr, &assertionsStr, &m.TrackChanges, &m.FailureThreshold, &m.SuccessThreshold,
		&m.Public, &createdAt, &updatedAt,
		&m.Status, &lastCheck, &m.ConsecFails, &m.ConsecSuccesses)
	if err != nil {
		return nil, err
	}
	m.CreatedAt = parseTime(createdAt)
	m.UpdatedAt = parseTime(updatedAt)
	json.Unmarshal([]byte(tagsStr), &m.Tags)
	m.Settings = json.RawMessage(settingsStr)
	m.Assertions = json.RawMessage(assertionsStr)
	m.LastCheckAt = parseTimePtr(lastCheck)
	if m.Tags == nil {
		m.Tags = []string{}
	}
	if !m.Enabled {
		m.Status = "paused"
	}
	if strings.TrimSpace(settingsStr) == "" {
		m.Settings = json.RawMessage("{}")
	}
	if strings.TrimSpace(assertionsStr) == "" {
		m.Assertions = json.RawMessage("[]")
	}
	return &m, nil
}
