package storage

const schemaVersion = 10

const schema = `
CREATE TABLE IF NOT EXISTS schema_version (
	version INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS monitors (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	name            TEXT    NOT NULL,
	description     TEXT    NOT NULL DEFAULT '',
	type            TEXT    NOT NULL,
	target          TEXT    NOT NULL,
	interval_secs   INTEGER NOT NULL DEFAULT 60,
	timeout_secs    INTEGER NOT NULL DEFAULT 10,
	enabled         INTEGER NOT NULL DEFAULT 1,
	tags            TEXT    NOT NULL DEFAULT '[]',
	settings        TEXT    NOT NULL DEFAULT '{}',
	assertions      TEXT    NOT NULL DEFAULT '[]',
	track_changes   INTEGER NOT NULL DEFAULT 0,
	failure_threshold INTEGER NOT NULL DEFAULT 3,
	success_threshold INTEGER NOT NULL DEFAULT 1,
	public          INTEGER NOT NULL DEFAULT 0,
	upside_down     INTEGER NOT NULL DEFAULT 0,
	resend_interval INTEGER NOT NULL DEFAULT 0,
	created_at      TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
	updated_at      TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE IF NOT EXISTS monitor_status (
	monitor_id       INTEGER PRIMARY KEY REFERENCES monitors(id) ON DELETE CASCADE,
	status           TEXT    NOT NULL DEFAULT 'pending',
	last_check_at    TEXT,
	consec_fails     INTEGER NOT NULL DEFAULT 0,
	consec_successes INTEGER NOT NULL DEFAULT 0,
	last_body_hash   TEXT    NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS check_results (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	monitor_id    INTEGER NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
	status        TEXT    NOT NULL,
	response_time INTEGER NOT NULL DEFAULT 0,
	status_code   INTEGER NOT NULL DEFAULT 0,
	message       TEXT    NOT NULL DEFAULT '',
	headers       TEXT    NOT NULL DEFAULT '',
	body          TEXT    NOT NULL DEFAULT '',
	body_hash     TEXT    NOT NULL DEFAULT '',
	cert_expiry   TEXT,
	dns_records   TEXT    NOT NULL DEFAULT '',
	created_at    TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_check_results_monitor_id ON check_results(monitor_id, created_at DESC);

CREATE TABLE IF NOT EXISTS incidents (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	monitor_id      INTEGER NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
	status          TEXT    NOT NULL DEFAULT 'open',
	cause           TEXT    NOT NULL DEFAULT '',
	started_at      TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
	acknowledged_at TEXT,
	acknowledged_by TEXT    NOT NULL DEFAULT '',
	resolved_at     TEXT,
	resolved_by     TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_incidents_monitor_id ON incidents(monitor_id, status);

CREATE TABLE IF NOT EXISTS incident_events (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	incident_id INTEGER NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
	type        TEXT    NOT NULL,
	message     TEXT    NOT NULL DEFAULT '',
	created_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_incident_events_incident_id ON incident_events(incident_id);

CREATE TABLE IF NOT EXISTS notification_channels (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	name       TEXT    NOT NULL,
	type       TEXT    NOT NULL,
	enabled    INTEGER NOT NULL DEFAULT 1,
	settings   TEXT    NOT NULL DEFAULT '{}',
	events     TEXT    NOT NULL DEFAULT '[]',
	created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
	updated_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE IF NOT EXISTS maintenance_windows (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	name        TEXT    NOT NULL,
	monitor_ids TEXT    NOT NULL DEFAULT '[]',
	start_time  TEXT    NOT NULL,
	end_time    TEXT    NOT NULL,
	recurring   TEXT    NOT NULL DEFAULT '',
	created_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
	updated_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE IF NOT EXISTS content_changes (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	monitor_id INTEGER NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
	old_hash   TEXT    NOT NULL DEFAULT '',
	new_hash   TEXT    NOT NULL,
	diff       TEXT    NOT NULL DEFAULT '',
	old_body   TEXT    NOT NULL DEFAULT '',
	new_body   TEXT    NOT NULL DEFAULT '',
	created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_content_changes_monitor_id ON content_changes(monitor_id, created_at DESC);

CREATE TABLE IF NOT EXISTS audit_log (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	action       TEXT    NOT NULL,
	entity       TEXT    NOT NULL,
	entity_id    INTEGER NOT NULL DEFAULT 0,
	api_key_name TEXT    NOT NULL DEFAULT '',
	detail       TEXT    NOT NULL DEFAULT '',
	created_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE TABLE IF NOT EXISTS heartbeats (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	monitor_id  INTEGER NOT NULL UNIQUE REFERENCES monitors(id) ON DELETE CASCADE,
	token       TEXT    NOT NULL UNIQUE,
	grace       INTEGER NOT NULL DEFAULT 0,
	last_ping_at TEXT,
	status      TEXT    NOT NULL DEFAULT 'pending'
);

CREATE TABLE IF NOT EXISTS sessions (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	token_hash   TEXT    NOT NULL UNIQUE,
	api_key_name TEXT    NOT NULL,
	key_hash     TEXT    NOT NULL DEFAULT '',
	ip_address   TEXT    NOT NULL DEFAULT '',
	created_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
	expires_at   TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);

CREATE TABLE IF NOT EXISTS request_logs (
	id             INTEGER PRIMARY KEY AUTOINCREMENT,
	method         TEXT    NOT NULL,
	path           TEXT    NOT NULL,
	status_code    INTEGER NOT NULL,
	latency_ms     INTEGER NOT NULL,
	client_ip      TEXT    NOT NULL,
	user_agent     TEXT    NOT NULL DEFAULT '',
	referer        TEXT    NOT NULL DEFAULT '',
	monitor_id     INTEGER DEFAULT NULL,
	route_group    TEXT    NOT NULL DEFAULT '',
	created_at     TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_request_logs_created ON request_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_request_logs_monitor ON request_logs(monitor_id, created_at) WHERE monitor_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_request_logs_group   ON request_logs(route_group, created_at);

CREATE TABLE IF NOT EXISTS request_log_rollups (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	date            TEXT    NOT NULL,
	route_group     TEXT    NOT NULL DEFAULT '',
	monitor_id      INTEGER DEFAULT NULL,
	requests        INTEGER NOT NULL DEFAULT 0,
	unique_visitors INTEGER NOT NULL DEFAULT 0,
	avg_latency_ms  INTEGER NOT NULL DEFAULT 0,
	UNIQUE(date, route_group, monitor_id)
);
`

