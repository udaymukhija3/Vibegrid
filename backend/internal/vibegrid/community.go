package vibegrid

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// CommunityPuzzleStore is the write side for user-created puzzles. Only the
// Postgres store implements it; the feature requires a database.
type CommunityPuzzleStore interface {
	CreateCommunityPuzzle(ctx context.Context, input AdminPuzzleInput) (Puzzle, error)
}

// createdPuzzleResponse is the minimal payload a creator needs to share their
// puzzle: an id for the play link and the assigned number.
type createdPuzzleResponse struct {
	OK           bool   `json:"ok"`
	ID           string `json:"id"`
	PuzzleNumber int    `json:"puzzleNumber"`
}

// rateLimiter is a small in-memory fixed-window limiter keyed by client. It is
// enough to blunt casual abuse of the public create endpoint; a multi-instance
// deployment would move this to Redis.
type rateLimiter struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	limit  int
	window time.Duration
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{hits: map[string][]time.Time{}, limit: limit, window: window}
}

func (limiter *rateLimiter) allow(key string) bool {
	now := time.Now()
	cutoff := now.Add(-limiter.window)

	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	recent := make([]time.Time, 0, len(limiter.hits[key]))
	for _, hit := range limiter.hits[key] {
		if hit.After(cutoff) {
			recent = append(recent, hit)
		}
	}

	if len(recent) >= limiter.limit {
		limiter.hits[key] = recent
		return false
	}

	limiter.hits[key] = append(recent, now)
	return true
}

func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		return strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func (server *Server) handleCommunityCreate(w http.ResponseWriter, r *http.Request) {
	if server.community == nil {
		writeError(w, http.StatusServiceUnavailable, "Community puzzles require a database.")
		return
	}

	if server.createLimiter != nil && !server.createLimiter.allow(clientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "You're creating puzzles too quickly. Try again later.")
		return
	}

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

	puzzle, err := server.community.CreateCommunityPuzzle(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not save that puzzle.")
		return
	}

	writeJSON(w, http.StatusCreated, createdPuzzleResponse{
		OK:           true,
		ID:           puzzle.ID,
		PuzzleNumber: puzzle.PuzzleNumber,
	})
}

// handleGetPuzzle serves any single puzzle by id as a public payload (tiles
// shuffled, group membership hidden). This is the play-by-link entry point for
// community puzzles, and works for editorial puzzles too.
func (server *Server) handleGetPuzzle(w http.ResponseWriter, r *http.Request) {
	puzzleID := r.PathValue("id")
	if puzzleID == "" {
		writeError(w, http.StatusBadRequest, "Puzzle id is required.")
		return
	}

	puzzles, err := server.puzzles.Puzzles(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load puzzles.")
		return
	}

	puzzle, err := FindPuzzleByID(puzzles, puzzleID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Puzzle not found.")
		return
	}

	writeJSON(w, http.StatusOK, ToPublicPuzzle(*puzzle))
}
