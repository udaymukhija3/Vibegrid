package vibegrid

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"time"
)

const maxVibeMutationBodyBytes = 8 << 10 // 8 KiB

type vibeCardView struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Tiles      []Tile `json:"tiles"`
	IsYours    bool   `json:"isYours"`
	AuthorName string `json:"authorName,omitempty"`
	Votes      int    `json:"votes,omitempty"`
	Winner     bool   `json:"winner,omitempty"`
}

type vibeMakeView struct {
	Board          VibeBoard     `json:"board"`
	Submission     *vibeCardView `json:"submission,omitempty"`
	SubmittedCount int           `json:"submittedCount"`
	MemberCount    int           `json:"memberCount"`
}

type vibeJudgeView struct {
	Board          VibeBoard      `json:"board"`
	Eligible       bool           `json:"eligible"`
	HasVoted       bool           `json:"hasVoted"`
	YourVoteID     string         `json:"yourVoteId,omitempty"`
	Cards          []vibeCardView `json:"cards,omitempty"`
	SubmittedCount int            `json:"submittedCount"`
}

type vibeResultView struct {
	Board           VibeBoard      `json:"board"`
	Official        bool           `json:"official"`
	Tied            bool           `json:"tied"`
	SubmissionCount int            `json:"submissionCount"`
	VoteCount       int            `json:"voteCount"`
	Cards           []vibeCardView `json:"cards"`
}

type vibeMemberView struct {
	MemberID       string `json:"memberId,omitempty"`
	DisplayName    string `json:"displayName"`
	IsYou          bool   `json:"isYou"`
	SubmittedToday bool   `json:"submittedToday"`
}

type vibeCrewDailyResponse struct {
	Crew       crewView         `json:"crew"`
	IsMember   bool             `json:"isMember"`
	CrewStreak int              `json:"crewStreak"`
	Today      vibeMakeView     `json:"today"`
	Judge      *vibeJudgeView   `json:"judge,omitempty"`
	Result     *vibeResultView  `json:"result,omitempty"`
	Members    []vibeMemberView `json:"members,omitempty"`
}

func (server *Server) handleTodayVibeBoard(w http.ResponseWriter, r *http.Request) {
	if !server.allowPuzzleRead(w, r) {
		return
	}
	board, err := server.vibeBoardForDate(r.Context(), server.todayString())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load today's fragments.")
		return
	}
	// Practice is intentionally 4x4. A legacy persisted 12-fragment board is
	// extended in-memory from the canonical bank; durable crew history remains
	// untouched and continues to use its original frozen palette.
	board = practiceVibeBoard(board)
	templateIndex, err := VibeTemplateIndexForDate(board.PublishDate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load today's fragments.")
		return
	}
	w.Header().Set("Cache-Control", server.dailyCacheControl())
	writeJSON(w, http.StatusOK, VibePracticeBoard{VibeBoard: board, HouseCards: vibeHouseCardsFor(templateIndex)})
}

func (server *Server) handleUnlimitedVibeBoard(w http.ResponseWriter, r *http.Request) {
	if !server.allowPuzzleRead(w, r) {
		return
	}
	sequence, err := strconv.ParseUint(r.PathValue("sequence"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "Practice board not found.")
		return
	}
	board, err := UnlimitedVibeBoard(sequence)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not deal another board.")
		return
	}
	// A sequence always maps to the same local-practice deal. It contains no
	// identity or crew state and is safe for shared immutable caching.
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	writeJSON(w, http.StatusOK, VibePracticeBoard{
		VibeBoard:  board,
		HouseCards: vibeHouseCardsFor(vibeTemplateIndex(int(sequence % uint64(len(vibeBoardTemplates))))),
	})
}

func (server *Server) handleCrewDaily(w http.ResponseWriter, r *http.Request) {
	if !server.vibeRoundsEnabled(w) {
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

	todayDate, judgeDate, resultDate := server.vibeRoundDates()
	today, err := server.vibeBoardForDate(r.Context(), todayDate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load today's fragments.")
		return
	}
	judge, err := server.vibeBoardForDate(r.Context(), judgeDate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load yesterday's ballot.")
		return
	}
	result, err := server.vibeBoardForDate(r.Context(), resultDate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load the latest result.")
		return
	}

	snapshot, err := server.vibeRounds.CrewSnapshot(r.Context(), crew.ID, []string{today.ID, judge.ID, result.ID})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load this crew's daily.")
		return
	}
	// Opening the room is a read. It used to freeze the palette for all three
	// boards, which meant the size was decided by whoever looked first — and the
	// natural order is to make the crew, glance at today's board, and only then
	// send the invite. A twelve-person crew formed that way played all of day one
	// on the three-row board sized for the one person who had arrived.
	//
	// The size is now locked by the first card submitted (see handleSubmitVibe),
	// so it tracks the crew right up until the round actually starts. Boards the
	// crew already started keep their locked size, which is also what keeps an old
	// result rendering the palette its cards were made from rather than one
	// reprojected for whoever is left in the crew today.
	frozen, err := server.vibeRounds.FrozenCrewBoards(r.Context(), crew.ID, []string{today.ID, judge.ID, result.ID})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load this crew's boards.")
		return
	}
	for _, board := range []*VibeBoard{&today, &judge, &result} {
		if tileCount, locked := frozen[board.ID]; locked {
			*board = projectVibeBoard(*board, tileCount)
			continue
		}
		*board = projectVibeBoardForMembers(*board, len(snapshot.Members))
	}
	response := buildVibeCrewDaily(crew, sessionID, today, judge, result, snapshot)
	if response.IsMember {
		response.CrewStreak, err = server.vibeRounds.CrewStreak(r.Context(), crew.ID, result.PublishDate)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Could not load this crew's streak.")
			return
		}
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, response)
}

