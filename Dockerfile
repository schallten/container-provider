FROM ubuntu:22.04

# ============================================================================
# System Setup
# ============================================================================

RUN apt-get update && apt-get upgrade -y

# Core development tools
RUN apt-get install -y \
    curl wget git vim nano htop tmux tree \
    build-essential pkg-config \
    openssh-client openssh-server \
    sudo \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# ============================================================================
# Python 3
# ============================================================================

RUN apt-get update && apt-get install -y \
    python3 python3-pip python3-venv python3-dev \
    && rm -rf /var/lib/apt/lists/*

# ============================================================================
# Node.js
# ============================================================================

RUN apt-get update && apt-get install -y \
    nodejs npm \
    && rm -rf /var/lib/apt/lists/*

# ============================================================================
# Go
# ============================================================================

RUN apt-get update && apt-get install -y \
    golang-go \
    && rm -rf /var/lib/apt/lists/*

# ============================================================================
# ttyd (terminal over WebSocket)
# ============================================================================

RUN curl -fsSL https://github.com/tsl0922/ttyd/releases/download/1.7.3/ttyd.x86_64 \
    -o /usr/local/bin/ttyd && \
    chmod +x /usr/local/bin/ttyd

# ============================================================================
# cloudflared (tunneling)
# ============================================================================

RUN curl -fsSL https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64 \
    -o /usr/local/bin/cloudflared && \
    chmod +x /usr/local/bin/cloudflared

# ============================================================================
# Security Hardening
# ============================================================================

# Create non-root user
RUN useradd -m -s /bin/bash -u 1000 dev && \
    mkdir -p /home/dev/.config && \
    chown -R dev:dev /home/dev

# Secure permissions
RUN chmod 1777 /tmp && chmod 1777 /var/tmp

# ============================================================================
# Workspace Setup
# ============================================================================

WORKDIR /home/dev

# Create common directories
RUN mkdir -p /home/dev/projects /home/dev/.ssh && \
    chown -R dev:dev /home/dev

# Switch to non-root user
USER dev

# ============================================================================
# Entry Point
# ============================================================================

CMD ["/bin/bash"]