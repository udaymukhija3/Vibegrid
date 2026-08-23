package vibegrid

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

const (
	MaxVibeTitleRunes = 40
	maxVibeClientID   = 96
)

var (
	ErrVibeBoardExists        = errors.New("a vibe board already exists for that date")
	ErrVibeAlreadySubmitted   = errors.New("vibe already submitted")
	ErrVibeAlreadyVoted       = errors.New("vibe vote already submitted")
	ErrVibeSubmissionNotFound = errors.New("vibe submission not found")
	ErrVibeSelfVote           = errors.New("cannot vote for your own vibe")
	ErrVibeNotEligible        = errors.New("only submitting members can judge")
	ErrVibeRequestInvalid     = errors.New("vibe request is invalid")
	ErrVibeReplayConflict     = errors.New("vibe replay key was reused with different input")
)

type VibeSubmissionRequest struct {
	BoardID            string   `json:"boardId"`
	Title              string   `json:"title"`
	SelectedTileIDs    []string `json:"selectedTileIds"`
	ClientSubmissionID string   `json:"clientSubmissionId"`
}

type VibeVoteRequest struct {
	BoardID      string `json:"boardId"`
	SubmissionID string `json:"submissionId"`
	ClientVoteID string `json:"clientVoteId"`
}

type VibeRoundMember struct {
	MemberID    string
	SessionID   string
	DisplayName string
	JoinedAt    time.Time
}

type VibeSubmission struct {
	ID                 string
	CrewID             string
	BoardID            string
	MemberID           string
	DisplayName        string
	Title              string
	SelectedTileIDs    []string
	ClientSubmissionID string
	CreatedAt          time.Time
}

type VibeVote struct {
	ID            string
	CrewID        string
	BoardID       string
	VoterMemberID string
	SubmissionID  string
	ClientVoteID  string
	CreatedAt     time.Time
}

type VibeCrewSnapshot struct {
	Members     []VibeRoundMember
	Submissions []VibeSubmission
	Votes       []VibeVote
}

// VibeRoundStore owns the private, multi-member part of the product. Every
// mutation resolves membership inside its transaction; handlers never perform
// a check-then-write authorization sequence.
type VibeRoundStore interface {
	EnsureBoard(ctx context.Context, board VibeBoard) (VibeBoard, error)
	EnsureCrewBoard(ctx context.Context, crewID, sessionID string, board VibeBoard) (VibeBoard, error)
	CrewSnapshot(ctx context.Context, crewID string, boardIDs []string) (VibeCrewSnapshot, error)
	SubmitVibe(ctx context.Context, crewID, sessionID string, request VibeSubmissionRequest, now time.Time) (VibeSubmission, error)
	CastVibeVote(ctx context.Context, crewID, sessionID string, request VibeVoteRequest, now time.Time) (VibeVote, error)
	CrewStreak(ctx context.Context, crewID, throughDate string) (int, error)
}

// VibeBoardAdminStore is separate from VibeRoundStore so the public practice
// round can run from the built-in editorial bank without granting or requiring
// administrative board mutation capabilities.
type VibeBoardAdminStore interface {
	ListVibeBoards(ctx context.Context, limit int) ([]VibeBoard, error)
	CreateVibeBoard(ctx context.Context, board VibeBoard) (VibeBoard, error)
}

type PostgresVibeRoundStore struct {
	db *sql.DB
}

func NewPostgresVibeRoundStore(database *sql.DB) *PostgresVibeRoundStore {
	return &PostgresVibeRoundStore{db: database}
}

