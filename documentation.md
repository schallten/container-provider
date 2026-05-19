# Container Provider - Technical Documentation & API Reference

This document provides a comprehensive technical overview, API specification, security model, and troubleshooting guide for Container Provider.

For installation instructions, see [quickstart.md](quickstart.md).
For network exposing instructions, see [expose_guide.md](expose_guide.md).

---

## Detailed Usage Reference

### Running Commands
Once inside the browser terminal, any standard Linux command works. Pre-installed tools can be used for typical development workflows:

```bash
# Verify runtimes
python3 --version
node --version
go version

# Install dependencies
pip install flask
npm install express

# Start a local web server
python3 -m http.server 8000
```

### Exposing Ports Publicly
To expose a port inside your temporary environment:
1. Start your server inside the browser terminal (e.g., `python3 -m http.server 8000`).
2. Click the **Expose** button in the Web UI.
3. Enter the port number (e.g. `8000`).
4. The backend will invoke `cloudflared` inside the container and return a public HTTPS URL (e.g., `https://tempdev-abc123-8000.trycloudflare.com`).
5. Click **Stop** next to the URL to kill the tunnel.

### Checking Status
The status dashboard is hosted at:
```
http://yourserver/status.html
```
It queries `/envs` and displays active environments, uptime, idle counters, and public URLs in a central table.

---

## API Reference

The backend exposes a REST API on port `8080` (or whichever port is configured).

### 1. Create Environment
* **Method**: `POST`
* **Path**: `/env`
* **Rate Limit**: Max 5 creations per hour per IP.
* **Success Response**: `200 OK`
  ```json
  {
    "id": "abc123",
    "ws_url": "/ws/env/abc123"
  }
  ```

### 2. Get Environment Status
* **Method**: `GET`
* **Path**: `/env/:id`
* **Success Response**: `200 OK`
  ```json
  {
    "id": "abc123",
    "created_at": "2026-05-19T10:30:00Z",
    "last_ping": "2026-05-19T10:35:12Z",
    "tunnel_url": "https://tempdev-abc123-8000.trycloudflare.com"
  }
  ```

### 3. Delete Environment
* **Method**: `DELETE`
* **Path**: `/env/:id`
* **Success Response**: `204 No Content`

### 4. List All Environments
* **Method**: `GET`
* **Path**: `/envs`
* **Success Response**: `200 OK`
  ```json
  [
    {
      "id": "abc123",
      "created_at": "2026-05-19T10:30:00Z",
      "last_ping": "2026-05-19T10:35:12Z",
      "uptime": "5m30s",
      "idle": "2m10s",
      "tunnel_url": "https://tempdev-abc123-8000.trycloudflare.com"
    }
  ]
  ```

### 5. Expose Port
* **Method**: `POST`
* **Path**: `/expose/:id/:port`
* **Success Response**: `200 OK`
  ```json
  {
    "tunnel_url": "https://tempdev-abc123-8000.trycloudflare.com",
    "port": "8000",
    "note": "Cloudflared generates a unique HTTPS URL. If not shown, wait 10-15 seconds and refresh.",
    "how_to": "Inside terminal, run: cloudflared tunnel --url http://localhost:PORT to see the URL"
  }
  ```

### 6. Stop Expose Tunnel
* **Method**: `DELETE`
* **Path**: `/expose/:id`
* **Success Response**: `204 No Content`

### 7. Interactive Shell (WebSocket)
* **Protocol**: `WS` / `WSS`
* **Path**: `/ws/env/:id`
* **Sub-protocols**:
  * **Outbound (Container -> Client)**: Binary messages containing the stdout/stderr stream from the PTY shell.
  * **Inbound (Client -> Container)**:
    * Binary messages containing stdin streams (keys pressed).
    * JSON messages to synchronize PTY window resizes:
      ```json
      {
        "cols": 120,
        "rows": 30
      }
      ```

---

## Security Model

Container Provider uses layered defense mechanisms to prevent resource abuse and secure the host system.

