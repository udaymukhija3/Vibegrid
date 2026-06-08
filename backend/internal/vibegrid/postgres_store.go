package vibegrid

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // register the "pgx" database/sql driver
	"github.com/lib/pq"
	"github.com/pressly/goose/v3"

	"github.com/vibegrid/vibegrid/backend/db"
)

// pgUniqueViolation is the SQLSTATE Postgres returns for a unique-constraint
// violation. We use it to detect a replayed client guess racing against itself.
const pgUniqueViolation = "23505"

// Keep DB work below the outer HTTP timeout so handlers can return intentional
// errors instead of letting http.TimeoutHandler be the first line of defense.
const databaseOperationTimeout = 4 * time.Second

func withDatabaseTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, databaseOperationTimeout)
}

// seedTimeout bounds the one-time startup seed. Unlike request handlers, the
// seed inserts many rows in a single transaction and runs off the request path,
// so the tight per-request databaseOperationTimeout is far too small for it —
// especially when the database is remote (app and DB in different regions),
// where the per-statement round trips add up.
const seedTimeout = 2 * time.Minute

func withSeedTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, seedTimeout)
}

// ConnectDB opens a connection pool and verifies connectivity. The attempt
// store and puzzle store share the returned pool; the caller owns Close.
func ConnectDB(ctx context.Context, databaseURL string) (*sql.DB, error) {
	return openDB(ctx, databaseURL, false)
}

// OpenDB opens a connection pool, verifies connectivity, and applies migrations.
// It is used by the explicit migrate command and integration tests.
func OpenDB(ctx context.Context, databaseURL string) (*sql.DB, error) {
	return openDB(ctx, databaseURL, true)
}

func openDB(ctx context.Context, databaseURL string, applyMigrations bool) (*sql.DB, error) {
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	database.SetMaxOpenConns(10)
	database.SetMaxIdleConns(5)
	database.SetConnMaxLifetime(time.Hour)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := database.PingContext(pingCtx); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	if applyMigrations {
		if err := runMigrations(database); err != nil {
			_ = database.Close()
			return nil, err
		}
	}

	return database, nil
}

func MigrateDB(ctx context.Context, databaseURL string) error {
	database, err := OpenDB(ctx, databaseURL)
	if err != nil {
		return err
	}
	if err := database.Close(); err != nil {
		return fmt.Errorf("close migrated postgres pool: %w", err)
	}
	return nil
}

func runMigrations(database *sql.DB) error {
	goose.SetBaseFS(db.Migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}

	if err := goose.Up(database, "migrations"); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}

// PostgresAttemptStore is the durable, transaction-safe Store. Every guess is
// recorded inside a single transaction that locks the attempt row, so
// concurrent or retried submissions for the same session cannot corrupt
// mistake counts, double-count guesses, or race past completion.
type PostgresAttemptStore struct {
	db *sql.DB
}

func NewPostgresAttemptStore(database *sql.DB) *PostgresAttemptStore {
	return &PostgresAttemptStore{db: database}
}

func (store *PostgresAttemptStore) GetAttempt(ctx context.Context, puzzle Puzzle, sessionID string, now time.Time) (AttemptSnapshot, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	// Read-only: the attempt row is created lazily on the first guess (see
	// SubmitGuess -> ensureAttempt), so loading a board never writes. A session
	// that has not guessed yet gets a fresh, empty snapshot.
	state, err := store.readState(ctx, store.db, puzzle.ID, sessionID, false)
	if errors.Is(err, sql.ErrNoRows) {
		return buildSnapshot(puzzle, freshState(puzzle.ID, sessionID, now)), nil
	}
	if err != nil {
		return AttemptSnapshot{}, err
	}

	history, err := store.guessHistoryBySession(ctx, store.db, puzzle.ID, sessionID)
	if err != nil {
		return AttemptSnapshot{}, err
	}
	state.GuessHistory = history

	return buildSnapshot(puzzle, state), nil
}

