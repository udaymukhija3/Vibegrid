package vibegrid

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeCommunityStore struct{}

func (fakeCommunityStore) CreateCommunityPuzzle(_ context.Context, input AdminPuzzleInput, _ string) (Puzzle, error) {
	puzzle := input.toPuzzle(999)
	puzzle.Status = PuzzleStatusPending
	puzzle.Origin = OriginCommunity
	return puzzle, nil
}

func (fakeCommunityStore) CreatorStatus(context.Context, string, string) (CreatorPuzzleStatus, error) {
	return CreatorPuzzleStatus{}, ErrCreatorClaimInvalid
}

func (fakeCommunityStore) WithdrawCommunityPuzzle(context.Context, string, string) (CreatorPuzzleStatus, error) {
	return CreatorPuzzleStatus{}, ErrCreatorClaimInvalid
}

type claimCommunityStore struct {
	mu        sync.Mutex
	puzzle    Puzzle
	claimHash string
	updatedAt time.Time
	withdrawn bool
}

func (store *claimCommunityStore) CreateCommunityPuzzle(_ context.Context, input AdminPuzzleInput, claimHash string) (Puzzle, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.puzzle = input.toPuzzle(777)
	store.puzzle.Status = PuzzleStatusPending
	store.puzzle.Origin = OriginCommunity
	store.claimHash = claimHash
	store.updatedAt = fixedClock()
	return store.puzzle, nil
}

func (store *claimCommunityStore) CreatorStatus(_ context.Context, puzzleID, claimHash string) (CreatorPuzzleStatus, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.puzzle.ID != puzzleID || store.claimHash != claimHash {
		return CreatorPuzzleStatus{}, ErrCreatorClaimInvalid
	}
	return store.statusLocked(), nil
}

func (store *claimCommunityStore) WithdrawCommunityPuzzle(_ context.Context, puzzleID, claimHash string) (CreatorPuzzleStatus, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.puzzle.ID != puzzleID || store.claimHash != claimHash {
		return CreatorPuzzleStatus{}, ErrCreatorClaimInvalid
	}
	if store.withdrawn {
		return store.statusLocked(), nil
	}
	if store.puzzle.Status != PuzzleStatusPending {
		return CreatorPuzzleStatus{}, ErrCreatorWithdrawalUnavailable
	}
	store.puzzle.Status = PuzzleStatusArchived
	store.withdrawn = true
	store.updatedAt = fixedClock().Add(time.Minute)
	return store.statusLocked(), nil
}

func (store *claimCommunityStore) statusLocked() CreatorPuzzleStatus {
	return CreatorPuzzleStatus{
		ID:           store.puzzle.ID,
		PuzzleNumber: store.puzzle.PuzzleNumber,
		Status:       store.puzzle.Status,
		UpdatedAt:    store.updatedAt,
		Withdrawn:    store.withdrawn,
	}
}

func TestCommunityCreateNeedsNoToken(t *testing.T) {
	handler, _ := newAdminTestServer(t)

	// No Authorization header at all.
	response := adminRequest(t, handler, http.MethodPost, "/api/community/puzzles", "", validPuzzleInput())
	if response.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for community create without token, got %d: %s", response.Code, response.Body.String())
	}
}

