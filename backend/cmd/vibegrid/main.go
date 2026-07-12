package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/signal"
	"strconv"
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
	environment, err := runtimeEnvironment(os.Getenv("VIBEGRID_ENV"))
	if err != nil {
		return err
	}
	production := environment == "production"
	addr := resolveAddr()
	timeZone := env("VIBEGRID_TIMEZONE", "Asia/Kolkata")
	databaseURL := os.Getenv("DATABASE_URL")
	adminToken := os.Getenv("VIBEGRID_ADMIN_TOKEN")
	adminPassword := os.Getenv("VIBEGRID_ADMIN_PASSWORD")
	adminSessionSecret := os.Getenv("VIBEGRID_ADMIN_SESSION_SECRET")
	secureCookies, err := boolEnv("VIBEGRID_SECURE_COOKIES", production)
	if err != nil {
		return err
	}
	allowedOrigins := splitCommaList(os.Getenv("VIBEGRID_ALLOWED_ORIGINS"))
	devCORS := os.Getenv("VIBEGRID_DEV_CORS") == "true"
	requireDatabase, err := boolEnv("VIBEGRID_REQUIRE_DATABASE", production)
	if err != nil {
		return err
	}
	blockedTerms := splitCommaList(os.Getenv("VIBEGRID_BLOCKED_TERMS"))
	metricsToken := os.Getenv("VIBEGRID_METRICS_TOKEN")
	publicBaseURL, err := validatedPublicBaseURL(os.Getenv("VIBEGRID_PUBLIC_BASE_URL"), production)
	if err != nil {
		return err
	}
	trustedProxyCIDRs := splitCommaList(os.Getenv("VIBEGRID_TRUSTED_PROXY_CIDRS"))
	if err := validateTrustedProxyCIDRs(trustedProxyCIDRs); err != nil {
		return err
	}

	if production {
		if !requireDatabase || databaseURL == "" {
			return errors.New("DATABASE_URL and VIBEGRID_REQUIRE_DATABASE=true are required in production")
		}
		if !secureCookies {
			return errors.New("VIBEGRID_SECURE_COOKIES=true is required in production")
		}
		if metricsToken == "" {
			return errors.New("VIBEGRID_METRICS_TOKEN is required in production")
		}
		if devCORS {
			return errors.New("VIBEGRID_DEV_CORS must be false in production")
		}
		if len(trustedProxyCIDRs) == 0 {
			// Not fatal (small hosts may terminate TLS on the instance), but behind
			// a platform proxy this means every visitor shares the proxy's address —
			// and therefore one rate-limit bucket — so real traffic throttles itself.
			logger.Warn("VIBEGRID_TRUSTED_PROXY_CIDRS is empty: all clients behind the platform proxy will share one rate-limit bucket; set it to your host's proxy network(s)")
		}
	}

	// Root context cancelled on SIGINT/SIGTERM so startup and shutdown share one
	// lifecycle signal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Single-instance hosts without a release/pre-deploy hook (e.g. Render's free
	// tier) can apply migrations here instead of via the `vibegrid migrate`
	// release command. Leave this unset on multi-instance hosts so instances do
	// not race to migrate on boot — use the release command there.
	if os.Getenv("VIBEGRID_MIGRATE_ON_BOOT") == "true" {
		if databaseURL == "" {
			return errors.New("VIBEGRID_MIGRATE_ON_BOOT=true but DATABASE_URL is empty")
		}
		logger.Info("applying migrations on boot (VIBEGRID_MIGRATE_ON_BOOT=true)")
		if err := vibegrid.MigrateDB(ctx, databaseURL); err != nil {
			return fmt.Errorf("migrate on boot: %w", err)
		}
		logger.Info("migrations applied on boot")
	}

	deps, err := buildDeps(ctx, logger, databaseURL, requireDatabase)
	if err != nil {
		return err
	}
	defer deps.close()
	startRetentionPruner(ctx, logger, deps.pruneExpired)

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
		AdminSessions:      deps.adminSessions,
		Community:          deps.community,
		Stats:              deps.stats,
		RateLimits:         deps.rateLimits,
		Moderation:         deps.moderation,
		ReadyCheck:         deps.ready,
		Frontend:           frontend.NewHandler(frontend.Embedded()),
		AdminToken:         adminToken,
		AdminPassword:      adminPassword,
		AdminSessionSecret: adminSessionSecret,
		MetricsToken:       metricsToken,
		PublicBaseURL:      publicBaseURL,
		TrustedProxyCIDRs:  trustedProxyCIDRs,
		Clock:              time.Now,
		TimeZone:           timeZone,
		AllowedOrigins:     allowedOrigins,
		DevCORS:            devCORS,
		SecureCookies:      secureCookies,
		BlockedTerms:       blockedTerms,
		DBStats:            deps.dbStats,
		PuzzleCacheStats:   deps.puzzleCacheStats,
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
	attempts         vibegrid.Store
	puzzles          vibegrid.PuzzleSource
	adminPuzzles     vibegrid.AdminPuzzleStore
	community        vibegrid.CommunityPuzzleStore
	adminSessions    vibegrid.AdminSessionStore
	stats            vibegrid.StatsStore
	rateLimits       vibegrid.RateLimitStore
	moderation       vibegrid.ModerationStore
	ready            func(context.Context) error
	dbStats          func() sql.DBStats
	puzzleCacheStats func() vibegrid.CacheStats
	pruneExpired     func(context.Context) (int64, error)
	close            func()
}

