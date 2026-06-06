package vibegrid

import (
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
		Tiles:           seededShuffle(tiles, uint32(puzzle.PuzzleNumber)),
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

func seededShuffle(items []Tile, seed uint32) []Tile {
	shuffled := append([]Tile(nil), items...)
	state := seed
	if state == 0 {
		state = 1
	}

	for index := len(shuffled) - 1; index > 0; index-- {
		state = state*1664525 + 1013904223
		swapIndex := int(state % uint32(index+1))
		shuffled[index], shuffled[swapIndex] = shuffled[swapIndex], shuffled[index]
	}

	return shuffled
}
