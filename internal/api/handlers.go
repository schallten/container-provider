//cat > ~/vps-provider/container-provider/internal/api/handlers.go << 'EOF'
package api

import (
	"encoding/json"
	"net/http"

	"vps-provider/internal/container"
	"vps-provider/internal/models"
	"vps-provider/internal/store"
)

type Handler struct {
	lifecycle *container.LifecycleManager
	store     *store.MemoryStore
}

func NewHandler(lm *container.LifecycleManager, store *store.MemoryStore) *Handler {
	return &Handler{lifecycle: lm, store: store}
}

func (h *Handler) CreateSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req models.CreateSessionRequest
	json.NewDecoder(r.Body).Decode(&req)
	
	ctx := r.Context()
	session, err := h.lifecycle.CreateSession(ctx, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(session)
}

func (h *Handler) GetSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "session id required", http.StatusBadRequest)
		return
	}
	session, ok := h.store.Get(id)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(session)
}

func (h *Handler) ListSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sessions := h.store.List()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.SessionListResponse{Sessions: sessions})
}

func (h *Handler) DestroySession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "session id required", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if err := h.lifecycle.DestroySession(ctx, id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ExposePort(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "session id required", http.StatusBadRequest)
		return
	}

	session, ok := h.store.Get(id)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	var req models.ExposePortRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	// Start cloudflared INSIDE the container
	info, err := container.StartTunnel(r.Context(), id, session.ContainerID, req.ContainerPort)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	exposed := models.ExposedPort{
		ContainerPort: req.ContainerPort,
		PublicURL:     info.URL,
	}
	session.ExposedPorts = append(session.ExposedPorts, exposed)
	h.store.UpdateStatus(id, session.Status)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(models.ExposePortResponse{PublicURL: info.URL})
}

func (h *Handler) MarkDetached(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "session id required", http.StatusBadRequest)
		return
	}
	h.store.UpdateStatus(id, models.StatusDetached)
	w.WriteHeader(http.StatusNoContent)
}
