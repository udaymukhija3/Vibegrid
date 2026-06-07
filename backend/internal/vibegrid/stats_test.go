package vibegrid

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"
)

func TestComputeStreak(t *testing.T) {
	// Three consecutive days ending today.
	got := computeStreak([]string{"2026-06-03", "2026-06-02", "2026-06-01"}, "2026-06-03")
	if got.CurrentStreak != 3 || got.LongestStreak != 3 || got.TotalCompleted != 3 {
		t.Fatalf("expected 3/3/3, got %#v", got)
	}

	// Today not played yet, but yesterday + the day before were: streak alive at 2.
	alive := computeStreak([]string{"2026-06-02", "2026-06-01"}, "2026-06-03")
	if alive.CurrentStreak != 2 {
		t.Fatalf("expected current streak 2 (alive via yesterday), got %d", alive.CurrentStreak)
	}

	// A gap two days back: current streak is broken (0), but history is kept.
	broken := computeStreak([]string{"2026-05-30", "2026-05-29"}, "2026-06-03")
	if broken.CurrentStreak != 0 || broken.LongestStreak != 2 || broken.TotalCompleted != 2 {
		t.Fatalf("expected 0/2/2 for a broken streak, got %#v", broken)
	}

	// No plays.
	if empty := computeStreak(nil, "2026-06-03"); empty != (StreakSummary{}) {
		t.Fatalf("expected zero summary, got %#v", empty)
	}
}

type countingStatsStore struct {
	mu      sync.Mutex
	calls   int
	blocker chan struct{}
}

