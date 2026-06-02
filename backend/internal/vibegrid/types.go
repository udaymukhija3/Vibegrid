package vibegrid

type Difficulty string
type PuzzleStatus string

const (
	DifficultyEasy   Difficulty = "EASY"
	DifficultyMedium Difficulty = "MEDIUM"
	DifficultyHard   Difficulty = "HARD"

	PuzzleStatusDraft     PuzzleStatus = "DRAFT"
	PuzzleStatusPublished PuzzleStatus = "PUBLISHED"
	PuzzleStatusArchived  PuzzleStatus = "ARCHIVED"

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
	SolvedGroups   []SolvedGroup `json:"solvedGroups"`
	RevealedGroups []SolvedGroup `json:"revealedGroups"`
	Mistakes       int           `json:"mistakes"`
	GuessCount     int           `json:"guessCount"`
	StartedAt      string        `json:"startedAt"`
	CompletedAt    *string       `json:"completedAt,omitempty"`
	Failed         bool          `json:"failed"`
	Completed      bool          `json:"completed"`
}

type GuessRequest struct {
	PuzzleID        string   `json:"puzzleId"`
	SelectedTileIDs []string `json:"selectedTileIds"`
	ClientGuessID   string   `json:"clientGuessId"`
}

type GuessResponse struct {
	OK             bool             `json:"ok"`
	IsCorrect      bool             `json:"isCorrect"`
	Group          *SolvedGroup     `json:"group,omitempty"`
	Attempt        *AttemptSnapshot `json:"attempt,omitempty"`
	RevealedGroups []SolvedGroup    `json:"revealedGroups,omitempty"`
	SessionID      string           `json:"sessionId,omitempty"`
	Error          string           `json:"error,omitempty"`
}
