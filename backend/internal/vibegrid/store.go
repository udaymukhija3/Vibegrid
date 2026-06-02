package vibegrid

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

var ErrAttemptFinished = errors.New("attempt is already finished")

// Store owns mutable per-session game state: attempts and the guesses made
// against them. Puzzle content is static and served from the seed package, so
// the store only deals with attempt lifecycle and idempotent guess handling.
//
// Two implementations exist: MemoryAttemptStore (default, used in tests and for
// no-database local runs) and PostgresAttemptStore (durable, transaction-safe).
// Handlers depend on this interface, never a concrete store.
type Store interface {
	GetOrCreate(ctx context.Context, puzzle Puzzle, sessionID string, now time.Time) (AttemptSnapshot, error)
	SubmitGuess(ctx context.Context, puzzle Puzzle, sessionID string, request GuessRequest, now time.Time) (GuessSubmission, error)
}

type StoredGuess struct {
	IsCorrect      bool
	MatchedGroupID string
	Revealed       bool
}

type GuessSubmission struct {
	IsCorrect bool
	Group     *SolvedGroup
	Attempt   AttemptSnapshot
}

// attemptState is the minimal, store-agnostic view of an attempt. Both stores
// project their storage into this struct so snapshot and submission building
// stays in one place and the two implementations cannot drift apart.
type attemptState struct {
	PuzzleID       string
	SessionID      string
	Mistakes       int
	GuessCount     int
	StartedAt      time.Time
	CompletedAt    *time.Time
	Failed         bool
	SolvedGroupIDs map[string]bool
}

func (state attemptState) completed(puzzle Puzzle) bool {
	return len(state.SolvedGroupIDs) == len(puzzle.Groups)
}

// applyGuess mutates the state for a freshly evaluated guess and returns the
// StoredGuess that should be persisted. matchedGroup is nil for a valid-but-wrong
// guess. This is the single source of truth for mistake/completion/failure
// transitions, shared by both stores.
func (state *attemptState) applyGuess(puzzle Puzzle, matchedGroup *PuzzleGroup, now time.Time) StoredGuess {
	state.GuessCount++
	storedGuess := StoredGuess{}

	if matchedGroup != nil {
		state.SolvedGroupIDs[matchedGroup.ID] = true
		storedGuess.IsCorrect = true
		storedGuess.MatchedGroupID = matchedGroup.ID

		if state.completed(puzzle) {
			completedAt := now.UTC()
			state.CompletedAt = &completedAt
		}
		return storedGuess
	}

	state.Mistakes++
	if state.Mistakes >= MaxMistakes {
		state.Failed = true
		storedGuess.Revealed = true
	}
	return storedGuess
}

func buildSnapshot(puzzle Puzzle, state attemptState) AttemptSnapshot {
	solvedGroups := make([]SolvedGroup, 0, len(state.SolvedGroupIDs))
	for _, group := range puzzle.Groups {
		if state.SolvedGroupIDs[group.ID] {
			solvedGroups = append(solvedGroups, SolvedGroupFor(group))
		}
	}

	sort.Slice(solvedGroups, func(left, right int) bool {
		return solvedGroups[left].ColorIndex < solvedGroups[right].ColorIndex
	})

	var completedAt *string
	if state.CompletedAt != nil {
		formatted := state.CompletedAt.Format(time.RFC3339)
		completedAt = &formatted
	}

	revealedGroups := []SolvedGroup{}
	if state.Failed {
		revealedGroups = AllSolvedGroups(puzzle)
	}

	return AttemptSnapshot{
		PuzzleID:       state.PuzzleID,
		SolvedGroups:   solvedGroups,
		RevealedGroups: revealedGroups,
		Mistakes:       state.Mistakes,
		GuessCount:     state.GuessCount,
		StartedAt:      state.StartedAt.Format(time.RFC3339),
		CompletedAt:    completedAt,
		Failed:         state.Failed,
		Completed:      state.completed(puzzle),
	}
}

func buildSubmission(puzzle Puzzle, state attemptState, storedGuess StoredGuess) GuessSubmission {
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
		Attempt:   buildSnapshot(puzzle, state),
	}
}

// MemoryAttemptStore keeps attempts in a mutex-guarded map. It is correct and
// idempotent within a single process but not durable or shared across
// instances; PostgresAttemptStore is the production path.
type MemoryAttemptStore struct {
	mu       sync.Mutex
	attempts map[string]*memoryAttempt
}

type memoryAttempt struct {
	state   attemptState
	guesses map[string]StoredGuess
}

func NewMemoryAttemptStore() *MemoryAttemptStore {
	return &MemoryAttemptStore{
		attempts: map[string]*memoryAttempt{},
	}
}

func (store *MemoryAttemptStore) GetOrCreate(_ context.Context, puzzle Puzzle, sessionID string, now time.Time) (AttemptSnapshot, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	attempt := store.getOrCreateLocked(puzzle.ID, sessionID, now)
	return buildSnapshot(puzzle, attempt.state), nil
}

func (store *MemoryAttemptStore) SubmitGuess(_ context.Context, puzzle Puzzle, sessionID string, request GuessRequest, now time.Time) (GuessSubmission, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	attempt := store.getOrCreateLocked(puzzle.ID, sessionID, now)

	if storedGuess, ok := attempt.guesses[request.ClientGuessID]; ok {
		return buildSubmission(puzzle, attempt.state, storedGuess), nil
	}

	if attempt.state.completed(puzzle) || attempt.state.Failed {
		return GuessSubmission{}, ErrAttemptFinished
	}

	matchedGroup, err := EvaluateGuess(puzzle, request.SelectedTileIDs, attempt.state.SolvedGroupIDs)
	if err != nil {
		return GuessSubmission{}, err
	}

	storedGuess := attempt.state.applyGuess(puzzle, matchedGroup, now)
	attempt.guesses[request.ClientGuessID] = storedGuess
	return buildSubmission(puzzle, attempt.state, storedGuess), nil
}

func (store *MemoryAttemptStore) getOrCreateLocked(puzzleID string, sessionID string, now time.Time) *memoryAttempt {
	key := attemptKey(puzzleID, sessionID)
	if attempt, ok := store.attempts[key]; ok {
		return attempt
	}

	attempt := &memoryAttempt{
		state: attemptState{
			PuzzleID:       puzzleID,
			SessionID:      sessionID,
			StartedAt:      now.UTC(),
			SolvedGroupIDs: map[string]bool{},
		},
		guesses: map[string]StoredGuess{},
	}
	store.attempts[key] = attempt
	return attempt
}

func attemptKey(puzzleID string, sessionID string) string {
	return puzzleID + ":" + sessionID
}
