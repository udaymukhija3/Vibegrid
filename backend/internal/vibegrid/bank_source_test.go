package vibegrid

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBankFillsDailyWhenNothingScheduled(t *testing.T) {
	source := NewBankPuzzleSource(StaticPuzzleSource(SeedPuzzles()), PuzzleBank())

	const future = "2026-07-01" // well past the seeded range (ends 2026-06-10)
	daily, err := source.TodaysPuzzle(context.Background(), future)
	if err != nil {
		t.Fatalf("expected a bank daily, got error: %v", err)
	}
	if daily.ID != "vibegrid-"+future {
		t.Fatalf("unexpected id %q", daily.ID)
	}
	if daily.PublishDate != future || daily.Status != PuzzleStatusPublished || daily.Origin != OriginEditorial {
		t.Fatalf("daily not stamped correctly: %#v", daily)
	}
	if len(daily.Groups) != 4 {
		t.Fatalf("expected 4 groups, got %d", len(daily.Groups))
	}
	if !PubliclyPlayable(daily, future) {
		t.Fatal("a bank daily should be publicly playable on its own date")
	}
}

func TestScheduledPuzzleWinsOverBank(t *testing.T) {
	source := NewBankPuzzleSource(StaticPuzzleSource(SeedPuzzles()), PuzzleBank())

	daily, err := source.TodaysPuzzle(context.Background(), "2026-06-02")
	if err != nil {
		t.Fatal(err)
	}
	if daily.ID != "vibegrid-2026-06-02" {
		t.Fatalf("expected the seeded puzzle to win, got %q", daily.ID)
	}
}

func TestBankDailyResolvableByID(t *testing.T) {
	source := NewBankPuzzleSource(StaticPuzzleSource(SeedPuzzles()), PuzzleBank())

	const date = "2026-08-15"
	byID, err := source.PuzzleByID(context.Background(), "vibegrid-"+date)
	if err != nil {
		t.Fatalf("expected to resolve a bank daily by id, got %v", err)
	}
	today, err := source.TodaysPuzzle(context.Background(), date)
	if err != nil {
		t.Fatal(err)
	}
	if byID.ID != today.ID || byID.Groups[0].ID != today.Groups[0].ID {
		t.Fatalf("id resolution diverged from today: %q vs %q", byID.ID, today.ID)
	}

	if _, err := source.PuzzleByID(context.Background(), "totally-made-up"); err == nil {
		t.Fatal("expected ErrPuzzleNotFound for a non-date id")
	}
}

func TestBankDailyNumberContinuesFromSeeds(t *testing.T) {
	// Seeds run #1 (2026-06-02) .. #9 (2026-06-10); the first bank day is #10.
	if got := dayNumber("2026-06-02"); got != 1 {
		t.Fatalf("dailyEpoch number = %d, want 1", got)
	}
	if got := dayNumber("2026-06-11"); got != 10 {
		t.Fatalf("first bank day number = %d, want 10", got)
	}

	source := NewBankPuzzleSource(StaticPuzzleSource(SeedPuzzles()), PuzzleBank())
	daily, err := source.TodaysPuzzle(context.Background(), "2026-06-11")
	if err != nil {
		t.Fatal(err)
	}
	if daily.PuzzleNumber != 10 {
		t.Fatalf("bank daily #%d, want #10", daily.PuzzleNumber)
	}
}

func TestBankRotationIsDeterministicAndCycles(t *testing.T) {
	source := NewBankPuzzleSource(StaticPuzzleSource(nil), PuzzleBank()) // empty inner ⇒ always bank

	a, err := source.TodaysPuzzle(context.Background(), "2026-09-01")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := source.TodaysPuzzle(context.Background(), "2026-09-01")
	if a.Groups[0].ID != b.Groups[0].ID {
		t.Fatal("same date produced different bank entries")
	}

	// A date exactly len(bank) days later must select the same entry.
	bank := PuzzleBank()
	start, _ := parseDay("2026-09-01")
	repeat := time.Unix((start+int64(len(bank)))*86400, 0).UTC().Format("2006-01-02")
	c, _ := source.TodaysPuzzle(context.Background(), repeat)
	if a.Groups[0].ID != c.Groups[0].ID {
		t.Fatalf("rotation did not cycle after len(bank) days: %q vs %q", a.Groups[0].ID, c.Groups[0].ID)
	}
}

func TestDailyEndpointNeverEmptyWithBank(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles:  NewBankPuzzleSource(StaticPuzzleSource(SeedPuzzles()), PuzzleBank()),
		Store:    NewMemoryAttemptStore(),
		Clock:    func() time.Time { return time.Date(2026, 12, 25, 12, 0, 0, 0, time.UTC) },
		TimeZone: "UTC",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/puzzles/today", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("daily should never 404 with a bank; got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestPuzzleBankIsWellFormed guards the hand-written content: every entry must be
// a valid 4×4 puzzle with four distinct colours and no duplicate tile text/ids
// within a puzzle (a duplicate would make a guess ambiguous or unfair).
func TestPuzzleBankIsWellFormed(t *testing.T) {
	bank := PuzzleBank()
	if len(bank) < 7 {
		t.Fatalf("bank unexpectedly small: %d", len(bank))
	}

	seenPuzzleID := map[string]bool{}
	for _, puzzle := range bank {
		if seenPuzzleID[puzzle.ID] {
			t.Fatalf("duplicate bank puzzle id %q", puzzle.ID)
		}
		seenPuzzleID[puzzle.ID] = true

		if len(puzzle.Groups) != 4 {
			t.Fatalf("%s: expected 4 groups, got %d", puzzle.ID, len(puzzle.Groups))
		}

		colors := map[int]bool{}
		tileText := map[string]bool{}
		tileID := map[string]bool{}
		for _, group := range puzzle.Groups {
			if group.Name == "" {
				t.Fatalf("%s: group %s missing a name", puzzle.ID, group.ID)
			}
			colors[group.ColorIndex] = true
			if len(group.Tiles) != GroupSize {
				t.Fatalf("%s/%s: expected %d tiles, got %d", puzzle.ID, group.ID, GroupSize, len(group.Tiles))
			}
			for _, tile := range group.Tiles {
				if tile.Text == "" {
					t.Fatalf("%s: empty tile text", puzzle.ID)
				}
				if tileText[tile.Text] {
					t.Fatalf("%s: duplicate tile text %q within the puzzle", puzzle.ID, tile.Text)
				}
				tileText[tile.Text] = true
				if tileID[tile.ID] {
					t.Fatalf("%s: duplicate tile id %q", puzzle.ID, tile.ID)
				}
				tileID[tile.ID] = true
			}
		}
		if len(colors) != 4 {
			t.Fatalf("%s: expected 4 distinct colour indexes, got %d", puzzle.ID, len(colors))
		}
	}
}
