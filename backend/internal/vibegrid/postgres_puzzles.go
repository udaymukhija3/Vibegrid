package vibegrid

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"
)

var ErrPublishDateTaken = errors.New("a puzzle is already published for that date")
var ErrPuzzleNotPending = errors.New("puzzle is not a pending community submission")

// PostgresPuzzleStore reads and writes puzzle content in Postgres. It satisfies
// PuzzleSource for the public read path and adds admin authoring operations.
type PostgresPuzzleStore struct {
	db *sql.DB
}

func NewPostgresPuzzleStore(database *sql.DB) *PostgresPuzzleStore {
	return &PostgresPuzzleStore{db: database}
}

// Puzzles loads every puzzle with its groups and tiles. It is intentionally kept
// for admin list views; public traffic should use the targeted methods below.
func (store *PostgresPuzzleStore) Puzzles(ctx context.Context) ([]Puzzle, error) {
	return store.loadPuzzleSet(ctx,
		`select id, puzzle_number, publish_date, status, difficulty, origin
		 from puzzles
		 order by puzzle_number`)
}

func (store *PostgresPuzzleStore) PublishedPuzzles(ctx context.Context, today string, limit, offset int) ([]Puzzle, error) {
	return store.loadPuzzleSet(ctx,
		`select id, puzzle_number, publish_date, status, difficulty, origin
		 from puzzles
		 where status = 'PUBLISHED'
		   and origin <> 'COMMUNITY'
		   and publish_date is not null
		   and publish_date <= $1::date
		 order by publish_date desc, puzzle_number desc
		 limit $2 offset $3`,
		today, limit, offset,
	)
}

func (store *PostgresPuzzleStore) TodaysPuzzle(ctx context.Context, today string) (Puzzle, error) {
	puzzles, err := store.loadPuzzleSet(ctx,
		`select id, puzzle_number, publish_date, status, difficulty, origin
		 from puzzles
		 where status = 'PUBLISHED'
		   and origin <> 'COMMUNITY'
		   and publish_date is not null
		   and publish_date <= $1::date
		 order by publish_date desc, puzzle_number desc
		 limit 1`,
		today,
	)
	if err != nil {
		return Puzzle{}, err
	}
	if len(puzzles) == 0 {
		return Puzzle{}, ErrPuzzleNotFound
	}
	return puzzles[0], nil
}

func (store *PostgresPuzzleStore) PuzzleByID(ctx context.Context, puzzleID string) (Puzzle, error) {
	puzzles, err := store.loadPuzzleSet(ctx,
		`select id, puzzle_number, publish_date, status, difficulty, origin
		 from puzzles
		 where id = $1`,
		puzzleID,
	)
	if err != nil {
		return Puzzle{}, err
	}
	if len(puzzles) == 0 {
		return Puzzle{}, ErrPuzzleNotFound
	}
	return puzzles[0], nil
}

// readQuerier returns the transaction driving this request, or the pool when the
// call is not part of one.
//
// Reads reached from inside an idempotent mutation must reuse that mutation's
// transaction. The transaction already holds a connection, so taking a second
// one from the same pool deadlocks it: once as many mutations are in flight as
// the pool is wide, every connection is held by a transaction waiting for a
// connection that only another one of them could release, and nothing unwinds
// until databaseOperationTimeout expires.
func (store *PostgresPuzzleStore) readQuerier(ctx context.Context) rowsQuerier {
	if tx := transactionFromContext(ctx); tx != nil {
		return tx
	}
	return store.db
}