func (store *countingStatsStore) PuzzleStats(context.Context, string) (PuzzleStats, error) {
	if store.blocker != nil {
		<-store.blocker
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.calls++
	return PuzzleStats{Players: store.calls}, nil
}

func (store *countingStatsStore) callCount() int {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.calls
}

func (store *countingStatsStore) WrongGuessGroupings(context.Context, string, int) ([]WrongGuessGrouping, error) {
	return nil, nil
}

func (store *countingStatsStore) SessionStreak(context.Context, string, string) (StreakSummary, error) {
	return StreakSummary{}, nil
}

func TestCachedStatsStoreCachesPuzzleStats(t *testing.T) {
	inner := &countingStatsStore{}
	cache := NewCachedStatsStore(inner, time.Minute).(*cachedStatsStore)
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	cache.clock = func() time.Time { return now }

	first, err := cache.PuzzleStats(context.Background(), "puzzle-1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := cache.PuzzleStats(context.Background(), "puzzle-1")
	if err != nil {
		t.Fatal(err)
	}
	if first.Players != 1 || second.Players != 1 || inner.callCount() != 1 {
		t.Fatalf("expected cached value after one call, got first=%#v second=%#v calls=%d", first, second, inner.callCount())
	}

	now = now.Add(2 * time.Minute)
	third, err := cache.PuzzleStats(context.Background(), "puzzle-1")
	if err != nil {
		t.Fatal(err)
	}
	if third.Players != 2 || inner.callCount() != 2 {
		t.Fatalf("expected cache refresh after ttl, got third=%#v calls=%d", third, inner.callCount())
	}
}

func TestCachedStatsStoreSingleflightsConcurrentMisses(t *testing.T) {
	blocker := make(chan struct{})
	inner := &countingStatsStore{blocker: blocker}
	cache := NewCachedStatsStore(inner, time.Minute).(*cachedStatsStore)

	const requestCount = 8
	results := make(chan PuzzleStats, requestCount)
	for range requestCount {
		go func() {
			stats, err := cache.PuzzleStats(context.Background(), "puzzle-1")
			if err != nil {
				t.Errorf("PuzzleStats returned error: %v", err)
				return
			}
			results <- stats
		}()
	}

	time.Sleep(10 * time.Millisecond)
	close(blocker)

	for range requestCount {
		stats := <-results
		if stats.Players != 1 {
			t.Fatalf("expected shared stats result, got %#v", stats)
		}
	}
	if inner.callCount() != 1 {
		t.Fatalf("expected one underlying call, got %d", inner.callCount())
	}
}

func TestCachedStatsStoreBoundsEntries(t *testing.T) {
	inner := &countingStatsStore{}
	cache := NewCachedStatsStore(inner, time.Hour).(*cachedStatsStore)
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	cache.clock = func() time.Time { return now }
	cache.maxEntries = 2

	for _, puzzleID := range []string{"puzzle-1", "puzzle-2", "puzzle-3"} {
		if _, err := cache.PuzzleStats(context.Background(), puzzleID); err != nil {
			t.Fatal(err)
		}
		now = now.Add(time.Second)
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()
	if len(cache.stats) != 2 {
		t.Fatalf("expected cache to stay bounded at 2 entries, got %d", len(cache.stats))
	}
	if _, exists := cache.stats["puzzle-1"]; exists {
		t.Fatal("expected oldest cache entry to be evicted")
	}
}

func newStatsTest(t *testing.T) (*PostgresAttemptStore, *PostgresStatsStore) {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres stats tests")
	}

	database, err := OpenDB(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	if _, err := database.Exec(`truncate rate_limit_hits, moderation_actions, moderation_reports, moderation_appeals, puzzles, attempts, attempt_guesses restart identity cascade`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if err := NewPostgresPuzzleStore(database).Seed(context.Background(), SeedPuzzles()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return NewPostgresAttemptStore(database), NewPostgresStatsStore(database)
}

func guess(clientGuessID string, tileIDs ...string) GuessRequest {
	return GuessRequest{
		PuzzleID:        "vibegrid-2026-06-02",
		ClientGuessID:   clientGuessID,
		SelectedTileIDs: tileIDs,
	}
}

func TestPuzzleStatsAggregates(t *testing.T) {
	attempts, stats := newStatsTest(t)
	puzzle := SeedPuzzles()[0]
	ctx := context.Background()

	// session-1: solves all four groups cleanly (0 mistakes -> completed).
	groups := [][]string{
		{"p1-espresso", "p1-linen", "p1-vespa", "p1-balcony"},
		{"p1-slack", "p1-deck", "p1-panic", "p1-959"},
		{"p1-rain", "p1-jazz", "p1-window", "p1-lamp"},
		{"p1-oats", "p1-whey", "p1-hoodie", "p1-deadlift"},
	}
	for i, g := range groups {
		if _, err := attempts.SubmitGuess(ctx, puzzle, "session-1", guess(string(rune('a'+i)), g...), fixedClock()); err != nil {
			t.Fatalf("solve guess %d: %v", i, err)
		}
	}

	// session-2: four wrong guesses -> failed (4 mistakes).
	wrong := []string{"p1-espresso", "p1-linen", "p1-slack", "p1-balcony"}
	for i := 0; i < 4; i++ {
		w := append([]string{}, wrong...)
		w[2] = []string{"p1-slack", "p1-deck", "p1-panic", "p1-959"}[i] // vary the 3rd tile
		if _, err := attempts.SubmitGuess(ctx, puzzle, "session-2", guess("w"+string(rune('a'+i)), w...), fixedClock()); err != nil {
			t.Fatalf("wrong guess %d: %v", i, err)
		}
	}

	// session-3: one wrong guess, identical to session-2's first -> shared grouping.
	if _, err := attempts.SubmitGuess(ctx, puzzle, "session-3", guess("solo", wrong...), fixedClock()); err != nil {
		t.Fatalf("session-3 guess: %v", err)
	}

	got, err := stats.PuzzleStats(ctx, puzzle.ID)
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if got.Players != 3 {
		t.Fatalf("expected 3 players, got %d", got.Players)
	}
	if got.SolveRate < 0.33 || got.SolveRate > 0.34 {
		t.Fatalf("expected solve rate ~0.333, got %v", got.SolveRate)
	}
	if got.MedianMistakes != 1 {
		t.Fatalf("expected median mistakes 1 (mistakes were 0,4,1), got %v", got.MedianMistakes)
	}

	groupings, err := stats.WrongGuessGroupings(ctx, puzzle.ID, 10)
	if err != nil {
		t.Fatalf("groupings: %v", err)
	}
	if len(groupings) == 0 {
		t.Fatal("expected wrong-guess groupings")
	}
	// The shared {espresso, linen, slack, balcony} guess (session-2 + session-3)
	// should be the most common, with a count of 2.
	if groupings[0].Count != 2 {
		t.Fatalf("expected top wrong grouping count 2, got %d", groupings[0].Count)
	}
}
