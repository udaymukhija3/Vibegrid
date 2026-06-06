package vibegrid

import (
	"context"
	"errors"
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
	mu        sync.Mutex
	hits      map[string][]time.Time
	limit     int
	window    time.Duration
	maxKeys   int
	lastPrune time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{hits: map[string][]time.Time{}, limit: limit, window: window, maxKeys: 10000}
}

type rateLimitDecision struct {
	allowed    bool
	retryAfter time.Duration
}

func (server *Server) checkRateLimit(ctx context.Context, key string, limit int, window time.Duration, fallback *rateLimiter) (rateLimitDecision, error) {
	if server.rateLimits != nil {
		return server.rateLimits.Check(ctx, key, limit, window, server.clock())
	}
	if fallback != nil {
		return fallback.check(key), nil
	}
	return rateLimitDecision{allowed: true}, nil
}

func (limiter *rateLimiter) check(key string) rateLimitDecision {
	now := time.Now()
	cutoff := now.Add(-limiter.window)

	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	if limiter.lastPrune.IsZero() || now.Sub(limiter.lastPrune) > limiter.window/4 {
		limiter.pruneLocked(cutoff, now)
	}

	if _, exists := limiter.hits[key]; !exists && limiter.maxKeys > 0 && len(limiter.hits) >= limiter.maxKeys {
		return rateLimitDecision{retryAfter: limiter.window}
	}

	recent := make([]time.Time, 0, len(limiter.hits[key]))
	for _, hit := range limiter.hits[key] {
		if hit.After(cutoff) {
			recent = append(recent, hit)
		}
	}

	if len(recent) >= limiter.limit {
		limiter.hits[key] = recent
		return rateLimitDecision{retryAfter: recent[0].Add(limiter.window).Sub(now)}
	}

	limiter.hits[key] = append(recent, now)
	return rateLimitDecision{allowed: true}
}

func (limiter *rateLimiter) pruneLocked(cutoff, now time.Time) {
	for key, hits := range limiter.hits {
		recent := hits[:0]
		for _, hit := range hits {
			if hit.After(cutoff) {
				recent = append(recent, hit)
			}
		}
		if len(recent) == 0 {
			delete(limiter.hits, key)
		} else {
			limiter.hits[key] = recent
		}
	}
	limiter.lastPrune = now
}

func clientIP(r *http.Request) string {
	for _, header := range []string{"Fly-Client-IP", "X-Real-IP"} {
		value := strings.TrimSpace(r.Header.Get(header))
		if ip := net.ParseIP(value); ip != nil {
			return ip.String()
		}
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

	if server.createLimiter != nil || server.rateLimits != nil {
		decision, err := server.checkRateLimit(r.Context(), "create:"+clientIP(r), 20, time.Hour, server.createLimiter)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Could not check request limits.")
			return
		}
		if !decision.allowed {
			writeRateLimit(w, "You're creating puzzles too quickly. Try again later.", decision.retryAfter)
			return
		}
	}

	var input AdminPuzzleInput
	if !decodeJSONBody(w, r, maxAdminBodyBytes, &input, "That puzzle payload is not valid JSON.") {
		return
	}

	if err := input.Validate(); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := server.blocklist.review(input); err != nil {
		if errors.Is(err, ErrBlockedTerm) {
			writeError(w, http.StatusUnprocessableEntity, "This puzzle contains a blocked word or phrase.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Could not review that puzzle.")
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

	puzzle, err := server.publicPuzzleByID(r.Context(), puzzleID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Puzzle not found.")
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=60, s-maxage=300")
	writeJSON(w, http.StatusOK, ToPublicPuzzle(puzzle))
}
