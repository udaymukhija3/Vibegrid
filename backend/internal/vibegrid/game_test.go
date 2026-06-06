package vibegrid

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEvaluateGuessMatchesRegardlessOfOrder(t *testing.T) {
	puzzle := SeedPuzzles()[0]

	group, err := EvaluateGuess(puzzle, []string{
		"p1-balcony",
		"p1-vespa",
		"p1-linen",
		"p1-espresso",
	}, map[string]bool{})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if group == nil || group.ID != "italian-summer" {
		t.Fatalf("expected italian summer group, got %#v", group)
	}
}

func TestReadinessReflectsReadyCheck(t *testing.T) {
	// Healthy: no ready check configured (in-memory mode) → always ready.
	healthy := NewServer(ServerConfig{Puzzles: StaticPuzzleSource(SeedPuzzles())})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	healthy.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with no ready check, got %d", rec.Code)
	}

	// Unhealthy: a ready check that fails → 503.
	failing := NewServer(ServerConfig{
		Puzzles:    StaticPuzzleSource(SeedPuzzles()),
		ReadyCheck: func(context.Context) error { return errors.New("db down") },
	})
	rec2 := httptest.NewRecorder()
	failing.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec2.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when ready check fails, got %d", rec2.Code)
	}
}

func TestPublicArchiveExcludesFuturePuzzles(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles: StaticPuzzleSource(SeedPuzzles()),
		Clock:   fixedClock,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/puzzles", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var puzzles []PublicPuzzle
	if err := json.NewDecoder(rec.Body).Decode(&puzzles); err != nil {
		t.Fatal(err)
	}
	if len(puzzles) != 1 || puzzles[0].ID != "vibegrid-2026-06-02" {
		t.Fatalf("expected only the live puzzle, got %#v", puzzles)
	}
}

func TestFutureAndDraftPuzzlesAreNotPubliclyPlayable(t *testing.T) {
	draft := SeedPuzzles()[0]
	draft.ID = "draft-puzzle"
	draft.Status = PuzzleStatusDraft

	handler := NewServer(ServerConfig{
		Puzzles: StaticPuzzleSource(append(SeedPuzzles(), draft)),
		Store:   NewMemoryAttemptStore(),
		Clock:   fixedClock,
	})

	for _, path := range []string{
		"/api/puzzles/vibegrid-2026-06-03",
		"/api/puzzles/draft-puzzle",
		"/api/puzzles/vibegrid-2026-06-03/stats",
		"/api/attempts/vibegrid-2026-06-03",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: expected 404, got %d: %s", path, rec.Code, rec.Body.String())
		}
	}

	response := postGuess(t, handler, "", GuessRequest{
		PuzzleID:        "vibegrid-2026-06-03",
		ClientGuessID:   "future",
		SelectedTileIDs: []string{"p2-passport", "p2-gate", "p2-neck-pillow", "p2-boarding-group"},
	})
	if response.Code != http.StatusNotFound {
		t.Fatalf("future guess: expected 404, got %d: %s", response.Code, response.Body.String())
	}
}

func TestTodayCacheControlExpiresAtMidnight(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles:  StaticPuzzleSource(SeedPuzzles()),
		Clock:    func() time.Time { return time.Date(2026, 6, 2, 23, 59, 30, 0, time.UTC) },
		TimeZone: "UTC",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/puzzles/today", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=30, s-maxage=30" {
		t.Fatalf("unexpected cache header: %q", got)
	}
}

func TestSecurityHeadersAreApplied(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles: StaticPuzzleSource(SeedPuzzles()),
		Frontend: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("frontend"))
		}),
	})

	apiReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	apiRec := httptest.NewRecorder()
	handler.ServeHTTP(apiRec, apiReq)

	if got := apiRec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("expected DENY frame header, got %q", got)
	}
	if got := apiRec.Header().Get("Content-Security-Policy"); got != "default-src 'none'; frame-ancestors 'none'; base-uri 'none'" {
		t.Fatalf("unexpected API content security policy: %q", got)
	}

	frontendReq := httptest.NewRequest(http.MethodGet, "/", nil)
	frontendRec := httptest.NewRecorder()
	handler.ServeHTTP(frontendRec, frontendReq)
	if got := frontendRec.Header().Get("Content-Security-Policy"); !strings.Contains(got, "connect-src 'self'") {
		t.Fatalf("expected frontend content security policy, got %q", got)
	}
}

func TestMetricsEndpointCountsRequests(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles: StaticPuzzleSource(SeedPuzzles()),
		Clock:   fixedClock,
	})

	for _, path := range []string{"/healthz", "/api/puzzles/today"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d: %s", path, rec.Code, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	for _, expected := range []string{
		"vibegrid_up 1",
		`vibegrid_http_requests_total{method="GET",route="/healthz",status="200"} 1`,
		`vibegrid_http_requests_total{method="GET",route="/api/puzzles/today",status="200"} 1`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected metrics to contain %q, got:\n%s", expected, body)
		}
	}
}