func (store *PostgresVibeRoundStore) ListVibeBoards(ctx context.Context, limit int) ([]VibeBoard, error) {
	if limit < 1 || limit > 366 {
		limit = 90
	}
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	rows, err := store.db.QueryContext(ctx,
		`select b.id, b.board_number, b.publish_date::text, b.prompt, b.tiles,
		        coalesce(e.tiles, '[]'::jsonb)
		 from vibe_daily_boards b
		 left join vibe_board_expansions e on e.board_id = b.id
		 order by b.publish_date desc
		 limit $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list vibe boards: %w", err)
	}
	defer func() { _ = rows.Close() }()

	boards := make([]VibeBoard, 0)
	for rows.Next() {
		var board VibeBoard
		var baseTiles, expansionTiles []byte
		if err := rows.Scan(&board.ID, &board.BoardNumber, &board.PublishDate, &board.Prompt, &baseTiles, &expansionTiles); err != nil {
			return nil, fmt.Errorf("scan vibe board: %w", err)
		}
		if err := decodeStoredVibeBoard(&board, baseTiles, expansionTiles); err != nil {
			return nil, fmt.Errorf("stored vibe board: %w", err)
		}
		boards = append(boards, board)
	}
	return boards, rows.Err()
}

// jsonb params must be bound as text. A []byte goes to the driver as bytea,
// which Postgres then fails to parse as json (SQLSTATE 22P02) — the same trap
// that broke the idempotency header column. Marshal, then string().
func (store *PostgresVibeRoundStore) CreateVibeBoard(ctx context.Context, board VibeBoard) (VibeBoard, error) {
	if err := validateVibeBoard(board); err != nil {
		return VibeBoard{}, err
	}
	if len(board.Tiles) != VibeBoardMaxTileCount {
		return VibeBoard{}, ErrVibeBoardInvalid
	}
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	baseTiles, expansionTiles, err := marshalStoredVibeBoard(board)
	if err != nil {
		return VibeBoard{}, fmt.Errorf("marshal vibe board: %w", err)
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return VibeBoard{}, fmt.Errorf("begin create vibe board: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`insert into vibe_daily_boards (id, publish_date, board_number, prompt, tiles)
		 values ($1, $2, $3, $4, $5)`,
		board.ID, board.PublishDate, board.BoardNumber, board.Prompt, string(baseTiles),
	); err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			return VibeBoard{}, ErrVibeBoardExists
		}
		return VibeBoard{}, fmt.Errorf("create vibe board: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`insert into vibe_board_expansions (board_id, tiles) values ($1, $2)`,
		board.ID, string(expansionTiles),
	); err != nil {
		return VibeBoard{}, fmt.Errorf("create vibe board expansion: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return VibeBoard{}, fmt.Errorf("commit create vibe board: %w", err)
	}
	return board, nil
}

func (store *PostgresVibeRoundStore) EnsureBoard(ctx context.Context, board VibeBoard) (VibeBoard, error) {
	if err := validateVibeBoard(board); err != nil {
		return VibeBoard{}, err
	}
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	baseTiles, expansionTiles, err := marshalStoredVibeBoard(board)
	if err != nil {
		return VibeBoard{}, fmt.Errorf("marshal vibe board: %w", err)
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return VibeBoard{}, fmt.Errorf("begin persist vibe board: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx,
		`insert into vibe_daily_boards (id, publish_date, board_number, prompt, tiles)
		 values ($1, $2, $3, $4, $5)
		 on conflict (publish_date) do nothing`,
		board.ID, board.PublishDate, board.BoardNumber, board.Prompt, string(baseTiles),
	)
	if err != nil {
		return VibeBoard{}, fmt.Errorf("persist vibe board: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return VibeBoard{}, fmt.Errorf("inspect persisted vibe board: %w", err)
	}
	if inserted == 1 && len(expansionTiles) > 0 {
		if _, err := tx.ExecContext(ctx,
			`insert into vibe_board_expansions (board_id, tiles) values ($1, $2)`,
			board.ID, string(expansionTiles),
		); err != nil {
			return VibeBoard{}, fmt.Errorf("persist vibe board expansion: %w", err)
		}
	}

	var stored VibeBoard
	var storedBaseTiles, storedExpansionTiles []byte
	if err := tx.QueryRowContext(ctx,
		`select b.id, b.board_number, b.publish_date::text, b.prompt, b.tiles,
		        coalesce(e.tiles, '[]'::jsonb)
		 from vibe_daily_boards b
		 left join vibe_board_expansions e on e.board_id = b.id
		 where b.publish_date = $1`, board.PublishDate,
	).Scan(&stored.ID, &stored.BoardNumber, &stored.PublishDate, &stored.Prompt, &storedBaseTiles, &storedExpansionTiles); err != nil {
		return VibeBoard{}, fmt.Errorf("load vibe board: %w", err)
	}
	if err := decodeStoredVibeBoard(&stored, storedBaseTiles, storedExpansionTiles); err != nil {
		return VibeBoard{}, fmt.Errorf("stored vibe board: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return VibeBoard{}, fmt.Errorf("commit persist vibe board: %w", err)
	}
	return stored, nil
}

// EnsureCrewBoard freezes one crew's palette size for one dated board. The
// crew row lock is shared with joins and leaves, which gives membership changes
// and first-open sizing one deterministic order. The unique key makes two
// members opening at once converge on the same snapshot.
func (store *PostgresVibeRoundStore) EnsureCrewBoard(ctx context.Context, crewID, sessionID string, board VibeBoard) (VibeBoard, error) {
	if err := validateVibeBoard(board); err != nil {
		return VibeBoard{}, err
	}
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return VibeBoard{}, fmt.Errorf("begin crew board freeze: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var lockedCrewID string
	if err := tx.QueryRowContext(ctx,
		`select id from crews where id = $1 for update`, crewID,
	).Scan(&lockedCrewID); errors.Is(err, sql.ErrNoRows) {
		return VibeBoard{}, ErrCrewNotFound
	} else if err != nil {
		return VibeBoard{}, fmt.Errorf("lock crew for board freeze: %w", err)
	}

	var memberCount int
	if err := tx.QueryRowContext(ctx,
		`select count(*) from crew_members where crew_id = $1 and session_id = $2`,
		crewID, sessionID,
	).Scan(&memberCount); err != nil {
		return VibeBoard{}, fmt.Errorf("authorize crew board freeze: %w", err)
	}
	if memberCount != 1 {
		return VibeBoard{}, ErrNotCrewMember
	}
	if err := tx.QueryRowContext(ctx,
		`select count(*) from crew_members where crew_id = $1`, crewID,
	).Scan(&memberCount); err != nil {
		return VibeBoard{}, fmt.Errorf("count crew for board freeze: %w", err)
	}

	tileCount := vibeBoardRowsForMembers(memberCount) * VibeBoardColumns
	if tileCount > len(board.Tiles) {
		tileCount = len(board.Tiles) - len(board.Tiles)%VibeBoardColumns
	}
	if _, err := tx.ExecContext(ctx,
		`insert into vibe_crew_boards (crew_id, board_id, member_count_snapshot, tile_count)
		 values ($1, $2, $3, $4)
		 on conflict (crew_id, board_id) do nothing`,
		crewID, board.ID, memberCount, tileCount,
	); err != nil {
		return VibeBoard{}, fmt.Errorf("freeze crew board: %w", err)
	}

	if err := tx.QueryRowContext(ctx,
		`select tile_count from vibe_crew_boards where crew_id = $1 and board_id = $2`,
		crewID, board.ID,
	).Scan(&tileCount); err != nil {
		return VibeBoard{}, fmt.Errorf("load frozen crew board: %w", err)
	}
	if !validVibeBoardTileCount(tileCount) || tileCount > len(board.Tiles) {
		return VibeBoard{}, fmt.Errorf("frozen crew board: %w", ErrVibeBoardInvalid)
	}
	if err := tx.Commit(); err != nil {
		return VibeBoard{}, fmt.Errorf("commit crew board freeze: %w", err)
	}
	return projectVibeBoard(board, tileCount), nil
}

func (store *PostgresVibeRoundStore) CrewSnapshot(ctx context.Context, crewID string, boardIDs []string) (VibeCrewSnapshot, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	snapshot := VibeCrewSnapshot{Members: []VibeRoundMember{}, Submissions: []VibeSubmission{}, Votes: []VibeVote{}}
	members, err := store.db.QueryContext(ctx,
		`select member_id, session_id, display_name, joined_at
		 from crew_members where crew_id = $1 order by joined_at, member_id`, crewID)
	if err != nil {
		return VibeCrewSnapshot{}, fmt.Errorf("load vibe crew members: %w", err)
	}
	for members.Next() {
		var member VibeRoundMember
		if err := members.Scan(&member.MemberID, &member.SessionID, &member.DisplayName, &member.JoinedAt); err != nil {
			_ = members.Close()
			return VibeCrewSnapshot{}, fmt.Errorf("scan vibe crew member: %w", err)
		}
		snapshot.Members = append(snapshot.Members, member)
	}
	if err := members.Close(); err != nil {
		return VibeCrewSnapshot{}, fmt.Errorf("close vibe crew members: %w", err)
	}
	if len(boardIDs) == 0 {
		return snapshot, nil
	}

	submissions, err := store.db.QueryContext(ctx,
		`select id, crew_id, board_id, submitted_by_member, display_name, title,
		        selected_tile_ids, client_submission_id, created_at
		 from vibe_submissions
		 where crew_id = $1 and board_id = any($2)
		 order by created_at, id`, crewID, pq.Array(boardIDs))
	if err != nil {
		return VibeCrewSnapshot{}, fmt.Errorf("load vibe submissions: %w", err)
	}
	for submissions.Next() {
		var submission VibeSubmission
		if err := scanVibeSubmission(submissions, &submission); err != nil {
			_ = submissions.Close()
			return VibeCrewSnapshot{}, err
		}
		snapshot.Submissions = append(snapshot.Submissions, submission)
	}
	if err := submissions.Close(); err != nil {
		return VibeCrewSnapshot{}, fmt.Errorf("close vibe submissions: %w", err)
	}

	votes, err := store.db.QueryContext(ctx,
		`select id, crew_id, board_id, voter_member_id, submission_id, client_vote_id, created_at
		 from vibe_votes
		 where crew_id = $1 and board_id = any($2)
		 order by created_at, id`, crewID, pq.Array(boardIDs))
	if err != nil {
		return VibeCrewSnapshot{}, fmt.Errorf("load vibe votes: %w", err)
	}
	defer func() { _ = votes.Close() }()
	for votes.Next() {
		var vote VibeVote
		if err := votes.Scan(&vote.ID, &vote.CrewID, &vote.BoardID, &vote.VoterMemberID, &vote.SubmissionID, &vote.ClientVoteID, &vote.CreatedAt); err != nil {
			return VibeCrewSnapshot{}, fmt.Errorf("scan vibe vote: %w", err)
		}
		snapshot.Votes = append(snapshot.Votes, vote)
	}
	return snapshot, votes.Err()
}

func (store *PostgresVibeRoundStore) SubmitVibe(ctx context.Context, crewID, sessionID string, request VibeSubmissionRequest, now time.Time) (VibeSubmission, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return VibeSubmission{}, fmt.Errorf("begin vibe submission: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var memberID, displayName string
	if err := tx.QueryRowContext(ctx,
		`select member_id, display_name from crew_members
		 where crew_id = $1 and session_id = $2 for share`, crewID, sessionID,
	).Scan(&memberID, &displayName); errors.Is(err, sql.ErrNoRows) {
		return VibeSubmission{}, ErrNotCrewMember
	} else if err != nil {
		return VibeSubmission{}, fmt.Errorf("authorize vibe submission: %w", err)
	}

	if replayed, ok, err := loadSubmissionByReplayKey(ctx, tx, crewID, memberID, request.ClientSubmissionID); err != nil {
		return VibeSubmission{}, err
	} else if ok {
		if !submissionMatchesRequest(replayed, request) {
			return VibeSubmission{}, ErrVibeReplayConflict
		}
		return replayed, nil
	}

	var exists bool
	if err := tx.QueryRowContext(ctx,
		`select exists (
		   select 1 from vibe_submissions
		   where crew_id = $1 and board_id = $2 and submitted_by_member = $3
		 )`, crewID, request.BoardID, memberID,
	).Scan(&exists); err != nil {
		return VibeSubmission{}, fmt.Errorf("check existing vibe submission: %w", err)
	}
	if exists {
		// The original request may have committed between the replay lookup and
		// this statement. Re-read the key before reporting a distinct-action
		// conflict so an overlapping network retry still receives the winner.
		if replayed, ok, err := loadSubmissionByReplayKey(ctx, tx, crewID, memberID, request.ClientSubmissionID); err != nil {
			return VibeSubmission{}, err
		} else if ok {
			if !submissionMatchesRequest(replayed, request) {
				return VibeSubmission{}, ErrVibeReplayConflict
			}
			return replayed, nil
		}
		return VibeSubmission{}, ErrVibeAlreadySubmitted
	}

	if valid, err := frozenCrewBoardContainsSelection(ctx, tx, crewID, request.BoardID, request.SelectedTileIDs); err != nil {
		return VibeSubmission{}, err
	} else if !valid {
		return VibeSubmission{}, ErrVibeRequestInvalid
	}

	submission := VibeSubmission{
		ID:                 newVibeObjectID("card"),
		CrewID:             crewID,
		BoardID:            request.BoardID,
		MemberID:           memberID,
		DisplayName:        displayName,
		Title:              request.Title,
		SelectedTileIDs:    append([]string(nil), request.SelectedTileIDs...),
		ClientSubmissionID: request.ClientSubmissionID,
		CreatedAt:          now.UTC(),
	}
	if _, err := tx.ExecContext(ctx,
		`insert into vibe_submissions (
		   id, crew_id, board_id, submitted_by_member, display_name, title,
		   selected_tile_ids, client_submission_id, created_at
		 ) values ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		submission.ID, crewID, request.BoardID, memberID, displayName, request.Title,
		pq.Array(request.SelectedTileIDs), request.ClientSubmissionID, submission.CreatedAt,
	); err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) {
			switch postgresError.ConstraintName {
			case "vibe_submissions_one_card", "vibe_submissions_replay_key":
				// The insert may have waited for an identical request in another
				// transaction to commit. PostgreSQL aborts this transaction after
				// the uniqueness error, so release it and read the winner on a fresh
				// connection before deciding whether this is a replay or a conflict.
				if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
					return VibeSubmission{}, fmt.Errorf("rollback raced vibe submission: %w", rollbackErr)
				}
				replayed, ok, replayErr := loadSubmissionByReplayKey(ctx, store.db, crewID, memberID, request.ClientSubmissionID)
				if replayErr != nil {
					return VibeSubmission{}, replayErr
				}
				if ok {
					if !submissionMatchesRequest(replayed, request) {
						return VibeSubmission{}, ErrVibeReplayConflict
					}
					return replayed, nil
				}
				if postgresError.ConstraintName == "vibe_submissions_one_card" {
					return VibeSubmission{}, ErrVibeAlreadySubmitted
				}
				return VibeSubmission{}, ErrVibeReplayConflict
			}
		}
		return VibeSubmission{}, fmt.Errorf("insert vibe submission: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return VibeSubmission{}, fmt.Errorf("commit vibe submission: %w", err)
	}
	return submission, nil
}

