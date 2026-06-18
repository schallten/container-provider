# TempDev — Technical Documentation & API Reference

---

## API Endpoints

Base URL: `http://localhost:8080`

### Environments

#### Create Environment
`POST /env`

Rate limited: 5 per hour per IP. Requires minimum 10 credits.

```json
Response: { "id": "a1b2c3d4", "ws_url": "/ws/env/a1b2c3d4" }
```

#### List Environments
`GET /envs`

```json
[{
  "id": "a1b2c3d4",
  "created_at": "2026-06-18T10:00:00Z",
  "last_ping": "2026-06-18T10:05:00Z",
  "uptime": "5m",
  "idle": "0s",
  "tunnel_url": "https://xxx.trycloudflare.com",
  "tags": {"project": "demo"},
  "ssh_port": 22000
}]
```

#### Get Environment
`GET /env/:id`

#### Delete Environment
`DELETE /env/:id`

Kills container, cleans up SSH keys and tunnels.

---

### Shell (WebSocket)

`WS /ws/env/:id`

* **Client → Server**: Binary (stdin) or JSON `{"cols":120,"rows":30}` for resize
* **Server → Client**: Binary (stdout/stderr)

---

### Port Exposure

#### Expose Port
`POST /expose/:id/:port`

Uses `cloudflared tunnel --protocol http2` inside the container. Returns a `trycloudflare.com` HTTPS URL.

```json
{
  "tunnel_url": "https://xxx.trycloudflare.com",
  "port": "8000",
  "note": "Cloudflared generates a unique HTTPS URL."
}
```

#### Stop Tunnel
`DELETE /expose/:id`

---

### SSH

#### Download SSH Key
`GET /ssh/:id`

Returns the private key as a `.pem` file download.

```bash
# After downloading:
chmod 600 tempdev-*.pem
ssh dev@localhost -p 22000 -i tempdev-*.pem
```

SSH keys are ED25519, generated per environment. Public key is injected into the container's `dev` user `authorized_keys`.

---

### Tags

#### Get Tags
`GET /tags/:id`

```json
{"project": "frontend", "env": "staging"}
```

#### Set Tags
`POST /tags/:id`

```json
{"project": "frontend", "env": "staging", "team": "backend"}
```

---

### Settings

#### Get Settings
`GET /settings/:id/get`

```json
{"no_idle_timeout": false, "no_max_lifetime": false}
```

#### Toggle Idle Timeout
`POST /settings/:id/no-idle-timeout`

```json
{"enabled": true}
```

#### Toggle Max Lifetime Bypass
`POST /settings/:id/no-max-lifetime`

```json
{"enabled": true}
```

---

### Billing

#### Get Balance & Transactions
`GET /billing`

```json
{
  "user_id": "user-default",
  "balance": 950,
  "transactions": [
    {"type": "deduction", "amount": -5, "description": "Environment created — a1b2c3d4", "created_at": "..."}
  ]
}
```

#### Top Up Credits
`POST /billing/topup`

```json
{
  "amount": 500,
  "card_number": "4242424242424242",
  "expiry": "12/28",
  "cvv": "123"
}
```

~25% random bank decline rate for realism. Accepts cards starting with 4, 5, 6, or 3.

#### Get Cost Breakdown
`GET /billing/costs?days=7`

```json
{
  "by_day": [{"date": "2026-06-18", "credits": 45}],
  "by_env": [{"env_id": "a1b2c3d4", "action": "Container runtime (1 min)", "credits": 30}]
}
```

#### Get Usage History
`GET /billing/usage`

---

### Logs

`GET /logs?filter=tunnel&limit=100`

```json
[{
  "id": 1,
  "level": "info",
  "event": "tunnel_created",
  "env_id": "a1b2c3d4",
  "detail": "8000",
  "created_at": "2026-06-18T10:00:00Z"
}]
```

---

### Location

`GET /location`

Returns geolocation from `ipapi.co`, cached in SQLite for 1 hour.

```json
{
  "city": "Santa Cruz",
  "region_code": "MH",
  "org": "Cloudflare, Inc."
}
```

Response header `X-Cache: HIT` or `MISS` indicates cache status.

---

## Databases

### tempdev.db (WAL mode)
| Table | Purpose |
|-------|---------|
| `envs` | Active environments, tags, settings |
| `logs` | Event audit trail |
| `cache` | Location cache with TTL |

### billing.db (WAL mode)
| Table | Purpose |
|-------|---------|
| `users` | Account balances |
| `transactions` | Top-ups and deductions |
| `usage` | Per-env credit consumption |

---

## Docker Container

* **Base**: Ubuntu 22.04
* **Entry**: `entrypoint.sh` (generates SSH host keys, starts sshd, sleeps)
* **SSH**: Port 2222 (mapped to random host port 22000+)
* **Resource limits**: 512MB RAM, 0.5 CPU, 64 PIDs
* **Security**: `--security-opt=no-new-privileges`, AWS metadata blocked

---

## Cleanup Loop

Runs every 60 seconds:
1. Kills envs older than 12 hours (unless `no_max_lifetime` is set)
2. Kills idle envs > 15 minutes (unless `no_idle_timeout` is set)
3. Deducts 3 credits/min per running env
4. Kills envs with insufficient credits

Orphan cleanup on startup: checks each DB env's container via `docker inspect`, removes dead ones.

---

## Project Structure

```
├── main.go              # HTTP handlers, WS proxy, cleanup loop
├── db.go                # SQLite env/log/cache operations
├── billing_db.go        # SQLite billing operations
├── Dockerfile           # Container image (Ubuntu + Python + SSH + cloudflared)
├── entrypoint.sh        # Container entrypoint (sshd + sleep)
├── go.mod / go.sum      # Go dependencies
├── tempdev.db           # Env state database (auto-created)
├── billing.db           # Billing database (auto-created)
├── ssh_keys/            # Generated SSH keypairs (auto-created)
├── public/
│   ├── index.html       # Redirector to dashboard
│   ├── css/             # Shared stylesheets
│   ├── js/              # Shared JS (nav, shell, location)
│   └── pages/           # Dashboard, compute, tempdev, billing, logs, costs
├── README.md
├── documentation.md
└── quickstart.md
```
