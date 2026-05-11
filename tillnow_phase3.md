# Phase 3 Retrospective
## Cloudflare Tunnel Integration — Public URLs for Container Apps

**Date:** 2026-05-11
**Goal:** Expose container ports (e.g., Flask, Python HTTP server) to public URLs via Cloudflare Tunnel
**Status:** ✅ COMPLETE

---

## What We Built

| Component | Implementation | Status |
|-----------|---------------|--------|
| cloudflared integration | `internal/container/tunnel.go` | ✅ |
| Dynamic tunnel spawning | `nerdctl exec` + `cloudflared tunnel --url` inside container | ✅ |
| Port expose API | `POST /api/v1/sessions/{id}/ports` | ✅ |
| Tunnel cleanup | `StopTunnel()` on session destroy | ✅ |
| URL parsing from stdout | Regex + scanner on cloudflared output | ✅ |

---

## Architecture (Final)

```
User Browser / curl
    ↓
Cloudflare Edge Network
    ↓
cloudflared (running INSIDE container)
    ↓
localhost:8080 inside container
    ↓
Python http.server / Flask app
```

**Key decision:** Run `cloudflared` INSIDE the container, not on the host. This eliminates port forwarding, NAT, and CNI complexity entirely.

---

## Error Log & Mitigations

### Error 1: cloudflared Dies Immediately After Starting
**Symptom:** `[INFO] tunnel exited for sess-...` appears seconds after `[INFO] tunnel started`. Cloudflare URL returns Error 1033 (tunnel unreachable).

**Root Cause:** `StartTunnel` used `r.Context()` from the HTTP handler. When the `curl` request completed, the context cancelled, killing the `cloudflared` process.

**Attempted Mitigations:**
1. Tried `context.Background()` — still died because `nerdctl exec` process management was wrong
2. Tried `cmd.Wait()` goroutine — process exited because stdout pipe closed
3. **Final Fix:** Complete rewrite of `tunnel.go`. Used `cmd.StdoutPipe()`, started process with `cmd.Start()`, read output via `bufio.Scanner` in a goroutine, and used `cmd.Wait()` in a separate goroutine. No request context dependency. Process survives HTTP response.

**File:** `internal/container/tunnel.go`

---

### Error 2: Read-Only Filesystem Blocks cloudflared Copy
**Symptom:** `nerdctl cp` failed with `cannot copy into read-only location`. Container was created with `--read-only` flag.

**Root Cause:** Phase 2 containers used `--read-only` rootfs for security. `nerdctl cp` requires a writable layer.

**Mitigation:** Removed `--read-only` from container creation. Containers now have writable rootfs, allowing `nerdctl cp` to copy `cloudflared` binary into `/tmp/cloudflared`.

**File:** `internal/container/client.go`

---

### Error 3: cloudflared Binary Not Found Inside Container
**Symptom:** `exec failed: unable to start container process: exec: "/tmp/cloudflared": stat /tmp/cloudflared: no such file or directory`

**Root Cause:** Bind-mount (`-v /usr/local/bin/cloudflared:/usr/local/bin/cloudflared:ro`) didn't work inside container. The mount either failed or the binary wasn't executable in the container's environment.

**Attempted Mitigations:**
1. Tried bind-mount at container creation — failed silently
2. Tried different mount paths — same issue
3. **Final Fix:** Abandoned bind-mounts entirely. Instead, copy `cloudflared` from host to container at runtime using `nerdctl cp`, then `chmod +x`, then `nerdctl exec` to run it. This is slower but 100% reliable.

**File:** `internal/container/tunnel.go`

---

### Error 4: Port Conflict Confusion
**Symptom:** User asked "how can two things exist on same port 8080 twice?"

**Root Cause:** Confusion between host port 8080, container port 8080, and SSH tunnel port 8080. Each exists in a different network namespace.

**Clarification:** 
- Container has its own `localhost:8080` (isolated)
- Host (Debian) has its own `localhost:8080` (different)
- SSH tunnel forwards Void:8080 → Debian:8080 (yet another layer)
- `cloudflared` inside container connects to container's `localhost:8080` directly

**No mitigation needed** — this was a conceptual clarification, not a bug.

---

### Error 5: Trailing Space in URL Response
**Symptom:** JSON response showed `"public_url":"https://...trycloudflare.com "` with trailing space.

**Root Cause:** Regex captured the URL from cloudflared output which included padding spaces from the terminal table format.

**Mitigation:** Added `strings.TrimSpace(url)` before returning the URL in `TunnelInfo`.

**File:** `internal/container/tunnel.go`

---

## Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| cloudflared INSIDE container | Eliminates host port forwarding, NAT, CNI dependencies |
| `nerdctl cp` at runtime | Avoids bind-mount complexity, works with any base image |
| Process survives HTTP response | `cmd.Start()` + background goroutine, no request context |
| Regex stdout parsing | cloudflared has no API for URL retrieval, stdout is only option |
| Quick tunnels (trycloudflare.com) | No domain, no DNS, no account needed for testing |