func (store *PostgresVibeRoundStore) CastVibeVote(ctx context.Context, crewID, sessionID string, request VibeVoteRequest, now time.Time) (VibeVote, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return VibeVote{}, fmt.Errorf("begin vibe vote: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var voterMemberID string
	if err := tx.QueryRowContext(ctx,
		`select member_id from crew_members
		 where crew_id = $1 and session_id = $2 for share`, crewID, sessionID,
	).Scan(&voterMemberID); errors.Is(err, sql.ErrNoRows) {
		return VibeVote{}, ErrNotCrewMember
	} else if err != nil {
		return VibeVote{}, fmt.Errorf("authorize vibe vote: %w", err)
	}

	if replayed, ok, err := loadVoteByReplayKey(ctx, tx, crewID, voterMemberID, request.ClientVoteID); err != nil {
		return VibeVote{}, err
	} else if ok {
		if !voteMatchesRequest(replayed, request) {
			return VibeVote{}, ErrVibeReplayConflict
		}
		return replayed, nil
	}

	var ownSubmissionID string
	if err := tx.QueryRowContext(ctx,
		`select id from vibe_submissions
		 where crew_id = $1 and board_id = $2 and submitted_by_member = $3`,
		crewID, request.BoardID, voterMemberID,
	).Scan(&ownSubmissionID); errors.Is(err, sql.ErrNoRows) {
		return VibeVote{}, ErrVibeNotEligible
	} else if err != nil {
		return VibeVote{}, fmt.Errorf("check vibe voter eligibility: %w", err)
	}

	var targetMemberID string
	if err := tx.QueryRowContext(ctx,
		`select submitted_by_member from vibe_submissions
		 where id = $1 and crew_id = $2 and board_id = $3`,
		request.SubmissionID, crewID, request.BoardID,
	).Scan(&targetMemberID); errors.Is(err, sql.ErrNoRows) {
		return VibeVote{}, ErrVibeSubmissionNotFound
	} else if err != nil {
		return VibeVote{}, fmt.Errorf("load vibe vote target: %w", err)
	}
	if targetMemberID == voterMemberID || request.SubmissionID == ownSubmissionID {
		return VibeVote{}, ErrVibeSelfVote
	}

	var alreadyVoted bool
	if err := tx.QueryRowContext(ctx,
		`select exists (
		   select 1 from vibe_votes
		   where crew_id = $1 and board_id = $2 and voter_member_id = $3
		 )`, crewID, request.BoardID, voterMemberID,
	).Scan(&alreadyVoted); err != nil {
		return VibeVote{}, fmt.Errorf("check existing vibe vote: %w", err)
	}
	if alreadyVoted {
		if replayed, ok, err := loadVoteByReplayKey(ctx, tx, crewID, voterMemberID, request.ClientVoteID); err != nil {
			return VibeVote{}, err
		} else if ok {
			if !voteMatchesRequest(replayed, request) {
				return VibeVote{}, ErrVibeReplayConflict
			}
			return replayed, nil
		}
		return VibeVote{}, ErrVibeAlreadyVoted
	}

	vote := VibeVote{
		ID:            newVibeObjectID("vote"),
		CrewID:        crewID,
		BoardID:       request.BoardID,
		VoterMemberID: voterMemberID,
		SubmissionID:  request.SubmissionID,
		ClientVoteID:  request.ClientVoteID,
		CreatedAt:     now.UTC(),
	}
	if _, err := tx.ExecContext(ctx,
		`insert into vibe_votes (
		   id, crew_id, board_id, voter_member_id, submission_id, client_vote_id, created_at
		 ) values ($1, $2, $3, $4, $5, $6, $7)`,
		vote.ID, crewID, request.BoardID, voterMemberID, request.SubmissionID, request.ClientVoteID, vote.CreatedAt,
	); err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) {
			switch postgresError.ConstraintName {
			case "vibe_votes_one_ballot", "vibe_votes_replay_key":
				if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
					return VibeVote{}, fmt.Errorf("rollback raced vibe vote: %w", rollbackErr)
				}
				replayed, ok, replayErr := loadVoteByReplayKey(ctx, store.db, crewID, voterMemberID, request.ClientVoteID)
				if replayErr != nil {
					return VibeVote{}, replayErr
				}
				if ok {
					if !voteMatchesRequest(replayed, request) {
						return VibeVote{}, ErrVibeReplayConflict
					}
					return replayed, nil
				}
				if postgresError.ConstraintName == "vibe_votes_one_ballot" {
					return VibeVote{}, ErrVibeAlreadyVoted
				}
				return VibeVote{}, ErrVibeReplayConflict
			}
		}
		return VibeVote{}, fmt.Errorf("insert vibe vote: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return VibeVote{}, fmt.Errorf("commit vibe vote: %w", err)
	}
	return vote, nil
}

