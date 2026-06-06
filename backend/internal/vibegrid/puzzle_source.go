package vibegrid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

// PuzzleSource provides puzzle content (including group membership) to the
// server. Hot public paths use targeted methods so they do not load the whole
// archive before serving a single board or validating a guess.
type PuzzleSource interface {
	Puzzles(ctx context.Context) ([]Puzzle, error)
	PublishedPuzzles(ctx context.Context, today string) ([]Puzzle, error)
	TodaysPuzzle(ctx context.Context, today string) (Puzzle, error)
	PuzzleByID(ctx context.Context, puzzleID string) (Puzzle, error)
}

// StaticPuzzleSource serves a fixed in-memory puzzle set, used in tests and when
// no database is configured.
type StaticPuzzleSource []Puzzle

func (source StaticPuzzleSource) Puzzles(_ context.Context) ([]Puzzle, error) {
	return source, nil
}

func (source StaticPuzzleSource) PublishedPuzzles(_ context.Context, today string) ([]Puzzle, error) {
	return PublishedPuzzlesThrough(source, today), nil
}

func (source StaticPuzzleSource) TodaysPuzzle(_ context.Context, today string) (Puzzle, error) {
	puzzles := PublishedPuzzlesThrough(source, today)
	if len(puzzles) == 0 {
		return Puzzle{}, ErrPuzzleNotFound
	}
	return puzzles[0], nil
}

func (source StaticPuzzleSource) PuzzleByID(_ context.Context, puzzleID string) (Puzzle, error) {
	puzzle, err := FindPuzzleByID(source, puzzleID)
	if err != nil {
		return Puzzle{}, err
	}
	return *puzzle, nil
}

// newID returns a short, prefixed, collision-resistant identifier for
// admin-authored puzzles, groups, and tiles (e.g. "pzl_a1b2c3d4e5f6").
func newID(prefix string) string {
	bytes := make([]byte, 6)
	if _, err := rand.Read(bytes); err != nil {
		panic("crypto/rand failed while generating id: " + err.Error())
	}
	return prefix + "_" + hex.EncodeToString(bytes)
}
