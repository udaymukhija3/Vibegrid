package vibegrid

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWithSimpleProtocol(t *testing.T) {
	t.Run("adds query mode to url dsn", func(t *testing.T) {
		got := withSimpleProtocol("postgres://user:pass@example.com/vibegrid?sslmode=require")
		if !strings.Contains(got, "default_query_exec_mode=simple_protocol") {
			t.Fatalf("expected simple protocol query param, got %q", got)
		}
		if !strings.Contains(got, "sslmode=require") {
			t.Fatalf("expected existing query params to be preserved, got %q", got)
		}
	})

	t.Run("preserves explicit query mode", func(t *testing.T) {
		raw := "postgres://user:pass@example.com/vibegrid?default_query_exec_mode=cache_statement"
		if got := withSimpleProtocol(raw); got != raw {
			t.Fatalf("expected explicit query mode to be preserved, got %q", got)
		}
	})

	t.Run("adds query mode to keyword dsn", func(t *testing.T) {
		got := withSimpleProtocol("host=localhost dbname=vibegrid sslmode=disable")
		if !strings.Contains(got, " default_query_exec_mode=simple_protocol") {
			t.Fatalf("expected simple protocol keyword option, got %q", got)
		}
	})
}

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

func TestPostgresBankDailyCanRecordAttempts(t *testing.T) {
	store := newTestStore(t)
	source := NewBankPuzzleSource(StaticPuzzleSource(SeedPuzzles()), PuzzleBank())
	puzzle, err := source.TodaysPuzzle(context.Background(), "2026-06-13")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	guess := GuessRequest{
		PuzzleID:      puzzle.ID,
		ClientGuessID: "bank-daily",
		SelectedTileIDs: []string{
			puzzle.Groups[0].Tiles[0].ID,
			puzzle.Groups[0].Tiles[1].ID,
			puzzle.Groups[0].Tiles[2].ID,
			puzzle.Groups[0].Tiles[3].ID,
		},
	}
	submission, err := store.SubmitGuess(ctx, puzzle, "session-bank", guess, fixedClock())
	if err != nil {
		t.Fatalf("submit bank daily guess: %v", err)
	}
	if !submission.IsCorrect || submission.Attempt.GuessCount != 1 {
		t.Fatalf("expected bank daily guess to record, got %#v", submission)
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
	if last.Attempt.Completed || last.Attempt.CompletedAt == nil {
		t.Fatalf("failed attempt should be terminal but not solved, got %#v", last.Attempt)
	}
	if len(last.Attempt.RevealedGroups) != len(puzzle.Groups) {
		t.Fatalf("expected all groups revealed on failure, got %d", len(last.Attempt.RevealedGroups))
	}

	var completed, hasCompletedAt bool
	if err := store.db.QueryRowContext(ctx,
		`select completed, completed_at is not null from attempts
		 where puzzle_id = $1 and session_id = $2`,
		puzzle.ID, "session-c",
	).Scan(&completed, &hasCompletedAt); err != nil {
		t.Fatalf("load terminal flags: %v", err)
	}
	if completed || !hasCompletedAt {
		t.Fatalf("expected failed row to have completed=false and completed_at set, got completed=%v completed_at_set=%v", completed, hasCompletedAt)
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

	snapshot, err := store.GetAttempt(ctx, puzzle, "session-race", fixedClock())
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

	snapshot, err := store.GetAttempt(ctx, puzzle, "session-serial", time.Now())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if snapshot.Mistakes != MaxMistakes || !snapshot.Failed {
		t.Fatalf("distinct concurrent guesses must all count, got mistakes=%d failed=%v",
			snapshot.Mistakes, snapshot.Failed)
	}
}

// TestPostgresHydratesGuessHistory proves the durable store rebuilds the full,
// ordered guess history from attempt_guesses on both the submission response and
// a fresh read, and that an idempotent replay never duplicates a history row.
func TestPostgresHydratesGuessHistory(t *testing.T) {
	store := newTestStore(t)
	puzzle := SeedPuzzles()[0]
	ctx := context.Background()

	if _, err := store.SubmitGuess(ctx, puzzle, "session-hist", wrongGuess("w1"), fixedClock()); err != nil {
		t.Fatalf("wrong submit: %v", err)
	}
	submission, err := store.SubmitGuess(ctx, puzzle, "session-hist", correctGuess("c1"), fixedClock())
	if err != nil {
		t.Fatalf("correct submit: %v", err)
	}
	if len(submission.Attempt.GuessHistory) != 2 {
		t.Fatalf("submission should carry full history, got %#v", submission.Attempt.GuessHistory)
	}

	// A fresh read is what a second tab does on load: it must rebuild the same
	// ordered history from the database with no client state involved.
	snapshot, err := store.GetAttempt(ctx, puzzle, "session-hist", fixedClock())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(snapshot.GuessHistory) != 2 {
		t.Fatalf("fresh read should rebuild 2 guesses, got %#v", snapshot.GuessHistory)
	}
	assertSameTileSet(t, "first guess", snapshot.GuessHistory[0], wrongGuess("").SelectedTileIDs)
	assertSameTileSet(t, "second guess", snapshot.GuessHistory[1], correctGuess("").SelectedTileIDs)

	// A replayed (idempotent) guess must not append a duplicate history row.
	if _, err := store.SubmitGuess(ctx, puzzle, "session-hist", wrongGuess("w1"), fixedClock()); err != nil {
		t.Fatalf("replay submit: %v", err)
	}
	replayed, err := store.GetAttempt(ctx, puzzle, "session-hist", fixedClock())
	if err != nil {
		t.Fatalf("get after replay: %v", err)
	}
	if len(replayed.GuessHistory) != 2 {
		t.Fatalf("idempotent replay must not grow history, got %d rows", len(replayed.GuessHistory))
	}
}
