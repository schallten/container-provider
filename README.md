# TempDev

TempDev is a lightweight, ephemeral cloud sandbox platform for spinning up isolated Linux development environments, running code, and sharing temporary public URLs.

## Features

- **Instant Environments**: Spin up containers in seconds.
- **In-Browser Terminal**: Interactive shell via WebSocket and xterm.js.
- **Auto-Cleanup**: Automated container cleanup after 15 minutes of inactivity or 12 hours max lifetime.
- **Port Exposure**: Public HTTPS tunnels using cloudflared.
- **Abuse Prevention**: Automatic termination of resource-intensive processes.
- **Strict Isolation**: Containers limited to 512MB RAM, 0.5 CPU, and 64 processes.

## Architecture Decisions

- **In-Memory State**: Uses `sync.Map` in Go instead of a database.
- **No Authentication**: Employs IP-based rate limiting (5 envs/hour per IP).
- **Ephemeral**: No persistent storage.

## API Endpoints

- `POST /env` - Create a sandbox.
- `GET /env/{id}` - Get sandbox details.
- `DELETE /env/{id}` - Delete a sandbox.
- `WS /ws/env/{id}` - Connect to sandbox terminal.
- `POST /expose` - Expose container ports.
- `GET /envs` - List active sandboxes.

## Getting Started

### 1. Build Container Image
```bash
docker build -t tempdev:latest .
```

### 2. Run Backend
```bash
go run main.go
```