func (server *Server) handleSubmitVibe(w http.ResponseWriter, r *http.Request) {
	crew, sessionID, ok := server.vibeCrewWriteRequest(w, r, "vibe-submit:", "You're submitting too quickly. Try again in a minute.")
	if !ok {
		return
	}
	var request VibeSubmissionRequest
	if !decodeJSONBody(w, r, maxVibeMutationBodyBytes, &request, "That vibe card is not valid JSON.") {
		return
	}
	board, err := server.vibeBoardForDate(r.Context(), server.todayString())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load today's fragments.")
		return
	}
	board, err = server.vibeRounds.EnsureCrewBoard(r.Context(), crew.ID, sessionID, board)
	if err != nil {
		writeVibeRoundError(w, err)
		return
	}
	request.Title, err = normalizeVibeTitle(request.Title)
	if err != nil || request.BoardID != board.ID || !validateVibeSelection(board, request.SelectedTileIDs) || !validateVibeClientID(request.ClientSubmissionID) {
		writeError(w, http.StatusUnprocessableEntity, "Pick four different fragments and give the vibe a short title.")
		return
	}
	if err := server.blocklist.reviewText(request.Title); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "That title contains a blocked word or phrase.")
		return
	}

	submission, err := server.vibeRounds.SubmitVibe(r.Context(), crew.ID, sessionID, request, server.clock())
	if err != nil {
		writeVibeRoundError(w, err)
		return
	}
	server.metrics.observeOperation("vibe_round", "submit", "success", 0)
	writeJSON(w, http.StatusCreated, toVibeCardView(submission, board, submission.MemberID, false, 0, false))
}

func (server *Server) handleCastVibeVote(w http.ResponseWriter, r *http.Request) {
	crew, sessionID, ok := server.vibeCrewWriteRequest(w, r, "vibe-vote:", "You're judging too quickly. Try again in a minute.")
	if !ok {
		return
	}
	var request VibeVoteRequest
	if !decodeJSONBody(w, r, maxVibeMutationBodyBytes, &request, "That vote is not valid JSON.") {
		return
	}
	_, judgeDate, _ := server.vibeRoundDates()
	board, err := server.vibeBoardForDate(r.Context(), judgeDate)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load yesterday's ballot.")
		return
	}
	if request.BoardID != board.ID || !validCrewID(request.SubmissionID) || !validateVibeClientID(request.ClientVoteID) {
		writeError(w, http.StatusUnprocessableEntity, "That ballot choice is not valid.")
		return
	}
	vote, err := server.vibeRounds.CastVibeVote(r.Context(), crew.ID, sessionID, request, server.clock())
	if err != nil {
		writeVibeRoundError(w, err)
		return
	}
	server.metrics.observeOperation("vibe_round", "vote", "success", 0)
	writeJSON(w, http.StatusCreated, map[string]string{"submissionId": vote.SubmissionID})
}

func (server *Server) vibeCrewWriteRequest(w http.ResponseWriter, r *http.Request, prefix, message string) (Crew, string, bool) {
	if !server.vibeRoundsEnabled(w) {
		return Crew{}, "", false
	}
	inviteCode := r.PathValue("id")
	if !validCrewID(inviteCode) {
		writeError(w, http.StatusNotFound, "Crew not found.")
		return Crew{}, "", false
	}
	if !server.allowCrewWrite(w, r, prefix, crewJoinRateLimit, crewJoinRateWindow, message) {
		return Crew{}, "", false
	}
	crew, err := server.crews.CrewByInviteCode(r.Context(), inviteCode)
	if err != nil {
		writeCrewError(w, err)
		return Crew{}, "", false
	}
	return crew, EnsureSessionID(w, r, server.secureCookies), true
}

