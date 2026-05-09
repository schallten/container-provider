package container

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	defaultMemoryLimit = 512 * 1024 * 1024 // 512MB
	defaultCPUMax      = "50000"            // 50% of 1 core
	defaultPidsLimit   = 64
	ttydPort           = 7681
	namespace          = "vps-provider"
)

type Client struct{}

func NewClient(socketPath string) (*Client, error) {
	return &Client{}, nil
}

func (c *Client) Close() error {
	return nil
}

func (c *Client) CreateContainer(ctx context.Context, sessionID string) (*ContainerInfo, error) {
	// Use nerdctl run - handles CNI, image, everything
	cmd := exec.CommandContext(ctx,
		"nerdctl", "-n", namespace, "run", "-d",
		"--name", sessionID,
		"--memory", fmt.Sprintf("%d", defaultMemoryLimit),
		"--cpus", "0.5",
		"--pids-limit", fmt.Sprintf("%d", defaultPidsLimit),
		"--read-only",
		"--cap-drop", "ALL",
		"--cap-add", "CHOWN",
		"--cap-add", "DAC_OVERRIDE",
		"--cap-add", "SETGID",
		"--cap-add", "SETUID",
		"--security-opt", "no-new-privileges=true",
		"docker.io/library/vps-base:latest",
		"ttyd", "-p", fmt.Sprintf("%d", ttydPort), "-W", "/bin/bash",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("nerdctl run failed: %w\noutput: %s", err, string(out))
	}

	containerID := strings.TrimSpace(string(out))

	// Wait for network
	time.Sleep(2 * time.Second)

	// Get IP
	ip, err := c.discoverIP(sessionID)
	if err != nil {
		ip = ""
	}

	return &ContainerInfo{
		ContainerID: containerID,
		IP:          ip,
		TTYDPort:    ttydPort,
	}, nil
}

func (c *Client) DestroyContainer(ctx context.Context, containerID string) error {
	cmd := exec.CommandContext(ctx, "nerdctl", "-n", namespace, "rm", "-f", containerID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nerdctl rm failed: %w\noutput: %s", err, string(out))
	}
	return nil
}

func (c *Client) discoverIP(containerID string) (string, error) {
	cmd := exec.Command("nerdctl", "-n", namespace, "inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", containerID)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(string(out))
	if ip == "" {
		return "", fmt.Errorf("no IP found")
	}
	return ip, nil
}

type ContainerInfo struct {
	ContainerID string
	IP          string
	TTYDPort    int
}