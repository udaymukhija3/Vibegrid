package vibegrid

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/jackc/pgx/v5/pgconn"
)

// TestClassifyPushStatus pins which push service answers are worth a retry. A
// retry that can only get the same refusal still burns an outbox attempt and
// ages the event toward the dead-letter count operators watch.
func TestClassifyPushStatus(t *testing.T) {
	for status, want := range map[int]pushVerdict{
		http.StatusCreated:               pushAccepted,
		http.StatusOK:                    pushAccepted,
		http.StatusBadRequest:            pushRejected,
		http.StatusUnauthorized:          pushRejected,
		http.StatusForbidden:             pushRejected,
		http.StatusRequestEntityTooLarge: pushRejected,
		http.StatusNotFound:              pushGone,
		http.StatusGone:                  pushGone,
		http.StatusRequestTimeout:        pushRetry,
		http.StatusTooManyRequests:       pushRetry,
		http.StatusInternalServerError:   pushRetry,
		http.StatusBadGateway:            pushRetry,
		http.StatusServiceUnavailable:    pushRetry,
		http.StatusFound:                 pushRetry,
	} {
		if got := classifyPushStatus(status); got != want {
			t.Errorf("status %d classified as %d, want %d", status, got, want)
		}
	}
}

// fakePushService stands in for FCM/APNs/autopush. It counts every request per
// endpoint, including ones it fails, because "the retry only reached the device
// that missed it" is a statement about requests, not about outcomes.
type fakePushService struct {
	t       *testing.T
	respond func(endpoint string, attempt int) (int, error)

	mu    sync.Mutex
	sends map[string]int
}

