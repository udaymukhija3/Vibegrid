package vibegrid

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrPublishDateTaken = errors.New("a puzzle is already published for that date")

// PostgresPuzzleStore reads and writes puzzle content in Postgres. It satisfies
// PuzzleSource for the public read path and adds admin authoring operations.
type PostgresPuzzleStore struct {
	db *sql.DB
}

func NewPostgresPuzzleStore(database *sql.DB) *PostgresPuzzleStore {
	return &PostgresPuzzleStore{db: database}
}

// Puzzles loads every puzzle with its groups and tiles in three queries,
// assembled in memory. At daily-puzzle scale the full set is tiny; a cache can
// be layered on later if the archive grows large.
func (store *PostgresPuzzleStore) Puzzles(ctx context.Context) ([]Puzzle, error) {
	puzzles, order, err := store.loadPuzzles(ctx)
	if err != nil {
		return nil, err
	}
	if err := store.attachGroups(ctx, puzzles); err != nil {
		return nil, err
	}
	if err := store.attachTiles(ctx, puzzles); err != nil {
		return nil, err
	}

	result := make([]Puzzle, 0, len(order))
	for _, id := range order {
		result = append(result, *puzzles[id])
	}
	return result, nil
}

func (store *PostgresPuzzleStore) loadPuzzles(ctx context.Context) (map[string]*Puzzle, []string, error) {
	rows, err := store.db.QueryContext(ctx,
		`select id, puzzle_number, publish_date, status, difficulty, origin
		 from puzzles order by puzzle_number`)
	if err != nil {
		return nil, nil, fmt.Errorf("query puzzles: %w", err)
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
			return nil, nil, fmt.Errorf("scan puzzle: %w", err)
		}
		if publishDate.Valid {
			puzzle.PublishDate = publishDate.Time.Format("2006-01-02")
		}
		puzzle.Groups = []PuzzleGroup{}
		stored := puzzle
		puzzles[puzzle.ID] = &stored
		order = append(order, puzzle.ID)
	}
	return puzzles, order, rows.Err()
}

func (store *PostgresPuzzleStore) attachGroups(ctx context.Context, puzzles map[string]*Puzzle) error {
	rows, err := store.db.QueryContext(ctx,
		`select id, puzzle_id, name, explanation, color_index
		 from puzzle_groups order by puzzle_id, sort_order`)
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

func (store *PostgresPuzzleStore) attachTiles(ctx context.Context, puzzles map[string]*Puzzle) error {
	rows, err := store.db.QueryContext(ctx,
		`select id, puzzle_id, group_id, text
		 from puzzle_tiles order by puzzle_id, sort_order`)
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
	return tx.Commit()
}

// CreateDraft persists a new DRAFT puzzle. The caller is expected to have
// validated input already; this assigns ids, the next puzzle number, and order.
func (store *PostgresPuzzleStore) CreateDraft(ctx context.Context, input AdminPuzzleInput) (Puzzle, error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return Puzzle{}, fmt.Errorf("begin create tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var nextNumber int
	if err := tx.QueryRowContext(ctx,
		`select coalesce(max(puzzle_number), 0) + 1 from puzzles`).Scan(&nextNumber); err != nil {
		return Puzzle{}, fmt.Errorf("next puzzle number: %w", err)
	}

	puzzle := input.toPuzzle(nextNumber)
	if err := insertPuzzleTx(ctx, tx, puzzle, false); err != nil {
		return Puzzle{}, err
	}
	if err := tx.Commit(); err != nil {
		return Puzzle{}, fmt.Errorf("commit create: %w", err)
	}
	return puzzle, nil
}

// CreateCommunityPuzzle persists a user-created puzzle. Unlike an editorial
// draft it is immediately PUBLISHED (so the share link works right away), has no
// publish_date, and is tagged COMMUNITY so it stays out of the daily rotation.
func (store *PostgresPuzzleStore) CreateCommunityPuzzle(ctx context.Context, input AdminPuzzleInput) (Puzzle, error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return Puzzle{}, fmt.Errorf("begin community create tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var nextNumber int
	if err := tx.QueryRowContext(ctx,
		`select coalesce(max(puzzle_number), 0) + 1 from puzzles`).Scan(&nextNumber); err != nil {
		return Puzzle{}, fmt.Errorf("next puzzle number: %w", err)
	}

	puzzle := input.toPuzzle(nextNumber)
	puzzle.Status = PuzzleStatusPublished
	puzzle.Origin = OriginCommunity
	if err := insertPuzzleTx(ctx, tx, puzzle, false); err != nil {
		return Puzzle{}, err
	}
	if err := tx.Commit(); err != nil {
		return Puzzle{}, fmt.Errorf("commit community create: %w", err)
	}
	return puzzle, nil
}

// Publish sets a draft live for a date. The unique constraint on publish_date
// turns a same-date collision into ErrPublishDateTaken rather than a duplicate.
func (store *PostgresPuzzleStore) Publish(ctx context.Context, puzzleID, publishDate string) error {
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

// ensure interface compliance at compile time.
var _ PuzzleSource = (*PostgresPuzzleStore)(nil)