func (store *PostgresAttemptStore) SubmitGuess(ctx context.Context, puzzle Puzzle, sessionID string, request GuessRequest, now time.Time) (GuessSubmission, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	if err := store.ensureAttempt(ctx, puzzle.ID, sessionID, now); err != nil {
		return GuessSubmission{}, err
	}

	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return GuessSubmission{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Lock the attempt row for the duration of the transaction so concurrent
	// guesses for this session serialize behind us.
	state, err := store.readState(ctx, tx, puzzle.ID, sessionID, true)
	if err != nil {
		return GuessSubmission{}, err
	}

	var attemptID string
	if err := tx.QueryRowContext(ctx,
		`select id from attempts where puzzle_id = $1 and session_id = $2`,
		puzzle.ID, sessionID,
	).Scan(&attemptID); err != nil {
		return GuessSubmission{}, fmt.Errorf("load attempt id: %w", err)
	}

	// Hydrate the prior guess history under the same lock so both the
	// idempotent-replay response and a freshly applied guess return the full,
	// ordered list a second tab needs to rebuild the share grid.
	history, err := store.guessHistoryByAttempt(ctx, tx, attemptID)
	if err != nil {
		return GuessSubmission{}, err
	}
	state.GuessHistory = history

	// Idempotency: a previously recorded client guess returns its original result.
	if stored, ok, err := store.findGuess(ctx, tx, attemptID, request.ClientGuessID); err != nil {
		return GuessSubmission{}, err
	} else if ok {
		if err := tx.Commit(); err != nil {
			return GuessSubmission{}, fmt.Errorf("commit: %w", err)
		}
		return buildSubmission(puzzle, state, stored), nil
	}

	if state.completed(puzzle) || state.Failed {
		return GuessSubmission{}, ErrAttemptFinished
	}

	matchedGroup, err := EvaluateGuess(puzzle, request.SelectedTileIDs, state.SolvedGroupIDs)
	if err != nil {
		return GuessSubmission{}, err
	}

	storedGuess := state.applyGuess(puzzle, matchedGroup, request.SelectedTileIDs, now)

	if err := store.insertGuess(ctx, tx, attemptID, request, storedGuess); err != nil {
		// A concurrent insert of the same client guess id won the race. Treat it
		// as idempotent rather than an error: replay the stored result.
		if isUniqueViolation(err) {
			if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				return GuessSubmission{}, fmt.Errorf("rollback after race: %w", err)
			}
			return store.replayGuess(ctx, puzzle, sessionID, request.ClientGuessID)
		}
		return GuessSubmission{}, err
	}

	if err := store.updateAttempt(ctx, tx, attemptID, state); err != nil {
		return GuessSubmission{}, err
	}

	if err := tx.Commit(); err != nil {
		return GuessSubmission{}, fmt.Errorf("commit: %w", err)
	}

	return buildSubmission(puzzle, state, storedGuess), nil
}

// ensureAttempt inserts the attempt row if it does not exist. The unique key on
// (puzzle_id, session_id) keeps this to exactly one attempt per session.
func (store *PostgresAttemptStore) ensureAttempt(ctx context.Context, puzzleID, sessionID string, now time.Time) error {
	_, err := store.db.ExecContext(ctx,
		`insert into attempts (puzzle_id, session_id, started_at)
		 values ($1, $2, $3)
		 on conflict (puzzle_id, session_id) do nothing`,
		puzzleID, sessionID, now.UTC(),
	)
	if err != nil {
		return fmt.Errorf("ensure attempt: %w", err)
	}
	return nil
}

// rowQuerier is satisfied by both *sql.DB and *sql.Tx so readState can run
// locked (in a transaction) or unlocked (for plain reads).
type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// rowsQuerier is satisfied by both *sql.DB and *sql.Tx so guess history can be
// loaded locked (inside SubmitGuess's transaction) or unlocked (plain reads).
type rowsQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// guessHistoryByAttempt returns every guess for an attempt as ordered tile-id
// rows. Guesses for one attempt are serialized by the row lock in SubmitGuess,
// so created_at is strictly increasing and gives a stable submission order.
func (store *PostgresAttemptStore) guessHistoryByAttempt(ctx context.Context, q rowsQuerier, attemptID string) ([][]string, error) {
	rows, err := q.QueryContext(ctx,
		`select selected_tile_ids from attempt_guesses
		 where attempt_id = $1 order by created_at asc`,
		attemptID,
	)
	if err != nil {
		return nil, fmt.Errorf("load guess history: %w", err)
	}
	defer rows.Close()
	return scanGuessHistory(rows)
}

// guessHistoryBySession loads guess history without a known attempt id, joining
// through attempts on (puzzle_id, session_id). Used by the read paths.
func (store *PostgresAttemptStore) guessHistoryBySession(ctx context.Context, q rowsQuerier, puzzleID, sessionID string) ([][]string, error) {
	rows, err := q.QueryContext(ctx,
		`select g.selected_tile_ids from attempt_guesses g
		 join attempts a on a.id = g.attempt_id
		 where a.puzzle_id = $1 and a.session_id = $2
		 order by g.created_at asc`,
		puzzleID, sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("load guess history: %w", err)
	}
	defer rows.Close()
	return scanGuessHistory(rows)
}

func scanGuessHistory(rows *sql.Rows) ([][]string, error) {
	history := [][]string{}
	for rows.Next() {
		var tileIDs []string
		if err := rows.Scan(pq.Array(&tileIDs)); err != nil {
			return nil, fmt.Errorf("scan guess history: %w", err)
		}
		history = append(history, tileIDs)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate guess history: %w", err)
	}
	return history, nil
}

