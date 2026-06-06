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

	"github.com/vibegrid/vibegrid/backend/internal/frontend"
	"github.com/vibegrid/vibegrid/backend/internal/vibegrid"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// `vibegrid migrate` applies migrations and exits — used as the deploy
	// release step so schema changes land once, before any instance serves
	// traffic, instead of racing across instances on boot.
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		if err := runMigrate(logger); err != nil {
			logger.Error("migration failed", "error", err)
			os.Exit(1)
		}
		return
	}

	if err := run(logger); err != nil {
		logger.Error("vibegrid exited with error", "error", err)
		os.Exit(1)
	}
}

func runMigrate(logger *slog.Logger) error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required to migrate")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := vibegrid.MigrateDB(ctx, databaseURL); err != nil {
		return err
	}
	logger.Info("migrations applied")
	return nil
}

func run(logger *slog.Logger) error {
	addr := env("VIBEGRID_ADDR", ":8081")
	timeZone := env("VIBEGRID_TIMEZONE", "Asia/Kolkata")
	databaseURL := os.Getenv("DATABASE_URL")
	adminToken := os.Getenv("VIBEGRID_ADMIN_TOKEN")
	adminPassword := os.Getenv("VIBEGRID_ADMIN_PASSWORD")
	adminSessionSecret := os.Getenv("VIBEGRID_ADMIN_SESSION_SECRET")
	secureCookies := os.Getenv("VIBEGRID_SECURE_COOKIES") == "true"
	allowedOrigins := splitCommaList(os.Getenv("VIBEGRID_ALLOWED_ORIGINS"))
	blockedTerms := splitCommaList(os.Getenv("VIBEGRID_BLOCKED_TERMS"))

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
	} else if adminPassword == "" && adminToken == "" {
		logger.Warn("admin endpoints disabled: set VIBEGRID_ADMIN_PASSWORD or VIBEGRID_ADMIN_TOKEN to enable")
	} else if adminPassword != "" && adminSessionSecret == "" {
		logger.Warn("admin password set without VIBEGRID_ADMIN_SESSION_SECRET; browser admin login disabled")
	}

	handler := vibegrid.NewServer(vibegrid.ServerConfig{
		Puzzles:            deps.puzzles,
		Store:              deps.attempts,
		AdminPuzzles:       deps.adminPuzzles,
		Community:          deps.community,
		Stats:              deps.stats,
		RateLimits:         deps.rateLimits,
		Moderation:         deps.moderation,
		ReadyCheck:         deps.ready,
		Frontend:           frontend.NewHandler(frontend.Embedded()),
		AdminToken:         adminToken,
		AdminPassword:      adminPassword,
		AdminSessionSecret: adminSessionSecret,
		Clock:              time.Now,
		TimeZone:           timeZone,
		AllowedOrigins:     allowedOrigins,
		SecureCookies:      secureCookies,
		BlockedTerms:       blockedTerms,
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
		logger.Info("vibegrid listening", "addr", addr)
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

// deps bundles the store implementations the server needs, plus a close hook
// and a readiness probe (nil when there is no database to check).
type deps struct {
	attempts     vibegrid.Store
	puzzles      vibegrid.PuzzleSource
	adminPuzzles vibegrid.AdminPuzzleStore
	community    vibegrid.CommunityPuzzleStore
	stats        vibegrid.StatsStore
	rateLimits   vibegrid.RateLimitStore
	moderation   vibegrid.ModerationStore
	ready        func(context.Context) error
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

	database, err := vibegrid.ConnectDB(ctx, databaseURL)
	if err != nil {
		return deps{}, err
	}

	puzzleStore := vibegrid.NewPostgresPuzzleStore(database)
	if err := puzzleStore.Seed(ctx, vibegrid.SeedPuzzles()); err != nil {
		_ = database.Close()
		return deps{}, fmt.Errorf("seed puzzles: %w", err)
	}

	logger.Info("connected to postgres, puzzles seeded")
	return deps{
		attempts:     vibegrid.NewPostgresAttemptStore(database),
		puzzles:      puzzleStore,
		adminPuzzles: puzzleStore,
		community:    puzzleStore,
		stats:        vibegrid.NewCachedStatsStore(vibegrid.NewPostgresStatsStore(database), 5*time.Minute),
		rateLimits:   vibegrid.NewPostgresRateLimitStore(database),
		moderation:   vibegrid.NewPostgresModerationStore(database),
		ready:        database.PingContext,
		close: func() {
			if err := database.Close(); err != nil {
				logger.Error("closing postgres pool", "error", err)
			}
		},
	}, nil
}

func splitCommaList(raw string) []string {
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
