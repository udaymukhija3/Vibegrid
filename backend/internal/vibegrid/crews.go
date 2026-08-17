package vibegrid

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/lib/pq"
)

var (
	ErrCrewNotFound      = errors.New("crew not found")
	ErrCrewFull          = errors.New("crew is full")
	ErrCrewNameInvalid   = errors.New("crew name is invalid")
	ErrDisplayNameTaken  = errors.New("display name is already used in this crew")
	ErrNotCrewOwner      = errors.New("only the crew owner can do that")
	ErrNotCrewMember     = errors.New("not a member of this crew")
	ErrCrewMemberUnknown = errors.New("crew member not found")
)

const (
	// maxCrewMembers keeps a leaked invite link from turning a friend group into
	// an unbounded public room. It is a product limit, not a technical one.
	maxCrewMembers = 20

	maxCrewNameLength    = 40
	maxDisplayNameLength = 24

	// crewIDBytes is the entropy behind the invite link. The id is the only
	// secret protecting a crew, so it is sized like a token, not like a slug.
	crewIDBytes = 12
)

// Crew is a private group of friends playing the same daily grid.
//
// ID is the stable internal key. InviteCode is what appears in the shared URL
// and is what actually grants access, so it can be rotated when a link leaks
// without destroying the crew or its history.
type Crew struct {
	ID         string
	InviteCode string
	Name       string
	CreatedAt  time.Time
	OwnerID    string
}

// CrewMember is one browser session's identity inside a single crew.
type CrewMember struct {
	MemberID    string
	SessionID   string
	DisplayName string
	JoinedAt    time.Time
}

// CrewMemberProgress is one member's state on a given puzzle. It deliberately
// carries no tile-level detail: the guess grid is loaded separately and only
// once the viewer has finished, so an unfinished player cannot read answers off
// a friend's board (see CrewGuessHistory).
type CrewMemberProgress struct {
	MemberID    string
	SessionID   string
	DisplayName string
	Started     bool
	SolvedCount int
	Mistakes    int
	Completed   bool
	Failed      bool
	StartedAt   *time.Time
	CompletedAt *time.Time
}

// CrewStore owns crew membership. Crews are inherently multi-session and
// durable, so unlike attempts there is no in-memory implementation: without a
// database the endpoints report the feature as unavailable rather than pretend
// to work for one process.
type CrewStore interface {
	CreateCrew(ctx context.Context, name, displayName, sessionID string, now time.Time) (Crew, error)
	JoinCrew(ctx context.Context, inviteCode, displayName, sessionID string, now time.Time) (Crew, error)
	CrewByInviteCode(ctx context.Context, inviteCode string) (Crew, error)
	CrewsForSession(ctx context.Context, sessionID string) ([]Crew, error)
	CrewProgress(ctx context.Context, crewID, puzzleID string) ([]CrewMemberProgress, error)
	// CrewGuessHistory returns each member's ordered guesses for a puzzle. Only
	// call it once the viewer's own attempt is finished — it is the spoiler.
	CrewGuessHistory(ctx context.Context, crewID, puzzleID string) (map[string][][]string, error)

	// RotateInvite issues a fresh invite code, killing every link already shared.
	// Owner only.
	RotateInvite(ctx context.Context, crewID, sessionID string) (Crew, error)
	// RemoveMember evicts one membership by its opaque id. Owner only, and the
	// owner cannot remove themselves this way — that is what LeaveCrew is for.
	RemoveMember(ctx context.Context, crewID, memberID, sessionID string) error
	// LeaveCrew removes the caller. If the owner leaves, ownership passes to the
	// longest-standing remaining member; the last member out deletes the crew.
	LeaveCrew(ctx context.Context, crewID, sessionID string) error
}

// normalizeCrewName trims and validates a crew or display name. Names are shown
// to other people, so control characters are rejected outright rather than
// escaped downstream.
func normalizeCrewName(raw string, limit int) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", ErrCrewNameInvalid
	}
	if utf8.RuneCountInString(name) > limit {
		return "", ErrCrewNameInvalid
	}
	for _, r := range name {
		if r == '\n' || r == '\r' || r == '\t' {
			return "", ErrCrewNameInvalid
		}
		if unicode.IsControl(r) {
			return "", ErrCrewNameInvalid
		}
	}
	return name, nil
}

