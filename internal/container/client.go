package container

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	"github.com/opencontainers/runtime-spec/specs-go"
)

const (
	defaultNamespace   = "vps-provider"
	baseImage          = "docker.io/library/vps-base:latest"
	defaultMemoryLimit = 512 * 1024 * 1024 // 512MB
	defaultCPUMax      = "50000"            // 50% of 1 core
	defaultPidsLimit   = 64
	ttydPort           = 7681
)

type Client struct {
	client *containerd.Client
}

func NewClient(socketPath string) (*Client, error) {
	client, err := containerd.New(socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to containerd: %w", err)
	}
	return &Client{client: client}, nil
}

func (c *Client) Close() error {
	return c.client.Close()
}

func (c *Client) CreateContainer(ctx context.Context, sessionID string) (*ContainerInfo, error) {
	ctx = namespaces.WithNamespace(ctx, defaultNamespace)

	// Try to get image - nerdctl stores in default namespace usually
	image, err := c.client.GetImage(ctx, baseImage)
	if err != nil {
		// Try to pull or find in default namespace
		image, err = c.client.Pull(ctx, baseImage, containerd.WithPullUnpack)
		if err != nil {
			return nil, fmt.Errorf("image %s not found: %w", baseImage, err)
		}
	}

	// Build OCI spec
	opts := []oci.SpecOpts{
		oci.WithDefaultSpecForPlatform("linux/amd64"),
		oci.WithDefaultPathEnv,
		oci.WithProcessArgs("ttyd", "-p", fmt.Sprintf("%d", ttydPort), "-W", "/bin/bash"),
		oci.WithUser("user"),
		oci.WithAddedCapabilities([]string{"CAP_CHOWN", "CAP_DAC_OVERRIDE", "CAP_SETGID", "CAP_SETUID"}),
		oci.WithNoNewPrivileges,
		oci.WithLinuxNamespace(specs.LinuxNamespace{Type: specs.NetworkNamespace, Path: ""}),
	}

	// Create container
	container, err := c.client.NewContainer(
		ctx,
		sessionID,
		containerd.WithImage(image),
		containerd.WithNewSnapshot(sessionID+"-snapshot", image),
		containerd.WithNewSpec(opts...),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	// Create task
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		container.Delete(ctx, containerd.WithSnapshotCleanup)
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	// Start task
	if err := task.Start(ctx); err != nil {
		task.Delete(ctx, containerd.WithProcessKill)
		container.Delete(ctx, containerd.WithSnapshotCleanup)
		return nil, fmt.Errorf("failed to start task: %w", err)
	}

	// Wait for network setup
	time.Sleep(1 * time.Second)

	// Discover IP via nerdctl inspect
	ip, err := c.discoverIP(sessionID)
	if err != nil {
		ip = "" // Non-fatal
	}

	return &ContainerInfo{
		ContainerID: sessionID,
		TaskID:      task.ID(),
		IP:          ip,
		TTYDPort:    ttydPort,
	}, nil
}

func (c *Client) DestroyContainer(ctx context.Context, containerID string) error {
	ctx = namespaces.WithNamespace(ctx, defaultNamespace)

	container, err := c.client.LoadContainer(ctx, containerID)
	if err != nil {
		return fmt.Errorf("failed to load container: %w", err)
	}

	task, err := container.Task(ctx, cio.Load)
	if err == nil {
		task.Kill(ctx, 9)
		task.Delete(ctx, containerd.WithProcessKill)
	}

	if err := container.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
		return fmt.Errorf("failed to delete container: %w", err)
	}

	return nil
}

func (c *Client) discoverIP(containerID string) (string, error) {
	// Use nerdctl to inspect container in our namespace
	cmd := exec.Command("nerdctl", "-n", "vps-provider", "inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", containerID)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("nerdctl inspect failed: %w", err)
	}
	ip := strings.TrimSpace(string(out))
	if ip == "" {
		return "", fmt.Errorf("no IP found")
	}
	return ip, nil
}

type ContainerInfo struct {
	ContainerID string
	TaskID      string
	IP          string
	TTYDPort    int
}
