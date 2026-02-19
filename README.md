<p align="center">
  <h1 align="center">
    <img src="assets/asura.gif" alt="Asura Logo"/>
  </h1>
  <p align="center">A self-contained Go monitoring service with no external runtime dependencies.</p>
  <p align="center">
    <a href="https://github.com/y0f/Asura/actions/workflows/ci.yml"><img src="https://github.com/y0f/Asura/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
    <a href="https://goreportcard.com/report/github.com/y0f/Asura"><img src="https://goreportcard.com/badge/github.com/y0f/Asura" alt="Go Report Card"></a>
    <a href="https://github.com/y0f/Asura/blob/main/go.mod"><img src="https://img.shields.io/github/go-mod/go-version/y0f/Asura" alt="Go Version"></a>
    <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License"></a>
    <a href="https://github.com/y0f/Asura/releases/latest"><img src="https://img.shields.io/github/v/release/y0f/Asura?include_prereleases&sort=semver" alt="Release"></a>
    <a href="https://github.com/y0f/Asura/pkgs/container/asura"><img src="https://img.shields.io/badge/ghcr.io-asura-blue?logo=docker" alt="Docker"></a>
  </p>
  <p align="center">
    <a href="#quick-start">Quick Start</a> &middot;
    <a href="#production-deployment">Production Deployment</a> &middot;
    <a href="#api">API Docs</a> &middot;
    <a href="#configuration">Configuration</a> &middot;
    <a href="CONTRIBUTING.md">Contributing</a>
  </p>
</p>

---

Asura monitors your infrastructure from a single Go binary backed by SQLite. No Postgres. No Redis. No Node.js. Just `scp` a binary and go.

```bash
git clone https://github.com/y0f/Asura.git && cd Asura && sudo bash install.sh
```

### Why Asura?

| | Asura | Typical alternative |
|---|---|---|
| **Runtime** | Single static binary | Node.js / Java / Python runtime |
| **Database** | SQLite compiled in | Requires Postgres, MySQL, or Redis |
| **Binary size** | ~15 MB | 100-500 MB installed |
| **Concurrency** | Goroutine worker pool with channel backpressure | Single-threaded or thread-per-request |
| **Deploy** | `scp` binary + run | Package manager, runtime install, migrations |
| **Config** | One YAML file | Multiple config files, env vars, database setup |
| **RAM** | Runs on a $5 VPS | Often needs 512 MB+ |

No runtime. No external database. No container required. Build, copy, run.

### Highlights

| Feature | |
|---|---|
| **8 protocols** | HTTP, TCP, DNS, ICMP, TLS, WebSocket, Command, Heartbeat |
| **Assertion engine** | 9 types -- status code, body text, body regex, JSON path, headers, response time, cert expiry, DNS records |
| **Change detection** | Line-level diffs on response bodies |
| **Incidents** | Automatic creation, thresholds, ack, recovery |
| **Notifications** | Webhook (HMAC-SHA256), Email, Telegram, Discord, Slack |
| **Maintenance** | Recurring windows to suppress alerts |
| **Heartbeat monitoring** | Cron jobs, workers, and pipelines report in -- silence triggers incidents |
| **Web dashboard** | Dark/light-mode UI with system preference -- manage everything from the browser |
| **Request logging** | Built-in request log viewer with visitor analytics and per-monitor tracking |
| **Public status page** | Configurable hosted page with 90-day uptime bars, or build your own via API |
| **Analytics** | Uptime %, response time percentiles |
| **Prometheus** | `/metrics` endpoint, ready to scrape |
| **Sub-path support** | Serve from `/asura` or any prefix behind a reverse proxy |
| **Trusted proxies** | Correct client IP detection behind nginx/caddy |
| **SQLite + WAL** | Concurrent reads, single writer, zero config |
| **~15 MB** | Cross-compiles to `linux/amd64` and `linux/arm64` |

---

## Quick Start

### VPS (recommended)

```bash
git clone https://github.com/y0f/Asura.git
cd Asura
sudo bash install.sh
```

