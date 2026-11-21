package run

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Serve returns only after the server has drained or its shutdown deadline expires.
func Serve(ctx context.Context, server *http.Server, listener net.Listener, timeout time.Duration, log *slog.Logger) error {
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return errors.New("HTTP server stopped unexpectedly")
	case <-ctx.Done():
		log.Info("api_stopping")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			<-done
			return errors.New("HTTP shutdown deadline exceeded")
		}
		err := <-done
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return errors.New("HTTP server shutdown failed")
		}
		log.Info("api_stopped")
		return nil
	}
}
func Worker(ctx context.Context, log *slog.Logger) {
	log.Info("worker_started", "mode", "idle", "queue_enabled", false)
	<-ctx.Done()
	log.Info("worker_stopped")
}
