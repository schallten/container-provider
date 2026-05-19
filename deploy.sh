#!/bin/bash

# ============================================================================
# TempDev Simple AWS Deployment Script
# ============================================================================
# Usage:
#   ./deploy.sh
#
# This script:
# 1. Installs Docker
# 2. Installs Go
# 3. Builds Docker image
# 4. Compiles Go binary
# 5. Creates systemd service
# 6. Starts TempDev (listening on :8080)
#
# Cloudflared tunnel handles HTTPS & public URLs automatically
# ============================================================================

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}🚀 TempDev Simple Deployment${NC}"
echo ""

# ============================================================================
# System Updates
# ============================================================================

echo -e "${YELLOW}[1/5] Updating system...${NC}"
sudo apt-get update -qq
sudo apt-get upgrade -y -qq
sudo apt-get install -y -qq curl wget git build-essential

# ============================================================================
# Install Docker
# ============================================================================

echo -e "${YELLOW}[2/5] Installing Docker...${NC}"
if ! command -v docker &> /dev/null; then
  curl -fsSL https://get.docker.com -o get-docker.sh
  sudo sh get-docker.sh
  rm get-docker.sh
  sudo usermod -aG docker ubuntu
  echo -e "${GREEN}✓ Docker installed${NC}"
else
  echo -e "${GREEN}✓ Docker already installed${NC}"
fi

# ============================================================================
# Install Go
# ============================================================================

echo -e "${YELLOW}[3/5] Installing Go...${NC}"
if ! command -v go &> /dev/null; then
  GO_VERSION="1.26.1"
  wget -q https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz
  sudo tar -C /usr/local -xzf go${GO_VERSION}.linux-amd64.tar.gz
  rm go${GO_VERSION}.linux-amd64.tar.gz
  
  echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
  export PATH=$PATH:/usr/local/go/bin
  echo -e "${GREEN}✓ Go installed${NC}"
else
  echo -e "${GREEN}✓ Go already installed${NC}"
fi

# ============================================================================
# Build Docker Image
# ============================================================================

echo -e "${YELLOW}[4/5] Building Docker image...${NC}"
docker build -t tempdev:latest .
echo -e "${GREEN}✓ Docker image built${NC}"

# ============================================================================
# Build Go Binary
# ============================================================================

echo -e "${YELLOW}[5/5] Building Go binary...${NC}"
export PATH=$PATH:/usr/local/go/bin
go get github.com/gorilla/websocket
go get github.com/creack/pty
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o tempdev main.go
echo -e "${GREEN}✓ Binary compiled${NC}"

# ============================================================================
# Create systemd Service
# ============================================================================

echo -e "${YELLOW}Setting up systemd service...${NC}"
sudo tee /etc/systemd/system/tempdev.service > /dev/null <<'EOF'
[Unit]
Description=TempDev - Temporary Cloud Dev Environments
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

sudo systemctl daemon-reload
sudo systemctl enable tempdev
sudo systemctl start tempdev

sleep 2
if sudo systemctl is-active --quiet tempdev; then
  echo -e "${GREEN}✓ Systemd service started${NC}"
else
  echo -e "${RED}✗ Service failed to start${NC}"
  sudo journalctl -u tempdev -n 20
  exit 1
fi

# ============================================================================
# Summary
# ============================================================================

echo ""
echo -e "${GREEN}✓ Deployment complete!${NC}"
echo ""
echo "Your TempDev instance is running:"
echo -e "${GREEN}  http://localhost:8080${NC}"
echo ""
echo "To access from the internet:"
echo "  1. Run cloudflared on your local machine or use a reverse proxy"
echo "  2. Or set up Cloudflare tunnel from your domain"
echo "  3. Or use ngrok/similar service"
echo ""
echo "Monitor logs:"
echo "  sudo journalctl -u tempdev -f"
echo ""
echo "View events:"
echo "  tail -f events.log"
echo ""
echo "Check Docker containers:"
echo "  docker ps"
echo "  docker stats"
echo ""
echo "Restart service:"
echo "  sudo systemctl restart tempdev"
echo ""
echo -e "${YELLOW}⚠️  Important: TempDev is running on :8080 internally.${NC}"
echo -e "${YELLOW}   To expose publicly, use Cloudflare/ngrok/your reverse proxy.${NC}"