### Container Hardening
Containers are launched with restrictive Docker parameters to limit privileges and resources:
* `--cap-drop=ALL`: Drops all default Linux capabilities.
* `--security-opt=no-new-privileges`: Disables capability/privilege escalation (preventing `setuid` binaries).
* `-u dev`: Executes shell and services as a restricted `dev` user (UID 1000).
* `--memory=512m` & `--memory-swap=512m`: Caps RAM consumption and prevents memory expansion.
* `--cpus=0.5`: Restricts CPU usage to half a core.
* `--pids-limit=64`: Limits the maximum number of concurrent processes to prevent fork bombs.

### Host Protection & Network Isolation
* **Network Isolation**: Docker containers are deployed on an isolated bridge network with no direct host port mappings. Expose tunnels are established from inside the container out to Cloudflare.
* **AWS Metadata Block**: The AWS metadata server (`169.254.169.254`) is mapped to an invalid loopback address within the container using `--add-host` to prevent credentials harvesting.
* **IP Rate Limiting**: An in-memory rate limiter limits container creation to 5 environments per hour per IP.
* **Abuse Process Detection**: A background routine checks running processes inside each container every 3 minutes. If a blacklisted process signature is detected, the container is destroyed immediately.
  * *Blacklisted keywords*: `xmrig`, `miner`, `stratum`, `masscan`, `nmap`, `zmap`, `hydra`, `john`, `hashcat`, `nikto`, `sqlmap`, `metasploit`, `aircrack`, `airmon`, `bettercap`.

---

## Performance & Capacity

### Resource Profile (Per Environment)
* **Memory Limits**: 512MB RAM maximum.
* **Idle RAM Usage**: ~120MB (Go proxy, bash session, and cloudflared tunnel client).
* **CPU limits**: 0.5 cores maximum.
* **Idle CPU Usage**: <5%.
* **Average Startup**: 2–3 seconds from click to PTY interaction.

### Server Capacity Estimates (t3.micro)
* **RAM (1GB)**: Safely supports 4–6 concurrent active environments.
* **Storage (30GB)**: Ephemeral workspaces reside in container overlay storage.
* **Data Transfer**: Managed within AWS/Cloudflare Free Tier limits.

---

## Monitoring & Logs

### 1. Application Logs
Service logs are piped to `systemd` (if using the systemd installer):
```bash
sudo journalctl -u tempdev -f -n 100
```

### 2. Security Audits
Lifecycle activities and abuse detections are written to `events.log` in JSON format:
```bash
tail -f events.log
```
Example event log format:
```json
{"timestamp":"2026-05-19T10:30:00Z","event":"env_created","env_id":"abc123","detail":"a1b2c3d4e5f6"}
{"timestamp":"2026-05-19T10:33:00Z","event":"abuse_detected","env_id":"abc123","detail":"xmrig"}
```

### 3. Container Logs
Inspect stdout/stderr from a specific environment:
```bash
docker logs <container-id>
```

---

## Troubleshooting

### Terminal does not appear
1. Open the browser inspector (F12) and check the **Console** for JavaScript errors.
2. Check the **Network** tab to ensure the WebSocket connection (`/ws/env/:id`) upgrades successfully.
3. Check application logs on the host to verify if the container successfully started: `sudo journalctl -u tempdev -f`.
4. Perform a hard refresh (`Ctrl + Shift + R`) to clear cached frontend assets.

### Port Exposure/Tunnel fails to open
1. Verify the service is actively listening on the target port inside the container:
   ```bash
   netstat -tlnp
   ```
2. Verify `cloudflared` is running inside the container:
   ```bash
   ps aux | grep cloudflared
   ```
3. Inspect `/tmp/tunnel.log` inside the container:
   ```bash
   cat /tmp/tunnel.log
   ```
4. Note that Cloudflare Tunnels can take 10–15 seconds to fully establish. Refresh the UI status if the URL displays as loading.

### Rate limit errors
* The client receives a `429 Too Many Requests` status if they request more than 5 environments in a 1-hour window. Wait for the reset interval (flushed every hour) or request from a different client IP.
