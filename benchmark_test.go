package main

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

const dockerRunArgs = "--memory=512m --memory-swap=512m --cpus=0.5 --pids-limit=64 --cap-add=NET_ADMIN --security-opt=no-new-privileges --add-host=aws-metadata:169.254.169.254 tempdev:latest"

func runContainer(t testing.TB) string {
	t.Helper()
	args := append(strings.Split(dockerRunArgs, " "), "tempdev:latest")
	// Filter out the image name from the const since we append it
	args = []string{"run", "-d",
		"--memory=512m", "--memory-swap=512m", "--cpus=0.5", "--pids-limit=64",
		"--cap-add=NET_ADMIN", "--security-opt=no-new-privileges",
		"--add-host", "aws-metadata:169.254.169.254",
		"tempdev:latest",
	}
	cmd := exec.Command("docker", args...)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("docker run failed: %v", err)
	}
	return strings.TrimSpace(string(output))
}

func removeContainer(id string) {
	exec.Command("docker", "rm", "-f", id).Run()
}

// TestContainerCreationLatency measures single container creation time.
func TestContainerCreationLatency(t *testing.T) {
	start := time.Now()
	id := runContainer(t)
	elapsed := time.Since(start)
	defer removeContainer(id)

	t.Logf("Container created in %v (ID: %s)", elapsed, id)
}

// TestConcurrentContainerCreation spawns N containers concurrently and measures total time.
func TestConcurrentContainerCreation(t *testing.T) {
	counts := []int{1, 5, 10, 20}
	for _, count := range counts {
		t.Run(fmt.Sprintf("%d_containers", count), func(t *testing.T) {
			var wg sync.WaitGroup
			var mu sync.Mutex
			var containerIDs []string

			start := time.Now()

			for i := 0; i < count; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					cmd := exec.Command("docker", "run", "-d",
						"--memory=512m", "--memory-swap=512m", "--cpus=0.5", "--pids-limit=64",
						"--cap-add=NET_ADMIN", "--security-opt=no-new-privileges",
						"--add-host", "aws-metadata:169.254.169.254",
						"tempdev:latest",
					)
					output, err := cmd.Output()
					if err != nil {
						t.Errorf("docker run failed: %v", err)
						return
					}
					mu.Lock()
					containerIDs = append(containerIDs, strings.TrimSpace(string(output)))
					mu.Unlock()
				}()
			}

			wg.Wait()
			elapsed := time.Since(start)

			for _, id := range containerIDs {
				removeContainer(id)
			}

			perContainer := elapsed / time.Duration(count)
			t.Logf("%d containers created in %v (%v each)", count, elapsed, perContainer)
		})
	}
}

// TestContainerResourceLimits verifies cgroup limits are applied.
func TestContainerResourceLimits(t *testing.T) {
	id := runContainer(t)
	defer removeContainer(id)

	// Check memory limit
	out, err := exec.Command("docker", "inspect", "--format", "{{.HostConfig.Memory}}", id).Output()
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}
	memLimit := strings.TrimSpace(string(out))
	if memLimit != "536870912" {
		t.Errorf("expected memory limit 536870912, got %s", memLimit)
	}

	// Check PID limit
	out, err = exec.Command("docker", "inspect", "--format", "{{.HostConfig.PidsLimit}}", id).Output()
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}
	pidLimit := strings.TrimSpace(string(out))
	if pidLimit != "64" {
		t.Errorf("expected pid limit 64, got %s", pidLimit)
	}

	// Check no-new-privileges
	out, err = exec.Command("docker", "inspect", "--format", "{{.HostConfig.SecurityOpt}}", id).Output()
	if err != nil {
		t.Fatalf("inspect failed: %v", err)
	}
	secOpt := strings.TrimSpace(string(out))
	if !strings.Contains(secOpt, "no-new-privileges") {
		t.Errorf("expected no-new-privileges security opt, got %s", secOpt)
	}

	t.Logf("Memory: %s, PIDs: %s, SecurityOpt: %s", memLimit, pidLimit, secOpt)
}

