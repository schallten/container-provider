
package api

import (
	"bufio"
	"io"
	"log"
	"net/http"
	"os/exec"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (h *Handler) TerminalWS(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	log.Printf("[DEBUG] TerminalWS: id=%s", id)

	if id == "" {
		http.Error(w, "session id required", http.StatusBadRequest)
		return
	}

	session, ok := h.store.Get(id)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if session.Status != "running" {
		http.Error(w, "session not running", http.StatusBadRequest)
		return
	}

	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[ERROR] ws upgrade: %v", err)
		return
	}
	defer clientConn.Close()

	// Use script to force a PTY, no -t flag needed
	cmd := exec.Command("nerdctl", "-n", "vps-provider", "exec", "-i", session.ContainerID, 
		"script", "-q", "/dev/null", "/bin/bash")
	
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Printf("[ERROR] stdin pipe: %v", err)
		return
	}
	
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("[ERROR] stdout pipe: %v", err)
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("[ERROR] stderr pipe: %v", err)
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("[ERROR] start bash: %v", err)
		return
	}
	defer cmd.Process.Kill()

	log.Printf("[INFO] terminal connected: %s", id)

	// Forward: WebSocket -> bash stdin
	go func() {
		for {
			_, data, err := clientConn.ReadMessage()
			if err != nil {
				return
			}
			stdin.Write(append(data, '\n'))
		}
	}()

	// Forward: bash stdout/stderr -> WebSocket
	go func() {
		reader := io.MultiReader(stdout, stderr)
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			line := scanner.Text()
			clientConn.WriteMessage(websocket.TextMessage, []byte(line+"\r\n"))
		}
	}()

	cmd.Wait()
	log.Printf("[INFO] terminal disconnected: %s", id)
}