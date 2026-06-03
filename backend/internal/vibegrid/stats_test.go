package vibegrid

import (
	"context"
	"os"
	"testing"
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

	if _, err := database.Exec(`truncate puzzles, attempts, attempt_guesses restart identity cascade`); err != nil {
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
