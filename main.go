package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

// Env represents a temporary development environment
type Env struct {
	ID        string    `json:"id"`
	Container string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
	LastPing  time.Time `json:"last_ping"`
	TunnelURL string    `json:"tunnel_url,omitempty"`
	TunnelPID int       `json:"-"`
}

var (
	envs       = sync.Map{} // ID -> *Env
	upgrader   = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	rateLimits = sync.Map{} // IP -> count
	logMutex   = sync.Mutex{}
)

func main() {
	// HTTP routes
	http.HandleFunc("/env", handleCreateEnv)
	http.HandleFunc("/env/", handleEnvAction)
	http.HandleFunc("/ws/env/", handleShell)
	http.HandleFunc("/expose/", handleExpose)
	http.HandleFunc("/envs", handleListEnvs)
	http.Handle("/", http.FileServer(http.Dir("./public")))

	// Background goroutines
	go cleanupLoop()
	go abuseDetectLoop()
	go rateLimitResetLoop()

	log.Println("🚀 TempDev starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// ============================================================================
// Helpers
// ============================================================================

func generateID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func sanitizeEnvID(id string) bool {
	if len(id) == 0 || len(id) > 16 {
		return false
	}
	for _, c := range id {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

func extractIP(r *http.Request) string {
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.Header.Get("X-Real-IP")
	}
	if ip == "" {
		ip = strings.Split(r.RemoteAddr, ":")[0]
	}
	return ip
}

func logEvent(eventType, envID, detail string) {
	logMutex.Lock()
	defer logMutex.Unlock()

	logEntry := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"event":     eventType,
		"env_id":    envID,
		"detail":    detail,
	}

	data, _ := json.Marshal(logEntry)

	file, err := os.OpenFile("events.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("Failed to log event: %v", err)
		return
	}
	defer file.Close()
	file.Write(append(data, '\n'))
}

func formatDuration(d time.Duration) string {
	total := int(d.Seconds())
	hours := total / 3600
	minutes := (total % 3600) / 60
	seconds := total % 60

	if hours > 0 {
		return fmt.Sprintf("%dh%dm", hours, minutes)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm%ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// ============================================================================
// Env Creation & Management
// ============================================================================

func handleCreateEnv(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	ip := extractIP(r)

	// Rate limit: 5 envs per hour per IP
	val, _ := rateLimits.LoadOrStore(ip, 0)
	count := val.(int)
	if count >= 5 {
		logEvent("rate_limit_exceeded", "", ip)
		http.Error(w, "Rate limit exceeded (5 per hour)", http.StatusTooManyRequests)
		return
	}
	rateLimits.Store(ip, count+1)

	// Create Docker container
	id := generateID()

	cmd := exec.Command("docker", "run", "-d",
		"--memory=512m",
		"--memory-swap=512m",
		"--cpus=0.5",
		"--pids-limit=64",
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"-u", "dev",
		"tempdev:latest",
		"sleep", "infinity",
	)

	output, err := cmd.Output()
	if err != nil {
		log.Printf("docker run failed: %v", err)
		logEvent("create_failed", id, err.Error())
		http.Error(w, "Failed to create environment", http.StatusInternalServerError)
		return
	}

	container := strings.TrimSpace(string(output))
	env := &Env{
		ID:        id,
		Container: container,
		CreatedAt: time.Now(),
		LastPing:  time.Now(),
	}

	envs.Store(id, env)
	logEvent("env_created", id, container[:12])
	log.Printf("✓ Created env %s (container %s)", id, container[:12])

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"id":     id,
		"ws_url": "/ws/env/" + id,
	})
}

func handleEnvAction(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/env/")
	id = strings.Split(id, "/")[0]

	if !sanitizeEnvID(id) {
		http.Error(w, "Invalid env ID", http.StatusBadRequest)
		return
	}

	val, ok := envs.Load(id)
	if !ok {
		http.Error(w, "Environment not found", http.StatusNotFound)
		return
	}

	env := val.(*Env)

	switch r.Method {
	case "GET":
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(env)

	case "DELETE":
		deleteEnv(env)
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func deleteEnv(env *Env) {
	// Kill Docker container
	exec.Command("docker", "rm", "-f", env.Container).Run()

	// Kill tunnel if exists
	if env.TunnelPID > 0 {
		exec.Command("kill", "-9", fmt.Sprintf("%d", env.TunnelPID)).Run()
	}

	envs.Delete(env.ID)
	logEvent("env_deleted", env.ID, "")
	log.Printf("✗ Deleted env %s", env.ID)
}

// ============================================================================
// WebSocket Shell
// ============================================================================

func handleShell(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/ws/env/")

	if !sanitizeEnvID(id) {
		http.Error(w, "Invalid env ID", http.StatusBadRequest)
		return
	}

	val, ok := envs.Load(id)
	if !ok {
		http.Error(w, "Environment not found", http.StatusNotFound)
		return
	}

	env := val.(*Env)

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// Start bash in container with PTY
	bash := exec.Command("docker", "exec", "-it", env.Container, "bash")

	// Allocate PTY for proper terminal emulation
	ptmx, err := pty.Start(bash)
	if err != nil {
		log.Printf("PTY failed: %v", err)
		conn.WriteMessage(websocket.TextMessage, []byte("Error: "+err.Error()))
		return
	}
	defer ptmx.Close()
	defer bash.Process.Kill()

	// Client input → container
	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			ptmx.Write(msg)
			env.LastPing = time.Now()
		}
	}()

	// Container output → client
	buf := make([]byte, 4096)
	for {
		n, err := ptmx.Read(buf)
		if err != nil {
			break
		}
		if n > 0 {
			if err := conn.WriteMessage(websocket.TextMessage, buf[:n]); err != nil {
				break
			}
		}
	}

	logEvent("shell_closed", env.ID, "")
}

// ============================================================================
// Tunnel (Cloudflared)
// ============================================================================

func handleExpose(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/expose/"), "/")
	if len(parts) < 2 {
		http.Error(w, "Invalid request: /expose/:id/:port", http.StatusBadRequest)
		return
	}

	id := parts[0]
	port := parts[1]

	if !sanitizeEnvID(id) {
		http.Error(w, "Invalid env ID", http.StatusBadRequest)
		return
	}

	// Validate port
	portNum, err := strconv.Atoi(port)
	if err != nil || portNum < 1 || portNum > 65535 {
		http.Error(w, "Port must be between 1 and 65535", http.StatusBadRequest)
		return
	}

	val, ok := envs.Load(id)
	if !ok {
		http.Error(w, "Environment not found", http.StatusNotFound)
		return
	}

	env := val.(*Env)

	// Start cloudflared in background and capture its output
	// cloudflared prints the tunnel URL to stdout when it starts
	cmdStr := fmt.Sprintf(`
cloudflared tunnel --no-autoupdate run --url http://localhost:%s 2>&1 | tee /tmp/tunnel.log | grep -m1 "https://" &
`, port)

	startCmd := exec.Command("docker", "exec", "-d", env.Container, "bash", "-c", cmdStr)
	if err := startCmd.Run(); err != nil {
		log.Printf("cloudflared start failed: %v", err)
	}

	// Wait a moment for cloudflared to print the URL
	time.Sleep(2 * time.Second)

	// Try to read the URL from the log file
	catCmd := exec.Command("docker", "exec", env.Container, "cat", "/tmp/tunnel.log")
	output, err := catCmd.Output()
	tunnelURL := ""

	if err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "https://") && strings.Contains(line, "trycloudflare") {
				// Extract URL from line like: "2024-01-15 10:30:00 https://abc123.trycloudflare.com"
				parts := strings.Fields(line)
				for _, part := range parts {
					if strings.HasPrefix(part, "https://") {
						tunnelURL = strings.TrimSpace(part)
						break
					}
				}
				if tunnelURL != "" {
					break
				}
			}
		}
	}

	// If we couldn't get the URL yet, tell user to check back
	if tunnelURL == "" {
		tunnelURL = "(cloudflared starting... check again in 10 seconds)"
	}

	env.TunnelURL = tunnelURL

	logEvent("tunnel_created", env.ID, port)
	log.Printf("🔗 Exposed port %s for env %s: %s", port, id, tunnelURL)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"tunnel_url": tunnelURL,
		"port":       port,
		"note":       "Cloudflared generates a unique HTTPS URL. If not shown, wait 10-15 seconds and refresh.",
		"how_to":     "Inside terminal, run: cloudflared tunnel --url http://localhost:PORT to see the URL",
	})
}