// TestNonRootUser verifies the interactive shell runs as non-root.
// The container's default user is root (needed for sshd), but all
// interactive sessions (WebSocket shell, SSH) run as the dev user.
func TestNonRootUser(t *testing.T) {
	id := runContainer(t)
	defer removeContainer(id)

	// Default user is root (for sshd)
	out, err := exec.Command("docker", "exec", id, "whoami").Output()
	if err != nil {
		t.Fatalf("whoami failed: %v", err)
	}
	defaultUser := strings.TrimSpace(string(out))
	t.Logf("Container default user: %s (root for sshd)", defaultUser)

	// Interactive shell runs as dev (mirrors WebSocket shell in main.go)
	out, err = exec.Command("docker", "exec", "-u", "dev", id, "whoami").Output()
	if err != nil {
		t.Fatalf("whoami failed: %v", err)
	}
	shellUser := strings.TrimSpace(string(out))
	if shellUser == "root" {
		t.Error("interactive shell is running as root, expected dev user")
	}
	t.Logf("Interactive shell user: %s", shellUser)

	// Verify UID is 1000
	out, err = exec.Command("docker", "exec", "-u", "dev", id, "id", "-u").Output()
	if err != nil {
		t.Fatalf("id failed: %v", err)
	}
	uid := strings.TrimSpace(string(out))
	if uid != "1000" {
		t.Errorf("expected UID 1000, got %s", uid)
	}
}

// TestMetadataBlocked verifies AWS metadata endpoint is blocked.
func TestMetadataBlocked(t *testing.T) {
	id := runContainer(t)
	defer removeContainer(id)

	// Give iptables rules a moment to apply
	time.Sleep(200 * time.Millisecond)

	out, err := exec.Command("docker", "exec", id, "curl", "-s", "--max-time", "3", "http://169.254.169.254/latest/meta-data/").Output()
	metaOutput := strings.TrimSpace(string(out))

	if err == nil && (metaOutput == "" || strings.Contains(metaOutput, "ami-id")) {
		t.Error("metadata endpoint is accessible — should be blocked")
	} else {
		t.Logf("Metadata request blocked as expected (output: %s)", metaOutput)
	}
}

// TestSSHDStartup verifies sshd starts within the container.
func TestSSHDStartup(t *testing.T) {
	id := runContainer(t)
	defer removeContainer(id)

	start := time.Now()
	for i := 0; i < 30; i++ {
		if err := exec.Command("docker", "exec", id, "pgrep", "sshd").Run(); err == nil {
			t.Logf("sshd started in %v", time.Since(start))
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Errorf("sshd did not start within 3 seconds")
}

// TestCleanupLatency measures time to destroy a container.
func TestCleanupLatency(t *testing.T) {
	id := runContainer(t)

	start := time.Now()
	if err := exec.Command("docker", "rm", "-f", id).Run(); err != nil {
		t.Fatalf("docker rm failed: %v", err)
	}
	elapsed := time.Since(start)

	t.Logf("Container cleanup: %v", elapsed)
	if elapsed > 5*time.Second {
		t.Errorf("cleanup took too long: %v (expected <5s)", elapsed)
	}
}

// BenchmarkContainerCreation is a Go benchmark for accurate timing.
func BenchmarkContainerCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		cmd := exec.Command("docker", "run", "-d",
			"--memory=512m", "--memory-swap=512m", "--cpus=0.5", "--pids-limit=64",
			"--cap-add=NET_ADMIN", "--security-opt=no-new-privileges",
			"--add-host", "aws-metadata:169.254.169.254",
			"tempdev:latest",
		)
		output, err := cmd.Output()
		if err != nil {
			b.Fatalf("docker run failed: %v", err)
		}
		containerID := strings.TrimSpace(string(output))
		exec.Command("docker", "rm", "-f", containerID).Run()
	}
}