Installs Go (if needed), builds the binary, creates a systemd service with a cryptographically generated admin key. Under 2 minutes on a fresh Ubuntu box.

**Important:** By default, Asura binds to `127.0.0.1:8090` and is **not** accessible from the internet. You must set up a reverse proxy to expose it. See [Production Deployment](#production-deployment) below.

```bash
systemctl status asura
curl http://127.0.0.1:8090/api/v1/health
```

### Docker Compose

```bash
git clone https://github.com/y0f/Asura.git
cd Asura
cp config.example.yaml config.yaml
# set your API key hash and database.path to /app/data/asura.db
docker compose up -d
```

### From source

```bash
make build
./asura --setup                        # generates key + hash
cp config.example.yaml config.yaml     # paste the hash
./asura -config config.yaml
```

### Cross-compile + deploy

```bash
make release
scp dist/asura-linux-amd64 you@server:/usr/local/bin/asura
```

---

## Production Deployment

This section walks through deploying Asura on a VPS so that it is **never exposed directly to the internet**. The pattern: Asura listens on localhost, nginx terminates TLS and proxies to it.

### 1. Install Asura

```bash
git clone https://github.com/y0f/Asura.git
cd Asura
sudo bash install.sh
```

Save the admin API key printed at the end. It cannot be recovered.

### 2. Configure

Edit `/etc/asura/config.yaml`:

```yaml
server:
  # Bind to localhost only — never bind to 0.0.0.0 on a public server
  listen: "127.0.0.1:8090"

  # Serve all routes under /asura (optional, remove for root)
  base_path: "/asura"

  # Your public URL — used in notification links
  external_url: "https://example.com/asura"

  # Trust nginx's forwarded headers for real client IP
  trusted_proxies:
    - "127.0.0.1"
    - "::1"

auth:
  session:
    cookie_secure: true    # Requires HTTPS (which nginx provides)
```

### 3. Set up nginx reverse proxy

Create `/etc/nginx/sites-available/asura`:

```nginx
# Redirect HTTP to HTTPS
server {
    listen 80;
    server_name example.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name example.com;

    ssl_certificate     /etc/letsencrypt/live/example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/example.com/privkey.pem;

    # Redirect /asura to /asura/ (trailing slash required)
    location = /asura {
        return 301 /asura/;
    }

    # Proxy /asura/ to Asura (base_path handles the prefix natively)
    location /asura/ {
        proxy_pass http://127.0.0.1:8090;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

Enable and reload:

```bash
sudo ln -s /etc/nginx/sites-available/asura /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx
```

### 4. Verify

```bash
# Local health check (bypasses nginx)
curl http://127.0.0.1:8090/asura/api/v1/health

# Public health check (through nginx + TLS)
curl https://example.com/asura/api/v1/health

# Web UI
# Open https://example.com/asura/ in your browser
```

### Alternative: Caddy

Caddy handles TLS automatically:

```
example.com {
    redir /asura /asura/ permanent
    reverse_proxy /asura/* 127.0.0.1:8090
}
```

### Serving from Root (no base_path)

If you want Asura at `https://monitor.example.com/` instead of a sub-path, omit `base_path` and proxy the entire domain:

```yaml
server:
  listen: "127.0.0.1:8090"
  # base_path is empty — serves from root
```

```nginx
server {
    listen 443 ssl http2;
    server_name monitor.example.com;
    # ...TLS config...
    location / {
        proxy_pass http://127.0.0.1:8090;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

---

## Manual Setup

```bash
# Build
make build
sudo install -m 755 asura /usr/local/bin/asura

# Generate API key
asura --setup

# System user + directories
sudo useradd --system --no-create-home --shell /usr/sbin/nologin asura
sudo mkdir -p /etc/asura /var/lib/asura
sudo chown asura:asura /var/lib/asura
sudo chmod 700 /var/lib/asura

# Config
sudo cp config.example.yaml /etc/asura/config.yaml
# set hash, database.path, base_path, trusted_proxies
sudo chmod 640 /etc/asura/config.yaml
sudo chown root:asura /etc/asura/config.yaml

# Systemd
sudo cp asura.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now asura
```

---

## Configuration

See [`config.example.yaml`](config.example.yaml) for all options. Environment variables expand via `${VAR_NAME}`.

| Section    | Controls                                              |
|------------|-------------------------------------------------------|
| `server`   | Listen address, TLS, timeouts, CORS, rate limiting, base path, external URL, trusted proxies, web UI toggle, frame embedding |
| `database` | SQLite path, read pool size, retention policy, request log retention |
| `auth`     | API keys (SHA-256 hashed), roles, session lifetime, login rate limiting |
| `monitor`  | Worker count, default intervals, thresholds           |
| `logging`  | Level (debug/info/warn/error), format (text/json)     |

### Key Server Settings

| Setting | Default | Description |
|---------|---------|-------------|
| `listen` | `127.0.0.1:8090` | Address to bind. Use `127.0.0.1:PORT` in production |
| `base_path` | `""` | URL prefix for all routes (e.g. `/asura`) |
| `external_url` | auto | Public URL for notification links |
| `trusted_proxies` | `[]` | IPs/CIDRs whose `X-Real-IP`/`X-Forwarded-For` headers are trusted |
| `rate_limit_per_sec` | `10` | Per-IP request rate limit |
| `web_ui_enabled` | `true` | Set `false` for API-only mode |

---

## Authentication

Asura uses API keys authenticated via SHA-256 hashes. Keys are configured in `config.yaml` -- there is no user registration or database-stored auth.

### Generating a Key (Recommended)

Use the built-in generator for a cryptographically secure key:

```bash
./asura --setup
```

Output:

```
  API Key : ak_a8f3e7b2c1d9...
  Hash    : fa223e3e1c4b96...

  Put the hash in config.yaml under auth.api_keys[].hash
  Save the API key -- it cannot be recovered.
```

The `ak_` prefix makes keys identifiable in logs and config without exposing the secret.

### Hashing an Existing Key

If you prefer to choose your own key:

```bash
./asura --hash-key "your-secret-key"
# Output: 2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae
```

### Config

```yaml
auth:
  api_keys:
    - name: "admin"
      hash: "2c26b46b68ffc..."
      role: "admin"
```

### Roles

| Role | Access |
|------|--------|
| `admin` | Full read/write access to all resources |
| `readonly` | Read-only access (monitors, incidents, notifications, maintenance, metrics) |

### Custom Permissions

Instead of a role, you can grant specific permissions:

```yaml
auth:
  api_keys:
    - name: "ci-bot"
      hash: "..."
      permissions:
        - "monitors.read"
        - "monitors.write"
        - "incidents.read"
```

Available permissions: `monitors.read`, `monitors.write`, `incidents.read`, `incidents.write`, `notifications.read`, `notifications.write`, `maintenance.read`, `maintenance.write`, `metrics.read`.

### Using Your Key

**API**: Pass the raw key (not the hash) in the `X-API-Key` header:

```bash
curl -H "X-API-Key: ak_a8f3e7b2c1d9..." https://example.com/asura/api/v1/monitors
```

**Web UI**: Enter the raw key on the login page. A server-side session is created with a secure random token stored in a cookie (24h expiry by default, HttpOnly, Secure). The raw API key is never stored in the cookie. Login attempts are rate-limited per IP.

You can configure multiple keys with different names and permissions. Each key's name is recorded in the audit log for traceability. Login successes and failures are also audited.

---

## API

All endpoints return JSON. Authenticate with `X-API-Key` header. When `base_path` is configured, all paths are prefixed (e.g. `/asura/api/v1/health`).

### Health *(no auth)*

```
GET  /api/v1/health       Status, version, uptime
```

### Metrics *(read auth)*

```
GET  /metrics             Prometheus exposition format
```

### Monitors

```
GET    /api/v1/monitors                List
POST   /api/v1/monitors                Create
GET    /api/v1/monitors/{id}           Get
PUT    /api/v1/monitors/{id}           Update
DELETE /api/v1/monitors/{id}           Delete
POST   /api/v1/monitors/{id}/pause     Pause
POST   /api/v1/monitors/{id}/resume    Resume
GET    /api/v1/monitors/{id}/checks    Check history
GET    /api/v1/monitors/{id}/metrics   Analytics
GET    /api/v1/monitors/{id}/changes   Content changes
```

| Field              | Type     | Required | Description                                        |
|--------------------|----------|----------|----------------------------------------------------|
| `name`             | string   | yes      | Display name                                       |
| `type`             | string   | yes      | `http` `tcp` `dns` `icmp` `tls` `websocket` `command` `heartbeat` |
| `target`           | string   | yes      | URL, host:port, domain, or command                 |
| `interval`         | int      |          | Seconds between checks (default: 60)               |
| `timeout`          | int      |          | Timeout in seconds (default: 10)                   |
| `tags`             | string[] |          | Grouping tags                                      |
| `settings`         | object   |          | Protocol-specific ([see below](#protocol-settings)) |
| `assertions`       | array    |          | Assertion rules ([see below](#assertions))          |
| `track_changes`    | bool     |          | Enable content change detection                    |
| `failure_threshold`| int      |          | Failures before incident (default: 3)              |
| `success_threshold`| int      |          | Successes before recovery (default: 1)             |
| `public`           | bool     |          | Expose to badges and public status page (default: false) |

### Heartbeat Monitoring

Create a heartbeat monitor to track cron jobs, workers, or pipelines. If they stop pinging, Asura fires an incident.

```bash
# Create heartbeat monitor
curl -X POST https://example.com/asura/api/v1/monitors \
  -H "X-API-Key: $KEY" \
  -H "Content-Type: application/json" \
  -d '{"name":"Nightly Backup","type":"heartbeat","interval":3600,"settings":{"grace":300}}'
```

Response includes the ping token:

```json
{
  "monitor": { "id": 1, "name": "Nightly Backup", "type": "heartbeat", ... },
  "heartbeat": { "token": "a1b2c3d4e5f6...", "grace": 300, "status": "pending" }
}
```

Ping from your script (no auth needed):

```bash
curl -X POST https://example.com/asura/api/v1/heartbeat/a1b2c3d4e5f6...
```

If no ping arrives within `interval + grace` seconds, the monitor goes down and an incident is created.

### Public Status Page *(no auth)*

```
GET  /api/v1/status          Public status overview (monitors, uptime, incidents)
```

Returns only safe fields (name, type, status, uptime) — no targets, settings, or credentials are exposed. Returns 404 if the status page is disabled in settings. Set `"public": true` on monitors to include them.

### Status Page Config

```
GET  /api/v1/status/config   Get status page settings
PUT  /api/v1/status/config   Update status page settings
```

Configure the public status page via API. Fields: `enabled` (bool), `title`, `description`, `show_incidents` (bool), `custom_css`, `slug` (URL path, e.g. `"status"` serves at `/{slug}`).

The built-in web UI also serves a hosted status page at `/{slug}` with 90-day uptime bars. Configure it from the sidebar under **Status Page** — set the title, description, URL slug, toggle incident history, and add custom CSS. Monitors with `public: true` appear automatically.

### Status Badges *(no auth, public monitors only)*

```
GET  /api/v1/badge/{id}/status     Status badge (up/down/degraded)
GET  /api/v1/badge/{id}/uptime     30-day uptime percentage
GET  /api/v1/badge/{id}/response   24h median response time
```

Set `"public": true` on a monitor to enable badges. Embed in a README:

```markdown
![Status](https://example.com/asura/api/v1/badge/1/status)
![Uptime](https://example.com/asura/api/v1/badge/1/uptime)
```

### Incidents

```
GET    /api/v1/incidents               List (filter: monitor_id, status)
GET    /api/v1/incidents/{id}          Get with timeline
POST   /api/v1/incidents/{id}/ack      Acknowledge
POST   /api/v1/incidents/{id}/resolve  Resolve
DELETE /api/v1/incidents/{id}          Delete
```

### Notifications

```
GET    /api/v1/notifications           List
POST   /api/v1/notifications           Create
PUT    /api/v1/notifications/{id}      Update
DELETE /api/v1/notifications/{id}      Delete
POST   /api/v1/notifications/{id}/test Test
```

Types: `webhook` `email` `telegram` `discord` `slack`

Events: `incident.created` `incident.acknowledged` `incident.resolved` `content.changed`

### Maintenance Windows

```
GET    /api/v1/maintenance             List
POST   /api/v1/maintenance             Create
PUT    /api/v1/maintenance/{id}        Update
DELETE /api/v1/maintenance/{id}        Delete
```

`recurring` values: `""` (one-time), `"daily"`, `"weekly"`, `"monthly"`

### Request Logs

```
GET    /api/v1/request-logs            List (filter: group, method, status_code, range)
GET    /api/v1/request-logs/stats      Aggregate stats (requests, visitors, latency)
```

### Other

```
GET    /api/v1/overview                Status summary
GET    /api/v1/tags                    All tags
```

Pagination: `?page=1&per_page=20` on all list endpoints.

---

## Assertions

Evaluated after each check. Failed assertions mark a monitor `down` or `degraded`.

| Type            | Description                 | Operators                          |
|-----------------|-----------------------------|------------------------------------|
| `status_code`   | HTTP status code            | eq, neq, gt, lt, gte, lte         |
| `body_contains` | Body text search            | contains, not_contains             |
| `body_regex`    | Body regex match            | matches, not_matches               |
| `json_path`     | JSON value at dot-path      | eq, neq, gt, lt, contains, exists |
| `header`        | Response header value       | eq, neq, contains, exists         |
| `response_time` | Response time (ms)          | lt, lte, gt, gte                  |
| `cert_expiry`   | Days until cert expires     | gt, gte, lt, lte                  |
| `dns_record`    | DNS record value            | contains, eq                       |

```json
{
  "assertions": [
    {"type": "status_code", "operator": "eq", "value": "200"},
    {"type": "response_time", "operator": "lt", "value": "2000"},
    {"type": "json_path", "target": "status", "operator": "eq", "value": "ok"},
    {"type": "response_time", "operator": "lt", "value": "5000", "degraded": true}
  ]
}
```

---

## Protocol Settings

<details><summary><strong>HTTP</strong></summary>

```json
{"method":"POST","headers":{"Authorization":"Bearer token"},"body":"{\"key\":\"value\"}","follow_redirects":true,"skip_tls_verify":false,"basic_auth_user":"user","basic_auth_pass":"pass"}
```
</details>

<details><summary><strong>TCP</strong></summary>

```json
{"send_data":"PING\r\n","expect_data":"PONG"}
```
</details>

<details><summary><strong>DNS</strong></summary>

```json
{"record_type":"A","server":"8.8.8.8"}
```
</details>

<details><summary><strong>TLS</strong></summary>

```json
{"warn_days_before":30}
```
</details>

<details><summary><strong>WebSocket</strong></summary>

```json
{"headers":{"Authorization":"Bearer token"},"send_message":"ping","expect_reply":"pong"}
```
</details>

<details><summary><strong>Command</strong></summary>

```json
{"command":"/usr/local/bin/check_health","args":["--host","db.local"]}
```
</details>

---

## Webhook Signing

Webhook notifications include an `X-Asura-Signature` header: `sha256=<hex HMAC-SHA256 of body>`.

## Architecture

```
Scheduler -> Worker Pool -> Result Processor -> Dispatcher
    |            |              |                |
  Cron      Concurrent     Incidents +      Webhook/Email/
 Tickers     Checks       Change Diffs    Telegram/Discord/Slack
```

Channel-based pipeline with backpressure. SQLite WAL mode with separate read/write pools.

---

## Web UI

Asura includes a lightweight built-in dashboard implemented with HTMX, TailwindCSS, and Alpine.js.

The UI is enabled by default and can be disabled for API-only deployments:

```yaml
server:
  web_ui_enabled: true
```

See the [Configuration](#configuration) section for details.

![Web UI](assets/webpanel.png)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Bug reports, feature requests, and pull requests are welcome.

## Security

See [SECURITY.md](SECURITY.md) for reporting vulnerabilities.

## License

[MIT](LICENSE)
