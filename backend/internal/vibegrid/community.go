package vibegrid

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
)

// CommunityPuzzleStore is the write side for user-created puzzles. Only the
// Postgres store implements it; the feature requires a database.
type CommunityPuzzleStore interface {
	CreateCommunityPuzzle(ctx context.Context, input AdminPuzzleInput, claimHash string) (Puzzle, error)
	CreatorStatus(ctx context.Context, puzzleID, claimHash string) (CreatorPuzzleStatus, error)
	WithdrawCommunityPuzzle(ctx context.Context, puzzleID, claimHash string) (CreatorPuzzleStatus, error)
}

var ErrCreatorClaimInvalid = errors.New("creator claim not found")
var ErrCreatorWithdrawalUnavailable = errors.New("community puzzle can no longer be withdrawn")

const creatorClaimHeader = "X-VibeGrid-Creator-Claim"

type CreatorPuzzleStatus struct {
	ID           string
	PuzzleNumber int
	Status       PuzzleStatus
	UpdatedAt    time.Time
	Withdrawn    bool
}

type creatorPuzzleStatusResponse struct {
	ID           string       `json:"id"`
	PuzzleNumber int          `json:"puzzleNumber"`
	Status       PuzzleStatus `json:"status"`
	UpdatedAt    string       `json:"updatedAt"`
	Withdrawn    bool         `json:"withdrawn"`
	CanWithdraw  bool         `json:"canWithdraw"`
	CanAppeal    bool         `json:"canAppeal"`
	PlayPath     string       `json:"playPath,omitempty"`
}

// createdPuzzleResponse confirms receipt of a community submission. The grid is
// immediately playable at PlayPath so the creator can send it to friends;
// editor review only decides whether it also enters public listings.
type createdPuzzleResponse struct {
	OK           bool         `json:"ok"`
	ID           string       `json:"id"`
	PuzzleNumber int          `json:"puzzleNumber"`
	Status       PuzzleStatus `json:"status"`
	ClaimSecret  string       `json:"claimSecret"`
	ClaimPath    string       `json:"claimPath"`
	PlayPath     string       `json:"playPath"`
}

const maxCommunityBodyBytes = 16 << 10 // 16 KiB

// rateLimiter is a bounded in-memory sliding-window limiter keyed by client.
// Public reads always use it so cached content never causes a database write;
// mutation endpoints use it when no shared Postgres limiter is configured.
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
		// The fallback limiter carries its own ceiling on purpose: it is the seam
		// tests use to inject a tight limiter and prove throttling still happens
		// when the shared store is gone. Crew writes never reach here anyway —
		// crewsEnabled refuses them outright without a database.
		return fallback.check(key, server.clock()), nil
	}
	return rateLimitDecision{allowed: true}, nil
}

func (limiter *rateLimiter) check(key string, now time.Time) rateLimitDecision {
	return limiter.checkLimit(key, limiter.limit, now)
}

// checkLimit meters one key against a caller-supplied ceiling. Callers that
// charge the same limiter for both a session and its network need two different
// ceilings over one set of counters, so the limit cannot live only on the
// limiter (see Server.rateLimitScopes).
func (limiter *rateLimiter) checkLimit(key string, limit int, now time.Time) rateLimitDecision {
	if limit < 1 {
		limit = limiter.limit
	}
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

	if len(recent) >= limit {
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

func (server *Server) handleCommunityCreate(w http.ResponseWriter, r *http.Request) {
	if server.community == nil {
		writeError(w, http.StatusServiceUnavailable, "Community puzzles require a database.")
		return
	}

	if server.createLimiter != nil || server.rateLimits != nil {
		decision, err := server.checkRateLimit(r.Context(), "create:"+server.clientIP(r), 20, time.Hour, server.createLimiter)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "Could not check request limits.")
			return
		}
		if !decision.allowed {
			writeRateLimit(w, "You're creating puzzles too quickly. Try again later.", decision.retryAfter)
			return
		}
	}
	server.withIdempotency("community-puzzle.create", server.guestIdempotencyCaller, server.handleCommunityCreateMutation)(w, r)
}

func (server *Server) handleCommunityCreateMutation(w http.ResponseWriter, r *http.Request) {
	if !server.verifyBot(w, r, "community_create") {
		return
	}
	var input AdminPuzzleInput
	if !decodeJSONBody(w, r, maxCommunityBodyBytes, &input, "That puzzle payload is not valid JSON.") {
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

	claimSecret := newCreatorClaimSecret()
	puzzle, err := server.community.CreateCommunityPuzzle(r.Context(), input, hashCreatorClaimSecret(claimSecret))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not save that puzzle.")
		return
	}

	writeJSON(w, http.StatusAccepted, createdPuzzleResponse{
		OK:           true,
		ID:           puzzle.ID,
		PuzzleNumber: puzzle.PuzzleNumber,
		Status:       puzzle.Status,
		ClaimSecret:  claimSecret,
		ClaimPath:    "/claim?id=" + puzzle.ID,
		PlayPath:     "/p/" + puzzle.ID,
	})
}

