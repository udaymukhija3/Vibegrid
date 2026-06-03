package vibegrid

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

// maxGuessBodyBytes caps the guess payload. A legal guess is four short tile
// ids, so anything large is malformed or hostile.
const maxGuessBodyBytes = 16 << 10 // 16 KiB

type ServerConfig struct {
	Puzzles        PuzzleSource
	Store          Store
	AdminPuzzles   AdminPuzzleStore
	Community      CommunityPuzzleStore
	Stats          StatsStore
	AdminToken     string
	Clock          func() time.Time
	TimeZone       string
	AllowedOrigins []string
	SecureCookies  bool
}

type Server struct {
	puzzles       PuzzleSource
	store         Store
	adminPuzzles  AdminPuzzleStore
	community     CommunityPuzzleStore
	stats         StatsStore
	createLimiter *rateLimiter
	adminToken    string
	clock         func() time.Time
	timeZone      string
	secureCookies bool
}

func NewServer(config ServerConfig) http.Handler {
	clock := config.Clock
	if clock == nil {
		clock = time.Now
	}

	timeZone := config.TimeZone
	if timeZone == "" {
		timeZone = "Asia/Kolkata"
	}

	server := &Server{
		puzzles:       config.Puzzles,
		store:         config.Store,
		adminPuzzles:  config.AdminPuzzles,
		community:     config.Community,
		stats:         config.Stats,
		createLimiter: newRateLimiter(20, time.Hour),
		adminToken:    config.AdminToken,
		clock:         clock,
		timeZone:      timeZone,
		secureCookies: config.SecureCookies,
	}
	if server.store == nil {
		server.store = NewMemoryAttemptStore()
	}
	if server.puzzles == nil {
		server.puzzles = StaticPuzzleSource(nil)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.handleHealth)
	mux.HandleFunc("GET /api/puzzles/today", server.handleTodayPuzzle)
	mux.HandleFunc("GET /api/puzzles", server.handlePuzzles)
	mux.HandleFunc("GET /api/puzzles/{id}", server.handleGetPuzzle)
	mux.HandleFunc("GET /api/puzzles/{id}/stats", server.handleStats)
	mux.HandleFunc("GET /api/attempts/", server.handleAttempt)
	mux.HandleFunc("POST /api/guesses", server.handleGuess)
	mux.HandleFunc("POST /api/community/puzzles", server.handleCommunityCreate)

	mux.HandleFunc("GET /api/admin/puzzles", server.requireAdmin(server.handleAdminListPuzzles))
	mux.HandleFunc("POST /api/admin/puzzles", server.requireAdmin(server.handleAdminCreatePuzzle))
	mux.HandleFunc("POST /api/admin/puzzles/{id}/publish", server.requireAdmin(server.handleAdminPublishPuzzle))
	mux.HandleFunc("GET /api/admin/puzzles/{id}/analytics", server.requireAdmin(server.handleAdminAnalytics))

	return withCORS(mux, config.AllowedOrigins)
}

func (server *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (server *Server) handleTodayPuzzle(w http.ResponseWriter, r *http.Request) {
	puzzles, err := server.puzzles.Puzzles(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load puzzles.")
		return
	}

	puzzle, err := TodaysPuzzle(puzzles, server.clock(), server.timeZone)
	if err != nil {
		writeError(w, http.StatusNotFound, "Puzzle not found.")
		return
	}

	writeJSON(w, http.StatusOK, ToPublicPuzzle(*puzzle))
}

func (server *Server) handlePuzzles(w http.ResponseWriter, r *http.Request) {
	puzzles, err := server.puzzles.Puzzles(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load puzzles.")
		return
	}

	published := PublishedPuzzles(puzzles)
	publicPuzzles := make([]PublicPuzzle, 0, len(published))
	for _, puzzle := range published {
		publicPuzzles = append(publicPuzzles, ToPublicPuzzle(puzzle))
	}

	writeJSON(w, http.StatusOK, publicPuzzles)
}

func (server *Server) handleAttempt(w http.ResponseWriter, r *http.Request) {
	puzzleID := strings.TrimPrefix(r.URL.Path, "/api/attempts/")
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

	sessionID := EnsureSessionID(w, r, server.secureCookies)
	attempt, err := server.store.GetOrCreate(r.Context(), *puzzle, sessionID, server.clock())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load that attempt.")
		return
	}
	writeJSON(w, http.StatusOK, attempt)
}

func (server *Server) handleGuess(w http.ResponseWriter, r *http.Request) {
	var request GuessRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxGuessBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "That guess payload is not valid.")
		return
	}

	if request.PuzzleID == "" || request.ClientGuessID == "" {
		writeError(w, http.StatusBadRequest, "Puzzle id and client guess id are required.")
		return
	}

	puzzles, err := server.puzzles.Puzzles(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load puzzles.")
		return
	}

	puzzle, err := FindPuzzleByID(puzzles, request.PuzzleID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Puzzle not found.")
		return
	}

	sessionID := EnsureSessionID(w, r, server.secureCookies)
	submission, err := server.store.SubmitGuess(r.Context(), *puzzle, sessionID, request, server.clock())
	if err != nil {
		status := http.StatusUnprocessableEntity
		switch {
		case errors.Is(err, ErrAttemptFinished):
			status = http.StatusConflict
		case isGuessValidationError(err):
			status = http.StatusUnprocessableEntity
		default:
			// Unexpected (storage/transaction) failures are 500s, not client errors.
			writeError(w, http.StatusInternalServerError, "Could not record that guess.")
			return
		}

		writeError(w, status, humanError(err))
		return
	}

	response := GuessResponse{
		OK:        true,
		IsCorrect: submission.IsCorrect,
		Group:     submission.Group,
		Attempt:   &submission.Attempt,
		SessionID: sessionID,
	}
	if submission.Attempt.Failed {
		response.RevealedGroups = submission.Attempt.RevealedGroups
	}

	writeJSON(w, http.StatusOK, response)
}

// isGuessValidationError reports whether err is a client-fixable problem with
// the guess (wrong size, unknown tile, already-solved group) as opposed to a
// storage failure.
func isGuessValidationError(err error) bool {
	return errors.Is(err, ErrInvalidGroupSize) ||
		errors.Is(err, ErrUnknownTile) ||
		errors.Is(err, ErrAlreadySolved)
}

func humanError(err error) string {
	switch {
	case errors.Is(err, ErrInvalidGroupSize):
		return "Pick exactly four unique tiles."
	case errors.Is(err, ErrUnknownTile):
		return "This guess contains a tile that is not in the puzzle."
	case errors.Is(err, ErrAlreadySolved):
		return "That vibe is already locked."
	case errors.Is(err, ErrAttemptFinished):
		return "This attempt is already finished."
	default:
		return "Something went wrong."
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, GuessResponse{
		OK:    false,
		Error: message,
	})
}

func withCORS(next http.Handler, origins []string) http.Handler {
	if len(origins) == 0 {
		origins = []string{
			"http://localhost:3000",
			"http://localhost:3001",
			"http://localhost:3002",
		}
	}

	allowedOrigins := make(map[string]bool, len(origins))
	for _, origin := range origins {
		allowedOrigins[origin] = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowedOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
