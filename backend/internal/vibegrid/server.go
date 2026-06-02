package vibegrid

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

type ServerConfig struct {
	Puzzles  []Puzzle
	Store    *MemoryAttemptStore
	Clock    func() time.Time
	TimeZone string
}

type Server struct {
	puzzles  []Puzzle
	store    *MemoryAttemptStore
	clock    func() time.Time
	timeZone string
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
		puzzles:  config.Puzzles,
		store:    config.Store,
		clock:    clock,
		timeZone: timeZone,
	}
	if server.store == nil {
		server.store = NewMemoryAttemptStore()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.handleHealth)
	mux.HandleFunc("GET /api/puzzles/today", server.handleTodayPuzzle)
	mux.HandleFunc("GET /api/puzzles", server.handlePuzzles)
	mux.HandleFunc("GET /api/attempts/", server.handleAttempt)
	mux.HandleFunc("POST /api/guesses", server.handleGuess)

	return withCORS(mux)
}

func (server *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (server *Server) handleTodayPuzzle(w http.ResponseWriter, _ *http.Request) {
	puzzle, err := TodaysPuzzle(server.puzzles, server.clock(), server.timeZone)
	if err != nil {
		writeError(w, http.StatusNotFound, "Puzzle not found.")
		return
	}

	writeJSON(w, http.StatusOK, ToPublicPuzzle(*puzzle))
}

func (server *Server) handlePuzzles(w http.ResponseWriter, _ *http.Request) {
	published := PublishedPuzzles(server.puzzles)
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

	puzzle, err := FindPuzzleByID(server.puzzles, puzzleID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Puzzle not found.")
		return
	}

	sessionID := EnsureSessionID(w, r)
	attempt := server.store.GetOrCreate(*puzzle, sessionID, server.clock())
	writeJSON(w, http.StatusOK, attempt)
}

func (server *Server) handleGuess(w http.ResponseWriter, r *http.Request) {
	var request GuessRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "That guess payload is not valid.")
		return
	}

	if request.PuzzleID == "" || request.ClientGuessID == "" {
		writeError(w, http.StatusBadRequest, "Puzzle id and client guess id are required.")
		return
	}

	puzzle, err := FindPuzzleByID(server.puzzles, request.PuzzleID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Puzzle not found.")
		return
	}

	sessionID := EnsureSessionID(w, r)
	submission, err := server.store.SubmitGuess(*puzzle, sessionID, request, server.clock())
	if err != nil {
		status := http.StatusUnprocessableEntity
		if errors.Is(err, ErrAttemptFinished) {
			status = http.StatusConflict
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

func withCORS(next http.Handler) http.Handler {
	allowedOrigins := map[string]bool{
		"http://localhost:3000": true,
		"http://localhost:3001": true,
		"http://localhost:3002": true,
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