func (store *PostgresVibeRoundStore) CrewStreak(ctx context.Context, crewID, throughDate string) (int, error) {
	through, err := time.Parse("2006-01-02", throughDate)
	if err != nil {
		return 0, ErrVibeRequestInvalid
	}
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	rows, err := store.db.QueryContext(ctx,
		`select b.publish_date::text
		 from vibe_daily_boards b
		 join vibe_submissions s on s.board_id = b.id and s.crew_id = $1
		 left join vibe_votes v on v.board_id = b.id and v.crew_id = $1
		 where b.publish_date <= $2
		 group by b.publish_date
		 having count(distinct s.id) >= 3 and count(distinct v.voter_member_id) >= 2
		 order by b.publish_date desc
		 limit 366`, crewID, throughDate)
	if err != nil {
		return 0, fmt.Errorf("load vibe crew streak: %w", err)
	}
	defer func() { _ = rows.Close() }()

	want := through
	streak := 0
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return 0, fmt.Errorf("scan vibe crew streak: %w", err)
		}
		date, err := time.Parse("2006-01-02", raw)
		if err != nil {
			return 0, fmt.Errorf("parse vibe crew streak: %w", err)
		}
		if date.Equal(want) {
			streak++
			want = want.AddDate(0, 0, -1)
			continue
		}
		if date.Before(want) {
			break
		}
	}
	return streak, rows.Err()
}

