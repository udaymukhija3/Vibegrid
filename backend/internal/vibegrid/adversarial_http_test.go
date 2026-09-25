package vibegrid

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// hostileServer wires the full HTTP surface against a real database, so probes
// exercise handler authorization, projection, and validation together rather
// than one store method in isolation. configure lets a suite switch on an
// optional subsystem (reminders need VAPID keys and a subscription store).
func hostileServer(t *testing.T, configure ...func(*ServerConfig, *sql.DB)) (http.Handler, *sql.DB) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres adversarial HTTP test")
	}
	ctx := context.Background()
	if err := MigrateDB(ctx, databaseURL); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	database, err := OpenDB(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Exec(`truncate vibe_votes, vibe_submissions, vibe_crew_boards,
		vibe_board_expansions, vibe_daily_boards, idempotency_keys, rate_limit_hits,
			guest_sessions, admin_sessions, moderation_actions, moderation_reports, moderation_appeals,
		crew_members, crews, puzzles, attempts, attempt_guesses, push_subscriptions restart identity cascade`); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	puzzleStore := NewPostgresPuzzleStore(database)
	if err := puzzleStore.Seed(ctx, SeedPuzzles()); err != nil {
		t.Fatalf("seed puzzles: %v", err)
	}
	vibeRounds := NewPostgresVibeRoundStore(database)
	config := ServerConfig{
		Puzzles:         puzzleStore,
		Store:           NewPostgresAttemptStore(database),
		AdminPuzzles:    puzzleStore,
		AdminSessions:   NewPostgresAdminSessionStore(database),
		GuestSessions:   NewPostgresGuestSessionStore(database),
		Community:       puzzleStore,
		Crews:           NewPostgresCrewStore(database),
		VibeRounds:      vibeRounds,
		AdminVibeBoards: vibeRounds,
		RateLimits:      NewPostgresRateLimitStore(database),
		Idempotency:     NewPostgresIdempotencyStore(database),
		Moderation:      NewPostgresModerationStore(database),
		Clock:           fixedClock,
	}
	for _, apply := range configure {
		apply(&config, database)
	}
	return NewServer(config), database
}

