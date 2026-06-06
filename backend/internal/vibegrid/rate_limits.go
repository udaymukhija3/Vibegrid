package vibegrid

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type RateLimitStore interface {
	Check(ctx context.Context, key string, limit int, window time.Duration, now time.Time) (rateLimitDecision, error)
}

type PostgresRateLimitStore struct {
	db *sql.DB
}

func NewPostgresRateLimitStore(database *sql.DB) *PostgresRateLimitStore {
	return &PostgresRateLimitStore{db: database}
}

func (store *PostgresRateLimitStore) Check(ctx context.Context, key string, limit int, window time.Duration, now time.Time) (rateLimitDecision, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	if limit <= 0 || window <= 0 {
		return rateLimitDecision{allowed: true}, nil
	}

	bucketStart := now.UTC().Truncate(window)
	var hits int
	if err := store.db.QueryRowContext(ctx,
		`insert into rate_limit_hits (key, bucket_start, hits, updated_at)
		 values ($1, $2, 1, now())
		 on conflict (key, bucket_start)
		 do update set hits = rate_limit_hits.hits + 1, updated_at = now()
		 returning hits`,
		key, bucketStart,
	).Scan(&hits); err != nil {
		return rateLimitDecision{}, fmt.Errorf("rate limit hit: %w", err)
	}

	// Best-effort pruning keeps the table from growing forever. It is intentionally
	// not in a transaction with the hit path; a prune failure should not reject a user.
	if hits == 1 {
		_, _ = store.db.ExecContext(ctx,
			`delete from rate_limit_hits where updated_at < now() - interval '2 days'`)
	}

	if hits > limit {
		retryAfter := bucketStart.Add(window).Sub(now.UTC())
		if retryAfter <= 0 {
			retryAfter = time.Second
		}
		return rateLimitDecision{retryAfter: retryAfter}, nil
	}
	return rateLimitDecision{allowed: true}, nil
}
