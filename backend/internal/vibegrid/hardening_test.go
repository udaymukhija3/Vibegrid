package vibegrid

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestInvalidGuessDoesNotAllocateAnonymousAttempt(t *testing.T) {
	store := NewMemoryAttemptStore()
	puzzle := SeedPuzzles()[0]
	request := GuessRequest{
		PuzzleID:        puzzle.ID,
		ClientGuessID:   "invalid-before-write",
		SelectedTileIDs: []string{"unknown-a", "unknown-b", "unknown-c", "unknown-d"},
		Mode:            AttemptModeMedium,
	}

	if _, err := store.SubmitGuess(context.Background(), puzzle, "0123456789abcdef0123456789abcdef", request, fixedClock()); !errors.Is(err, ErrUnknownTile) {
		t.Fatalf("expected unknown tile validation error, got %v", err)
	}
	if len(store.attempts) != 0 {
		t.Fatalf("invalid guess must not allocate an attempt, found %d", len(store.attempts))
	}
}

func TestAttemptModeLocksOnFirstGuess(t *testing.T) {
	store := NewMemoryAttemptStore()
	puzzle := SeedPuzzles()[0]
	first := wrongGuess("mode-first")
	first.Mode = AttemptModeEasy

	submission, err := store.SubmitGuess(context.Background(), puzzle, "mode-session", first, fixedClock())
	if err != nil {
		t.Fatalf("first guess: %v", err)
	}
	if submission.Attempt.Mode != AttemptModeEasy {
		t.Fatalf("expected Easy mode in snapshot, got %q", submission.Attempt.Mode)
	}

	changed := wrongGuess("mode-second")
	changed.Mode = AttemptModeHard
	if _, err := store.SubmitGuess(context.Background(), puzzle, "mode-session", changed, fixedClock()); !errors.Is(err, ErrAttemptModeConflict) {
		t.Fatalf("expected mode conflict, got %v", err)
	}

	snapshot, err := store.GetAttempt(context.Background(), puzzle, "mode-session", fixedClock())
	if err != nil {
		t.Fatalf("get attempt: %v", err)
	}
	if snapshot.Mode != AttemptModeEasy || snapshot.GuessCount != 1 {
		t.Fatalf("mode conflict mutated attempt: %#v", snapshot)
	}
}

func TestMemoryAttemptsAreBoundedAndExpire(t *testing.T) {
	store := NewMemoryAttemptStore()
	store.maxAttempts = 1
	puzzle := SeedPuzzles()[0]

	if _, err := store.SubmitGuess(context.Background(), puzzle, "session-one", wrongGuess("one"), fixedClock()); err != nil {
		t.Fatalf("first attempt: %v", err)
	}
	if _, err := store.SubmitGuess(context.Background(), puzzle, "session-two", wrongGuess("two"), fixedClock()); !errors.Is(err, ErrAttemptCapacity) {
		t.Fatalf("expected capacity error, got %v", err)
	}

	later := fixedClock().Add(defaultAttemptRetention + time.Second)
	if _, err := store.SubmitGuess(context.Background(), puzzle, "session-two", wrongGuess("two"), later); err != nil {
		t.Fatalf("expired attempt should be pruned before accepting a new one: %v", err)
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
		handler.ServeHTTP(rec, adminLoginRequest(body))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401 for wrong password, got %d", attempt+1, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, adminLoginRequest(body))
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
	handler.ServeHTTP(login, adminLoginRequest(`{"password":"correct-horse-battery-staple"}`))
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
	if got := getRec.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("admin response must be private, got %q", got)
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

func TestAdminLogoutRevokesSession(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles:            StaticPuzzleSource(SeedPuzzles()),
		AdminPuzzles:       newFakePuzzleBackend(),
		AdminPassword:      "correct-horse-battery-staple",
		AdminSessionSecret: "test-admin-session-signing-secret",
		Clock:              fixedClock,
	})

	login := httptest.NewRecorder()
	handler.ServeHTTP(login, adminLoginRequest(`{"password":"correct-horse-battery-staple"}`))
	if login.Code != http.StatusOK {
		t.Fatalf("login: expected 200, got %d: %s", login.Code, login.Body.String())
	}
	cookie := login.Result().Cookies()[0]

	logoutReq := httptest.NewRequest(http.MethodDelete, "/api/admin/session", nil)
	logoutReq.AddCookie(cookie)
	logout := httptest.NewRecorder()
	handler.ServeHTTP(logout, logoutReq)
	if logout.Code != http.StatusOK {
		t.Fatalf("logout: expected 200, got %d: %s", logout.Code, logout.Body.String())
	}

	replay := httptest.NewRequest(http.MethodGet, "/api/admin/puzzles", nil)
	replay.AddCookie(cookie)
	replayRec := httptest.NewRecorder()
	handler.ServeHTTP(replayRec, replay)
	if replayRec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked cookie must be rejected, got %d: %s", replayRec.Code, replayRec.Body.String())
	}
}

