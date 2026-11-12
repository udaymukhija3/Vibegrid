package vibegrid

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

const (
	// pushSweepInterval is how often the sweeper looks; the dedupe key, not the
	// interval, is what makes a digest happen once. Ticking often just means the
	// digest goes out promptly after the hour passes, and survives a restart.
	pushSweepInterval = 15 * time.Minute

	// DefaultPushDigestHourUTC is early evening in Europe and the afternoon in
	// the Americas. There is no per-member time zone in this product — there is
	// no account to hang one on — so one hour has to serve everyone, and an
	// evening hour beats the midnight rollover that would reach Asia at dawn.
	DefaultPushDigestHourUTC = 18
)

// PushDigestEnqueuer is the write side of the outbox the sweeper needs.
type PushDigestEnqueuer interface {
	EnqueueCrewDigest(ctx context.Context, crewID, date string) error
}

func (store *PostgresNotificationOutbox) EnqueueCrewDigest(ctx context.Context, crewID, date string) error {
	payload, err := json.Marshal(crewDigestPayload{CrewID: crewID, Date: date})
	if err != nil {
		return fmt.Errorf("encode crew digest: %w", err)
	}
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	// The dedupe key is the whole scheduler. Two instances sweeping at once, or
	// one instance restarting mid-sweep, converge on a single digest per crew
	// per day rather than notifying everybody twice.
	if _, err := store.db.ExecContext(ctx,
		`insert into notification_outbox (topic, aggregate_type, aggregate_id, dedupe_key, payload)
		 values ($1, 'crew', $2, $3, $4)
		 on conflict (dedupe_key) do nothing`,
		CrewDigestTopic, crewID, "crew-digest:"+crewID+":"+date, string(payload),
	); err != nil {
		return fmt.Errorf("enqueue crew digest: %w", err)
	}
	return nil
}

// PushDigestSweeper enqueues one reminder per crew per day, for crews that have
// somebody able to receive it.
type PushDigestSweeper struct {
	subscriptions PushSubscriptionStore
	outbox        PushDigestEnqueuer
	clock         func() time.Time
	hourUTC       int
	logger        *slog.Logger
}

func NewPushDigestSweeper(
	subscriptions PushSubscriptionStore,
	outbox PushDigestEnqueuer,
	clock func() time.Time,
	hourUTC int,
	logger *slog.Logger,
) *PushDigestSweeper {
	if hourUTC < 0 || hourUTC > 23 {
		hourUTC = DefaultPushDigestHourUTC
	}
	return &PushDigestSweeper{subscriptions: subscriptions, outbox: outbox, clock: clock, hourUTC: hourUTC, logger: logger}
}

// Run sweeps until the context is cancelled.
func (sweeper *PushDigestSweeper) Run(ctx context.Context) {
	ticker := time.NewTicker(pushSweepInterval)
	defer ticker.Stop()

	sweeper.SweepOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweeper.SweepOnce(ctx)
		}
	}
}

// SweepOnce enqueues today's digest for every crew with a subscriber, once the
// digest hour has passed in UTC. Before that hour it does nothing, so a restart
// in the morning does not fire the evening's reminder early.
func (sweeper *PushDigestSweeper) SweepOnce(ctx context.Context) {
	now := sweeper.clock().UTC()
	if now.Hour() < sweeper.hourUTC {
		return
	}
	date := now.Format("2006-01-02")

	crewIDs, err := sweeper.subscriptions.CrewsAwaitingDigest(ctx)
	if err != nil {
		sweeper.logger.Error("could not list crews for the daily digest", "error", err)
		return
	}
	for _, crewID := range crewIDs {
		if err := sweeper.outbox.EnqueueCrewDigest(ctx, crewID, date); err != nil {
			sweeper.logger.Error("could not enqueue a crew digest", "crew", crewID, "error", err)
		}
	}
}

// CrewByID resolves a crew by its internal id. The digest carries the internal
// id rather than the invite code so a rotated invite — the thing you do when a
// link leaks — does not strand queued reminders.
func (store *PostgresCrewStore) CrewByID(ctx context.Context, crewID string) (Crew, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	var crew Crew
	err := store.db.QueryRowContext(ctx,
		`select id, invite_code, name, created_at, created_by_session
		 from crews where id = $1`, crewID,
	).Scan(&crew.ID, &crew.InviteCode, &crew.Name, &crew.CreatedAt, &crew.OwnerID)
	if errors.Is(err, sql.ErrNoRows) {
		return Crew{}, ErrCrewNotFound
	}
	if err != nil {
		return Crew{}, fmt.Errorf("load crew by id: %w", err)
	}
	return crew, nil
}
