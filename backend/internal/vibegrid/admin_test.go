package vibegrid

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

const testAdminToken = "test-admin-token"

// newAdminTestServer connects to TEST_DATABASE_URL, clears all puzzle and
// attempt data, and returns a server handler wired with admin enabled plus the
// underlying puzzle store. Skipped when no test database is configured.
func newAdminTestServer(t *testing.T) (http.Handler, *PostgresPuzzleStore) {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres admin tests")
	}

	database, err := OpenDB(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	if _, err := database.Exec(`truncate puzzles, attempts, attempt_guesses restart identity cascade`); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	puzzleStore := NewPostgresPuzzleStore(database)
	handler := NewServer(ServerConfig{
		Puzzles:      puzzleStore,
		Store:        NewPostgresAttemptStore(database),
		AdminPuzzles: puzzleStore,
		Community:    puzzleStore,
		AdminToken:   testAdminToken,
		Clock:        fixedClock,
	})
	return handler, puzzleStore
}

func validPuzzleInput() AdminPuzzleInput {
	return AdminPuzzleInput{
		Difficulty: DifficultyMedium,
		Groups: []AdminGroupInput{
			{Name: "Italian summer", Explanation: "Sun, fabric, wheels.", Tiles: []string{"espresso", "linen", "Vespa", "balcony"}},
			{Name: "Corporate panic", Explanation: "The meeting before the meeting.", Tiles: []string{"Slack", "deck", "panic", "9:59"}},
			{Name: "Noir evening", Explanation: "Moody window scene.", Tiles: []string{"rain", "jazz", "window", "lamp"}},
			{Name: "Gym bro", Explanation: "Breakfast and the lift.", Tiles: []string{"oats", "whey", "hoodie", "deadlift"}},
		},
	}
}

func adminRequest(t *testing.T, handler http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var reader *bytes.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(payload)
	} else {
		reader = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func TestAdminRequiresValidToken(t *testing.T) {
	handler, _ := newAdminTestServer(t)

	noToken := adminRequest(t, handler, http.MethodGet, "/api/admin/puzzles", "", nil)
	if noToken.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", noToken.Code)
	}

	wrongToken := adminRequest(t, handler, http.MethodGet, "/api/admin/puzzles", "nope", nil)
	if wrongToken.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong token, got %d", wrongToken.Code)
	}
}

func TestAdminRejectsInvalidPuzzle(t *testing.T) {
	handler, _ := newAdminTestServer(t)

	input := validPuzzleInput()
	input.Groups[0].Tiles = []string{"only", "three", "tiles"} // too few

	response := adminRequest(t, handler, http.MethodPost, "/api/admin/puzzles", testAdminToken, input)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for malformed puzzle, got %d: %s", response.Code, response.Body.String())
	}
}

func TestAdminRejectsDuplicateTiles(t *testing.T) {
	handler, _ := newAdminTestServer(t)

	input := validPuzzleInput()
	input.Groups[1].Tiles[0] = "espresso" // already used in group 0

	response := adminRequest(t, handler, http.MethodPost, "/api/admin/puzzles", testAdminToken, input)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for duplicate tile, got %d: %s", response.Code, response.Body.String())
	}
}

func TestAdminRejectsOverlongPuzzleFields(t *testing.T) {
	handler, _ := newAdminTestServer(t)

	input := validPuzzleInput()
	input.Groups[0].Name = stringOfLength(MaxGroupNameLength + 1)

	response := adminRequest(t, handler, http.MethodPost, "/api/admin/puzzles", testAdminToken, input)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for overlong group name, got %d: %s", response.Code, response.Body.String())
	}

	input = validPuzzleInput()
	input.Groups[0].Explanation = stringOfLength(MaxGroupExplanationLength + 1)

	response = adminRequest(t, handler, http.MethodPost, "/api/admin/puzzles", testAdminToken, input)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for overlong explanation, got %d: %s", response.Code, response.Body.String())
	}

	input = validPuzzleInput()
	input.Groups[0].Tiles[0] = stringOfLength(MaxTileTextLength + 1)

	response = adminRequest(t, handler, http.MethodPost, "/api/admin/puzzles", testAdminToken, input)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for overlong tile, got %d: %s", response.Code, response.Body.String())
	}
}

func TestAdminCreateAndPublishFlow(t *testing.T) {
	handler, store := newAdminTestServer(t)

	// Create a draft.
	created := adminRequest(t, handler, http.MethodPost, "/api/admin/puzzles", testAdminToken, validPuzzleInput())
	if created.Code != http.StatusCreated {
		t.Fatalf("expected 201 on create, got %d: %s", created.Code, created.Body.String())
	}
	var draft Puzzle
	if err := json.NewDecoder(created.Body).Decode(&draft); err != nil {
		t.Fatal(err)
	}
	if draft.Status != PuzzleStatusDraft || len(draft.Groups) != PuzzleGroupCount {
		t.Fatalf("unexpected draft: %#v", draft)
	}

	// A draft is not visible on the public published endpoint yet.
	if puzzles, err := store.Puzzles(context.Background()); err != nil {
		t.Fatal(err)
	} else if got := len(PublishedPuzzles(puzzles)); got != 0 {
		t.Fatalf("expected no published puzzles before publish, got %d", got)
	}

	// Publish it for a date.
	published := adminRequest(t, handler, http.MethodPost, "/api/admin/puzzles/"+draft.ID+"/publish", testAdminToken, publishRequest{PublishDate: "2026-07-01"})
	if published.Code != http.StatusOK {
		t.Fatalf("expected 200 on publish, got %d: %s", published.Code, published.Body.String())
	}

	puzzles, err := store.Puzzles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := len(PublishedPuzzles(puzzles)); got != 1 {
		t.Fatalf("expected one published puzzle, got %d", got)
	}
}

func TestAdminPublishRejectsDuplicateDate(t *testing.T) {
	handler, _ := newAdminTestServer(t)

	first := mustCreateDraft(t, handler)
	second := mustCreateDraft(t, handler)

	ok := adminRequest(t, handler, http.MethodPost, "/api/admin/puzzles/"+first+"/publish", testAdminToken, publishRequest{PublishDate: "2026-08-01"})
	if ok.Code != http.StatusOK {
		t.Fatalf("expected first publish to succeed, got %d: %s", ok.Code, ok.Body.String())
	}

	conflict := adminRequest(t, handler, http.MethodPost, "/api/admin/puzzles/"+second+"/publish", testAdminToken, publishRequest{PublishDate: "2026-08-01"})
	if conflict.Code != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate publish date, got %d: %s", conflict.Code, conflict.Body.String())
	}
}

func mustCreateDraft(t *testing.T, handler http.Handler) string {
	t.Helper()

	response := adminRequest(t, handler, http.MethodPost, "/api/admin/puzzles", testAdminToken, validPuzzleInput())
	if response.Code != http.StatusCreated {
		t.Fatalf("create draft failed: %d %s", response.Code, response.Body.String())
	}
	var puzzle Puzzle
	if err := json.NewDecoder(response.Body).Decode(&puzzle); err != nil {
		t.Fatal(err)
	}
	return puzzle.ID
}

func stringOfLength(length int) string {
	value := make([]byte, length)
	for index := range value {
		value[index] = 'a'
	}
	return string(value)
}
