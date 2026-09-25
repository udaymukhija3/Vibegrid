package vibegrid

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// withReminders switches on the reminder surface for hostileServer. The VAPID
// values only need to be present; nothing in these tests signs a real push.
func withReminders(config *ServerConfig, database *sql.DB) {
	config.PushSubscriptions = NewPostgresPushSubscriptionStore(database)
	config.VAPIDKeys = VAPIDKeys{Public: "test-public", Private: "test-private", Subject: "mailto:ops@example.test"}
}

const testPushSubscribeBody = `{"endpoint":"https://push.example.test/send/abc123","keys":{"p256dh":"BNcRdreALRFXTkOOUHK1EtK2wtaz5Ry4YfYCA_0QTpQtUbVlUls0VJXg7A8u-Ts1XbjhazAkj7I99e8QcYP7DkM","auth":"tBHItJI5svbpez7KI4CCXg"}}`

// A reminder is addressed by joining push_subscriptions.session_id against
// crew_members.session_id. Since migration 00021 that column holds the stable
// anonymous identity, not the cookie. A subscription stored under the raw bearer
// would put a plaintext credential in the table and, for every browser minted
// after that migration, would never match a membership, so the reminder would
// silently never arrive.
func TestPushSubscriptionIsKeyedByStableIdentityNotBearer(t *testing.T) {
	handler, database := hostileServer(t, withReminders)
	cookie := newCrewSession(t, handler)
	crew := createCrew(t, handler, cookie, "Reminded", "Ari")
	crewID := crewInternalID(t, database, crew.InviteCode)

	response := rawRequest(t, handler, http.MethodPost, "/api/push/subscribe", "application/json", []byte(testPushSubscribeBody), cookie)
	if response.Code != http.StatusCreated {
		t.Fatalf("subscribe: %d %s", response.Code, response.Body.String())
	}

	var stored, member string
	if err := database.QueryRow(`select session_id from push_subscriptions`).Scan(&stored); err != nil {
		t.Fatalf("load subscription: %v", err)
	}
	if err := database.QueryRow(`select session_id from crew_members where crew_id = $1`, crewID).Scan(&member); err != nil {
		t.Fatalf("load membership: %v", err)
	}
	if stored == cookie.Value {
		t.Fatal("push subscription stored the plaintext browser bearer")
	}
	if stored != member {
		t.Fatalf("subscription identity %q does not match the crew membership identity %q", stored, member)
	}

	store := NewPostgresPushSubscriptionStore(database)
	assertDigestReaches(t, store, crewID, member)

	// Replacing a leaked browser key keeps the identity, so reminders must stay
	// attached without the member doing anything.
	replacement := strings.Repeat("e", 32)
	rotated := rawRequest(t, handler, http.MethodPost, "/api/session/rotate", "application/json",
		[]byte(`{"replacementToken":"`+replacement+`"}`), cookie)
	if rotated.Code != http.StatusOK {
		t.Fatalf("rotate browser key: %d %s", rotated.Code, rotated.Body.String())
	}
	assertDigestReaches(t, store, crewID, member)
}

func assertDigestReaches(t *testing.T, store *PostgresPushSubscriptionStore, crewID, identity string) {
	t.Helper()
	ctx := context.Background()
	crews, err := store.CrewsAwaitingDigest(ctx)
	if err != nil {
		t.Fatalf("crews awaiting digest: %v", err)
	}
	if !slices.Contains(crews, crewID) {
		t.Fatalf("crew %s is not queued for a digest; got %v", crewID, crews)
	}
	subscriptions, err := store.SubscriptionsForSessions(ctx, []string{identity})
	if err != nil {
		t.Fatalf("subscriptions for member: %v", err)
	}
	if len(subscriptions[identity]) != 1 {
		t.Fatalf("member identity resolves to %d subscriptions, want 1", len(subscriptions[identity]))
	}
}

// Reminder writes are cookie-authenticated like crew writes, so they carry the
// same explicit CSRF requirement on top of JSON/CORS framing.
func TestPushWritesRequireExplicitCSRFToken(t *testing.T) {
	handler, database := hostileServer(t, withReminders)
	cookie := newCrewSession(t, handler)

	for _, path := range []string{"/api/push/subscribe", "/api/push/unsubscribe"} {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(testPushSubscribeBody))
		request.Header.Set("Content-Type", "application/json")
		request.AddCookie(cookie)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s without CSRF token returned %d: %s", path, response.Code, response.Body.String())
		}
	}

	var subscriptions int
	if err := database.QueryRow(`select count(*) from push_subscriptions`).Scan(&subscriptions); err != nil {
		t.Fatalf("count subscriptions: %v", err)
	}
	if subscriptions != 0 {
		t.Fatalf("a CSRF-rejected subscribe stored %d subscriptions", subscriptions)
	}
}

func TestPushRoutesAreNeverCached(t *testing.T) {
	handler, _ := hostileServer(t, withReminders)
	response := rawRequest(t, handler, http.MethodGet, "/api/push/config", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("push config: %d %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("push config Cache-Control = %q", got)
	}
	if !strings.Contains(response.Body.String(), `"enabled":true`) {
		t.Fatalf("push config should report reminders enabled: %s", response.Body.String())
	}
}
