package vibegrid

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPagePuzzlesBounds(t *testing.T) {
	items := []Puzzle{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	cases := []struct {
		name          string
		limit, offset int
		wantIDs       []string
	}{
		{"first page", 2, 0, []string{"a", "b"}},
		{"second page is partial", 2, 2, []string{"c"}},
		{"offset past end is empty", 2, 5, nil},
		{"negative offset clamps to zero", 2, -1, []string{"a", "b"}},
		{"non-positive limit returns the rest", 0, 1, []string{"b", "c"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := pagePuzzles(items, tc.limit, tc.offset)
			if len(got) != len(tc.wantIDs) {
				t.Fatalf("got %d ids, want %d (%v)", len(got), len(tc.wantIDs), tc.wantIDs)
			}
			for i, id := range tc.wantIDs {
				if got[i].ID != id {
					t.Fatalf("index %d: got %q want %q", i, got[i].ID, id)
				}
			}
		})
	}
}

func TestArchivePaginationLimitsAndPages(t *testing.T) {
	base := SeedPuzzles()[0]
	makePuzzle := func(id string, number int, date string) Puzzle {
		puzzle := base
		puzzle.ID = id
		puzzle.PuzzleNumber = number
		puzzle.PublishDate = date
		puzzle.Status = PuzzleStatusPublished
		return puzzle
	}
	// All four are published on or before the fixed clock date (2026-06-02).
	puzzles := []Puzzle{
		makePuzzle("a", 1, "2026-05-30"),
		makePuzzle("b", 2, "2026-05-31"),
		makePuzzle("c", 3, "2026-06-01"),
		makePuzzle("d", 4, "2026-06-02"),
	}
	handler := NewServer(ServerConfig{Puzzles: StaticPuzzleSource(puzzles), Clock: fixedClock})

	if first := archiveIDs(t, handler, "/api/puzzles?limit=2"); len(first) != 2 || first[0] != "d" || first[1] != "c" {
		t.Fatalf("expected newest two [d c], got %v", first)
	}
	if second := archiveIDs(t, handler, "/api/puzzles?limit=2&offset=2"); len(second) != 2 || second[0] != "b" || second[1] != "a" {
		t.Fatalf("expected next two [b a], got %v", second)
	}
	// An over-max limit is clamped, not rejected, so all four still fit.
	if all := archiveIDs(t, handler, "/api/puzzles?limit=9999"); len(all) != 4 {
		t.Fatalf("expected all four within the clamped max, got %d", len(all))
	}
	// Junk params fall back to defaults rather than erroring.
	if def := archiveIDs(t, handler, "/api/puzzles?limit=abc&offset=-9"); len(def) != 4 {
		t.Fatalf("expected default page to include all four, got %d", len(def))
	}
}

func archiveIDs(t *testing.T, handler http.Handler, target string) []string {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: expected 200, got %d: %s", target, rec.Code, rec.Body.String())
	}
	var puzzles []PublicPuzzle
	if err := json.NewDecoder(rec.Body).Decode(&puzzles); err != nil {
		t.Fatalf("decode archive: %v", err)
	}
	ids := make([]string, 0, len(puzzles))
	for _, puzzle := range puzzles {
		ids = append(ids, puzzle.ID)
	}
	return ids
}