---

## Current Limitations

1. **No named tunnels:** Quick tunnels are ephemeral (random URLs). For production, need named tunnels with custom domain.
2. **cloudflared copied per expose:** ~2-3 second delay to copy binary into container. Could optimize by including in base image.
3. **No tunnel persistence:** If container restarts, tunnel dies. Acceptable for ephemeral sessions.
4. **No multiple ports per session:** Current API exposes one port per call. Could extend to support multiple.
5. **URL parsing fragile:** Depends on cloudflared output format. If Cloudflare changes it, regex breaks.

---

## Testing Commands

```bash
# Build
 cd ~/vps-provider/container-provider
 go build -o bin/vps-server cmd/server/main.go
 sudo ./bin/vps-server

# Create session
 curl -X POST http://localhost:8080/api/v1/sessions

# Open terminal in browser
 # http://localhost:8080/terminal.html?id=SESSION_ID
 # Inside container: python3 -m http.server 8080 &

# Expose port
 curl -X POST http://localhost:8080/api/v1/sessions/SESSION_ID/ports    -H "Content-Type: application/json"    -d '{"container_port": 8080}'

# Visit returned URL in browser
 # https://xxxx.trycloudflare.com
```

---

## Full System Architecture (Phases 1-3)

```
┌─────────────────────────────────────────────────────────────┐
│                        USER BROWSER                          │
│  ┌──────────────┐  ┌──────────────┐  ┌─────────────────────┐│
│  │   Terminal   │  │   Exposed    │  │   Dashboard (temp)  ││
│  │  (line-based)│  │     App      │  │   (terminal.html)   ││
│  └──────┬───────┘  └──────┬───────┘  └──────────┬──────────┘│
└─────────┼─────────────────┼─────────────────────┼────────────┘
          │                 │                     │
          ▼                 ▼                     ▼
┌─────────────────────────────────────────────────────────────┐
│                      GO API SERVER                           │
│  ┌──────────────┐  ┌──────────────┐  ┌─────────────────────┐│
│  │  REST API    │  │  WS Proxy    │  │  Cloudflared        ││
│  │  (Session    │  │  (nerdctl    │  │  Spawner           ││
│  │   Lifecycle) │  │   exec bash) │  │  (nerdctl exec)    ││
│  └──────┬───────┘  └──────┬───────┘  └──────────┬──────────┘│
│         │                 │                     │            │
│  ┌──────┴─────────────────┴─────────────────────┴──────┐   │
│  │              containerd (via nerdctl)                  │   │
│  └──────────────────────┬───────────────────────────────┘   │
└─────────────────────────┼───────────────────────────────────┘
                          │
┌─────────────────────────┼───────────────────────────────────┐
│                    CONTAINERS                                │
│  ┌──────────────────────┴───────────────────────────────┐  │
│  │  Container A (sess-xxx)                                │  │
│  │  ├─ sleep infinity (PID 1)                              │  │
│  │  ├─ /tmp/cloudflared tunnel --url http://127.0.0.1:8080 │  │
│  │  └─ python3 -m http.server 8080 (user app)             │  │
│  │                                                         │  │
│  │  Container B (sess-yyy)                                │  │
│  │  ├─ sleep infinity                                      │  │
│  │  └─ /tmp/cloudflared tunnel --url http://127.0.0.1:3000 │  │
│  └─────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

---

## Duct Tape Inventory (All Phases)

| # | Location | Description |
|---|----------|-------------|
| 1 | `client.go` | `nerdctl run` via exec instead of containerd gRPC |
| 2 | `client.go` | `sleep infinity` as PID 1 instead of proper init |
| 3 | `websocket.go` | `nerdctl exec` spawned per connection instead of persistent ttyd |
| 4 | `websocket.go` | `script -q /dev/null` to fake PTY for bash |
| 5 | `terminal.html` | Line-based input instead of xterm.js |
| 6 | `lifecycle.go` | `time.Sleep(500ms)` to wait for container readiness |
| 7 | `tunnel.go` | `nerdctl cp` + `chmod` + `exec` for cloudflared |
| 8 | `tunnel.go` | Regex stdout parsing for URL extraction |
| 9 | `tunnel.go` | `cmd.Start()` + goroutine to keep process alive |
| 10 | `client.go` | Removed `--read-only` to allow file copies |

---

## Next Phase: Production Hardening (Phase 7)

**Goal:** Ready for AWS deployment

**Tasks:**
- Systemd service files
- Security: seccomp, capabilities review
- Cloudflare named tunnels (custom domain)
- Monitoring/logging cleanup
- Session limit per user

**Blockers:** None. Ready to proceed.

---

*Document generated for future reference. Do not delete.*
