# TempDev - Quick Start Guide

## Prerequisites

- Linux/macOS with Docker installed
- Go 1.19+ (for development)
- git

---

## Local Development Setup

### 1. Clone/Setup Project

```bash
mkdir -p tempdev && cd tempdev

# Create directory structure
mkdir -p public logs

# Copy files:
# - main.go
# - Dockerfile
# - public/index.html
# - public/status.html
```

### 2. Install Go Dependencies

```bash
go get github.com/gorilla/websocket
go get github.com/creack/pty
```

### 3. Build Docker Image

```bash
# Build the base environment image
docker build -t tempdev:latest .

# Test it builds
docker run -it tempdev:latest echo "Hello from TempDev"
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
3. You'll get a public HTTPS URL via cloudflared

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

- **Type**: t3.micro (free tier eligible)
- **OS**: Ubuntu 22.04 LTS
- **Storage**: 30GB
- **Security Group**: Allow ports 80, 443, 22

### 2. SSH into Instance

```bash
ssh -i your-key.pem ubuntu@<instance-ip>
```

### 3. Install Docker

```bash
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh
sudo usermod -aG docker ubuntu
newgrp docker
```

### 4. Prepare TempDev

```bash
# Clone or upload your code
git clone <your-repo> tempdev
cd tempdev

# Or upload manually:
# scp -i key.pem -r . ubuntu@instance:/home/ubuntu/tempdev

# Build Docker image
docker build -t tempdev:latest .

# Verify image
docker images | grep tempdev
```

### 5. Build and Deploy Go Binary

```bash
# Install Go 1.21+ if not present
wget https://go.dev/dl/go1.21.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# Build binary
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o tempdev main.go

# Create systemd service
sudo tee /etc/systemd/system/tempdev.service > /dev/null <<'EOF'
[Unit]
Description=TempDev - Temporary Cloud Dev Environments
After=network.target docker.service
Requires=docker.service

[Service]
Type=simple
User=ubuntu
WorkingDirectory=/home/ubuntu/tempdev
ExecStart=/home/ubuntu/tempdev/tempdev
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

### 6. Setup HTTPS with Certbot

```bash
# Install certbot and nginx
sudo apt-get update
sudo apt-get install -y certbot nginx

# Create nginx config
sudo tee /etc/nginx/sites-available/tempdev > /dev/null <<'EOF'
upstream tempdev {
  server localhost:8080;
}

server {
  listen 80;
  server_name yourdomain.com;
  return 301 https://$server_name$request_uri;
}

server {
  listen 443 ssl http2;
  server_name yourdomain.com;

  ssl_certificate /etc/letsencrypt/live/yourdomain.com/fullchain.pem;
  ssl_certificate_key /etc/letsencrypt/live/yourdomain.com/privkey.pem;

  ssl_protocols TLSv1.2 TLSv1.3;
  ssl_ciphers HIGH:!aNULL:!MD5;
  ssl_prefer_server_ciphers on;

  location / {
    proxy_pass http://tempdev;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_read_timeout 86400;
  }
}
EOF

# Enable nginx site
sudo ln -s /etc/nginx/sites-available/tempdev /etc/nginx/sites-enabled/
sudo rm /etc/nginx/sites-enabled/default

# Get SSL certificate
sudo certbot certonly --standalone -d yourdomain.com --agree-tos -m your@email.com

# Start nginx
sudo systemctl start nginx
sudo systemctl enable nginx
```

### 7. Point Domain DNS

Using Route 53 or external DNS:
```
A record: yourdomain.com → <EC2-instance-public-ip>
```

Or use Cloudflare:
```
A record: yourdomain.com → <EC2-instance-public-ip>
Proxy status: Proxied (orange cloud)
```

### 8. Test Deployment

```bash
# From your local machine
curl https://yourdomain.com/envs

# Open in browser
https://yourdomain.com/
```

### 9. Monitor Logs

```bash
# TempDev logs
sudo journalctl -u tempdev -f -n 100

# Nginx access logs
sudo tail -f /var/log/nginx/access.log

# Nginx error logs
sudo tail -f /var/log/nginx/error.log

# Event log
tail -f /home/ubuntu/tempdev/events.log
```

---

## Troubleshooting

### WebSocket Connection Issues

**Symptom**: Browser WebSocket fails to connect

**Solutions**:
1. Check nginx proxy configuration includes `Upgrade` and `Connection` headers
2. Ensure firewall allows ports 80 and 443
3. Check CloudFlare is not interfering (try DNS only)
4. Look at browser console (F12) for CORS errors

### Docker Permission Issues

**Symptom**: `permission denied while trying to connect to the Docker daemon`

**Solution**:
```bash
sudo usermod -aG docker $USER
newgrp docker
# May need to logout/login
```

### Port Already in Use

**Symptom**: `listen tcp :8080: bind: address already in use`

**Solution**:
```bash
# Find process using port 8080
sudo lsof -i :8080

# Kill it
sudo kill -9 <PID>
```

### Out of Memory

**Symptom**: Containers get killed, OOM errors

**Solution**:
1. Check Docker disk usage: `docker system df`
2. Reduce container limit: `--memory=256m` in main.go
3. Reduce concurrent environments
4. Upgrade instance size

### High CPU Usage

**Symptom**: Server is slow/unresponsive

**Solutions**:
1. Check for runaway mining/abuse: `docker stats`
2. Verify abuse detection loop is running
3. Lower CPU limit: `--cpus=0.25`
4. Upgrade instance size

---

## Operations

### Backup/Restore

Since all data is ephemeral, no backups needed. But keep these:
- `events.log` (audit trail)
- Source code and configs

### Update Code

```bash
cd tempdev

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

# Containers will use old image; kill and recreate:
docker system prune -a
```

### Monitor Health

```bash
# Active environments
curl https://yourdomain.com/envs | jq '.'

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

- **t3.micro**: $0 (free tier, first 12 months)
- **Storage**: 30GB EBS ≈ $3/month
- **Data transfer**: ~$0 (within free tier)
- **Domain**: $10-15/month
- **Certbot**: Free

**Total**: ~$13-18/month after free tier expires

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

# Nginx
sudo systemctl status nginx
sudo nginx -t
sudo systemctl reload nginx
```

---

## Next Steps

1. **Test locally** - Create an env, expose a port, verify everything works
2. **Deploy to AWS** - Follow AWS Deployment section
3. **Monitor** - Watch logs for issues, test from external network
4. **Optimize** - Adjust resource limits, add more features based on usage

Enjoy! 🚀