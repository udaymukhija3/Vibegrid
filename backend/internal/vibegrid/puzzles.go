package vibegrid

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"sort"
	"time"
)

var ErrPuzzleNotFound = errors.New("puzzle not found")

func FindPuzzleByID(puzzles []Puzzle, puzzleID string) (*Puzzle, error) {
	for i := range puzzles {
		if puzzles[i].ID == puzzleID {
			return &puzzles[i], nil
		}
	}

	return nil, ErrPuzzleNotFound
}

// PublishedPuzzles returns the editorial daily set in reverse date order.
// Community (user-created) puzzles are intentionally excluded so they never
// appear in Today or the Archive; they are reachable only by direct link.
func PublishedPuzzles(puzzles []Puzzle) []Puzzle {
	published := make([]Puzzle, 0, len(puzzles))
	for _, puzzle := range puzzles {
		if puzzle.Status == PuzzleStatusPublished && puzzle.Origin != OriginCommunity {
			published = append(published, puzzle)
		}
	}

	sort.Slice(published, func(left, right int) bool {
		return published[left].PublishDate > published[right].PublishDate
	})

	return published
}

// PublishedPuzzlesThrough returns editorial puzzles that are live on or before
// today. This is the public archive view and prevents scheduled puzzles from
// leaking before their publish date.
func PublishedPuzzlesThrough(puzzles []Puzzle, today string) []Puzzle {
	published := make([]Puzzle, 0, len(puzzles))
	for _, puzzle := range PublishedPuzzles(puzzles) {
		if puzzle.PublishDate != "" && puzzle.PublishDate <= today {
			published = append(published, puzzle)
		}
	}
	return published
}

func TodaysPuzzle(puzzles []Puzzle, now time.Time, timeZone string) (*Puzzle, error) {
	location, err := time.LoadLocation(timeZone)
	if err != nil {
		location = time.UTC
	}

	today := now.In(location).Format("2006-01-02")
	published := PublishedPuzzlesThrough(puzzles, today)
	if len(published) > 0 {
		return &published[0], nil
	}

	return nil, ErrPuzzleNotFound
}

// pagePuzzles returns the limit/offset window of an already-ordered puzzle
// slice, guarding against out-of-range bounds. A non-positive limit returns
// everything from offset onward.
func pagePuzzles(puzzles []Puzzle, limit, offset int) []Puzzle {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(puzzles) {
		return []Puzzle{}
	}
	end := len(puzzles)
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return puzzles[offset:end]
}

func PubliclyPlayable(puzzle Puzzle, today string) bool {
	if puzzle.Status != PuzzleStatusPublished {
		return false
	}
	if puzzle.Origin == OriginCommunity {
		return true
	}
	return puzzle.PublishDate != "" && puzzle.PublishDate <= today
}

func ToPublicPuzzle(puzzle Puzzle) PublicPuzzle {
	tiles := make([]Tile, 0, len(puzzle.Groups)*GroupSize)
	for _, group := range puzzle.Groups {
		tiles = append(tiles, group.Tiles...)
	}

	return PublicPuzzle{
		ID:              puzzle.ID,
		PuzzleNumber:    puzzle.PuzzleNumber,
		PublishDate:     puzzle.PublishDate,
		Difficulty:      puzzle.Difficulty,
		Tiles:           orderTilesForDisplay(tiles),
		GroupCount:      len(puzzle.Groups),
		MistakesAllowed: MaxMistakes,
	}
}

func SolvedGroupFor(group PuzzleGroup) SolvedGroup {
	tileIDs := make([]string, 0, len(group.Tiles))
	for _, tile := range group.Tiles {
		tileIDs = append(tileIDs, tile.ID)
	}

	return SolvedGroup{
		ID:          group.ID,
		Name:        group.Name,
		Explanation: group.Explanation,
		ColorIndex:  group.ColorIndex,
		TileIDs:     tileIDs,
		Tiles:       append([]Tile(nil), group.Tiles...),
	}
}

func AllSolvedGroups(puzzle Puzzle) []SolvedGroup {
	groups := make([]SolvedGroup, 0, len(puzzle.Groups))
	for _, group := range puzzle.Groups {
		groups = append(groups, SolvedGroupFor(group))
	}

	sort.Slice(groups, func(left, right int) bool {
		return groups[left].ColorIndex < groups[right].ColorIndex
	})

	return groups
}

// orderTilesForDisplay returns the puzzle's tiles in a stable order that is
// independent of group membership. The order is derived from a hash of each
// tile's id, so it is identical on every request (a stable board that does not
// reshuffle on refresh) yet encodes nothing about which tiles share a group.
//
// This replaces a Fisher-Yates shuffle seeded by the public puzzle number:
// because that seed and the algorithm were both knowable by the client, the
// permutation could be inverted to recover the original group-blocked layout —
// i.e. the answer key. Tile ids are assigned independently of grouping, so a
// per-tile hash ordering leaks nothing an attacker did not already have.
func orderTilesForDisplay(items []Tile) []Tile {
	ordered := append([]Tile(nil), items...)
	sort.SliceStable(ordered, func(left, right int) bool {
		leftKey, rightKey := tileOrderKey(ordered[left].ID), tileOrderKey(ordered[right].ID)
		if leftKey != rightKey {
			return leftKey < rightKey
		}
		// Deterministic tiebreak if two ids ever hash to the same prefix.
		return ordered[left].ID < ordered[right].ID
	})
	return ordered
}

func tileOrderKey(id string) uint64 {
	sum := sha256.Sum256([]byte(id))
	return binary.BigEndian.Uint64(sum[:8])
}
