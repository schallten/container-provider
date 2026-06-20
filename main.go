package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net"
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
	ID             string            `json:"id"`
	Container      string            `json:"-"`
	CreatedAt      time.Time         `json:"created_at"`
	LastPing       time.Time         `json:"last_ping"`
	TunnelURL      string            `json:"tunnel_url,omitempty"`
	TunnelPID      int               `json:"-"`
	NoIdleTimeout  bool              `json:"no_idle_timeout"`
	NoMaxLifetime  bool              `json:"no_max_lifetime"`
	Tags           map[string]string `json:"tags"`
	SSHPort        int               `json:"ssh_port,omitempty"`
	SSHHostKey     string            `json:"-"`
}

var (
	envs       = sync.Map{} // ID -> *Env
	upgrader   = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	rateLimits = sync.Map{} // IP -> count
	logMutex   = sync.Mutex{}
	db         *DB
	billingDB  *BillingDB
	defaultUser = "user-default"
)

func main() {
	var err error
	db, err = NewDB("tempdev.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	billingDB, err = NewBillingDB("billing.db")
	if err != nil {
		log.Fatalf("Failed to open billing database: %v", err)
	}
	defer billingDB.Close()
	billingDB.GetOrCreateUser(defaultUser)

	// Load existing envs from DB and clean up orphans
	loaded := loadEnvsFromDB()
	orphaned := db.CleanupOrphans()
	if orphaned > 0 {
		log.Printf("🧹 Cleaned up %d orphaned env(s) from previous run", orphaned)
	}
	log.Printf("📦 Loaded %d env(s) from database", loaded)

	// HTTP routes
	http.HandleFunc("/env", handleCreateEnv)
	http.HandleFunc("/env/", handleEnvAction)
	http.HandleFunc("/ws/env/", handleShell)
	http.HandleFunc("/expose/", handleExpose)
	http.HandleFunc("/envs", handleListEnvs)
	http.HandleFunc("/settings/", handleSettings)
	http.HandleFunc("/billing", handleBilling)
	http.HandleFunc("/billing/topup", handleBillingTopup)
	http.HandleFunc("/billing/usage", handleBillingUsage)
	http.HandleFunc("/billing/costs", handleBillingCosts)
	http.HandleFunc("/logs", handleLogs)
	http.HandleFunc("/tags/", handleTags)
	http.HandleFunc("/location", handleLocation)
	http.HandleFunc("/ssh/", handleSSHDownload)
	http.Handle("/", http.FileServer(http.Dir("./public")))

	// Background goroutines
	go cleanupLoop()
	go abuseDetectLoop()
	go rateLimitResetLoop()

	log.Println("🚀 TempDev starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func loadEnvsFromDB() int {
	savedEnvs, err := db.GetActiveEnvs()
	if err != nil {
		log.Printf("DB: failed to load envs: %v", err)
		return 0
	}
	for _, env := range savedEnvs {
		envs.Store(env.ID, env)
	}
	return len(savedEnvs)
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

func findFreePort() int {
	// Use a range of high ports to avoid conflicts
	for port := 22000; port < 23000; port++ {
		addr := fmt.Sprintf(":%d", port)
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			ln.Close()
			return port
		}
	}
	return 22000
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

	// Write to SQLite
	db.LogEvent("info", eventType, envID, detail)

	// Also write to file
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

	// Check billing credits
	balance := billingDB.GetBalance(defaultUser)
	if balance < 10 {
		http.Error(w, "Insufficient credits. Please top up in Billing.", http.StatusPaymentRequired)
		return
	}

	// Create Docker container
	id := generateID()

	// Generate SSH keypair for this env
	sshDir := "ssh_keys"
	os.MkdirAll(sshDir, 0700)
	privPath := sshDir + "/" + id

	cmd := exec.Command("ssh-keygen", "-t", "ed25519", "-f", privPath, "-N", "", "-q")
	if err := cmd.Run(); err != nil {
		log.Printf("ssh-keygen failed: %v", err)
		http.Error(w, "Failed to generate SSH key", http.StatusInternalServerError)
		return
	}
	os.Chmod(privPath, 0600)

	pubKey, _ := os.ReadFile(privPath + ".pub")

	// Find a free host port for SSH
	sshPort := findFreePort()

	cmd = exec.Command("docker", "run", "-d",
		"--memory=512m",
		"--memory-swap=512m",
		"--cpus=0.5",
		"--pids-limit=64",
		"--cap-add=NET_ADMIN",
		"--security-opt=no-new-privileges",
		"--add-host", "aws-metadata:169.254.169.254",
		"-p", fmt.Sprintf("%d:2222", sshPort),
		"tempdev:latest",
	)

	output, err := cmd.Output()
	if err != nil {
		log.Printf("docker run failed: %v", err)
		logEvent("create_failed", id, err.Error())
		os.Remove(privPath)
		os.Remove(privPath + ".pub")
		http.Error(w, "Failed to create environment", http.StatusInternalServerError)
		return
	}

	container := strings.TrimSpace(string(output))

	// Inject public key into container for dev user
	injectCmd := exec.Command("docker", "exec", container, "bash", "-c",
		fmt.Sprintf("mkdir -p /home/dev/.ssh && chmod 700 /home/dev/.ssh && echo '%s' >> /home/dev/.ssh/authorized_keys && chmod 600 /home/dev/.ssh/authorized_keys && chown -R dev:dev /home/dev/.ssh", strings.TrimSpace(string(pubKey))))
	if err := injectCmd.Run(); err != nil {
		log.Printf("SSH key injection failed: %v", err)
	}

	env := &Env{
		ID:        id,
		Container: container,
		CreatedAt: time.Now(),
		LastPing:  time.Now(),
		Tags:      make(map[string]string),
		SSHPort:   sshPort,
	}

	envs.Store(id, env)
	db.InsertEnv(env)
	billingDB.DeductCredits(defaultUser, 5, id, "Environment created")
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

	// Clean up SSH keys
	os.Remove("ssh_keys/" + env.ID)
	os.Remove("ssh_keys/" + env.ID + ".pub")

	envs.Delete(env.ID)
	db.DeleteEnv(env.ID)
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

	// Start bash in container with PTY as non-root user
	bash := exec.Command("docker", "exec", "-u", "dev", "-it", env.Container, "bash")
	bash.Env = append(os.Environ(), "TERM=xterm")

	// Allocate PTY for proper terminal emulation
	ptmx, err := pty.Start(bash)
	if err != nil {
		log.Printf("PTY failed: %v", err)
		conn.WriteMessage(websocket.BinaryMessage, []byte("Error: "+err.Error()))
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

			// Check if it's a resize message (JSON)
			if len(msg) > 0 && msg[0] == '{' {
				var resize struct {
					Cols uint16 `json:"cols"`
					Rows uint16 `json:"rows"`
				}
				if err := json.Unmarshal(msg, &resize); err == nil {
					pty.Setsize(ptmx, &pty.Winsize{
						Cols: resize.Cols,
						Rows: resize.Rows,
					})
					continue
				}
			}

			ptmx.Write(msg)
			env.LastPing = time.Now()
			db.UpdatePing(env.ID, env.LastPing)
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
			if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
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
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/expose/"), "/")
	if len(parts) < 1 {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	id := parts[0]
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

	if r.Method == "DELETE" {
		// Kill cloudflared in container
		exec.Command("docker", "exec", env.Container, "pkill", "cloudflared").Run()
		env.TunnelURL = ""
		db.UpdateEnv(env)
		log.Printf("🛑 Unexposed env %s", id)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if len(parts) < 2 {
		http.Error(w, "Port required", http.StatusBadRequest)
		return
	}
	port := parts[1]

	// Validate port
	portNum, err := strconv.Atoi(port)
	if err != nil || portNum < 1 || portNum > 65535 {
		http.Error(w, "Port must be between 1 and 65535", http.StatusBadRequest)
		return
	}

	// Start cloudflared in background and capture its output
	// cloudflared prints the tunnel URL to stderr/stdout
	cmdStr := fmt.Sprintf(`
cloudflared tunnel --no-autoupdate --protocol http2 --url http://localhost:%s > /tmp/tunnel.log 2>&1 &
`, port)

	startCmd := exec.Command("docker", "exec", env.Container, "bash", "-c", cmdStr)
	if err := startCmd.Run(); err != nil {
		log.Printf("cloudflared start failed: %v", err)
	}

	// Poll for URL (max 10 seconds)
	tunnelURL := ""
	for i := 0; i < 20; i++ {
		time.Sleep(500 * time.Millisecond)
		catCmd := exec.Command("docker", "exec", env.Container, "cat", "/tmp/tunnel.log")
		output, err := catCmd.Output()
		if err != nil {
			log.Printf("Debug: cat /tmp/tunnel.log failed: %v", err)
			continue
		}

		// Debug: print log content to console
		if i % 4 == 0 {
			log.Printf("Debug: Tunnel log poll %d: %s", i, string(output))
		}

		// Look for https://*.trycloudflare.com
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "trycloudflare.com") {
				// Find index of https://
				idx := strings.Index(line, "https://")
				if idx != -1 {
					endIdx := strings.Index(line[idx:], " ")
					if endIdx == -1 {
						tunnelURL = strings.TrimSpace(line[idx:])
					} else {
						tunnelURL = strings.TrimSpace(line[idx : idx+endIdx])
					}
					// Clean up any trailing bars or characters
					tunnelURL = strings.TrimRight(tunnelURL, " |")
					if tunnelURL != "" {
						break
					}
				}
			}
		}
		if tunnelURL != "" {
			break
		}
	}

	// If we couldn't get the URL yet, tell user to check back
	if tunnelURL == "" {
		tunnelURL = "(Cloudflared still starting... refresh in a few seconds)"
	}

	env.TunnelURL = tunnelURL
	db.UpdateEnv(env)
	billingDB.DeductCredits(defaultUser, 2, id, "Tunnel exposed port "+port)

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
// Billing
// ============================================================================

func handleBilling(w http.ResponseWriter, r *http.Request) {
	balance := billingDB.GetBalance(defaultUser)
	txns, _ := billingDB.GetTransactions(defaultUser, 20)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user_id":     defaultUser,
		"balance":     balance,
		"transactions": txns,
	})
}

func handleBillingTopup(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Amount   int    `json:"amount"`
		CardNum  string `json:"card_number"`
		Expiry   string `json:"expiry"`
		CVV      string `json:"cvv"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	if body.Amount < 100 || body.Amount > 10000 {
		http.Error(w, "Amount must be between 100 and 10,000", http.StatusBadRequest)
		return
	}

	if len(body.CardNum) < 4 {
		http.Error(w, "Invalid card number", http.StatusBadRequest)
		return
	}

	// Simulate random bank failures (~25% chance)
	failRoll, _ := rand.Int(rand.Reader, big.NewInt(100))
	if failRoll.Int64() < 25 {
		bankErrors := []struct {
			code string
			msg  string
		}{
			{"CARD_DECLINED", "Your card was declined. Please try a different card."},
			{"INSUFFICIENT_FUNDS", "Insufficient funds on this card."},
			{"EXPIRED_CARD", "This card has expired."},
			{"PROCESSING_ERROR", "Bank processing error. Please try again later."},
			{"DO_NOT_HONOR", "Transaction not authorized by card issuer."},
			{"LIMIT_EXCEEDED", "Transaction limit exceeded. Try a smaller amount."},
		}
		errIdx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(bankErrors))))
		e := bankErrors[errIdx.Int64()]

		log.Printf("💳 Bank declined: %s — %s (card ****%s)", e.code, e.msg, body.CardNum[len(body.CardNum)-4:])

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":      false,
			"error":        e.msg,
			"bank_code":    e.code,
			"card_last4":   body.CardNum[len(body.CardNum)-4:],
		})
		return
	}

	last4 := body.CardNum[len(body.CardNum)-4:]
	billingDB.AddCredits(defaultUser, body.Amount, last4)
	balance := billingDB.GetBalance(defaultUser)

	log.Printf("💰 Top-up: +%d credits (card ****%s) new balance: %d", body.Amount, last4, balance)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":    true,
		"amount":     body.Amount,
		"balance":    balance,
		"card_last4": last4,
	})
}

func handleBillingUsage(w http.ResponseWriter, r *http.Request) {
	usage, _ := billingDB.GetUsage(defaultUser, 50)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(usage)
}

// ============================================================================
// Logs
// ============================================================================

func handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	filter := r.URL.Query().Get("filter")
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}

	logs, err := db.GetLogs(limit, filter)
	if err != nil {
		http.Error(w, "Failed to fetch logs", http.StatusInternalServerError)
		return
	}
	if logs == nil {
		logs = []map[string]interface{}{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

// ============================================================================
// Tags
// ============================================================================

func handleTags(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/tags/")
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

	if r.Method == "GET" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(env.Tags)
		return
	}

	if r.Method == "POST" || r.Method == "PUT" {
		var tags map[string]string
		if err := json.NewDecoder(r.Body).Decode(&tags); err != nil {
			http.Error(w, "Invalid body", http.StatusBadRequest)
			return
		}
		env.Tags = tags
		db.SetTags(id, tags)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(env.Tags)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// ============================================================================
// Billing - Cost Explorer
// ============================================================================

func handleBillingCosts(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 && parsed <= 90 {
			days = parsed
		}
	}

	byDay, _ := billingDB.GetCostsByDay(defaultUser, days)
	byEnv, _ := billingDB.GetCostsByEnv(defaultUser, days)

	if byDay == nil {
		byDay = []map[string]interface{}{}
	}
	if byEnv == nil {
		byEnv = []map[string]interface{}{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"by_day": byDay,
		"by_env": byEnv,
	})
}

// ============================================================================
// Settings
// ============================================================================

func handleSettings(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/settings/"), "/")
	if len(parts) < 2 {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	id := parts[0]
	action := parts[1]

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

	if action == "no-idle-timeout" && r.Method == "POST" {
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "Invalid body", http.StatusBadRequest)
			return
		}
		env.NoIdleTimeout = body.Enabled
		db.SetNoIdleTimeout(id, body.Enabled)
		log.Printf("Settings: env %s no_idle_timeout=%v", id, body.Enabled)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"no_idle_timeout": body.Enabled})
		return
	}

	if action == "no-max-lifetime" && r.Method == "POST" {
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "Invalid body", http.StatusBadRequest)
			return
		}
		env.NoMaxLifetime = body.Enabled
		db.SetNoMaxLifetime(id, body.Enabled)
		log.Printf("Settings: env %s no_max_lifetime=%v", id, body.Enabled)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"no_max_lifetime": body.Enabled})
		return
	}

	if action == "get" && r.Method == "GET" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{
			"no_idle_timeout": env.NoIdleTimeout,
			"no_max_lifetime": env.NoMaxLifetime,
		})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// ============================================================================
// SSH Key Download
// ============================================================================

func handleSSHDownload(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/ssh/")
	if !sanitizeEnvID(id) {
		http.Error(w, "Invalid env ID", http.StatusBadRequest)
		return
	}

	privPath := "ssh_keys/" + id
	privKey, err := os.ReadFile(privPath)
	if err != nil {
		http.Error(w, "SSH key not found for this environment", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="tempdev-%s.pem"`, id))
	w.Write(privKey)
}

