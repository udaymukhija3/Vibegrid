package vibegrid

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type memoryIdempotencyRecord struct {
	requestHash string
	response    idempotencyResponse
}

type memoryIdempotencyStore struct {
	mu      sync.Mutex
	records map[string]memoryIdempotencyRecord
}

func newMemoryIdempotencyStore() *memoryIdempotencyStore {
	return &memoryIdempotencyStore{records: map[string]memoryIdempotencyRecord{}}
}

func (store *memoryIdempotencyStore) Execute(
	ctx context.Context,
	scope, keyHash, requestHash string,
	action func(context.Context) idempotencyResponse,
) (idempotencyResponse, bool, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	recordKey := scope + ":" + keyHash
	if record, ok := store.records[recordKey]; ok {
		if record.requestHash != requestHash {
			return idempotencyResponse{}, false, true, nil
		}
		return cloneIdempotencyResponse(record.response), true, false, nil
	}
	response := action(ctx)
	if response.status >= http.StatusOK && response.status < http.StatusMultipleChoices {
		store.records[recordKey] = memoryIdempotencyRecord{
			requestHash: requestHash,
			response:    cloneIdempotencyResponse(response),
		}
	}
	return response, false, false, nil
}

func (*memoryIdempotencyStore) PruneExpired(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}

func cloneIdempotencyResponse(response idempotencyResponse) idempotencyResponse {
	return idempotencyResponse{
		status: response.status,
		header: response.header.Clone(),
		body:   append([]byte(nil), response.body...),
	}
}

type countingCommunityStore struct {
	mu    sync.Mutex
	calls int
}

type countingModerationStore struct {
	mu          sync.Mutex
	reportCalls int
	appealCalls int
}

func (store *countingModerationStore) CreateReport(_ context.Context, input ReportInput, _ string) (ModerationReport, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.reportCalls++
	return ModerationReport{ID: "report-created", PuzzleID: input.PuzzleID}, nil
}

func (*countingModerationStore) ListReports(context.Context) ([]ModerationReport, error) {
	return nil, nil
}

func (*countingModerationStore) ResolveReport(context.Context, string, string, string, string) (ModerationReport, error) {
	return ModerationReport{}, nil
}

func (store *countingModerationStore) CreateAppeal(_ context.Context, input AppealInput) (ModerationAppeal, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.appealCalls++
	return ModerationAppeal{ID: "appeal-created", PuzzleID: input.PuzzleID}, nil
}

func (*countingModerationStore) ListAppeals(context.Context) ([]ModerationAppeal, error) {
	return nil, nil
}

func (*countingModerationStore) ResolveAppeal(context.Context, string, string, string) (ModerationAppeal, error) {
	return ModerationAppeal{}, nil
}

func (*countingModerationStore) AddAction(context.Context, ModerationActionInput) error {
	return nil
}

func (*countingModerationStore) AuditLog(context.Context, int) ([]ModerationAction, error) {
	return nil, nil
}

type countingAdminStore struct {
	mu    sync.Mutex
	calls int
}

func (store *countingAdminStore) CreateDraft(_ context.Context, input AdminPuzzleInput) (Puzzle, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.calls++
	return input.toPuzzle(store.calls), nil
}

func (*countingAdminStore) Publish(context.Context, string, string) error  { return nil }
func (*countingAdminStore) ApproveCommunity(context.Context, string) error { return nil }
func (*countingAdminStore) Archive(context.Context, string) error          { return nil }
func (*countingAdminStore) Reinstate(context.Context, string) error        { return nil }
func (*countingAdminStore) PersistDaily(context.Context, Puzzle) error     { return nil }

func (store *countingAdminStore) callCount() int {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.calls
}

func (store *countingCommunityStore) CreateCommunityPuzzle(_ context.Context, input AdminPuzzleInput, _ string) (Puzzle, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.calls++
	puzzle := input.toPuzzle(store.calls)
	puzzle.Status = PuzzleStatusPending
	puzzle.Origin = OriginCommunity
	return puzzle, nil
}