func TestJSONEndpointsRequireApplicationJSON(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles:            StaticPuzzleSource(SeedPuzzles()),
		AdminPassword:      "correct-horse-battery-staple",
		AdminSessionSecret: "test-admin-session-signing-secret",
		Clock:              fixedClock,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/admin/session", strings.NewReader(`{"password":"correct-horse-battery-staple"}`))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415 for non-JSON content type, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestOversizedJSONBodyReturns413(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles:            StaticPuzzleSource(SeedPuzzles()),
		AdminPassword:      "correct-horse-battery-staple",
		AdminSessionSecret: "test-admin-session-signing-secret",
		Clock:              fixedClock,
	})

	body := `{"password":"` + strings.Repeat("a", int(maxAdminBodyBytes)+1) + `"}`
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, adminLoginRequest(body))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for oversized JSON body, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminCookieMaxAgeUsesServerClock(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles:            StaticPuzzleSource(SeedPuzzles()),
		AdminPassword:      "correct-horse-battery-staple",
		AdminSessionSecret: "test-admin-session-signing-secret",
		Clock:              fixedClock,
	})

	login := httptest.NewRecorder()
	handler.ServeHTTP(login, adminLoginRequest(`{"password":"correct-horse-battery-staple"}`))
	if login.Code != http.StatusOK {
		t.Fatalf("login: expected 200, got %d: %s", login.Code, login.Body.String())
	}
	var adminCookie *http.Cookie
	for _, cookie := range login.Result().Cookies() {
		if cookie.Name == adminSessionCookieName {
			adminCookie = cookie
			break
		}
	}
	if adminCookie == nil {
		t.Fatal("expected admin login to set the admin session cookie")
	}
	if adminCookie.MaxAge != int(adminSessionDuration.Seconds()) {
		t.Fatalf("expected admin cookie MaxAge to use injected clock, got %d", adminCookie.MaxAge)
	}
}

func TestPublicWriteRateLimiterErrorsFailClosed(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles:    StaticPuzzleSource(SeedPuzzles()),
		Community:  fakeCommunityStore{},
		RateLimits: failingRateLimitStore{},
		Clock:      fixedClock,
	})

	payload, err := json.Marshal(validPuzzleInput())
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/community/puzzles", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected community create to fail closed on limiter error, got %d: %s", rec.Code, rec.Body.String())
	}

	server := &Server{
		rateLimits:     failingRateLimitStore{},
		reportLimiter:  newRateLimiter(reportRateLimit, reportRateWindow),
		clientIdentity: newClientIdentity(nil),
		clock:          fixedClock,
	}
	moderationRec := httptest.NewRecorder()
	moderationReq := httptest.NewRequest(http.MethodPost, "/api/reports", nil)
	if server.allowModerationWrite(moderationRec, moderationReq, "report:", "limited") {
		t.Fatal("expected moderation write to stop on limiter error")
	}
	if moderationRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected moderation write to fail closed on limiter error, got %d", moderationRec.Code)
	}
}

