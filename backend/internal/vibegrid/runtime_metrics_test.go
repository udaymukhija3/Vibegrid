package vibegrid

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestMetricsExposesPoolAndCacheGauges verifies the runtime gauges render when
// collectors are configured, and stay absent (no-database mode) when they are not.
func TestMetricsExposesPoolAndCacheGauges(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles: StaticPuzzleSource(SeedPuzzles()),
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
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics missing %q\n%s", want, body)
		}
	}

	// Without collectors (no-database mode) the runtime gauges must not appear.
	bare := getMetrics(t, NewServer(ServerConfig{Puzzles: StaticPuzzleSource(SeedPuzzles())}))
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

func getMetrics(t *testing.T, handler http.Handler) string {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /metrics: expected 200, got %d", rec.Code)
	}
	return rec.Body.String()
}
