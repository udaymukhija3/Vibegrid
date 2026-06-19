package vibegrid

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPublicPuzzleOrderDoesNotRevealGroups guards the answer-key fix: the public
// tile order must be a stable permutation that does not match the group-blocked
// layout, so a client cannot recover the grouping from the payload.
func TestPublicPuzzleOrderDoesNotRevealGroups(t *testing.T) {
	puzzle := SeedPuzzles()[0]
	public := ToPublicPuzzle(puzzle)

	want := len(puzzle.Groups) * GroupSize
	if len(public.Tiles) != want {
		t.Fatalf("expected %d tiles, got %d", want, len(public.Tiles))
	}

	// Pre-shuffle layout is group-blocked (tiles 0..3 = group 0, etc.). If the
	// public order still matched it, the answer key would be trivially readable.
	grouped := make([]Tile, 0, want)
	for _, group := range puzzle.Groups {
		grouped = append(grouped, group.Tiles...)
	}
	identical := true
	for index := range grouped {
		if grouped[index].ID != public.Tiles[index].ID {
			identical = false
			break
		}
	}
	if identical {
		t.Fatal("public tile order matches group-blocked order; grouping is leaked")
	}

	// Must be a permutation: same set of tile ids, nothing dropped or added.
	seen := map[string]int{}
	for _, tile := range grouped {
		seen[tile.ID]++
	}
	for _, tile := range public.Tiles {
		seen[tile.ID]--
	}
	for id, count := range seen {
		if count != 0 {
			t.Fatalf("public tiles are not a permutation of puzzle tiles (id %q off by %d)", id, count)
		}
	}

	// Stable across calls so the board does not reshuffle on refresh.
	again := ToPublicPuzzle(puzzle)
	for index := range public.Tiles {
		if public.Tiles[index].ID != again.Tiles[index].ID {
			t.Fatal("public tile order is not stable across calls")
		}
	}
}

// TestGetAttemptDoesNotCreateRow guards the write-free read path: loading an
// attempt for a session that has not guessed must not persist anything; the row
// is created lazily on the first guess.
func TestGetAttemptDoesNotCreateRow(t *testing.T) {
	store := NewMemoryAttemptStore()
	puzzle := SeedPuzzles()[0]
	ctx := context.Background()

	snapshot, err := store.GetAttempt(ctx, puzzle, "sess-readonly", fixedClock())
	if err != nil {
		t.Fatalf("GetAttempt: %v", err)
	}
	if snapshot.GuessCount != 0 || snapshot.Completed || snapshot.Failed || len(snapshot.SolvedGroups) != 0 {
		t.Fatalf("expected an empty snapshot, got %#v", snapshot)
	}
	if len(store.attempts) != 0 {
		t.Fatalf("GetAttempt must not create an attempt row, found %d", len(store.attempts))
	}

	if _, err := store.SubmitGuess(ctx, puzzle, "sess-readonly", wrongGuess("g1"), fixedClock()); err != nil {
		t.Fatalf("SubmitGuess: %v", err)
	}
	if len(store.attempts) != 1 {
		t.Fatalf("expected exactly one attempt after a guess, found %d", len(store.attempts))
	}
}

// TestAdminLoginThrottlesRepeatedAttempts guards the brute-force throttle: after
// the per-IP allowance is spent, further login attempts get 429 with Retry-After
// instead of another password check.
func TestAdminLoginThrottlesRepeatedAttempts(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles:            StaticPuzzleSource(SeedPuzzles()),
		Store:              NewMemoryAttemptStore(),
		AdminPassword:      "correct-horse-battery-staple",
		AdminSessionSecret: "test-admin-session-signing-secret",
		Clock:              fixedClock,
	})

	const body = `{"password":"wrong"}`
	for attempt := 0; attempt < adminLoginRateLimit; attempt++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/admin/session", strings.NewReader(body)))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401 for wrong password, got %d", attempt+1, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/admin/session", strings.NewReader(body)))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 once the login allowance is spent, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("expected a Retry-After header on a throttled login")
	}
}

func TestAdminCookieMutationsRequireCSRF(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles:            StaticPuzzleSource(SeedPuzzles()),
		AdminPuzzles:       newFakePuzzleBackend(),
		AdminToken:         "script-token",
		AdminPassword:      "correct-horse-battery-staple",
		AdminSessionSecret: "test-admin-session-signing-secret",
		Clock:              fixedClock,
	})

	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest(http.MethodPost, "/api/admin/session", strings.NewReader(`{"password":"correct-horse-battery-staple"}`)))
	if login.Code != http.StatusOK {
		t.Fatalf("login: expected 200, got %d: %s", login.Code, login.Body.String())
	}

	var session adminSessionResponse
	if err := json.NewDecoder(login.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	if session.CSRFToken == "" {
		t.Fatal("expected login to return a CSRF token")
	}
	cookies := login.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected login to set an admin cookie")
	}

	get := httptest.NewRequest(http.MethodGet, "/api/admin/puzzles", nil)
	get.AddCookie(cookies[0])
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, get)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET should not require CSRF, got %d: %s", getRec.Code, getRec.Body.String())
	}

	noCSRF := adminCookiePostRequest(cookies[0], "")
	noCSRFRec := httptest.NewRecorder()
	handler.ServeHTTP(noCSRFRec, noCSRF)
	if noCSRFRec.Code != http.StatusForbidden {
		t.Fatalf("POST without CSRF: expected 403, got %d: %s", noCSRFRec.Code, noCSRFRec.Body.String())
	}

	withCSRF := adminCookiePostRequest(cookies[0], session.CSRFToken)
	withCSRFRec := httptest.NewRecorder()
	handler.ServeHTTP(withCSRFRec, withCSRF)
	if withCSRFRec.Code != http.StatusCreated {
		t.Fatalf("POST with CSRF: expected 201, got %d: %s", withCSRFRec.Code, withCSRFRec.Body.String())
	}

	bearer := adminCookiePostRequest(nil, "")
	bearer.Header.Set("Authorization", "Bearer script-token")
	bearerRec := httptest.NewRecorder()
	handler.ServeHTTP(bearerRec, bearer)
	if bearerRec.Code != http.StatusCreated {
		t.Fatalf("bearer POST should bypass CSRF, got %d: %s", bearerRec.Code, bearerRec.Body.String())
	}
}

func adminCookiePostRequest(cookie *http.Cookie, csrfToken string) *http.Request {
	payload, _ := json.Marshal(validPuzzleInput())
	req := httptest.NewRequest(http.MethodPost, "/api/admin/puzzles", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if csrfToken != "" {
		req.Header.Set(adminCSRFHeader, csrfToken)
	}
	return req
}
