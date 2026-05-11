package store

import (
	"sync"
	"time"

	"vps-provider/internal/models"
)

type MemoryStore struct {
	mu       sync.RWMutex
	sessions map[string]*models.Session
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{sessions: make(map[string]*models.Session)}
}

func (s *MemoryStore) Create(session *models.Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ID] = session
}

func (s *MemoryStore) Get(id string) (*models.Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	return sess, ok
}

func (s *MemoryStore) List() []models.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]models.Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		result = append(result, *sess)
	}
	return result
}

func (s *MemoryStore) UpdateStatus(id string, status models.SessionStatus) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return false
	}
	sess.Status = status
	if status == models.StatusDetached {
		now := time.Now()
		sess.DetachedAt = &now
	} else if status == models.StatusRunning {
		sess.DetachedAt = nil
	}
	return true
}

func (s *MemoryStore) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.sessions[id]
	if !ok {
		return false
	}
	delete(s.sessions, id)
	return true
}