<p align="center">
  <h1 align="center">
    <img src="assets/asura.gif" alt="Asura Logo"/>
  </h1>
  <p align="center">A self-contained Go monitoring service with no external runtime dependencies.</p>
  <p align="center">
    <a href="https://github.com/y0f/asura/actions/workflows/ci.yml"><img src="https://github.com/y0f/asura/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
    <a href="https://goreportcard.com/report/github.com/y0f/asura?branch=main"><img src="https://goreportcard.com/badge/github.com/y0f/asura?branch=main" alt="Go Report Card"></a>
    <a href="https://github.com/y0f/asura/blob/main/go.mod"><img src="https://img.shields.io/github/go-mod/go-version/y0f/asura" alt="Go Version"></a>
    <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License"></a>
    <a href="https://github.com/y0f/asura/releases/latest"><img src="https://img.shields.io/github/v/release/y0f/asura?include_prereleases&sort=semver" alt="Release"></a>
    <a href="https://github.com/y0f/asura/pkgs/container/asura"><img src="https://img.shields.io/badge/ghcr.io-asura-blue?logo=docker" alt="Docker"></a>
  </p>
  <p align="center">
    <a href="https://y0f.github.io/asura/">Documentation</a> &middot;
    <a href="https://y0f.github.io/asura/#getting-started">Quick Start</a> &middot;
    <a href="https://y0f.github.io/asura/#api">API Docs</a> &middot;
    <a href="CONTRIBUTING.md">Contributing</a>
  </p>
</p>

---

Asura monitors your infrastructure from a single Go binary backed by SQLite. No Postgres. No Redis. No Node.js. Just `scp` a binary and go.

### Why Asura?

| | Asura | Typical alternative |
|---|---|---|
| **Runtime** | Single static binary | Node.js / Java / Python runtime |
| **Database** | SQLite compiled in | Requires Postgres, MySQL, or Redis |
| **Binary size** | ~15 MB | 100-500 MB installed |
| **Deploy** | `scp` binary + run | Package manager, runtime install, migrations |
| **RAM** | ~20 MB idle | Varies — runtime + database overhead |

### Highlights

| Feature | |
|---|---|
| **11 protocols** | HTTP, TCP, DNS, ICMP, TLS, WebSocket, Command, Docker, Domain (WHOIS), gRPC, MQTT |
| **Assertion engine** | 9 types -- status code, body text, body regex, JSON path, headers, response time, cert expiry, DNS records |
| **Incidents** | Automatic creation, thresholds, ack, recovery |
| **Notifications** | Webhook (HMAC-SHA256), Email, Telegram, Discord, Slack, ntfy, Teams, PagerDuty, Opsgenie, Pushover |
| **Status pages** | Multiple public status pages with custom slugs and grouping |
| **Prometheus** | `/metrics` endpoint with per-monitor, incident, and request metrics |

---

## Web UI

![Web UI](assets/webpanel.png)

---

## Quick Start

```bash
git clone https://github.com/y0f/Asura.git && cd Asura && sudo bash install.sh
```

Installs Go (if needed), builds the binary, creates a systemd service and generates an admin key.

By default Asura binds to `127.0.0.1:8090` — set up a [reverse proxy](https://y0f.github.io/Asura/#deployment) to expose it.

### Docker

```bash
git clone https://github.com/y0f/Asura.git && cd Asura
cp config.example.yaml config.yaml
docker compose up -d
```

---

## Documentation

| Topic | |
|---|---|
| [Getting Started](https://y0f.github.io/Asura/#getting-started) | Install via VPS, Docker, or source |
| [Configuration](https://y0f.github.io/Asura/#configuration) | Config reference, auth, adaptive intervals |
| [Monitors](https://y0f.github.io/Asura/#monitors) | 11 protocols, assertions, heartbeats |
| [Notifications](https://y0f.github.io/Asura/#notifications) | 10 channels, webhook signing, per-monitor routing |
| [Deployment](https://y0f.github.io/Asura/#deployment) | Production nginx/caddy setup, TLS |
| [API Reference](https://y0f.github.io/Asura/#api) | All endpoints, fields, pagination |
| [Architecture](https://y0f.github.io/Asura/#architecture) | Pipeline, storage, checker registry |

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## Security

See [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE)
