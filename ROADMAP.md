# Asura Roadmap

Implementation plan for expanding Asura. Each phase builds on the previous one. Every feature stays inside the single binary -- no external dependencies, no extra services.

---

## Phase 1 -- Heartbeat Monitoring

**Why:** No self-hosted tool does this well. Lets cron jobs, workers, and pipelines report in. If they go silent, Asura fires an incident. Completely new user segment with zero competition.

**What changes:**

### New model: `Heartbeat`
```go
type Heartbeat struct {
    ID          int64     // PK
    MonitorID   int64     // FK to monitors table
    Token       string    // unique URL token (32-char random)
    Grace       int       // grace period in seconds after expected interval
    LastPingAt  *time.Time
    Status      string    // "up", "down", "pending"
}
```

### New monitor type: `heartbeat`
- Created like any monitor: `POST /api/v1/monitors` with `"type": "heartbeat"`
- Returns a unique ping URL: `POST /api/v1/heartbeat/{token}`
- The heartbeat token is generated server-side on monitor creation
- No checker runs -- instead, a background goroutine checks if `LastPingAt + Interval + Grace` has passed
- If expired, mark down and trigger incident through existing pipeline

### Storage changes
- Migration: `CREATE TABLE heartbeats (id, monitor_id, token UNIQUE, grace, last_ping_at, status)`
- New store methods: `CreateHeartbeat`, `GetHeartbeatByToken`, `UpdateHeartbeatPing`, `ListExpiredHeartbeats`

### API changes
- `POST /api/v1/heartbeat/{token}` -- no auth, just the token. Returns 200 + records ping time
- `GET /api/v1/monitors/{id}` -- includes heartbeat info (token, last ping, grace) when type is heartbeat
- Heartbeat URL shown in monitor creation response so the user can copy it

### Pipeline changes
- New `HeartbeatWatcher` goroutine started alongside scheduler
- Ticks every 30s, calls `ListExpiredHeartbeats`, feeds failures into existing incident manager
- Reuses all existing incident + notification infrastructure

### Config
- `monitor.heartbeat_check_interval: 30s` -- how often to scan for expired heartbeats

### Test plan
- Unit: heartbeat expiry logic with mocked time
- Integration: create heartbeat monitor via API, send pings, verify status transitions
- Edge: grace period boundary, rapid pings, token collision

---

## Phase 2 -- SVG Status Badges

**Why:** Free viral marketing. People embed these in READMEs and dashboards. Every badge links back to the project.

**What changes:**

### New endpoints (no auth)
```
GET /api/v1/badge/{id}.svg          Monitor status badge
GET /api/v1/badge/{id}/uptime.svg   Uptime percentage badge
GET /api/v1/badge/{id}/response.svg Response time badge
```