// ============================================================================
// Cleanup & Monitoring
// ============================================================================

func cleanupLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		var toDelete []*Env

		envs.Range(func(k, v interface{}) bool {
			env := v.(*Env)
			age := now.Sub(env.CreatedAt)
			idle := now.Sub(env.LastPing)

			if age > 12*time.Hour {
				log.Printf("⏱  Cleanup: env %s exceeded max lifetime (12h)", env.ID)
				toDelete = append(toDelete, env)
				return true
			}

			if idle > 15*time.Minute {
				log.Printf("⏱  Cleanup: env %s idle for 15m", env.ID)
				toDelete = append(toDelete, env)
				return true
			}

			return true
		})

		for _, env := range toDelete {
			deleteEnv(env)
		}
	}
}

func abuseDetectLoop() {
	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()

	blacklist := []string{
		"xmrig", "miner", "stratum", "masscan", "nmap", "zmap",
		"hydra", "john", "hashcat", "nikto", "sqlmap",
		"metasploit", "aircrack", "airmon", "bettercap",
	}

	for range ticker.C {
		envs.Range(func(k, v interface{}) bool {
			env := v.(*Env)

			// Get process list
			cmd := exec.Command("docker", "exec", env.Container, "ps", "aux")
			output, err := cmd.Output()
			if err != nil {
				return true
			}

			psOutput := strings.ToLower(string(output))

			// Check for blacklisted processes
			for _, pattern := range blacklist {
				if strings.Contains(psOutput, pattern) {
					log.Printf("🚨 ABUSE: env %s detected %s", env.ID, pattern)
					deleteEnv(env)
					logEvent("abuse_detected", env.ID, pattern)
					return true
				}
			}

			// Check memory usage (simplified)
			// statsCmd := exec.Command("docker", "stats", "--no-stream", env.Container)
			// statsOutput, _ := statsCmd.Output()
			// (implement memory/CPU checks if needed)

			return true
		})
	}
}

func rateLimitResetLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		var ips []string
		rateLimits.Range(func(k, v interface{}) bool {
			ips = append(ips, k.(string))
			return true
		})

		for _, ip := range ips {
			rateLimits.Delete(ip)
		}

		log.Printf("🔄 Rate limits reset (%d IPs)", len(ips))
	}
}

// ============================================================================
// Status & Monitoring
// ============================================================================

func handleListEnvs(w http.ResponseWriter, r *http.Request) {
	type EnvStatus struct {
		ID        string `json:"id"`
		CreatedAt string `json:"created_at"`
		LastPing  string `json:"last_ping"`
		Uptime    string `json:"uptime"`
		Idle      string `json:"idle"`
		TunnelURL string `json:"tunnel_url,omitempty"`
	}

	var envList []*EnvStatus
	now := time.Now()

	envs.Range(func(k, v interface{}) bool {
		env := v.(*Env)

		age := now.Sub(env.CreatedAt)
		idle := now.Sub(env.LastPing)

		envList = append(envList, &EnvStatus{
			ID:        env.ID,
			CreatedAt: env.CreatedAt.Format(time.RFC3339),
			LastPing:  env.LastPing.Format(time.RFC3339),
			Uptime:    formatDuration(age),
			Idle:      formatDuration(idle),
			TunnelURL: env.TunnelURL,
		})

		return true
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(envList)
}