type submissionScanner interface {
	Scan(dest ...any) error
}

type vibeQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func frozenCrewBoardContainsSelection(ctx context.Context, queryer vibeQueryer, crewID, boardID string, selected []string) (bool, error) {
	var rawBaseTiles, rawExpansionTiles []byte
	var tileCount int
	if err := queryer.QueryRowContext(ctx,
		`select b.tiles, coalesce(e.tiles, '[]'::jsonb), cb.tile_count
		 from vibe_crew_boards cb
		 join vibe_daily_boards b on b.id = cb.board_id
		 left join vibe_board_expansions e on e.board_id = b.id
		 where cb.crew_id = $1 and cb.board_id = $2`,
		crewID, boardID,
	).Scan(&rawBaseTiles, &rawExpansionTiles, &tileCount); errors.Is(err, sql.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("load frozen crew board selection: %w", err)
	}
	var baseTiles, expansionTiles []Tile
	if err := json.Unmarshal(rawBaseTiles, &baseTiles); err != nil {
		return false, fmt.Errorf("decode frozen crew board selection: %w", err)
	}
	if err := json.Unmarshal(rawExpansionTiles, &expansionTiles); err != nil {
		return false, fmt.Errorf("decode frozen crew board expansion selection: %w", err)
	}
	tiles := append(baseTiles, expansionTiles...)
	if !validVibeBoardTileCount(tileCount) || tileCount > len(tiles) {
		return false, fmt.Errorf("frozen crew board selection: %w", ErrVibeBoardInvalid)
	}
	return validateVibeTileIDs(tiles[:tileCount], selected), nil
}