func TestCreatorClaimStatusAndWithdrawalLifecycle(t *testing.T) {
	community := &claimCommunityStore{}
	handler := NewServer(ServerConfig{
		Puzzles:     StaticPuzzleSource(SeedPuzzles()),
		Community:   community,
		Idempotency: newMemoryIdempotencyStore(),
		Clock:       fixedClock,
	})

	created := adminRequest(t, handler, http.MethodPost, "/api/community/puzzles", "", validPuzzleInput())
	if created.Code != http.StatusAccepted {
		t.Fatalf("create failed: %d %s", created.Code, created.Body.String())
	}
	var creation createdPuzzleResponse
	if err := json.NewDecoder(created.Body).Decode(&creation); err != nil {
		t.Fatal(err)
	}
	if !validCreatorClaimSecret(creation.ClaimSecret) || creation.ClaimPath != "/claim?id="+creation.ID {
		t.Fatalf("invalid creator claim response: %#v", creation)
	}
	community.mu.Lock()
	storedHash := community.claimHash
	community.mu.Unlock()
	if storedHash == creation.ClaimSecret || storedHash != hashCreatorClaimSecret(creation.ClaimSecret) {
		t.Fatal("creator store did not retain only the claim hash")
	}

	wrong := creatorRequest(t, handler, http.MethodGet, "/api/community/puzzles/"+creation.ID+"/claim", newCreatorClaimSecret(), nil)
	if wrong.Code != http.StatusNotFound {
		t.Fatalf("wrong claim should be indistinguishable from missing, got %d", wrong.Code)
	}

	status := creatorRequest(t, handler, http.MethodGet, "/api/community/puzzles/"+creation.ID+"/claim", creation.ClaimSecret, nil)
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"canWithdraw":true`) {
		t.Fatalf("pending claim status failed: %d %s", status.Code, status.Body.String())
	}

	withdrawn := creatorRequest(t, handler, http.MethodPost, "/api/community/puzzles/"+creation.ID+"/withdraw", creation.ClaimSecret, nil)
	if withdrawn.Code != http.StatusOK || !strings.Contains(withdrawn.Body.String(), `"withdrawn":true`) {
		t.Fatalf("withdraw failed: %d %s", withdrawn.Code, withdrawn.Body.String())
	}
	replayed := creatorRequest(t, handler, http.MethodPost, "/api/community/puzzles/"+creation.ID+"/withdraw", creation.ClaimSecret, nil)
	if replayed.Code != http.StatusOK {
		t.Fatalf("withdrawal should be naturally idempotent, got %d %s", replayed.Code, replayed.Body.String())
	}
}

func TestCommunityCreateValidatesInput(t *testing.T) {
	handler, _ := newAdminTestServer(t)

	input := validPuzzleInput()
	input.Groups[2].Name = "" // missing name

	response := adminRequest(t, handler, http.MethodPost, "/api/community/puzzles", "", input)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for invalid community puzzle, got %d", response.Code)
	}
}

func TestCommunityCreateRejectsOverlongTiles(t *testing.T) {
	handler, _ := newAdminTestServer(t)

	input := validPuzzleInput()
	input.Groups[0].Tiles[0] = stringOfLength(MaxTileTextLength + 1)

	response := adminRequest(t, handler, http.MethodPost, "/api/community/puzzles", "", input)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for overlong public tile, got %d: %s", response.Code, response.Body.String())
	}
}

func TestCommunityCreateRejectsBlockedTerms(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles:      StaticPuzzleSource(SeedPuzzles()),
		Community:    fakeCommunityStore{},
		BlockedTerms: []string{"forbidden phrase"},
	})

	input := validPuzzleInput()
	input.Groups[0].Tiles[0] = "forbidden phrase"

	response := adminRequest(t, handler, http.MethodPost, "/api/community/puzzles", "", input)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for blocked term, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "blocked word or phrase") {
		t.Fatalf("expected blocked-term message, got %s", response.Body.String())
	}
}

func TestClientIPOnlyUsesHeadersFromTrustedProxy(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/community/puzzles", nil)
	request.RemoteAddr = "203.0.113.10:12345"
	request.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2")
	request.Header.Set("X-Real-IP", "198.51.100.9")
	request.Header.Set("Fly-Client-IP", "198.51.100.8")

	untrusted := newClientIdentity(nil)
	if got := untrusted.clientIP(request); got != "203.0.113.10" {
		t.Fatalf("untrusted client headers must be ignored, got %q", got)
	}

	trusted := newClientIdentity([]string{"203.0.113.0/24"})
	if got := trusted.clientIP(request); got != "198.51.100.8" {
		t.Fatalf("expected trusted Fly-Client-IP, got %q", got)
	}

	request.Header.Del("X-Real-IP")
	request.Header.Del("Fly-Client-IP")
	if got := trusted.clientIP(request); got != "10.0.0.1" {
		t.Fatalf("expected trusted X-Forwarded-For fallback, got %q", got)
	}
}

// TestCommunityPuzzleIsPlayableByLinkBeforeReview covers the unlisted-by-link
// contract end to end through the HTTP surface: a freshly created grid is
// immediately playable by its link so the creator can send it to friends, it
// never leaks into the daily/archive listing, and review only promotes it from
// unlisted to listed.
func TestCommunityPuzzleIsPlayableByLinkBeforeReview(t *testing.T) {
	handler, _ := newAdminTestServer(t)

	created := adminRequest(t, handler, http.MethodPost, "/api/community/puzzles", "", validPuzzleInput())
	if created.Code != http.StatusAccepted {
		t.Fatalf("create failed: %d %s", created.Code, created.Body.String())
	}
	var body createdPuzzleResponse
	if err := json.NewDecoder(created.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}

	if body.Status != PuzzleStatusPending {
		t.Fatalf("expected pending submission, got %q", body.Status)
	}
	if body.PlayPath != "/p/"+body.ID {
		t.Fatalf("create response must hand back a share link, got %q", body.PlayPath)
	}

	// The whole point: playable straight away, before any editor touches it.
	play := adminRequest(t, handler, http.MethodGet, "/api/puzzles/"+body.ID, "", nil)
	if play.Code != http.StatusOK {
		t.Fatalf("expected unreviewed puzzle to be playable by link, got %d: %s", play.Code, play.Body.String())
	}
	var public PublicPuzzle
	if err := json.NewDecoder(play.Body).Decode(&public); err != nil {
		t.Fatal(err)
	}
	if len(public.Tiles) != PuzzleGroupCount*GroupSize {
		t.Fatalf("expected %d tiles, got %d", PuzzleGroupCount*GroupSize, len(public.Tiles))
	}

	// Unlisted means unlisted, both before and after approval.
	assertAbsentFromPublicList(t, handler, body.ID)

	approved := adminRequest(t, handler, http.MethodPost, "/api/admin/puzzles/"+body.ID+"/approve", testAdminToken, nil)
	if approved.Code != http.StatusOK {
		t.Fatalf("approve failed: %d %s", approved.Code, approved.Body.String())
	}

	play = adminRequest(t, handler, http.MethodGet, "/api/puzzles/"+body.ID, "", nil)
	if play.Code != http.StatusOK {
		t.Fatalf("expected approved community puzzle to stay playable, got %d", play.Code)
	}
	assertAbsentFromPublicList(t, handler, body.ID)
}

// TestCommunityPuzzleLinkDiesOnTakedown is the other half of the contract:
// opening the link early must not make moderation toothless.
func TestCommunityPuzzleLinkDiesOnTakedown(t *testing.T) {
	handler, _ := newAdminTestServer(t)

	created := adminRequest(t, handler, http.MethodPost, "/api/community/puzzles", "", validPuzzleInput())
	if created.Code != http.StatusAccepted {
		t.Fatalf("create failed: %d %s", created.Code, created.Body.String())
	}
	var body createdPuzzleResponse
	if err := json.NewDecoder(created.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}

	archived := adminRequest(t, handler, http.MethodPost, "/api/admin/puzzles/"+body.ID+"/archive", testAdminToken, nil)
	if archived.Code != http.StatusOK {
		t.Fatalf("archive failed: %d %s", archived.Code, archived.Body.String())
	}

	play := adminRequest(t, handler, http.MethodGet, "/api/puzzles/"+body.ID, "", nil)
	if play.Code != http.StatusNotFound {
		t.Fatalf("a taken-down grid must stop being playable, got %d: %s", play.Code, play.Body.String())
	}
}

func assertAbsentFromPublicList(t *testing.T, handler http.Handler, puzzleID string) {
	t.Helper()
	list := adminRequest(t, handler, http.MethodGet, "/api/puzzles", "", nil)
	var published []PublicPuzzle
	if err := json.NewDecoder(list.Body).Decode(&published); err != nil {
		t.Fatal(err)
	}
	for _, puzzle := range published {
		if puzzle.ID == puzzleID {
			t.Fatalf("community puzzle %s must not appear in the daily/archive list", puzzleID)
		}
	}
}

func TestPostgresConcurrentCommunityCreateIsIdempotent(t *testing.T) {
	handler, puzzleStore := newAdminTestServer(t)
	payload, err := json.Marshal(validPuzzleInput())
	if err != nil {
		t.Fatal(err)
	}

	sessionResponse := adminRequest(t, handler, http.MethodGet, "/api/session", "", nil)
	sessionCookie := responseCookie(sessionResponse, sessionCookieName)
	if sessionCookie == nil {
		t.Fatal("session endpoint did not set guest cookie")
	}

	const requestCount = 8
	responses := make(chan *httptest.ResponseRecorder, requestCount)
	var wait sync.WaitGroup
	for index := 0; index < requestCount; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			responses <- postIdempotentJSON(handler, "/api/community/puzzles", payload, "concurrent-community-create", sessionCookie)
		}()
	}
	wait.Wait()
	close(responses)

	var originalBody string
	for response := range responses {
		if response.Code != http.StatusAccepted {
			t.Fatalf("concurrent create failed: %d %s", response.Code, response.Body.String())
		}
		if originalBody == "" {
			originalBody = response.Body.String()
		} else if response.Body.String() != originalBody {
			t.Fatalf("concurrent replay returned a different response:\nfirst: %s\ngot: %s", originalBody, response.Body.String())
		}
	}

	var puzzlesCreated int
	if err := puzzleStore.db.QueryRow(`select count(*) from puzzles where origin = 'COMMUNITY'`).Scan(&puzzlesCreated); err != nil {
		t.Fatal(err)
	}
	if puzzlesCreated != 1 {
		t.Fatalf("expected one durable community puzzle, got %d", puzzlesCreated)
	}
	var keysStored int
	if err := puzzleStore.db.QueryRow(`select count(*) from idempotency_keys`).Scan(&keysStored); err != nil {
		t.Fatal(err)
	}
	if keysStored != 1 {
		t.Fatalf("expected one idempotency record, got %d", keysStored)
	}
}

func TestGetUnknownPuzzleReturns404(t *testing.T) {
	handler, _ := newAdminTestServer(t)

	response := adminRequest(t, handler, http.MethodGet, "/api/puzzles/does-not-exist", "", nil)
	if response.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown puzzle, got %d", response.Code)
	}
}