func (service *fakePushService) RoundTrip(request *http.Request) (*http.Response, error) {
	_, _ = io.Copy(io.Discard, request.Body)
	_ = request.Body.Close()
	// Prove the real webpush path ran: signed and encrypted, not a bare POST.
	if !strings.HasPrefix(request.Header.Get("Authorization"), "vapid ") || request.Header.Get("Content-Encoding") != "aes128gcm" {
		service.t.Errorf("push request was not VAPID-signed and encrypted: %v", request.Header)
	}
	endpoint := request.URL.String()
	service.mu.Lock()
	service.sends[endpoint]++
	attempt := service.sends[endpoint]
	service.mu.Unlock()

	status, err := service.respond(endpoint, attempt)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"reason":"from the fake push service"}`)),
		Request:    request,
	}, nil
}

func (service *fakePushService) count(endpoint string) int {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.sends[endpoint]
}

type stubDigestCrews map[string]VibeCrewSnapshot

func (crews stubDigestCrews) CrewByID(_ context.Context, crewID string) (Crew, error) {
	if _, ok := crews[crewID]; !ok {
		return Crew{}, ErrCrewNotFound
	}
	return Crew{ID: crewID, InviteCode: "invite-" + crewID, Name: "Crew " + crewID}, nil
}

func (crews stubDigestCrews) CrewSnapshot(_ context.Context, crewID string, _ []string) (VibeCrewSnapshot, error) {
	return crews[crewID], nil
}

// digestFixture runs crew digests through the real Postgres outbox, the real
// subscription store and ledger, and the real webpush encryption, with only
// the push service itself faked. Nobody in either crew has played, so every
// subscribed member is owed today's nudge.
type digestFixture struct {
	t         *testing.T
	ctx       context.Context
	database  *sql.DB
	store     *PostgresPushSubscriptionStore
	outbox    *PostgresNotificationOutbox
	push      *fakePushService
	deliverer NotificationDeliverer
}

const digestFixtureDate = "2026-08-21"

func newDigestFixture(t *testing.T, respond func(endpoint string, attempt int) (int, error)) *digestFixture {
	t.Helper()
	databaseURL := requireTestDatabase(t)
	ctx := context.Background()
	if err := MigrateDB(ctx, databaseURL); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	database, err := OpenDB(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	// The cascade clears the ledger with its two parents.
	if _, err := database.Exec(`truncate notification_outbox, push_subscriptions restart identity cascade`); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	private, public, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("vapid keys: %v", err)
	}
	store := NewPostgresPushSubscriptionStore(database)
	member := func(name string) VibeRoundMember {
		return VibeRoundMember{MemberID: "member-" + name, SessionID: "session-" + name, DisplayName: name}
	}
	crews := stubDigestCrews{
		"crew-1": {Members: []VibeRoundMember{member("ari"), member("bea"), member("cam")}},
		// Ari is in two crews, so Ari is owed two different digests today.
		"crew-2": {Members: []VibeRoundMember{member("ari"), member("dee")}},
	}
	push := &fakePushService{t: t, respond: respond, sends: map[string]int{}}
	webPush := NewWebPushDeliverer(
		VAPIDKeys{Public: public, Private: private, Subject: "mailto:ops@example.test"},
		store, crews,
		func(_ context.Context, date string) (VibeBoard, error) { return VibeBoardForDate(date) },
		"https://vibegrid.test",
		func() time.Time { return time.Date(2026, 8, 21, 18, 5, 0, 0, time.UTC) },
		testLogger(),
	)
	webPush.client = &http.Client{Transport: push}

	return &digestFixture{
		t:         t,
		ctx:       ctx,
		database:  database,
		store:     store,
		outbox:    NewPostgresNotificationOutbox(database),
		push:      push,
		deliverer: NewRoutedDeliverer(nil, map[string]NotificationDeliverer{CrewDigestTopic: webPush}),
	}
}

// subscribe registers a browser with real P-256 keys, because the deliverer
// encrypts to them before anything reaches the fake service.
func (fixture *digestFixture) subscribe(name string) string {
	fixture.t.Helper()
	private, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		fixture.t.Fatalf("subscription key: %v", err)
	}
	secret := make([]byte, 16)
	if _, err := rand.Read(secret); err != nil {
		fixture.t.Fatalf("subscription secret: %v", err)
	}
	endpoint := "https://push.example.test/send/" + name
	if err := fixture.store.SaveSubscription(fixture.ctx, PushSubscription{
		SessionID: "session-" + name,
		Endpoint:  endpoint,
		P256dh:    base64.RawURLEncoding.EncodeToString(private.PublicKey().Bytes()),
		Auth:      base64.RawURLEncoding.EncodeToString(secret),
	}, time.Now()); err != nil {
		fixture.t.Fatalf("subscribe %s: %v", name, err)
	}
	return endpoint
}

func (fixture *digestFixture) enqueue(crewID string) int64 {
	fixture.t.Helper()
	if err := fixture.outbox.EnqueueCrewDigest(fixture.ctx, crewID, digestFixtureDate); err != nil {
		fixture.t.Fatalf("enqueue %s: %v", crewID, err)
	}
	var id int64
	if err := fixture.database.QueryRow(`select id from notification_outbox where dedupe_key = $1`,
		"crew-digest:"+crewID+":"+digestFixtureDate).Scan(&id); err != nil {
		fixture.t.Fatalf("load enqueued digest: %v", err)
	}
	return id
}

// runAttempt is one outbox tick. Before it, every retry is made due at once, so
// the test does not wait out the backoff the worker would.
func (fixture *digestFixture) runAttempt() {
	fixture.t.Helper()
	if _, err := fixture.database.Exec(`update notification_outbox set available_at = now() where status = 'PENDING'`); err != nil {
		fixture.t.Fatalf("make retries due: %v", err)
	}
	runNotificationBatch(fixture.ctx, testLogger(), fixture.outbox, fixture.deliverer)
}

func (fixture *digestFixture) event(id int64) (status string, attempts int, lastError string) {
	fixture.t.Helper()
	if err := fixture.database.QueryRow(`select status, attempt_count, last_error from notification_outbox where id = $1`, id).
		Scan(&status, &attempts, &lastError); err != nil {
		fixture.t.Fatalf("load event %d: %v", id, err)
	}
	return status, attempts, lastError
}

// ledger returns endpoint -> "OUTCOME status" for one event.
func (fixture *digestFixture) ledger(eventID int64) map[string]string {
	fixture.t.Helper()
	rows, err := fixture.database.Query(`
		select s.endpoint, d.outcome || ' ' || d.status_code
		from push_deliveries d join push_subscriptions s on s.id = d.subscription_id
		where d.event_id = $1`, eventID)
	if err != nil {
		fixture.t.Fatalf("load ledger: %v", err)
	}
	defer func() { _ = rows.Close() }()
	settled := map[string]string{}
	for rows.Next() {
		var endpoint, outcome string
		if err := rows.Scan(&endpoint, &outcome); err != nil {
			fixture.t.Fatalf("scan ledger: %v", err)
		}
		settled[endpoint] = outcome
	}
	if err := rows.Err(); err != nil {
		fixture.t.Fatalf("read ledger: %v", err)
	}
	return settled
}

func (fixture *digestFixture) subscriptionExists(endpoint string) bool {
	fixture.t.Helper()
	var exists bool
	if err := fixture.database.QueryRow(`select exists (select 1 from push_subscriptions where endpoint = $1)`, endpoint).Scan(&exists); err != nil {
		fixture.t.Fatalf("load subscription: %v", err)
	}
	return exists
}

// The product promise is at most one reminder per crew per day. The outbox
// retries a digest as a whole when any send fails, so a member whose phone
// already showed it must not be sent it again when another member's push
// service hiccups — a dismissed notification with a reused tag reappears.
func TestCrewDigestRetryReachesOnlyTheDeviceThatMissedIt(t *testing.T) {
	for name, transient := range map[string]func() (int, error){
		"push service 503":  func() (int, error) { return http.StatusServiceUnavailable, nil },
		"push service 429":  func() (int, error) { return http.StatusTooManyRequests, nil },
		"network failure":   func() (int, error) { return 0, errors.New("connection reset by peer") },
		"push service 500":  func() (int, error) { return http.StatusInternalServerError, nil },
		"request timed out": func() (int, error) { return http.StatusRequestTimeout, nil },
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newDigestFixture(t, func(endpoint string, attempt int) (int, error) {
				if strings.HasSuffix(endpoint, "/bea") && attempt == 1 {
					return transient()
				}
				return http.StatusCreated, nil
			})
			ari := fixture.subscribe("ari")
			bea := fixture.subscribe("bea")
			event := fixture.enqueue("crew-1")

			fixture.runAttempt()
			if status, attempts, lastError := fixture.event(event); status != "PENDING" || attempts != 1 {
				t.Fatalf("after a transient failure the digest is %s at attempt %d, want PENDING at 1", status, attempts)
			} else if strings.Contains(lastError, "/send/") {
				// The endpoint is the per-browser push address; it does not
				// belong in the outbox's error column or the logs it feeds.
				t.Fatalf("outbox error leaked an endpoint: %q", lastError)
			}
			if got := fixture.ledger(event); len(got) != 1 || got[ari] != "DELIVERED 201" {
				t.Fatalf("after attempt 1 the ledger is %v, want only Ari delivered", got)
			}

			fixture.runAttempt()
			if got := fixture.push.count(ari); got != 1 {
				t.Fatalf("Ari was sent the same digest %d times; the retry re-notified a delivered device", got)
			}
			if got := fixture.push.count(bea); got != 2 {
				t.Fatalf("Bea's device got %d requests, want the failed one plus exactly one retry", got)
			}
			if status, attempts, _ := fixture.event(event); status != "DELIVERED" || attempts != 2 {
				t.Fatalf("after the retry the digest is %s at attempt %d, want DELIVERED at 2", status, attempts)
			}
			if got := fixture.ledger(event); len(got) != 2 || got[ari] != "DELIVERED 201" || got[bea] != "DELIVERED 201" {
				t.Fatalf("final ledger %v, want both devices delivered once", got)
			}
		})
	}
}

// A 400/403/413 is the push service refusing this exact request, so retrying it
// only burns attempts — and before the ledger, every one of those attempts
// re-notified everybody else in the crew. 404/410 still forget the device.
func TestCrewDigestPermanentRejectionSettlesWithoutRetrying(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusForbidden, http.StatusRequestEntityTooLarge} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			fixture := newDigestFixture(t, func(endpoint string, _ int) (int, error) {
				switch {
				case strings.HasSuffix(endpoint, "/bea"):
					return status, nil
				case strings.HasSuffix(endpoint, "/cam"):
					return http.StatusGone, nil
				}
				return http.StatusCreated, nil
			})
			ari := fixture.subscribe("ari")
			bea := fixture.subscribe("bea")
			cam := fixture.subscribe("cam")
			event := fixture.enqueue("crew-1")

			fixture.runAttempt()
			if got, _, lastError := fixture.event(event); got != "DELIVERED" {
				t.Fatalf("a permanent %d left the digest %s (%q); it should retire on the first attempt", status, got, lastError)
			}
			settled := fixture.ledger(event)
			if settled[ari] != "DELIVERED 201" {
				t.Fatalf("Ari's delivery was not settled: %v", settled)
			}
			if want := "REJECTED " + strconv.Itoa(status); settled[bea] != want {
				t.Fatalf("Bea's refusal settled as %q, want %q", settled[bea], want)
			}
			// A refusal is about this request, not proof the browser is gone:
			// a 403 from a VAPID key mismatch must not wipe every subscription.
			if !fixture.subscriptionExists(bea) {
				t.Fatal("a permanent rejection deleted the subscription")
			}
			if fixture.subscriptionExists(cam) {
				t.Fatal("a 410 Gone subscription was kept")
			}
			if _, ok := settled[cam]; ok {
				t.Fatal("a forgotten subscription left a ledger row behind")
			}

			// Nothing is left to retry, so another tick sends nothing.
			fixture.runAttempt()
			if fixture.push.count(ari) != 1 || fixture.push.count(bea) != 1 || fixture.push.count(cam) != 1 {
				t.Fatalf("a settled digest was sent again: ari=%d bea=%d cam=%d",
					fixture.push.count(ari), fixture.push.count(bea), fixture.push.count(cam))
			}
		})
	}
}

// When the event does retry for someone else's transient failure, a device
// that was permanently refused is settled too and must not be tried again.
func TestCrewDigestRetrySkipsPermanentlyRejectedDevices(t *testing.T) {
	fixture := newDigestFixture(t, func(endpoint string, attempt int) (int, error) {
		switch {
		case strings.HasSuffix(endpoint, "/bea"):
			return http.StatusForbidden, nil
		case strings.HasSuffix(endpoint, "/cam") && attempt == 1:
			return http.StatusBadGateway, nil
		}
		return http.StatusCreated, nil
	})
	ari := fixture.subscribe("ari")
	bea := fixture.subscribe("bea")
	cam := fixture.subscribe("cam")
	event := fixture.enqueue("crew-1")

	fixture.runAttempt()
	fixture.runAttempt()
	if a, b, c := fixture.push.count(ari), fixture.push.count(bea), fixture.push.count(cam); a != 1 || b != 1 || c != 2 {
		t.Fatalf("requests ari=%d bea=%d cam=%d, want 1/1/2: only the transient failure is retried", a, b, c)
	}
	if status, attempts, _ := fixture.event(event); status != "DELIVERED" || attempts != 2 {
		t.Fatalf("digest is %s at attempt %d, want DELIVERED at 2", status, attempts)
	}
}

// The ledger is per event, not per device. last_success_at cannot do this job:
// a member of two crews is owed two different digests on the same day, and
// delivering one must not suppress the other.
func TestCrewDigestLedgerIsPerEvent(t *testing.T) {
	fixture := newDigestFixture(t, func(string, int) (int, error) { return http.StatusCreated, nil })
	ari := fixture.subscribe("ari")
	dee := fixture.subscribe("dee")
	first := fixture.enqueue("crew-1")
	second := fixture.enqueue("crew-2")

	fixture.runAttempt()
	if got := fixture.push.count(ari); got != 2 {
		t.Fatalf("Ari is in two crews and got %d digests, want one from each", got)
	}
	if got := fixture.push.count(dee); got != 1 {
		t.Fatalf("Dee got %d digests, want 1", got)
	}
	for _, event := range []int64{first, second} {
		if status, _, _ := fixture.event(event); status != "DELIVERED" {
			t.Fatalf("event %d is %s", event, status)
		}
		if fixture.ledger(event)[ari] != "DELIVERED 201" {
			t.Fatalf("event %d did not settle Ari's device: %v", event, fixture.ledger(event))
		}
	}
}

// The ledger's constraints are what make "settled once" true under concurrency
// and keep it from holding contradictory or orphaned rows.
func TestPushDeliveryLedgerConstraints(t *testing.T) {
	fixture := newDigestFixture(t, func(string, int) (int, error) { return http.StatusCreated, nil })
	endpoint := fixture.subscribe("ari")
	event := fixture.enqueue("crew-1")
	var subscription int64
	if err := fixture.database.QueryRow(`select id from push_subscriptions where endpoint = $1`, endpoint).Scan(&subscription); err != nil {
		t.Fatalf("load subscription: %v", err)
	}

	insert := func(eventID, subscriptionID int64, outcome string, status int) error {
		_, err := fixture.database.Exec(
			`insert into push_deliveries (event_id, subscription_id, outcome, status_code) values ($1, $2, $3, $4)`,
			eventID, subscriptionID, outcome, status)
		return err
	}
	if err := insert(event, subscription, "DELIVERED", 201); err != nil {
		t.Fatalf("a valid ledger row was refused: %v", err)
	}
	// Each probe targets a second, untouched event unless it is testing the
	// primary key, so it can only fail for the reason it names.
	other := fixture.enqueue("crew-2")
	for _, probe := range []struct {
		name     string
		eventID  int64
		subID    int64
		outcome  string
		status   int
		sqlstate string
	}{
		{"second settlement of one send", event, subscription, "REJECTED", 403, "23505"},
		{"delivered with a refusal code", other, subscription, "DELIVERED", 403, "23514"},
		{"rejected with a success code", other, subscription, "REJECTED", 201, "23514"},
		{"a 5xx is never settled", other, subscription, "REJECTED", 503, "23514"},
		{"unknown outcome", other, subscription, "MAYBE", 201, "23514"},
		{"unknown event", other + 1000, subscription, "DELIVERED", 201, "23503"},
		{"unknown subscription", other, subscription + 1000, "DELIVERED", 201, "23503"},
	} {
		err := insert(probe.eventID, probe.subID, probe.outcome, probe.status)
		var postgresError *pgconn.PgError
		if !errors.As(err, &postgresError) || postgresError.Code != probe.sqlstate {
			t.Errorf("%s: got %v, want SQLSTATE %s", probe.name, err, probe.sqlstate)
		}
	}

	// Settling the same send twice is a no-op, not an error: a replayed
	// success must not fail the delivery that just worked.
	if err := fixture.store.SettleDelivery(fixture.ctx, event, subscription, 201, time.Now()); err != nil {
		t.Fatalf("re-settling a delivered send failed: %v", err)
	}

	// Forgetting a subscription forgets its ledger rows, so the endpoint is not
	// kept alive anywhere after an unsubscribe.
	if err := fixture.store.DeleteSubscription(fixture.ctx, endpoint); err != nil {
		t.Fatalf("unsubscribe: %v", err)
	}
	if got := fixture.ledger(event); len(got) != 0 {
		t.Fatalf("unsubscribe left ledger rows: %v", got)
	}
	var orphans int
	if err := fixture.database.QueryRow(`select count(*) from push_deliveries where subscription_id = $1`, subscription).Scan(&orphans); err != nil {
		t.Fatalf("count orphans: %v", err)
	}
	if orphans != 0 {
		t.Fatalf("unsubscribe left %d ledger rows", orphans)
	}

	// A send that races an unsubscribe records nothing rather than failing.
	if err := fixture.store.SettleDelivery(fixture.ctx, event, subscription, 201, time.Now()); err != nil {
		t.Fatalf("settling a send for a just-removed subscription failed: %v", err)
	}
}