func marshalStoredVibeBoard(board VibeBoard) ([]byte, []byte, error) {
	if len(board.Tiles) != VibeBoardMinTileCount && len(board.Tiles) != VibeBoardMaxTileCount {
		return nil, nil, ErrVibeBoardInvalid
	}
	baseTiles, err := json.Marshal(board.Tiles[:VibeBoardMinTileCount])
	if err != nil {
		return nil, nil, err
	}
	if len(board.Tiles) == VibeBoardMinTileCount {
		return baseTiles, nil, nil
	}
	expansionTiles, err := json.Marshal(board.Tiles[VibeBoardMinTileCount:])
	if err != nil {
		return nil, nil, err
	}
	return baseTiles, expansionTiles, nil
}

func decodeStoredVibeBoard(board *VibeBoard, rawBaseTiles, rawExpansionTiles []byte) error {
	var baseTiles, expansionTiles []Tile
	if err := json.Unmarshal(rawBaseTiles, &baseTiles); err != nil {
		return fmt.Errorf("decode base tiles: %w", err)
	}
	if err := json.Unmarshal(rawExpansionTiles, &expansionTiles); err != nil {
		return fmt.Errorf("decode expansion tiles: %w", err)
	}
	if len(baseTiles) != VibeBoardMinTileCount || (len(expansionTiles) != 0 && len(expansionTiles) != VibeBoardMaxTileCount-VibeBoardMinTileCount) {
		return ErrVibeBoardInvalid
	}
	board.Tiles = append(baseTiles, expansionTiles...)
	return validateVibeBoard(*board)
}

