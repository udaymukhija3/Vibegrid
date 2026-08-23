package vibegrid

import (
	"errors"
	"net/http"
	"strings"
	"time"
)

type adminVibeBoardInput struct {
	PublishDate string   `json:"publishDate"`
	Prompt      string   `json:"prompt"`
	Tiles       []string `json:"tiles"`
}

func (server *Server) handleAdminListVibeBoards(w http.ResponseWriter, r *http.Request) {
	if server.adminVibeBoards == nil {
		writeError(w, http.StatusServiceUnavailable, "Daily board authoring requires a database.")
		return
	}
	boards, err := server.adminVibeBoards.ListVibeBoards(r.Context(), 90)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load the daily board pipeline.")
		return
	}
	writeJSON(w, http.StatusOK, boards)
}

func (server *Server) handleAdminCreateVibeBoard(w http.ResponseWriter, r *http.Request) {
	if server.adminVibeBoards == nil {
		writeError(w, http.StatusServiceUnavailable, "Daily board authoring requires a database.")
		return
	}
	var input adminVibeBoardInput
	if !decodeJSONBody(w, r, maxVibeMutationBodyBytes, &input, "That daily board is not valid JSON.") {
		return
	}
	board, err := input.toBoard(server.todayString())
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "Choose today or a future date, write one clear prompt, and provide 28 different fragments.")
		return
	}
	if err := server.blocklist.reviewText(board.Prompt); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "The prompt contains a blocked word or phrase.")
		return
	}
	for _, tile := range board.Tiles {
		if err := server.blocklist.reviewText(tile.Text); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "A fragment contains a blocked word or phrase.")
			return
		}
	}
	created, err := server.adminVibeBoards.CreateVibeBoard(r.Context(), board)
	if errors.Is(err, ErrVibeBoardExists) {
		writeError(w, http.StatusConflict, "That date is already frozen. Pick another date.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not freeze that daily board.")
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (input adminVibeBoardInput) toBoard(today string) (VibeBoard, error) {
	publishDate := strings.TrimSpace(input.PublishDate)
	if _, err := time.Parse("2006-01-02", publishDate); err != nil || publishDate < today || len(input.Tiles) != VibeBoardMaxTileCount {
		return VibeBoard{}, ErrVibeBoardInvalid
	}
	canonical, err := VibeBoardForDate(publishDate)
	if err != nil {
		return VibeBoard{}, err
	}
	canonical.Prompt = strings.Join(strings.Fields(input.Prompt), " ")
	canonical.Tiles = make([]Tile, VibeBoardMaxTileCount)
	for index, raw := range input.Tiles {
		canonical.Tiles[index] = Tile{
			ID:   bankTileID(canonical.ID, index),
			Text: strings.Join(strings.Fields(raw), " "),
		}
	}
	if err := validateVibeBoard(canonical); err != nil {
		return VibeBoard{}, err
	}
	return canonical, nil
}
