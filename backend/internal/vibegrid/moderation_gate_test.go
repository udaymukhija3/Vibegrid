package vibegrid

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
)

// These tests cover the community review gate: unreviewed content must not reach
// the public through any path other than admin approval.

func newTestPuzzleStore(t *testing.T) (*PostgresPuzzleStore, *sql.DB) {
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

	if _, err := database.Exec(`truncate rate_limit_hits, admin_sessions, moderation_actions, moderation_reports, moderation_appeals, crew_members, crews, puzzles, attempts, attempt_guesses restart identity cascade`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return NewPostgresPuzzleStore(database), database
}

func insertPuzzle(t *testing.T, database *sql.DB, id string, status PuzzleStatus, origin PuzzleOrigin) {
	t.Helper()
	if _, err := database.Exec(
		`insert into puzzles (id, puzzle_number, status, difficulty, origin)
		 values ($1, nextval('puzzle_number_seq'), $2, 'MEDIUM', $3)`,
		id, string(status), string(origin),
	); err != nil {
		t.Fatalf("insert puzzle %s: %v", id, err)
	}
}

func puzzleStatus(t *testing.T, database *sql.DB, id string) string {
	t.Helper()
	var status string
	if err := database.QueryRow(`select status from puzzles where id = $1`, id).Scan(&status); err != nil {
		t.Fatalf("read status %s: %v", id, err)
	}
	return status
}

// TestReinstateOnlyRestoresArchivedPuzzles is the regression test for the review
// bypass: Reinstate was an unconditional publish, so a PENDING community puzzle
// pushed through the appeal queue went live without ApproveCommunity — the
// review gate — ever running.
func TestReinstateOnlyRestoresArchivedPuzzles(t *testing.T) {
	store, database := newTestPuzzleStore(t)
	ctx := context.Background()

	insertPuzzle(t, database, "pzl_pending", PuzzleStatusPending, OriginCommunity)
	if err := store.Reinstate(ctx, "pzl_pending"); !errors.Is(err, ErrPuzzleNotFound) {
		t.Errorf("reinstating a PENDING puzzle: got err %v, want ErrPuzzleNotFound", err)
	}
	if got := puzzleStatus(t, database, "pzl_pending"); got != string(PuzzleStatusPending) {
		t.Errorf("reinstate published an unreviewed puzzle: status is %s, want PENDING", got)
	}

	insertPuzzle(t, database, "pzl_draft", PuzzleStatusDraft, OriginEditorial)
	if err := store.Reinstate(ctx, "pzl_draft"); !errors.Is(err, ErrPuzzleNotFound) {
		t.Errorf("reinstating a DRAFT puzzle: got err %v, want ErrPuzzleNotFound", err)
	}
	if got := puzzleStatus(t, database, "pzl_draft"); got != string(PuzzleStatusDraft) {
		t.Errorf("reinstate published a draft: status is %s, want DRAFT", got)
	}
}

// TestReinstateRestoresArchivedPuzzle keeps the legitimate takedown-reversal
// path working — the guard must not break real reinstatement.
func TestReinstateRestoresArchivedPuzzle(t *testing.T) {
	store, database := newTestPuzzleStore(t)
	ctx := context.Background()

	insertPuzzle(t, database, "pzl_archived", PuzzleStatusArchived, OriginCommunity)
	if err := store.Reinstate(ctx, "pzl_archived"); err != nil {
		t.Fatalf("reinstate archived puzzle: %v", err)
	}
	if got := puzzleStatus(t, database, "pzl_archived"); got != string(PuzzleStatusPublished) {
		t.Errorf("archived puzzle was not reinstated: status is %s, want PUBLISHED", got)
	}
}

func TestReinstateUnknownPuzzle(t *testing.T) {
	store, _ := newTestPuzzleStore(t)
	if err := store.Reinstate(context.Background(), "pzl_missing"); !errors.Is(err, ErrPuzzleNotFound) {
		t.Errorf("got err %v, want ErrPuzzleNotFound", err)
	}
}
