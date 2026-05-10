package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (h *Handler) RegisterRoutes() http.Handler {
	r := chi.NewRouter()

	// API routes
	r.Post("/api/v1/sessions", h.CreateSession)
	r.Get("/api/v1/sessions", h.ListSessions)
	r.Get("/api/v1/sessions/{id}", h.GetSession)
	r.Delete("/api/v1/sessions/{id}", h.DestroySession)
	r.Get("/api/v1/sessions/{id}/terminal", h.TerminalWS)

	// Static files
	r.Get("/*", http.FileServer(http.Dir("./static")).ServeHTTP)

	return r
}
