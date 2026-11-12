package vibegrid

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"
)

// GuestSessionStore separates the bearer token in the cookie from the stable
// anonymous identity referenced by attempts, crew memberships, cards, and
// ballots. Only token hashes are stored. Rotation can therefore invalidate a
// copied cookie without moving or rewriting the player's durable history.
type GuestSessionStore interface {
	ResolveOrCreate(ctx context.Context, tokenHash, legacyIdentity, proposedIdentity string, now, expiresAt time.Time) (identity string, active bool, err error)
	Rotate(ctx context.Context, oldTokenHash, newTokenHash string, now, expiresAt time.Time) (identity string, err error)
}

type memoryGuestSession struct {
	identity  string
	expiresAt time.Time
	revokedAt *time.Time
	lastSeen  time.Time
}

type MemoryGuestSessionStore struct {
	mu       sync.Mutex
	sessions map[string]memoryGuestSession
}

const maxMemoryGuestSessions = 10_000

func NewMemoryGuestSessionStore() *MemoryGuestSessionStore {
	return &MemoryGuestSessionStore{sessions: map[string]memoryGuestSession{}}
}

func (store *MemoryGuestSessionStore) ResolveOrCreate(_ context.Context, tokenHash, _ string, proposedIdentity string, now, expiresAt time.Time) (string, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	if session, ok := store.sessions[tokenHash]; ok {
		if session.revokedAt != nil || !session.expiresAt.After(now) {
			if session.revokedAt == nil {
				revoked := now
				session.revokedAt = &revoked
				store.sessions[tokenHash] = session
			}
			return "", false, nil
		}
		session.expiresAt = expiresAt
		session.lastSeen = now
		store.sessions[tokenHash] = session
		return session.identity, true, nil
	}

	for _, session := range store.sessions {
		if session.identity == proposedIdentity && session.revokedAt == nil && session.expiresAt.After(now) {
			return "", false, fmt.Errorf("guest identity already has an active token")
		}
	}
	store.makeRoomLocked(now)
	store.sessions[tokenHash] = memoryGuestSession{identity: proposedIdentity, expiresAt: expiresAt, lastSeen: now}
	return proposedIdentity, true, nil
}

func (store *MemoryGuestSessionStore) Rotate(_ context.Context, oldTokenHash, newTokenHash string, now, expiresAt time.Time) (string, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	old, ok := store.sessions[oldTokenHash]
	if !ok || !old.expiresAt.After(now) {
		return "", ErrGuestSessionInactive
	}
	if old.revokedAt != nil {
		if replacement, exists := store.sessions[newTokenHash]; exists && replacement.identity == old.identity && replacement.revokedAt == nil && replacement.expiresAt.After(now) {
			replacement.expiresAt = expiresAt
			replacement.lastSeen = now
			store.sessions[newTokenHash] = replacement
			return old.identity, nil
		}
		return "", ErrGuestSessionInactive
	}
	if _, exists := store.sessions[newTokenHash]; exists {
		return "", errors.New("guest replacement token already exists")
	}
	revoked := now
	old.revokedAt = &revoked
	store.sessions[oldTokenHash] = old
	store.makeRoomLocked(now)
	store.sessions[newTokenHash] = memoryGuestSession{identity: old.identity, expiresAt: expiresAt, lastSeen: now}
	return old.identity, nil
}

func (store *MemoryGuestSessionStore) makeRoomLocked(now time.Time) {
	if len(store.sessions) < maxMemoryGuestSessions {
		return
	}
	for tokenHash, session := range store.sessions {
		if !session.expiresAt.After(now) {
			delete(store.sessions, tokenHash)
		}
	}
	if len(store.sessions) < maxMemoryGuestSessions {
		return
	}
	var oldestToken string
	var oldestSeen time.Time
	for tokenHash, session := range store.sessions {
		if oldestToken == "" || session.lastSeen.Before(oldestSeen) {
			oldestToken = tokenHash
			oldestSeen = session.lastSeen
		}
	}
	delete(store.sessions, oldestToken)
}

type PostgresGuestSessionStore struct {
	db *sql.DB
}

func NewPostgresGuestSessionStore(database *sql.DB) *PostgresGuestSessionStore {
	return &PostgresGuestSessionStore{db: database}
}

