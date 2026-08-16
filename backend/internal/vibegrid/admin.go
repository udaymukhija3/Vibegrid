package vibegrid

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

// PuzzleGroupCount is the number of groups in a valid puzzle (four groups of
// GroupSize tiles each).
const PuzzleGroupCount = 4

const maxAdminBodyBytes = 64 << 10 // 64 KiB

const (
	MaxGroupNameLength        = 64
	MaxGroupExplanationLength = 160
	MaxTileTextLength         = 32
)

// AdminPuzzleStore is the write side of puzzle authoring. Only the Postgres
// store implements it; admin endpoints are unavailable without a database.
type AdminPuzzleStore interface {
	CreateDraft(ctx context.Context, input AdminPuzzleInput) (Puzzle, error)
	Publish(ctx context.Context, puzzleID, publishDate string) error
	ApproveCommunity(ctx context.Context, puzzleID string) error
	Archive(ctx context.Context, puzzleID string) error
	Reinstate(ctx context.Context, puzzleID string) error
	PersistDaily(ctx context.Context, puzzle Puzzle) error
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

type adminPuzzlePreviewResponse struct {
	Puzzle PublicPuzzle  `json:"puzzle"`
	Groups []SolvedGroup `json:"groups"`
}

type adminQueueHealthResponse struct {
	Today              string          `json:"today"`
	Days               []adminQueueDay `json:"days"`
	Drafts             int             `json:"drafts"`
	PendingCommunity   int             `json:"pendingCommunity"`
	ScheduledEditorial int             `json:"scheduledEditorial"`
	EvergreenFallbacks int             `json:"evergreenFallbacks"`
}

type adminQueueDay struct {
	Date         string `json:"date"`
	Coverage     string `json:"coverage"`
	PuzzleID     string `json:"puzzleId,omitempty"`
	PuzzleNumber int    `json:"puzzleNumber,omitempty"`
}

const (
	defaultQueueHealthDays = 14
	maxQueueHealthDays     = 30

	queueCoverageEditorial   = "EDITORIAL"
	queueCoverageEvergreen   = "EVERGREEN"
	queueCoverageUnscheduled = "UNSCHEDULED"
)

// Validate enforces puzzle structure server-side so a malformed or unfair puzzle
// can never be persisted: exactly four groups of four tiles, non-empty labels,
// and sixteen tiles unique across the whole board.
func (input AdminPuzzleInput) Validate() error {
	if len(input.Groups) != PuzzleGroupCount {
		return fmt.Errorf("a puzzle needs exactly %d groups, got %d", PuzzleGroupCount, len(input.Groups))
	}

	seenTiles := map[string]bool{}
	for index, group := range input.Groups {
		name := strings.TrimSpace(group.Name)
		explanation := strings.TrimSpace(group.Explanation)
		if name == "" {
			return fmt.Errorf("group %d is missing a name", index+1)
		}
		if overRuneLimit(name, MaxGroupNameLength) {
			return fmt.Errorf("group %d name is too long; keep it to %d characters", index+1, MaxGroupNameLength)
		}
		if explanation == "" {
			return fmt.Errorf("group %q is missing an explanation", name)
		}
		if overRuneLimit(explanation, MaxGroupExplanationLength) {
			return fmt.Errorf(
				"group %q explanation is too long; keep it to %d characters",
				name,
				MaxGroupExplanationLength,
			)
		}
		if len(group.Tiles) != GroupSize {
			return fmt.Errorf("group %q needs exactly %d tiles, got %d", name, GroupSize, len(group.Tiles))
		}
		for _, tile := range group.Tiles {
			text := strings.TrimSpace(tile)
			if text == "" {
				return fmt.Errorf("group %q has an empty tile", name)
			}
			if overRuneLimit(text, MaxTileTextLength) {
				return fmt.Errorf("tile %q is too long; keep tiles to %d characters", text, MaxTileTextLength)
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

func overRuneLimit(value string, max int) bool {
	return utf8.RuneCountInString(value) > max
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
		Origin:       OriginEditorial,
		Groups:       groups,
	}
}

// requireAdmin gates admin handlers behind either the opaque, revocable admin
// session cookie used by the browser UI or the legacy bearer token used by scripts.
func (server *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hasSessionAuth := server.adminPassword != "" && server.adminSessionSecret != ""
		if server.adminPuzzles == nil || (server.adminToken == "" && !hasSessionAuth) {
			writeError(w, http.StatusServiceUnavailable, "Admin is not configured on this server.")
			return
		}

		if !server.isAdminRequest(r) {
			writeError(w, http.StatusUnauthorized, "Admin authorization required.")
			return
		}
		if requiresAdminCSRF(r.Method) && !server.validAdminBearerToken(r) && !server.validAdminCSRFToken(r) {
			writeError(w, http.StatusForbidden, "Admin CSRF token required.")
			return
		}
		next(w, r)
	}
}

func requiresAdminCSRF(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
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

func (server *Server) handleAdminQueueHealth(w http.ResponseWriter, r *http.Request) {
	days := clampedQueryInt(r, "days", defaultQueueHealthDays, 1, maxQueueHealthDays)
	puzzles, err := server.puzzles.Puzzles(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load queue health.")
		return
	}

	today := server.todayString()
	start, err := time.Parse(dateLayout, today)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not read launch calendar.")
		return
	}

	response := adminQueueHealthResponse{
		Today: today,
		Days:  make([]adminQueueDay, 0, days),
	}
	editorialByDate := make(map[string]Puzzle, len(puzzles))
	for _, puzzle := range puzzles {
		switch {
		case puzzle.Status == PuzzleStatusDraft && puzzle.Origin == OriginEditorial:
			response.Drafts++
		case puzzle.Status == PuzzleStatusPending && puzzle.Origin == OriginCommunity:
			response.PendingCommunity++
		case puzzle.Status == PuzzleStatusPublished && puzzle.Origin == OriginEditorial && puzzle.PublishDate != "":
			editorialByDate[puzzle.PublishDate] = puzzle
		}
	}

	for offset := 0; offset < days; offset++ {
		date := start.AddDate(0, 0, offset).Format(dateLayout)
		if puzzle, ok := editorialByDate[date]; ok {
			response.ScheduledEditorial++
			response.Days = append(response.Days, adminQueueDay{
				Date:         date,
				Coverage:     queueCoverageEditorial,
				PuzzleID:     puzzle.ID,
				PuzzleNumber: puzzle.PuzzleNumber,
			})
			continue
		}

		day := adminQueueDay{
			Date:     date,
			Coverage: queueCoverageUnscheduled,
		}
		if puzzle, err := server.puzzles.TodaysPuzzle(r.Context(), date); err == nil && puzzle.PublishDate == date && strings.HasPrefix(puzzle.ID, "vibegrid-") {
			day.Coverage = queueCoverageEvergreen
			day.PuzzleID = puzzle.ID
			day.PuzzleNumber = puzzle.PuzzleNumber
			response.EvergreenFallbacks++
		}
		response.Days = append(response.Days, day)
	}

	writeJSON(w, http.StatusOK, response)
}

func (server *Server) handleAdminCreatePuzzle(w http.ResponseWriter, r *http.Request) {
	var input AdminPuzzleInput
	if !decodeJSONBody(w, r, maxAdminBodyBytes, &input, "That puzzle payload is not valid JSON.") {
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
	puzzleID := r.PathValue("id")
	if !validPuzzleID(puzzleID) {
		writeError(w, http.StatusBadRequest, "Puzzle id is required.")
		return
	}

	var request publishRequest
	if !decodeJSONBody(w, r, maxAdminBodyBytes, &request, "That publish payload is not valid JSON.") {
		return
	}
	if !isValidDate(request.PublishDate) {
		writeError(w, http.StatusUnprocessableEntity, "publishDate must be a YYYY-MM-DD date.")
		return
	}

	err := server.adminPuzzles.Publish(r.Context(), puzzleID, request.PublishDate)
	switch {
	case err == nil:
		if server.moderation != nil {
			_ = server.moderation.AddAction(r.Context(), ModerationActionInput{
				PuzzleID: puzzleID,
				Actor:    server.adminActor(r),
				Action:   "PUZZLE_PUBLISHED",
			})
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case errors.Is(err, ErrPublishDateTaken):
		writeError(w, http.StatusConflict, "A puzzle is already published for that date.")
	case errors.Is(err, ErrPuzzleNotFound):
		writeError(w, http.StatusNotFound, "Puzzle not found.")
	default:
		writeError(w, http.StatusInternalServerError, "Could not publish that puzzle.")
	}
}

func (server *Server) handleAdminApproveCommunityPuzzle(w http.ResponseWriter, r *http.Request) {
	puzzleID := r.PathValue("id")
	if !validPuzzleID(puzzleID) {
		writeError(w, http.StatusBadRequest, "Puzzle id is required.")
		return
	}

	err := server.adminPuzzles.ApproveCommunity(r.Context(), puzzleID)
	switch {
	case err == nil:
		if server.moderation != nil {
			_ = server.moderation.AddAction(r.Context(), ModerationActionInput{
				PuzzleID: puzzleID,
				Actor:    server.adminActor(r),
				Action:   "COMMUNITY_PUZZLE_APPROVED",
			})
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case errors.Is(err, ErrPuzzleNotFound):
		writeError(w, http.StatusNotFound, "Puzzle not found.")
	case errors.Is(err, ErrPuzzleNotPending):
		writeError(w, http.StatusConflict, "Only pending community puzzles can be approved.")
	default:
		writeError(w, http.StatusInternalServerError, "Could not approve that puzzle.")
	}
}

func (server *Server) handleAdminArchivePuzzle(w http.ResponseWriter, r *http.Request) {
	puzzleID := r.PathValue("id")
	if !validPuzzleID(puzzleID) {
		writeError(w, http.StatusBadRequest, "Puzzle id is required.")
		return
	}

	err := server.adminPuzzles.Archive(r.Context(), puzzleID)
	switch {
	case err == nil:
		if server.moderation != nil {
			_ = server.moderation.AddAction(r.Context(), ModerationActionInput{
				PuzzleID: puzzleID,
				Actor:    server.adminActor(r),
				Action:   "PUZZLE_ARCHIVED",
			})
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case errors.Is(err, ErrPuzzleNotFound):
		writeError(w, http.StatusNotFound, "Puzzle not found.")
	default:
		writeError(w, http.StatusInternalServerError, "Could not archive that puzzle.")
	}
}

func (server *Server) handleAdminReinstatePuzzle(w http.ResponseWriter, r *http.Request) {
	puzzleID := r.PathValue("id")
	if !validPuzzleID(puzzleID) {
		writeError(w, http.StatusBadRequest, "Puzzle id is required.")
		return
	}

	err := server.adminPuzzles.Reinstate(r.Context(), puzzleID)
	switch {
	case err == nil:
		if server.moderation != nil {
			_ = server.moderation.AddAction(r.Context(), ModerationActionInput{
				PuzzleID: puzzleID,
				Actor:    server.adminActor(r),
				Action:   "PUZZLE_REINSTATED",
			})
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case errors.Is(err, ErrPuzzleNotFound):
		writeError(w, http.StatusNotFound, "Puzzle not found.")
	default:
		writeError(w, http.StatusInternalServerError, "Could not reinstate that puzzle.")
	}
}

func (server *Server) handleAdminPreviewPuzzle(w http.ResponseWriter, r *http.Request) {
	puzzleID := r.PathValue("id")
	if !validPuzzleID(puzzleID) {
		writeError(w, http.StatusBadRequest, "Puzzle id is required.")
		return
	}

	puzzle, err := server.puzzles.PuzzleByID(r.Context(), puzzleID)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, adminPuzzlePreviewResponse{
			Puzzle: ToPublicPuzzle(puzzle),
			Groups: AllSolvedGroups(puzzle),
		})
	case errors.Is(err, ErrPuzzleNotFound):
		writeError(w, http.StatusNotFound, "Puzzle not found.")
	default:
		writeError(w, http.StatusInternalServerError, "Could not load that puzzle preview.")
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
