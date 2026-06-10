package vibegrid

import (
	"context"
	"errors"
	"strings"
	"time"
)

// dailyEpoch is the date of daily puzzle #1 (the first seeded puzzle). Daily
// numbers count forward from here so the player-facing "#N" stays monotonic as
// the bank takes over from the explicitly-dated seeds.
const dailyEpoch = "2026-06-02"

// bankPuzzleSource decorates a PuzzleSource so the daily never goes stale. When
// no puzzle is dated for exactly today, a puzzle is chosen from an evergreen bank
// and stamped with that day's id, date, and number — otherwise the inner source's
// "most recent on-or-before today" would freeze the daily on the last authored
// date. A puzzle scheduled for exactly today always wins, so the operator can
// still override any day.
//
// Synthesized dailies are never persisted: attempts key off puzzle_id (which has
// no foreign key to puzzles), so play, scoring, sharing, and stats all work
// against a puzzle that exists only in memory.
type bankPuzzleSource struct {
	inner PuzzleSource
	bank  []Puzzle
}

// NewBankPuzzleSource wraps inner with bank-backed daily rotation. If the bank is
// empty it behaves exactly like inner.
func NewBankPuzzleSource(inner PuzzleSource, bank []Puzzle) PuzzleSource {
	return &bankPuzzleSource{inner: inner, bank: bank}
}

func (s *bankPuzzleSource) Puzzles(ctx context.Context) ([]Puzzle, error) {
	return s.inner.Puzzles(ctx)
}

func (s *bankPuzzleSource) PublishedPuzzles(ctx context.Context, today string, limit, offset int) ([]Puzzle, error) {
	// The archive lists explicitly-scheduled puzzles only; bank dailies remain
	// playable by their date id but are not enumerated here.
	return s.inner.PublishedPuzzles(ctx, today, limit, offset)
}

func (s *bankPuzzleSource) TodaysPuzzle(ctx context.Context, today string) (Puzzle, error) {
	puzzle, err := s.inner.TodaysPuzzle(ctx, today)
	switch {
	case err == nil && puzzle.PublishDate == today:
		// An explicit puzzle is scheduled for exactly today — it always wins.
		return puzzle, nil
	case err != nil && !errors.Is(err, ErrPuzzleNotFound):
		return Puzzle{}, err
	}
	// Either nothing is scheduled, or the inner source only has an older puzzle
	// (its "today" is the most recent on-or-before date, which would otherwise
	// leave the daily frozen on a stale date). Fill today from the evergreen bank.
	return s.bankDailyFor(today)
}

func (s *bankPuzzleSource) PuzzleByID(ctx context.Context, puzzleID string) (Puzzle, error) {
	puzzle, err := s.inner.PuzzleByID(ctx, puzzleID)
	if err == nil {
		return puzzle, nil
	}
	if !errors.Is(err, ErrPuzzleNotFound) {
		return Puzzle{}, err
	}
	// Resolve a synthesized daily by its date id (vibegrid-YYYY-MM-DD) so it can be
	// played by link, scored, and shared like any other daily. PubliclyPlayable in
	// the server still gates future dates, so this can safely build any valid date.
	if date, ok := dailyDateFromID(puzzleID); ok {
		return s.bankDailyFor(date)
	}
	return Puzzle{}, ErrPuzzleNotFound
}

// bankDailyFor builds the daily for a date from the evergreen bank, rotating by
// day so the series cycles through the whole bank before any entry repeats.
func (s *bankPuzzleSource) bankDailyFor(date string) (Puzzle, error) {
	if len(s.bank) == 0 {
		return Puzzle{}, ErrPuzzleNotFound
	}
	day, ok := parseDay(date)
	if !ok {
		return Puzzle{}, ErrPuzzleNotFound
	}

	size := int64(len(s.bank))
	index := ((day % size) + size) % size
	daily := s.bank[index]

	// Stamp the entry as that day's daily. Groups (and their tile ids) are shared
	// read-only from the bank entry; nothing downstream mutates them in place.
	daily.ID = "vibegrid-" + date
	daily.PublishDate = date
	daily.Status = PuzzleStatusPublished
	daily.Origin = OriginEditorial
	daily.PuzzleNumber = dayNumber(date)
	return daily, nil
}

// parseDay returns the number of whole days since the Unix epoch for a
// YYYY-MM-DD date (parsed as UTC midnight), and whether the date was valid.
func parseDay(date string) (int64, bool) {
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil {
		return 0, false
	}
	return parsed.Unix() / 86400, true
}

// dayNumber is the 1-based daily index counted from dailyEpoch, so the
// player-facing "#N" continues seamlessly from the seeded puzzles.
func dayNumber(date string) int {
	day, ok := parseDay(date)
	if !ok {
		return 0
	}
	epoch, _ := parseDay(dailyEpoch)
	return int(day-epoch) + 1
}

// dailyDateFromID extracts the YYYY-MM-DD from a daily id like
// "vibegrid-2026-06-11", returning false for anything that is not a valid date id.
func dailyDateFromID(id string) (string, bool) {
	const prefix = "vibegrid-"
	if !strings.HasPrefix(id, prefix) {
		return "", false
	}
	date := strings.TrimPrefix(id, prefix)
	if _, ok := parseDay(date); !ok {
		return "", false
	}
	return date, true
}
