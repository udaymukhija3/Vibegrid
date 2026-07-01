package vibegrid

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// AdminSessionStore keeps opaque browser sessions revocable. The production
// implementation stores only a SHA-256 token hash; a database leak therefore
// cannot be replayed directly as an admin cookie.
type AdminSessionStore interface {
	Create(ctx context.Context, tokenHash string, expiresAt time.Time) error
	Active(ctx context.Context, tokenHash string, now time.Time) (time.Time, bool, error)
	Revoke(ctx context.Context, tokenHash string, now time.Time) error
	PruneExpired(ctx context.Context, before time.Time, limit int) (int64, error)
}

func newAdminSessionToken() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		panic("crypto/rand failed while generating admin session token: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(bytes)
}

func hashAdminSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

type memoryAdminSessionStore struct {
	mu       sync.Mutex
	sessions map[string]memoryAdminSession
}

type memoryAdminSession struct {
	expiresAt time.Time
	revokedAt *time.Time
}

func NewMemoryAdminSessionStore() AdminSessionStore {
	return &memoryAdminSessionStore{sessions: map[string]memoryAdminSession{}}
}

func (store *memoryAdminSessionStore) Create(_ context.Context, tokenHash string, expiresAt time.Time) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.sessions[tokenHash] = memoryAdminSession{expiresAt: expiresAt.UTC()}
	return nil
}

func (store *memoryAdminSessionStore) Active(_ context.Context, tokenHash string, now time.Time) (time.Time, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	session, ok := store.sessions[tokenHash]
	if !ok || session.revokedAt != nil || !now.Before(session.expiresAt) {
		return time.Time{}, false, nil
	}
	return session.expiresAt, true, nil
}

func (store *memoryAdminSessionStore) Revoke(_ context.Context, tokenHash string, now time.Time) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	session, ok := store.sessions[tokenHash]
	if !ok {
		return nil
	}
	revokedAt := now.UTC()
	session.revokedAt = &revokedAt
	store.sessions[tokenHash] = session
	return nil
}

func (store *memoryAdminSessionStore) PruneExpired(_ context.Context, before time.Time, limit int) (int64, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	var deleted int64
	for hash, session := range store.sessions {
		if limit > 0 && deleted >= int64(limit) {
			break
		}
		if session.expiresAt.Before(before) || session.revokedAt != nil {
			delete(store.sessions, hash)
			deleted++
		}
	}
	return deleted, nil
}

type PostgresAdminSessionStore struct {
	db *sql.DB
}

func NewPostgresAdminSessionStore(database *sql.DB) *PostgresAdminSessionStore {
	return &PostgresAdminSessionStore{db: database}
}

func (store *PostgresAdminSessionStore) Create(ctx context.Context, tokenHash string, expiresAt time.Time) error {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()
	_, err := store.db.ExecContext(ctx,
		`insert into admin_sessions (token_hash, expires_at) values ($1, $2)`,
		tokenHash, expiresAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("create admin session: %w", err)
	}
	return nil
}

func (store *PostgresAdminSessionStore) Active(ctx context.Context, tokenHash string, now time.Time) (time.Time, bool, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()
	var expiresAt time.Time
	err := store.db.QueryRowContext(ctx,
		`select expires_at from admin_sessions
		 where token_hash = $1 and revoked_at is null and expires_at > $2`,
		tokenHash, now.UTC(),
	).Scan(&expiresAt)
	if err == sql.ErrNoRows {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("load admin session: %w", err)
	}
	return expiresAt.UTC(), true, nil
}

func (store *PostgresAdminSessionStore) Revoke(ctx context.Context, tokenHash string, now time.Time) error {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()
	_, err := store.db.ExecContext(ctx,
		`update admin_sessions set revoked_at = $2 where token_hash = $1 and revoked_at is null`,
		tokenHash, now.UTC(),
	)
	if err != nil {
		return fmt.Errorf("revoke admin session: %w", err)
	}
	return nil
}

func (store *PostgresAdminSessionStore) PruneExpired(ctx context.Context, before time.Time, limit int) (int64, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()
	if limit <= 0 || limit > 10_000 {
		limit = 1_000
	}
	result, err := store.db.ExecContext(ctx,
		`delete from admin_sessions
		 where token_hash in (
		   select token_hash from admin_sessions
		   where expires_at < $1 or revoked_at is not null
		   order by expires_at asc limit $2
		 )`,
		before.UTC(), limit,
	)
	if err != nil {
		return 0, fmt.Errorf("prune admin sessions: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count pruned admin sessions: %w", err)
	}
	return deleted, nil
}