func (server *Server) handleCreatorStatus(w http.ResponseWriter, r *http.Request) {
	if server.community == nil {
		writeError(w, http.StatusServiceUnavailable, "Creator claims require a database.")
		return
	}
	if !server.allowPuzzleRead(w, r) {
		return
	}

	status, err := server.loadCreatorStatus(r)
	if err != nil {
		writeCreatorClaimError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toCreatorStatusResponse(status))
}

func (server *Server) handleCreatorWithdraw(w http.ResponseWriter, r *http.Request) {
	if server.community == nil {
		writeError(w, http.StatusServiceUnavailable, "Creator claims require a database.")
		return
	}
	if !server.allowModerationWrite(w, r, "creator-withdraw:", "You're changing creator submissions too quickly. Try again later.") {
		return
	}
	server.withIdempotency("community-puzzle.withdraw", server.creatorIdempotencyCaller, server.handleCreatorWithdrawMutation)(w, r)
}

func (server *Server) handleCreatorWithdrawMutation(w http.ResponseWriter, r *http.Request) {
	puzzleID, claimHash, err := creatorClaimRequest(r)
	if err != nil {
		writeCreatorClaimError(w, err)
		return
	}
	status, err := server.community.WithdrawCommunityPuzzle(r.Context(), puzzleID, claimHash)
	if err != nil {
		writeCreatorClaimError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toCreatorStatusResponse(status))
}

func (server *Server) loadCreatorStatus(r *http.Request) (CreatorPuzzleStatus, error) {
	puzzleID, claimHash, err := creatorClaimRequest(r)
	if err != nil {
		return CreatorPuzzleStatus{}, err
	}
	return server.community.CreatorStatus(r.Context(), puzzleID, claimHash)
}

func creatorClaimRequest(r *http.Request) (string, string, error) {
	puzzleID := strings.TrimSpace(r.PathValue("id"))
	claimHash, err := creatorClaimHashFromRequest(r)
	if !validPuzzleID(puzzleID) || err != nil {
		return "", "", ErrCreatorClaimInvalid
	}
	return puzzleID, claimHash, nil
}

func creatorClaimHashFromRequest(r *http.Request) (string, error) {
	secret := strings.TrimSpace(r.Header.Get(creatorClaimHeader))
	if !validCreatorClaimSecret(secret) {
		return "", ErrCreatorClaimInvalid
	}
	return hashCreatorClaimSecret(secret), nil
}

func writeCreatorClaimError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrCreatorClaimInvalid):
		writeError(w, http.StatusNotFound, "Creator claim not found.")
	case errors.Is(err, ErrCreatorWithdrawalUnavailable):
		writeError(w, http.StatusConflict, "Only a pending submission can be withdrawn.")
	default:
		writeError(w, http.StatusInternalServerError, "Could not load that creator claim.")
	}
}

func toCreatorStatusResponse(status CreatorPuzzleStatus) creatorPuzzleStatusResponse {
	response := creatorPuzzleStatusResponse{
		ID:           status.ID,
		PuzzleNumber: status.PuzzleNumber,
		Status:       status.Status,
		UpdatedAt:    status.UpdatedAt.UTC().Format(time.RFC3339),
		Withdrawn:    status.Withdrawn,
		CanWithdraw:  status.Status == PuzzleStatusPending && !status.Withdrawn,
		CanAppeal:    status.Status == PuzzleStatusArchived && !status.Withdrawn,
	}
	// PENDING is playable-by-link too (see PubliclyPlayable): review only gates
	// public listing, so a creator gets a link to send the moment they submit.
	if status.Status == PuzzleStatusPublished || (status.Status == PuzzleStatusPending && !status.Withdrawn) {
		response.PlayPath = "/p/" + status.ID
	}
	return response
}

func newCreatorClaimSecret() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		panic("crypto/rand failed while generating creator claim: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(bytes)
}

func hashCreatorClaimSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func validCreatorClaimSecret(secret string) bool {
	if len(secret) < 32 || len(secret) > 128 {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(secret)
	return err == nil
}

// handleGetPuzzle serves any single puzzle by id as a public payload (tiles
// shuffled, group membership hidden). This is the play-by-link entry point for
// community puzzles, and works for editorial puzzles too.
func (server *Server) handleGetPuzzle(w http.ResponseWriter, r *http.Request) {
	if !server.allowPuzzleRead(w, r) {
		return
	}
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