func scanVibeSubmission(scanner submissionScanner, submission *VibeSubmission) error {
	if err := scanner.Scan(
		&submission.ID, &submission.CrewID, &submission.BoardID, &submission.MemberID,
		&submission.DisplayName, &submission.Title, pq.Array(&submission.SelectedTileIDs),
		&submission.ClientSubmissionID, &submission.CreatedAt,
	); err != nil {
		return fmt.Errorf("scan vibe submission: %w", err)
	}
	return nil
}

func loadSubmissionByReplayKey(ctx context.Context, queryer vibeQueryer, crewID, memberID, clientID string) (VibeSubmission, bool, error) {
	var submission VibeSubmission
	err := scanVibeSubmission(queryer.QueryRowContext(ctx,
		`select id, crew_id, board_id, submitted_by_member, display_name, title,
		        selected_tile_ids, client_submission_id, created_at
		 from vibe_submissions
		 where crew_id = $1 and submitted_by_member = $2 and client_submission_id = $3`,
		crewID, memberID, clientID,
	), &submission)
	if errors.Is(err, sql.ErrNoRows) || errors.Is(errors.Unwrap(err), sql.ErrNoRows) {
		return VibeSubmission{}, false, nil
	}
	if err != nil {
		return VibeSubmission{}, false, fmt.Errorf("load replayed vibe submission: %w", err)
	}
	return submission, true, nil
}

