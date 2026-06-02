package vibegrid

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		Puzzles: SeedPuzzles(),
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

func TestDuplicateClientGuessIsIdempotent(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles: SeedPuzzles(),
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
		Puzzles: SeedPuzzles(),
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

func fixedClock() time.Time {
	return time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
}