func (store *countingCommunityStore) CreatorStatus(context.Context, string, string) (CreatorPuzzleStatus, error) {
	return CreatorPuzzleStatus{ID: SeedPuzzles()[0].ID, Status: PuzzleStatusArchived}, nil
}

func (store *countingCommunityStore) WithdrawCommunityPuzzle(context.Context, string, string) (CreatorPuzzleStatus, error) {
	return CreatorPuzzleStatus{}, ErrCreatorClaimInvalid
}

func (store *countingCommunityStore) callCount() int {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.calls
}

func TestCommunityCreateReplaysIdempotentResponse(t *testing.T) {
	community := &countingCommunityStore{}
	handler := NewServer(ServerConfig{
		Puzzles:     StaticPuzzleSource(SeedPuzzles()),
		Community:   community,
		Idempotency: newMemoryIdempotencyStore(),
		Clock:       fixedClock,
	})
	payload, err := json.Marshal(validPuzzleInput())
	if err != nil {
		t.Fatal(err)
	}

	first := postIdempotentJSON(handler, "/api/community/puzzles", payload, "community-create-1", nil)
	if first.Code != http.StatusAccepted {
		t.Fatalf("first create failed: %d %s", first.Code, first.Body.String())
	}
	sessionCookie := responseCookie(first, sessionCookieName)
	if sessionCookie == nil {
		t.Fatal("idempotent public mutation did not establish a guest session")
	}

	second := postIdempotentJSON(handler, "/api/community/puzzles", payload, "community-create-1", sessionCookie)
	if second.Code != http.StatusAccepted {
		t.Fatalf("replayed create failed: %d %s", second.Code, second.Body.String())
	}
	if second.Header().Get(idempotencyReplayHeader) != "true" {
		t.Fatal("replayed response did not identify itself")
	}
	if first.Body.String() != second.Body.String() {
		t.Fatalf("replay changed the original response:\nfirst: %s\nsecond: %s", first.Body.String(), second.Body.String())
	}
	if community.callCount() != 1 {
		t.Fatalf("idempotent replay created %d puzzles", community.callCount())
	}

	changed := validPuzzleInput()
	changed.Groups[0].Name = "Different request"
	changedPayload, err := json.Marshal(changed)
	if err != nil {
		t.Fatal(err)
	}
	conflict := postIdempotentJSON(handler, "/api/community/puzzles", changedPayload, "community-create-1", sessionCookie)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("expected key reuse with a different body to conflict, got %d: %s", conflict.Code, conflict.Body.String())
	}
	if community.callCount() != 1 {
		t.Fatalf("conflicting key reuse created %d puzzles", community.callCount())
	}
}

func TestIdempotencyKeyValidationRunsBeforeMutation(t *testing.T) {
	community := &countingCommunityStore{}
	handler := NewServer(ServerConfig{
		Puzzles:     StaticPuzzleSource(SeedPuzzles()),
		Community:   community,
		Idempotency: newMemoryIdempotencyStore(),
	})
	payload, err := json.Marshal(validPuzzleInput())
	if err != nil {
		t.Fatal(err)
	}

	response := postIdempotentJSON(handler, "/api/community/puzzles", payload, "bad key", nil)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid idempotency key to return 400, got %d", response.Code)
	}
	if community.callCount() != 0 {
		t.Fatal("invalid idempotency key reached the mutation")
	}
}

