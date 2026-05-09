# VPS Provider - Phase 2 Setup

## Prerequisites

- Debian 13 with containerd, CNI, nerdctl installed (Phase 1 complete)
- Go 1.22+ installed

## Setup Service User

```bash
# Create vps user (no login shell)
sudo useradd -r -s /bin/false -M vps

# Add vps user to containerd group (so it can access socket)
sudo usermod -aG containerd vps

# Or: set socket permissions for vps user
sudo chmod 660 /run/containerd/containerd.sock
sudo chown root:containerd /run/containerd/containerd.sock

# Create working directory
sudo mkdir -p /opt/vps-provider
sudo chown vps:vps /opt/vps-provider
```

## Build & Run

```bash
# Copy project to /opt/vps-provider
cp -r vps-provider /opt/vps-provider/
cd /opt/vps-provider

# Download dependencies
sudo -u vps go mod tidy

# Build
sudo -u vps go build -o bin/vps-server cmd/server/main.go

# Run manually first (test)
sudo -u vps ./bin/vps-server

# Test API
curl -X POST http://localhost:8080/api/v1/sessions
curl http://localhost:8080/api/v1/sessions
curl -X DELETE http://localhost:8080/api/v1/sessions/<id>
```

## Systemd Service

```bash
sudo tee /etc/systemd/system/vps-provider.service << 'EOF'
[Unit]
Description=VPS Provider API
After=containerd.service network.target
Requires=containerd.service

[Service]
Type=simple
User=vps
Group=vps
WorkingDirectory=/opt/vps-provider
ExecStart=/opt/vps-provider/bin/vps-server
Restart=always
RestartSec=5
Environment="PORT=8080"

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable vps-provider
sudo systemctl start vps-provider
sudo systemctl status vps-provider
```

## Known Limitations (Phase 2 Step 1)

- IP discovery not implemented yet (returns empty string)
- No WebSocket terminal proxy yet
- No TTL/grace period automation yet
- No Cloudflare Tunnel integration yet
- Session ID generation uses timestamp (not cryptographically random)

## Next Steps

1. Test container creation via API
2. Implement IP discovery via CNI state
3. Add WebSocket terminal proxy
4. Add TTL manager
