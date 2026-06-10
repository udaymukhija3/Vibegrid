package vibegrid

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestPuzzleTemplatesPassValidation guarantees every starter can actually be
// published: each template is run through the same AdminPuzzleInput.Validate the
// community create endpoint uses, so "Play this" can never hit a validation error.
func TestPuzzleTemplatesPassValidation(t *testing.T) {
	templates := PuzzleTemplates()
	if len(templates) < 4 {
		t.Fatalf("expected several templates, got %d", len(templates))
	}

	ids := map[string]bool{}
	for _, tpl := range templates {
		if tpl.ID == "" || tpl.Title == "" {
			t.Fatalf("template missing id/title: %#v", tpl)
		}
		if ids[tpl.ID] {
			t.Fatalf("duplicate template id %q", tpl.ID)
		}
		ids[tpl.ID] = true

		input := AdminPuzzleInput{Difficulty: tpl.Difficulty, Groups: make([]AdminGroupInput, len(tpl.Groups))}
		for index, group := range tpl.Groups {
			input.Groups[index] = AdminGroupInput{Name: group.Name, Explanation: group.Explanation, Tiles: group.Tiles}
		}
		if err := input.Validate(); err != nil {
			t.Fatalf("template %q (%s) fails community validation: %v", tpl.ID, tpl.Title, err)
		}
	}
}

func TestPuzzleTemplatesEndpoint(t *testing.T) {
	handler := NewServer(ServerConfig{Puzzles: StaticPuzzleSource(SeedPuzzles())})

	req := httptest.NewRequest(http.MethodGet, "/api/puzzle-templates", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Templates []PuzzleTemplate `json:"templates"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Templates) != len(PuzzleTemplates()) {
		t.Fatalf("expected %d templates, got %d", len(PuzzleTemplates()), len(payload.Templates))
	}
}