func loadVoteByReplayKey(ctx context.Context, queryer vibeQueryer, crewID, memberID, clientID string) (VibeVote, bool, error) {
	var vote VibeVote
	err := queryer.QueryRowContext(ctx,
		`select id, crew_id, board_id, voter_member_id, submission_id, client_vote_id, created_at
		 from vibe_votes
		 where crew_id = $1 and voter_member_id = $2 and client_vote_id = $3`,
		crewID, memberID, clientID,
	).Scan(&vote.ID, &vote.CrewID, &vote.BoardID, &vote.VoterMemberID, &vote.SubmissionID, &vote.ClientVoteID, &vote.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return VibeVote{}, false, nil
	}
	if err != nil {
		return VibeVote{}, false, fmt.Errorf("load replayed vibe vote: %w", err)
	}
	return vote, true, nil
}

func submissionMatchesRequest(submission VibeSubmission, request VibeSubmissionRequest) bool {
	return submission.BoardID == request.BoardID && submission.Title == request.Title &&
		slices.Equal(submission.SelectedTileIDs, request.SelectedTileIDs)
}

func voteMatchesRequest(vote VibeVote, request VibeVoteRequest) bool {
	return vote.BoardID == request.BoardID && vote.SubmissionID == request.SubmissionID
}

func normalizeVibeTitle(raw string) (string, error) {
	// Fields collapses newlines and repeated whitespace into a share-safe title.
	// Other control characters are rejected before the collapse.
	for _, r := range raw {
		if unicode.IsControl(r) && !unicode.IsSpace(r) {
			return "", ErrVibeRequestInvalid
		}
	}
	title := strings.Join(strings.Fields(raw), " ")
	if title == "" || utf8.RuneCountInString(title) > MaxVibeTitleRunes {
		return "", ErrVibeRequestInvalid
	}
	return title, nil
}

func validateVibeClientID(value string) bool {
	if value == "" || len(value) > maxVibeClientID {
		return false
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

func newVibeObjectID(prefix string) string {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		panic("crypto/rand failed while generating a vibe object id: " + err.Error())
	}
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(buffer)
}
