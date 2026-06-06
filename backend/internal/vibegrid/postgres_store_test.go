package vibegrid

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"
)

// newTestStore connects to TEST_DATABASE_URL and returns a store with empty
// tables. Tests are skipped when the variable is unset so the suite still
// passes with no database (e.g. a plain `go test ./...`).
func newTestStore(t *testing.T) *PostgresAttemptStore {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres integration tests")
	}

	database, err := OpenDB(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	// Truncating puzzles cascades to groups, tiles, attempts, and guesses, giving
	// each test a clean slate regardless of what other tests left behind.
	if _, err := database.Exec(`truncate rate_limit_hits, moderation_actions, moderation_reports, moderation_appeals, puzzles, attempts, attempt_guesses restart identity cascade`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	// Attempts reference puzzles by foreign key, so the seed puzzle must exist
	// before any attempt can be created.
	if err := NewPostgresPuzzleStore(database).Seed(context.Background(), SeedPuzzles()); err != nil {
		t.Fatalf("seed puzzles: %v", err)
	}
	return NewPostgresAttemptStore(database)
}

func correctGuess(clientGuessID string) GuessRequest {
	return GuessRequest{
		PuzzleID:        "vibegrid-2026-06-02",
		ClientGuessID:   clientGuessID,
		SelectedTileIDs: []string{"p1-espresso", "p1-linen", "p1-vespa", "p1-balcony"},
	}
}

func wrongGuess(clientGuessID string) GuessRequest {
	return GuessRequest{
		PuzzleID:        "vibegrid-2026-06-02",
		ClientGuessID:   clientGuessID,
		SelectedTileIDs: []string{"p1-espresso", "p1-linen", "p1-slack", "p1-balcony"},
	}
}

func TestPostgresCorrectGuessSolvesGroup(t *testing.T) {
	store := newTestStore(t)
	puzzle := SeedPuzzles()[0]
	ctx := context.Background()

	submission, err := store.SubmitGuess(ctx, puzzle, "session-a", correctGuess("g1"), fixedClock())
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if !submission.IsCorrect || submission.Group == nil || submission.Group.ID != "italian-summer" {
		t.Fatalf("expected correct italian-summer guess, got %#v", submission)
	}
	if len(submission.Attempt.SolvedGroups) != 1 || submission.Attempt.GuessCount != 1 {
		t.Fatalf("expected one solved group and one guess, got %#v", submission.Attempt)
	}
}

func TestPostgresDuplicateClientGuessIsIdempotent(t *testing.T) {
	store := newTestStore(t)
	puzzle := SeedPuzzles()[0]
	ctx := context.Background()

	if _, err := store.SubmitGuess(ctx, puzzle, "session-b", wrongGuess("same"), fixedClock()); err != nil {
		t.Fatalf("first submit: %v", err)
	}
	second, err := store.SubmitGuess(ctx, puzzle, "session-b", wrongGuess("same"), fixedClock())
	if err != nil {
		t.Fatalf("second submit: %v", err)
	}

	if second.Attempt.Mistakes != 1 || second.Attempt.GuessCount != 1 {
		t.Fatalf("duplicate guess must not double-count, got %#v", second.Attempt)
	}
}

func TestPostgresFourMistakesFailsAndReveals(t *testing.T) {
	store := newTestStore(t)
	puzzle := SeedPuzzles()[0]
	ctx := context.Background()

	var last GuessSubmission
	for i := 0; i < MaxMistakes; i++ {
		var err error
		last, err = store.SubmitGuess(ctx, puzzle, "session-c", wrongGuess(string(rune('a'+i))), fixedClock())
		if err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}

	if !last.Attempt.Failed {
		t.Fatalf("expected failed attempt, got %#v", last.Attempt)
	}
	if len(last.Attempt.RevealedGroups) != len(puzzle.Groups) {
		t.Fatalf("expected all groups revealed on failure, got %d", len(last.Attempt.RevealedGroups))
	}

	// A finished attempt rejects further guesses.
	if _, err := store.SubmitGuess(ctx, puzzle, "session-c", wrongGuess("late"), fixedClock()); err != ErrAttemptFinished {
		t.Fatalf("expected ErrAttemptFinished, got %v", err)
	}
}

// TestPostgresConcurrentDuplicateGuessCountsOnce is the core safety test: many
// retries of the same client guess id, fired concurrently, must record exactly
// one mistake. This is what the row lock plus the unique constraint buy us.
func TestPostgresConcurrentDuplicateGuessCountsOnce(t *testing.T) {
	store := newTestStore(t)
	puzzle := SeedPuzzles()[0]
	ctx := context.Background()

	const racers = 16
	var wg sync.WaitGroup
	wg.Add(racers)
	for i := 0; i < racers; i++ {
		go func() {
			defer wg.Done()
			_, _ = store.SubmitGuess(ctx, puzzle, "session-race", wrongGuess("dup"), fixedClock())
		}()
	}
	wg.Wait()

	snapshot, err := store.GetOrCreate(ctx, puzzle, "session-race", fixedClock())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if snapshot.Mistakes != 1 || snapshot.GuessCount != 1 {
		t.Fatalf("concurrent duplicate guess must count once, got mistakes=%d guesses=%d",
			snapshot.Mistakes, snapshot.GuessCount)
	}
}

// TestPostgresConcurrentDistinctGuessesSerialize fires four *distinct* wrong
// guesses at once. The row lock must serialize them so all four are counted and
// the attempt ends up failed, with no lost updates.
func TestPostgresConcurrentDistinctGuessesSerialize(t *testing.T) {
	store := newTestStore(t)
	puzzle := SeedPuzzles()[0]
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(MaxMistakes)
	for i := 0; i < MaxMistakes; i++ {
		guessID := string(rune('a' + i))
		go func() {
			defer wg.Done()
			_, _ = store.SubmitGuess(ctx, puzzle, "session-serial", wrongGuess(guessID), fixedClock())
		}()
	}
	wg.Wait()

	snapshot, err := store.GetOrCreate(ctx, puzzle, "session-serial", time.Now())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if snapshot.Mistakes != MaxMistakes || !snapshot.Failed {
		t.Fatalf("distinct concurrent guesses must all count, got mistakes=%d failed=%v",
			snapshot.Mistakes, snapshot.Failed)
	}
}
