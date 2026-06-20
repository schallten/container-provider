# Security Model — TempDev

This document describes the isolation and hardening model used by TempDev's container sandbox engine.

## Isolation Layers

### 1. Linux Namespaces (via Docker)

Every sandbox runs in an isolated Docker container, providing:

- **PID namespace** — container cannot see or signal host processes
- **NET namespace** — isolated network stack, no access to host interfaces
- **MNT namespace** — separate filesystem mount points
- **UTS namespace** — isolated hostname
- **IPC namespace** — no shared memory with host or other containers

### 2. Resource Limits (cgroups)

Each container is hard-capped via Docker cgroup flags:

| Resource | Limit | Flag |
|----------|-------|------|
| Memory | 512 MB | `--memory=512m` |
| Memory swap | 512 MB (no swap beyond RAM) | `--memory-swap=512m` |
| CPU | 0.5 cores | `--cpus=0.5` |
| Processes | 64 PIDs max | `--pids-limit=64` |

These limits prevent resource exhaustion attacks (fork bombs, memory leaks, CPU hogging).

### 3. Privilege Restrictions

- `--security-opt=no-new-privileges` prevents privilege escalation via setuid/setgid binaries.
- Container's default user is root (required for sshd to bind port 2222 and manage sessions).
- **All interactive sessions** (WebSocket shell, SSH) run as non-root user `dev` (UID 1000) via `docker exec -u dev`.
- SSH enforces: `PermitRootLogin no`, `PasswordAuthentication no`, `PubkeyAuthentication yes`.
- `--cap-add=NET_ADMIN` is granted solely for iptables-based metadata blocking.

### 4. Network Isolation

- Each container gets its own network namespace (Docker default bridge).
- No inter-container communication — containers cannot reach each other.
- Outbound internet access is available (for package installs, cloudflared tunnels).
- **AWS metadata blocking:** iptables rules in the entrypoint drop all packets to `169.254.169.254` (IPv4) and `fd00:ec2::254` (IPv6), preventing SSRF-based credential theft on AWS/GCP/Azure.

### 5. Filesystem Isolation

- Each container has an overlay filesystem — writes don't persist after container deletion.
- No volume mounts to host filesystem.
- `/tmp` and `/var/tmp` are world-writable inside container (standard), but isolated via mount namespace.

### 6. Process Isolation

- PID 1 in container is the entrypoint, not a host PID.
- `ps aux` inside container only shows container processes.
- Abuse detection loop scans process list every 3 minutes for blacklisted binaries (xmrig, masscan, nmap, hydra, etc.).

## SSH Access

When SSH is enabled for an environment:

- ED25519 keypair generated per environment (not shared).
- Public key injected into `/home/dev/.ssh/authorized_keys` (owner: dev:dev, mode 600).
- Container sshd runs on port 2222, mapped to a random host port (22000–23000).
- `sshd_config` enforces: `PermitRootLogin no`, `PasswordAuthentication no`, `PubkeyAuthentication yes`.
- Private key downloadable as `.pem` file via `/ssh/:id` endpoint.

## Tunnel Exposure (Cloudflare)

- Port exposure uses `cloudflared tunnel --protocol http2` (QUIC/UDP disabled — breaks in Docker bridge networking).
- Tunnel runs inside the container, not on the host.
- Each tunnel gets a unique `*.trycloudflare.com` HTTPS URL.
- Tunnel killed on environment deletion or explicit unexpose.

## Abuse Detection

- **Process scanning:** Every 3 minutes, `ps aux` output is checked against a blacklist of known attack/mining tools.
- **Rate limiting:** 5 environments per IP per hour.
- **Per-minute billing:** Environments auto-terminate if credits are insufficient.
- **Idle timeout:** 15 minutes of inactivity (configurable per-env) triggers cleanup.
- **Max lifetime:** 12 hours hard cap (configurable per-env).

Blacklisted processes:
```
xmrig, miner, stratum, masscan, nmap, zmap,
hydra, john, hashcat, nikto, sqlmap,
metasploit, aircrack, airmon, bettercap
```

## Threat Model

| Threat | Mitigation |
|--------|------------|
| Container escape | Docker namespace isolation + no-new-privileges + Docker default seccomp |
| Resource exhaustion | cgroup limits (512MB, 0.5 CPU, 64 PIDs) |
| Privilege escalation | Non-root user for sessions (UID 1000), no setuid allowed |
| Network pivot | Isolated network namespace, no inter-container traffic |
| SSRF to cloud metadata | iptables rules block 169.254.169.254 and fd00:ec2::254 |
| Crypto mining | Process scanning + auto-termination |
| SSH brute force | Key-only auth, no password login |
| Data persistence | Overlay filesystem, no host mounts, containers destroyed on cleanup |

## Benchmark Results

Measured on AMD Ryzen 7 8840HS (16 cores), Docker with cached image:

| Metric | Result |
|--------|--------|
| Single container creation | ~400ms |
| 20 concurrent containers | ~1.6s total (~80ms each) |
| Container cleanup | ~170ms |
| sshd startup | ~90ms |

Run benchmarks yourself:
```bash
go test -bench=. -benchmem -count=3
go test -v -run=Test
```

## What This Is NOT

This is a development sandbox, not a production security boundary:

- Containers share the same Linux kernel as the host.
- Kernel vulnerabilities could theoretically allow escape.
- Docker's default seccorp profile is used (blocks ~44 dangerous syscalls), but no custom profile.
- No AppArmor or SELinux profiles are configured.
- No user namespace remapping (`--userns-remap`).
- No network egress filtering beyond Docker defaults.

For high-security use cases, consider Firecracker microVMs or gVisor, which provide stronger isolation at the cost of complexity and performance.

## Known Limitations & Improvements

| Gap | Current State | What Would Fix It |
|-----|---------------|-------------------|
| Custom seccomp | Docker default only | Custom seccomp profile restricting to needed syscalls |
| Capability model | `NET_ADMIN` added for iptables | Drop ALL caps, use host-level iptables instead |
| User namespaces | Not used | `--userns-remap` to map container UIDs to unprivileged host UIDs |
| MAC | None | AppArmor or SELinux profile |
| Root filesystem | Read-write | `--read-only` with explicit writable tmpfs mounts |
| Egress filtering | None | Docker network policies or Calico/Cilium |
