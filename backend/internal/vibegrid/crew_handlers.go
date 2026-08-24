package vibegrid

import (
	"errors"
	"net/http"
	"strings"
	"time"
)

// maxCrewBodyBytes caps crew payloads. A crew create is two short names.
const maxCrewBodyBytes = 4 << 10 // 4 KiB

const (
	crewCreateRateLimit  = 10
	crewCreateRateWindow = time.Hour
	crewJoinRateLimit    = 30
	crewJoinRateWindow   = time.Hour
)

// crewShareSquares mirrors the client's share palette (mint / yolk / tomato /
// plum), indexed by a group's colorIndex.
var crewShareSquares = [...]string{"🟩", "🟨", "🟥", "🟪"}

// crewView never carries the crew's internal id — the invite code is the only
// handle a client needs, and it is the one that can be revoked.
type crewView struct {
	InviteCode string `json:"inviteCode"`
	Name       string `json:"name"`
	JoinPath   string `json:"joinPath"`
	IsOwner    bool   `json:"isOwner"`
}

// CrewBoardEntry is one member's row on the crew board. Grid is populated only
// once the viewer has finished the puzzle themselves.
type CrewBoardEntry struct {
	// MemberID is set only for the owner's view: it is the handle for removing
	// someone, and nobody else has any use for it.
	MemberID       string   `json:"memberId,omitempty"`
	DisplayName    string   `json:"displayName"`
	IsYou          bool     `json:"isYou"`
	Playing        bool     `json:"playing"`
	Solved         bool     `json:"solved"`
	Failed         bool     `json:"failed"`
	SolvedCount    int      `json:"solvedCount"`
	Mistakes       int      `json:"mistakes"`
	ElapsedSeconds *int     `json:"elapsedSeconds,omitempty"`
	Grid           []string `json:"grid,omitempty"`
}

type crewBoardResponse struct {
	Crew             crewView         `json:"crew"`
	PuzzleID         string           `json:"puzzleId"`
	PuzzleNumber     int              `json:"puzzleNumber"`
	GroupCount       int              `json:"groupCount"`
	IsMember         bool             `json:"isMember"`
	SpoilersUnlocked bool             `json:"spoilersUnlocked"`
	Members          []CrewBoardEntry `json:"members"`
}

type crewCreateRequest struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
}

type crewJoinRequest struct {
	DisplayName string `json:"displayName"`
}

// crewsEnabled reports whether crews can be served at all. Crews are inherently
// multi-session and durable, so in no-database mode the endpoints say so rather
// than pretending to work for a single process.
func (server *Server) crewsEnabled(w http.ResponseWriter) bool {
	if server.crews == nil {
		writeError(w, http.StatusServiceUnavailable, "Crews require a database.")
		return false
	}
	return true
}

// allowCrewWrite throttles crew mutations per client. Crew creation is the
// expensive one (it makes rows), so it gets the tighter budget.
//
// The budget is charged per session first, then per network. Keying on address
// alone put a whole crew into one bucket whenever they played from one uplink —
// the exact situation this product is designed for, a group of friends in one
// room — and turned an undeclared platform proxy into a global cap.
func (server *Server) allowCrewWrite(w http.ResponseWriter, r *http.Request, keyPrefix string, limit int, window time.Duration, message string) bool {
	for _, scope := range server.rateLimitScopes(r, keyPrefix, limit) {
		decision, err := server.checkRateLimit(r.Context(), scope.key, scope.limit, window, server.createLimiter)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "Could not check request limits.")
			return false
		}
		if !decision.allowed {
			writeRateLimit(w, message, decision.retryAfter)
			return false
		}
	}
	return true
}

func (server *Server) handleCreateCrew(w http.ResponseWriter, r *http.Request) {
	if !server.crewsEnabled(w) {
		return
	}
	sessionID := EnsureSessionID(w, r, server.secureCookies)
	if !server.allowCrewWrite(w, r, "crew-create:", crewCreateRateLimit, crewCreateRateWindow,
		"You're making crews too quickly. Try again later.") {
		return
	}

	var request crewCreateRequest
	if !decodeJSONBody(w, r, maxCrewBodyBytes, &request, "That crew payload is not valid JSON.") {
		return
	}
	if err := server.blocklist.reviewText(request.Name, request.DisplayName); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "That name contains a blocked word or phrase.")
		return
	}

	crew, err := server.crews.CreateCrew(r.Context(), request.Name, request.DisplayName, sessionID, server.clock())
	if err != nil {
		writeCrewError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toCrewView(crew, sessionID))
}

