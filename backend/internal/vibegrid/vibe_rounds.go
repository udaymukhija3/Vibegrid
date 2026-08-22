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
		`select id, board_number, publish_date::text, prompt, tiles
		 from vibe_daily_boards
		 order by publish_date desc
		 limit $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list vibe boards: %w", err)
	}
	defer func() { _ = rows.Close() }()

	boards := make([]VibeBoard, 0)
	for rows.Next() {
		var board VibeBoard
		var tiles []byte
		if err := rows.Scan(&board.ID, &board.BoardNumber, &board.PublishDate, &board.Prompt, &tiles); err != nil {
			return nil, fmt.Errorf("scan vibe board: %w", err)
		}
		if err := json.Unmarshal(tiles, &board.Tiles); err != nil {
			return nil, fmt.Errorf("decode vibe board: %w", err)
		}
		if err := validateVibeBoard(board); err != nil {
			return nil, fmt.Errorf("stored vibe board: %w", err)
		}
		boards = append(boards, board)
	}
	return boards, rows.Err()
}

func (store *PostgresVibeRoundStore) CreateVibeBoard(ctx context.Context, board VibeBoard) (VibeBoard, error) {
	if err := validateVibeBoard(board); err != nil {
		return VibeBoard{}, err
	}
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	tiles, err := json.Marshal(board.Tiles)
	if err != nil {
		return VibeBoard{}, fmt.Errorf("marshal vibe board: %w", err)
	}
	if _, err := store.db.ExecContext(ctx,
		`insert into vibe_daily_boards (id, publish_date, board_number, prompt, tiles)
		 values ($1, $2, $3, $4, $5)`,
		board.ID, board.PublishDate, board.BoardNumber, board.Prompt, tiles,
	); err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			return VibeBoard{}, ErrVibeBoardExists
		}
		return VibeBoard{}, fmt.Errorf("create vibe board: %w", err)
	}
	return board, nil
}

func (store *PostgresVibeRoundStore) EnsureBoard(ctx context.Context, board VibeBoard) (VibeBoard, error) {
	if err := validateVibeBoard(board); err != nil {
		return VibeBoard{}, err
	}
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	tiles, err := json.Marshal(board.Tiles)
	if err != nil {
		return VibeBoard{}, fmt.Errorf("marshal vibe board: %w", err)
	}
	if _, err := store.db.ExecContext(ctx,
		`insert into vibe_daily_boards (id, publish_date, board_number, prompt, tiles)
		 values ($1, $2, $3, $4, $5)
		 on conflict (publish_date) do nothing`,
		board.ID, board.PublishDate, board.BoardNumber, board.Prompt, tiles,
	); err != nil {
		return VibeBoard{}, fmt.Errorf("persist vibe board: %w", err)
	}

	var stored VibeBoard
	var storedTiles []byte
	if err := store.db.QueryRowContext(ctx,
		`select id, board_number, publish_date::text, prompt, tiles
		 from vibe_daily_boards where publish_date = $1`, board.PublishDate,
	).Scan(&stored.ID, &stored.BoardNumber, &stored.PublishDate, &stored.Prompt, &storedTiles); err != nil {
		return VibeBoard{}, fmt.Errorf("load vibe board: %w", err)
	}
	if err := json.Unmarshal(storedTiles, &stored.Tiles); err != nil {
		return VibeBoard{}, fmt.Errorf("decode vibe board: %w", err)
	}
	if err := validateVibeBoard(stored); err != nil {
		return VibeBoard{}, fmt.Errorf("stored vibe board: %w", err)
	}
	return stored, nil
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
		if replayed.BoardID != request.BoardID || replayed.Title != request.Title || !slices.Equal(replayed.SelectedTileIDs, request.SelectedTileIDs) {
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
		return VibeSubmission{}, ErrVibeAlreadySubmitted
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
			case "vibe_submissions_one_card":
				return VibeSubmission{}, ErrVibeAlreadySubmitted
			case "vibe_submissions_replay_key":
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

	var replayed VibeVote
	err = tx.QueryRowContext(ctx,
		`select id, crew_id, board_id, voter_member_id, submission_id, client_vote_id, created_at
		 from vibe_votes
		 where crew_id = $1 and voter_member_id = $2 and client_vote_id = $3`,
		crewID, voterMemberID, request.ClientVoteID,
	).Scan(&replayed.ID, &replayed.CrewID, &replayed.BoardID, &replayed.VoterMemberID, &replayed.SubmissionID, &replayed.ClientVoteID, &replayed.CreatedAt)
	if err == nil {
		if replayed.BoardID != request.BoardID || replayed.SubmissionID != request.SubmissionID {
			return VibeVote{}, ErrVibeReplayConflict
		}
		return replayed, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return VibeVote{}, fmt.Errorf("load replayed vibe vote: %w", err)
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
			case "vibe_votes_one_ballot":
				return VibeVote{}, ErrVibeAlreadyVoted
			case "vibe_votes_replay_key":
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

func loadSubmissionByReplayKey(ctx context.Context, tx *sql.Tx, crewID, memberID, clientID string) (VibeSubmission, bool, error) {
	var submission VibeSubmission
	err := scanVibeSubmission(tx.QueryRowContext(ctx,
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
