package vibegrid

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/lib/pq"
)

const (
	maxPushEndpointLength = 2000
	maxPushKeyLength      = 200
)

var ErrPushSubscriptionInvalid = errors.New("push subscription is invalid")

// PushSubscription is one browser's address for reminders. It carries no
// personal data: the endpoint is issued by the push service, and the two keys
// are what encrypt the payload to that browser.
type PushSubscription struct {
	ID        int64  `json:"-"`
	SessionID string `json:"-"`
	Endpoint  string `json:"endpoint"`
	P256dh    string `json:"p256dh"`
	Auth      string `json:"auth"`
}

// PushSubscriptionStore owns reminder addresses. Like crews it is durable and
// inherently multi-session, so there is no in-memory implementation: without a
// database the endpoints report the feature as unavailable rather than pretend
// to have remembered anything.
type PushSubscriptionStore interface {
	SaveSubscription(ctx context.Context, subscription PushSubscription, now time.Time) error
	DeleteSubscription(ctx context.Context, endpoint string) error
	// UnsettledSubscriptions returns every subscription belonging to any of the
	// given sessions, keyed by session, that has not yet been settled for this
	// outbox event. One person may be subscribed on a phone and a laptop and
	// should be reminded on both — once each, however often the event retries.
	UnsettledSubscriptions(ctx context.Context, eventID int64, sessionIDs []string) (map[string][]PushSubscription, error)
	// SettleDelivery records that the push service accepted this event for this
	// subscription, so a retry of the event skips it.
	SettleDelivery(ctx context.Context, eventID, subscriptionID int64, statusCode int, now time.Time) error
	// SettleRejection records that the push service refused this event for this
	// subscription in a way no retry can change, so a retry skips it too.
	SettleRejection(ctx context.Context, eventID, subscriptionID int64, statusCode int, now time.Time) error
	// CrewsAwaitingDigest lists crews that have at least one subscribed member,
	// so the sweeper never enqueues a reminder nobody could receive.
	CrewsAwaitingDigest(ctx context.Context) ([]string, error)
}

type PostgresPushSubscriptionStore struct {
	db *sql.DB
}

func NewPostgresPushSubscriptionStore(database *sql.DB) *PostgresPushSubscriptionStore {
	return &PostgresPushSubscriptionStore{db: database}
}

func (store *PostgresPushSubscriptionStore) SaveSubscription(ctx context.Context, subscription PushSubscription, now time.Time) error {
	if err := validatePushSubscription(subscription); err != nil {
		return err
	}
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	// Re-subscribing the same browser rebinds the endpoint to the current
	// session and clears its failure history, so a browser that was gone and
	// came back is not carrying a grudge from before.
	if _, err := store.db.ExecContext(ctx,
		`insert into push_subscriptions (session_id, endpoint, p256dh, auth, created_at)
		 values ($1, $2, $3, $4, $5)
		 on conflict (endpoint) do update
		   set session_id = excluded.session_id,
		       p256dh = excluded.p256dh,
		       auth = excluded.auth,
		       failure_count = 0`,
		subscription.SessionID, subscription.Endpoint, subscription.P256dh, subscription.Auth, now.UTC(),
	); err != nil {
		return fmt.Errorf("save push subscription: %w", err)
	}
	return nil
}

func (store *PostgresPushSubscriptionStore) DeleteSubscription(ctx context.Context, endpoint string) error {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	if _, err := store.db.ExecContext(ctx,
		`delete from push_subscriptions where endpoint = $1`, endpoint,
	); err != nil {
		return fmt.Errorf("delete push subscription: %w", err)
	}
	return nil
}