func (store *PostgresPuzzleStore) loadPuzzleSet(ctx context.Context, query string, args ...any) ([]Puzzle, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	querier := store.readQuerier(ctx)
	rows, err := querier.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query puzzles: %w", err)
	}
	defer rows.Close()

	puzzles := map[string]*Puzzle{}
	order := []string{}
	for rows.Next() {
		var (
			puzzle      Puzzle
			publishDate sql.NullTime
		)
		if err := rows.Scan(&puzzle.ID, &puzzle.PuzzleNumber, &publishDate, &puzzle.Status, &puzzle.Difficulty, &puzzle.Origin); err != nil {
			return nil, fmt.Errorf("scan puzzle: %w", err)
		}
		if publishDate.Valid {
			puzzle.PublishDate = publishDate.Time.Format("2006-01-02")
		}
		puzzle.Groups = []PuzzleGroup{}
		stored := puzzle
		puzzles[puzzle.ID] = &stored
		order = append(order, puzzle.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(order) == 0 {
		return []Puzzle{}, nil
	}
	if err := attachGroups(ctx, querier, puzzles, order); err != nil {
		return nil, err
	}
	if err := attachTiles(ctx, querier, puzzles, order); err != nil {
		return nil, err
	}

	result := make([]Puzzle, 0, len(order))
	for _, id := range order {
		result = append(result, *puzzles[id])
	}
	return result, nil
}

func attachGroups(ctx context.Context, querier rowsQuerier, puzzles map[string]*Puzzle, puzzleIDs []string) error {
	rows, err := querier.QueryContext(ctx,
		`select id, puzzle_id, name, explanation, color_index
		 from puzzle_groups
		 where puzzle_id = any($1)
		 order by puzzle_id, sort_order`,
		pq.Array(puzzleIDs),
	)
	if err != nil {
		return fmt.Errorf("query groups: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			group    PuzzleGroup
			puzzleID string
		)
		if err := rows.Scan(&group.ID, &puzzleID, &group.Name, &group.Explanation, &group.ColorIndex); err != nil {
			return fmt.Errorf("scan group: %w", err)
		}
		group.Tiles = []Tile{}
		if puzzle, ok := puzzles[puzzleID]; ok {
			puzzle.Groups = append(puzzle.Groups, group)
		}
	}
	return rows.Err()
}

func attachTiles(ctx context.Context, querier rowsQuerier, puzzles map[string]*Puzzle, puzzleIDs []string) error {
	rows, err := querier.QueryContext(ctx,
		`select id, puzzle_id, group_id, text
		 from puzzle_tiles
		 where puzzle_id = any($1)
		 order by puzzle_id, sort_order`,
		pq.Array(puzzleIDs),
	)
	if err != nil {
		return fmt.Errorf("query tiles: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			tile     Tile
			puzzleID string
			groupID  string
		)
		if err := rows.Scan(&tile.ID, &puzzleID, &groupID, &tile.Text); err != nil {
			return fmt.Errorf("scan tile: %w", err)
		}
		puzzle, ok := puzzles[puzzleID]
		if !ok {
			continue
		}
		for index := range puzzle.Groups {
			if puzzle.Groups[index].ID == groupID {
				puzzle.Groups[index].Tiles = append(puzzle.Groups[index].Tiles, tile)
				break
			}
		}
	}
	return rows.Err()
}

// Seed inserts the given puzzles if they are not already present. on conflict do
// nothing makes it a safe bootstrap that never clobbers admin edits.
func (store *PostgresPuzzleStore) Seed(ctx context.Context, puzzles []Puzzle) error {
	ctx, cancel := withSeedTimeout(ctx)
	defer cancel()

	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin seed tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, puzzle := range puzzles {
		if err := insertPuzzleTx(ctx, tx, puzzle, true); err != nil {
			return err
		}
	}
	if err := resetPuzzleNumberSequenceTx(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

// CreateDraft persists a new DRAFT puzzle. The caller is expected to have
// validated input already; this assigns ids, the next puzzle number, and order.
func (store *PostgresPuzzleStore) CreateDraft(ctx context.Context, input AdminPuzzleInput) (Puzzle, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()
	if tx := transactionFromContext(ctx); tx != nil {
		return createDraftTx(ctx, tx, input)
	}

	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return Puzzle{}, fmt.Errorf("begin create tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	puzzle, err := createDraftTx(ctx, tx, input)
	if err != nil {
		return Puzzle{}, err
	}
	if err := tx.Commit(); err != nil {
		return Puzzle{}, fmt.Errorf("commit create: %w", err)
	}
	return puzzle, nil
}

func createDraftTx(ctx context.Context, tx *sql.Tx, input AdminPuzzleInput) (Puzzle, error) {
	nextNumber, err := nextPuzzleNumberTx(ctx, tx)
	if err != nil {
		return Puzzle{}, err
	}
	puzzle := input.toPuzzle(nextNumber)
	if err := insertPuzzleTx(ctx, tx, puzzle, false); err != nil {
		return Puzzle{}, err
	}
	return puzzle, nil
}

// CreateCommunityPuzzle persists a user-created puzzle as PENDING. It has no
// publish date and cannot be read publicly until an administrator approves it.
func (store *PostgresPuzzleStore) CreateCommunityPuzzle(ctx context.Context, input AdminPuzzleInput, claimHash string) (Puzzle, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()
	if tx := transactionFromContext(ctx); tx != nil {
		return createCommunityPuzzleTx(ctx, tx, input, claimHash)
	}

	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return Puzzle{}, fmt.Errorf("begin community create tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	puzzle, err := createCommunityPuzzleTx(ctx, tx, input, claimHash)
	if err != nil {
		return Puzzle{}, err
	}
	if err := tx.Commit(); err != nil {
		return Puzzle{}, fmt.Errorf("commit community create: %w", err)
	}
	return puzzle, nil
}

func createCommunityPuzzleTx(ctx context.Context, tx *sql.Tx, input AdminPuzzleInput, claimHash string) (Puzzle, error) {
	nextNumber, err := nextPuzzleNumberTx(ctx, tx)
	if err != nil {
		return Puzzle{}, err
	}
	puzzle := input.toPuzzle(nextNumber)
	puzzle.Status = PuzzleStatusPending
	puzzle.Origin = OriginCommunity
	if err := insertPuzzleTx(ctx, tx, puzzle, false); err != nil {
		return Puzzle{}, err
	}
	if _, err := tx.ExecContext(ctx,
		`update puzzles set creator_claim_hash = $2 where id = $1`,
		puzzle.ID, claimHash,
	); err != nil {
		return Puzzle{}, fmt.Errorf("store creator claim: %w", err)
	}
	return puzzle, nil
}

func (store *PostgresPuzzleStore) CreatorStatus(ctx context.Context, puzzleID, claimHash string) (CreatorPuzzleStatus, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()
	if tx := transactionFromContext(ctx); tx != nil {
		return loadCreatorStatus(ctx, tx, puzzleID, claimHash)
	}
	return loadCreatorStatus(ctx, store.db, puzzleID, claimHash)
}

func (store *PostgresPuzzleStore) WithdrawCommunityPuzzle(ctx context.Context, puzzleID, claimHash string) (CreatorPuzzleStatus, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()
	if tx := transactionFromContext(ctx); tx != nil {
		return withdrawCommunityPuzzle(ctx, tx, puzzleID, claimHash)
	}

	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return CreatorPuzzleStatus{}, fmt.Errorf("begin creator withdrawal: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	status, err := withdrawCommunityPuzzle(ctx, tx, puzzleID, claimHash)
	if err != nil {
		return CreatorPuzzleStatus{}, err
	}
	if err := tx.Commit(); err != nil {
		return CreatorPuzzleStatus{}, fmt.Errorf("commit creator withdrawal: %w", err)
	}
	return status, nil
}

type puzzleMutationQuerier interface {
	rowQuerier
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func loadCreatorStatus(ctx context.Context, q rowQuerier, puzzleID, claimHash string) (CreatorPuzzleStatus, error) {
	var status CreatorPuzzleStatus
	err := q.QueryRowContext(ctx,
		`select id, puzzle_number, status, updated_at, creator_withdrawn_at is not null
		 from puzzles
		 where id = $1 and origin = 'COMMUNITY' and creator_claim_hash = $2`,
		puzzleID, claimHash,
	).Scan(&status.ID, &status.PuzzleNumber, &status.Status, &status.UpdatedAt, &status.Withdrawn)
	if errors.Is(err, sql.ErrNoRows) {
		return CreatorPuzzleStatus{}, ErrCreatorClaimInvalid
	}
	if err != nil {
		return CreatorPuzzleStatus{}, fmt.Errorf("load creator status: %w", err)
	}
	return status, nil
}

func withdrawCommunityPuzzle(ctx context.Context, q puzzleMutationQuerier, puzzleID, claimHash string) (CreatorPuzzleStatus, error) {
	var status CreatorPuzzleStatus
	err := q.QueryRowContext(ctx,
		`update puzzles
		 set status = 'ARCHIVED', creator_withdrawn_at = now(), updated_at = now()
		 where id = $1
		   and origin = 'COMMUNITY'
		   and creator_claim_hash = $2
		   and status = 'PENDING'
		   and creator_withdrawn_at is null
		 returning id, puzzle_number, status, updated_at, true`,
		puzzleID, claimHash,
	).Scan(&status.ID, &status.PuzzleNumber, &status.Status, &status.UpdatedAt, &status.Withdrawn)
	if err == nil {
		return status, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return CreatorPuzzleStatus{}, fmt.Errorf("withdraw community puzzle: %w", err)
	}

	current, loadErr := loadCreatorStatus(ctx, q, puzzleID, claimHash)
	if loadErr != nil {
		return CreatorPuzzleStatus{}, loadErr
	}
	if current.Withdrawn {
		return current, nil
	}
	return CreatorPuzzleStatus{}, ErrCreatorWithdrawalUnavailable
}

func (store *PostgresPuzzleStore) ApproveCommunity(ctx context.Context, puzzleID string) error {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	result, err := store.db.ExecContext(ctx,
		`update puzzles set status = 'PUBLISHED', updated_at = now()
		 where id = $1 and origin = 'COMMUNITY' and status = 'PENDING'`,
		puzzleID,
	)
	if err != nil {
		return fmt.Errorf("approve community puzzle: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("approve community rows affected: %w", err)
	}
	if affected > 0 {
		return nil
	}
	var exists bool
	if err := store.db.QueryRowContext(ctx, `select exists(select 1 from puzzles where id = $1)`, puzzleID).Scan(&exists); err != nil {
		return fmt.Errorf("check community puzzle: %w", err)
	}
	if !exists {
		return ErrPuzzleNotFound
	}
	return ErrPuzzleNotPending
}

// Publish sets a draft live for a date. The unique constraint on publish_date
// turns a same-date collision into ErrPublishDateTaken rather than a duplicate.
func (store *PostgresPuzzleStore) Publish(ctx context.Context, puzzleID, publishDate string) error {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	result, err := store.db.ExecContext(ctx,
		`update puzzles
		 set status = 'PUBLISHED', publish_date = $1::date, updated_at = now()
		 where id = $2`,
		publishDate, puzzleID,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrPublishDateTaken
		}
		return fmt.Errorf("publish puzzle: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("publish rows affected: %w", err)
	}
	if affected == 0 {
		return ErrPuzzleNotFound
	}
	return nil
}

func (store *PostgresPuzzleStore) Archive(ctx context.Context, puzzleID string) error {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	result, err := store.db.ExecContext(ctx,
		`update puzzles
		 set status = 'ARCHIVED', updated_at = now()
		 where id = $1`,
		puzzleID,
	)
	if err != nil {
		return fmt.Errorf("archive puzzle: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("archive rows affected: %w", err)
	}
	if affected == 0 {
		return ErrPuzzleNotFound
	}
	return nil
}

func (store *PostgresPuzzleStore) Reinstate(ctx context.Context, puzzleID string) error {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	// Reinstating means undoing a takedown, so it only moves ARCHIVED back to
	// PUBLISHED. Without the status guard this was an unconditional publish, and
	// a PENDING community puzzle reinstated through the appeal queue went live
	// without ApproveCommunity — the review gate — ever running. ApproveCommunity
	// has always carried the equivalent guard.
	result, err := store.db.ExecContext(ctx,
		`update puzzles
		 set status = 'PUBLISHED', updated_at = now()
		 where id = $1 and status = 'ARCHIVED'`,
		puzzleID,
	)
	if err != nil {
		return fmt.Errorf("reinstate puzzle: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("reinstate rows affected: %w", err)
	}
	if affected == 0 {
		return ErrPuzzleNotFound
	}
	return nil
}

func nextPuzzleNumberTx(ctx context.Context, tx *sql.Tx) (int, error) {
	var nextNumber int
	if err := tx.QueryRowContext(ctx, `select nextval('puzzle_number_seq')`).Scan(&nextNumber); err != nil {
		return 0, fmt.Errorf("next puzzle number: %w", err)
	}
	return nextNumber, nil
}

func resetPuzzleNumberSequenceTx(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx,
		`select setval(
		   'puzzle_number_seq',
		   greatest(coalesce((select max(puzzle_number) from puzzles), 0) + 1, 1),
		   false
		 )`)
	if err != nil {
		return fmt.Errorf("reset puzzle number sequence: %w", err)
	}
	return nil
}

func insertPuzzleTx(ctx context.Context, tx *sql.Tx, puzzle Puzzle, ignoreConflicts bool) error {
	puzzleInsert := `insert into puzzles (id, puzzle_number, publish_date, status, difficulty, origin)
	                 values ($1, $2, $3, $4, $5, $6)`
	if ignoreConflicts {
		puzzleInsert += " on conflict (id) do nothing"
	}

	var publishDate sql.NullString
	if puzzle.PublishDate != "" {
		publishDate = sql.NullString{String: puzzle.PublishDate, Valid: true}
	}

	origin := puzzle.Origin
	if origin == "" {
		origin = OriginEditorial
	}

	if _, err := tx.ExecContext(ctx, puzzleInsert,
		puzzle.ID, puzzle.PuzzleNumber, publishDate, puzzle.Status, puzzle.Difficulty, origin); err != nil {
		return fmt.Errorf("insert puzzle: %w", err)
	}

	for groupIndex, group := range puzzle.Groups {
		groupInsert := `insert into puzzle_groups (id, puzzle_id, name, explanation, color_index, sort_order)
		                values ($1, $2, $3, $4, $5, $6)`
		if ignoreConflicts {
			groupInsert += " on conflict (id) do nothing"
		}
		if _, err := tx.ExecContext(ctx, groupInsert,
			group.ID, puzzle.ID, group.Name, group.Explanation, group.ColorIndex, groupIndex); err != nil {
			return fmt.Errorf("insert group: %w", err)
		}

		for tileIndex, tile := range group.Tiles {
			tileInsert := `insert into puzzle_tiles (id, puzzle_id, group_id, text, sort_order)
			               values ($1, $2, $3, $4, $5)`
			if ignoreConflicts {
				tileInsert += " on conflict (id) do nothing"
			}
			if _, err := tx.ExecContext(ctx, tileInsert,
				tile.ID, puzzle.ID, group.ID, tile.Text, groupIndex*GroupSize+tileIndex); err != nil {
				return fmt.Errorf("insert tile: %w", err)
			}
		}
	}
	return nil
}

func (store *PostgresPuzzleStore) PersistDaily(ctx context.Context, puzzle Puzzle) error {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin persist daily tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	nextNumber, err := nextPuzzleNumberTx(ctx, tx)
	if err != nil {
		return err
	}
	puzzle.PuzzleNumber = nextNumber

	if err := insertPuzzleTx(ctx, tx, puzzle, true); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit persist daily: %w", err)
	}
	return nil
}

// ensure interface compliance at compile time.
var _ PuzzleSource = (*PostgresPuzzleStore)(nil)
