
package models

import "time"

type SessionStatus string

const (
	StatusCreating   SessionStatus = "creating"
	StatusRunning    SessionStatus = "running"
	StatusDetached   SessionStatus = "detached"
	StatusDestroying SessionStatus = "destroying"
	StatusDestroyed  SessionStatus = "destroyed"
)

type Session struct {
	ID          string        `json:"id"`
	Status      SessionStatus `json:"status"`
	ContainerID string        `json:"container_id,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
	ExpiresAt   time.Time     `json:"expires_at"`
	DetachedAt  *time.Time    `json:"detached_at,omitempty"`
}

type CreateSessionRequest struct {
	Template string `json:"template,omitempty"`
	GitURL   string `json:"git_url,omitempty"`
}

type CreateSessionResponse struct {
	Session
}

type SessionListResponse struct {
	Sessions []Session `json:"sessions"`
}
