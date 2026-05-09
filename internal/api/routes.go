package api

import (
	"net/http"
)

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/sessions", h.CreateSession)
	mux.HandleFunc("GET /api/v1/sessions", h.ListSessions)
	mux.HandleFunc("GET /api/v1/sessions/{id}", h.GetSession)
	mux.HandleFunc("DELETE /api/v1/sessions/{id}", h.DestroySession)
}
