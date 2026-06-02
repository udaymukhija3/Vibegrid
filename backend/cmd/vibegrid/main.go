package main

import (
	"context"
	"errors"
	"fmt"
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
	adminToken := os.Getenv("VIBEGRID_ADMIN_TOKEN")
	secureCookies := os.Getenv("VIBEGRID_SECURE_COOKIES") == "true"
	allowedOrigins := splitOrigins(os.Getenv("VIBEGRID_ALLOWED_ORIGINS"))

	// Root context cancelled on SIGINT/SIGTERM so startup and shutdown share one
	// lifecycle signal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	deps, err := buildDeps(ctx, logger, databaseURL)
	if err != nil {
		return err
	}
	defer deps.close()

	if deps.adminPuzzles == nil {
		logger.Warn("admin endpoints disabled (requires DATABASE_URL)")
	} else if adminToken == "" {
		logger.Warn("admin endpoints disabled: set VIBEGRID_ADMIN_TOKEN to enable")
	}

	handler := vibegrid.NewServer(vibegrid.ServerConfig{
		Puzzles:        deps.puzzles,
		Store:          deps.attempts,
		AdminPuzzles:   deps.adminPuzzles,
		AdminToken:     adminToken,
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

// deps bundles the store implementations the server needs, plus a close hook.
type deps struct {
	attempts     vibegrid.Store
	puzzles      vibegrid.PuzzleSource
	adminPuzzles vibegrid.AdminPuzzleStore
	close        func()
}

// buildDeps wires the durable Postgres stores when DATABASE_URL is set and
// otherwise falls back to in-memory attempts plus seed puzzles, so local runs
// and tests work with no database. Admin authoring requires Postgres.
func buildDeps(ctx context.Context, logger *slog.Logger, databaseURL string) (deps, error) {
	if databaseURL == "" {
		logger.Warn("DATABASE_URL not set, using in-memory store and seed puzzles (non-durable)")
		return deps{
			attempts: vibegrid.NewMemoryAttemptStore(),
			puzzles:  vibegrid.StaticPuzzleSource(vibegrid.SeedPuzzles()),
			close:    func() {},
		}, nil
	}

	database, err := vibegrid.OpenDB(ctx, databaseURL)
	if err != nil {
		return deps{}, err
	}

	puzzleStore := vibegrid.NewPostgresPuzzleStore(database)
	if err := puzzleStore.Seed(ctx, vibegrid.SeedPuzzles()); err != nil {
		_ = database.Close()
		return deps{}, fmt.Errorf("seed puzzles: %w", err)
	}

	logger.Info("connected to postgres, migrations applied, puzzles seeded")
	return deps{
		attempts:     vibegrid.NewPostgresAttemptStore(database),
		puzzles:      puzzleStore,
		adminPuzzles: puzzleStore,
		close: func() {
			if err := database.Close(); err != nil {
				logger.Error("closing postgres pool", "error", err)
			}
		},
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
