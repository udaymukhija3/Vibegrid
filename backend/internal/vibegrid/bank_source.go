package vibegrid

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"sort"
	"strings"
	"time"
)

// dailyEpoch is the date of daily puzzle #1 (the first seeded puzzle). Daily
// numbers count forward from here so the player-facing "#N" stays monotonic as
// the bank takes over from the explicitly-dated seeds.
const dailyEpoch = "2026-06-02"

// bankPuzzleSource decorates a PuzzleSource so the daily never goes stale. When
// no puzzle is dated for exactly today, a date-specific board is composed from
// the evergreen bank's curated group pool and stamped with that day's id, date,
// and number — otherwise the inner source's "most recent on-or-before today"
// would freeze the daily on the last authored date. A puzzle scheduled for
// exactly today always wins, so the operator can still override any day.
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

type bankGroupCandidate struct {
	group      PuzzleGroup
	difficulty Difficulty
}

// bankDailyFor builds a deterministic daily for a date from the evergreen bank's
// group pool. This avoids the product failure mode of repeating the same finite
// boards every N days while still keeping all answer data server-side.
func (s *bankPuzzleSource) bankDailyFor(date string) (Puzzle, error) {
	if len(s.bank) == 0 {
		return Puzzle{}, ErrPuzzleNotFound
	}
	if _, ok := parseDay(date); !ok {
		return Puzzle{}, ErrPuzzleNotFound
	}

	groups, difficulty, ok := generatedDailyGroups(s.bank, date)
	if !ok {
		return Puzzle{}, ErrPuzzleNotFound
	}

	return Puzzle{
		ID:           "vibegrid-" + date,
		PuzzleNumber: dayNumber(date),
		PublishDate:  date,
		Status:       PuzzleStatusPublished,
		Origin:       OriginEditorial,
		Difficulty:   difficulty,
		Groups:       groups,
	}, nil
}

func generatedDailyGroups(bank []Puzzle, date string) ([]PuzzleGroup, Difficulty, bool) {
	candidates := bankGroupPool(bank)
	if len(candidates) < GroupSize {
		return nil, "", false
	}

	sort.SliceStable(candidates, func(left, right int) bool {
		return dailyCandidateRank(date, candidates[left].group.ID) < dailyCandidateRank(date, candidates[right].group.ID)
	})

	selected := make([]bankGroupCandidate, 0, GroupSize)
	usedText := map[string]bool{}
	for _, candidate := range candidates {
		if !groupTextAvailable(candidate.group, usedText) {
			continue
		}
		selected = append(selected, candidate)
		for _, tile := range candidate.group.Tiles {
			usedText[normalizedTileText(tile.Text)] = true
		}
		if len(selected) == GroupSize {
			break
		}
	}
	if len(selected) != GroupSize {
		return nil, "", false
	}

	groups := make([]PuzzleGroup, 0, GroupSize)
	difficultyScore := 0
	for index, candidate := range selected {
		group := cloneDailyGroup(candidate.group, index)
		groups = append(groups, group)
		difficultyScore += difficultyValue(candidate.difficulty)
	}

	return groups, difficultyFromScore(difficultyScore), true
}

func bankGroupPool(bank []Puzzle) []bankGroupCandidate {
	candidates := []bankGroupCandidate{}
	for _, puzzle := range bank {
		for _, group := range puzzle.Groups {
			candidates = append(candidates, bankGroupCandidate{
				group:      group,
				difficulty: puzzle.Difficulty,
			})
		}
	}
	return candidates
}

func dailyCandidateRank(date string, groupID string) uint64 {
	sum := sha256.Sum256([]byte(date + ":" + groupID))
	return binary.BigEndian.Uint64(sum[:8])
}

func groupTextAvailable(group PuzzleGroup, used map[string]bool) bool {
	for _, tile := range group.Tiles {
		if used[normalizedTileText(tile.Text)] {
			return false
		}
	}
	return true
}

func normalizedTileText(text string) string {
	return strings.ToLower(strings.TrimSpace(text))
}

func cloneDailyGroup(group PuzzleGroup, colorIndex int) PuzzleGroup {
	cloned := group
	cloned.ColorIndex = colorIndex
	cloned.Tiles = append([]Tile(nil), group.Tiles...)
	return cloned
}

func difficultyValue(difficulty Difficulty) int {
	switch difficulty {
	case DifficultyEasy:
		return 1
	case DifficultyHard:
		return 3
	default:
		return 2
	}
}

func difficultyFromScore(score int) Difficulty {
	average := float64(score) / GroupSize
	switch {
	case average < 1.75:
		return DifficultyEasy
	case average > 2.35:
		return DifficultyHard
	default:
		return DifficultyMedium
	}
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
