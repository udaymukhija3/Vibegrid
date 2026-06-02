package vibegrid

import (
	"errors"
	"sort"
	"sync"
	"time"
)

var ErrAttemptFinished = errors.New("attempt is already finished")

type StoredGuess struct {
	IsCorrect      bool
	MatchedGroupID string
	Revealed       bool
}

type Attempt struct {
	PuzzleID       string
	SessionID      string
	Mistakes       int
	GuessCount     int
	StartedAt      time.Time
	CompletedAt    *time.Time
	Failed         bool
	SolvedGroupIDs map[string]bool
	Guesses        map[string]StoredGuess
}

type GuessSubmission struct {
	IsCorrect bool
	Group     *SolvedGroup
	Attempt   AttemptSnapshot
}

type MemoryAttemptStore struct {
	mu       sync.Mutex
	attempts map[string]*Attempt
}

func NewMemoryAttemptStore() *MemoryAttemptStore {
	return &MemoryAttemptStore{
		attempts: map[string]*Attempt{},
	}
}

func (store *MemoryAttemptStore) GetOrCreate(puzzle Puzzle, sessionID string, now time.Time) AttemptSnapshot {
	store.mu.Lock()
	defer store.mu.Unlock()

	attempt := store.getOrCreateLocked(puzzle.ID, sessionID, now)
	return snapshotFor(puzzle, attempt)
}

func (store *MemoryAttemptStore) SubmitGuess(puzzle Puzzle, sessionID string, request GuessRequest, now time.Time) (GuessSubmission, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	attempt := store.getOrCreateLocked(puzzle.ID, sessionID, now)

	if storedGuess, ok := attempt.Guesses[request.ClientGuessID]; ok {
		return submissionForStoredGuess(puzzle, attempt, storedGuess), nil
	}

	if isAttemptComplete(puzzle, attempt) || attempt.Failed {
		return GuessSubmission{}, ErrAttemptFinished
	}

	matchedGroup, err := EvaluateGuess(puzzle, request.SelectedTileIDs, attempt.SolvedGroupIDs)
	if err != nil {
		return GuessSubmission{}, err
	}

	attempt.GuessCount++
	storedGuess := StoredGuess{}

	if matchedGroup != nil {
		attempt.SolvedGroupIDs[matchedGroup.ID] = true
		storedGuess.IsCorrect = true
		storedGuess.MatchedGroupID = matchedGroup.ID

		if isAttemptComplete(puzzle, attempt) {
			completedAt := now.UTC()
			attempt.CompletedAt = &completedAt
		}
	} else {
		attempt.Mistakes++
		if attempt.Mistakes >= MaxMistakes {
			attempt.Failed = true
			storedGuess.Revealed = true
		}
	}

	attempt.Guesses[request.ClientGuessID] = storedGuess
	return submissionForStoredGuess(puzzle, attempt, storedGuess), nil
}

func (store *MemoryAttemptStore) getOrCreateLocked(puzzleID string, sessionID string, now time.Time) *Attempt {
	key := attemptKey(puzzleID, sessionID)
	if attempt, ok := store.attempts[key]; ok {
		return attempt
	}

	attempt := &Attempt{
		PuzzleID:       puzzleID,
		SessionID:      sessionID,
		StartedAt:      now.UTC(),
		SolvedGroupIDs: map[string]bool{},
		Guesses:        map[string]StoredGuess{},
	}
	store.attempts[key] = attempt
	return attempt
}

func attemptKey(puzzleID string, sessionID string) string {
	return puzzleID + ":" + sessionID
}

func submissionForStoredGuess(puzzle Puzzle, attempt *Attempt, storedGuess StoredGuess) GuessSubmission {
	var group *SolvedGroup
	if storedGuess.MatchedGroupID != "" {
		for _, puzzleGroup := range puzzle.Groups {
			if puzzleGroup.ID == storedGuess.MatchedGroupID {
				solved := SolvedGroupFor(puzzleGroup)
				group = &solved
				break
			}
		}
	}

	return GuessSubmission{
		IsCorrect: storedGuess.IsCorrect,
		Group:     group,
		Attempt:   snapshotFor(puzzle, attempt),
	}
}

func snapshotFor(puzzle Puzzle, attempt *Attempt) AttemptSnapshot {
	solvedGroups := make([]SolvedGroup, 0, len(attempt.SolvedGroupIDs))
	for _, group := range puzzle.Groups {
		if attempt.SolvedGroupIDs[group.ID] {
			solvedGroups = append(solvedGroups, SolvedGroupFor(group))
		}
	}

	sort.Slice(solvedGroups, func(left, right int) bool {
		return solvedGroups[left].ColorIndex < solvedGroups[right].ColorIndex
	})

	var completedAt *string
	if attempt.CompletedAt != nil {
		formatted := attempt.CompletedAt.Format(time.RFC3339)
		completedAt = &formatted
	}

	revealedGroups := []SolvedGroup{}
	if attempt.Failed {
		revealedGroups = AllSolvedGroups(puzzle)
	}

	return AttemptSnapshot{
		PuzzleID:       attempt.PuzzleID,
		SolvedGroups:   solvedGroups,
		RevealedGroups: revealedGroups,
		Mistakes:       attempt.Mistakes,
		GuessCount:     attempt.GuessCount,
		StartedAt:      attempt.StartedAt.Format(time.RFC3339),
		CompletedAt:    completedAt,
		Failed:         attempt.Failed,
		Completed:      isAttemptComplete(puzzle, attempt),
	}
}

func isAttemptComplete(puzzle Puzzle, attempt *Attempt) bool {
	return len(attempt.SolvedGroupIDs) == len(puzzle.Groups)
}