func (server *Server) vibeRoundsEnabled(w http.ResponseWriter) bool {
	if server.crews == nil || server.vibeRounds == nil {
		writeError(w, http.StatusServiceUnavailable, "Crew rounds require a database.")
		return false
	}
	return true
}

func (server *Server) vibeBoardForDate(ctx context.Context, date string) (VibeBoard, error) {
	board, err := VibeBoardForDate(date)
	if err != nil || server.vibeRounds == nil {
		return board, err
	}
	return server.vibeRounds.EnsureBoard(ctx, board)
}

func practiceVibeBoard(stored VibeBoard) VibeBoard {
	if len(stored.Tiles) >= VibePracticeTileCount {
		return projectVibeBoard(stored, VibePracticeTileCount)
	}
	canonical, err := VibeBoardForDate(stored.PublishDate)
	if err != nil || canonical.ID != stored.ID || len(canonical.Tiles) < VibePracticeTileCount {
		return stored
	}
	for index := range stored.Tiles {
		if stored.Tiles[index] != canonical.Tiles[index] {
			return stored
		}
	}
	return projectVibeBoard(canonical, VibePracticeTileCount)
}

func (server *Server) vibeRoundDates() (today, judge, result string) {
	location, err := time.LoadLocation(server.timeZone)
	if err != nil {
		location = time.UTC
	}
	now := server.clock().In(location)
	return now.Format("2006-01-02"), now.AddDate(0, 0, -1).Format("2006-01-02"), now.AddDate(0, 0, -2).Format("2006-01-02")
}

func buildVibeCrewDaily(crew Crew, sessionID string, today, judge, result VibeBoard, snapshot VibeCrewSnapshot) vibeCrewDailyResponse {
	currentMember := VibeRoundMember{}
	isMember := false
	for _, member := range snapshot.Members {
		if member.SessionID == sessionID {
			currentMember = member
			isMember = true
			break
		}
	}

	todaySubmissions := submissionsForBoard(snapshot.Submissions, today.ID)
	judgeSubmissions := submissionsForBoard(snapshot.Submissions, judge.ID)
	resultSubmissions := submissionsForBoard(snapshot.Submissions, result.ID)
	response := vibeCrewDailyResponse{
		Crew:     toCrewView(crew, sessionID),
		IsMember: isMember,
		Today: vibeMakeView{
			Board:          today,
			SubmittedCount: len(todaySubmissions),
			MemberCount:    len(snapshot.Members),
		},
	}
	if !isMember {
		return response
	}

	todayByMember := map[string]bool{}
	for _, submission := range todaySubmissions {
		todayByMember[submission.MemberID] = true
		if submission.MemberID == currentMember.MemberID {
			card := toVibeCardView(submission, today, currentMember.MemberID, false, 0, false)
			response.Today.Submission = &card
		}
	}
	response.Members = make([]vibeMemberView, 0, len(snapshot.Members))
	for _, member := range snapshot.Members {
		view := vibeMemberView{
			DisplayName:    member.DisplayName,
			IsYou:          member.SessionID == sessionID,
			SubmittedToday: todayByMember[member.MemberID],
		}
		if crew.OwnerID == sessionID && member.SessionID != sessionID {
			view.MemberID = member.MemberID
		}
		response.Members = append(response.Members, view)
	}

	response.Judge = buildJudgeView(judge, currentMember.MemberID, judgeSubmissions, votesForBoard(snapshot.Votes, judge.ID))
	response.Result = buildResultView(result, currentMember.MemberID, resultSubmissions, votesForBoard(snapshot.Votes, result.ID))
	return response
}

func snapshotHasSession(snapshot VibeCrewSnapshot, sessionID string) bool {
	for _, member := range snapshot.Members {
		if member.SessionID == sessionID {
			return true
		}
	}
	return false
}

func buildJudgeView(board VibeBoard, currentMemberID string, submissions []VibeSubmission, votes []VibeVote) *vibeJudgeView {
	if len(submissions) == 0 {
		return nil
	}
	view := &vibeJudgeView{Board: board, SubmittedCount: len(submissions)}
	for _, submission := range submissions {
		if submission.MemberID == currentMemberID {
			view.Eligible = true
			break
		}
	}
	if !view.Eligible {
		return view
	}
	for _, vote := range votes {
		if vote.VoterMemberID == currentMemberID {
			view.HasVoted = true
			view.YourVoteID = vote.SubmissionID
			break
		}
	}
	view.Cards = make([]vibeCardView, 0, len(submissions))
	for _, submission := range submissions {
		view.Cards = append(view.Cards, toVibeCardView(submission, board, currentMemberID, false, 0, false))
	}
	return view
}

