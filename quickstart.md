# TempDev — Quick Start Guide

## Prerequisites

* Linux/macOS with Docker installed
* Go 1.26+
* GCC (for CGO / SQLite)
* git

---

## Local Setup

```bash
# Clone
git clone <repo-url> && cd container-provider

# Build Docker image
docker build -t tempdev:latest .

# Build and run
go build -o tempdev main.go db.go billing_db.go
./tempdev

# Open http://localhost:8080
```

New accounts start with **1,000 free credits**.

---

## Build Command

Because we use CGO for SQLite:

```bash
go build -o tempdev main.go db.go billing_db.go
```

---

## Features to Test

### 1. Create an Environment
Click **Launch** on the TempDev page. A container starts with a web terminal.

### 2. SSH Access
Click the **SSH** button → download the `.pem` key → run:
```bash
chmod 600 tempdev-*.pem
ssh dev@localhost -p <port> -i tempdev-*.pem
```

### 3. Expose a Port
Inside the terminal, start a server (e.g. `python3 -m http.server 8000`), click **Expose Port**, enter `8000`. Get a public HTTPS URL.

### 4. Tag Environments
Click **+ Add** in the tags bar → enter key/value → **Save Tags**.

### 5. Billing
Go to **Billing** page → top up with any card (e.g. `4242 4242 4242 4242`). ~25% random decline rate for realism.

### 6. Cost Explorer
Go to **Cost Explorer** → view spending charts by day and environment.

### 7. Logs
Go to **Logs** → search/filter events (env created, deleted, tunnels, abuse).

### 8. Settings
Open an env → click **Settings** in sidebar → toggle idle timeout / max lifetime bypass.

---

## Cleanup

```bash
# Remove all tempdev containers
docker rm -f $(docker ps -q --filter "ancestor=tempdev:latest")

# Remove databases (fresh start)
rm -f tempdev.db tempdev.db-shm tempdev.db-wal billing.db billing.db-shm billing.db-wal

# Remove SSH keys
rm -rf ssh_keys/
```

---

## Common Issues

### "Permission denied (publickey)" on SSH
```bash
chmod 600 tempdev-*.pem
```

### "Session ended" in web terminal
Container was killed. Refresh and launch a new one.

### Cloudflare tunnel returns 1033
This was a QUIC protocol issue. Fixed by using `--protocol http2`. If you see this, rebuild the Docker image:
```bash
docker build -t tempdev:latest .
```

### Browser shows old UI
Hard refresh: `Ctrl+Shift+R` (or `Cmd+Shift+R` on Mac).

---

## Rebuild After Code Changes

```bash
# If main.go, db.go, or billing_db.go changed:
go build -o tempdev main.go db.go billing_db.go

# If Dockerfile or entrypoint.sh changed:
docker build -t tempdev:latest .

# Restart
kill $(lsof -t -i:8080)
./tempdev
```
