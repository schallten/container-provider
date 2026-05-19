# Container Provider

![Container Ship](https://i.pinimg.com/1200x/79/fa/10/79fa10d0c3f47fa15351ab187b937e98.jpg)

Lightweight temporary cloud development environments on demand.

No signup. No authentication. Create isolated Linux shells in seconds. Share temporary public URLs. Everything self-destructs in 15 minutes.

---

## Features

* **Instant Isolated Environments**
  * Click "New" to get a full Linux shell in less than 3 seconds.
  * 512MB RAM, 0.5 CPU per environment.
  * Automatic cleanup after 15 minutes idle or 12 hours maximum lifetime.

* **Public URLs via Cloudflare Tunnel**
  * Expose any port inside your environment.
  * Get a public HTTPS URL instantly.
  * Share with anyone, anywhere.
  * Stop active tunnels dynamically via the UI or API.

* **Browser Terminal**
  * Full xterm.js integration with FitAddon.
  * Real terminal emulation (not a fake console) with window resize synchronization.
  * Keyboard shortcuts, paste, and copy support.

* **Secure Isolation**
  * Docker containers with dropped capabilities (`--cap-drop=ALL`).
  * Non-root user execution (`-u dev`).
  * No host filesystem access.
  * Rate limited to 5 environments per hour per IP.

* **Minimal Overhead**
  * About 120MB memory per idle environment.
  * No database (in-memory state).
  * No external dependencies.
  * Runs on AWS free tier t3.micro.

---

## Architecture

```
Browser (xterm.js)
       ↓
   HTTPS/WSS
       ↓
┌─────────────────────┐
│   Go Backend        │
│  • Environment mgr  │
│  • WS proxy         │
│  • Cleanup loop     │
│  • Tunnel provisioner│
│  • Abuse detection  │
└────────────┬────────┘
             ↓
      Docker Containers
      (512MB, 0.5CPU)
      • Python 3
      • Node.js
      • Go toolchain
      • Git
      • Interactive Bash (via Go PTY proxy)
      • cloudflared (tunneling)
```

---

## Quick Start

For detailed step-by-step local setup and cloud deployment instructions, see [quickstart.md](quickstart.md).

### Local Run
```bash
# Clone the repository
git clone https://github.com/schallten/container-provider.git && cd container-provider

# Build docker sandbox image & run application
docker build -t tempdev:latest .
go mod download
go run main.go

# Open http://localhost:8080
```

---

## Technical Documentation & API Reference

Detailed specifications of endpoints, security mechanisms, PTY window resizing WebSocket sub-protocol, monitoring logs, and troubleshooting steps have been moved to:

👉 **[documentation.md](documentation.md)**

For step-by-step instructions on exposing the provider or sandbox environments publicly using Cloudflare tunnels, ngrok, or reverse proxies, see:

👉 **[expose_guide.md](expose_guide.md)**

---

## Included Tools

Every sandbox environment comes pre-installed with:
* **Languages**: Python 3 (with pip/venv), Node.js (with npm), Go toolchain.
* **CLI Tools**: Git, curl, wget, htop, tmux, tree, ssh-client.
* **Editors**: Vim, Nano.
* **Tunneling**: cloudflared.

---

## Security Highlights

Container Provider runs hardened sandboxes to prevent host compromise and resource abuse:
* **Isolation**: Containers run with `--cap-drop=ALL` (no capabilities), `--security-opt=no-new-privileges`, and under a non-root `dev` user.
* **Limits**: Tight boundaries are enforced per sandbox (512MB memory, 0.5 CPU cores, max 64 processes).
* **Abuse Scan**: Host background daemon actively kills containers spawning unauthorized network scanner or cryptocurrency mining binaries (e.g. `xmrig`, `nmap`).
* **Metadata Protection**: The cloud metadata endpoint (`169.254.169.254`) is mapped to an invalid loopback address within the container to block access.

See [documentation.md](documentation.md#security-model) for a comprehensive deep dive.

---

## Costs

| Component | Cost |
|-----------|------|
| t3.micro (1 year free) | $0-10/month |
| 30GB EBS storage | ~$3/month |
| Data transfer | Free tier |
| Domain / Tunnel | Free |
| **Total** | ~$3/month |


---

## License

MIT