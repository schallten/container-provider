#!/bin/bash
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
