# TempDev 🚀

**Lightweight temporary cloud development environments on demand.**

No signup. No authentication. Create isolated Linux shells in seconds. Share temporary public URLs. Everything self-destructs in 15 minutes.

---

## Features

✨ **Instant Isolated Environments**
- Click "New" → Get a full Linux shell in < 3 seconds
- 512MB RAM, 0.5 CPU per environment
- Automatic cleanup after 15 min idle or 12 hours max

🔗 **Public URLs via Cloudflare Tunnel**
- Expose any port inside your environment
- Get a public HTTPS URL instantly
- Share with anyone, anywhere

🖥️ **Browser Terminal**
- Full xterm.js integration
- Real terminal emulation (not a fake console)
- Keyboard shortcuts, paste, copy all work

🔒 **Secure Isolation**
- Docker containers with dropped capabilities
- Non-root user (can't escalate)
- No host filesystem access
- Rate limited (5 per hour per IP)

⚡ **Minimal Overhead**
- ~120MB memory per idle environment
- No database (in-memory state)
- No external dependencies
- Runs on AWS free tier t3.micro

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
│  • Tunnel provisioner
│  • Abuse detection  │
└────────────┬────────┘
             ↓
      Docker Containers
      (512MB, 0.5CPU)
      • Python 3
      • Node.js
      • Go toolchain
      • Git
      • ttyd (shell over WS)
      • cloudflared (tunneling)
```

---

## Quick Start

### Local Development

```bash
# Clone the repo
git clone <your-repo> tempdev && cd tempdev

# Build Docker image
docker build -t tempdev:latest .

# Install Go dependencies
go get github.com/gorilla/websocket github.com/creack/pty

# Build and run
go build -o tempdev main.go
./tempdev

# Open browser
open http://localhost:8080
```

### AWS Deployment

```bash
# On EC2 instance (Ubuntu 22.04, t3.micro)
ssh -i key.pem ubuntu@<instance-ip>

# Clone your repo
git clone <your-repo> tempdev && cd tempdev

# Run deployment script
chmod +x deploy.sh
./deploy.sh --domain yourdomain.com --email your@email.com

# Wait for DNS to propagate, then visit:
# https://yourdomain.com
```

See [QUICKSTART.md](QUICKSTART.md) for detailed instructions.

---

## Usage

### Create Environment

Click the **New** button. You'll see:
- A terminal window appear
- Your environment ID
- Real-time uptime/idle counters

### Run Commands

```bash
# Any shell commands work
python3 --version
node --version
go version

# Install packages
pip install flask
npm install express

# Run a server
python3 -m http.server 8000
node server.js
```

### Expose a Port

1. Start a server inside the environment (e.g., `python3 -m http.server 8000`)
2. Click the **Expose** button
3. Enter the port number (e.g., `8000`)
4. Get a public HTTPS URL instantly
5. Share with anyone, anywhere

Example public URL:
```
https://tempdev-abc123-8000.trycloudflare.com
```

### Destroy Environment

Click **Destroy** to immediately delete the environment, or it auto-deletes after:
- **15 minutes of inactivity** (no input), or
- **12 hours max lifetime**

### Check Status

Visit the status dashboard:
```
http://yourserver/status.html
```

Shows:
- All active environments
- Uptime per environment
- Idle time (turns red at 15 min)
- Public tunnel URLs

---

## API

### Create Environment

```bash
POST /env
```

Response:
```json
{
  "id": "abc123",
  "ws_url": "/ws/env/abc123"
}
```

### Get Environment Status

```bash
GET /env/:id
```

Response:
```json
{
  "id": "abc123",
  "created_at": "2024-01-15T10:30:00Z",
  "last_ping": "2024-01-15T10:35:12Z",
  "tunnel_url": "https://..."
}
```

### Delete Environment

```bash
DELETE /env/:id
```

### List All Environments

```bash
GET /envs
```

Response:
```json
[
  {
    "id": "abc123",
    "uptime": "5m30s",
    "idle": "2m10s",
    "tunnel_url": "https://..."
  }
]
```

### Shell via WebSocket

```
WS /ws/env/:id
```

Binary protocol: send keys, receive terminal output.

### Expose Port

```bash
POST /expose/:id/:port
```

Example: `POST /expose/abc123/3000`

Response:
```json
{
  "tunnel_url": "https://tempdev-abc123-3000.trycloudflare.com",
  "note": "URL may take 5-10 seconds to activate"
}
```

---

## Included Tools

Every environment comes pre-installed with:

- **Python 3** with pip
- **Node.js** with npm
- **Go** toolchain
- **Git**
- **curl, wget, vim, nano, htop, tmux**
- **ttyd** (shell over WebSocket)
- **cloudflared** (Cloudflare tunneling)

---

## Security

### Container Hardening

- `--cap-drop=ALL` - Drop all Linux capabilities
- `--security-opt=no-new-privileges` - No privilege escalation
- `-u dev` - Non-root user
- `--memory=512m` - RAM limit
- `--cpus=0.5` - CPU limit
- `--pids-limit=64` - Max processes

### Host Protection

- AWS metadata server blocked (`169.254.169.254`)
- Rate limiter: 5 environments per hour per IP
- Abuse detection: Auto-kill xmrig, nmap, masscan, etc.
- Logs all events to `events.log` (JSON format)

### Network

- Docker containers on isolated network
- No host port mapping (tunneling via cloudflared)
- Outbound traffic allowed (user's responsibility)

---

## Performance

### Resource Usage per Environment

| Resource | Limit | Typical Idle |
|----------|-------|--------------|
| Memory | 512MB | ~120MB |
| CPU | 0.5 cores | <5% |
| Processes | 64 | ~10 |
| Startup time | - | 2-3s |

### Server Capacity (t3.micro)

- **Memory**: 1GB total → ~4-6 concurrent environments
- **CPU**: 1 vCPU → Good for development workloads
- **Disk**: 30GB → Plenty for ephemeral use

---

## Monitoring

### View Logs

```bash
# TempDev process logs
sudo journalctl -u tempdev -f

# Event log (JSON)
tail -f events.log

# Docker logs
docker logs <container-id>

# Nginx logs
sudo tail -f /var/log/nginx/access.log
```

### Check Environment Status

```bash
# Active environments
curl https://yourdomain.com/envs | jq '.'

# Docker stats
docker stats --no-stream

# Disk usage
df -h

# Memory
free -h
```

---

## Troubleshooting

### Terminal doesn't appear

**Check**:
1. Browser console (F12) for JavaScript errors
2. Network tab for WebSocket connection
3. Server logs: `sudo journalctl -u tempdev -f`

**Try**:
- Hard refresh (Ctrl+Shift+R)
- Try a different browser
- Check if firewall blocks WebSocket

### Expose/Tunnel doesn't work

**Check**:
1. Port is correct and open inside container: `netstat -tlnp`
2. Cloudflared is running: `docker exec <container> ps aux | grep cloudflared`
3. No firewall blocking port

**Try**:
- Wait 10 seconds for tunnel to activate
- Use a different port (1000+)

### High memory/CPU usage

**Check**:
1. `docker stats` for resource usage
2. `docker exec <container> ps aux` for runaway processes
3. `tail -f events.log` for abuse detection

**Try**:
- Reduce container limits in `main.go`
- Upgrade instance size
- Lower concurrent environment cap

### Rate limit / Too many requests

**Solution**: The IP is rate limited to 5 new environments per hour. Wait an hour or use a different IP.

---

## Deployment

### Minimal Setup (macOS/Linux)

```bash
docker build -t tempdev .
go build -o tempdev main.go
./tempdev
# Visit http://localhost:8080
```

### Production (AWS)

1. Launch EC2 instance (t3.micro, Ubuntu 22.04)
2. Run `deploy.sh`:
   ```bash
   ./deploy.sh --domain yourdomain.com --email your@email.com
   ```
3. Point domain DNS to instance IP
4. Wait 5-10 min for SSL certificate
5. Visit `https://yourdomain.com`

See [QUICKSTART.md](QUICKSTART.md) for step-by-step.

---

## Costs

| Component | Cost |
|-----------|------|
| t3.micro (1 year free) | $0-10/month |
| 30GB EBS storage | ~$3/month |
| Data transfer | $0-1/month (free tier) |
| Domain | $10-15/month |
| **Total** | ~$13-29/month |

After free tier expires, roughly $13-30/month for a working MVP.

---

## Roadmap

### MVP (Done) ✅
- ✅ Instant environment creation
- ✅ Browser terminal (xterm.js)
- ✅ Port exposure (cloudflared)
- ✅ Auto-cleanup (15m idle, 12h max)
- ✅ Abuse detection
- ✅ Lightweight (fits t3.micro)

### Phase 2
- [ ] Authentication (OAuth)
- [ ] Persistent storage (optional)
- [ ] Multiple base images
- [ ] Environment snapshots
- [ ] Team collaboration

### Phase 3
- [ ] IDE in browser (VS Code Web)
- [ ] Multi-user sessions
- [ ] Scheduled shutdowns
- [ ] Custom startup scripts
- [ ] Marketplace for configs

---

## Contributing

This is a minimal MVP. Contributions welcome!

Areas for improvement:
- Better error messages
- Performance optimizations
- More base images
- Database persistence
- Better UI/UX

---

## License

MIT

---

## Questions?

- **Docs**: See [QUICKSTART.md](QUICKSTART.md)
- **Issues**: Check `events.log` and `journalctl`
- **Performance**: Check resource limits in `main.go`
- **Security**: Review Docker flags and firewall rules

---

## Built in 7 Days 🚀

This project was built rapidly using:
- **Go** for simplicity and speed
- **Docker** for isolation
- **xterm.js** for UI
- **Cloudflared** for tunneling
- Minimal dependencies, maximum value