package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/vibegrid/vibegrid/backend/internal/vibegrid"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("vibegrid exited with error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	addr := env("VIBEGRID_ADDR", ":8081")
	timeZone := env("VIBEGRID_TIMEZONE", "Asia/Kolkata")
	databaseURL := os.Getenv("DATABASE_URL")
	secureCookies := os.Getenv("VIBEGRID_SECURE_COOKIES") == "true"
	allowedOrigins := splitOrigins(os.Getenv("VIBEGRID_ALLOWED_ORIGINS"))

	// Root context cancelled on SIGINT/SIGTERM so startup and shutdown share one
	// lifecycle signal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, closeStore, err := buildStore(ctx, logger, databaseURL)
	if err != nil {
		return err
	}
	defer closeStore()

	handler := vibegrid.NewServer(vibegrid.ServerConfig{
		Puzzles:        vibegrid.SeedPuzzles(),
		Store:          store,
		Clock:          time.Now,
		TimeZone:       timeZone,
		AllowedOrigins: allowedOrigins,
		SecureCookies:  secureCookies,
	})

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("vibegrid backend listening", "addr", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining connections")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}

// buildStore picks the durable Postgres store when DATABASE_URL is set and
// otherwise falls back to the in-memory store, so local runs and tests work
// with no database.
func buildStore(ctx context.Context, logger *slog.Logger, databaseURL string) (vibegrid.Store, func(), error) {
	if databaseURL == "" {
		logger.Warn("DATABASE_URL not set, using in-memory attempt store (non-durable)")
		return vibegrid.NewMemoryAttemptStore(), func() {}, nil
	}

	store, err := vibegrid.OpenPostgres(ctx, databaseURL)
	if err != nil {
		return nil, nil, err
	}

	logger.Info("connected to postgres, migrations applied")
	return store, func() {
		if err := store.Close(); err != nil {
			logger.Error("closing postgres store", "error", err)
		}
	}, nil
}

func splitOrigins(raw string) []string {
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			origins = append(origins, trimmed)
		}
	}
	return origins
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