func TestSharedPuzzleHTMLInjectsOGMetadata(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles: StaticPuzzleSource(SeedPuzzles()),
		Frontend: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<html><head></head><body>share</body></html>"))
		}),
	})

	req := httptest.NewRequest(http.MethodGet, "/p/vibegrid-2026-06-02", nil)
	req.Host = "vibegrid.example"
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `https://vibegrid.example/api/og/puzzles/vibegrid-2026-06-02.svg`) {
		t.Fatalf("expected injected OG image metadata, got %s", rec.Body.String())
	}
}

func TestPuzzleOGImageEndpoint(t *testing.T) {
	handler := NewServer(ServerConfig{Puzzles: StaticPuzzleSource(SeedPuzzles()), Clock: fixedClock})

	req := httptest.NewRequest(http.MethodGet, "/api/og/puzzles/vibegrid-2026-06-02.svg", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/svg+xml; charset=utf-8" {
		t.Fatalf("unexpected content type: %q", got)
	}
	if !strings.Contains(rec.Body.String(), "VibeGrid #1") {
		t.Fatalf("expected puzzle number in OG SVG, got %s", rec.Body.String())
	}
}

func TestRequestTimeoutMiddleware(t *testing.T) {
	handler := withRequestTimeout(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}), time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 on timeout, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Request timed out.") {
		t.Fatalf("expected timeout body, got %q", rec.Body.String())
	}
}

func TestIsOneAway(t *testing.T) {
	puzzle := SeedPuzzles()[0]

	// Three Italian-summer tiles + one intruder = one away.
	if !IsOneAway(puzzle, []string{"p1-espresso", "p1-linen", "p1-vespa", "p1-slack"}) {
		t.Fatal("expected one away for a 3+1 guess")
	}
	// Two and two = not one away.
	if IsOneAway(puzzle, []string{"p1-espresso", "p1-linen", "p1-slack", "p1-deck"}) {
		t.Fatal("did not expect one away for a 2+2 guess")
	}
	// An exact group is not reported as one away (it is a correct guess).
	if IsOneAway(puzzle, []string{"p1-espresso", "p1-linen", "p1-vespa", "p1-balcony"}) {
		t.Fatal("a full group is not one away")
	}
}

func TestGuessResponseFlagsOneAway(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles: StaticPuzzleSource(SeedPuzzles()),
		Store:   NewMemoryAttemptStore(),
		Clock:   fixedClock,
	})

	response := postGuess(t, handler, "", GuessRequest{
		PuzzleID:        "vibegrid-2026-06-02",
		ClientGuessID:   "near-miss",
		SelectedTileIDs: []string{"p1-espresso", "p1-linen", "p1-vespa", "p1-slack"},
	})

	var body GuessResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.IsCorrect || !body.OneAway {
		t.Fatalf("expected a wrong, one-away guess, got %#v", body)
	}
}

func TestEvaluateGuessRejectsDuplicateTiles(t *testing.T) {
	puzzle := SeedPuzzles()[0]

	_, err := EvaluateGuess(puzzle, []string{
		"p1-espresso",
		"p1-espresso",
		"p1-linen",
		"p1-vespa",
	}, map[string]bool{})

	if err != ErrInvalidGroupSize {
		t.Fatalf("expected ErrInvalidGroupSize, got %v", err)
	}
}

func TestGuessAPIStoresAttemptAndSetsSessionCookie(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles: StaticPuzzleSource(SeedPuzzles()),
		Store:   NewMemoryAttemptStore(),
		Clock:   fixedClock,
	})

	response := postGuess(t, handler, "", GuessRequest{
		PuzzleID:        "vibegrid-2026-06-02",
		ClientGuessID:   "guess-1",
		SelectedTileIDs: []string{"p1-espresso", "p1-linen", "p1-vespa", "p1-balcony"},
	})

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if len(response.Result().Cookies()) == 0 {
		t.Fatal("expected session cookie")
	}

	var body GuessResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.OK || !body.IsCorrect || body.Group == nil || body.Group.ID != "italian-summer" {
		t.Fatalf("unexpected body: %#v", body)
	}
	if body.Attempt == nil || len(body.Attempt.SolvedGroups) != 1 {
		t.Fatalf("expected one solved group, got %#v", body.Attempt)
	}
}

func TestGuessRejectsUnknownJSONFields(t *testing.T) {
	handler := guessTestHandler(newRateLimiter(10, time.Minute))

	response := postRawGuess(t, handler, `{
		"puzzleId": "vibegrid-2026-06-02",
		"clientGuessId": "unknown-field",
		"selectedTileIds": ["p1-espresso", "p1-linen", "p1-vespa", "p1-balcony"],
		"extra": true
	}`)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown JSON field, got %d: %s", response.Code, response.Body.String())
	}
	if len(response.Result().Cookies()) != 0 {
		t.Fatalf("malformed guesses must not create sessions, got cookies %#v", response.Result().Cookies())
	}
}

