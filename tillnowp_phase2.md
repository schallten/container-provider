# Phase 2 Retrospective
## Container Provider Build — Lessons Learned & Battle Log

**Date:** 2026-05-09 to 2026-05-11
**Goal:** Go API server with containerd integration, WebSocket terminal, and session lifecycle
**Status:** ✅ COMPLETE (with duct tape)

---

## What We Built

| Component | Implementation | Status |
|-----------|---------------|--------|
| containerd client wrapper | `internal/container/client.go` | ✅ |
| Session store (in-memory) | `internal/store/memory.go` | ✅ |
| REST API (create/list/get/destroy) | `internal/api/handlers.go` | ✅ |
| WebSocket terminal proxy | `internal/api/websocket.go` | ✅ |
| TTL & grace period manager | `internal/container/lifecycle.go` | ✅ |
| Chi router | `internal/api/routes.go` | ✅ |
| Static dashboard (HTML terminal) | `static/terminal.html` | ✅ |

---

## Architecture (Final)

```
User Browser
    ↓ (HTTP/WebSocket via SSH tunnel)
Go API Server (port 8080)
    ↓ (chi router)
    ├─ POST /api/v1/sessions → Create container
    ├─ GET /api/v1/sessions → List sessions
    ├─ GET /api/v1/sessions/{id} → Get session
    ├─ DELETE /api/v1/sessions/{id} → Destroy session
    ├─ GET /api/v1/sessions/{id}/terminal → WebSocket to container bash
    └─ /* → static/terminal.html
    ↓
nerdctl run/exec (no direct containerd gRPC for creation)
    ↓
containerd → runc → Container (sleep infinity)
```

**Key decision:** We abandoned direct containerd gRPC for container creation in favor of `nerdctl run` via `exec.Command`. This handles CNI, image resolution, port forwarding, and cleanup automatically.

---

## Error Log & Mitigations

### Error 1: containerd Image Not Found
**Symptom:** `image vps-base:latest not found: failed to resolve reference "vps-base:latest": parse "dummy://vps-base:latest": invalid port ":latest" after host`

**Root Cause:** containerd's Go client expects fully-qualified image references. `vps-base:latest` is not valid — needs `docker.io/library/vps-base:latest`.

**Mitigation:** Changed `baseImage` constant from `"vps-base:latest"` to `"docker.io/library/vps-base:latest"`.

**File:** `internal/container/client.go`

---

### Error 2: Go 1.22 ServeMux Pattern Matching Failure
**Symptom:** `GET /api/v1/sessions/{id}/terminal` returned `404 page not found` even though handler was registered. Server logs showed request hitting the mux but not the handler.

**Root Cause:** Go 1.22's `http.ServeMux` has inconsistent behavior with wildcard patterns (`{id}`) combined with literal suffixes (`/terminal`). Method-specific patterns (`GET /...`) conflict with wildcard patterns in non-obvious ways.

**Attempted Mitigations:**
1. Removed method prefix from terminal route → still 404
2. Added custom prefix-based handler (`handleTerminalOrSession` with `strings.HasSuffix`) → still 404 because `ServeMux` was catching requests before custom handler
3. **Final Fix:** Switched to `github.com/go-chi/chi/v5` router. `chi.URLParam(r, "id")` works correctly. One dependency, problem solved permanently.

**Files:** `internal/api/routes.go`, `cmd/server/main.go`

---

### Error 3: Container IP Discovery Failure
**Symptom:** `session has no IP` — `nerdctl inspect` returned empty string for IPAddress.

**Root Cause:** CNI bridge network assigns IPs, but `nerdctl inspect` format string or timing was wrong. Also, when using `nerdctl run` with containerd directly, CNI state might not be immediately available.

**Attempted Mitigations:**
1. Increased sleep after container creation → still empty
2. Used `nerdctl ps` to discover IP → worked but added complexity
3. **Final Fix:** Abandoned IP-based routing entirely. Switched to localhost port forwarding (`-p 127.0.0.1:30001:7681`). Each container gets a unique host port. Go API connects to `ws://127.0.0.1:30001` instead of `ws://containerIP:7681`.

**Files:** `internal/container/client.go`, `internal/models/session.go`

---

### Error 4: ttyd WebSocket Server Broken
**Symptom:** `websocat ws://127.0.0.1:30001/ws` connected but returned no output. Typing `ls`, `pwd`, `exit` produced nothing. Browser terminal showed black screen with blinking cursor.

**Root Cause:** ttyd 1.7.7 inside container could not properly allocate a PTY when run as PID 1 without a controlling terminal. Bash detects no TTY and runs in non-interactive mode.

**Attempted Mitigations:**
1. Added `script -q /dev/null` to force PTY → ttyd still broken
2. Checked `nerdctl exec -it` directly → bash worked fine, confirming container was healthy
3. **Final Fix:** Abandoned ttyd entirely. Switched to `nerdctl exec` from Go API. Container runs `sleep infinity` as PID 1. When terminal connects, Go spawns `nerdctl exec -i containerID script -q /dev/null /bin/bash` and pipes stdin/stdout through WebSocket.

**Files:** `internal/container/client.go`, `internal/api/websocket.go`

---

### Error 5: "provided file is not a console"
**Symptom:** `nerdctl exec -it` failed with `level=fatal msg="provided file is not a console"`.

**Root Cause:** `-t` (TTY) flag requires an actual terminal device. When run via Go's `exec.Command`, there's no controlling TTY available.

