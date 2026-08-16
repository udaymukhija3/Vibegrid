package vibegrid

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

var ErrAttemptFinished = errors.New("attempt is already finished")

var ErrAttemptModeConflict = errors.New("attempt mode is already locked")

var ErrAttemptCapacity = errors.New("anonymous attempt capacity is temporarily full")

// Store owns mutable per-session game state: attempts and the guesses made
// against them. Puzzle content is static and served from the seed package, so
// the store only deals with attempt lifecycle and idempotent guess handling.
//
// Two implementations exist: MemoryAttemptStore (default, used in tests and for
// no-database local runs) and PostgresAttemptStore (durable, transaction-safe).
// Handlers depend on this interface, never a concrete store.
type Store interface {
	// GetAttempt returns the current snapshot for a session's attempt. It never
	// writes: a session that has not guessed yet gets a fresh, empty snapshot and
	// no row is created until the first guess (see SubmitGuess). This keeps the
	// read path free of writes and stops an unauthenticated caller from inflating
	// the attempts table by hitting it with throwaway sessions.
	GetAttempt(ctx context.Context, puzzle Puzzle, sessionID string, now time.Time) (AttemptSnapshot, error)
	SubmitGuess(ctx context.Context, puzzle Puzzle, sessionID string, request GuessRequest, now time.Time) (GuessSubmission, error)
}

// freshState is the zero-guess starting state for an attempt that does not yet
// exist in storage. Shared so read-only reads and lazy creation agree.
func freshState(puzzleID, sessionID string, now time.Time) attemptState {
	return attemptState{
		PuzzleID:       puzzleID,
		SessionID:      sessionID,
		StartedAt:      now.UTC(),
		SolvedGroupIDs: map[string]bool{},
	}
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
	Mode           AttemptMode
	Mistakes       int
	GuessCount     int
	StartedAt      time.Time
	CompletedAt    *time.Time
	Failed         bool
	SolvedGroupIDs map[string]bool
	// GuessHistory is every submitted guess in order, each as the tile ids that
	// were guessed. The Postgres store hydrates it from attempt_guesses; the
	// memory store accumulates it as guesses are applied.
	GuessHistory [][]string
}

func (state attemptState) completed(puzzle Puzzle) bool {
	return len(state.SolvedGroupIDs) == len(puzzle.Groups)
}

// applyGuess mutates the state for a freshly evaluated guess and returns the
// StoredGuess that should be persisted. matchedGroup is nil for a valid-but-wrong
// guess. This is the single source of truth for mistake/completion/failure
// transitions, shared by both stores.
func (state *attemptState) applyGuess(puzzle Puzzle, matchedGroup *PuzzleGroup, selectedTileIDs []string, now time.Time) StoredGuess {
	state.GuessCount++
	state.GuessHistory = append(state.GuessHistory, append([]string(nil), selectedTileIDs...))
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
		completedAt := now.UTC()
		state.Failed = true
		state.CompletedAt = &completedAt
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

	guessHistory := state.GuessHistory
	if guessHistory == nil {
		guessHistory = [][]string{}
	}

	return AttemptSnapshot{
		PuzzleID:       state.PuzzleID,
		Mode:           state.Mode,
		SolvedGroups:   solvedGroups,
		RevealedGroups: revealedGroups,
		Mistakes:       state.Mistakes,
		GuessCount:     state.GuessCount,
		StartedAt:      state.StartedAt.Format(time.RFC3339),
		CompletedAt:    completedAt,
		Failed:         state.Failed,
		Completed:      state.completed(puzzle),
		GuessHistory:   guessHistory,
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
	mu          sync.Mutex
	attempts    map[string]*memoryAttempt
	maxAttempts int
	retention   time.Duration
}

const (
	defaultMemoryAttemptLimit = 10_000
	defaultAttemptRetention   = 30 * 24 * time.Hour
)

type memoryAttempt struct {
	state   attemptState
	guesses map[string]StoredGuess
}

func NewMemoryAttemptStore() *MemoryAttemptStore {
	return &MemoryAttemptStore{
		attempts:    map[string]*memoryAttempt{},
		maxAttempts: defaultMemoryAttemptLimit,
		retention:   defaultAttemptRetention,
	}
}

func (store *MemoryAttemptStore) GetAttempt(_ context.Context, puzzle Puzzle, sessionID string, now time.Time) (AttemptSnapshot, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.pruneExpiredLocked(now)

	if attempt, ok := store.attempts[attemptKey(puzzle.ID, sessionID)]; ok {
		return buildSnapshot(puzzle, attempt.state), nil
	}
	return buildSnapshot(puzzle, freshState(puzzle.ID, sessionID, now)), nil
}

func (store *MemoryAttemptStore) SubmitGuess(_ context.Context, puzzle Puzzle, sessionID string, request GuessRequest, now time.Time) (GuessSubmission, error) {
	if err := validateGuessRequest(request); err != nil {
		return GuessSubmission{}, err
	}
	// Reject syntactically valid but unknown tile ids before allocating state for a
	// new anonymous session. The actual match is recalculated below with the
	// attempt's solved groups.
	if _, err := EvaluateGuess(puzzle, request.SelectedTileIDs, map[string]bool{}); err != nil {
		return GuessSubmission{}, err
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	store.pruneExpiredLocked(now)

	attempt, err := store.getOrCreateLocked(puzzle.ID, sessionID, request.Mode, now)
	if err != nil {
		return GuessSubmission{}, err
	}
	if attempt.state.Mode != request.Mode {
		return GuessSubmission{}, ErrAttemptModeConflict
	}

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

	storedGuess := attempt.state.applyGuess(puzzle, matchedGroup, request.SelectedTileIDs, now)
	attempt.guesses[request.ClientGuessID] = storedGuess
	return buildSubmission(puzzle, attempt.state, storedGuess), nil
}

func (store *MemoryAttemptStore) getOrCreateLocked(puzzleID string, sessionID string, mode AttemptMode, now time.Time) (*memoryAttempt, error) {
	key := attemptKey(puzzleID, sessionID)
	if attempt, ok := store.attempts[key]; ok {
		return attempt, nil
	}
	if store.maxAttempts > 0 && len(store.attempts) >= store.maxAttempts {
		return nil, ErrAttemptCapacity
	}

	attempt := &memoryAttempt{
		state:   freshState(puzzleID, sessionID, now),
		guesses: map[string]StoredGuess{},
	}
	attempt.state.Mode = mode
	store.attempts[key] = attempt
	return attempt, nil
}

func (store *MemoryAttemptStore) pruneExpiredLocked(now time.Time) {
	if store.retention <= 0 {
		return
	}
	cutoff := now.UTC().Add(-store.retention)
	for key, attempt := range store.attempts {
		if attempt.state.StartedAt.Before(cutoff) {
			delete(store.attempts, key)
		}
	}
}

func attemptKey(puzzleID string, sessionID string) string {
	return puzzleID + ":" + sessionID
}