// migrations holds incremental schema changes after the initial schema.
var migrations = []struct {
	version int
	sql     string
}{
	{
		version: 2,
		sql: `
ALTER TABLE monitors ADD COLUMN public INTEGER NOT NULL DEFAULT 0;
CREATE TABLE IF NOT EXISTS heartbeats (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	monitor_id  INTEGER NOT NULL UNIQUE REFERENCES monitors(id) ON DELETE CASCADE,
	token       TEXT    NOT NULL UNIQUE,
	grace       INTEGER NOT NULL DEFAULT 0,
	last_ping_at TEXT,
	status      TEXT    NOT NULL DEFAULT 'pending'
);`,
	},
	{
		version: 3,
		sql: `
CREATE INDEX IF NOT EXISTS idx_check_results_created_at ON check_results(created_at);
CREATE INDEX IF NOT EXISTS idx_incidents_resolved_at ON incidents(status, resolved_at);
CREATE INDEX IF NOT EXISTS idx_audit_log_created_at ON audit_log(created_at);`,
	},
	{
		version: 4,
		sql: `
CREATE TABLE IF NOT EXISTS sessions (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	token_hash   TEXT    NOT NULL UNIQUE,
	api_key_name TEXT    NOT NULL,
	ip_address   TEXT    NOT NULL DEFAULT '',
	created_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
	expires_at   TEXT    NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);`,
	},
	{
		version: 5,
		sql:     `ALTER TABLE sessions ADD COLUMN key_hash TEXT NOT NULL DEFAULT '';`,
	},
	{
		version: 6,
		sql: `
CREATE TABLE IF NOT EXISTS request_logs (
	id             INTEGER PRIMARY KEY AUTOINCREMENT,
	method         TEXT    NOT NULL,
	path           TEXT    NOT NULL,
	status_code    INTEGER NOT NULL,
	latency_ms     INTEGER NOT NULL,
	client_ip      TEXT    NOT NULL,
	user_agent     TEXT    NOT NULL DEFAULT '',
	referer        TEXT    NOT NULL DEFAULT '',
	monitor_id     INTEGER DEFAULT NULL,
	route_group    TEXT    NOT NULL DEFAULT '',
	created_at     TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_request_logs_created ON request_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_request_logs_monitor ON request_logs(monitor_id, created_at) WHERE monitor_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_request_logs_group   ON request_logs(route_group, created_at);

CREATE TABLE IF NOT EXISTS request_log_rollups (
	id              INTEGER PRIMARY KEY AUTOINCREMENT,
	date            TEXT    NOT NULL,
	route_group     TEXT    NOT NULL DEFAULT '',
	monitor_id      INTEGER DEFAULT NULL,
	requests        INTEGER NOT NULL DEFAULT 0,
	unique_visitors INTEGER NOT NULL DEFAULT 0,
	avg_latency_ms  INTEGER NOT NULL DEFAULT 0,
	UNIQUE(date, route_group, monitor_id)
);`,
	},
	{
		version: 7,
		sql: `
CREATE TABLE IF NOT EXISTS status_page_config (
	id              INTEGER PRIMARY KEY DEFAULT 1,
	enabled         INTEGER NOT NULL DEFAULT 0,
	title           TEXT    NOT NULL DEFAULT 'Service Status',
	description     TEXT    NOT NULL DEFAULT '',
	show_incidents  INTEGER NOT NULL DEFAULT 1,
	custom_css      TEXT    NOT NULL DEFAULT '',
	updated_at      TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);
INSERT OR IGNORE INTO status_page_config (id) VALUES (1);`,
	},
	{
		version: 8,
		sql:     `ALTER TABLE status_page_config ADD COLUMN slug TEXT NOT NULL DEFAULT 'status';`,
	},
	{
		version: 9,
		sql: `ALTER TABLE monitors ADD COLUMN description TEXT NOT NULL DEFAULT '';
ALTER TABLE monitors ADD COLUMN upside_down INTEGER NOT NULL DEFAULT 0;
ALTER TABLE monitors ADD COLUMN resend_interval INTEGER NOT NULL DEFAULT 0;`,
	},
	{
		version: 10,
		sql: `CREATE INDEX IF NOT EXISTS idx_request_logs_client_ip ON request_logs(client_ip, created_at);
CREATE INDEX IF NOT EXISTS idx_check_results_monitor_latest ON check_results(monitor_id, id DESC);`,
	},
}
