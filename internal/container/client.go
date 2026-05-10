
package container

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	defaultMemoryLimit = 512 * 1024 * 1024
	defaultCPUMax      = "50000"
	defaultPidsLimit   = 64
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
		"sleep", "infinity",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("nerdctl run failed: %w\noutput: %s", err, string(out))
	}

	containerID := strings.TrimSpace(string(out))
	time.Sleep(500 * time.Millisecond)

	return &ContainerInfo{
		ContainerID: containerID,
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

type ContainerInfo struct {
	ContainerID string
}