func TestPublicReadsUseMemoryLimiterAndWritesUseSharedLimiter(t *testing.T) {
	shared := &countingFailingRateLimitStore{}
	handler := NewServer(ServerConfig{
		Puzzles:    StaticPuzzleSource(SeedPuzzles()),
		RateLimits: shared,
		Clock:      fixedClock,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/puzzles/today", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected public read to use the in-memory limiter, got %d: %s", rec.Code, rec.Body.String())
	}
	if shared.checks != 0 {
		t.Fatalf("public read touched the shared limiter %d times", shared.checks)
	}

	// The local limiter still limits: exhaust the in-memory read budget and the next
	// read must be throttled, not allowed through unbounded.
	for i := 0; i < readRateLimit; i++ {
		exhaust := httptest.NewRecorder()
		handler.ServeHTTP(exhaust, httptest.NewRequest(http.MethodGet, "/api/puzzles/today", nil))
	}
	limited := httptest.NewRecorder()
	handler.ServeHTTP(limited, httptest.NewRequest(http.MethodGet, "/api/puzzles/today", nil))
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("expected in-memory fallback to throttle after %d reads, got %d", readRateLimit, limited.Code)
	}
	if shared.checks != 0 {
		t.Fatalf("rate-limited public reads touched the shared limiter %d times", shared.checks)
	}

	// Guesses are anonymous writes and must keep failing closed on limiter errors.
	guessBody := `{"puzzleId":"vibegrid-2026-06-02","clientGuessId":"guess-1","selectedTileIds":["a","b","c","d"]}`
	guessReq := httptest.NewRequest(http.MethodPost, "/api/guesses", strings.NewReader(guessBody))
	guessReq.Header.Set("Content-Type", "application/json")
	guessRec := httptest.NewRecorder()
	handler.ServeHTTP(guessRec, guessReq)
	if guessRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected guess to fail closed on limiter error, got %d: %s", guessRec.Code, guessRec.Body.String())
	}
	if shared.checks != 1 {
		t.Fatalf("expected guess to check the shared limiter once, got %d", shared.checks)
	}
}

func TestModerationPuzzleIDsAreValidatedBeforeStorage(t *testing.T) {
	invalidID := strings.Repeat("x", maxPuzzleIDLength+1)
	if err := validateReport(ReportInput{PuzzleID: invalidID, Reason: "SPAM"}); err == nil {
		t.Fatal("expected overlong report puzzle id to be rejected")
	}
	if err := validateAppeal(AppealInput{PuzzleID: invalidID, Message: "please review"}); err == nil {
		t.Fatal("expected overlong appeal puzzle id to be rejected")
	}
}

func TestAdminCanPreviewDraftWithoutMakingItPublic(t *testing.T) {
	draft := validPuzzleInput().toPuzzle(42)
	draft.ID = "draft-preview"
	draft.Status = PuzzleStatusDraft
	backend := newFakePuzzleBackend(draft)
	handler := NewServer(ServerConfig{
		Puzzles:      backend,
		AdminPuzzles: backend,
		AdminToken:   "script-token",
		Clock:        fixedClock,
	})

	public := adminRequest(t, handler, http.MethodGet, "/api/puzzles/"+draft.ID, "", nil)
	if public.Code != http.StatusNotFound {
		t.Fatalf("draft must stay hidden from public play, got %d: %s", public.Code, public.Body.String())
	}

	unauthorized := adminRequest(t, handler, http.MethodGet, "/api/admin/puzzles/"+draft.ID+"/preview", "", nil)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("preview must require admin auth, got %d: %s", unauthorized.Code, unauthorized.Body.String())
	}

	preview := adminRequest(t, handler, http.MethodGet, "/api/admin/puzzles/"+draft.ID+"/preview", "script-token", nil)
	if preview.Code != http.StatusOK {
		t.Fatalf("expected admin preview to load draft, got %d: %s", preview.Code, preview.Body.String())
	}
	var body adminPuzzlePreviewResponse
	if err := json.NewDecoder(preview.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Puzzle.ID != draft.ID || len(body.Puzzle.Tiles) != PuzzleGroupCount*GroupSize {
		t.Fatalf("unexpected preview puzzle: %#v", body.Puzzle)
	}
	if len(body.Groups) != PuzzleGroupCount {
		t.Fatalf("expected answer key in preview, got %d groups", len(body.Groups))
	}
}

