package api

import (
	"io"
	"log"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"
	"vps-provider/internal/models"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func (h *Handler) TerminalWS(w http.ResponseWriter, r *http.Request) {
	log.Printf("[DEBUG] TerminalWS called: %s %s", r.Method, r.URL.Path)
	
	id := r.PathValue("id")
	log.Printf("[DEBUG] extracted id: %s", id)
	
	if id == "" {
		log.Printf("[DEBUG] id is empty, returning 400")
		http.Error(w, "session id required", http.StatusBadRequest)
		return
	}

	session, ok := h.store.Get(id)
	log.Printf("[DEBUG] store.Get returned: ok=%v, session=%+v", ok, session)
	
	if !ok {
		log.Printf("[DEBUG] session not found, returning 404")
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	if session.Status != models.StatusRunning {
		log.Printf("[DEBUG] session not running, status=%s, returning 400", session.Status)
		http.Error(w, "session not running", http.StatusBadRequest)
		return
	}

	if session.IP == "" {
		log.Printf("[DEBUG] session has no IP, returning 500")
		http.Error(w, "session has no IP", http.StatusInternalServerError)
		return
	}

	log.Printf("[DEBUG] attempting WebSocket upgrade")
	
	// Upgrade client connection to WebSocket
	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[ERROR] ws upgrade failed: %v", err)
		return
	}
	defer clientConn.Close()

	log.Printf("[DEBUG] WebSocket upgraded, connecting to ttyd at %s:7681", session.IP)

	// Connect to ttyd inside container
	ttydURL := url.URL{Scheme: "ws", Host: session.IP + ":7681", Path: "/ws"}
	ttydConn, _, err := websocket.DefaultDialer.Dial(ttydURL.String(), nil)
	if err != nil {
		log.Printf("[ERROR] failed to connect to ttyd at %s: %v", ttydURL.String(), err)
		clientConn.WriteMessage(websocket.TextMessage, []byte("Failed to connect to container terminal"))
		return
	}
	defer ttydConn.Close()

	// Update status to running (in case it was detached)
	h.store.UpdateStatus(id, models.StatusRunning)

	log.Printf("[INFO] terminal connected for session %s", id)

	// Bidirectional copy
	errChan := make(chan error, 2)

	go func() {
		for {
			msgType, data, err := ttydConn.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}
			if err := clientConn.WriteMessage(msgType, data); err != nil {
				errChan <- err
				return
			}
		}
	}()

	go func() {
		for {
			msgType, data, err := clientConn.ReadMessage()
			if err != nil {
				errChan <- err
				return
			}
			if err := ttydConn.WriteMessage(msgType, data); err != nil {
				errChan <- err
				return
			}
		}
	}()

	// Wait for either direction to fail
	err = <-errChan
	log.Printf("[INFO] terminal disconnected for session %s: %v", id, err)

	// Mark as detached
	h.store.UpdateStatus(id, models.StatusDetached)
}
