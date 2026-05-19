# Container Provider - Quick Start Guide

## Prerequisites

* Linux/macOS with Docker installed
* Go 1.26+ (for development)
* git

---

## Local Development Setup

### 1. Clone/Setup Project

```bash
git clone https://github.com/schallten/container-provider.git && cd container-provider

# Create directory structure
mkdir -p public logs
```

### 2. Download Go Dependencies

```bash
go mod download
```

### 3. Build Docker Image

```bash
# Build the base environment image
docker build -t tempdev:latest .

# Test it builds
docker run -it tempdev:latest echo "Hello from Container Provider"
```

### 4. Build and Run Locally

```bash
# Build the Go binary
go build -o tempdev main.go

# Run the server
./tempdev

# In another terminal, test:
curl -X POST http://localhost:8080/env
```

### 5. Access the App

Open your browser:
```
http://localhost:8080/
```

Click "New" to create an environment.

### 6. Test All Features

#### Create Environment
```bash
curl -X POST http://localhost:8080/env
# Returns: {"id": "abc123", "ws_url": "/ws/env/abc123"}
```

#### List Environments
```bash
curl http://localhost:8080/envs
```

#### Delete Environment
```bash
curl -X DELETE http://localhost:8080/env/abc123
```

#### Open Terminal
Click "New" in the browser. Terminal should appear.

#### Expose a Port
1. In terminal, run: `python3 -m http.server 8000`
2. Click "Expose" button, enter `8000`
3. Get a public HTTPS URL via cloudflared.

#### Stop Exposing a Port
Click "Stop" next to the public URL in the UI, or run:
```bash
curl -X DELETE http://localhost:8080/expose/abc123
```

#### Check Status Dashboard
```
http://localhost:8080/status.html
```

---

## Docker Cleanup

```bash
# Remove all containers
docker ps -aq | xargs docker rm -f

# View Docker logs for a container
docker logs <container-id>

# View disk usage
docker system df
```

---

## AWS Deployment

### 1. Launch EC2 Instance

* **Type**: t3.micro (free tier eligible)
* **OS**: Ubuntu 22.04 LTS
* **Storage**: 30GB
* **Security Group**: Allow ports 8080, 22

### 2. SSH into Instance

```bash
ssh -i your-key.pem ubuntu@<instance-ip>
```

### 3. Install Docker

```bash
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh
rm get-docker.sh
sudo usermod -aG docker ubuntu
newgrp docker
```

### 4. Prepare Container Provider

```bash
# Clone the repository
git clone https://github.com/schallten/container-provider.git
cd container-provider

# Build Docker image
docker build -t tempdev:latest .

# Verify image
docker images | grep tempdev
```

### 5. Build and Deploy Go Binary

```bash
# Install Go 1.26.1
wget https://go.dev/dl/go1.26.1.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.26.1.linux-amd64.tar.gz
rm go1.26.1.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# Build binary
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o tempdev main.go

# Create systemd service
sudo tee /etc/systemd/system/tempdev.service > /dev/null <<'EOF'
[Unit]
Description=Container Provider - Temporary Cloud Dev Environments
After=network.target docker.service
Requires=docker.service

[Service]
Type=simple
User=ubuntu
WorkingDirectory=/home/ubuntu/container-provider
ExecStart=/home/ubuntu/container-provider/tempdev
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

# Enable and start service
sudo systemctl daemon-reload
sudo systemctl enable tempdev
sudo systemctl start tempdev

# Check status
sudo systemctl status tempdev
sudo journalctl -u tempdev -f
```

### 6. Accessing the App Publicly

To expose the service running on port 8080:
* **Option A**: Use a Cloudflare Tunnel pointing to `http://localhost:8080` (Recommended)
* **Option B**: Allow port 8080 in security groups to access directly at `http://<EC2-instance-public-ip>:8080`

See [expose_guide.md](expose_guide.md) for full instructions and security implications.

### 7. Monitor Logs

```bash
# Process logs
sudo journalctl -u tempdev -f -n 100

# Event log
tail -f /home/ubuntu/container-provider/events.log
```

---

## Troubleshooting

### WebSocket Connection Issues

* **Symptom**: Browser WebSocket fails to connect.
* **Solutions**:
  1. Ensure network allows port 8080.
  2. Inspect browser console (F12) for connection errors.

### Docker Permission Issues

* **Symptom**: `permission denied while trying to connect to the Docker daemon`
* **Solution**:
  ```bash
  sudo usermod -aG docker $USER
  newgrp docker
  ```

### Port Already in Use

* **Symptom**: `listen tcp :8080: bind: address already in use`
* **Solution**:
  ```bash
  # Find process using port 8080
  sudo lsof -i :8080

  # Kill it
  sudo kill -9 <PID>
  ```

### Out of Memory

* **Symptom**: Containers get killed, OOM errors.
* **Solution**:
  1. Check Docker disk usage: `docker system df`.
  2. Reduce container limit to `--memory=256m` in `main.go`.
  3. Reduce concurrent environments.
  4. Upgrade instance size.

---

## Operations

### Update Code

```bash
cd container-provider

# Pull latest
git pull

# Rebuild binary
go build -o tempdev main.go

# Restart service
sudo systemctl restart tempdev
```

### Rebuild Image

```bash
# If Dockerfile changed
docker build -t tempdev:latest .

# Clean up unused Docker resources
docker system prune -a
```

### Monitor Health

```bash
# Active environments
curl http://localhost:8080/envs | jq '.'

# Service status
sudo systemctl status tempdev

# Disk space
df -h

# Memory usage
free -h

# Docker stats
docker stats --no-stream
```

---

## Cost Estimate (AWS)

* **t3.micro**: $0 (free tier, first 12 months)
* **Storage**: 30GB EBS ≈ $3/month
* **Data transfer**: ~$0 (within free tier limits)
* **Domain / Tunnel**: Free

**Total**: ~$3/month

---

## Common Commands Cheat Sheet

```bash
# Development
go build -o tempdev main.go
docker build -t tempdev:latest .
./tempdev

# Testing
curl -X POST http://localhost:8080/env
curl http://localhost:8080/envs
curl http://localhost:8080/status.html

# Deployment
ssh -i key.pem ubuntu@instance
sudo systemctl status tempdev
sudo journalctl -u tempdev -f
sudo systemctl restart tempdev

# Docker
docker ps
docker logs <container-id>
docker stats
docker system df
docker system prune -a
```

---

## Next Steps

1. **Test locally** - Create an environment, expose a port, verify functions work.
2. **Deploy to AWS** - Follow the AWS Deployment section.
3. **Monitor** - Watch logs for issues, test from external network.
4. **Optimize** - Adjust resource limits, customize features.