func (server *Server) handleJoinCrew(w http.ResponseWriter, r *http.Request) {
	if !server.crewsEnabled(w) {
		return
	}
	crewID := r.PathValue("id")
	if !validCrewID(crewID) {
		writeError(w, http.StatusNotFound, "Crew not found.")
		return
	}
	sessionID := EnsureSessionID(w, r, server.secureCookies)
	if !server.allowCrewWrite(w, r, "crew-join:", crewJoinRateLimit, crewJoinRateWindow,
		"You're joining crews too quickly. Try again later.") {
		return
	}

	var request crewJoinRequest
	if !decodeJSONBody(w, r, maxCrewBodyBytes, &request, "That crew payload is not valid JSON.") {
		return
	}
	if err := server.blocklist.reviewText(request.DisplayName); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "That name contains a blocked word or phrase.")
		return
	}

	crew, err := server.crews.JoinCrew(r.Context(), crewID, request.DisplayName, sessionID, server.clock())
	if err != nil {
		writeCrewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toCrewView(crew, sessionID))
}

// handleRotateCrewInvite revokes every invite link already shared for this crew.
// Members keep their access — they reach the crew from their own crew list.
func (server *Server) handleRotateCrewInvite(w http.ResponseWriter, r *http.Request) {
	crew, sessionID, ok := server.crewFromRequest(w, r)
	if !ok {
		return
	}
	rotated, err := server.crews.RotateInvite(r.Context(), crew.ID, sessionID)
	if err != nil {
		writeCrewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toCrewView(rotated, sessionID))
}

