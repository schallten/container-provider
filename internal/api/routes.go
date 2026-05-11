package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (h *Handler) RegisterRoutes() http.Handler {
	r := chi.NewRouter()
	
	r.Post("/api/v1/sessions", h.CreateSession)
	r.Get("/api/v1/sessions", h.ListSessions)
	r.Get("/api/v1/sessions/{id}", h.GetSession)
	r.Delete("/api/v1/sessions/{id}", h.DestroySession)
	r.Post("/api/v1/sessions/{id}/detach", h.MarkDetached) // Debug
	r.Get("/api/v1/sessions/{id}/terminal", h.TerminalWS)
	
	r.Get("/*", http.FileServer(http.Dir("./static")).ServeHTTP)
	
	return r
}