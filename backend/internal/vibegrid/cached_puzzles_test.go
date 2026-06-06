package vibegrid

import (
	"context"
	"testing"
	"time"
)

// fakePuzzleBackend is a minimal in-memory puzzleBackend that counts PuzzleByID
// loads, so tests can assert what the cache served versus what it fetched.
type fakePuzzleBackend struct {
	puzzles   map[string]Puzzle
	byIDCalls int
}

func newFakePuzzleBackend(puzzles ...Puzzle) *fakePuzzleBackend {
	backend := &fakePuzzleBackend{puzzles: map[string]Puzzle{}}
	for _, puzzle := range puzzles {
		backend.puzzles[puzzle.ID] = puzzle
	}
	return backend
}

func (backend *fakePuzzleBackend) PuzzleByID(_ context.Context, puzzleID string) (Puzzle, error) {
	backend.byIDCalls++
	puzzle, ok := backend.puzzles[puzzleID]
	if !ok {
		return Puzzle{}, ErrPuzzleNotFound
	}
	return puzzle, nil
}

func (backend *fakePuzzleBackend) Puzzles(context.Context) ([]Puzzle, error) { return nil, nil }
func (backend *fakePuzzleBackend) PublishedPuzzles(context.Context, string) ([]Puzzle, error) {
	return nil, nil
}
func (backend *fakePuzzleBackend) TodaysPuzzle(context.Context, string) (Puzzle, error) {
	return Puzzle{}, ErrPuzzleNotFound
}
func (backend *fakePuzzleBackend) CreateDraft(context.Context, AdminPuzzleInput) (Puzzle, error) {
	return Puzzle{}, nil
}
func (backend *fakePuzzleBackend) CreateCommunityPuzzle(context.Context, AdminPuzzleInput) (Puzzle, error) {
	return Puzzle{}, nil
}

func (backend *fakePuzzleBackend) Publish(_ context.Context, puzzleID, publishDate string) error {
	puzzle := backend.puzzles[puzzleID]
	puzzle.Status = PuzzleStatusPublished
	puzzle.PublishDate = publishDate
	backend.puzzles[puzzleID] = puzzle
	return nil
}

func (backend *fakePuzzleBackend) Archive(_ context.Context, puzzleID string) error {
	puzzle := backend.puzzles[puzzleID]
	puzzle.Status = PuzzleStatusArchived
	backend.puzzles[puzzleID] = puzzle
	return nil
}

func (backend *fakePuzzleBackend) Reinstate(_ context.Context, puzzleID string) error {
	puzzle := backend.puzzles[puzzleID]
	puzzle.Status = PuzzleStatusPublished
	backend.puzzles[puzzleID] = puzzle
	return nil
}

func TestCachedPuzzleStoreServesRepeatReadsFromCache(t *testing.T) {
	backend := newFakePuzzleBackend(Puzzle{ID: "p1", Status: PuzzleStatusPublished})
	store := NewCachedPuzzleStore(backend, time.Minute)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		puzzle, err := store.PuzzleByID(ctx, "p1")
		if err != nil {
			t.Fatalf("PuzzleByID: %v", err)
		}
		if puzzle.ID != "p1" {
			t.Fatalf("unexpected puzzle %q", puzzle.ID)
		}
	}
	if backend.byIDCalls != 1 {
		t.Fatalf("expected one backend load for repeated reads, got %d", backend.byIDCalls)
	}
}

func TestCachedPuzzleStoreInvalidatesOnArchive(t *testing.T) {
	backend := newFakePuzzleBackend(Puzzle{ID: "p1", Status: PuzzleStatusPublished})
	store := NewCachedPuzzleStore(backend, time.Minute)
	ctx := context.Background()

	if _, err := store.PuzzleByID(ctx, "p1"); err != nil { // miss -> load 1, cached as PUBLISHED
		t.Fatalf("PuzzleByID: %v", err)
	}
	if err := store.Archive(ctx, "p1"); err != nil { // must evict the cached copy
		t.Fatalf("Archive: %v", err)
	}

	puzzle, err := store.PuzzleByID(ctx, "p1") // miss again -> load 2, fresh status
	if err != nil {
		t.Fatalf("PuzzleByID after archive: %v", err)
	}
	if backend.byIDCalls != 2 {
		t.Fatalf("archive must invalidate the cache; expected 2 loads, got %d", backend.byIDCalls)
	}
	if puzzle.Status != PuzzleStatusArchived {
		t.Fatalf("expected archived status after invalidation, got %q", puzzle.Status)
	}
}

func TestCachedPuzzleStoreDoesNotCacheMisses(t *testing.T) {
	backend := newFakePuzzleBackend()
	store := NewCachedPuzzleStore(backend, time.Minute)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if _, err := store.PuzzleByID(ctx, "missing"); err == nil {
			t.Fatal("expected ErrPuzzleNotFound for a missing puzzle")
		}
	}
	// A negative result must not be cached, or a community puzzle shared by link
	// right after creation could appear missing until the entry expired.
	if backend.byIDCalls != 3 {
		t.Fatalf("misses must not be cached; expected 3 loads, got %d", backend.byIDCalls)
	}
}