func buildResultView(board VibeBoard, currentMemberID string, submissions []VibeSubmission, votes []VibeVote) *vibeResultView {
	if len(submissions) == 0 {
		return nil
	}
	counts := map[string]int{}
	for _, vote := range votes {
		counts[vote.SubmissionID]++
	}
	maxVotes, cardsAtMax := 0, 0
	for _, count := range counts {
		switch {
		case count > maxVotes:
			maxVotes, cardsAtMax = count, 1
		case count == maxVotes:
			cardsAtMax++
		}
	}
	// A crown is only meaningful when the crew actually converged on one card.
	// Marking every card at the top as a winner made the most natural voting
	// pattern there is — everyone backing someone else, one vote each — hand out
	// a crown to all of them, which is the opposite of a result. A tie is now
	// reported as a tie and nobody is crowned.
	official := len(submissions) >= minOfficialVibeCards && len(votes) >= minOfficialVibeBallots
	soleWinner := official && maxVotes > 0 && cardsAtMax == 1
	view := &vibeResultView{
		Board:           board,
		Official:        official,
		Tied:            official && maxVotes > 0 && cardsAtMax > 1,
		SubmissionCount: len(submissions),
		VoteCount:       len(votes),
		Cards:           make([]vibeCardView, 0, len(submissions)),
	}
	for _, submission := range submissions {
		votesForCard := counts[submission.ID]
		winner := soleWinner && votesForCard == maxVotes
		view.Cards = append(view.Cards, toVibeCardView(submission, board, currentMemberID, true, votesForCard, winner))
	}
	sort.SliceStable(view.Cards, func(left, right int) bool {
		if view.Cards[left].Votes != view.Cards[right].Votes {
			return view.Cards[left].Votes > view.Cards[right].Votes
		}
		return view.Cards[left].Title < view.Cards[right].Title
	})
	return view
}

func toVibeCardView(submission VibeSubmission, board VibeBoard, currentMemberID string, revealAuthor bool, votes int, winner bool) vibeCardView {
	tileByID := make(map[string]Tile, len(board.Tiles))
	for _, tile := range board.Tiles {
		tileByID[tile.ID] = tile
	}
	tiles := make([]Tile, 0, len(submission.SelectedTileIDs))
	for _, tileID := range submission.SelectedTileIDs {
		if tile, ok := tileByID[tileID]; ok {
			tiles = append(tiles, tile)
		}
	}
	view := vibeCardView{
		ID:      submission.ID,
		Title:   submission.Title,
		Tiles:   tiles,
		IsYours: submission.MemberID == currentMemberID,
		Votes:   votes,
		Winner:  winner,
	}
	if revealAuthor {
		view.AuthorName = submission.DisplayName
	}
	return view
}

func submissionsForBoard(submissions []VibeSubmission, boardID string) []VibeSubmission {
	filtered := make([]VibeSubmission, 0)
	for _, submission := range submissions {
		if submission.BoardID == boardID {
			filtered = append(filtered, submission)
		}
	}
	return filtered
}

func votesForBoard(votes []VibeVote, boardID string) []VibeVote {
	filtered := make([]VibeVote, 0)
	for _, vote := range votes {
		if vote.BoardID == boardID {
			filtered = append(filtered, vote)
		}
	}
	return filtered
}

func validateVibeSelection(board VibeBoard, selected []string) bool {
	return validateVibeTileIDs(board.Tiles, selected)
}

func validateVibeTileIDs(tiles []Tile, selected []string) bool {
	if len(selected) != VibeCardTileCount {
		return false
	}
	available := make(map[string]struct{}, len(tiles))
	for _, tile := range tiles {
		available[tile.ID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(selected))
	for _, tileID := range selected {
		if _, ok := available[tileID]; !ok {
			return false
		}
		if _, duplicate := seen[tileID]; duplicate {
			return false
		}
		seen[tileID] = struct{}{}
	}
	return true
}

func writeVibeRoundError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotCrewMember):
		writeError(w, http.StatusForbidden, "Join this crew before taking part.")
	case errors.Is(err, ErrVibeAlreadySubmitted):
		writeError(w, http.StatusConflict, "Your vibe is already locked in.")
	case errors.Is(err, ErrVibeAlreadyVoted):
		writeError(w, http.StatusConflict, "Your ballot is already locked in.")
	case errors.Is(err, ErrVibeSubmissionNotFound):
		writeError(w, http.StatusNotFound, "That vibe card is not on this ballot.")
	case errors.Is(err, ErrVibeSelfVote):
		writeError(w, http.StatusUnprocessableEntity, "Back someone else's vibe.")
	case errors.Is(err, ErrVibeNotEligible):
		writeError(w, http.StatusForbidden, "Only people who made a card can judge this round.")
	case errors.Is(err, ErrVibeReplayConflict), errors.Is(err, ErrVibeRequestInvalid):
		writeError(w, http.StatusUnprocessableEntity, "That request could not be replayed safely.")
	default:
		writeError(w, http.StatusInternalServerError, "Could not complete that crew round action.")
	}
}
