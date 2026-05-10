package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"vps-provider/internal/api"
	"vps-provider/internal/container"
	"vps-provider/internal/store"
)

func main() {
	containerdSocket := "/run/containerd/containerd.sock"
	if _, err := os.Stat(containerdSocket); os.IsNotExist(err) {
		log.Fatalf("containerd socket not found at %s", containerdSocket)
	}

	client, err := container.NewClient(containerdSocket)
	if err != nil {
		log.Fatalf("failed to create containerd client: %v", err)
	}
	defer client.Close()

	sessionStore := store.NewMemoryStore()
	lifecycle := container.NewLifecycleManager(client, sessionStore)
	handler := api.NewHandler(lifecycle, sessionStore)

	// Chi router handles all routes
	router := handler.RegisterRoutes()

	// Apply middleware
	var h http.Handler = router
	h = api.LoggingMiddleware(h)
	h = api.RecoveryMiddleware(h)

	port := ":8080"
	if envPort := os.Getenv("PORT"); envPort != "" {
		port = ":" + envPort
	}

	server := &http.Server{
		Addr:         port,
		Handler:      h,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	done := make(chan bool, 1)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		log.Println("shutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		server.SetKeepAlivesEnabled(false)
		if err := server.Shutdown(ctx); err != nil {
			log.Fatalf("server forced to shutdown: %v", err)
		}
		close(done)
	}()

	log.Printf("server starting on %s", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server failed: %v", err)
	}

	<-done
	log.Println("server exited")
}
