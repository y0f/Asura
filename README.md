<p align="center">
  <h1 align="center">Asura</h1>
  <p align="center">Self-hosted uptime monitoring. Single binary. Zero dependencies.</p>
  <p align="center">
    <a href="https://github.com/y0f/Asura/actions/workflows/ci.yml"><img src="https://github.com/y0f/Asura/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
    <a href="https://goreportcard.com/report/github.com/y0f/Asura"><img src="https://goreportcard.com/badge/github.com/y0f/Asura" alt="Go Report Card"></a>
    <a href="https://github.com/y0f/Asura/blob/main/go.mod"><img src="https://img.shields.io/github/go-mod/go-version/y0f/Asura" alt="Go Version"></a>
    <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License"></a>
  </p>
  <p align="center">
    <a href="#quick-start">Quick Start</a> &middot;
    <a href="#api">API Docs</a> &middot;
    <a href="#configuration">Configuration</a> &middot;
    <a href="CONTRIBUTING.md">Contributing</a>
  </p>
</p>

---

Asura monitors your infrastructure from a single Go binary backed by SQLite. No Postgres. No Redis. No Node.js. Just `scp` a binary and go.

```bash
git clone https://github.com/y0f/Asura.git && cd asura && sudo bash install.sh
```

### Highlights

| | |
|---|---|
| **8 protocols** | HTTP, TCP, DNS, ICMP, TLS, WebSocket, Command, Heartbeat |
| **Assertion engine** | 8 types -- status code, response time, JSON path, body regex, headers, cert expiry, DNS records |
| **Change detection** | Line-level diffs on response bodies |
| **Incidents** | Automatic creation, thresholds, ack, recovery |
| **Notifications** | Webhook (HMAC-SHA256), Email, Telegram, Discord, Slack |
| **Maintenance** | Recurring windows to suppress alerts |
| **Heartbeat monitoring** | Cron jobs, workers, and pipelines report in -- silence triggers incidents |
| **Web dashboard** | Built-in dark-mode UI -- manage everything from the browser |
| **Status badges** | Embeddable SVG badges for public monitors |
| **Analytics** | Uptime %, response time percentiles |
| **Prometheus** | `/metrics` endpoint, ready to scrape |
| **SQLite + WAL** | Concurrent reads, single writer, zero config |
| **~15 MB** | Cross-compiles to `linux/amd64` and `linux/arm64` |

---

## Quick Start

### VPS (recommended)

```bash
git clone https://github.com/y0f/Asura.git
cd asura
sudo bash install.sh
```

Installs Go (if needed), builds the binary, creates a systemd service with a generated admin key. Under 2 minutes on a fresh Ubuntu box.

```bash
systemctl status asura
curl http://localhost:8080/api/v1/health
```

### Docker Compose

```bash
git clone https://github.com/y0f/Asura.git
cd asura
cp config.example.yaml config.yaml
# set your API key hash and database.path to /app/data/asura.db
docker compose up -d
```

### From source

```bash
make build
./asura -hash-key "your-secret-key"
cp config.example.yaml config.yaml     # paste the hash
./asura -config config.yaml
```

### Cross-compile + deploy

```bash
make release
scp dist/asura-linux-amd64 you@server:/usr/local/bin/asura
```

---

## Manual Setup

```bash
# Build
make build
sudo install -m 755 asura /usr/local/bin/asura

# System user + directories
sudo useradd --system --no-create-home --shell /usr/sbin/nologin asura
sudo mkdir -p /etc/asura /var/lib/asura
sudo chown asura:asura /var/lib/asura

# Config
asura -hash-key "your-secret-key"
sudo cp config.example.yaml /etc/asura/config.yaml
# set hash + database.path to /var/lib/asura/asura.db

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
| `server`   | Listen address, TLS, timeouts, CORS, rate limiting    |
| `database` | SQLite path, read pool size, retention policy         |
| `auth`     | API keys (SHA-256 hashed), admin / readonly roles     |
| `monitor`  | Worker count, default intervals, thresholds           |
| `logging`  | Level (debug/info/warn/error), format (text/json)     |

---

## API

All endpoints return JSON. Authenticate with `X-API-Key` header.

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
| `public`           | bool     |          | Expose to badge endpoints (default: false)         |

### Heartbeat Monitoring

Create a heartbeat monitor to track cron jobs, workers, or pipelines. If they stop pinging, Asura fires an incident.

```bash
# Create heartbeat monitor
curl -X POST http://localhost:8080/api/v1/monitors \
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
curl -X POST http://your-server:8080/api/v1/heartbeat/a1b2c3d4e5f6...
```

If no ping arrives within `interval + grace` seconds, the monitor goes down and an incident is created.

### Status Badges *(no auth, public monitors only)*

```
GET  /api/v1/badge/{id}/status     Status badge (up/down/degraded)
GET  /api/v1/badge/{id}/uptime     30-day uptime percentage
GET  /api/v1/badge/{id}/response   24h median response time
```

Set `"public": true` on a monitor to enable badges. Embed in a README:

```markdown
![Status](https://your-server/api/v1/badge/1/status)
![Uptime](https://your-server/api/v1/badge/1/uptime)
```

### Incidents

```
GET    /api/v1/incidents               List (filter: monitor_id, status)
GET    /api/v1/incidents/{id}          Get with timeline
POST   /api/v1/incidents/{id}/ack      Acknowledge
POST   /api/v1/incidents/{id}/resolve  Resolve
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

### Maintenance Windows

```
GET    /api/v1/maintenance             List
POST   /api/v1/maintenance             Create
PUT    /api/v1/maintenance/{id}        Update
DELETE /api/v1/maintenance/{id}        Delete
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
Scheduler → Worker Pool → Result Processor → Dispatcher
    ↓            ↓              ↓                ↓
  Cron      Concurrent     Incidents +      Webhook/Email/
 Tickers     Checks       Change Diffs    Telegram/Discord/Slack
```

Channel-based pipeline with backpressure. SQLite WAL mode with separate read/write pools.

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Bug reports, feature requests, and pull requests are welcome.

## Security

See [SECURITY.md](SECURITY.md) for reporting vulnerabilities.

## License

[MIT](LICENSE)