func (store *PostgresPushSubscriptionStore) UnsettledSubscriptions(ctx context.Context, eventID int64, sessionIDs []string) (map[string][]PushSubscription, error) {
	subscriptions := map[string][]PushSubscription{}
	if len(sessionIDs) == 0 {
		return subscriptions, nil
	}
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	rows, err := store.db.QueryContext(ctx,
		`select s.id, s.session_id, s.endpoint, s.p256dh, s.auth
		 from push_subscriptions s
		 where s.session_id = any($2)
		   and not exists (
		     select 1 from push_deliveries d
		     where d.event_id = $1 and d.subscription_id = s.id)
		 order by s.id`, eventID, pq.Array(sessionIDs))
	if err != nil {
		return nil, fmt.Errorf("load push subscriptions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var subscription PushSubscription
		if err := rows.Scan(&subscription.ID, &subscription.SessionID, &subscription.Endpoint, &subscription.P256dh, &subscription.Auth); err != nil {
			return nil, fmt.Errorf("scan push subscription: %w", err)
		}
		subscriptions[subscription.SessionID] = append(subscriptions[subscription.SessionID], subscription)
	}
	return subscriptions, rows.Err()
}

func (store *PostgresPushSubscriptionStore) SettleDelivery(ctx context.Context, eventID, subscriptionID int64, statusCode int, now time.Time) error {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	// One statement, so the ledger row and the subscription's health move
	// together. Selecting the subscription rather than inserting its id blind
	// lets an unsubscribe that raced the send simply record nothing.
	if _, err := store.db.ExecContext(ctx,
		`with settled as (
		   insert into push_deliveries (event_id, subscription_id, outcome, status_code, settled_at)
		   select $1, id, 'DELIVERED', $3, $4 from push_subscriptions where id = $2
		   on conflict (event_id, subscription_id) do nothing
		 )
		 update push_subscriptions set last_success_at = $4, failure_count = 0 where id = $2`,
		eventID, subscriptionID, statusCode, now.UTC(),
	); err != nil {
		return fmt.Errorf("record push delivery: %w", err)
	}
	return nil
}

func (store *PostgresPushSubscriptionStore) SettleRejection(ctx context.Context, eventID, subscriptionID int64, statusCode int, now time.Time) error {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	if _, err := store.db.ExecContext(ctx,
		`insert into push_deliveries (event_id, subscription_id, outcome, status_code, settled_at)
		 select $1, id, 'REJECTED', $3, $4 from push_subscriptions where id = $2
		 on conflict (event_id, subscription_id) do nothing`,
		eventID, subscriptionID, statusCode, now.UTC(),
	); err != nil {
		return fmt.Errorf("record push rejection: %w", err)
	}
	return nil
}

func (store *PostgresPushSubscriptionStore) CrewsAwaitingDigest(ctx context.Context) ([]string, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	rows, err := store.db.QueryContext(ctx,
		`select distinct m.crew_id
		 from crew_members m
		 join push_subscriptions s on s.session_id = m.session_id
		 order by m.crew_id`)
	if err != nil {
		return nil, fmt.Errorf("load crews awaiting digest: %w", err)
	}
	defer func() { _ = rows.Close() }()

	crewIDs := []string{}
	for rows.Next() {
		var crewID string
		if err := rows.Scan(&crewID); err != nil {
			return nil, fmt.Errorf("scan crew awaiting digest: %w", err)
		}
		crewIDs = append(crewIDs, crewID)
	}
	return crewIDs, rows.Err()
}

// validatePushSubscription screens what the browser handed us before it reaches
// storage or an outbound request. The endpoint is attacker-influenced — a page
// can call subscribe with anything — so it must be an https URL and nothing
// else, or the delivery worker becomes a request forwarder.
func validatePushSubscription(subscription PushSubscription) error {
	if subscription.SessionID == "" {
		return ErrPushSubscriptionInvalid
	}
	if !validPushKey(subscription.P256dh) || !validPushKey(subscription.Auth) {
		return ErrPushSubscriptionInvalid
	}
	return validatePushEndpoint(subscription.Endpoint)
}

func validatePushEndpoint(endpoint string) error {
	if endpoint == "" || len(endpoint) > maxPushEndpointLength {
		return ErrPushSubscriptionInvalid
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return ErrPushSubscriptionInvalid
	}
	if strings.ContainsAny(endpoint, " \t\r\n") {
		return ErrPushSubscriptionInvalid
	}
	return nil
}

// validPushKey accepts the base64url the Push API produces, without deciding
// how long a given push service's keys ought to be.
func validPushKey(value string) bool {
	if value == "" || len(value) > maxPushKeyLength {
		return false
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '=':
		default:
			return false
		}
	}
	return true
}