// buildDeps wires the durable Postgres stores when DATABASE_URL is set and
// otherwise falls back to in-memory attempts plus seed puzzles, so local runs
// and tests work with no database. Admin authoring requires Postgres.
func buildDeps(ctx context.Context, logger *slog.Logger, databaseURL string, requireDatabase bool) (deps, error) {
	if databaseURL == "" {
		if requireDatabase {
			return deps{}, errors.New("DATABASE_URL is required when VIBEGRID_REQUIRE_DATABASE=true")
		}
		logger.Warn("DATABASE_URL not set, using in-memory store and seed puzzles (non-durable)")
		return deps{
			attempts: vibegrid.NewMemoryAttemptStore(),
			puzzles: vibegrid.NewDemoPuzzleSource(
				vibegrid.NewBankPuzzleSource(vibegrid.StaticPuzzleSource(vibegrid.SeedPuzzles()), vibegrid.PuzzleBank()),
			),
			close: func() {},
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

	// Cache immutable puzzle content in process so the per-guess read path does
	// not reload groups and tiles from Postgres on every request. The same
	// instance backs the public, admin, and community surfaces, so status changes
	// (publish/archive/reinstate) invalidate the cached copy.
	cached := vibegrid.NewCachedPuzzleStore(puzzleStore, 5*time.Minute)

	// Expose cache effectiveness on /metrics when the decorator is active.
	var puzzleCacheStats func() vibegrid.CacheStats
	if provider, ok := cached.(interface{ CacheStats() vibegrid.CacheStats }); ok {
		puzzleCacheStats = provider.CacheStats
	}

	// Public reads go through the bank decorator so the daily never runs dry when
	// nothing is explicitly scheduled. Admin/community management uses the concrete
	// cached store directly (the bank only synthesizes the read-only daily).
	banked := vibegrid.NewBankPuzzleSource(cached, vibegrid.PuzzleBank())
	publicPuzzles := vibegrid.NewDemoPuzzleSource(banked)

	logger.Info("connected to postgres, puzzles seeded")
	attempts := vibegrid.NewPostgresAttemptStore(database)
	adminSessions := vibegrid.NewPostgresAdminSessionStore(database)
	return deps{
		attempts:         attempts,
		puzzles:          publicPuzzles,
		adminPuzzles:     cached,
		community:        cached,
		adminSessions:    adminSessions,
		stats:            vibegrid.NewCachedStatsStore(vibegrid.NewPostgresStatsStore(database), 5*time.Minute),
		rateLimits:       vibegrid.NewPostgresRateLimitStore(database),
		moderation:       vibegrid.NewPostgresModerationStore(database),
		ready:            database.PingContext,
		dbStats:          database.Stats,
		puzzleCacheStats: puzzleCacheStats,
		pruneExpired: func(pruneCtx context.Context) (int64, error) {
			before := time.Now().Add(-30 * 24 * time.Hour)
			attemptsDeleted, err := attempts.PruneExpired(pruneCtx, before, 1_000)
			if err != nil {
				return attemptsDeleted, err
			}
			sessionsDeleted, err := adminSessions.PruneExpired(pruneCtx, time.Now(), 1_000)
			return attemptsDeleted + sessionsDeleted, err
		},
		close: func() {
			if err := database.Close(); err != nil {
				logger.Error("closing postgres pool", "error", err)
			}
		},
	}, nil
}

func runtimeEnvironment(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return "production", nil
	}
	if value == "production" || value == "development" || value == "test" {
		return value, nil
	}
	return "", fmt.Errorf("VIBEGRID_ENV must be production, development, or test")
}

func boolEnv(key string, fallback bool) (bool, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", key)
	}
	return value, nil
}

func validatedPublicBaseURL(raw string, production bool) (string, error) {
	value := strings.TrimRight(strings.TrimSpace(raw), "/")
	if value == "" {
		if production {
			return "", errors.New("VIBEGRID_PUBLIC_BASE_URL is required in production")
		}
		return "http://localhost:3000", nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawPath != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("VIBEGRID_PUBLIC_BASE_URL must be an absolute http(s) origin without credentials, path, query, or fragment")
	}
	if production && parsed.Scheme != "https" {
		return "", errors.New("VIBEGRID_PUBLIC_BASE_URL must use https in production")
	}
	return value, nil
}

func validateTrustedProxyCIDRs(cidrs []string) error {
	for _, cidr := range cidrs {
		if _, err := netip.ParsePrefix(cidr); err != nil {
			return fmt.Errorf("VIBEGRID_TRUSTED_PROXY_CIDRS contains invalid CIDR %q", cidr)
		}
	}
	return nil
}

func startRetentionPruner(ctx context.Context, logger *slog.Logger, prune func(context.Context) (int64, error)) {
	if prune == nil {
		return
	}
	run := func() {
		pruneCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		deleted, err := prune(pruneCtx)
		if err != nil {
			logger.Warn("retention cleanup failed", "error", err)
			return
		}
		if deleted > 0 {
			logger.Info("retention cleanup complete", "deleted", deleted)
		}
	}
	go func() {
		run()
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
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

// resolveAddr picks the listen address. An explicit VIBEGRID_ADDR always wins
// (Fly sets it in fly.toml); otherwise it honors the PORT env injected by PaaS
// hosts (Render, Railway, Cloud Run, Koyeb), falling back to :8081 for local and
// plain `docker run`.
func resolveAddr() string {
	if addr := os.Getenv("VIBEGRID_ADDR"); addr != "" {
		return addr
	}
	if port := os.Getenv("PORT"); port != "" {
		return ":" + port
	}
	return ":8081"
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
