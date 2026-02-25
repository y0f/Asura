<p align="center">
  <h1 align="center">
    <img src="assets/asura.gif" alt="Asura Logo"/>
  </h1>
  <p align="center">A self-contained Go monitoring service with no external runtime dependencies.</p>
  <p align="center">
    <a href="https://github.com/y0f/Asura/actions/workflows/ci.yml"><img src="https://github.com/y0f/Asura/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
    <a href="https://goreportcard.com/report/github.com/y0f/Asura?branch=main"><img src="https://goreportcard.com/badge/github.com/y0f/Asura?branch=main" alt="Go Report Card"></a>
    <a href="https://github.com/y0f/Asura/blob/main/go.mod"><img src="https://img.shields.io/github/go-mod/go-version/y0f/Asura" alt="Go Version"></a>
    <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License"></a>
    <a href="https://github.com/y0f/Asura/releases/latest"><img src="https://img.shields.io/github/v/release/y0f/Asura?include_prereleases&sort=semver" alt="Release"></a>
    <a href="https://github.com/y0f/Asura/pkgs/container/asura"><img src="https://img.shields.io/badge/ghcr.io-asura-blue?logo=docker" alt="Docker"></a>
  </p>
  <p align="center">
    <a href="https://y0f.github.io/Asura/">Documentation</a> &middot;
    <a href="https://y0f.github.io/Asura/#getting-started">Quick Start</a> &middot;
    <a href="https://y0f.github.io/Asura/#api">API Docs</a> &middot;
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
| **Concurrency** | Goroutine worker pool with channel backpressure and adaptive scheduling | Event loop or thread pool |
| **Deploy** | `scp` binary + run | Package manager, runtime install, migrations |
| **Config** | One YAML file | Multiple config files, env vars, database setup |
| **RAM** | ~20 MB idle | Varies — runtime + database overhead |

No runtime. No external database. No container required. Build, copy, run.

### Highlights

| Feature | |
|---|---|
| **12 protocols** | HTTP, TCP, DNS, ICMP, TLS, WebSocket, Command, Docker, Heartbeat, Domain (WHOIS), gRPC, MQTT |
| **Assertion engine** | 9 types -- status code, body text, body regex, JSON path, headers, response time, cert expiry, DNS records |
| **Change detection** | Line-level diffs on response bodies |
| **Incidents** | Automatic creation, thresholds, ack, recovery |
| **Notifications** | Webhook (HMAC-SHA256), Email, Telegram, Discord, Slack, ntfy |
| **Monitor groups** | Organize monitors into named groups with custom sort order |
| **Proxy support** | HTTP and SOCKS5 proxies with per-monitor assignment |
| **Maintenance** | Recurring windows to suppress alerts |
| **Heartbeat monitoring** | Cron jobs, workers, and pipelines report in -- silence triggers incidents |
| **Web dashboard** | Form-based monitor & notification config, assertion builder, dark/light mode |
| **Request logging** | Built-in request log viewer with visitor analytics and per-monitor tracking |
| **Multiple status pages** | Create multiple public status pages, each with its own slug, monitors, and grouping |
| **Analytics** | Uptime %, response time percentiles |
| **Prometheus** | `/metrics` endpoint with per-monitor, incident, and request metrics |
| **Sub-path support** | Serve from `/asura` or any prefix behind a reverse proxy |
| **Trusted proxies** | Correct client IP detection behind nginx/caddy |
| **SQLite + WAL** | Concurrent reads, single writer, zero config |
| **~15 MB** | Cross-compiles to `linux/amd64` and `linux/arm64` |

---

## Web UI

Built with HTMX, Tailwind, and Alpine.js.

![Web UI](assets/webpanel.png)

---

## Quick Start

```bash
git clone https://github.com/y0f/Asura.git
cd Asura
sudo bash install.sh
```

Installs Go (if needed), builds the binary, creates a systemd service and generates an admin key. Under 2 minutes on a fresh Ubuntu box.

**Important:** By default, Asura binds to `127.0.0.1:8090` and is **not** accessible from the internet. You must set up a [reverse proxy](https://y0f.github.io/Asura/#deployment) to expose it.

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

---

## Documentation

Full documentation is available at **[y0f.github.io/Asura](https://y0f.github.io/Asura/)**.

| Topic | Description |
|---|---|
| [Getting Started](https://y0f.github.io/Asura/#getting-started) | Install via VPS, Docker, or source |
| [Configuration](https://y0f.github.io/Asura/#configuration) | Full config reference, auth, adaptive intervals |
| [Monitors](https://y0f.github.io/Asura/#monitors) | 12 protocols, settings, assertions, heartbeats |
| [API Reference](https://y0f.github.io/Asura/#api) | All endpoints, fields, pagination |
| [Notifications](https://y0f.github.io/Asura/#notifications) | 6 channels, webhook signing, per-monitor routing |
| [Deployment](https://y0f.github.io/Asura/#deployment) | Production nginx/caddy setup, TLS |
| [Architecture](https://y0f.github.io/Asura/#architecture) | Pipeline, storage, checker registry |

---

## Architecture

```
Scheduler -> Worker Pool -> Result Processor -> Dispatcher
    |            |               |                  |
 Min-heap    Concurrent      Incidents +      Webhook/Email/Telegram
 dispatch     Checks        Change Diffs      Discord/Slack/ntfy
```

Min-heap scheduler dispatches only due monitors each tick (O(log n) per dispatch). Channel-based pipeline with backpressure. SQLite WAL mode with separate read/write pools.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## Security

See [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE)