func TestReportAndAppealCreationReplayIdempotently(t *testing.T) {
	moderation := &countingModerationStore{}
	community := &countingCommunityStore{}
	handler := NewServer(ServerConfig{
		Puzzles:     StaticPuzzleSource(SeedPuzzles()),
		Moderation:  moderation,
		Community:   community,
		Idempotency: newMemoryIdempotencyStore(),
		Clock:       fixedClock,
	})
	reportPayload, err := json.Marshal(ReportInput{
		PuzzleID: SeedPuzzles()[0].ID,
		Reason:   "SPAM",
		Details:  "duplicate retry test",
	})
	if err != nil {
		t.Fatal(err)
	}
	firstReport := postIdempotentJSON(handler, "/api/reports", reportPayload, "report-create-1", nil)
	if firstReport.Code != http.StatusCreated {
		t.Fatalf("first report failed: %d %s", firstReport.Code, firstReport.Body.String())
	}
	sessionCookie := responseCookie(firstReport, sessionCookieName)
	secondReport := postIdempotentJSON(handler, "/api/reports", reportPayload, "report-create-1", sessionCookie)
	if secondReport.Code != http.StatusCreated || secondReport.Header().Get(idempotencyReplayHeader) != "true" {
		t.Fatalf("report replay failed: %d %s", secondReport.Code, secondReport.Body.String())
	}

	appealPayload, err := json.Marshal(AppealInput{
		PuzzleID: SeedPuzzles()[0].ID,
		Message:  "duplicate retry test",
	})
	if err != nil {
		t.Fatal(err)
	}
	claimSecret := newCreatorClaimSecret()
	firstAppeal := postCreatorIdempotentJSON(handler, "/api/appeals", appealPayload, "appeal-create-1", claimSecret)
	secondAppeal := postCreatorIdempotentJSON(handler, "/api/appeals", appealPayload, "appeal-create-1", claimSecret)
	if firstAppeal.Code != http.StatusCreated || secondAppeal.Code != http.StatusCreated || secondAppeal.Header().Get(idempotencyReplayHeader) != "true" {
		t.Fatalf("appeal replay failed: first=%d second=%d", firstAppeal.Code, secondAppeal.Code)
	}

	moderation.mu.Lock()
	defer moderation.mu.Unlock()
	if moderation.reportCalls != 1 || moderation.appealCalls != 1 {
		t.Fatalf("expected one report and one appeal mutation, got reports=%d appeals=%d", moderation.reportCalls, moderation.appealCalls)
	}
}

func TestAdminDraftCreationReplaysIdempotently(t *testing.T) {
	adminStore := &countingAdminStore{}
	handler := NewServer(ServerConfig{
		Puzzles:      StaticPuzzleSource(SeedPuzzles()),
		AdminPuzzles: adminStore,
		AdminToken:   testAdminToken,
		Idempotency:  newMemoryIdempotencyStore(),
	})
	payload, err := json.Marshal(validPuzzleInput())
	if err != nil {
		t.Fatal(err)
	}

	first := postIdempotentAdminJSON(handler, "/api/admin/puzzles", payload, "admin-draft-create-1")
	second := postIdempotentAdminJSON(handler, "/api/admin/puzzles", payload, "admin-draft-create-1")
	if first.Code != http.StatusCreated || second.Code != http.StatusCreated {
		t.Fatalf("admin draft replay failed: first=%d second=%d", first.Code, second.Code)
	}
	if second.Header().Get(idempotencyReplayHeader) != "true" {
		t.Fatal("admin draft replay did not identify itself")
	}
	if first.Body.String() != second.Body.String() || adminStore.callCount() != 1 {
		t.Fatalf("admin draft was not replayed exactly; calls=%d", adminStore.callCount())
	}
}