**Mitigation:** Dropped `-t` flag, kept `-i` (interactive). Used `script -q /dev/null` inside the container to create a pseudo-terminal without needing host TTY allocation.

**File:** `internal/api/websocket.go`

---

### Error 6: Keystrokes Sent Per-Character
**Symptom:** Typing `ls` sent `l` and `s` as separate commands: `sh: 1: l: not found`, `sh: 2: s: not found`.

**Root Cause:** WebSocket `onmessage` handler sent every keystroke immediately to bash stdin. Bash interpreted each character as a separate command.

**Mitigation:** Built a simple line-based terminal UI in HTML/JS. Input is buffered in a text field, sent only when Enter is pressed. Added Ctrl+C/D/Z handling via ``, ``, `` control characters.

**File:** `static/terminal.html`

---

### Error 7: WebSocket Disconnect Not Detected
**Symptom:** Closing browser tab did not mark session as `detached`. TTL grace period never triggered. Session stayed `running` forever.

**Root Cause:** Browser close does not always send a clean WebSocket close frame. Go's `websocket.ReadMessage()` can block indefinitely if the connection drops silently (no TCP RST, no close frame).

**Current State:** NOT FIXED. Manual workaround via `POST /api/v1/sessions/{id}/detach` debug endpoint.

**Impact:** Low for API-only usage (no dashboard). Users will explicitly call DELETE when done. For dashboard usage, sessions leak until 12h TTL hits.

---

## Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| `nerdctl run` via exec instead of containerd gRPC | Handles CNI, image resolution, port forwarding, cleanup automatically |
| Localhost port forwarding instead of container IPs | Avoids CNI discovery bugs, works reliably |
| `nerdctl exec` instead of ttyd | Eliminates in-container WebSocket server, one less failure point |
| Line-based terminal instead of xterm.js | Avoids binary frame handling complexity, control character issues |
| Chi router instead of stdlib ServeMux | Eliminates pattern matching bugs, cleaner code |
| In-memory store | Acceptable for 3 users, ephemeral sessions, no persistence requirement |

---

## Current Limitations

1. **WebSocket disconnect detection:** Browser tab close does not trigger grace period. Sessions leak until 12h TTL or manual DELETE.
2. **No terminal resize:** Fixed 80x24 terminal, no dynamic resizing.
3. **No scrollback:** Terminal output is not persisted in browser.
4. **Line-based input only:** No vim, nano, or interactive programs that need raw terminal mode.
5. **No file upload/download:** Git-only for file transfer.
6. **No Cloudflare Tunnel:** Public URLs not implemented yet.
7. **No authentication:** Anyone with API access can create/destroy sessions.

---

## Files Modified

```
vps-provider/
├── cmd/server/main.go              # Chi router integration
├── internal/api/handlers.go        # REST handlers + debug detach endpoint
├── internal/api/routes.go          # Chi route definitions
├── internal/api/websocket.go       # WebSocket proxy via nerdctl exec
├── internal/api/middleware.go      # Logging, recovery
├── internal/container/client.go    # nerdctl run/exec wrapper
├── internal/container/lifecycle.go # Session lifecycle + TTL tracker
├── internal/models/session.go      # Session struct (HostPort removed)
├── internal/store/memory.go        # Thread-safe session store
├── static/terminal.html            # Line-based terminal UI
└── go.mod                          # Added chi dependency
```

---

## Testing Commands

```bash
# Build
 cd ~/vps-provider/container-provider
 go build -o bin/vps-server cmd/server/main.go
 sudo ./bin/vps-server

# Create session
 curl -X POST http://localhost:8080/api/v1/sessions

# List sessions
 curl http://localhost:8080/api/v1/sessions

# Get session
 curl http://localhost:8080/api/v1/sessions/SESSION_ID

# Destroy session
 curl -X DELETE http://localhost:8080/api/v1/sessions/SESSION_ID

# Mark detached (debug)
 curl -X POST http://localhost:8080/api/v1/sessions/SESSION_ID/detach

# Open terminal
 # Browser: http://localhost:8080/terminal.html?id=SESSION_ID
```

---

## Next Phase: Cloudflare Tunnel

**Goal:** Expose container ports (e.g., Flask on 8080) to public URLs via Cloudflare Tunnel.

**Approach:**
1. Install `cloudflared` on host
2. Create named tunnel
3. Dynamic ingress config: `subdomain.containers.yourdomain.com` → `127.0.0.1:containerHostPort`
4. API endpoint: `POST /api/v1/sessions/{id}/ports` to expose a port

**Blockers:** None. Ready to proceed.

---

## Duct Tape Inventory

| # | Location | Description |
|---|----------|-------------|
| 1 | `client.go` | `nerdctl run` via exec instead of containerd gRPC |
| 2 | `client.go` | `sleep infinity` as PID 1 instead of proper init |
| 3 | `websocket.go` | `nerdctl exec` spawned per connection instead of persistent ttyd |
| 4 | `websocket.go` | `script -q /dev/null` to fake PTY |
| 5 | `terminal.html` | Line-based input instead of full terminal emulation |
| 6 | `lifecycle.go` | `time.Sleep(500 * time.Millisecond)` to wait for container readiness |
| 7 | `lifecycle.go` | Hardcoded port counter starting at 30000 |
| 8 | `lifecycle.go` | Manual `/detach` debug endpoint for testing TTL |

---

*Document generated for future reference. Do not delete.*
