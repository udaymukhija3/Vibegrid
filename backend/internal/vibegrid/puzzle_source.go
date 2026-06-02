package vibegrid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

// PuzzleSource provides the full puzzle set (including group membership) to the
// server. The seed-backed and Postgres-backed implementations both satisfy it,
// so handlers do not care where puzzle content lives.
type PuzzleSource interface {
	Puzzles(ctx context.Context) ([]Puzzle, error)
}

// StaticPuzzleSource serves a fixed in-memory puzzle set, used in tests and when
// no database is configured.
type StaticPuzzleSource []Puzzle

func (source StaticPuzzleSource) Puzzles(_ context.Context) ([]Puzzle, error) {
	return source, nil
}

// newID returns a short, prefixed, collision-resistant identifier for
// admin-authored puzzles, groups, and tiles (e.g. "pzl_a1b2c3d4e5f6").
func newID(prefix string) string {
	bytes := make([]byte, 6)
	if _, err := rand.Read(bytes); err != nil {
		// rand.Read only fails on a broken platform RNG; fall back to a
		// timestamp-free constant prefix so the caller still gets a usable id.
		return prefix + "_000000000000"
	}
	return prefix + "_" + hex.EncodeToString(bytes)
}
