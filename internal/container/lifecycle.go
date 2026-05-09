package container

import (
	"context"
	"fmt"
	"log"
	"time"

	"vps-provider/internal/models"
	"vps-provider/internal/store"
)

type LifecycleManager struct {
	client *Client
	store  *store.MemoryStore
}

func NewLifecycleManager(client *Client, store *store.MemoryStore) *LifecycleManager {
	return &LifecycleManager{
		client: client,
		store:  store,
	}
}

func (lm *LifecycleManager) CreateSession(ctx context.Context, req models.CreateSessionRequest) (*models.Session, error) {
	sessionID := generateSessionID()

	session := &models.Session{
		ID:        sessionID,
		Status:    models.StatusCreating,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(12 * time.Hour),
	}

	lm.store.Create(session)

	// Create container asynchronously
	go func() {
		// Recover from panics
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[PANIC] in container creation for %s: %v", sessionID, r)
				lm.store.UpdateStatus(sessionID, models.StatusDestroyed)
			}
		}()

		log.Printf("[DEBUG] starting container creation for session %s", sessionID)
		
		info, err := lm.client.CreateContainer(context.Background(), sessionID)
		if err != nil {
			log.Printf("[ERROR] failed to create container for session %s: %v", sessionID, err)
			lm.store.UpdateStatus(sessionID, models.StatusDestroyed)
			return
		}

		log.Printf("[INFO] container created for session %s: ip=%s port=%d", sessionID, info.IP, info.TTYDPort)

		// Update session with container info
		session.ContainerID = info.ContainerID
		session.IP = info.IP
		session.TTYDPort = info.TTYDPort
		lm.store.UpdateStatus(sessionID, models.StatusRunning)
	}()

	return session, nil
}

func (lm *LifecycleManager) DestroySession(ctx context.Context, sessionID string) error {
	session, ok := lm.store.Get(sessionID)
	if !ok {
		return fmt.Errorf("session not found")
	}

	if session.Status == models.StatusDestroying || session.Status == models.StatusDestroyed {
		return nil
	}

	lm.store.UpdateStatus(sessionID, models.StatusDestroying)

	if session.ContainerID != "" {
		if err := lm.client.DestroyContainer(ctx, sessionID); err != nil {
			log.Printf("[WARN] failed to destroy container %s: %v", sessionID, err)
		}
	}

	lm.store.Delete(sessionID)
	return nil
}

func generateSessionID() string {
	return fmt.Sprintf("sess-%d", time.Now().UnixNano())
}