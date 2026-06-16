package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		slog.Error("backend stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	if err := loadDotEnvFiles(".env", "../.env"); err != nil {
		return err
	}

	server := &http.Server{
		Addr:              ":" + port(),
		Handler:           newRouter(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if shouldPreloadLiteAvatar() {
		preloadCtx, cancelPreload := context.WithTimeout(ctx, avatarTimeout())
		worker, err := startLiteAvatarWorker(preloadCtx)
		cancelPreload()
		if err != nil {
			slog.Warn("liteavatar preload failed, falling back to per-request python", "error", err)
		} else {
			setLiteAvatarWorker(worker)
			defer func() {
				setLiteAvatarWorker(nil)
				worker.Close()
			}()
		}
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("backend listening", "addr", server.Addr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}

		return err
	}
}
