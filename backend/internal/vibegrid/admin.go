package vibegrid

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// PuzzleGroupCount is the number of groups in a valid puzzle (four groups of
// GroupSize tiles each).
const PuzzleGroupCount = 4

const maxAdminBodyBytes = 64 << 10 // 64 KiB

// AdminPuzzleStore is the write side of puzzle authoring. Only the Postgres
// store implements it; admin endpoints are unavailable without a database.
type AdminPuzzleStore interface {
	CreateDraft(ctx context.Context, input AdminPuzzleInput) (Puzzle, error)
	Publish(ctx context.Context, puzzleID, publishDate string) error
}

type AdminGroupInput struct {
	Name        string   `json:"name"`
	Explanation string   `json:"explanation"`
	Tiles       []string `json:"tiles"`
}

type AdminPuzzleInput struct {
	Difficulty Difficulty        `json:"difficulty"`
	Groups     []AdminGroupInput `json:"groups"`
}

type publishRequest struct {
	PublishDate string `json:"publishDate"`
}

// Validate enforces puzzle structure server-side so a malformed or unfair puzzle
// can never be persisted: exactly four groups of four tiles, non-empty labels,
// and sixteen tiles unique across the whole board.
func (input AdminPuzzleInput) Validate() error {
	if len(input.Groups) != PuzzleGroupCount {
		return fmt.Errorf("a puzzle needs exactly %d groups, got %d", PuzzleGroupCount, len(input.Groups))
	}

	seenTiles := map[string]bool{}
	for index, group := range input.Groups {
		if strings.TrimSpace(group.Name) == "" {
			return fmt.Errorf("group %d is missing a name", index+1)
		}
		if strings.TrimSpace(group.Explanation) == "" {
			return fmt.Errorf("group %q is missing an explanation", group.Name)
		}
		if len(group.Tiles) != GroupSize {
			return fmt.Errorf("group %q needs exactly %d tiles, got %d", group.Name, GroupSize, len(group.Tiles))
		}
		for _, tile := range group.Tiles {
			text := strings.TrimSpace(tile)
			if text == "" {
				return fmt.Errorf("group %q has an empty tile", group.Name)
			}
			key := strings.ToLower(text)
			if seenTiles[key] {
				return fmt.Errorf("tile %q appears more than once; tiles must be unique across the puzzle", text)
			}
			seenTiles[key] = true
		}
	}

	switch input.Difficulty {
	case "", DifficultyEasy, DifficultyMedium, DifficultyHard:
		return nil
	default:
		return fmt.Errorf("difficulty %q is not one of EASY, MEDIUM, HARD", input.Difficulty)
	}
}

// toPuzzle builds a persistable DRAFT puzzle, generating ids and assigning color
// and sort order by position. Tile text is trimmed to match validation.
func (input AdminPuzzleInput) toPuzzle(puzzleNumber int) Puzzle {
	difficulty := input.Difficulty
	if difficulty == "" {
		difficulty = DifficultyMedium
	}

	groups := make([]PuzzleGroup, 0, len(input.Groups))
	for index, group := range input.Groups {
		tiles := make([]Tile, 0, len(group.Tiles))
		for _, tile := range group.Tiles {
			tiles = append(tiles, Tile{ID: newID("tile"), Text: strings.TrimSpace(tile)})
		}
		groups = append(groups, PuzzleGroup{
			ID:          newID("grp"),
			Name:        strings.TrimSpace(group.Name),
			Explanation: strings.TrimSpace(group.Explanation),
			ColorIndex:  index,
			Tiles:       tiles,
		})
	}

	return Puzzle{
		ID:           newID("pzl"),
		PuzzleNumber: puzzleNumber,
		Status:       PuzzleStatusDraft,
		Difficulty:   difficulty,
		Groups:       groups,
	}
}

// requireAdmin gates admin handlers behind a bearer token compared in constant
// time. With no token configured or no database wired, admin is unavailable.
func (server *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if server.adminToken == "" || server.adminPuzzles == nil {
			writeError(w, http.StatusServiceUnavailable, "Admin is not configured on this server.")
			return
		}

		token := bearerToken(r)
		if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(server.adminToken)) != 1 {
			writeError(w, http.StatusUnauthorized, "Admin authorization required.")
			return
		}

		next(w, r)
	}
}

func bearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(header) > len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
		return strings.TrimSpace(header[len(prefix):])
	}
	return ""
}

func (server *Server) handleAdminListPuzzles(w http.ResponseWriter, r *http.Request) {
	puzzles, err := server.puzzles.Puzzles(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load puzzles.")
		return
	}
	writeJSON(w, http.StatusOK, puzzles)
}

func (server *Server) handleAdminCreatePuzzle(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAdminBodyBytes)
	var input AdminPuzzleInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "That puzzle payload is not valid JSON.")
		return
	}

	if err := input.Validate(); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	puzzle, err := server.adminPuzzles.CreateDraft(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not save that draft.")
		return
	}
	writeJSON(w, http.StatusCreated, puzzle)
}

func (server *Server) handleAdminPublishPuzzle(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAdminBodyBytes)
	puzzleID := r.PathValue("id")
	if puzzleID == "" {
		writeError(w, http.StatusBadRequest, "Puzzle id is required.")
		return
	}

	var request publishRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "That publish payload is not valid JSON.")
		return
	}
	if !isValidDate(request.PublishDate) {
		writeError(w, http.StatusUnprocessableEntity, "publishDate must be a YYYY-MM-DD date.")
		return
	}

	err := server.adminPuzzles.Publish(r.Context(), puzzleID, request.PublishDate)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case errors.Is(err, ErrPublishDateTaken):
		writeError(w, http.StatusConflict, "A puzzle is already published for that date.")
	case errors.Is(err, ErrPuzzleNotFound):
		writeError(w, http.StatusNotFound, "Puzzle not found.")
	default:
		writeError(w, http.StatusInternalServerError, "Could not publish that puzzle.")
	}
}

// isValidDate checks for a strict YYYY-MM-DD calendar date.
func isValidDate(value string) bool {
	if len(value) != len("2006-01-02") {
		return false
	}
	_, err := time.Parse("2006-01-02", value)
	return err == nil
}
