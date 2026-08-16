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
	turnstileSiteKey := strings.TrimSpace(os.Getenv("VIBEGRID_TURNSTILE_SITE_KEY"))
	turnstileSecretKey := strings.TrimSpace(os.Getenv("VIBEGRID_TURNSTILE_SECRET_KEY"))
	if (turnstileSiteKey == "") != (turnstileSecretKey == "") {
		return errors.New("VIBEGRID_TURNSTILE_SITE_KEY and VIBEGRID_TURNSTILE_SECRET_KEY must be set together")
	}
	operatorWebhookURL, err := validatedWebhookURL(os.Getenv("VIBEGRID_OPERATOR_WEBHOOK_URL"), production)
	if err != nil {
		return err
	}
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
		if devCORS {
			return errors.New("VIBEGRID_DEV_CORS must be false in production")
		}
		if turnstileSiteKey == "" {
			return errors.New("VIBEGRID_TURNSTILE_SITE_KEY and VIBEGRID_TURNSTILE_SECRET_KEY are required in production")
		}
		// Missing observability/metadata config degrades with a warning instead
		// of refusing to boot: a disabled /metrics or wrong OG URLs beat a
		// bricked deploy pipeline. (Fatal versions of these checks silently
		// failed every deploy after they shipped — the platform env never had
		// the new values, so the new binary always died at boot and the old
		// release kept serving.)
		if metricsToken == "" {
			logger.Warn("VIBEGRID_METRICS_TOKEN is not set: /metrics stays disabled until it is")
		}
		if strings.TrimSpace(os.Getenv("VIBEGRID_PUBLIC_BASE_URL")) == "" {
			logger.Warn("VIBEGRID_PUBLIC_BASE_URL is not set: share/OG and sitemap URLs fall back to localhost")
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
	startDailyGenerator(ctx, logger, deps.bankSource)
	startRateLimitPruner(ctx, logger, deps.rateLimits)
	if operatorWebhookURL != "" {
		vibegrid.RunNotificationOutbox(ctx, logger, deps.outbox, vibegrid.NewWebhookNotificationDeliverer(operatorWebhookURL))
	} else if production {
		logger.Warn("VIBEGRID_OPERATOR_WEBHOOK_URL is not set: notification events remain pending in the transactional outbox")
	}

	if deps.adminPuzzles == nil {
		logger.Warn("admin endpoints disabled (requires DATABASE_URL)")
	} else if adminPassword == "" && adminToken == "" {
		logger.Warn("admin endpoints disabled: set VIBEGRID_ADMIN_PASSWORD or VIBEGRID_ADMIN_TOKEN to enable")
	} else if adminPassword != "" && adminSessionSecret == "" {
		logger.Warn("admin password set without VIBEGRID_ADMIN_SESSION_SECRET; browser admin login disabled")
	}
	var botVerifier vibegrid.BotVerifier
	if turnstileSecretKey != "" {
		botVerifier = vibegrid.NewTurnstileVerifier(turnstileSecretKey, publicHostname(publicBaseURL))
	}

	handler := vibegrid.NewServer(vibegrid.ServerConfig{
		Puzzles:            deps.puzzles,
		Store:              deps.attempts,
		AdminPuzzles:       deps.adminPuzzles,
		AdminSessions:      deps.adminSessions,
		Community:          deps.community,
		Stats:              deps.stats,
		RateLimits:         deps.rateLimits,
		Idempotency:        deps.idempotency,
		Moderation:         deps.moderation,
		ReadyCheck:         deps.ready,
		Frontend:           frontend.NewHandler(frontend.Embedded()),
		AdminToken:         adminToken,
		AdminPassword:      adminPassword,
		AdminSessionSecret: adminSessionSecret,
		MetricsToken:       metricsToken,
		PublicBaseURL:      publicBaseURL,
		TurnstileSiteKey:   turnstileSiteKey,
		BotVerifier:        botVerifier,
		TrustedProxyCIDRs:  trustedProxyCIDRs,
		Clock:              time.Now,
		TimeZone:           timeZone,
		AllowedOrigins:     allowedOrigins,
		DevCORS:            devCORS,
		SecureCookies:      secureCookies,
		BlockedTerms:       blockedTerms,
		DBStats:            deps.dbStats,
		PuzzleCacheStats:   deps.puzzleCacheStats,
		OutboxStats:        deps.outboxStats,
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

func publicHostname(publicBaseURL string) string {
	parsed, err := url.Parse(publicBaseURL)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

func validatedWebhookURL(raw string, production bool) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Fragment != "" {
		return "", errors.New("VIBEGRID_OPERATOR_WEBHOOK_URL must be an absolute http(s) URL without credentials or a fragment")
	}
	if production && parsed.Scheme != "https" {
		return "", errors.New("VIBEGRID_OPERATOR_WEBHOOK_URL must use https in production")
	}
	return value, nil
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
	idempotency      vibegrid.IdempotencyStore
	moderation       vibegrid.ModerationStore
	outbox           vibegrid.NotificationOutbox
	ready            func(context.Context) error
	dbStats          func() sql.DBStats
	puzzleCacheStats func() vibegrid.CacheStats
	outboxStats      func(context.Context) vibegrid.NotificationOutboxStats
	pruneExpired     func(context.Context) (int64, error)
	close            func()
	bankSource       *vibegrid.BankPuzzleSource
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
				vibegrid.NewBankPuzzleSource(vibegrid.StaticPuzzleSource(vibegrid.SeedPuzzles()), vibegrid.PuzzleBank(), nil),
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
	banked := vibegrid.NewBankPuzzleSource(cached, vibegrid.PuzzleBank(), puzzleStore)
	publicPuzzles := vibegrid.NewDemoPuzzleSource(banked)

	logger.Info("connected to postgres, puzzles seeded")
	attempts := vibegrid.NewPostgresAttemptStore(database)
	adminSessions := vibegrid.NewPostgresAdminSessionStore(database)
	idempotency := vibegrid.NewPostgresIdempotencyStore(database)
	outbox := vibegrid.NewPostgresNotificationOutbox(database)
	return deps{
		attempts:         attempts,
		puzzles:          publicPuzzles,
		adminPuzzles:     cached,
		community:        cached,
		adminSessions:    adminSessions,
		bankSource:       banked,
		stats:            vibegrid.NewCachedStatsStore(vibegrid.NewPostgresStatsStore(database), 5*time.Minute),
		rateLimits:       vibegrid.NewPostgresRateLimitStore(database),
		idempotency:      idempotency,
		moderation:       vibegrid.NewPostgresModerationStore(database),
		outbox:           outbox,
		outboxStats:      outbox.Stats,
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
			if err != nil {
				return attemptsDeleted + sessionsDeleted, err
			}
			idempotencyDeleted, err := idempotency.PruneExpired(pruneCtx, time.Now().Add(-48*time.Hour), 1_000)
			return attemptsDeleted + sessionsDeleted + idempotencyDeleted, err
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
		// An absent value must not brick a deploy: boot with the local default
		// (run() logs a production warning). An explicitly wrong value below is
		// still fatal — misconfiguration should fail loudly, absence should not.
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

type rateLimitPruner interface {
	Prune(context.Context) error
}

func startRateLimitPruner(ctx context.Context, logger *slog.Logger, store rateLimitPruner) {
	if store == nil {
		return
	}
	run := func() {
		pruneCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := store.Prune(pruneCtx); err != nil {
			logger.Warn("rate limit cleanup failed", "error", err)
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

func startDailyGenerator(ctx context.Context, logger *slog.Logger, source *vibegrid.BankPuzzleSource) {
	if source == nil {
		return
	}
	run := func() {
		date := time.Now().UTC().Format("2006-01-02")
		// Also ensure tomorrow is persisted to be safe
		tomorrow := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")

		ensureCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		if err := source.EnsureDailyPersisted(ensureCtx, date); err != nil {
			logger.Warn("failed to persist today's daily puzzle", "date", date, "error", err)
		}
		if err := source.EnsureDailyPersisted(ensureCtx, tomorrow); err != nil {
			logger.Warn("failed to persist tomorrow's daily puzzle", "date", tomorrow, "error", err)
		}
	}

	go func() {
		// Run once on boot to ensure current day is caught up
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