func TestAdminQueueHealthUsesLaunchTimezoneAndEvergreenFallback(t *testing.T) {
	scheduled := validPuzzleInput().toPuzzle(43)
	scheduled.ID = "scheduled-editorial"
	scheduled.Status = PuzzleStatusPublished
	scheduled.Origin = OriginEditorial
	scheduled.PublishDate = "2026-06-03"

	draft := validPuzzleInput().toPuzzle(44)
	draft.ID = "draft-in-queue"
	draft.Status = PuzzleStatusDraft
	draft.Origin = OriginEditorial

	pendingCommunity := validPuzzleInput().toPuzzle(45)
	pendingCommunity.ID = "community-pending"
	pendingCommunity.Status = PuzzleStatusPending
	pendingCommunity.Origin = OriginCommunity

	handler := NewServer(ServerConfig{
		Puzzles:      NewBankPuzzleSource(StaticPuzzleSource([]Puzzle{scheduled, draft, pendingCommunity}), PuzzleBank(), nil),
		AdminPuzzles: newFakePuzzleBackend(),
		AdminToken:   "script-token",
		Clock:        fixedClock,
		TimeZone:     "UTC",
	})

	unauthorized := adminRequest(t, handler, http.MethodGet, "/api/admin/queue-health?days=3", "", nil)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("queue health must require admin auth, got %d: %s", unauthorized.Code, unauthorized.Body.String())
	}

	response := adminRequest(t, handler, http.MethodGet, "/api/admin/queue-health?days=3", "script-token", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("expected queue health, got %d: %s", response.Code, response.Body.String())
	}
	var body adminQueueHealthResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Today != "2026-06-02" || len(body.Days) != 3 {
		t.Fatalf("unexpected queue window: %#v", body)
	}
	if body.Drafts != 1 || body.PendingCommunity != 1 {
		t.Fatalf("unexpected queue counts: drafts=%d pending=%d", body.Drafts, body.PendingCommunity)
	}
	if body.ScheduledEditorial != 1 || body.EvergreenFallbacks != 2 {
		t.Fatalf("unexpected coverage counts: scheduled=%d evergreen=%d", body.ScheduledEditorial, body.EvergreenFallbacks)
	}
	if body.Days[0].Coverage != queueCoverageEvergreen || body.Days[0].Date != "2026-06-02" {
		t.Fatalf("expected first day to use evergreen fallback, got %#v", body.Days[0])
	}
	if body.Days[1].Coverage != queueCoverageEditorial || body.Days[1].PuzzleID != scheduled.ID {
		t.Fatalf("expected scheduled editorial day, got %#v", body.Days[1])
	}
	if body.Days[2].Coverage != queueCoverageEvergreen {
		t.Fatalf("expected third day to use evergreen fallback, got %#v", body.Days[2])
	}
}

type failingRateLimitStore struct{}

func (failingRateLimitStore) Check(context.Context, string, int, time.Duration, time.Time) (rateLimitDecision, error) {
	return rateLimitDecision{}, errors.New("failing DB limiter")
}
func (failingRateLimitStore) Prune(context.Context) error {
	return errors.New("failing DB limiter")
}

type countingFailingRateLimitStore struct {
	checks int
}

func (store *countingFailingRateLimitStore) Check(context.Context, string, int, time.Duration, time.Time) (rateLimitDecision, error) {
	store.checks++
	return rateLimitDecision{}, errors.New("failing DB limiter")
}

func (*countingFailingRateLimitStore) Prune(context.Context) error {
	return nil
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

func adminLoginRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/admin/session", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}
