package storage

import (
	"encoding/json"
	"time"
)

// Monitor represents a monitored endpoint.
type Monitor struct {
	ID               int64           `json:"id"`
	Name             string          `json:"name"`
	Type             string          `json:"type"` // http, tcp, dns, icmp, tls, websocket, command
	Target           string          `json:"target"`
	Interval         int             `json:"interval"` // seconds
	Timeout          int             `json:"timeout"`  // seconds
	Enabled          bool            `json:"enabled"`
	Tags             []string        `json:"tags"`
	Settings         json.RawMessage `json:"settings,omitempty"`
	Assertions       json.RawMessage `json:"assertions,omitempty"`
	TrackChanges     bool            `json:"track_changes"`
	FailureThreshold int             `json:"failure_threshold"`
	SuccessThreshold int             `json:"success_threshold"`
	Public           bool            `json:"public"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`

	// Computed fields (not stored directly)
	Status          string     `json:"status,omitempty"`
	LastCheckAt     *time.Time `json:"last_check_at,omitempty"`
	ConsecFails     int        `json:"consec_fails,omitempty"`
	ConsecSuccesses int        `json:"consec_successes,omitempty"`
}

// HTTPSettings holds configuration specific to HTTP checks.
type HTTPSettings struct {
	Method          string            `json:"method,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	Body            string            `json:"body,omitempty"`
	FollowRedirects *bool             `json:"follow_redirects,omitempty"`
	SkipTLSVerify   bool              `json:"skip_tls_verify,omitempty"`
	BasicAuthUser   string            `json:"basic_auth_user,omitempty"`
	BasicAuthPass   string            `json:"basic_auth_pass,omitempty"`
	ExpectedStatus  int               `json:"expected_status,omitempty"`
}

// TCPSettings holds TCP check configuration.
type TCPSettings struct {
	SendData   string `json:"send_data,omitempty"`
	ExpectData string `json:"expect_data,omitempty"`
}

// DNSSettings holds DNS check configuration.
type DNSSettings struct {
	RecordType string `json:"record_type"` // A, AAAA, CNAME, MX, TXT, NS, SOA
	Server     string `json:"server,omitempty"`
}

// TLSSettings holds TLS check configuration.
type TLSSettings struct {
	WarnDaysBefore int `json:"warn_days_before,omitempty"` // cert expiry warning threshold
}

// WebSocketSettings holds WebSocket check configuration.
type WebSocketSettings struct {
	Headers     map[string]string `json:"headers,omitempty"`
	SendMessage string            `json:"send_message,omitempty"`
	ExpectReply string            `json:"expect_reply,omitempty"`
}

// CommandSettings holds command check configuration.
type CommandSettings struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
}

// CheckResult stores the outcome of a single check execution.
type CheckResult struct {
	ID           int64      `json:"id"`
	MonitorID    int64      `json:"monitor_id"`
	Status       string     `json:"status"`        // up, down, degraded
	ResponseTime int64      `json:"response_time"` // milliseconds
	StatusCode   int        `json:"status_code,omitempty"`
	Message      string     `json:"message,omitempty"`
	Headers      string     `json:"headers,omitempty"` // JSON encoded
	Body         string     `json:"body,omitempty"`
	BodyHash     string     `json:"body_hash,omitempty"`
	CertExpiry   *time.Time `json:"cert_expiry,omitempty"`
	DNSRecords   string     `json:"dns_records,omitempty"` // JSON encoded
	CreatedAt    time.Time  `json:"created_at"`
}

// Incident tracks a period of downtime or degradation.
type Incident struct {
	ID             int64      `json:"id"`
	MonitorID      int64      `json:"monitor_id"`
	MonitorName    string     `json:"monitor_name,omitempty"`
	Status         string     `json:"status"` // open, acknowledged, resolved
	Cause          string     `json:"cause"`
	StartedAt      time.Time  `json:"started_at"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
	AcknowledgedBy string     `json:"acknowledged_by,omitempty"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
	ResolvedBy     string     `json:"resolved_by,omitempty"`
}

// IncidentEvent is a timeline entry within an incident.
type IncidentEvent struct {
	ID         int64     `json:"id"`
	IncidentID int64     `json:"incident_id"`
	Type       string    `json:"type"` // created, acknowledged, resolved, check_failed, check_recovered
	Message    string    `json:"message"`
	CreatedAt  time.Time `json:"created_at"`
}

// NotificationChannel configures how alerts are delivered.
type NotificationChannel struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	Type      string          `json:"type"` // webhook, email, telegram, discord, slack
	Enabled   bool            `json:"enabled"`
	Settings  json.RawMessage `json:"settings"`
	Events    []string        `json:"events"` // incident.created, incident.resolved, etc.
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// MaintenanceWindow defines a period where alerts are suppressed.
type MaintenanceWindow struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	MonitorIDs []int64   `json:"monitor_ids"` // empty = all monitors
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	Recurring  string    `json:"recurring,omitempty"` // "", "daily", "weekly", "monthly"
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ContentChange records when a monitored page's content changes.
type ContentChange struct {
	ID        int64     `json:"id"`
	MonitorID int64     `json:"monitor_id"`
	OldHash   string    `json:"old_hash"`
	NewHash   string    `json:"new_hash"`
	Diff      string    `json:"diff"`
	OldBody   string    `json:"-"` // not exposed in API
	NewBody   string    `json:"-"` // not exposed in API
	CreatedAt time.Time `json:"created_at"`
}

// MonitorStatus holds the runtime state of a monitor.
type MonitorStatus struct {
	MonitorID       int64      `json:"monitor_id"`
	Status          string     `json:"status"` // up, down, degraded, pending
	LastCheckAt     *time.Time `json:"last_check_at,omitempty"`
	ConsecFails     int        `json:"consec_fails"`
	ConsecSuccesses int        `json:"consec_successes"`
	LastBodyHash    string     `json:"-"`
}

// Pagination contains parameters for list queries.
type Pagination struct {
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
}

// PaginatedResult wraps a list response with metadata.
type PaginatedResult struct {
	Data       interface{} `json:"data"`
	Total      int64       `json:"total"`
	Page       int         `json:"page"`
	PerPage    int         `json:"per_page"`
	TotalPages int         `json:"total_pages"`
}

// Heartbeat tracks a heartbeat monitor's ping state.
type Heartbeat struct {
	ID         int64      `json:"id"`
	MonitorID  int64      `json:"monitor_id"`
	Token      string     `json:"token"`
	Grace      int        `json:"grace"` // grace period in seconds
	LastPingAt *time.Time `json:"last_ping_at,omitempty"`
	Status     string     `json:"status"` // up, down, pending
}

// AuditEntry logs a mutation in the system.
type AuditEntry struct {
	ID         int64     `json:"id"`
	Action     string    `json:"action"`
	Entity     string    `json:"entity"`
	EntityID   int64     `json:"entity_id"`
	APIKeyName string    `json:"api_key_name"`
	Detail     string    `json:"detail,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// RequestLog records a single HTTP request to the Asura server.
type RequestLog struct {
	ID         int64     `json:"id"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	StatusCode int       `json:"status_code"`
	LatencyMs  int64     `json:"latency_ms"`
	ClientIP   string    `json:"client_ip"`
	UserAgent  string    `json:"user_agent"`
	Referer    string    `json:"referer"`
	MonitorID  *int64    `json:"monitor_id,omitempty"`
	RouteGroup string    `json:"route_group"`
	CreatedAt  time.Time `json:"created_at"`
}

// RequestLogStats holds aggregate request statistics.
type RequestLogStats struct {
	TotalRequests  int64       `json:"total_requests"`
	UniqueVisitors int64       `json:"unique_visitors"`
	AvgLatencyMs   int64       `json:"avg_latency_ms"`
	TopPaths       []PathCount `json:"top_paths"`
	TopReferers    []PathCount `json:"top_referers"`
}

// PathCount pairs a path or referer with its request count.
type PathCount struct {
	Path  string `json:"path"`
	Count int64  `json:"count"`
}

// RequestLogFilter holds filter parameters for listing request logs.
type RequestLogFilter struct {
	Method     string
	Path       string
	StatusCode int
	RouteGroup string
	MonitorID  *int64
	From       time.Time
	To         time.Time
}

// TimeSeriesPoint is a single data point for response time charts.
type TimeSeriesPoint struct {
	Timestamp    int64  `json:"ts"`
	ResponseTime int64  `json:"rt"`
	Status       string `json:"s"`
}

// StatusPageConfig holds the configuration for the public status page.
type StatusPageConfig struct {
	Enabled       bool      `json:"enabled"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	ShowIncidents bool      `json:"show_incidents"`
	CustomCSS     string    `json:"custom_css"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// DailyUptime holds uptime statistics for a single day.
type DailyUptime struct {
	Date        string  `json:"date"`
	TotalChecks int64   `json:"total_checks"`
	UpChecks    int64   `json:"up_checks"`
	DownChecks  int64   `json:"down_checks"`
	UptimePct   float64 `json:"uptime_pct"`
}

// Session represents a server-side web UI session.
type Session struct {
	ID         int64     `json:"id"`
	TokenHash  string    `json:"-"`
	APIKeyName string    `json:"api_key_name"`
	KeyHash    string    `json:"-"`
	IPAddress  string    `json:"ip_address"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}