func (store *PostgresAttemptStore) readState(ctx context.Context, q rowQuerier, puzzleID, sessionID string, forUpdate bool) (attemptState, error) {
	query := `select puzzle_id, session_id, mistakes, guess_count, failed,
	                 started_at, completed_at, solved_group_ids
	          from attempts where puzzle_id = $1 and session_id = $2`
	if forUpdate {
		query += " for update"
	}

	var (
		state          attemptState
		completedAt    sql.NullTime
		solvedGroupIDs []string
	)
	err := q.QueryRowContext(ctx, query, puzzleID, sessionID).Scan(
		&state.PuzzleID,
		&state.SessionID,
		&state.Mistakes,
		&state.GuessCount,
		&state.Failed,
		&state.StartedAt,
		&completedAt,
		pq.Array(&solvedGroupIDs),
	)
	if err != nil {
		return attemptState{}, fmt.Errorf("read attempt: %w", err)
	}

	if completedAt.Valid {
		state.CompletedAt = &completedAt.Time
	}
	state.SolvedGroupIDs = make(map[string]bool, len(solvedGroupIDs))
	for _, id := range solvedGroupIDs {
		state.SolvedGroupIDs[id] = true
	}
	return state, nil
}

func (store *PostgresAttemptStore) findGuess(ctx context.Context, tx *sql.Tx, attemptID, clientGuessID string) (StoredGuess, bool, error) {
	var (
		stored         StoredGuess
		matchedGroupID sql.NullString
	)
	err := tx.QueryRowContext(ctx,
		`select is_correct, matched_group_id, revealed
		 from attempt_guesses where attempt_id = $1 and client_guess_id = $2`,
		attemptID, clientGuessID,
	).Scan(&stored.IsCorrect, &matchedGroupID, &stored.Revealed)
	if errors.Is(err, sql.ErrNoRows) {
		return StoredGuess{}, false, nil
	}
	if err != nil {
		return StoredGuess{}, false, fmt.Errorf("find guess: %w", err)
	}

	if matchedGroupID.Valid {
		stored.MatchedGroupID = matchedGroupID.String
	}
	return stored, true, nil
}

func (store *PostgresAttemptStore) insertGuess(ctx context.Context, tx *sql.Tx, attemptID string, request GuessRequest, stored StoredGuess) error {
	var matchedGroupID sql.NullString
	if stored.MatchedGroupID != "" {
		matchedGroupID = sql.NullString{String: stored.MatchedGroupID, Valid: true}
	}

	_, err := tx.ExecContext(ctx,
		`insert into attempt_guesses
		   (attempt_id, client_guess_id, selected_tile_ids, is_correct, matched_group_id, revealed)
		 values ($1, $2, $3, $4, $5, $6)`,
		attemptID, request.ClientGuessID, pq.Array(request.SelectedTileIDs),
		stored.IsCorrect, matchedGroupID, stored.Revealed,
	)
	if err != nil {
		return fmt.Errorf("insert guess: %w", err)
	}
	return nil
}

func (store *PostgresAttemptStore) updateAttempt(ctx context.Context, tx *sql.Tx, attemptID string, state attemptState) error {
	solvedGroupIDs := make([]string, 0, len(state.SolvedGroupIDs))
	for id := range state.SolvedGroupIDs {
		solvedGroupIDs = append(solvedGroupIDs, id)
	}

	var completedAt sql.NullTime
	if state.CompletedAt != nil {
		completedAt = sql.NullTime{Time: *state.CompletedAt, Valid: true}
	}

	_, err := tx.ExecContext(ctx,
		`update attempts
		 set mistakes = $1,
		     guess_count = $2,
		     failed = $3,
		     completed = $4,
		     completed_at = $5,
		     solved_group_ids = $6
		 where id = $7`,
		state.Mistakes,
		state.GuessCount,
		state.Failed,
		completedAt.Valid,
		completedAt,
		pq.Array(solvedGroupIDs),
		attemptID,
	)
	if err != nil {
		return fmt.Errorf("update attempt: %w", err)
	}
	return nil
}

// replayGuess re-reads committed state after losing an insert race and returns
// the winner's stored result, so the loser still gets an idempotent response.
func (store *PostgresAttemptStore) replayGuess(ctx context.Context, puzzle Puzzle, sessionID, clientGuessID string) (GuessSubmission, error) {
	state, err := store.readState(ctx, store.db, puzzle.ID, sessionID, false)
	if err != nil {
		return GuessSubmission{}, err
	}

	history, err := store.guessHistoryBySession(ctx, store.db, puzzle.ID, sessionID)
	if err != nil {
		return GuessSubmission{}, err
	}
	state.GuessHistory = history

	var attemptID string
	if err := store.db.QueryRowContext(ctx,
		`select id from attempts where puzzle_id = $1 and session_id = $2`,
		puzzle.ID, sessionID,
	).Scan(&attemptID); err != nil {
		return GuessSubmission{}, fmt.Errorf("replay load attempt id: %w", err)
	}

	var (
		stored         StoredGuess
		matchedGroupID sql.NullString
	)
	if err := store.db.QueryRowContext(ctx,
		`select is_correct, matched_group_id, revealed
		 from attempt_guesses where attempt_id = $1 and client_guess_id = $2`,
		attemptID, clientGuessID,
	).Scan(&stored.IsCorrect, &matchedGroupID, &stored.Revealed); err != nil {
		return GuessSubmission{}, fmt.Errorf("replay find guess: %w", err)
	}
	if matchedGroupID.Valid {
		stored.MatchedGroupID = matchedGroupID.String
	}

	return buildSubmission(puzzle, state, stored), nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation
}
