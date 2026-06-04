package vibegrid

import "testing"

func TestSeedPuzzlesAreValid(t *testing.T) {
	seenIDs := map[string]bool{}
	seenNumbers := map[int]bool{}
	seenDates := map[string]bool{}

	for _, puzzle := range SeedPuzzles() {
		if seenIDs[puzzle.ID] {
			t.Fatalf("duplicate puzzle id %q", puzzle.ID)
		}
		seenIDs[puzzle.ID] = true

		if seenNumbers[puzzle.PuzzleNumber] {
			t.Fatalf("duplicate puzzle number %d", puzzle.PuzzleNumber)
		}
		seenNumbers[puzzle.PuzzleNumber] = true

		if puzzle.PublishDate == "" {
			t.Fatalf("seed puzzle %q is missing a publish date", puzzle.ID)
		}
		if seenDates[puzzle.PublishDate] {
			t.Fatalf("duplicate publish date %q", puzzle.PublishDate)
		}
		seenDates[puzzle.PublishDate] = true

		input := AdminPuzzleInput{Difficulty: puzzle.Difficulty}
		for _, group := range puzzle.Groups {
			tiles := make([]string, 0, len(group.Tiles))
			for _, tile := range group.Tiles {
				tiles = append(tiles, tile.Text)
			}
			input.Groups = append(input.Groups, AdminGroupInput{
				Name:        group.Name,
				Explanation: group.Explanation,
				Tiles:       tiles,
			})
		}

		if err := input.Validate(); err != nil {
			t.Fatalf("seed puzzle %q is invalid: %v", puzzle.ID, err)
		}
	}
}
