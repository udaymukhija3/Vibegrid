package vibegrid

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeCommunityStore struct{}

func (fakeCommunityStore) CreateCommunityPuzzle(_ context.Context, input AdminPuzzleInput) (Puzzle, error) {
	puzzle := input.toPuzzle(999)
	puzzle.Status = PuzzleStatusPublished
	puzzle.Origin = OriginCommunity
	return puzzle, nil
}

func TestCommunityCreateNeedsNoToken(t *testing.T) {
	handler, _ := newAdminTestServer(t)

	// No Authorization header at all.
	response := adminRequest(t, handler, http.MethodPost, "/api/community/puzzles", "", validPuzzleInput())
	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201 for community create without token, got %d: %s", response.Code, response.Body.String())
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

func TestClientIPUsesTrustedProxyHeaders(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/community/puzzles", nil)
	request.RemoteAddr = "203.0.113.10:12345"
	request.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2")
	request.Header.Set("X-Real-IP", "198.51.100.9")
	request.Header.Set("Fly-Client-IP", "198.51.100.8")

	if got := clientIP(request); got != "198.51.100.8" {
		t.Fatalf("expected Fly-Client-IP, got %q", got)
	}

	request.Header.Del("Fly-Client-IP")
	if got := clientIP(request); got != "198.51.100.9" {
		t.Fatalf("expected X-Real-IP fallback, got %q", got)
	}

	request.Header.Del("X-Real-IP")
	if got := clientIP(request); got != "203.0.113.10" {
		t.Fatalf("expected RemoteAddr fallback, got %q", got)
	}
}

func TestCommunityPuzzlePlayableByLinkButNotInDaily(t *testing.T) {
	handler, _ := newAdminTestServer(t)

	created := adminRequest(t, handler, http.MethodPost, "/api/community/puzzles", "", validPuzzleInput())
	if created.Code != http.StatusCreated {
		t.Fatalf("create failed: %d %s", created.Code, created.Body.String())
	}
	var body createdPuzzleResponse
	if err := json.NewDecoder(created.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}

	// Playable by direct link.
	play := adminRequest(t, handler, http.MethodGet, "/api/puzzles/"+body.ID, "", nil)
	if play.Code != http.StatusOK {
		t.Fatalf("expected 200 fetching community puzzle by id, got %d", play.Code)
	}
	var public PublicPuzzle
	if err := json.NewDecoder(play.Body).Decode(&public); err != nil {
		t.Fatal(err)
	}
	if len(public.Tiles) != PuzzleGroupCount*GroupSize {
		t.Fatalf("expected %d tiles, got %d", PuzzleGroupCount*GroupSize, len(public.Tiles))
	}

	// Absent from the public published (daily/archive) list.
	list := adminRequest(t, handler, http.MethodGet, "/api/puzzles", "", nil)
	var published []PublicPuzzle
	if err := json.NewDecoder(list.Body).Decode(&published); err != nil {
		t.Fatal(err)
	}
	for _, puzzle := range published {
		if puzzle.ID == body.ID {
			t.Fatalf("community puzzle %s must not appear in the daily/archive list", body.ID)
		}
	}
}

func TestGetUnknownPuzzleReturns404(t *testing.T) {
	handler, _ := newAdminTestServer(t)

	response := adminRequest(t, handler, http.MethodGet, "/api/puzzles/does-not-exist", "", nil)
	if response.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown puzzle, got %d", response.Code)
	}
}
