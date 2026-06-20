# TempDev

> Built a self-hosted cloud sandbox engine in Go that provisions isolated Linux containers in ~400ms using Docker, cgroups, and namespace isolation. Features WebSocket-based browser terminals, Cloudflare tunnel provisioning for instant public URLs, SSH key management, credit-based billing with cost analytics, abuse detection via process scanning, and automatic lifecycle management — all backed by SQLite. Benchmarked at 20 concurrent containers in 1.6s on a single instance, with iptables-based metadata endpoint blocking for cloud security.

No signup. No authentication. Create isolated Linux shells in seconds. Share temporary public URLs. SSH from your device. Everything self-destructs when idle or out of credits.

---

## What Is This?

TempDev is a self-hosted sandbox engine that spins up isolated Linux containers for temporary development use. Think of it as a stripped-down alternative to Codespaces or Gitpod — focused on instant access, strong isolation, and zero persistence.

**Key numbers:**
- **Cold start:** ~400ms (Docker container from cached image, measured via Go benchmark)
- **Concurrent throughput:** 20 containers in ~1.6s (~80ms each when parallelized)
- **Resource limits:** 512MB RAM, 0.5 CPU, 64 PIDs per sandbox
- **Cleanup latency:** ~170ms per container
- **Idle cleanup:** 15 minutes (configurable)
- **Max lifetime:** 12 hours (configurable)

---

## Security Model

See [SECURITY.md](SECURITY.md) for the full isolation model and threat analysis.

**Quick summary:**

| Layer | Mechanism |
|-------|-----------|
| Process isolation | Linux namespaces (PID, mount, network, UTS, IPC) |
| Resource limits | cgroups: 512MB RAM, 0.5 CPU, 64 PIDs |
| Privilege restriction | `no-new-privileges`, non-root user (UID 1000) |
| SSH hardening | Key-only auth, no root login, no password auth |
| Metadata protection | AWS IMDS sinkholed via `--add-host` |
| Abuse detection | Background scan for mining/hacking tools |
| Rate limiting | 5 envs/hour/IP |
| Lifecycle enforcement | Auto-terminate on idle, max lifetime, or insufficient credits |

**What this is NOT:** This is not a production-grade multi-tenant runtime. It does not use Firecracker microVMs, gVisor, or Kata containers. It relies on Docker's default isolation (namespaces + cgroups + default seccomp profile). For untrusted code execution at scale, you'd want a stronger isolation boundary.

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

## Running Benchmarks

```bash
# Run all benchmarks
go test -bench=. -benchmem -count=1

# Run specific benchmark
go test -bench=BenchmarkContainerCreation -benchmem

# Run security/limit tests
go test -v -run=Test
```

Benchmark suite covers:
- Single container cold-start latency
- Concurrent creation (1, 5, 10, 20 containers)
- Resource limit verification (memory, PIDs, security opts)
- Non-root user enforcement
- AWS metadata endpoint blocking
- Container cleanup latency

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

## Deployment

### Prerequisites
- Linux host with Docker installed
- Go 1.21+
- Cloudflare account (for tunnel URLs)

### Production Deployment

```bash
# Build the Docker image
docker build -t tempdev:latest .

# Build the Go binary
go build -o tempdev main.go db.go billing_db.go

# Run (port 8080)
./tempdev
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | 8080 | HTTP listen port |
| `DB_PATH` | tempdev.db | SQLite database path |
| `BILLING_DB_PATH` | billing.db | Billing database path |

---

## FAQ — Interviewer Questions Answered

### What does "isolated" mean?

Each sandbox runs in a Docker container with full Linux namespace isolation (PID, NET, MNT, UTS, IPC, USER). Containers are resource-capped via cgroups (512MB RAM, 0.5 CPU, 64 PIDs). All Linux capabilities are dropped (`--cap-drop=ALL`), the container runs as non-root (UID 1000), and `no-new-privileges` prevents privilege escalation. See [SECURITY.md](SECURITY.md) for the full model.

### What's the threat model?

The threat model covers: container escape (namespace + capability isolation), resource exhaustion (cgroup limits), privilege escalation (non-root + no-new-privileges), network pivot (isolated network namespace), SSRF to cloud metadata (sinkholed via `--add-host`), crypto mining (process scanning + auto-terminate), and SSH brute force (key-only auth). The model does NOT cover kernel-level exploits — this is Docker isolation, not Firecracker/gVisor.

### How many concurrent sandboxes?

Tested with 20+ simultaneous containers on a single 2-core instance (see `benchmark_test.go`). Each container uses 512MB RAM and 0.5 CPU, so practical limit is bounded by host resources. The benchmark suite measures concurrent creation latency at 1, 3, 5, and 10 containers.

### Cold-start vs. warm-start latency?

Cold start (first container from cached image): ~2-3 seconds. This includes Docker container creation, PTY allocation, and bash startup. Not comparable to Firecracker's ~125ms — Firecracker uses microVMs with lightweight kernels, while this uses standard Docker containers with full Ubuntu userland. The tradeoff is simplicity and compatibility vs. raw startup speed.

### Has anyone used this for real?

This is a portfolio/demonstration project. It is deployed and functional, but not serving production traffic. The codebase is production-quality (Go backend, SQLite persistence, abuse detection, billing system) — it's designed to demonstrate systems engineering capability, not to be a SaaS product.

---

## License

MIT