// The tests above all run against memoryIdempotencyStore, which is why a store
// that could not persist a single record at all still shipped. These two drive
// the real Postgres store: the first covers an ordinary sequential replay, the
// second the lost-race path where the durable record already exists.
func TestPostgresIdempotentCreateReplaysThroughTheDatabase(t *testing.T) {
	handler, puzzleStore := newAdminTestServer(t)
	payload, err := json.Marshal(validPuzzleInput())
	if err != nil {
		t.Fatal(err)
	}

	first := postIdempotentJSON(handler, "/api/community/puzzles", payload, "durable-community-create", nil)
	if first.Code != http.StatusAccepted {
		t.Fatalf("first create failed: %d %s", first.Code, first.Body.String())
	}
	sessionCookie := responseCookie(first, sessionCookieName)
	if sessionCookie == nil {
		t.Fatal("idempotent public mutation did not establish a guest session")
	}

	second := postIdempotentJSON(handler, "/api/community/puzzles", payload, "durable-community-create", sessionCookie)
	if second.Code != http.StatusAccepted {
		t.Fatalf("replayed create failed: %d %s", second.Code, second.Body.String())
	}
	if second.Header().Get(idempotencyReplayHeader) != "true" {
		t.Fatal("replayed response did not identify itself")
	}
	if first.Body.String() != second.Body.String() {
		t.Fatalf("replay changed the original response:\nfirst: %s\nsecond: %s", first.Body.String(), second.Body.String())
	}
	// The stored headers live in a jsonb column, so a replay that loses them is
	// how a driver-level encoding problem would show up short of an outright error.
	if second.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("replay lost its stored headers: %v", second.Header())
	}

	var puzzlesCreated int
	if err := puzzleStore.db.QueryRow(`select count(*) from puzzles where origin = 'COMMUNITY'`).Scan(&puzzlesCreated); err != nil {
		t.Fatal(err)
	}
	if puzzlesCreated != 1 {
		t.Fatalf("expected one durable community puzzle, got %d", puzzlesCreated)
	}
}

func TestPostgresIdempotencyReplaysTheWinnerWhenTheKeyIsRaced(t *testing.T) {
	_, puzzleStore := newAdminTestServer(t)
	database := puzzleStore.db
	store := NewPostgresIdempotencyStore(database)

	scope := "community-puzzle.create:" + digestString("guest:raced")
	keyHash := digestString("raced-key")
	requestHash := digestString("raced-body")
	winnerBody := []byte(`{"ok":true,"id":"winner"}`)

	actionRuns := 0
	response, replayed, conflict, err := store.Execute(
		context.Background(), scope, keyHash, requestHash,
		func(context.Context) idempotencyResponse {
			actionRuns++
			// Commit the same key from outside this transaction, which is what a
			// writer that never took the advisory lock would do. Execute's own
			// insert must then lose to the primary key.
			if _, err := database.Exec(
				`insert into idempotency_keys
				 (scope, key_hash, request_hash, status_code, response_headers, response_body)
				 values ($1, $2, $3, $4, $5::jsonb, $6)`,
				scope, keyHash, requestHash, http.StatusAccepted,
				`{"Content-Type":["application/json"]}`, winnerBody,
			); err != nil {
				t.Errorf("seed the winning record: %v", err)
			}
			return idempotencyResponse{
				status: http.StatusAccepted,
				header: http.Header{"Content-Type": []string{"application/json"}},
				body:   []byte(`{"ok":true,"id":"loser"}`),
			}
		},
	)
	if err != nil {
		t.Fatalf("losing a race must not fail the request: %v", err)
	}
	if conflict {
		t.Fatal("a matching request hash is not a conflict")
	}
	if !replayed {
		t.Fatal("the loser served its own response instead of replaying the winner's")
	}
	if response.status != http.StatusAccepted || string(response.body) != string(winnerBody) {
		t.Fatalf("expected the winner's response, got %d %s", response.status, response.body)
	}
	if actionRuns != 1 {
		t.Fatalf("expected the mutation to run once, ran %d times", actionRuns)
	}

	var keysStored int
	if err := database.QueryRow(`select count(*) from idempotency_keys`).Scan(&keysStored); err != nil {
		t.Fatal(err)
	}
	if keysStored != 1 {
		t.Fatalf("expected one idempotency record, got %d", keysStored)
	}
}

func postIdempotentJSON(handler http.Handler, path string, payload []byte, key string, cookie *http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(idempotencyHeader, key)
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func postCreatorIdempotentJSON(handler http.Handler, path string, payload []byte, key, claimSecret string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(idempotencyHeader, key)
	request.Header.Set(creatorClaimHeader, claimSecret)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func postIdempotentAdminJSON(handler http.Handler, path string, payload []byte, key string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+testAdminToken)
	request.Header.Set(idempotencyHeader, key)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func responseCookie(response *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}
