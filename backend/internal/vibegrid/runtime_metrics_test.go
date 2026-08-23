package vibegrid

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestMetricsExposesPoolAndCacheGauges verifies the runtime gauges render when
// collectors are configured, and stay absent (no-database mode) when they are not.
func TestMetricsExposesPoolAndCacheGauges(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles:      StaticPuzzleSource(SeedPuzzles()),
		MetricsToken: "test-metrics-token",
		DBStats: func() sql.DBStats {
			return sql.DBStats{OpenConnections: 3, InUse: 1, Idle: 2, WaitCount: 7, WaitDuration: 250 * time.Millisecond}
		},
		PuzzleCacheStats: func() CacheStats {
			return CacheStats{Hits: 41, Misses: 9, Evictions: 2, Entries: 5}
		},
	})

	body := getMetrics(t, handler)
	for _, want := range []string{
		"vibegrid_db_open_connections 3",
		"vibegrid_db_in_use_connections 1",
		"vibegrid_db_idle_connections 2",
		"vibegrid_db_wait_count_total 7",
		"vibegrid_db_wait_seconds_total 0.25",
		"vibegrid_puzzle_cache_hits_total 41",
		"vibegrid_puzzle_cache_misses_total 9",
		"vibegrid_puzzle_cache_evictions_total 2",
		"vibegrid_puzzle_cache_entries 5",
		"vibegrid_process_goroutines",
		"vibegrid_process_heap_alloc_bytes",
		"vibegrid_process_heap_inuse_bytes",
		"vibegrid_process_stack_inuse_bytes",
		"vibegrid_process_sys_bytes",
		"vibegrid_process_gc_cycles_total",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics missing %q\n%s", want, body)
		}
	}

	// Without collectors (no-database mode) the runtime gauges must not appear.
	bare := getMetrics(t, NewServer(ServerConfig{Puzzles: StaticPuzzleSource(SeedPuzzles()), MetricsToken: "test-metrics-token"}))
	if strings.Contains(bare, "vibegrid_db_open_connections") || strings.Contains(bare, "vibegrid_puzzle_cache_hits_total") {
		t.Fatalf("runtime gauges leaked without collectors\n%s", bare)
	}
}

// TestPuzzleCacheStatsCountHitsAndMisses verifies the cache reports an accurate
// hit/miss split: the first read misses and loads, repeats hit.
func TestPuzzleCacheStatsCountHitsAndMisses(t *testing.T) {
	backend := newFakePuzzleBackend(Puzzle{ID: "p1", Status: PuzzleStatusPublished})
	cache := NewCachedPuzzleStore(backend, time.Minute)
	provider, ok := cache.(interface{ CacheStats() CacheStats })
	if !ok {
		t.Fatal("cached store should expose CacheStats")
	}
	ctx := context.Background()

	for i := 0; i < 4; i++ {
		if _, err := cache.PuzzleByID(ctx, "p1"); err != nil {
			t.Fatalf("PuzzleByID: %v", err)
		}
	}

	stats := provider.CacheStats()
	if stats.Misses != 1 {
		t.Fatalf("expected 1 miss, got %d", stats.Misses)
	}
	if stats.Hits != 3 {
		t.Fatalf("expected 3 hits, got %d", stats.Hits)
	}
	if stats.Entries != 1 {
		t.Fatalf("expected 1 cached entry, got %d", stats.Entries)
	}
}

func TestMetricsNormalizeUnknownAPIPathsAndRequireAuthorization(t *testing.T) {
	metrics := newHTTPMetrics()
	handler := withRequestMetrics(http.NotFoundHandler(), metrics)
	for index := 0; index < 400; index++ {
		req := httptest.NewRequest(http.MethodPost, "/api/unmatched-"+strconv.Itoa(index), nil)
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}
	if len(metrics.requests) != 1 {
		t.Fatalf("unmatched API paths must share one series, got %d", len(metrics.requests))
	}
	for key := range metrics.requests {
		if key.route != "/api/*" {
			t.Fatalf("unexpected normalized route %q", key.route)
		}
	}
	if got := knownRouteMetricLabel("/api/vibes/practice/18446744073709551615"); got != "/api/vibes/practice/{sequence}" {
		t.Fatalf("unlimited practice route must have a bounded metric label, got %q", got)
	}

	protected := NewServer(ServerConfig{Puzzles: StaticPuzzleSource(SeedPuzzles()), MetricsToken: "test-metrics-token"})
	unauthorized := httptest.NewRecorder()
	protected.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("metrics without a bearer token: expected 401, got %d", unauthorized.Code)
	}
	authorized := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer test-metrics-token")
	protected.ServeHTTP(authorized, req)
	if got := authorized.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("metrics must not be cacheable, got %q", got)
	}

	disabled := httptest.NewRecorder()
	NewServer(ServerConfig{Puzzles: StaticPuzzleSource(SeedPuzzles())}).ServeHTTP(disabled, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if disabled.Code != http.StatusNotFound {
		t.Fatalf("metrics must be absent when no token is configured, got %d", disabled.Code)
	}
}

func TestRequestLogsNormalizeCapabilityPaths(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	handler := withRequestLogging(http.NotFoundHandler())
	const invite = "invite-secret-must-not-reach-logs"
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/crews/"+invite+"/daily", nil))

	body := logs.String()
	if strings.Contains(body, invite) {
		t.Fatalf("request log leaked crew capability: %s", body)
	}
	if !strings.Contains(body, `"path":"/api/crews/{id}/daily"`) {
		t.Fatalf("request log did not retain the normalized route: %s", body)
	}
}

func TestCrewAPIsAreAlwaysPrivateNoStore(t *testing.T) {
	handler := withCacheSafety(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, path := range []string{"/api/crews", "/api/crews/invite-secret/daily", "/api/crews/invite-secret/votes"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if got := recorder.Header().Get("Cache-Control"); got != "private, no-store" {
			t.Fatalf("%s must not be cached, got %q", path, got)
		}
	}

	public := httptest.NewRecorder()
	handler.ServeHTTP(public, httptest.NewRequest(http.MethodGet, "/api/vibes/today", nil))
	if got := public.Header().Get("Cache-Control"); got != "" {
		t.Fatalf("cache-safety middleware should leave public policy to the handler, got %q", got)
	}
}

func getMetrics(t *testing.T, handler http.Handler) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer test-metrics-token")
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /metrics: expected 200, got %d", rec.Code)
	}
	return rec.Body.String()
}
