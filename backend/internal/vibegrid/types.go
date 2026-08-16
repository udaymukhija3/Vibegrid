package vibegrid

type Difficulty string
type PuzzleStatus string
type PuzzleOrigin string
type AttemptMode string

const (
	DifficultyEasy   Difficulty = "EASY"
	DifficultyMedium Difficulty = "MEDIUM"
	DifficultyHard   Difficulty = "HARD"

	AttemptModeEasy   AttemptMode = "easy"
	AttemptModeMedium AttemptMode = "medium"
	AttemptModeHard   AttemptMode = "hard"

	PuzzleStatusDraft     PuzzleStatus = "DRAFT"
	PuzzleStatusPending   PuzzleStatus = "PENDING"
	PuzzleStatusPublished PuzzleStatus = "PUBLISHED"
	PuzzleStatusArchived  PuzzleStatus = "ARCHIVED"

	// OriginEditorial puzzles are the curated daily/archive set. OriginCommunity
	// puzzles are user-created, held for review, and playable only by direct link
	// after approval.
	OriginEditorial PuzzleOrigin = "EDITORIAL"
	OriginCommunity PuzzleOrigin = "COMMUNITY"

	GroupSize   = 4
	MaxMistakes = 4
)

type Tile struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type PuzzleGroup struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Explanation string `json:"explanation"`
	ColorIndex  int    `json:"colorIndex"`
	Tiles       []Tile `json:"tiles"`
}

type Puzzle struct {
	ID           string        `json:"id"`
	PuzzleNumber int           `json:"puzzleNumber"`
	PublishDate  string        `json:"publishDate"`
	Status       PuzzleStatus  `json:"status"`
	Difficulty   Difficulty    `json:"difficulty"`
	Origin       PuzzleOrigin  `json:"origin"`
	Groups       []PuzzleGroup `json:"groups"`
}

type PublicPuzzle struct {
	ID              string     `json:"id"`
	PuzzleNumber    int        `json:"puzzleNumber"`
	PublishDate     string     `json:"publishDate"`
	Difficulty      Difficulty `json:"difficulty"`
	Tiles           []Tile     `json:"tiles"`
	GroupCount      int        `json:"groupCount"`
	MistakesAllowed int        `json:"mistakesAllowed"`
}

// VibeHint is a single group's name + colour, with no tile mapping. It powers
// guided Easy/Medium play without leaking which tiles belong to which group; the
// guess engine stays the sole authority.
type VibeHint struct {
	Name       string `json:"name"`
	ColorIndex int    `json:"colorIndex"`
}

// EasyHint is the extra clue unlocked by Easy mode after the player has made a
// couple of guesses. It intentionally omits tile membership, so the server still
// owns answer validation.
type EasyHint struct {
	Name       string `json:"name"`
	ColorIndex int    `json:"colorIndex"`
	Text       string `json:"text"`
}

type EasyHintResponse struct {
	Available          bool      `json:"available"`
	GuessCount         int       `json:"guessCount"`
	RequiredGuessCount int       `json:"requiredGuessCount"`
	Hint               *EasyHint `json:"hint,omitempty"`
}

type SolvedGroup struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Explanation string   `json:"explanation"`
	ColorIndex  int      `json:"colorIndex"`
	TileIDs     []string `json:"tileIds"`
	Tiles       []Tile   `json:"tiles"`
}

type AttemptSnapshot struct {
	PuzzleID       string        `json:"puzzleId"`
	Mode           AttemptMode   `json:"mode,omitempty"`
	SolvedGroups   []SolvedGroup `json:"solvedGroups"`
	RevealedGroups []SolvedGroup `json:"revealedGroups"`
	Mistakes       int           `json:"mistakes"`
	GuessCount     int           `json:"guessCount"`
	StartedAt      string        `json:"startedAt"`
	CompletedAt    *string       `json:"completedAt,omitempty"`
	Failed         bool          `json:"failed"`
	Completed      bool          `json:"completed"`
	// GuessHistory is the ordered list of every submitted guess (the tile ids per
	// guess). It is server-authoritative so any client — including a second tab
	// that never witnessed the guesses — can render the spoiler-safe share grid.
	GuessHistory [][]string `json:"guessHistory"`
}

type StreakSummary struct {
	CurrentStreak  int `json:"currentStreak"`
	LongestStreak  int `json:"longestStreak"`
	TotalCompleted int `json:"totalCompleted"`
}

type GuessRequest struct {
	PuzzleID        string      `json:"puzzleId"`
	SelectedTileIDs []string    `json:"selectedTileIds"`
	ClientGuessID   string      `json:"clientGuessId"`
	Mode            AttemptMode `json:"mode"`
}

type GuessResponse struct {
	OK             bool             `json:"ok"`
	IsCorrect      bool             `json:"isCorrect"`
	Group          *SolvedGroup     `json:"group,omitempty"`
	Attempt        *AttemptSnapshot `json:"attempt,omitempty"`
	OneAway        bool             `json:"oneAway,omitempty"`
	RevealedGroups []SolvedGroup    `json:"revealedGroups,omitempty"`
	Error          string           `json:"error,omitempty"`
}
