package vibegrid

import (
	"encoding/json"
	"net/http"
	"testing"
)

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
