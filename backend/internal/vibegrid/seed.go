package vibegrid

func SeedPuzzles() []Puzzle {
	return []Puzzle{
		{
			ID:           "vibegrid-2026-06-02",
			PuzzleNumber: 1,
			PublishDate:  "2026-06-02",
			Status:       PuzzleStatusPublished,
			Difficulty:   DifficultyMedium,
			Groups: []PuzzleGroup{
				{
					ID:          "italian-summer",
					Name:        "Italian summer",
					Explanation: "A tiny holiday built from espresso, fabric, wheels, and sun.",
					ColorIndex:  0,
					Tiles: []Tile{
						{ID: "p1-espresso", Text: "espresso"},
						{ID: "p1-linen", Text: "linen"},
						{ID: "p1-vespa", Text: "Vespa"},
						{ID: "p1-balcony", Text: "balcony"},
					},
				},
				{
					ID:          "corporate-hostage-situation",
					Name:        "Corporate hostage situation",
					Explanation: "The meeting before the meeting, now with adrenaline.",
					ColorIndex:  1,
					Tiles: []Tile{
						{ID: "p1-slack", Text: "Slack"},
						{ID: "p1-deck", Text: "deck"},
						{ID: "p1-panic", Text: "panic"},
						{ID: "p1-959", Text: "9:59"},
					},
				},
				{
					ID:          "noir-evening",
					Name:        "Noir evening",
					Explanation: "A moody window scene with enough jazz to make the lamp suspicious.",
					ColorIndex:  2,
					Tiles: []Tile{
						{ID: "p1-rain", Text: "rain"},
						{ID: "p1-jazz", Text: "jazz"},
						{ID: "p1-window", Text: "window"},
						{ID: "p1-lamp", Text: "lamp"},
					},
				},
				{
					ID:          "gym-bro-morning",
					Name:        "Gym bro morning",
					Explanation: "Breakfast, recovery layer, and the sacred lift.",
					ColorIndex:  3,
					Tiles: []Tile{
						{ID: "p1-oats", Text: "oats"},
						{ID: "p1-whey", Text: "whey"},
						{ID: "p1-hoodie", Text: "hoodie"},
						{ID: "p1-deadlift", Text: "deadlift"},
					},
				},
			},
		},
	}
}
