#!/bin/bash
# Runs as root (before USER switch)
# Block access to cloud metadata endpoints
iptables -A OUTPUT -d 169.254.169.254 -j DROP 2>/dev/null || true
ip6tables -A OUTPUT -d fd00:ec2::254 -j DROP 2>/dev/null || true

# Generate host keys if missing, start sshd, then sleep
if [ ! -f /etc/ssh/ssh_host_ed25519_key ]; then
  ssh-keygen -t ed25519 -f /etc/ssh/ssh_host_ed25519_key -N "" -q
fi
if [ ! -f /etc/ssh/ssh_host_rsa_key ]; then
  ssh-keygen -t rsa -b 4096 -f /etc/ssh/ssh_host_rsa_key -N "" -q
fi
mkdir -p /run/sshd
/usr/sbin/sshd
exec sleep infinity
