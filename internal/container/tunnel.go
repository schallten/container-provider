// cat > ~/vps-provider/container-provider/internal/container/tunnel.go << 'EOF'
package container

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	tunnelRegex = regexp.MustCompile(`https://[a-zA-Z0-9-]+\.trycloudflare\.com`)
	tunnelMu    sync.Mutex
	tunnels     = make(map[string]*exec.Cmd)
)

type TunnelInfo struct {
	URL string `json:"url"`
}

func StartTunnel(ctx context.Context, sessionID string, containerID string, localPort int) (*TunnelInfo, error) {
	tunnelMu.Lock()
	defer tunnelMu.Unlock()

	// Kill existing tunnel if any
	if oldCmd, ok := tunnels[sessionID]; ok {
		oldCmd.Process.Kill()
		delete(tunnels, sessionID)
	}

	// Step 1: Copy cloudflared from host to container
	cpCmd := exec.Command("nerdctl", "-n", "vps-provider", "cp", "/usr/local/bin/cloudflared", containerID+":/tmp/cloudflared")
	if out, err := cpCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to copy cloudflared: %w\noutput: %s", err, string(out))
	}

	// Step 2: Make it executable inside container
	chmodCmd := exec.Command("nerdctl", "-n", "vps-provider", "exec", containerID, "chmod", "+x", "/tmp/cloudflared")
	if out, err := chmodCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to chmod cloudflared: %w\noutput: %s", err, string(out))
	}

	// Step 3: Run cloudflared inside container
	cmd := exec.Command("nerdctl", "-n", "vps-provider", "exec", containerID,
		"/tmp/cloudflared", "tunnel", "--url", fmt.Sprintf("http://127.0.0.1:%d", localPort))

	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start cloudflared: %w", err)
	}

	tunnels[sessionID] = cmd

	// Read output with timeout
	urlChan := make(chan string, 1)
	go func() {
		reader := bufio.NewReader(pipe)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				break
			}
			log.Printf("[TUNNEL] %s: %s", sessionID, strings.TrimSpace(line))
			if matches := tunnelRegex.FindString(line); matches != "" {
				urlChan <- matches
				return
			}
		}
	}()

	select {
	case url := <-urlChan:
		log.Printf("[INFO] tunnel started for %s: %s", sessionID, url)
		go func() {
			cmd.Wait()
			tunnelMu.Lock()
			delete(tunnels, sessionID)
			tunnelMu.Unlock()
			log.Printf("[INFO] tunnel exited for %s", sessionID)
		}()
		return &TunnelInfo{URL: strings.TrimSpace(url)}, nil
	case <-time.After(30 * time.Second):
		cmd.Process.Kill()
		delete(tunnels, sessionID)
		return nil, fmt.Errorf("timeout waiting for tunnel URL")
	}
}

func StopTunnel(sessionID string) {
	tunnelMu.Lock()
	defer tunnelMu.Unlock()

	if cmd, ok := tunnels[sessionID]; ok {
		cmd.Process.Kill()
		delete(tunnels, sessionID)
		log.Printf("[INFO] tunnel stopped for %s", sessionID)
	}
}
// EOF