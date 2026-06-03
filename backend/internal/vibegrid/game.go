package vibegrid

import (
	"errors"
	"sort"
)

var (
	ErrInvalidGroupSize = errors.New("pick exactly four unique tiles")
	ErrUnknownTile      = errors.New("this guess contains a tile that is not in the puzzle")
	ErrAlreadySolved    = errors.New("that vibe is already locked")
)

func EvaluateGuess(puzzle Puzzle, selectedTileIDs []string, solvedGroupIDs map[string]bool) (*PuzzleGroup, error) {
	normalized := normalizeTileIDs(selectedTileIDs)
	if len(normalized) != GroupSize {
		return nil, ErrInvalidGroupSize
	}

	validTileIDs := map[string]bool{}
	for _, group := range puzzle.Groups {
		for _, tile := range group.Tiles {
			validTileIDs[tile.ID] = true
		}
	}

	for _, tileID := range normalized {
		if !validTileIDs[tileID] {
			return nil, ErrUnknownTile
		}
	}

	for index := range puzzle.Groups {
		group := &puzzle.Groups[index]
		groupTileIDs := make([]string, 0, len(group.Tiles))
		for _, tile := range group.Tiles {
			groupTileIDs = append(groupTileIDs, tile.ID)
		}

		if tileIDsMatch(groupTileIDs, normalized) {
			if solvedGroupIDs[group.ID] {
				return nil, ErrAlreadySolved
			}

			return group, nil
		}
	}

	return nil, nil
}

// maxGroupOverlap returns the largest number of selected tiles that fall within
// a single group. A wrong guess with an overlap of GroupSize-1 (three of four)
// is "one away" — the only near-miss signal the game reveals.
func maxGroupOverlap(puzzle Puzzle, selectedTileIDs []string) int {
	selected := map[string]bool{}
	for _, tileID := range normalizeTileIDs(selectedTileIDs) {
		selected[tileID] = true
	}

	best := 0
	for _, group := range puzzle.Groups {
		count := 0
		for _, tile := range group.Tiles {
			if selected[tile.ID] {
				count++
			}
		}
		if count > best {
			best = count
		}
	}
	return best
}

// IsOneAway reports whether a guess has exactly three tiles in a single group.
func IsOneAway(puzzle Puzzle, selectedTileIDs []string) bool {
	return maxGroupOverlap(puzzle, selectedTileIDs) == GroupSize-1
}

func normalizeTileIDs(tileIDs []string) []string {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(tileIDs))
	for _, tileID := range tileIDs {
		if seen[tileID] {
			continue
		}
		seen[tileID] = true
		normalized = append(normalized, tileID)
	}

	sort.Strings(normalized)
	return normalized
}

func tileIDsMatch(left []string, right []string) bool {
	normalizedLeft := normalizeTileIDs(left)
	normalizedRight := normalizeTileIDs(right)
	if len(normalizedLeft) != len(normalizedRight) {
		return false
	}

	for index := range normalizedLeft {
		if normalizedLeft[index] != normalizedRight[index] {
			return false
		}
	}

	return true
}
