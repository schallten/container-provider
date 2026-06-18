# TempDev

Lightweight temporary cloud development environments on demand.

No signup. No authentication. Create isolated Linux shells in seconds. Share temporary public URLs. SSH from your device. Everything self-destructs when idle or out of credits.

---

## Features

* **Console Dashboard** — AWS-style multi-page UI with sidebar navigation, stats, and env management
* **Instant Isolated Environments** — Click Launch to get a full Linux shell in seconds (512MB RAM, 0.5 CPU)
* **Browser Terminal** — Real xterm.js terminal with resize support, not a fake console
* **SSH Access** — Download an SSH key and connect from your own terminal
* **Public URLs via Cloudflare Tunnel** — Expose any port, get a public HTTPS URL instantly
* **Env Tagging** — Tag environments with key/value pairs (project, env, team, etc.)
* **Billing System** — Credit-based system with fake payment gateway, per-minute billing, and cost explorer
* **Event Logging** — Full audit trail of all actions with searchable log viewer
* **Settings** — Per-env idle timeout and max lifetime bypass toggles
* **Orphan Cleanup** — SQLite-backed state survives server restarts, cleans up dead containers on boot

---

## Architecture

```
Browser (xterm.js / console UI)
       ↓
   HTTPS/WSS
       ↓
┌──────────────────────────┐
│   Go Backend            │
│  • Environment mgr      │
│  • WS shell proxy       │
│  • Billing engine       │
│  • SSH key generation   │
│  • Cleanup loop         │
│  • Abuse detection      │
│  • Event logging        │
├──────────────────────────┤
│  SQLite (tempdev.db)    │
│  SQLite (billing.db)    │
└────────────┬────────────┘
             ↓
      Docker Containers
      (512MB, 0.5CPU)
      • Python 3
      • Git, curl, wget
      • SSH server (port 2222)
      • cloudflared (tunneling)
      • Interactive Bash (via Go PTY)
```

---

## Quick Start

```bash
# Clone
git clone <repo-url> && cd container-provider

# Build Docker image
docker build -t tempdev:latest .

# Build and run
go build -o tempdev main.go db.go billing_db.go
./tempdev

# Open http://localhost:8080
```

New accounts start with **1,000 free credits**.

---

## Included Tools

Every sandbox environment comes pre-installed with:
* **Languages**: Python 3 (with pip/venv)
* **CLI Tools**: Git, curl, wget, htop, tmux, tree
* **Editors**: Vim, Nano
* **Tunneling**: cloudflared
* **SSH**: OpenSSH server (port 2222)

---

## Billing

| Action | Cost |
|--------|------|
| New environment | 5 credits |
| Port tunnel | 2 credits |
| Container runtime | 3 credits / min |
| New account | +1,000 free |

Payments are processed through a fake gateway (~25% random bank decline rate for realism).

---

## Pages

| Page | Description |
|------|-------------|
| `/pages/dashboard.html` | Overview with active envs, stats, kill buttons |
| `/pages/compute.html` | Compute services listing |
| `/pages/tempdev.html` | Terminal, SSH, tags, tunnels, settings |
| `/pages/billing.html` | Credits, top-up, transaction history |
| `/pages/logs.html` | Searchable event log viewer |
| `/pages/costs.html` | Spending charts and breakdowns |

---

## License

MIT