### Implementation
- SVG templates embedded via `//go:embed` (3 template files, ~1KB each)
- Templates use shields.io-style format: left label + right value with color
- Colors: green (#4c1), yellow (#dfb317), red (#e05d44), grey (#9f9f9f)
- Cache-Control header: `max-age=300` (5 min cache)
- Monitor must be explicitly marked as `public: true` to expose badges (default false)

### Model change
- Add `Public bool` field to `Monitor` struct
- Migration: `ALTER TABLE monitors ADD COLUMN public BOOLEAN DEFAULT 0`

### Test plan
- Unit: SVG rendering with various statuses
- Integration: badge endpoint returns valid SVG, respects public flag, 404 for private monitors

---

## Phase 3 -- Embedded Dashboard

**Why:** Unlocks 90% of potential users. API-only limits the audience to developers. A web UI makes Asura accessible to anyone.

**What changes:**

### Frontend
- Minimal SPA (vanilla JS + HTML + CSS, or Preact for reactivity -- keep bundle under 200KB)
- Pages: Overview, Monitors list, Monitor detail (checks + chart), Incidents, Notifications, Settings
- Charts: response time sparkline (canvas-based, no chart library)
- Real-time: poll `/api/v1/overview` every 30s

### Embedding
- `//go:embed web/dist/*` in a new `internal/web/` package
- Served at `GET /` with fallback to `index.html` for SPA routing
- API at `/api/v1/*` unchanged
- Zero build step for backend -- `make build` compiles the frontend into the binary

### Build changes
- `web/` directory with frontend source
- `Makefile`: `build-web` target runs the frontend build, `build` depends on `build-web`
- CI: install Node only for frontend build step, final binary has no Node dependency

### Route registration
```go
// In server.go registerRoutes()
mux.Handle("GET /", web.Handler())  // serves embedded SPA
```

### Test plan
- Frontend: manual testing across monitors/incidents/notifications pages
- Backend: existing API tests cover all data endpoints
- Integration: verify `GET /` returns HTML, `GET /api/v1/health` still returns JSON

---

## Phase 4 -- Public Status Page

**Why:** This is what non-technical stakeholders see. Companies pay $29/mo for Statuspage.io. Build it in for free.

**What changes:**

### New config section
```yaml
status_page:
  enabled: true
  title: "Service Status"
  description: "Current status of our services"
  # only monitors with public: true are shown
```

### New endpoints (no auth)
```
GET /status              HTML status page (embedded template)
GET /api/v1/status       JSON status data (public monitors only)
```

### Implementation
- HTML template embedded via `//go:embed`
- Shows: current status per public monitor, uptime bars (90 days), active incidents, historical incidents
- Auto-refreshes via meta tag or polling
- Minimal CSS, mobile-responsive, no JS required for basic view
- Optional: custom CSS via config `status_page.custom_css_path`

### Storage
- `GetPublicMonitorsOverview(ctx) ([]*PublicMonitorStatus, error)` -- returns only public monitors with uptime data

### Test plan
- Unit: status page data aggregation
- Integration: status page shows correct monitors, respects public flag
- Visual: verify rendering on mobile/desktop

---

## Phase 5 -- Multi-Location Probes

**Why:** Eliminates false positives. SaaS tools charge $50+/mo for this. No self-hosted tool does it.

**What changes:**

### Architecture
```
Primary Node                    Probe Nodes
┌──────────────┐       ┌───────────────────┐
│ API server   │◄──────│ Probe (region: EU) │
│ Scheduler    │       └───────────────────┘
│ Storage      │       ┌───────────────────┐
│ Dashboard    │◄──────│ Probe (region: US) │
│ Incidents    │       └───────────────────┘
│ Notifications│       ┌───────────────────┐
└──────────────┘◄──────│ Probe (region: AP) │
                       └───────────────────┘
```

### Probe mode
- Same binary: `asura -mode probe -primary https://primary:8080 -key xxx -region eu-west`
- Probe pulls monitor list from primary via `GET /api/v1/internal/monitors`
- Runs checks locally, pushes results to primary via `POST /api/v1/internal/results`
- No local storage, no local API -- just a check runner

### Primary changes
- New internal API endpoints (authenticated with a probe-specific key):
  - `GET /api/v1/internal/monitors` -- returns enabled monitors
  - `POST /api/v1/internal/results` -- accepts check results from probes
- New config section:
  ```yaml
  probes:
    enabled: true
    probe_key_hash: "sha256..."
    consensus: 2        # N probes must agree before marking down
    consensus_window: 60 # seconds to wait for probe results
  ```

### Storage changes
- `check_results` table gets `probe_region TEXT DEFAULT 'local'`
- New `ProbeResult` model with region tag
- Consensus logic: collect results from all probes within window, if N agree on down → incident

### Model changes
- `CheckResult` gets `Region string` field
- Analytics can filter/group by region

### Config changes
- New `ProbeConfig` struct
- Primary mode is default (no flag needed)

### Test plan
- Unit: consensus logic with various probe result combinations
- Integration: probe registration, result push, consensus evaluation
- E2E: two probes + primary, simulate one probe seeing failure while other sees success

---

## Phase 6 -- On-Call Scheduling

**Why:** Replaces PagerDuty for small teams. Sticky feature -- once you set up rotations, you don't leave.

**What changes:**

### New models
```go
type OnCallSchedule struct {
    ID       int64
    Name     string
    Timezone string
    Layers   []OnCallLayer  // stored as JSON
}

type OnCallLayer struct {
    Users        []OnCallUser
    RotationType string  // "daily", "weekly", "custom"
    StartTime    string  // "09:00"
    EndTime      string  // "17:00" (empty = 24h)
    HandoffDay   string  // for weekly: "monday"
}

type OnCallUser struct {
    Name               string
    NotificationChannelID int64  // links to existing notification channels
}

type EscalationPolicy struct {
    ID    int64
    Name  string
    Steps []EscalationStep
}

type EscalationStep struct {
    DelayMinutes int
    ScheduleID   int64  // or direct channel ID
}
```

### API
```
GET/POST       /api/v1/oncall/schedules
GET/PUT/DELETE /api/v1/oncall/schedules/{id}
GET            /api/v1/oncall/schedules/{id}/current  -- who's on call now

GET/POST       /api/v1/oncall/policies
GET/PUT/DELETE /api/v1/oncall/policies/{id}
```

### Integration with monitors
- Monitors get optional `escalation_policy_id` field
- When incident fires, dispatcher looks up policy, finds current on-call person, sends to their channel
- If not acked within `delay_minutes`, escalates to next step
- Escalation tracked as incident events

### Storage
- New tables: `oncall_schedules`, `escalation_policies`
- Migration adds both tables + policy FK on monitors

### Test plan
- Unit: rotation calculation (who's on call at time X with timezone Y)
- Unit: escalation step timing
- Integration: incident → on-call lookup → notification → escalation on timeout

---

## Implementation Order

| # | Feature | Dependencies | Effort | Impact |
|---|---------|-------------|--------|--------|
| 1 | Heartbeat monitoring | None | Small | High -- new user segment |
| 2 | SVG badges | None | Small | High -- viral growth |
| 3 | Embedded dashboard | None | Large | Critical -- unlocks mass adoption |
| 4 | Public status page | Phase 2 (public flag) | Medium | High -- replaces paid tools |
| 5 | Multi-location probes | None | Large | High -- major differentiator |
| 6 | On-call scheduling | None | Large | Medium -- sticky retention feature |

Phases 1 and 2 can ship independently and fast. Phase 3 is the big investment that changes the trajectory. Phases 4-6 build on the growing user base.

---

## Rules

- Every feature compiles into the single binary
- No new external dependencies (no Redis, no Postgres, no message queues)
- SQLite stays the only storage backend
- `install.sh` still works unchanged after every phase
- Backward-compatible config -- new fields have sensible defaults
- Every new Store method gets a test
- Every new API endpoint gets a handler test