func TestGuessRejectsTrailingJSON(t *testing.T) {
	handler := guessTestHandler(newRateLimiter(10, time.Minute))

	response := postRawGuess(t, handler, `{
		"puzzleId": "vibegrid-2026-06-02",
		"clientGuessId": "trailing-json",
		"selectedTileIds": ["p1-espresso", "p1-linen", "p1-vespa", "p1-balcony"]
	} {}`)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for trailing JSON, got %d: %s", response.Code, response.Body.String())
	}
	if len(response.Result().Cookies()) != 0 {
		t.Fatalf("malformed guesses must not create sessions, got cookies %#v", response.Result().Cookies())
	}
}

func TestGuessRateLimitReturnsRetryAfter(t *testing.T) {
	handler := guessTestHandler(newRateLimiter(2, time.Minute))

	for index, clientGuessID := range []string{"rate-a", "rate-b"} {
		response := postGuess(t, handler, "", GuessRequest{
			PuzzleID:        "vibegrid-2026-06-02",
			ClientGuessID:   clientGuessID,
			SelectedTileIDs: []string{"p1-espresso", "p1-linen", "p1-vespa", "p1-balcony"},
		})
		if response.Code != http.StatusOK {
			t.Fatalf("allowed guess %d: expected 200, got %d: %s", index+1, response.Code, response.Body.String())
		}
	}

	response := postGuess(t, handler, "", GuessRequest{
		PuzzleID:        "vibegrid-2026-06-02",
		ClientGuessID:   "rate-c",
		SelectedTileIDs: []string{"p1-espresso", "p1-linen", "p1-vespa", "p1-balcony"},
	})
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after rate limit, got %d: %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Retry-After"); got != "60" {
		t.Fatalf("expected Retry-After 60, got %q", got)
	}
}

func TestMalformedSessionCookieIsRotated(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles: StaticPuzzleSource(SeedPuzzles()),
		Store:   NewMemoryAttemptStore(),
		Clock:   fixedClock,
	})

	response := postGuess(t, handler, sessionCookieName+"=not-a-session", GuessRequest{
		PuzzleID:        "vibegrid-2026-06-02",
		ClientGuessID:   "guess-1",
		SelectedTileIDs: []string{"p1-espresso", "p1-linen", "p1-vespa", "p1-balcony"},
	})

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if cookies := response.Result().Cookies(); len(cookies) == 0 || !validSessionID(cookies[0].Value) {
		t.Fatalf("expected a rotated valid session cookie, got %#v", cookies)
	}
}

func TestDuplicateClientGuessIsIdempotent(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles: StaticPuzzleSource(SeedPuzzles()),
		Store:   NewMemoryAttemptStore(),
		Clock:   fixedClock,
	})

	first := postGuess(t, handler, "", GuessRequest{
		PuzzleID:        "vibegrid-2026-06-02",
		ClientGuessID:   "same-id",
		SelectedTileIDs: []string{"p1-espresso", "p1-linen", "p1-slack", "p1-balcony"},
	})
	sessionCookie := first.Result().Cookies()[0].String()

	second := postGuess(t, handler, sessionCookie, GuessRequest{
		PuzzleID:        "vibegrid-2026-06-02",
		ClientGuessID:   "same-id",
		SelectedTileIDs: []string{"p1-espresso", "p1-linen", "p1-slack", "p1-balcony"},
	})

	var body GuessResponse
	if err := json.NewDecoder(second.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Attempt == nil || body.Attempt.Mistakes != 1 || body.Attempt.GuessCount != 1 {
		t.Fatalf("expected duplicate not to increment attempt, got %#v", body.Attempt)
	}
}

func TestFourthMistakeRevealsGroups(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles: StaticPuzzleSource(SeedPuzzles()),
		Store:   NewMemoryAttemptStore(),
		Clock:   fixedClock,
	})

	sessionCookie := ""
	var body GuessResponse
	for index := 0; index < MaxMistakes; index++ {
		response := postGuess(t, handler, sessionCookie, GuessRequest{
			PuzzleID:        "vibegrid-2026-06-02",
			ClientGuessID:   "miss-" + string(rune('a'+index)),
			SelectedTileIDs: []string{"p1-espresso", "p1-linen", "p1-slack", "p1-balcony"},
		})
		if sessionCookie == "" {
			sessionCookie = response.Result().Cookies()[0].String()
		}
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
	}

	if body.Attempt == nil || !body.Attempt.Failed {
		t.Fatalf("expected failed attempt, got %#v", body.Attempt)
	}
	if len(body.RevealedGroups) != 4 {
		t.Fatalf("expected revealed groups, got %#v", body.RevealedGroups)
	}
}

func postGuess(t *testing.T, handler http.Handler, cookie string, request GuessRequest) *httptest.ResponseRecorder {
	t.Helper()

	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/guesses", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func postRawGuess(t *testing.T, handler http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/guesses", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func guessTestHandler(limiter *rateLimiter) http.Handler {
	server := &Server{
		puzzles:      StaticPuzzleSource(SeedPuzzles()),
		store:        NewMemoryAttemptStore(),
		guessLimiter: limiter,
		clock:        fixedClock,
		timeZone:     "UTC",
	}
	return http.HandlerFunc(server.handleGuess)
}

func fixedClock() time.Time {
	return time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
}