func (store *PostgresGuestSessionStore) ResolveOrCreate(ctx context.Context, tokenHash, legacyIdentity, proposedIdentity string, now, expiresAt time.Time) (string, bool, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return "", false, fmt.Errorf("begin guest session resolve: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	// Two tabs commonly inherit the same cookie from one document navigation and
	// can call /api/session together. Serialize that first registration by token
	// so both requests resolve to one identity instead of letting the primary-key
	// race turn the loser into a newly minted browser session.
	if _, err := tx.ExecContext(ctx,
		`select pg_advisory_xact_lock($1)`, advisoryLockID("guest-session:"+tokenHash),
	); err != nil {
		return "", false, fmt.Errorf("lock guest session token: %w", err)
	}

	var identity string
	var lastSeen time.Time
	var storedExpiry time.Time
	var revokedAt sql.NullTime
	err = tx.QueryRowContext(ctx,
		`select identity_id, last_seen_at, expires_at, revoked_at
		 from guest_sessions where token_hash = $1 for update`, tokenHash,
	).Scan(&identity, &lastSeen, &storedExpiry, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		identity = proposedIdentity
		// Before revocable guest sessions, the cookie value was also the domain
		// identity. Preserve that value only when durable rows prove this is an
		// actual pre-migration browser. A cookie merely minted by a public HTML
		// response has no such rows and receives a one-way-derived identity, so a
		// new bearer secret is never copied into an identity column.
		var hasLegacyRows bool
		if legacyIdentity != "" && legacyIdentity != proposedIdentity {
			if err := tx.QueryRowContext(ctx,
				`select exists (
				   select 1 from crew_member_identities where session_id = $1
				   union all
				   select 1 from crew_members where session_id = $1
				   union all
				   select 1 from attempts where session_id = $1
				 )`, legacyIdentity,
			).Scan(&hasLegacyRows); err != nil {
				return "", false, fmt.Errorf("detect legacy guest identity: %w", err)
			}
		}
		if hasLegacyRows {
			identity = legacyIdentity
		}
		if _, err := tx.ExecContext(ctx,
			`insert into guest_sessions
			 (token_hash, identity_id, created_at, last_seen_at, expires_at)
			 values ($1, $2, $3, $3, $4)`,
			tokenHash, identity, now.UTC(), expiresAt.UTC(),
		); err != nil {
			return "", false, fmt.Errorf("register guest session: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return "", false, fmt.Errorf("commit guest session registration: %w", err)
		}
		return identity, true, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("load guest session: %w", err)
	}
	if revokedAt.Valid || !storedExpiry.After(now) {
		if !revokedAt.Valid {
			if _, err := tx.ExecContext(ctx,
				`update guest_sessions set revoked_at = $2 where token_hash = $1`, tokenHash, now.UTC(),
			); err != nil {
				return "", false, fmt.Errorf("expire guest session: %w", err)
			}
		}
		if err := tx.Commit(); err != nil {
			return "", false, fmt.Errorf("commit guest session expiry: %w", err)
		}
		return "", false, nil
	}
	// Visible crew pages poll every 30 seconds. Extending a 30-day session on
	// every poll turns one idle tab into 2,880 writes per day. Refresh at most
	// hourly; an active token still retains essentially the same sliding window.
	if now.Sub(lastSeen) >= time.Hour || storedExpiry.Before(now.Add(29*24*time.Hour)) {
		if _, err := tx.ExecContext(ctx,
			`update guest_sessions
			 set last_seen_at = $2, expires_at = $3
			 where token_hash = $1`, tokenHash, now.UTC(), expiresAt.UTC(),
		); err != nil {
			return "", false, fmt.Errorf("refresh guest session: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return "", false, fmt.Errorf("commit guest session refresh: %w", err)
	}
	return identity, true, nil
}

func (store *PostgresGuestSessionStore) Rotate(ctx context.Context, oldTokenHash, newTokenHash string, now, expiresAt time.Time) (string, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin guest session rotation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var storedIdentity string
	var storedExpiry time.Time
	var revokedAt sql.NullTime
	err = tx.QueryRowContext(ctx,
		`select identity_id, expires_at, revoked_at
		 from guest_sessions where token_hash = $1 for update`, oldTokenHash,
	).Scan(&storedIdentity, &storedExpiry, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) || !storedExpiry.After(now) {
		return "", ErrGuestSessionInactive
	}
	if err != nil {
		return "", fmt.Errorf("lock guest session for rotation: %w", err)
	}
	if revokedAt.Valid {
		var replacementIdentity string
		err := tx.QueryRowContext(ctx,
			`select identity_id from guest_sessions
			 where token_hash = $1 and revoked_at is null and expires_at > $2`,
			newTokenHash, now.UTC(),
		).Scan(&replacementIdentity)
		if errors.Is(err, sql.ErrNoRows) || replacementIdentity != storedIdentity {
			return "", ErrGuestSessionInactive
		}
		if err != nil {
			return "", fmt.Errorf("load replayed guest session rotation: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`update guest_sessions set last_seen_at = $2, expires_at = $3 where token_hash = $1`,
			newTokenHash, now.UTC(), expiresAt.UTC(),
		); err != nil {
			return "", fmt.Errorf("refresh replayed guest session rotation: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return "", fmt.Errorf("commit replayed guest session rotation: %w", err)
		}
		return storedIdentity, nil
	}
	if _, err := tx.ExecContext(ctx,
		`update guest_sessions set revoked_at = $2 where token_hash = $1`, oldTokenHash, now.UTC(),
	); err != nil {
		return "", fmt.Errorf("revoke guest session: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`insert into guest_sessions
		 (token_hash, identity_id, created_at, last_seen_at, expires_at)
		 values ($1, $2, $3, $3, $4)`,
		newTokenHash, storedIdentity, now.UTC(), expiresAt.UTC(),
	); err != nil {
		return "", fmt.Errorf("insert replacement guest session: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit guest session rotation: %w", err)
	}
	return storedIdentity, nil
}

var ErrGuestSessionInactive = errors.New("guest session is inactive")