// ============================================================================
// Location Cache
// ============================================================================

func handleLocation(w http.ResponseWriter, r *http.Request) {
	// Check cache first (1 hour TTL)
	if cached, ok := db.CacheGet("location"); ok {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		w.Write([]byte(cached))
		return
	}

	// Fetch from external API
	resp, err := http.Get("https://ipapi.co/json/")
	if err != nil {
		http.Error(w, "Location unavailable", http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		http.Error(w, "Failed to parse location", http.StatusInternalServerError)
		return
	}

	result, _ := json.Marshal(data)
	db.CacheSet("location", string(result), time.Hour)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	w.Write(result)
}

// ============================================================================
// Cleanup & Monitoring
// ============================================================================

func cleanupLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	// Per-minute billing: run every 60s
	billingTicker := time.NewTicker(60 * time.Second)
	defer billingTicker.Stop()

	go func() {
		for range billingTicker.C {
			envs.Range(func(k, v interface{}) bool {
				env := v.(*Env)
				// 3 credits per minute
				if err := billingDB.DeductCredits(defaultUser, 3, env.ID, "Container runtime (1 min)"); err != nil {
					log.Printf("💸 Billing: env %s — insufficient credits, terminating", env.ID)
					toDelete := append([]*Env(nil), env)
					for _, e := range toDelete {
						deleteEnv(e)
					}
					return false
				}
				return true
			})
		}
	}()

	for range ticker.C {
		now := time.Now()
		var toDelete []*Env

		envs.Range(func(k, v interface{}) bool {
			env := v.(*Env)
			age := now.Sub(env.CreatedAt)
			idle := now.Sub(env.LastPing)

			if age > 12*time.Hour && !env.NoMaxLifetime {
				log.Printf("⏱  Cleanup: env %s exceeded max lifetime (12h)", env.ID)
				toDelete = append(toDelete, env)
				return true
			}

			if idle > 15*time.Minute && !env.NoIdleTimeout {
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
		ID        string            `json:"id"`
		CreatedAt string            `json:"created_at"`
		LastPing  string            `json:"last_ping"`
		Uptime    string            `json:"uptime"`
		Idle      string            `json:"idle"`
		TunnelURL string            `json:"tunnel_url,omitempty"`
		Tags      map[string]string `json:"tags"`
		SSHPort   int               `json:"ssh_port,omitempty"`
	}

	var envList []*EnvStatus
	now := time.Now()

	envs.Range(func(k, v interface{}) bool {
		env := v.(*Env)

		age := now.Sub(env.CreatedAt)
		idle := now.Sub(env.LastPing)

		tags := env.Tags
		if tags == nil {
			tags = map[string]string{}
		}

		envList = append(envList, &EnvStatus{
			ID:        env.ID,
			CreatedAt: env.CreatedAt.Format(time.RFC3339),
			LastPing:  env.LastPing.Format(time.RFC3339),
			Uptime:    formatDuration(age),
			Idle:      formatDuration(idle),
			TunnelURL: env.TunnelURL,
			Tags:      tags,
			SSHPort:   env.SSHPort,
		})

		return true
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(envList)
}