func newCrewID() string {
	buffer := make([]byte, crewIDBytes)
	if _, err := rand.Read(buffer); err != nil {
		panic("crypto/rand failed while generating a crew id: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(buffer)
}

// validCrewID gates the path parameter before it reaches the database, matching
// how puzzle ids are screened.
func validCrewID(value string) bool {
	if len(value) == 0 || len(value) > 64 {
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

// PostgresCrewStore is the durable crew store.
type PostgresCrewStore struct {
	db *sql.DB
}

func NewPostgresCrewStore(database *sql.DB) *PostgresCrewStore {
	return &PostgresCrewStore{db: database}
}

func (store *PostgresCrewStore) CreateCrew(ctx context.Context, name, displayName, sessionID string, now time.Time) (Crew, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	crewName, err := normalizeCrewName(name, maxCrewNameLength)
	if err != nil {
		return Crew{}, err
	}
	member, err := normalizeCrewName(displayName, maxDisplayNameLength)
	if err != nil {
		return Crew{}, err
	}

	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return Crew{}, fmt.Errorf("begin crew create: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	crew := Crew{
		ID:         newCrewID(),
		InviteCode: newCrewID(),
		Name:       crewName,
		CreatedAt:  now.UTC(),
		OwnerID:    sessionID,
	}
	if _, err := tx.ExecContext(ctx,
		`insert into crews (id, invite_code, name, created_at, created_by_session)
		 values ($1, $2, $3, $4, $5)`,
		crew.ID, crew.InviteCode, crew.Name, crew.CreatedAt, sessionID,
	); err != nil {
		return Crew{}, fmt.Errorf("insert crew: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`insert into crew_members (crew_id, session_id, display_name, joined_at)
		 values ($1, $2, $3, $4)`,
		crew.ID, sessionID, member, now.UTC(),
	); err != nil {
		return Crew{}, fmt.Errorf("insert founding member: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Crew{}, fmt.Errorf("commit crew create: %w", err)
	}
	return crew, nil
}

// JoinCrew adds the session to the crew, or updates its display name if it is
// already a member, so a repeated link click is never an error. The member
// count is checked inside the transaction with the crew row locked, so two
// simultaneous joins cannot both slip past the cap.
func (store *PostgresCrewStore) JoinCrew(ctx context.Context, inviteCode, displayName, sessionID string, now time.Time) (Crew, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	member, err := normalizeCrewName(displayName, maxDisplayNameLength)
	if err != nil {
		return Crew{}, err
	}

	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return Crew{}, fmt.Errorf("begin crew join: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Joining is gated on the *current* invite code, so a rotated-away link
	// cannot be replayed to get back in.
	var crew Crew
	err = tx.QueryRowContext(ctx,
		`select id, invite_code, name, created_at, created_by_session
		 from crews where invite_code = $1 for update`,
		inviteCode,
	).Scan(&crew.ID, &crew.InviteCode, &crew.Name, &crew.CreatedAt, &crew.OwnerID)
	if errors.Is(err, sql.ErrNoRows) {
		return Crew{}, ErrCrewNotFound
	}
	if err != nil {
		return Crew{}, fmt.Errorf("lock crew: %w", err)
	}
	crewID := crew.ID

	var alreadyMember bool
	if err := tx.QueryRowContext(ctx,
		`select exists (select 1 from crew_members where crew_id = $1 and session_id = $2)`,
		crewID, sessionID,
	).Scan(&alreadyMember); err != nil {
		return Crew{}, fmt.Errorf("check membership: %w", err)
	}

	if !alreadyMember {
		var memberCount int
		if err := tx.QueryRowContext(ctx,
			`select count(*) from crew_members where crew_id = $1`, crewID,
		).Scan(&memberCount); err != nil {
			return Crew{}, fmt.Errorf("count crew members: %w", err)
		}
		if memberCount >= maxCrewMembers {
			return Crew{}, ErrCrewFull
		}
	}

	// Two people in the same crew answering to the same name makes the board
	// unreadable, so the name has to be free (unless it is already yours).
	var nameTaken bool
	if err := tx.QueryRowContext(ctx,
		`select exists (
		   select 1 from crew_members
		   where crew_id = $1 and lower(display_name) = lower($2) and session_id <> $3
		 )`,
		crewID, member, sessionID,
	).Scan(&nameTaken); err != nil {
		return Crew{}, fmt.Errorf("check display name: %w", err)
	}
	if nameTaken {
		return Crew{}, ErrDisplayNameTaken
	}

	if _, err := tx.ExecContext(ctx,
		`insert into crew_members (crew_id, session_id, display_name, joined_at)
		 values ($1, $2, $3, $4)
		 on conflict (crew_id, session_id) do update set display_name = excluded.display_name`,
		crewID, sessionID, member, now.UTC(),
	); err != nil {
		return Crew{}, fmt.Errorf("insert crew member: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Crew{}, fmt.Errorf("commit crew join: %w", err)
	}
	return crew, nil
}

func (store *PostgresCrewStore) CrewByInviteCode(ctx context.Context, inviteCode string) (Crew, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	var crew Crew
	err := store.db.QueryRowContext(ctx,
		`select id, invite_code, name, created_at, created_by_session
		 from crews where invite_code = $1`, inviteCode,
	).Scan(&crew.ID, &crew.InviteCode, &crew.Name, &crew.CreatedAt, &crew.OwnerID)
	if errors.Is(err, sql.ErrNoRows) {
		return Crew{}, ErrCrewNotFound
	}
	if err != nil {
		return Crew{}, fmt.Errorf("load crew: %w", err)
	}
	return crew, nil
}

// RotateInvite is the revocation path: every link already shared stops working.
// Members are unaffected — membership is by session, and they reach the crew
// from their own crew list rather than from the invite URL.
func (store *PostgresCrewStore) RotateInvite(ctx context.Context, crewID, sessionID string) (Crew, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	var crew Crew
	err := store.db.QueryRowContext(ctx,
		`update crews
		 set invite_code = $3
		 where id = $1 and created_by_session = $2
		 returning id, invite_code, name, created_at, created_by_session`,
		crewID, sessionID, newCrewID(),
	).Scan(&crew.ID, &crew.InviteCode, &crew.Name, &crew.CreatedAt, &crew.OwnerID)
	if errors.Is(err, sql.ErrNoRows) {
		return Crew{}, store.ownershipError(ctx, crewID)
	}
	if err != nil {
		return Crew{}, fmt.Errorf("rotate crew invite: %w", err)
	}
	return crew, nil
}

func (store *PostgresCrewStore) RemoveMember(ctx context.Context, crewID, memberID, sessionID string) error {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	// The owner guard lives in the statement itself, so a non-owner cannot race
	// between a check and a delete. The session_id guard stops the owner from
	// removing themselves here — leaving has to run the succession logic.
	result, err := store.db.ExecContext(ctx,
		`delete from crew_members m
		 using crews c
		 where m.crew_id = c.id
		   and c.id = $1
		   and c.created_by_session = $2
		   and m.member_id = $3
		   and m.session_id <> $2`,
		crewID, sessionID, memberID,
	)
	if err != nil {
		return fmt.Errorf("remove crew member: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("remove crew member rows: %w", err)
	}
	if affected == 0 {
		// Either not the owner, or no such member. Distinguish so the caller can
		// answer 403 vs 404 honestly.
		if err := store.requireOwner(ctx, crewID, sessionID); err != nil {
			return err
		}
		return ErrCrewMemberUnknown
	}
	return nil
}

// LeaveCrew removes the caller and keeps the crew owned. Succession is by join
// order so the crew does not end up ownerless and unrotatable, and an empty crew
// is deleted rather than left as an orphan row holding a name.
func (store *PostgresCrewStore) LeaveCrew(ctx context.Context, crewID, sessionID string) error {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin crew leave: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var ownerID string
	err = tx.QueryRowContext(ctx,
		`select created_by_session from crews where id = $1 for update`, crewID,
	).Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrCrewNotFound
	}
	if err != nil {
		return fmt.Errorf("lock crew for leave: %w", err)
	}

	result, err := tx.ExecContext(ctx,
		`delete from crew_members where crew_id = $1 and session_id = $2`, crewID, sessionID)
	if err != nil {
		return fmt.Errorf("leave crew: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("leave crew rows: %w", err)
	}
	if affected == 0 {
		return ErrNotCrewMember
	}

	if ownerID != sessionID {
		return commitCrewTx(tx, "leave")
	}

	var successor string
	err = tx.QueryRowContext(ctx,
		`select session_id from crew_members
		 where crew_id = $1
		 order by joined_at, member_id
		 limit 1`,
		crewID,
	).Scan(&successor)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := tx.ExecContext(ctx, `delete from crews where id = $1`, crewID); err != nil {
			return fmt.Errorf("delete empty crew: %w", err)
		}
		return commitCrewTx(tx, "leave")
	}
	if err != nil {
		return fmt.Errorf("find crew successor: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`update crews set created_by_session = $2 where id = $1`, crewID, successor,
	); err != nil {
		return fmt.Errorf("transfer crew ownership: %w", err)
	}
	return commitCrewTx(tx, "leave")
}

func commitCrewTx(tx *sql.Tx, action string) error {
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit crew %s: %w", action, err)
	}
	return nil
}

// requireOwner reports why an owner-only write matched nothing.
func (store *PostgresCrewStore) requireOwner(ctx context.Context, crewID, sessionID string) error {
	var ownerID string
	err := store.db.QueryRowContext(ctx,
		`select created_by_session from crews where id = $1`, crewID,
	).Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrCrewNotFound
	}
	if err != nil {
		return fmt.Errorf("load crew owner: %w", err)
	}
	if ownerID != sessionID {
		return ErrNotCrewOwner
	}
	return nil
}

func (store *PostgresCrewStore) ownershipError(ctx context.Context, crewID string) error {
	var exists bool
	if err := store.db.QueryRowContext(ctx,
		`select exists (select 1 from crews where id = $1)`, crewID,
	).Scan(&exists); err != nil {
		return fmt.Errorf("check crew exists: %w", err)
	}
	if !exists {
		return ErrCrewNotFound
	}
	return ErrNotCrewOwner
}

func (store *PostgresCrewStore) CrewsForSession(ctx context.Context, sessionID string) ([]Crew, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	// This is how a member gets back in after the invite is rotated, so it must
	// return the crew's *current* code.
	rows, err := store.db.QueryContext(ctx,
		`select c.id, c.invite_code, c.name, c.created_at, c.created_by_session
		 from crews c
		 join crew_members m on m.crew_id = c.id
		 where m.session_id = $1
		 order by m.joined_at desc`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("load crews for session: %w", err)
	}
	defer func() { _ = rows.Close() }()

	crews := []Crew{}
	for rows.Next() {
		var crew Crew
		if err := rows.Scan(&crew.ID, &crew.InviteCode, &crew.Name, &crew.CreatedAt, &crew.OwnerID); err != nil {
			return nil, fmt.Errorf("scan crew: %w", err)
		}
		crews = append(crews, crew)
	}
	return crews, rows.Err()
}

// CrewProgress reports where every member stands on one puzzle. A member with
// no attempt row yet simply has not started; the left join keeps them on the
// board instead of dropping them.
func (store *PostgresCrewStore) CrewProgress(ctx context.Context, crewID, puzzleID string) ([]CrewMemberProgress, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	rows, err := store.db.QueryContext(ctx,
		`select m.member_id,
		        m.session_id,
		        m.display_name,
		        a.id is not null,
		        coalesce(array_length(a.solved_group_ids, 1), 0),
		        coalesce(a.mistakes, 0),
		        coalesce(a.completed, false),
		        coalesce(a.failed, false),
		        a.started_at,
		        a.completed_at
		 from crew_members m
		 left join attempts a
		   on a.session_id = m.session_id and a.puzzle_id = $2
		 where m.crew_id = $1
		 order by m.joined_at, m.display_name`,
		crewID, puzzleID,
	)
	if err != nil {
		return nil, fmt.Errorf("load crew progress: %w", err)
	}
	defer func() { _ = rows.Close() }()

	progress := []CrewMemberProgress{}
	for rows.Next() {
		var row CrewMemberProgress
		if err := rows.Scan(
			&row.MemberID, &row.SessionID, &row.DisplayName, &row.Started,
			&row.SolvedCount, &row.Mistakes, &row.Completed, &row.Failed,
			&row.StartedAt, &row.CompletedAt,
		); err != nil {
			return nil, fmt.Errorf("scan crew progress: %w", err)
		}
		progress = append(progress, row)
	}
	return progress, rows.Err()
}

func (store *PostgresCrewStore) CrewGuessHistory(ctx context.Context, crewID, puzzleID string) (map[string][][]string, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	rows, err := store.db.QueryContext(ctx,
		`select a.session_id, g.selected_tile_ids
		 from crew_members m
		 join attempts a on a.session_id = m.session_id and a.puzzle_id = $2
		 join attempt_guesses g on g.attempt_id = a.id
		 where m.crew_id = $1
		 order by a.session_id, g.created_at`,
		crewID, puzzleID,
	)
	if err != nil {
		return nil, fmt.Errorf("load crew guess history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	history := map[string][][]string{}
	for rows.Next() {
		var sessionID string
		var tileIDs []string
		if err := rows.Scan(&sessionID, pq.Array(&tileIDs)); err != nil {
			return nil, fmt.Errorf("scan crew guess: %w", err)
		}
		history[sessionID] = append(history[sessionID], tileIDs)
	}
	return history, rows.Err()
}

// sortCrewBoard ranks the board the way a friend group reads it: finishers
// first (fastest first), then people still playing, then people who have not
// started. Ties fall back to display name so the order is stable.
func sortCrewBoard(board []CrewBoardEntry) {
	sort.SliceStable(board, func(left, right int) bool {
		a, b := board[left], board[right]
		if rank := crewRank(a); rank != crewRank(b) {
			return rank < crewRank(b)
		}
		if a.Solved && b.Solved {
			if a.ElapsedSeconds != nil && b.ElapsedSeconds != nil && *a.ElapsedSeconds != *b.ElapsedSeconds {
				return *a.ElapsedSeconds < *b.ElapsedSeconds
			}
			if a.Mistakes != b.Mistakes {
				return a.Mistakes < b.Mistakes
			}
		}
		if a.SolvedCount != b.SolvedCount {
			return a.SolvedCount > b.SolvedCount
		}
		return strings.ToLower(a.DisplayName) < strings.ToLower(b.DisplayName)
	})
}

func crewRank(entry CrewBoardEntry) int {
	switch {
	case entry.Solved:
		return 0
	case entry.Playing:
		return 1
	case entry.Failed:
		return 2
	default:
		return 3
	}
}