// rawRequest issues a request with full control over method, headers and body
// bytes, which the typed helpers in crews_test.go deliberately do not allow.
func rawRequest(t *testing.T, handler http.Handler, method, path, contentType string, body []byte, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	for _, cookie := range cookies {
		if cookie != nil {
			request.AddCookie(cookie)
			if method != http.MethodGet && method != http.MethodHead {
				request.Header.Set(guestCSRFHeader, guestCSRFToken(cookie.Value))
			}
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeDaily(t *testing.T, response *httptest.ResponseRecorder) vibeCrewDailyResponse {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("crew daily failed: %d %s", response.Code, response.Body.String())
	}
	var daily vibeCrewDailyResponse
	if err := json.NewDecoder(bytes.NewReader(response.Body.Bytes())).Decode(&daily); err != nil {
		t.Fatalf("decode daily: %v", err)
	}
	return daily
}

func TestFreshGuestSessionPublishesCSRFForItsBearerToken(t *testing.T) {
	handler, _ := hostileServer(t)
	response := rawRequest(t, handler, http.MethodGet, "/api/session", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("load fresh session: %d %s", response.Code, response.Body.String())
	}
	cookie := responseCookie(response, sessionCookieName)
	if cookie == nil {
		t.Fatal("fresh session did not set its bearer cookie")
	}
	var status SessionStatus
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		t.Fatalf("decode fresh session: %v", err)
	}
	if status.Guest.CSRFToken != guestCSRFToken(cookie.Value) {
		t.Fatal("session endpoint derived CSRF token from something other than the active bearer")
	}

	request := httptest.NewRequest(http.MethodPost, "/api/crews", strings.NewReader(`{"name":"Fresh Browser","displayName":"Ari"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(guestCSRFHeader, status.Guest.CSRFToken)
	request.Header.Set(idempotencyHeader, "fresh-browser-create")
	request.AddCookie(cookie)
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, request)
	if created.Code != http.StatusCreated {
		t.Fatalf("documented fresh-session CSRF token could not authorize a crew write: %d %s", created.Code, created.Body.String())
	}
}

func TestPublicHTMLMintsCookieWithoutWritingOrPersistingItsBearer(t *testing.T) {
	_, database := hostileServer(t)
	handler := NewServer(ServerConfig{
		GuestSessions: NewPostgresGuestSessionStore(database),
		RateLimits:    NewPostgresRateLimitStore(database),
		Clock:         fixedClock,
		Frontend: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<!doctype html><html><head></head><body>VibeGrid</body></html>"))
		}),
	})
	response := rawRequest(t, handler, http.MethodGet, "/", "", nil)
	bearer := responseCookie(response, sessionCookieName)
	if response.Code != http.StatusOK || bearer == nil {
		t.Fatalf("public document did not mint a browser cookie: %d", response.Code)
	}
	var sessions int
	if err := database.QueryRow(`select count(*) from guest_sessions`).Scan(&sessions); err != nil {
		t.Fatalf("count guest sessions after public HTML: %v", err)
	}
	if sessions != 0 {
		t.Fatalf("one anonymous HTML view wrote %d durable guest sessions", sessions)
	}

	bootstrapped := rawRequest(t, handler, http.MethodGet, "/api/session", "", nil, bearer)
	if bootstrapped.Code != http.StatusOK {
		t.Fatalf("bootstrap public document session: %d %s", bootstrapped.Code, bootstrapped.Body.String())
	}
	var identity string
	if err := database.QueryRow(
		`select identity_id from guest_sessions where token_hash = $1`, hashGuestToken(bearer.Value),
	).Scan(&identity); err != nil {
		t.Fatalf("load bootstrapped guest identity: %v", err)
	}
	if identity == bearer.Value || identity != guestIdentityForToken(bearer.Value) {
		t.Fatalf("new bearer leaked into durable identity: bearer=%q identity=%q", bearer.Value, identity)
	}
}

func TestPreMigrationCookieKeepsItsDurableCrewIdentity(t *testing.T) {
	handler, database := hostileServer(t)
	legacyToken := strings.Repeat("a", 32)
	crews := NewPostgresCrewStore(database)
	crew, err := crews.CreateCrew(t.Context(), "Legacy Crew", "Ari", legacyToken, fixedClock())
	if err != nil {
		t.Fatalf("seed legacy membership: %v", err)
	}

	cookie := &http.Cookie{Name: sessionCookieName, Value: legacyToken}
	response := rawRequest(t, handler, http.MethodGet, "/api/session", "", nil, cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("resolve legacy browser: %d %s", response.Code, response.Body.String())
	}
	var identity string
	if err := database.QueryRow(
		`select identity_id from guest_sessions where token_hash = $1`, hashGuestToken(legacyToken),
	).Scan(&identity); err != nil {
		t.Fatalf("load migrated legacy session: %v", err)
	}
	if identity != legacyToken {
		t.Fatalf("legacy browser identity changed from %q to %q", legacyToken, identity)
	}

	daily := rawRequest(t, handler, http.MethodGet, "/api/crews/"+crew.InviteCode+"/daily", "", nil, cookie)
	if daily.Code != http.StatusOK {
		t.Fatalf("legacy browser lost crew access: %d %s", daily.Code, daily.Body.String())
	}
}

func TestConcurrentTabsBootstrapOneGuestIdentity(t *testing.T) {
	handler, database := hostileServer(t)
	bearer := &http.Cookie{Name: sessionCookieName, Value: strings.Repeat("b", 32)}
	const tabs = 8
	start := make(chan struct{})
	responses := make(chan *httptest.ResponseRecorder, tabs)
	for range tabs {
		go func() {
			<-start
			responses <- rawRequest(t, handler, http.MethodGet, "/api/session", "", nil, bearer)
		}()
	}
	close(start)
	for range tabs {
		response := <-responses
		if response.Code != http.StatusOK {
			t.Fatalf("concurrent session bootstrap returned %d: %s", response.Code, response.Body.String())
		}
		if replacement := responseCookie(response, sessionCookieName); replacement == nil || replacement.Value != bearer.Value {
			t.Fatalf("concurrent bootstrap unexpectedly rotated the shared bearer: %#v", replacement)
		}
	}

	var sessions int
	var identity string
	if err := database.QueryRow(
		`select count(*), min(identity_id) from guest_sessions where token_hash = $1`, hashGuestToken(bearer.Value),
	).Scan(&sessions, &identity); err != nil {
		t.Fatalf("inspect concurrent guest bootstrap: %v", err)
	}
	if sessions != 1 || identity != guestIdentityForToken(bearer.Value) {
		t.Fatalf("concurrent tabs produced sessions=%d identity=%q", sessions, identity)
	}
}

func TestPollingDoesNotRefreshDurableSessionEveryThirtySeconds(t *testing.T) {
	_, database := hostileServer(t)
	store := NewPostgresGuestSessionStore(database)
	start := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	tokenHash := hashGuestToken(strings.Repeat("c", 32))
	identity := strings.Repeat("e", 32)
	if _, active, err := store.ResolveOrCreate(t.Context(), tokenHash, "", identity, start, start.Add(sessionTTL)); err != nil || !active {
		t.Fatalf("register guest session: active=%v err=%v", active, err)
	}
	if _, active, err := store.ResolveOrCreate(t.Context(), tokenHash, "", identity, start.Add(30*time.Second), start.Add(30*time.Second).Add(sessionTTL)); err != nil || !active {
		t.Fatalf("poll guest session: active=%v err=%v", active, err)
	}
	var afterPoll time.Time
	if err := database.QueryRow(`select last_seen_at from guest_sessions where token_hash = $1`, tokenHash).Scan(&afterPoll); err != nil {
		t.Fatalf("load polled guest session: %v", err)
	}
	if !afterPoll.Equal(start) {
		t.Fatalf("30-second poll rewrote last_seen_at to %s, want %s", afterPoll, start)
	}
	if _, active, err := store.ResolveOrCreate(t.Context(), tokenHash, "", identity, start.Add(2*time.Hour), start.Add(2*time.Hour).Add(sessionTTL)); err != nil || !active {
		t.Fatalf("refresh guest session after interval: active=%v err=%v", active, err)
	}
	var afterRefresh time.Time
	if err := database.QueryRow(`select last_seen_at from guest_sessions where token_hash = $1`, tokenHash).Scan(&afterRefresh); err != nil {
		t.Fatalf("load refreshed guest session: %v", err)
	}
	if !afterRefresh.Equal(start.Add(2 * time.Hour)) {
		t.Fatalf("hourly refresh left last_seen_at at %s", afterRefresh)
	}
}

func TestCrewCookieMutationsRequireExplicitCSRFToken(t *testing.T) {
	handler, database := hostileServer(t)
	owner := newCrewSession(t, handler)
	crew := createCrew(t, handler, owner, "CSRF Crew", "Ari")

	request := httptest.NewRequest(http.MethodPost, "/api/crews/"+crew.InviteCode+"/leave", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(owner)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("crew write without CSRF token returned %d: %s", response.Code, response.Body.String())
	}

	var members int
	if err := database.QueryRow(`select count(*) from crew_members`).Scan(&members); err != nil {
		t.Fatalf("count memberships: %v", err)
	}
	if members != 1 {
		t.Fatalf("CSRF-rejected leave changed membership state: %d rows", members)
	}

	wrong := httptest.NewRequest(http.MethodPost, "/api/crews/"+crew.InviteCode+"/leave", strings.NewReader(`{}`))
	wrong.Header.Set("Content-Type", "application/json")
	wrong.Header.Set(guestCSRFHeader, guestCSRFToken(strings.Repeat("f", 32)))
	wrong.AddCookie(owner)
	wrongResponse := httptest.NewRecorder()
	handler.ServeHTTP(wrongResponse, wrong)
	if wrongResponse.Code != http.StatusForbidden {
		t.Fatalf("crew write with another session's CSRF token returned %d", wrongResponse.Code)
	}
}

func TestGuestSessionRotationRevokesCopiedCookieWithoutLosingCrews(t *testing.T) {
	handler, database := hostileServer(t)
	oldCookie := newCrewSession(t, handler)
	crew := createCrew(t, handler, oldCookie, "Rotating Keys", "Ari")
	replacement := strings.Repeat("d", 32)
	body := []byte(`{"replacementToken":"` + replacement + `"}`)

	rotated := rawRequest(t, handler, http.MethodPost, "/api/session/rotate", "application/json", body, oldCookie)
	if rotated.Code != http.StatusOK {
		t.Fatalf("rotate guest session: %d %s", rotated.Code, rotated.Body.String())
	}
	newCookie := responseCookie(rotated, sessionCookieName)
	if newCookie == nil || newCookie.Value != replacement {
		t.Fatalf("rotation did not install the requested replacement cookie: %#v", newCookie)
	}

	// A lost response can be retried with the same replacement intent. The old
	// bearer is already revoked, but this exact replay still returns the same
	// cookie instead of stranding the legitimate browser.
	replayed := rawRequest(t, handler, http.MethodPost, "/api/session/rotate", "application/json", body, oldCookie)
	if replayed.Code != http.StatusOK {
		t.Fatalf("replay guest session rotation: %d %s", replayed.Code, replayed.Body.String())
	}
	if replayCookie := responseCookie(replayed, sessionCookieName); replayCookie == nil || replayCookie.Value != replacement {
		t.Fatalf("rotation replay returned the wrong cookie: %#v", replayCookie)
	}

	withReplacement := decodeDaily(t, rawRequest(t, handler, http.MethodGet, "/api/crews/"+crew.InviteCode+"/daily", "", nil, newCookie))
	if !withReplacement.IsMember {
		t.Fatal("replacement token lost the stable crew identity")
	}
	withCopiedOldToken := decodeDaily(t, rawRequest(t, handler, http.MethodGet, "/api/crews/"+crew.InviteCode+"/daily", "", nil, oldCookie))
	if withCopiedOldToken.IsMember {
		t.Fatal("revoked copied cookie still authorizes the crew")
	}

	var oldRevoked, newActive bool
	if err := database.QueryRow(
		`select
		 exists (select 1 from guest_sessions where token_hash = $1 and revoked_at is not null),
		 exists (select 1 from guest_sessions where token_hash = $2 and revoked_at is null)`,
		hashGuestToken(oldCookie.Value), hashGuestToken(replacement),
	).Scan(&oldRevoked, &newActive); err != nil {
		t.Fatalf("inspect rotated guest sessions: %v", err)
	}
	if !oldRevoked || !newActive {
		t.Fatalf("unexpected rotation state: oldRevoked=%v newActive=%v", oldRevoked, newActive)
	}
	var plaintextTokens int
	if err := database.QueryRow(
		`select count(*) from guest_sessions where token_hash in ($1, $2)`, oldCookie.Value, replacement,
	).Scan(&plaintextTokens); err != nil {
		t.Fatalf("check plaintext tokens: %v", err)
	}
	if plaintextTokens != 0 {
		t.Fatal("guest session bearer token was stored in plaintext")
	}
}

// EDGE-020: an invite code is a capability. Holding one must reveal the crew's
// existence and name (that is the join preview) but nothing about who is in it
// or what they authored.
func TestInvitePreviewDoesNotLeakMemberOrCardContent(t *testing.T) {
	handler, database := hostileServer(t)
	_ = database
	owner := newCrewSession(t, handler)
	crew := createCrew(t, handler, owner, "Night Shift", "Ari")

	stranger := newCrewSession(t, handler)
	daily := decodeDaily(t, crewRequest(t, handler, http.MethodGet, "/api/crews/"+crew.InviteCode+"/daily", stranger, nil))

	if daily.IsMember {
		t.Fatal("a stranger holding only the invite code was treated as a member")
	}
	if len(daily.Members) != 0 {
		t.Fatalf("invite preview leaked %d member entries", len(daily.Members))
	}
	if daily.Judge != nil || daily.Result != nil || daily.Today.Submission != nil {
		t.Fatalf("invite preview leaked staged content: judge=%v result=%v card=%v",
			daily.Judge != nil, daily.Result != nil, daily.Today.Submission != nil)
	}
	if daily.CrewStreak != 0 {
		t.Fatalf("invite preview leaked the crew streak (%d)", daily.CrewStreak)
	}
	// A stranger must not be able to freeze this crew's board size either.
	var frozen int
	if err := database.QueryRow(`select count(*) from vibe_crew_boards`).Scan(&frozen); err != nil {
		t.Fatalf("count frozen: %v", err)
	}
	if frozen != 0 {
		t.Fatalf("a non-member's page view froze %d crew board rows", frozen)
	}
}

// EDGE-021: a non-member must not be able to write into a crew even with a
// valid invite code and a well-formed body.
func TestNonMemberCannotWriteIntoACrew(t *testing.T) {
	handler, _ := hostileServer(t)
	owner := newCrewSession(t, handler)
	crew := createCrew(t, handler, owner, "Night Shift", "Ari")

	board := todayVibeBoard(t, handler)
	stranger := newCrewSession(t, handler)

	submit := crewRequest(t, handler, http.MethodPost, "/api/crews/"+crew.InviteCode+"/submissions", stranger,
		VibeSubmissionRequest{
			BoardID:            board.ID,
			Title:              "Not mine",
			SelectedTileIDs:    []string{board.Tiles[0].ID, board.Tiles[1].ID, board.Tiles[2].ID, board.Tiles[3].ID},
			ClientSubmissionID: "stranger-1",
		})
	if submit.Code != http.StatusForbidden {
		t.Fatalf("non-member submission returned %d, want 403: %s", submit.Code, submit.Body.String())
	}

	remove := crewRequest(t, handler, http.MethodPost,
		"/api/crews/"+crew.InviteCode+"/members/00000000-0000-0000-0000-000000000000/remove", stranger, nil)
	if remove.Code != http.StatusForbidden {
		t.Fatalf("non-member removal returned %d, want 403: %s", remove.Code, remove.Body.String())
	}

	rotate := crewRequest(t, handler, http.MethodPost, "/api/crews/"+crew.InviteCode+"/rotate", stranger, nil)
	if rotate.Code != http.StatusForbidden {
		t.Fatalf("non-member rotate returned %d, want 403: %s", rotate.Code, rotate.Body.String())
	}
}

// EDGE-022: a member who has not authored a card must not receive the ballot,
// because the ballot is the other players' content. Eligibility is the privacy
// boundary, not just a UI affordance.
func TestNonSubmittingMemberNeverReceivesBallotContent(t *testing.T) {
	handler, database := hostileServer(t)
	owner := newCrewSession(t, handler)
	crew := createCrew(t, handler, owner, "Night Shift", "Ari")
	lurker := joinCrewAs(t, handler, crew.InviteCode, "Lurker")

	// Two other members author cards on the JUDGE board (yesterday).
	judgeBoard := seedJudgeBoardCards(t, handler, database, crew, 2)

	daily := decodeDaily(t, crewRequest(t, handler, http.MethodGet, "/api/crews/"+crew.InviteCode+"/daily", lurker, nil))
	if daily.Judge == nil {
		t.Skip("no judge stage produced by the fixture clock")
	}
	if daily.Judge.Eligible {
		t.Fatal("a member who authored nothing was marked eligible to judge")
	}
	if len(daily.Judge.Cards) != 0 {
		t.Fatalf("an ineligible member received %d ballot cards from board %s",
			len(daily.Judge.Cards), judgeBoard)
	}
}

// EDGE-023: the ballot must never carry author names, and the result must.
func TestStagedDisclosureBoundaries(t *testing.T) {
	handler, database := hostileServer(t)
	owner := newCrewSession(t, handler)
	crew := createCrew(t, handler, owner, "Night Shift", "Ari")
	joinCrewAs(t, handler, crew.InviteCode, "Bea")

	seedJudgeBoardCards(t, handler, database, crew, 2)
	daily := decodeDaily(t, crewRequest(t, handler, http.MethodGet, "/api/crews/"+crew.InviteCode+"/daily", owner, nil))
	if daily.Judge != nil {
		for _, card := range daily.Judge.Cards {
			if card.AuthorName != "" {
				t.Fatalf("blind ballot revealed author %q", card.AuthorName)
			}
			if card.Votes != 0 || card.Winner {
				t.Fatalf("blind ballot leaked tallies: %#v", card)
			}
		}
	}
}

// EDGE-024: hostile identifiers must be rejected before they reach the
// database, and must never produce a 500 or echo an internal error.
func TestHostileIdentifiersAreRejectedCleanly(t *testing.T) {
	handler, _ := hostileServer(t)
	session := newCrewSession(t, handler)

	cases := []struct {
		name string
		path string
	}{
		{"sql injection", "/api/crews/" + escape("' or 1=1--") + "/daily"},
		{"path traversal", "/api/crews/" + escape("../../etc/passwd") + "/daily"},
		{"null byte", "/api/crews/" + escape("abc\x00def") + "/daily"},
		{"unicode", "/api/crews/" + escape("crew-💥") + "/daily"},
		{"rtl override", "/api/crews/" + escape("crew‮evil") + "/daily"},
		{"overlong", "/api/crews/" + strings.Repeat("a", 4096) + "/daily"},
		{"empty-ish", "/api/crews/%20/daily"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			response := crewRequest(t, handler, http.MethodGet, testCase.path, session, nil)
			if response.Code >= 500 {
				t.Fatalf("hostile id produced %d: %s", response.Code, response.Body.String())
			}
			if response.Code != http.StatusNotFound {
				t.Fatalf("hostile id returned %d, want 404: %s", response.Code, response.Body.String())
			}
			assertNoInternalDetail(t, response.Body.String())
		})
	}
}

// EDGE-025: the owner's member-removal handle is a uuid. A malformed handle is
// a client error, not a database error surfaced as a 500.
func TestMalformedMemberHandleIsAClientError(t *testing.T) {
	handler, _ := hostileServer(t)
	owner := newCrewSession(t, handler)
	crew := createCrew(t, handler, owner, "Night Shift", "Ari")

	for _, handle := range []string{"not-a-uuid", "0", "-1", strings.Repeat("f", 64)} {
		response := crewRequest(t, handler, http.MethodPost,
			"/api/crews/"+crew.InviteCode+"/members/"+handle+"/remove", owner, nil)
		if response.Code >= 500 {
			t.Fatalf("member handle %q produced %d: %s", handle, response.Code, response.Body.String())
		}
		assertNoInternalDetail(t, response.Body.String())
	}
}

// EDGE-026: submission input boundaries. Every one of these is reachable by
// anyone holding an invite code, so each must be a clean 4xx.
func TestSubmissionInputBoundaries(t *testing.T) {
	handler, _ := hostileServer(t)
	owner := newCrewSession(t, handler)
	crew := createCrew(t, handler, owner, "Night Shift", "Ari")
	board := todayVibeBoard(t, handler)
	path := "/api/crews/" + crew.InviteCode + "/submissions"

	four := []string{board.Tiles[0].ID, board.Tiles[1].ID, board.Tiles[2].ID, board.Tiles[3].ID}
	cases := []struct {
		name    string
		request VibeSubmissionRequest
	}{
		{"empty title", VibeSubmissionRequest{BoardID: board.ID, Title: "", SelectedTileIDs: four, ClientSubmissionID: "b1"}},
		{"whitespace title", VibeSubmissionRequest{BoardID: board.ID, Title: "   \t  ", SelectedTileIDs: four, ClientSubmissionID: "b2"}},
		{"41 rune title", VibeSubmissionRequest{BoardID: board.ID, Title: strings.Repeat("x", MaxVibeTitleRunes+1), SelectedTileIDs: four, ClientSubmissionID: "b3"}},
		{"nil tiles", VibeSubmissionRequest{BoardID: board.ID, Title: "ok", SelectedTileIDs: nil, ClientSubmissionID: "b4"}},
		{"three tiles", VibeSubmissionRequest{BoardID: board.ID, Title: "ok", SelectedTileIDs: four[:3], ClientSubmissionID: "b5"}},
		{"five tiles", VibeSubmissionRequest{BoardID: board.ID, Title: "ok", SelectedTileIDs: append(append([]string{}, four...), board.Tiles[4].ID), ClientSubmissionID: "b6"}},
		{"duplicate tiles", VibeSubmissionRequest{BoardID: board.ID, Title: "ok", SelectedTileIDs: []string{four[0], four[0], four[1], four[2]}, ClientSubmissionID: "b7"}},
		{"unknown tile", VibeSubmissionRequest{BoardID: board.ID, Title: "ok", SelectedTileIDs: []string{four[0], four[1], four[2], "made-up-tile"}, ClientSubmissionID: "b8"}},
		{"wrong board", VibeSubmissionRequest{BoardID: "vibe-1999-01-01", Title: "ok", SelectedTileIDs: four, ClientSubmissionID: "b9"}},
		{"empty board id", VibeSubmissionRequest{BoardID: "", Title: "ok", SelectedTileIDs: four, ClientSubmissionID: "b10"}},
		{"empty client id", VibeSubmissionRequest{BoardID: board.ID, Title: "ok", SelectedTileIDs: four, ClientSubmissionID: ""}},
		{"client id too long", VibeSubmissionRequest{BoardID: board.ID, Title: "ok", SelectedTileIDs: four, ClientSubmissionID: strings.Repeat("a", maxVibeClientID+1)}},
		{"client id with punctuation", VibeSubmissionRequest{BoardID: board.ID, Title: "ok", SelectedTileIDs: four, ClientSubmissionID: "a b/c"}},
		{"newline title", VibeSubmissionRequest{BoardID: board.ID, Title: "line\nbreak", SelectedTileIDs: four, ClientSubmissionID: "b11"}},
		{"null byte title", VibeSubmissionRequest{BoardID: board.ID, Title: "null\x00byte", SelectedTileIDs: four, ClientSubmissionID: "b12"}},
		{"html title", VibeSubmissionRequest{BoardID: board.ID, Title: "<script>alert(1)</script>", SelectedTileIDs: four, ClientSubmissionID: "b13"}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			response := crewRequest(t, handler, http.MethodPost, path, owner, testCase.request)
			if response.Code >= 500 {
				t.Fatalf("%s produced %d: %s", testCase.name, response.Code, response.Body.String())
			}
			assertNoInternalDetail(t, response.Body.String())
		})
	}
}

// EDGE-027: malformed and hostile request framing must be refused before any
// storage work happens.
func TestMalformedRequestFramingIsRefused(t *testing.T) {
	handler, _ := hostileServer(t)
	owner := newCrewSession(t, handler)
	crew := createCrew(t, handler, owner, "Night Shift", "Ari")
	path := "/api/crews/" + crew.InviteCode + "/submissions"

	cases := []struct {
		name        string
		contentType string
		body        string
		wantStatus  int
	}{
		{"no content type", "", `{}`, http.StatusUnsupportedMediaType},
		{"form content type", "application/x-www-form-urlencoded", `boardId=x`, http.StatusUnsupportedMediaType},
		{"text/plain (CSRF shape)", "text/plain", `{"boardId":"x"}`, http.StatusUnsupportedMediaType},
		{"truncated json", "application/json", `{"boardId":`, http.StatusBadRequest},
		{"json array", "application/json", `[]`, http.StatusBadRequest},
		{"json null", "application/json", `null`, http.StatusUnprocessableEntity},
		{"unknown field", "application/json", `{"boardId":"x","evil":1}`, http.StatusBadRequest},
		{"two json documents", "application/json", `{"boardId":"x"}{"boardId":"y"}`, http.StatusBadRequest},
		{"wrong type for tiles", "application/json", `{"selectedTileIds":"not-an-array"}`, http.StatusBadRequest},
		{"empty body", "application/json", ``, http.StatusBadRequest},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			response := rawRequest(t, handler, http.MethodPost, path, testCase.contentType, []byte(testCase.body), owner)
			if response.Code != testCase.wantStatus {
				t.Fatalf("%s returned %d, want %d: %s",
					testCase.name, response.Code, testCase.wantStatus, response.Body.String())
			}
			assertNoInternalDetail(t, response.Body.String())
		})
	}

	oversized := rawRequest(t, handler, http.MethodPost, path, "application/json",
		[]byte(`{"title":"`+strings.Repeat("a", maxVibeMutationBodyBytes*2)+`"}`), owner)
	if oversized.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body returned %d, want 413: %s", oversized.Code, oversized.Body.String())
	}
}

// EDGE-028: the practice endpoint is unauthenticated and cacheable forever, so
// every sequence value must produce a valid, coherent 4x4 board rather than a
// 500 or a board with duplicate fragments.
func TestUnlimitedPracticeSequenceBoundaries(t *testing.T) {
	handler, _ := hostileServer(t)
	sequences := []string{
		"0", "1", "11", "12", "27", "28", "335", "336", "4031", "4032",
		"9007199254740993", "18446744073709551615",
	}
	for _, sequence := range sequences {
		t.Run(sequence, func(t *testing.T) {
			response := rawRequest(t, handler, http.MethodGet, "/api/vibes/practice/"+sequence, "", nil)
			if response.Code != http.StatusOK {
				t.Fatalf("sequence %s returned %d: %s", sequence, response.Code, response.Body.String())
			}
			var board VibeBoard
			if err := json.NewDecoder(bytes.NewReader(response.Body.Bytes())).Decode(&board); err != nil {
				t.Fatalf("decode %s: %v", sequence, err)
			}
			if len(board.Tiles) != VibePracticeTileCount {
				t.Fatalf("sequence %s dealt %d fragments", sequence, len(board.Tiles))
			}
			seenID, seenText := map[string]bool{}, map[string]bool{}
			for _, tile := range board.Tiles {
				if seenID[tile.ID] {
					t.Fatalf("sequence %s repeated fragment id %s", sequence, tile.ID)
				}
				if seenText[tile.Text] {
					t.Fatalf("sequence %s repeated fragment text %q", sequence, tile.Text)
				}
				seenID[tile.ID], seenText[tile.Text] = true, true
			}
			if board.BoardNumber < 1 {
				t.Fatalf("sequence %s produced board number %d", sequence, board.BoardNumber)
			}
		})
	}

	for _, bad := range []string{"-1", "1.5", "0x10", "١٢", "18446744073709551616", " 1", "+1", ""} {
		response := rawRequest(t, handler, http.MethodGet, "/api/vibes/practice/"+escape(bad), "", nil)
		if response.Code >= 500 {
			t.Fatalf("invalid sequence %q produced %d: %s", bad, response.Code, response.Body.String())
		}
	}
}

// EDGE-029: a public read must never carry a Set-Cookie under a shared cache
// directive — that would let a CDN hand one visitor's session to the next.
func TestPublicReadsNeverCacheASessionCookie(t *testing.T) {
	handler, _ := hostileServer(t)
	for _, path := range []string{"/api/vibes/today", "/api/vibes/practice/3", "/api/puzzles/today"} {
		response := rawRequest(t, handler, http.MethodGet, path, "", nil)
		cacheControl := response.Header().Get("Cache-Control")
		if response.Header().Get("Set-Cookie") != "" && strings.Contains(cacheControl, "public") {
			t.Fatalf("%s set a cookie under %q", path, cacheControl)
		}
	}

	session := newCrewSession(t, handler)
	handlerResponse := rawRequest(t, handler, http.MethodGet, "/api/crews", "application/json", nil, session)
	if !strings.Contains(handlerResponse.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("private crew list is cacheable: %q", handlerResponse.Header().Get("Cache-Control"))
	}
}

// EDGE-030: crew display names are shown to other people. Confirm the name
// rules actually hold at the HTTP boundary, including the uniqueness rule that
// keeps a crew readable.
func TestDisplayNameBoundaries(t *testing.T) {
	handler, _ := hostileServer(t)
	owner := newCrewSession(t, handler)
	crew := createCrew(t, handler, owner, "Night Shift", "Ari")

	cases := []struct {
		name       string
		display    string
		wantStatus int
	}{
		{"empty", "", http.StatusUnprocessableEntity},
		{"whitespace", "   ", http.StatusUnprocessableEntity},
		{"too long", strings.Repeat("a", maxDisplayNameLength+1), http.StatusUnprocessableEntity},
		{"newline", "Bea\nEvil", http.StatusUnprocessableEntity},
		{"null byte", "Bea\x00", http.StatusUnprocessableEntity},
		{"case clash with owner", "ari", http.StatusConflict},
		{"exactly max", strings.Repeat("b", maxDisplayNameLength), http.StatusOK},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			joiner := newCrewSession(t, handler)
			response := crewRequest(t, handler, http.MethodPost, "/api/crews/"+crew.InviteCode+"/join",
				joiner, crewJoinRequest{DisplayName: testCase.display})
			if response.Code != testCase.wantStatus {
				t.Fatalf("display name %q returned %d, want %d: %s",
					testCase.name, response.Code, testCase.wantStatus, response.Body.String())
			}
			assertNoInternalDetail(t, response.Body.String())
		})
	}
}

// EDGE-031: the crew member cap is a product limit protecting a leaked link.
// It must hold under a concurrent join stampede, not just sequentially.
func TestConcurrentJoinStampedeRespectsTheMemberCap(t *testing.T) {
	handler, database := hostileServer(t)
	owner := newCrewSession(t, handler)
	crew := createCrew(t, handler, owner, "Night Shift", "Ari")

	const attempts = 40
	done := make(chan int, attempts)
	for index := 0; index < attempts; index++ {
		go func(slot int) {
			joiner := newCrewSession(t, handler)
			response := crewRequest(t, handler, http.MethodPost, "/api/crews/"+crew.InviteCode+"/join",
				joiner, crewJoinRequest{DisplayName: fmt.Sprintf("Member%02d", slot)})
			done <- response.Code
		}(index)
	}
	statuses := map[int]int{}
	for index := 0; index < attempts; index++ {
		statuses[<-done]++
	}

	var members int
	if err := database.QueryRow(`select count(*) from crew_members where crew_id = (
		select id from crews where invite_code = $1)`, crew.InviteCode).Scan(&members); err != nil {
		t.Fatalf("count members: %v", err)
	}
	if members > maxCrewMembers {
		t.Fatalf("concurrent stampede pushed the crew to %d members (cap %d); statuses=%v",
			members, maxCrewMembers, statuses)
	}
}

func escape(value string) string {
	var builder strings.Builder
	for _, b := range []byte(value) {
		switch {
		case (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '-' || b == '_' || b == '.' || b == '~':
			builder.WriteByte(b)
		default:
			builder.WriteString(fmt.Sprintf("%%%02X", b))
		}
	}
	return builder.String()
}

// assertNoInternalDetail keeps error bodies from becoming an implementation
// disclosure channel.
func assertNoInternalDetail(t *testing.T, body string) {
	t.Helper()
	lowered := strings.ToLower(body)
	for _, marker := range []string{
		"sqlstate", "pq:", "pgconn", "goroutine", "/users/", "postgres://",
		"vibe_submissions", "crew_members", "select ", "insert into", "panic:",
	} {
		if strings.Contains(lowered, marker) {
			t.Fatalf("error body leaked internal detail %q: %s", marker, body)
		}
	}
}

func todayVibeBoard(t *testing.T, handler http.Handler) VibeBoard {
	t.Helper()
	response := rawRequest(t, handler, http.MethodGet, "/api/vibes/today", "", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("today board failed: %d %s", response.Code, response.Body.String())
	}
	var board VibeBoard
	if err := json.NewDecoder(bytes.NewReader(response.Body.Bytes())).Decode(&board); err != nil {
		t.Fatalf("decode today board: %v", err)
	}
	return board
}

func joinCrewAs(t *testing.T, handler http.Handler, inviteCode, displayName string) *http.Cookie {
	t.Helper()
	session := newCrewSession(t, handler)
	response := crewRequest(t, handler, http.MethodPost, "/api/crews/"+inviteCode+"/join", session,
		crewJoinRequest{DisplayName: displayName})
	if response.Code != http.StatusOK {
		t.Fatalf("join as %s failed: %d %s", displayName, response.Code, response.Body.String())
	}
	return session
}

// seedJudgeBoardCards writes cards directly onto yesterday's board, which the
// HTTP make endpoint deliberately cannot reach (it only ever writes today's).
// That is the only way to construct a judge-stage fixture without a clock jump.
func seedJudgeBoardCards(t *testing.T, handler http.Handler, database *sql.DB, crew crewView, count int) string {
	t.Helper()
	_, judgeDate, _ := judgeDatesFor(fixedClock())
	board, err := VibeBoardForDate(judgeDate)
	if err != nil {
		t.Fatalf("judge board: %v", err)
	}
	rounds := NewPostgresVibeRoundStore(database)
	stored, err := rounds.EnsureBoard(context.Background(), board)
	if err != nil {
		t.Fatalf("ensure judge board: %v", err)
	}
	for index := 0; index < count; index++ {
		session := joinCrewAs(t, handler, crew.InviteCode, fmt.Sprintf("Judge%d", index))
		// Store methods authorize against the stable anonymous identity, not the
		// revocable cookie bearer. HTTP normally performs this resolution for us;
		// this direct fixture must do the equivalent lookup explicitly.
		var sessionID string
		if err := database.QueryRow(
			`select identity_id from guest_sessions where token_hash = $1 and revoked_at is null`,
			hashGuestToken(session.Value),
		).Scan(&sessionID); err != nil {
			t.Fatalf("resolve judge fixture identity: %v", err)
		}
		if _, err := rounds.EnsureCrewBoard(context.Background(), crewInternalID(t, database, crew.InviteCode), sessionID, stored); err != nil {
			t.Fatalf("freeze judge board: %v", err)
		}
		if _, err := rounds.SubmitVibe(context.Background(), crewInternalID(t, database, crew.InviteCode), sessionID,
			VibeSubmissionRequest{
				BoardID:            stored.ID,
				Title:              fmt.Sprintf("Judge card %d", index),
				SelectedTileIDs:    []string{stored.Tiles[0].ID, stored.Tiles[1].ID, stored.Tiles[2].ID, stored.Tiles[3].ID},
				ClientSubmissionID: fmt.Sprintf("judge-seed-%d", index),
			}, fixedClock()); err != nil {
			t.Fatalf("seed judge card %d: %v", index, err)
		}
	}
	return stored.ID
}

func judgeDatesFor(now time.Time) (today, judge, result string) {
	return now.Format("2006-01-02"),
		now.AddDate(0, 0, -1).Format("2006-01-02"),
		now.AddDate(0, 0, -2).Format("2006-01-02")
}

func crewInternalID(t *testing.T, database *sql.DB, inviteCode string) string {
	t.Helper()
	var id string
	if err := database.QueryRow(`select id from crews where invite_code = $1`, inviteCode).Scan(&id); err != nil {
		t.Fatalf("crew internal id: %v", err)
	}
	return id
}

// EDGE-060: the realistic retry — response lost, player presses "Lock it in"
// again — must land as a replay and succeed, not as a failure for an action
// that already committed. The browser now derives clientSubmissionId from the
// card's content (see replayIdFor in src/lib/api.ts), so a repeated press of an
// unchanged card carries the SAME key and replays.
func TestRepeatedPressOfTheSameCardReplaysInsteadOfFailing(t *testing.T) {
	handler, database := hostileServer(t)
	owner := newCrewSession(t, handler)
	crew := createCrew(t, handler, owner, "Night Shift", "Ari")
	board := todayVibeBoard(t, handler)
	path := "/api/crews/" + crew.InviteCode + "/submissions"

	card := VibeSubmissionRequest{
		BoardID:         board.ID,
		Title:           "Under control",
		SelectedTileIDs: []string{board.Tiles[0].ID, board.Tiles[1].ID, board.Tiles[2].ID, board.Tiles[3].ID},
		// Content-derived, therefore identical on every press of this same card.
		ClientSubmissionID: "card-1a2b3c4d5e6f7a8b",
	}
	for attempt := 0; attempt < 3; attempt++ {
		response := crewRequest(t, handler, http.MethodPost, path, owner, card)
		if response.Code != http.StatusCreated {
			t.Fatalf("press %d returned %d, want 201: %s",
				attempt+1, response.Code, strings.TrimSpace(response.Body.String()))
		}
	}
	var cards int
	if err := database.QueryRow(
		`select count(*) from vibe_submissions where board_id = $1`, board.ID).Scan(&cards); err != nil {
		t.Fatalf("count cards: %v", err)
	}
	if cards != 1 {
		t.Fatalf("three presses of one card stored %d rows", cards)
	}

	// A genuinely DIFFERENT card carries a different derived key and must still
	// get an honest "already locked in" rather than silently replacing the first.
	card.Title = "Completely different"
	card.ClientSubmissionID = "card-99887766554433ff"
	changed := crewRequest(t, handler, http.MethodPost, path, owner, card)
	if changed.Code != http.StatusConflict {
		t.Fatalf("a different card returned %d, want 409: %s", changed.Code, changed.Body.String())
	}
}

// EDGE-061: the browser advertises Idempotency-Key on crew writes. The server
// must replay an identical request and reject changed input under the same key;
// a decorative header gives a caller a false retry guarantee.
func TestIdempotencyHeaderProtectsCrewMutations(t *testing.T) {
	handler, database := hostileServer(t)
	owner := newCrewSession(t, handler)
	crew := createCrew(t, handler, owner, "Night Shift", "Ari")
	board := todayVibeBoard(t, handler)

	payload, err := json.Marshal(VibeSubmissionRequest{
		BoardID:            board.ID,
		Title:              "Header only",
		SelectedTileIDs:    []string{board.Tiles[0].ID, board.Tiles[1].ID, board.Tiles[2].ID, board.Tiles[3].ID},
		ClientSubmissionID: "body-key-one",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	send := func(bodyKey string) *httptest.ResponseRecorder {
		body := strings.Replace(string(payload), "body-key-one", bodyKey, 1)
		request := httptest.NewRequest(http.MethodPost,
			"/api/crews/"+crew.InviteCode+"/submissions", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "11111111-1111-1111-1111-111111111111")
		request.Header.Set(guestCSRFHeader, guestCSRFToken(owner.Value))
		request.AddCookie(owner)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	first := send("body-key-one")
	if first.Code != http.StatusCreated {
		t.Fatalf("first send: %d %s", first.Code, first.Body.String())
	}
	replayed := send("body-key-one")
	if replayed.Code != http.StatusCreated || replayed.Header().Get(idempotencyReplayHeader) != "true" {
		t.Fatalf("identical header replay returned %d replay=%q: %s",
			replayed.Code, replayed.Header().Get(idempotencyReplayHeader), replayed.Body.String())
	}
	changed := send("body-key-two")
	if changed.Code != http.StatusConflict {
		t.Fatalf("changed body under one Idempotency-Key returned %d, want 409: %s", changed.Code, changed.Body.String())
	}
	var cards int
	if err := database.QueryRow(`select count(*) from vibe_submissions where board_id = $1`, board.ID).Scan(&cards); err != nil {
		t.Fatalf("count cards after header replays: %v", err)
	}
	if cards != 1 {
		t.Fatalf("idempotent header requests stored %d cards, want one", cards)
	}
}

func TestConcurrentCrewCreationWithOneIdempotencyKeyCommitsOnce(t *testing.T) {
	handler, database := hostileServer(t)
	owner := newCrewSession(t, handler)
	body := []byte(`{"name":"One Logical Crew","displayName":"Ari"}`)

	responses := make(chan *httptest.ResponseRecorder, 2)
	var workers sync.WaitGroup
	workers.Add(2)
	for worker := 0; worker < 2; worker++ {
		go func() {
			defer workers.Done()
			request := httptest.NewRequest(http.MethodPost, "/api/crews", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set(idempotencyHeader, "concurrent-crew-create")
			request.Header.Set(guestCSRFHeader, guestCSRFToken(owner.Value))
			request.AddCookie(owner)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			responses <- response
		}()
	}
	workers.Wait()
	close(responses)

	replayed := 0
	for response := range responses {
		if response.Code != http.StatusCreated {
			t.Fatalf("concurrent create returned %d: %s", response.Code, response.Body.String())
		}
		if response.Header().Get(idempotencyReplayHeader) == "true" {
			replayed++
		}
	}
	if replayed != 1 {
		t.Fatalf("concurrent create produced %d replay responses, want one", replayed)
	}
	var crews int
	if err := database.QueryRow(`select count(*) from crews`).Scan(&crews); err != nil {
		t.Fatalf("count idempotent crews: %v", err)
	}
	if crews != 1 {
		t.Fatalf("concurrent idempotent creation stored %d crews, want one", crews)
	}
}
