Here's everything we did, step by step:

---

## What We Built

A lightweight container provider that gives users ephemeral Linux environments via API, running on your Debian 13 PC (accessed via SSH).

---

## Exact Steps Taken

### 1. Installed containerd
```bash
sudo apt update
sudo apt install -y containerd
```
- Verified: `sudo systemctl status containerd` — active
- Verified: `sudo ctr version` — client + server responding

### 2. Installed CNI plugins
```bash
sudo apt install -y containernetworking-plugins
```
- Verified: `ls /usr/lib/cni/` — bridge, host-local, loopback, etc. present

### 3. Created CNI network config
```bash
sudo mkdir -p /etc/cni/net.d
sudo tee /etc/cni/net.d/10-vps-bridge.conf << 'EOF'
{
  "cniVersion": "0.4.0",
  "name": "vps-bridge",
  "type": "bridge",
  "bridge": "cni0",
  "isGateway": true,
  "ipMasq": true,
  "ipam": {
    "type": "host-local",
    "subnet": "10.88.0.0/16",
    "routes": [{ "dst": "0.0.0.0/0" }]
  }
}
EOF
```

### 4. Installed nerdctl
```bash
cd /tmp
wget https://github.com/containerd/nerdctl/releases/download/v2.0.5/nerdctl-2.0.5-linux-amd64.tar.gz
sudo tar Cxzvf /usr/local/bin nerdctl-2.0.5-linux-amd64.tar.gz
```
- Verified: `sudo nerdctl --version` — 2.0.5

### 5. Installed buildkit (for building images)
```bash
cd /tmp
wget https://github.com/moby/buildkit/releases/download/v0.13.2/buildkit-v0.13.2.linux-amd64.tar.gz
sudo tar -xzf buildkit-v0.13.2.linux-amd64.tar.gz -C /usr/local/bin/ --strip-components=1

sudo tee /etc/systemd/system/buildkit.service << 'EOF'
[Unit]
Description=BuildKit
[Service]
ExecStart=/usr/local/bin/buildkitd
Restart=always
[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now buildkit
```
- Verified: `sudo systemctl status buildkit` — active

### 6. Built base image `vps-base:latest`
Created `~/vps-base/Dockerfile`:
```dockerfile
FROM ubuntu:22.04
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 python3-pip python3-venv nodejs npm git curl wget ca-certificates build-essential \
    && rm -rf /var/lib/apt/lists/*
RUN curl -L -o /usr/local/bin/ttyd \
    https://github.com/tsl0922/ttyd/releases/download/1.7.7/ttyd.x86_64 \
    && chmod +x /usr/local/bin/ttyd
RUN useradd -m -s /bin/bash user
WORKDIR /home/user
USER user
CMD ["/bin/bash"]
```

Built:
```bash
sudo nerdctl build -t vps-base:latest ~/vps-base
```

### 7. Tested container lifecycle
Pulled Alpine for initial test:
```bash
sudo ctr images pull docker.io/library/alpine:latest
sudo ctr run --rm docker.io/library/alpine:latest test-container uname -a
```

Tested CNI networking with nerdctl:
```bash
sudo nerdctl run --rm -it alpine:latest ip addr
# Got 10.4.0.2/24 on eth0 — CNI working
```

### 8. Tested ttyd in container
```bash
sudo nerdctl run -d --name test-ttyd vps-base:latest ttyd -p 7681 -W /bin/bash
sudo nerdctl inspect test-ttyd | grep IPAddress
# IP: 10.4.0.4
curl http://10.4.0.4:7681
# Got ttyd HTML page — working
```

Cleaned up:
```bash
sudo nerdctl rm -f test-ttyd
```

---

## Current State

| Component | Status |
|-----------|--------|
| containerd | Running, systemd-enabled |
| CNI plugins | Installed, bridge config written |
| nerdctl | Installed, working |
| buildkit | Running, systemd-enabled |
| Base image `vps-base:latest` | Built with Ubuntu + Python + Node + git + ttyd |
| Networking | Containers get `10.4.0.x/24` IPs via CNI bridge |
| ttyd | Serves terminal on `0.0.0.0:7681` inside container |

---

## What's Next (Not Done Yet)

- Go API server (session CRUD, containerd gRPC client)
- WebSocket proxy (browser → Go API → container ttyd)
- TTL manager (12h max, 15m grace)
- Cloudflare Tunnel integration (public URLs)
- Dashboard (vanilla JS + xterm.js)

---

## Key Decisions Made

- **Runtime:** containerd directly on host (no Docker nesting)
- **Networking:** CNI bridge, host routes directly to container IPs
- **Terminal:** ttyd on TCP port 7681 (no Unix socket — ttyd 1.7.7 lacks `--unix-socket`)
- **Base image:** Ubuntu 22.04 + Python + Node + ttyd pre-installed
- **Stack:** Go API + vanilla JS dashboard

---

Is anything missing or wrong? Want to correct course before Phase 2?