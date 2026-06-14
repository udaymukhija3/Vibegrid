package vibegrid

import (
	"context"
	"hash/fnv"
	"strings"
)

const demoPuzzlePrefix = "demo-"

// demoPuzzleSource adds stable, seeded demo-room puzzles to any public puzzle
// source. A room URL like /demo/investor-01 maps to puzzle id demo-investor-01;
// attempts still stay per browser session, so opening the same URL in a second
// browser simulates another player without accounts or authoring setup.
type demoPuzzleSource struct {
	inner PuzzleSource
}

func NewDemoPuzzleSource(inner PuzzleSource) PuzzleSource {
	return &demoPuzzleSource{inner: inner}
}

func (source *demoPuzzleSource) Puzzles(ctx context.Context) ([]Puzzle, error) {
	return source.inner.Puzzles(ctx)
}

func (source *demoPuzzleSource) PublishedPuzzles(ctx context.Context, today string, limit, offset int) ([]Puzzle, error) {
	return source.inner.PublishedPuzzles(ctx, today, limit, offset)
}

func (source *demoPuzzleSource) TodaysPuzzle(ctx context.Context, today string) (Puzzle, error) {
	return source.inner.TodaysPuzzle(ctx, today)
}

func (source *demoPuzzleSource) PuzzleByID(ctx context.Context, puzzleID string) (Puzzle, error) {
	if room, ok := demoRoomFromPuzzleID(puzzleID); ok {
		return demoPuzzleForRoom(room), nil
	}
	return source.inner.PuzzleByID(ctx, puzzleID)
}

func demoRoomFromPuzzleID(puzzleID string) (string, bool) {
	if !strings.HasPrefix(puzzleID, demoPuzzlePrefix) {
		return "", false
	}
	room := strings.TrimPrefix(puzzleID, demoPuzzlePrefix)
	if !validDemoRoom(room) {
		return "", false
	}
	return room, true
}

func validDemoRoom(room string) bool {
	if len(room) < 4 || len(room) > 48 || room[0] == '-' || room[len(room)-1] == '-' {
		return false
	}
	for _, char := range room {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' {
			continue
		}
		return false
	}
	return true
}

func demoPuzzleForRoom(room string) Puzzle {
	return Puzzle{
		ID:           demoPuzzlePrefix + room,
		PuzzleNumber: demoPuzzleNumber(room),
		Status:       PuzzleStatusPublished,
		Origin:       OriginCommunity,
		Difficulty:   DifficultyEasy,
		Groups: []PuzzleGroup{
			demoGroup("demo-g0", "Launch checks", "Things you do before sending a public demo.", 0,
				"open link", "try mobile", "copy result", "check console"),
			demoGroup("demo-g1", "Second-browser test", "A quick way to see the guest flow from another seat.", 1,
				"private window", "fresh cookie", "same room", "new attempt"),
			demoGroup("demo-g2", "Puzzle moments", "The core loop a viewer should feel in the first minute.", 2,
				"pick four", "one away", "locked group", "share grid"),
			demoGroup("demo-g3", "Creator path", "How a player turns the game into a link for someone else.", 3,
				"starter pack", "edit tiles", "publish link", "send friend"),
		},
	}
}

func demoPuzzleNumber(room string) int {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(room))
	return 9000 + int(hash.Sum32()%1000)
}

func demoGroup(id, name, explanation string, colorIndex int, tiles ...string) PuzzleGroup {
	built := make([]Tile, len(tiles))
	for index, text := range tiles {
		built[index] = Tile{ID: id + "-t" + string(rune('0'+index)), Text: text}
	}
	return PuzzleGroup{ID: id, Name: name, Explanation: explanation, ColorIndex: colorIndex, Tiles: built}
}