func (server *Server) handleRemoveCrewMember(w http.ResponseWriter, r *http.Request) {
	crew, sessionID, ok := server.crewFromRequest(w, r)
	if !ok {
		return
	}
	if err := server.crews.RemoveMember(r.Context(), crew.ID, r.PathValue("memberId"), sessionID); err != nil {
		writeCrewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (server *Server) handleLeaveCrew(w http.ResponseWriter, r *http.Request) {
	crew, sessionID, ok := server.crewFromRequest(w, r)
	if !ok {
		return
	}
	if err := server.crews.LeaveCrew(r.Context(), crew.ID, sessionID); err != nil {
		writeCrewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// crewFromRequest resolves the invite code in the path to a crew and settles the
// shared preconditions for crew mutations. It does not check membership or
// ownership — the store enforces those inside the statement that mutates, so
// there is no gap between checking and acting.
func (server *Server) crewFromRequest(w http.ResponseWriter, r *http.Request) (Crew, string, bool) {
	if !server.crewsEnabled(w) {
		return Crew{}, "", false
	}
	inviteCode := r.PathValue("id")
	if !validCrewID(inviteCode) {
		writeError(w, http.StatusNotFound, "Crew not found.")
		return Crew{}, "", false
	}
	sessionID := EnsureSessionID(w, r, server.secureCookies)
	if !server.allowCrewWrite(w, r, "crew-manage:", crewJoinRateLimit, crewJoinRateWindow,
		"You're changing crews too quickly. Try again later.") {
		return Crew{}, "", false
	}
	crew, err := server.crews.CrewByInviteCode(r.Context(), inviteCode)
	if err != nil {
		writeCrewError(w, err)
		return Crew{}, "", false
	}
	return crew, sessionID, true
}

// handleMyCrews lists the crews this browser belongs to, so the app can offer a
// way back in without the player having kept the invite link.
func (server *Server) handleMyCrews(w http.ResponseWriter, r *http.Request) {
	if !server.crewsEnabled(w) {
		return
	}
	if !server.allowPuzzleRead(w, r) {
		return
	}
	sessionID := EnsureSessionID(w, r, server.secureCookies)

	crews, err := server.crews.CrewsForSession(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load your crews.")
		return
	}
	views := make([]crewView, 0, len(crews))
	for _, crew := range crews {
		views = append(views, toCrewView(crew, sessionID))
	}
	writeJSON(w, http.StatusOK, views)
}

// handleCrewBoard serves one crew's standing on today's daily grid.
//
// Spoiler rule: a member who is still playing sees only how far everyone else
// has got — never which tiles they guessed. The guess grids are not merely
// hidden in the response, they are not loaded at all until the viewer's own
// attempt is finished, so there is no payload to inspect in devtools.
func (server *Server) handleCrewBoard(w http.ResponseWriter, r *http.Request) {
	if !server.crewsEnabled(w) {
		return
	}
	inviteCode := r.PathValue("id")
	if !validCrewID(inviteCode) {
		writeError(w, http.StatusNotFound, "Crew not found.")
		return
	}
	if !server.allowPuzzleRead(w, r) {
		return
	}
	sessionID := EnsureSessionID(w, r, server.secureCookies)

	crew, err := server.crews.CrewByInviteCode(r.Context(), inviteCode)
	if err != nil {
		writeCrewError(w, err)
		return
	}
	puzzle, err := server.puzzles.TodaysPuzzle(r.Context(), server.todayString())
	if err != nil {
		writeError(w, http.StatusNotFound, "No puzzle today.")
		return
	}

	board, err := server.buildCrewBoard(r, crew, puzzle, sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load the crew board.")
		return
	}

	// Per-session content: never cache it at the edge.
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, board)
}

func (server *Server) buildCrewBoard(r *http.Request, crew Crew, puzzle Puzzle, sessionID string) (crewBoardResponse, error) {
	progress, err := server.crews.CrewProgress(r.Context(), crew.ID, puzzle.ID)
	if err != nil {
		return crewBoardResponse{}, err
	}

	isMember := false
	spoilersUnlocked := false
	for _, member := range progress {
		if member.SessionID == sessionID {
			isMember = true
			spoilersUnlocked = member.Completed || member.Failed
			break
		}
	}

	// Only reach for the grids once the viewer has earned them.
	var history map[string][][]string
	if spoilersUnlocked {
		history, err = server.crews.CrewGuessHistory(r.Context(), crew.ID, puzzle.ID)
		if err != nil {
			return crewBoardResponse{}, err
		}
	}

	isOwner := crew.OwnerID != "" && crew.OwnerID == sessionID
	colorByTile := tileColorIndex(puzzle)
	members := make([]CrewBoardEntry, 0, len(progress))
	for _, member := range progress {
		entry := CrewBoardEntry{
			DisplayName: member.DisplayName,
			IsYou:       member.SessionID == sessionID,
			Solved:      member.Completed,
			Failed:      member.Failed,
			Playing:     member.Started && !member.Completed && !member.Failed,
			SolvedCount: member.SolvedCount,
			Mistakes:    member.Mistakes,
		}
		if member.Completed && member.StartedAt != nil && member.CompletedAt != nil {
			seconds := int(member.CompletedAt.Sub(*member.StartedAt).Seconds())
			if seconds < 0 {
				seconds = 0
			}
			entry.ElapsedSeconds = &seconds
		}
		if rows, ok := history[member.SessionID]; ok {
			entry.Grid = buildCrewShareGrid(rows, colorByTile)
		}
		// Only the owner can act on a member, so only the owner is handed the
		// handle for doing it.
		if isOwner && member.SessionID != sessionID {
			entry.MemberID = member.MemberID
		}
		members = append(members, entry)
	}
	sortCrewBoard(members)

	return crewBoardResponse{
		Crew:             toCrewView(crew, sessionID),
		PuzzleID:         puzzle.ID,
		PuzzleNumber:     puzzle.PuzzleNumber,
		GroupCount:       len(puzzle.Groups),
		IsMember:         isMember,
		SpoilersUnlocked: spoilersUnlocked,
		Members:          members,
	}, nil
}

// tileColorIndex maps every tile to the colour of the group it belongs to, the
// same mapping the client uses to paint its own share grid.
func tileColorIndex(puzzle Puzzle) map[string]int {
	colors := make(map[string]int, len(puzzle.Groups)*GroupSize)
	for _, group := range puzzle.Groups {
		for _, tile := range group.Tiles {
			colors[tile.ID] = group.ColorIndex
		}
	}
	return colors
}

// buildCrewShareGrid renders a member's guesses as spoiler-safe coloured rows.
// It names no group and reveals no tile text — it is the same artefact the
// player would paste into a group chat.
func buildCrewShareGrid(guesses [][]string, colorByTile map[string]int) []string {
	grid := make([]string, 0, len(guesses))
	for _, guess := range guesses {
		if len(guess) == 0 {
			continue
		}
		var row strings.Builder
		for _, tileID := range guess {
			index := colorByTile[tileID] % len(crewShareSquares)
			if index < 0 {
				index = 0
			}
			row.WriteString(crewShareSquares[index])
		}
		grid = append(grid, row.String())
	}
	return grid
}

func toCrewView(crew Crew, sessionID string) crewView {
	return crewView{
		InviteCode: crew.InviteCode,
		Name:       crew.Name,
		JoinPath:   "/crew/" + crew.InviteCode,
		IsOwner:    crew.OwnerID != "" && crew.OwnerID == sessionID,
	}
}

func writeCrewError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrCrewNotFound):
		writeError(w, http.StatusNotFound, "Crew not found.")
	case errors.Is(err, ErrCrewMemberUnknown):
		writeError(w, http.StatusNotFound, "That member is not in this crew.")
	case errors.Is(err, ErrNotCrewOwner):
		writeError(w, http.StatusForbidden, "Only the crew owner can do that.")
	case errors.Is(err, ErrNotCrewMember):
		writeError(w, http.StatusForbidden, "You are not in this crew.")
	case errors.Is(err, ErrCrewFull):
		writeError(w, http.StatusConflict, "This crew is full.")
	case errors.Is(err, ErrDisplayNameTaken):
		writeError(w, http.StatusConflict, "Someone in this crew already uses that name.")
	case errors.Is(err, ErrCrewNameInvalid):
		writeError(w, http.StatusUnprocessableEntity, "Pick a shorter name with no line breaks.")
	default:
		writeError(w, http.StatusInternalServerError, "Could not complete that crew action.")
	}
}
