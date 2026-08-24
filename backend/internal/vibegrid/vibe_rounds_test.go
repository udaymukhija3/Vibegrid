package vibegrid

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestVibeBoardForDateIsStableAndHasNoHiddenPartition(t *testing.T) {
	first, err := VibeBoardForDate("2026-08-19")
	if err != nil {
		t.Fatalf("board: %v", err)
	}
	again, err := VibeBoardForDate("2026-08-19")
	if err != nil {
		t.Fatalf("board again: %v", err)
	}
	next, err := VibeBoardForDate("2026-08-20")
	if err != nil {
		t.Fatalf("next board: %v", err)
	}
	if first.ID != again.ID || first.Prompt != again.Prompt || first.BoardNumber != 47 {
		t.Fatalf("same date was not stable: %#v / %#v", first, again)
	}
	if len(first.Tiles) != VibeBoardMaxTileCount {
		t.Fatalf("got %d tiles, want %d", len(first.Tiles), VibeBoardMaxTileCount)
	}
	if next.ID == first.ID || next.Prompt == first.Prompt || next.BoardNumber != 48 {
		t.Fatalf("next date did not advance the editorial board: %#v", next)
	}
	seen := map[string]bool{}
	for _, tile := range first.Tiles {
		if seen[tile.ID] || tile.Text == "" {
			t.Fatalf("invalid public fragment: %#v", tile)
		}
		seen[tile.ID] = true
	}
}

func TestAdminVibeBoardInputFreezesAValidPalette(t *testing.T) {
	tiles := make([]string, VibeBoardMaxTileCount)
	for index := range tiles {
		tiles[index] = fmt.Sprintf("fragment %02d", index+1)
	}
	input := adminVibeBoardInput{
		PublishDate: "2026-08-22",
		Prompt:      "  Build   the friend who says five more minutes. ",
		Tiles:       tiles,
	}
	board, err := input.toBoard("2026-08-21")
	if err != nil {
		t.Fatalf("to board: %v", err)
	}
	if board.Prompt != "Build the friend who says five more minutes." || len(board.Tiles) != VibeBoardMaxTileCount {
		t.Fatalf("board was not normalized: %#v", board)
	}

	input.PublishDate = "2026-08-20"
	if _, err := input.toBoard("2026-08-21"); !errors.Is(err, ErrVibeBoardInvalid) {
		t.Fatalf("past board should fail, got %v", err)
	}
	input.PublishDate = "2026-08-22"
	input.Tiles[VibeBoardMaxTileCount-1] = input.Tiles[0]
	if _, err := input.toBoard("2026-08-21"); !errors.Is(err, ErrVibeBoardInvalid) {
		t.Fatalf("duplicate fragment should fail, got %v", err)
	}
}

func TestVibeBoardRowsScaleInFourPersonBands(t *testing.T) {
	tests := []struct {
		members int
		rows    int
	}{
		{members: 0, rows: 3},
		{members: 1, rows: 3},
		{members: 4, rows: 3},
		{members: 5, rows: 4},
		{members: 8, rows: 4},
		{members: 9, rows: 5},
		{members: 12, rows: 5},
		{members: 13, rows: 6},
		{members: 16, rows: 6},
		{members: 17, rows: 7},
		{members: 20, rows: 7},
		{members: 200, rows: 7},
	}
	board, err := VibeBoardForDate("2026-08-23")
	if err != nil {
		t.Fatalf("board: %v", err)
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("members_%d", test.members), func(t *testing.T) {
			if got := vibeBoardRowsForMembers(test.members); got != test.rows {
				t.Fatalf("got %d rows, want %d", got, test.rows)
			}
			projected := projectVibeBoardForMembers(board, test.members)
			if got := len(projected.Tiles); got != test.rows*VibeBoardColumns {
				t.Fatalf("got %d tiles, want %d", got, test.rows*VibeBoardColumns)
			}
		})
	}

	legacy := board
	legacy.Tiles = append([]Tile(nil), board.Tiles[:VibeBoardMinTileCount]...)
	if got := len(projectVibeBoardForMembers(legacy, maxCrewMembers).Tiles); got != VibeBoardMinTileCount {
		t.Fatalf("legacy board grew past its immutable palette: %d", got)
	}
	if got := len(practiceVibeBoard(legacy).Tiles); got != VibePracticeTileCount {
		t.Fatalf("canonical legacy practice did not render 4x4: %d", got)
	}
}

func TestUnlimitedVibeBoardsKeepCoherentFourByFourDeals(t *testing.T) {
	first, err := UnlimitedVibeBoard(0)
	if err != nil {
		t.Fatalf("first unlimited board: %v", err)
	}
	again, err := UnlimitedVibeBoard(0)
	if err != nil {
		t.Fatalf("repeat unlimited board: %v", err)
	}
	rotated, err := UnlimitedVibeBoard(uint64(len(vibeBoardTemplates)))
	if err != nil {
		t.Fatalf("rotated unlimited board: %v", err)
	}
	if len(first.Tiles) != VibePracticeTileCount || first.ID != again.ID || first.Prompt != again.Prompt {
		t.Fatalf("unlimited deal was not deterministic 4x4: %#v / %#v", first, again)
	}
	for index := range first.Tiles {
		if first.Tiles[index] != again.Tiles[index] {
			t.Fatalf("same unlimited sequence changed tile %d", index)
		}
	}
	if rotated.Prompt != first.Prompt || rotated.ID == first.ID || rotated.Tiles[0].Text == first.Tiles[0].Text {
		t.Fatalf("next variation did not rotate one coherent master: %#v / %#v", first, rotated)
	}
	if _, err := UnlimitedVibeBoard(^uint64(0)); err != nil {
		t.Fatalf("maximum sequence should still produce a valid board: %v", err)
	}

	handler := NewServer(ServerConfig{Puzzles: StaticPuzzleSource(SeedPuzzles())})
	request := httptest.NewRequest(http.MethodGet, "/api/vibes/practice/12", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unlimited HTTP deal: status=%d body=%s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("unlimited deal cache contract: %q", got)
	}
	var wireBoard VibeBoard
	if err := json.NewDecoder(response.Body).Decode(&wireBoard); err != nil {
		t.Fatalf("decode unlimited HTTP deal: %v", err)
	}
	if len(wireBoard.Tiles) != VibePracticeTileCount || wireBoard.ID != rotated.ID {
		t.Fatalf("unlimited HTTP contract diverged: %#v", wireBoard)
	}

	invalid := httptest.NewRecorder()
	handler.ServeHTTP(invalid, httptest.NewRequest(http.MethodGet, "/api/vibes/practice/not-a-number", nil))
	if invalid.Code != http.StatusNotFound {
		t.Fatalf("invalid unlimited sequence: status=%d body=%s", invalid.Code, invalid.Body.String())
	}
}

func TestCrewDailyDisclosureAndTieRules(t *testing.T) {
	today, _ := VibeBoardForDate("2026-08-21")
	judge, _ := VibeBoardForDate("2026-08-20")
	result, _ := VibeBoardForDate("2026-08-19")
	crew := Crew{ID: "crew-internal", InviteCode: "crew_public", Name: "Night Shift", OwnerID: "owner-session"}
	members := []VibeRoundMember{
		{MemberID: "member-a", SessionID: "owner-session", DisplayName: "Ari"},
		{MemberID: "member-b", SessionID: "session-b", DisplayName: "Bea"},
		{MemberID: "member-c", SessionID: "session-c", DisplayName: "Cam"},
	}
	makeSubmission := func(id, boardID, memberID, name, title string, board VibeBoard) VibeSubmission {
		return VibeSubmission{ID: id, BoardID: boardID, MemberID: memberID, DisplayName: name, Title: title,
			SelectedTileIDs: []string{board.Tiles[0].ID, board.Tiles[1].ID, board.Tiles[2].ID, board.Tiles[3].ID}}
	}
	snapshot := VibeCrewSnapshot{
		Members: members,
		Submissions: []VibeSubmission{
			makeSubmission("today-a", today.ID, "member-a", "Ari", "Calendar cosplay", today),
			makeSubmission("judge-a", judge.ID, "member-a", "Ari", "Main character layover", judge),
			makeSubmission("judge-b", judge.ID, "member-b", "Bea", "Gate goblin", judge),
			makeSubmission("result-a", result.ID, "member-a", "Ari", "Soft launch", result),
			makeSubmission("result-b", result.ID, "member-b", "Bea", "Barely operational", result),
			makeSubmission("result-c", result.ID, "member-c", "Cam", "Fresh start fraud", result),
		},
		Votes: []VibeVote{
			{BoardID: result.ID, VoterMemberID: "member-a", SubmissionID: "result-b"},
			{BoardID: result.ID, VoterMemberID: "member-b", SubmissionID: "result-a"},
		},
	}

	outsider := buildVibeCrewDaily(crew, "not-a-member", today, judge, result, snapshot)
	if outsider.IsMember || len(outsider.Members) != 0 || outsider.Today.Submission != nil || outsider.Judge != nil || outsider.Result != nil {
		t.Fatalf("outsider received private crew content: %#v", outsider)
	}

	member := buildVibeCrewDaily(crew, "owner-session", today, judge, result, snapshot)
	if !member.IsMember || member.Today.Submission == nil || member.Today.Submission.Title != "Calendar cosplay" {
		t.Fatalf("member did not receive their own make state: %#v", member.Today)
	}
	if member.Judge == nil || !member.Judge.Eligible || len(member.Judge.Cards) != 2 {
		t.Fatalf("eligible member did not receive the blind ballot: %#v", member.Judge)
	}
	for _, card := range member.Judge.Cards {
		if card.AuthorName != "" {
			t.Fatalf("judge ballot leaked author %q", card.AuthorName)
		}
	}
	if member.Result == nil || !member.Result.Official || member.Result.VoteCount != 2 {
		t.Fatalf("result should be official: %#v", member.Result)
	}
	winners := 0
	for _, card := range member.Result.Cards {
		if card.AuthorName == "" {
			t.Fatal("result did not reveal an author")
		}
		if card.Winner {
			winners++
		}
	}
	// Ari and Bea each drew one ballot. Crowning both of them was the old
	// behaviour and it made the most natural voting pattern in the game —
	// everyone backing someone else, one vote each — hand a crown to the whole
	// crew. A split is reported as a split and nobody is crowned.
	if winners != 0 {
		t.Fatalf("a split vote must crown nobody, got %d winners", winners)
	}
	if !member.Result.Tied {
		t.Fatalf("a split vote must report as tied: %#v", member.Result)
	}

	// One extra ballot for Ari breaks the split, and the crown reappears.
	decided := snapshot
	decided.Votes = append(append([]VibeVote(nil), snapshot.Votes...),
		VibeVote{BoardID: result.ID, VoterMemberID: "member-c", SubmissionID: "result-a"})
	settled := buildVibeCrewDaily(crew, "owner-session", today, judge, result, decided)
	if settled.Result.Tied {
		t.Fatalf("a clear plurality must not report as tied: %#v", settled.Result)
	}
	crowned := []string{}
	for _, card := range settled.Result.Cards {
		if card.Winner {
			crowned = append(crowned, card.Title)
		}
	}
	if len(crowned) != 1 || crowned[0] != "Soft launch" {
		t.Fatalf("expected Ari's card alone to be crowned, got %v", crowned)
	}
}

// TestTwoPersonCrewRoundCounts covers the group the official thresholds used to
// exclude. Two people who both make a card and both judge have run the ritual
// exactly as designed, but three cards were required, so the round was
// permanently unofficial and the streak was pinned at zero with nothing in the
// product explaining why.
func TestTwoPersonCrewRoundCounts(t *testing.T) {
	today, _ := VibeBoardForDate("2026-08-21")
	judge, _ := VibeBoardForDate("2026-08-20")
	result, _ := VibeBoardForDate("2026-08-19")
	crew := Crew{ID: "crew-internal", InviteCode: "crew_public", Name: "Two Up", OwnerID: "session-a"}
	card := func(id, memberID, name, title string) VibeSubmission {
		return VibeSubmission{ID: id, BoardID: result.ID, MemberID: memberID, DisplayName: name, Title: title,
			SelectedTileIDs: []string{result.Tiles[0].ID, result.Tiles[1].ID, result.Tiles[2].ID, result.Tiles[3].ID}}
	}
	snapshot := VibeCrewSnapshot{
		Members: []VibeRoundMember{
			{MemberID: "member-a", SessionID: "session-a", DisplayName: "Ari"},
			{MemberID: "member-b", SessionID: "session-b", DisplayName: "Bea"},
		},
		Submissions: []VibeSubmission{
			card("result-a", "member-a", "Ari", "Soft launch"),
			card("result-b", "member-b", "Bea", "Barely operational"),
		},
		// With two people neither can vote for themselves, so the round is always
		// a one-all split. It still counts; it just never crowns anyone.
		Votes: []VibeVote{
			{BoardID: result.ID, VoterMemberID: "member-a", SubmissionID: "result-b"},
			{BoardID: result.ID, VoterMemberID: "member-b", SubmissionID: "result-a"},
		},
	}

	view := buildVibeCrewDaily(crew, "session-a", today, judge, result, snapshot)
	if view.Result == nil || !view.Result.Official {
		t.Fatalf("a two-person round with two cards and two ballots must count: %#v", view.Result)
	}
	if !view.Result.Tied {
		t.Fatalf("a forced two-person split must report as tied: %#v", view.Result)
	}
	for _, card := range view.Result.Cards {
		if card.Winner {
			t.Fatalf("a forced split must crown nobody, got %q", card.Title)
		}
	}
}

func TestPostgresVibeRoundTransactions(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres vibe-round integration test")
	}
	ctx := context.Background()
	if err := MigrateDB(ctx, databaseURL); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	database, err := OpenDB(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Exec(`truncate vibe_votes, vibe_submissions, vibe_daily_boards, crew_members, crews restart identity cascade`); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	crewStore := NewPostgresCrewStore(database)
	rounds := NewPostgresVibeRoundStore(database)
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	crew, err := crewStore.CreateCrew(ctx, "Night Shift", "Ari", "session-a", now)
	if err != nil {
		t.Fatalf("create crew: %v", err)
	}
	for _, member := range []struct{ name, session string }{{"Bea", "session-b"}, {"Cam", "session-c"}} {
		if _, err := crewStore.JoinCrew(ctx, crew.InviteCode, member.name, member.session, now); err != nil {
			t.Fatalf("join %s: %v", member.name, err)
		}
	}
	board, _ := VibeBoardForDate("2026-08-21")
	if _, err := rounds.CreateVibeBoard(ctx, board); err != nil {
		t.Fatalf("create board: %v", err)
	}
	if projected, err := rounds.EnsureCrewBoard(ctx, crew.ID, "session-a", board); err != nil || len(projected.Tiles) != 12 {
		t.Fatalf("freeze three-member board: tiles=%d err=%v", len(projected.Tiles), err)
	}
	selection := []string{board.Tiles[0].ID, board.Tiles[1].ID, board.Tiles[2].ID, board.Tiles[3].ID}
	submit := func(session, clientID, title string) VibeSubmission {
		card, submitErr := rounds.SubmitVibe(ctx, crew.ID, session, VibeSubmissionRequest{
			BoardID: board.ID, Title: title, SelectedTileIDs: selection, ClientSubmissionID: clientID,
		}, now)
		if submitErr != nil {
			t.Fatalf("submit %s: %v", session, submitErr)
		}
		return card
	}
	a := submit("session-a", "submit-a", "Soft launch")
	b := submit("session-b", "submit-b", "Barely operational")
	_ = submit("session-c", "submit-c", "Fresh start fraud")

	replayed, err := rounds.SubmitVibe(ctx, crew.ID, "session-a", VibeSubmissionRequest{
		BoardID: board.ID, Title: "Soft launch", SelectedTileIDs: selection, ClientSubmissionID: "submit-a",
	}, now.Add(time.Minute))
	if err != nil || replayed.ID != a.ID {
		t.Fatalf("safe replay changed the submission: %#v, %v", replayed, err)
	}
	if _, err := rounds.SubmitVibe(ctx, crew.ID, "session-a", VibeSubmissionRequest{
		BoardID: board.ID, Title: "Different title", SelectedTileIDs: selection, ClientSubmissionID: "submit-a",
	}, now); !errors.Is(err, ErrVibeReplayConflict) {
		t.Fatalf("changed replay should conflict, got %v", err)
	}
	if _, err := rounds.SubmitVibe(ctx, crew.ID, "outsider", VibeSubmissionRequest{
		BoardID: board.ID, Title: "Nope", SelectedTileIDs: selection, ClientSubmissionID: "submit-x",
	}, now); !errors.Is(err, ErrNotCrewMember) {
		t.Fatalf("outsider submit should fail, got %v", err)
	}

	voteA, err := rounds.CastVibeVote(ctx, crew.ID, "session-a", VibeVoteRequest{
		BoardID: board.ID, SubmissionID: b.ID, ClientVoteID: "vote-a",
	}, now)
	if err != nil {
		t.Fatalf("vote a: %v", err)
	}
	replayedVote, err := rounds.CastVibeVote(ctx, crew.ID, "session-a", VibeVoteRequest{
		BoardID: board.ID, SubmissionID: b.ID, ClientVoteID: "vote-a",
	}, now.Add(time.Minute))
	if err != nil || replayedVote.ID != voteA.ID {
		t.Fatalf("safe vote replay changed the ballot: %#v, %v", replayedVote, err)
	}
	if _, err := rounds.CastVibeVote(ctx, crew.ID, "session-b", VibeVoteRequest{
		BoardID: board.ID, SubmissionID: b.ID, ClientVoteID: "vote-self",
	}, now); !errors.Is(err, ErrVibeSelfVote) {
		t.Fatalf("self vote should fail, got %v", err)
	}
	if _, err := rounds.CastVibeVote(ctx, crew.ID, "session-b", VibeVoteRequest{
		BoardID: board.ID, SubmissionID: a.ID, ClientVoteID: "vote-b",
	}, now); err != nil {
		t.Fatalf("vote b: %v", err)
	}

	snapshot, err := rounds.CrewSnapshot(ctx, crew.ID, []string{board.ID})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(snapshot.Members) != 3 || len(snapshot.Submissions) != 3 || len(snapshot.Votes) != 2 {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	streak, err := rounds.CrewStreak(ctx, crew.ID, board.PublishDate)
	if err != nil || streak != 1 {
		t.Fatalf("official round should count toward streak: %d, %v", streak, err)
	}

	// Different browser actions can race before either response arrives. The
	// database constraint is the final arbiter, and the loser must receive a
	// product conflict rather than a generic 500.
	raceBoard, _ := VibeBoardForDate("2026-08-22")
	if _, err := rounds.CreateVibeBoard(ctx, raceBoard); err != nil {
		t.Fatalf("create race board: %v", err)
	}
	if _, err := rounds.EnsureCrewBoard(ctx, crew.ID, "session-a", raceBoard); err != nil {
		t.Fatalf("freeze race board: %v", err)
	}
	raceSelection := []string{raceBoard.Tiles[0].ID, raceBoard.Tiles[1].ID, raceBoard.Tiles[2].ID, raceBoard.Tiles[3].ID}
	type submitOutcome struct {
		card VibeSubmission
		err  error
	}
	startSubmits := make(chan struct{})
	submitOutcomes := make(chan submitOutcome, 2)
	for _, clientID := range []string{"race-submit-a", "race-submit-b"} {
		go func(clientID string) {
			<-startSubmits
			card, submitErr := rounds.SubmitVibe(ctx, crew.ID, "session-a", VibeSubmissionRequest{
				BoardID: raceBoard.ID, Title: "Concurrent card", SelectedTileIDs: raceSelection, ClientSubmissionID: clientID,
			}, now)
			submitOutcomes <- submitOutcome{card: card, err: submitErr}
		}(clientID)
	}
	close(startSubmits)
	racedCard := VibeSubmission{}
	submitConflicts := 0
	for range 2 {
		outcome := <-submitOutcomes
		switch {
		case outcome.err == nil:
			racedCard = outcome.card
		case errors.Is(outcome.err, ErrVibeAlreadySubmitted):
			submitConflicts++
		default:
			t.Fatalf("unexpected concurrent submit error: %v", outcome.err)
		}
	}
	if racedCard.ID == "" || submitConflicts != 1 {
		t.Fatalf("concurrent submit should produce one card and one conflict: %#v, conflicts=%d", racedCard, submitConflicts)
	}
	raceTarget, err := rounds.SubmitVibe(ctx, crew.ID, "session-b", VibeSubmissionRequest{
		BoardID: raceBoard.ID, Title: "Vote target", SelectedTileIDs: raceSelection, ClientSubmissionID: "race-target",
	}, now)
	if err != nil {
		t.Fatalf("create race vote target: %v", err)
	}
	type voteOutcome struct{ err error }
	startVotes := make(chan struct{})
	voteOutcomes := make(chan voteOutcome, 2)
	for _, clientID := range []string{"race-vote-a", "race-vote-b"} {
		go func(clientID string) {
			<-startVotes
			_, voteErr := rounds.CastVibeVote(ctx, crew.ID, "session-a", VibeVoteRequest{
				BoardID: raceBoard.ID, SubmissionID: raceTarget.ID, ClientVoteID: clientID,
			}, now)
			voteOutcomes <- voteOutcome{err: voteErr}
		}(clientID)
	}
	close(startVotes)
	voteSuccesses, voteConflicts := 0, 0
	for range 2 {
		outcome := <-voteOutcomes
		switch {
		case outcome.err == nil:
			voteSuccesses++
		case errors.Is(outcome.err, ErrVibeAlreadyVoted):
			voteConflicts++
		default:
			t.Fatalf("unexpected concurrent vote error: %v", outcome.err)
		}
	}
	if voteSuccesses != 1 || voteConflicts != 1 {
		t.Fatalf("concurrent vote should produce one ballot and one conflict: successes=%d conflicts=%d", voteSuccesses, voteConflicts)
	}

	// A network retry can overlap the still-running original request on another
	// process. Every identical caller must observe the winner, not an arbitrary
	// uniqueness error, while the database still stores one row.
	exactBoard, _ := VibeBoardForDate("2026-08-23")
	if _, err := rounds.CreateVibeBoard(ctx, exactBoard); err != nil {
		t.Fatalf("create exact replay board: %v", err)
	}
	if _, err := rounds.EnsureCrewBoard(ctx, crew.ID, "session-a", exactBoard); err != nil {
		t.Fatalf("freeze exact replay board: %v", err)
	}
	exactSelection := []string{exactBoard.Tiles[0].ID, exactBoard.Tiles[1].ID, exactBoard.Tiles[2].ID, exactBoard.Tiles[3].ID}
	const exactRacers = 16
	startExactSubmits := make(chan struct{})
	exactSubmitOutcomes := make(chan submitOutcome, exactRacers)
	for range exactRacers {
		go func() {
			<-startExactSubmits
			card, submitErr := rounds.SubmitVibe(ctx, crew.ID, "session-a", VibeSubmissionRequest{
				BoardID: exactBoard.ID, Title: "One logical card", SelectedTileIDs: exactSelection, ClientSubmissionID: "same-submit-replay",
			}, now)
			exactSubmitOutcomes <- submitOutcome{card: card, err: submitErr}
		}()
	}
	close(startExactSubmits)
	exactCardID := ""
	for range exactRacers {
		outcome := <-exactSubmitOutcomes
		if outcome.err != nil {
			t.Fatalf("identical concurrent submit did not replay the winner: %v", outcome.err)
		}
		if exactCardID == "" {
			exactCardID = outcome.card.ID
		} else if outcome.card.ID != exactCardID {
			t.Fatalf("identical concurrent submit returned different cards: %s / %s", exactCardID, outcome.card.ID)
		}
	}

	exactTarget, err := rounds.SubmitVibe(ctx, crew.ID, "session-b", VibeSubmissionRequest{
		BoardID: exactBoard.ID, Title: "Exact vote target", SelectedTileIDs: exactSelection, ClientSubmissionID: "exact-target",
	}, now)
	if err != nil {
		t.Fatalf("create exact vote target: %v", err)
	}
	type exactVoteOutcome struct {
		vote VibeVote
		err  error
	}
	startExactVotes := make(chan struct{})
	exactVoteOutcomes := make(chan exactVoteOutcome, exactRacers)
	for range exactRacers {
		go func() {
			<-startExactVotes
			vote, voteErr := rounds.CastVibeVote(ctx, crew.ID, "session-a", VibeVoteRequest{
				BoardID: exactBoard.ID, SubmissionID: exactTarget.ID, ClientVoteID: "same-vote-replay",
			}, now)
			exactVoteOutcomes <- exactVoteOutcome{vote: vote, err: voteErr}
		}()
	}
	close(startExactVotes)
	exactVoteID := ""
	for range exactRacers {
		outcome := <-exactVoteOutcomes
		if outcome.err != nil {
			t.Fatalf("identical concurrent vote did not replay the winner: %v", outcome.err)
		}
		if exactVoteID == "" {
			exactVoteID = outcome.vote.ID
		} else if outcome.vote.ID != exactVoteID {
			t.Fatalf("identical concurrent vote returned different ballots: %s / %s", exactVoteID, outcome.vote.ID)
		}
	}

	// The system cap is 20 members. At that size every opener must converge on
	// seven rows, and 20 distinct card writes plus 20 distinct ballots must all
	// survive concurrent delivery without lost rows or false conflicts.
	scaleCrew, err := crewStore.CreateCrew(ctx, "Scale Lab", "Player 00", "scale-session-00", now)
	if err != nil {
		t.Fatalf("create scale crew: %v", err)
	}
	scaleSessions := []string{"scale-session-00"}
	for index := 1; index < maxCrewMembers; index++ {
		session := fmt.Sprintf("scale-session-%02d", index)
		if _, err := crewStore.JoinCrew(ctx, scaleCrew.InviteCode, fmt.Sprintf("Player %02d", index), session, now); err != nil {
			t.Fatalf("join scale member %d: %v", index, err)
		}
		scaleSessions = append(scaleSessions, session)
	}
	scaleBoard, _ := VibeBoardForDate("2026-08-24")
	if _, err := rounds.CreateVibeBoard(ctx, scaleBoard); err != nil {
		t.Fatalf("create scale board: %v", err)
	}
	type boardOutcome struct {
		board VibeBoard
		err   error
	}
	const boardOpeners = 32
	startBoardOpens := make(chan struct{})
	boardOutcomes := make(chan boardOutcome, boardOpeners)
	for index := range boardOpeners {
		session := scaleSessions[index%len(scaleSessions)]
		go func() {
			<-startBoardOpens
			projected, freezeErr := rounds.EnsureCrewBoard(ctx, scaleCrew.ID, session, scaleBoard)
			boardOutcomes <- boardOutcome{board: projected, err: freezeErr}
		}()
	}
	close(startBoardOpens)
	for range boardOpeners {
		outcome := <-boardOutcomes
		if outcome.err != nil || len(outcome.board.Tiles) != VibeBoardMaxTileCount {
			t.Fatalf("max-size crew board diverged: tiles=%d err=%v", len(outcome.board.Tiles), outcome.err)
		}
	}

	scaleSelection := []string{scaleBoard.Tiles[0].ID, scaleBoard.Tiles[1].ID, scaleBoard.Tiles[2].ID, scaleBoard.Tiles[3].ID}
	type indexedSubmitOutcome struct {
		index int
		card  VibeSubmission
		err   error
	}
	startScaleSubmits := make(chan struct{})
	scaleSubmitOutcomes := make(chan indexedSubmitOutcome, maxCrewMembers)
	for index, session := range scaleSessions {
		go func() {
			<-startScaleSubmits
			card, submitErr := rounds.SubmitVibe(ctx, scaleCrew.ID, session, VibeSubmissionRequest{
				BoardID: scaleBoard.ID, Title: fmt.Sprintf("Card %02d", index), SelectedTileIDs: scaleSelection,
				ClientSubmissionID: fmt.Sprintf("scale-submit-%02d", index),
			}, now)
			scaleSubmitOutcomes <- indexedSubmitOutcome{index: index, card: card, err: submitErr}
		}()
	}
	close(startScaleSubmits)
	scaleCards := make([]VibeSubmission, maxCrewMembers)
	for range maxCrewMembers {
		outcome := <-scaleSubmitOutcomes
		if outcome.err != nil {
			t.Fatalf("concurrent scale submit %d: %v", outcome.index, outcome.err)
		}
		scaleCards[outcome.index] = outcome.card
	}

	startScaleVotes := make(chan struct{})
	scaleVoteErrors := make(chan error, maxCrewMembers)
	for index, session := range scaleSessions {
		target := scaleCards[(index+1)%len(scaleCards)]
		go func() {
			<-startScaleVotes
			_, voteErr := rounds.CastVibeVote(ctx, scaleCrew.ID, session, VibeVoteRequest{
				BoardID: scaleBoard.ID, SubmissionID: target.ID, ClientVoteID: fmt.Sprintf("scale-vote-%02d", index),
			}, now)
			scaleVoteErrors <- voteErr
		}()
	}
	close(startScaleVotes)
	for range maxCrewMembers {
		if err := <-scaleVoteErrors; err != nil {
			t.Fatalf("concurrent scale vote: %v", err)
		}
	}
	scaleSnapshot, err := rounds.CrewSnapshot(ctx, scaleCrew.ID, []string{scaleBoard.ID})
	if err != nil || len(scaleSnapshot.Members) != 20 || len(scaleSnapshot.Submissions) != 20 || len(scaleSnapshot.Votes) != 20 {
		t.Fatalf("max-size snapshot lost concurrent state: members=%d cards=%d votes=%d err=%v",
			len(scaleSnapshot.Members), len(scaleSnapshot.Submissions), len(scaleSnapshot.Votes), err)
	}

	// Once frozen, later joins cannot enlarge a round. A tile present in the
	// master palette but outside the crew projection is rejected in the write
	// transaction, not merely hidden by the browser.
	freezeCrew, err := crewStore.CreateCrew(ctx, "Freeze Lab", "Freeze 0", "freeze-session-0", now)
	if err != nil {
		t.Fatalf("create freeze crew: %v", err)
	}
	for index := 1; index < 5; index++ {
		if _, err := crewStore.JoinCrew(ctx, freezeCrew.InviteCode, fmt.Sprintf("Freeze %d", index), fmt.Sprintf("freeze-session-%d", index), now); err != nil {
			t.Fatalf("join freeze member %d: %v", index, err)
		}
	}
	freezeBoard, _ := VibeBoardForDate("2026-08-25")
	if _, err := rounds.CreateVibeBoard(ctx, freezeBoard); err != nil {
		t.Fatalf("create freeze board: %v", err)
	}
	frozen, err := rounds.EnsureCrewBoard(ctx, freezeCrew.ID, "freeze-session-0", freezeBoard)
	if err != nil || len(frozen.Tiles) != 16 {
		t.Fatalf("five-member crew should freeze at 4x4: tiles=%d err=%v", len(frozen.Tiles), err)
	}
	for index := 5; index < 9; index++ {
		if _, err := crewStore.JoinCrew(ctx, freezeCrew.InviteCode, fmt.Sprintf("Freeze %d", index), fmt.Sprintf("freeze-session-%d", index), now); err != nil {
			t.Fatalf("join post-freeze member %d: %v", index, err)
		}
	}
	stillFrozen, err := rounds.EnsureCrewBoard(ctx, freezeCrew.ID, "freeze-session-8", freezeBoard)
	if err != nil || len(stillFrozen.Tiles) != 16 {
		t.Fatalf("post-freeze joins resized the board: tiles=%d err=%v", len(stillFrozen.Tiles), err)
	}
	outOfPalette := []string{freezeBoard.Tiles[0].ID, freezeBoard.Tiles[1].ID, freezeBoard.Tiles[2].ID, freezeBoard.Tiles[16].ID}
	if _, err := rounds.SubmitVibe(ctx, freezeCrew.ID, "freeze-session-0", VibeSubmissionRequest{
		BoardID: freezeBoard.ID, Title: "Cannot see this", SelectedTileIDs: outOfPalette, ClientSubmissionID: "outside-frozen-palette",
	}, now); !errors.Is(err, ErrVibeRequestInvalid) {
		t.Fatalf("out-of-palette tile should fail transaction validation, got %v", err)
	}

	// A first open racing the fifth join is linearizable: the board is either
	// frozen before the join at 3x4 or after it at 4x4, never a torn size, and all
	// subsequent readers receive the winner.
	joinRaceCrew, err := crewStore.CreateCrew(ctx, "Join Race", "Join 0", "join-race-0", now)
	if err != nil {
		t.Fatalf("create join race crew: %v", err)
	}
	for index := 1; index < 4; index++ {
		if _, err := crewStore.JoinCrew(ctx, joinRaceCrew.InviteCode, fmt.Sprintf("Join %d", index), fmt.Sprintf("join-race-%d", index), now); err != nil {
			t.Fatalf("seed join race %d: %v", index, err)
		}
	}
	joinRaceBoard, _ := VibeBoardForDate("2026-08-26")
	if _, err := rounds.CreateVibeBoard(ctx, joinRaceBoard); err != nil {
		t.Fatalf("create join race board: %v", err)
	}
	startJoinRace := make(chan struct{})
	joinRaceBoardResult := make(chan boardOutcome, 1)
	joinRaceJoinError := make(chan error, 1)
	go func() {
		<-startJoinRace
		projected, freezeErr := rounds.EnsureCrewBoard(ctx, joinRaceCrew.ID, "join-race-0", joinRaceBoard)
		joinRaceBoardResult <- boardOutcome{board: projected, err: freezeErr}
	}()
	go func() {
		<-startJoinRace
		_, joinErr := crewStore.JoinCrew(ctx, joinRaceCrew.InviteCode, "Join 4", "join-race-4", now)
		joinRaceJoinError <- joinErr
	}()
	close(startJoinRace)
	joinRaceOutcome := <-joinRaceBoardResult
	if err := <-joinRaceJoinError; err != nil {
		t.Fatalf("fifth-member join race failed: %v", err)
	}
	if joinRaceOutcome.err != nil || (len(joinRaceOutcome.board.Tiles) != 12 && len(joinRaceOutcome.board.Tiles) != 16) {
		t.Fatalf("join/freeze race produced torn board: tiles=%d err=%v", len(joinRaceOutcome.board.Tiles), joinRaceOutcome.err)
	}
	afterJoinRace, err := rounds.EnsureCrewBoard(ctx, joinRaceCrew.ID, "join-race-4", joinRaceBoard)
	if err != nil || len(afterJoinRace.Tiles) != len(joinRaceOutcome.board.Tiles) {
		t.Fatalf("join/freeze race did not preserve its winner: first=%d later=%d err=%v",
			len(joinRaceOutcome.board.Tiles), len(afterJoinRace.Tiles), err)
	}

	// Eight people trying the last place at once must still produce one winner,
	// seven capacity errors, and a 20-member room. The same crew-row lock used by
	// sizing is also the arbiter for the membership cap.
	capCrew, err := crewStore.CreateCrew(ctx, "Capacity Race", "Cap 00", "cap-session-00", now)
	if err != nil {
		t.Fatalf("create capacity race crew: %v", err)
	}
	for index := 1; index < maxCrewMembers-1; index++ {
		if _, err := crewStore.JoinCrew(ctx, capCrew.InviteCode, fmt.Sprintf("Cap %02d", index), fmt.Sprintf("cap-session-%02d", index), now); err != nil {
			t.Fatalf("seed capacity race %d: %v", index, err)
		}
	}
	const lastSeatRacers = 8
	startLastSeat := make(chan struct{})
	lastSeatErrors := make(chan error, lastSeatRacers)
	for index := range lastSeatRacers {
		go func() {
			<-startLastSeat
			_, joinErr := crewStore.JoinCrew(ctx, capCrew.InviteCode, fmt.Sprintf("Late %02d", index), fmt.Sprintf("cap-late-%02d", index), now)
			lastSeatErrors <- joinErr
		}()
	}
	close(startLastSeat)
	lastSeatSuccesses, lastSeatFull := 0, 0
	for range lastSeatRacers {
		switch joinErr := <-lastSeatErrors; {
		case joinErr == nil:
			lastSeatSuccesses++
		case errors.Is(joinErr, ErrCrewFull):
			lastSeatFull++
		default:
			t.Fatalf("unexpected last-seat join error: %v", joinErr)
		}
	}
	capSnapshot, err := rounds.CrewSnapshot(ctx, capCrew.ID, nil)
	if err != nil || lastSeatSuccesses != 1 || lastSeatFull != lastSeatRacers-1 || len(capSnapshot.Members) != maxCrewMembers {
		t.Fatalf("capacity race diverged: successes=%d full=%d members=%d err=%v",
			lastSeatSuccesses, lastSeatFull, len(capSnapshot.Members), err)
	}

	// A fifth member exiting while the owner first opens the board is also
	// linearizable. Removal and voluntary leave both share the crew lock, so the
	// frozen result is either 3x4 or 4x4 and can never change afterward.
	for caseIndex, exitKind := range []string{"remove", "leave"} {
		t.Run("freeze_races_"+exitKind, func(t *testing.T) {
			ownerSession := fmt.Sprintf("%s-owner", exitKind)
			exitCrew, createErr := crewStore.CreateCrew(ctx, "Exit Race "+exitKind, "Owner", ownerSession, now)
			if createErr != nil {
				t.Fatalf("create exit race crew: %v", createErr)
			}
			departingSession := ""
			for index := 1; index < 5; index++ {
				session := fmt.Sprintf("%s-member-%d", exitKind, index)
				if _, joinErr := crewStore.JoinCrew(ctx, exitCrew.InviteCode, fmt.Sprintf("Member %d", index), session, now); joinErr != nil {
					t.Fatalf("seed exit race %d: %v", index, joinErr)
				}
				departingSession = session
			}
			exitSnapshot, snapshotErr := rounds.CrewSnapshot(ctx, exitCrew.ID, nil)
			if snapshotErr != nil {
				t.Fatalf("load exit race membership: %v", snapshotErr)
			}
			departingMemberID := ""
			for _, member := range exitSnapshot.Members {
				if member.SessionID == departingSession {
					departingMemberID = member.MemberID
					break
				}
			}
			if departingMemberID == "" {
				t.Fatal("departing member was not found")
			}

			exitBoard, _ := VibeBoardForDate(fmt.Sprintf("2026-08-%02d", 27+caseIndex))
			if _, createBoardErr := rounds.CreateVibeBoard(ctx, exitBoard); createBoardErr != nil {
				t.Fatalf("create exit race board: %v", createBoardErr)
			}
			startExitRace := make(chan struct{})
			exitBoardResult := make(chan boardOutcome, 1)
			exitError := make(chan error, 1)
			go func() {
				<-startExitRace
				projected, freezeErr := rounds.EnsureCrewBoard(ctx, exitCrew.ID, ownerSession, exitBoard)
				exitBoardResult <- boardOutcome{board: projected, err: freezeErr}
			}()
			go func() {
				<-startExitRace
				if exitKind == "remove" {
					exitError <- crewStore.RemoveMember(ctx, exitCrew.ID, departingMemberID, ownerSession)
					return
				}
				exitError <- crewStore.LeaveCrew(ctx, exitCrew.ID, departingSession)
			}()
			close(startExitRace)
			exitOutcome := <-exitBoardResult
			if exitErr := <-exitError; exitErr != nil {
				t.Fatalf("%s/freeze race failed: %v", exitKind, exitErr)
			}
			if exitOutcome.err != nil || (len(exitOutcome.board.Tiles) != 12 && len(exitOutcome.board.Tiles) != 16) {
				t.Fatalf("%s/freeze race produced torn board: tiles=%d err=%v", exitKind, len(exitOutcome.board.Tiles), exitOutcome.err)
			}
			afterExit, freezeErr := rounds.EnsureCrewBoard(ctx, exitCrew.ID, ownerSession, exitBoard)
			if freezeErr != nil || len(afterExit.Tiles) != len(exitOutcome.board.Tiles) {
				t.Fatalf("%s/freeze race did not preserve its winner: first=%d later=%d err=%v",
					exitKind, len(exitOutcome.board.Tiles), len(afterExit.Tiles), freezeErr)
			}
		})
	}
}

// TestPostgresCrewBoardLocksOnPlayNotOnLook covers the ordering that made the
// crew-sized palette useless on the day it mattered. The natural sequence is to
// make a crew, glance at today's board, then send the invite — and because the
// glance froze the size, a crew that filled up afterwards spent all of day one
// on the three-row board sized for the one person who had arrived.
func TestPostgresCrewBoardLocksOnPlayNotOnLook(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres vibe-round integration test")
	}
	ctx := context.Background()
	if err := MigrateDB(ctx, databaseURL); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	database, err := OpenDB(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Exec(`truncate vibe_votes, vibe_submissions, vibe_daily_boards, crew_members, crews restart identity cascade`); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	crewStore := NewPostgresCrewStore(database)
	rounds := NewPostgresVibeRoundStore(database)
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	crew, err := crewStore.CreateCrew(ctx, "Slow Fill", "Ari", "session-a", now)
	if err != nil {
		t.Fatalf("create crew: %v", err)
	}
	board, _ := VibeBoardForDate("2026-08-21")
	if _, err := rounds.EnsureBoard(ctx, board); err != nil {
		t.Fatalf("persist board: %v", err)
	}

	// The owner looks at the board alone. Nothing may be locked yet.
	frozen, err := rounds.FrozenCrewBoards(ctx, crew.ID, []string{board.ID})
	if err != nil {
		t.Fatalf("read frozen boards: %v", err)
	}
	if len(frozen) != 0 {
		t.Fatalf("opening the room must not lock a size, got %#v", frozen)
	}

	// Eleven more people arrive before anybody plays.
	for index := 0; index < 11; index++ {
		name := fmt.Sprintf("P%02d", index)
		if _, err := crewStore.JoinCrew(ctx, crew.InviteCode, name, "session-"+name, now); err != nil {
			t.Fatalf("join %s: %v", name, err)
		}
	}

	// The first card locks the palette, and it is sized for the full room:
	// twelve members is five rows, not the three the owner would have frozen.
	locked, err := rounds.EnsureCrewBoard(ctx, crew.ID, "session-a", board)
	if err != nil {
		t.Fatalf("lock board on submit: %v", err)
	}
	if want := vibeBoardRowsForMembers(12) * VibeBoardColumns; len(locked.Tiles) != want {
		t.Fatalf("expected a %d-fragment palette for twelve members, got %d", want, len(locked.Tiles))
	}

	frozen, err = rounds.FrozenCrewBoards(ctx, crew.ID, []string{board.ID})
	if err != nil {
		t.Fatalf("read frozen boards after play: %v", err)
	}
	if frozen[board.ID] != len(locked.Tiles) {
		t.Fatalf("frozen size %d does not match the dealt palette %d", frozen[board.ID], len(locked.Tiles))
	}

	// Once play has started the size is settled, even as the room keeps changing.
	if _, err := crewStore.JoinCrew(ctx, crew.InviteCode, "Late", "session-late", now); err != nil {
		t.Fatalf("late join: %v", err)
	}
	stable, err := rounds.EnsureCrewBoard(ctx, crew.ID, "session-late", board)
	if err != nil {
		t.Fatalf("late member board: %v", err)
	}
	if len(stable.Tiles) != len(locked.Tiles) {
		t.Fatalf("a started round must not resize: had %d, got %d", len(locked.Tiles), len(stable.Tiles))
	}
}

// TestPracticeHouseCardsAreOnPromptAndVaried guards the acquisition surface. A
// single trio of titles used to be reused on every board forever, so the only
// round a newcomer ever saw was the same three canned opponents.
func TestPracticeHouseCardsAreOnPromptAndVaried(t *testing.T) {
	seen := map[string]string{}
	for index, template := range vibeBoardTemplates {
		cards := vibeHouseCardsFor(index)
		if len(cards) != vibePracticeHouseCards {
			t.Fatalf("template %d dealt %d house cards", index, len(cards))
		}
		for _, card := range cards {
			if !validVibeText(card.Title, MaxVibeTitleRunes) {
				t.Fatalf("template %d has an unusable house title %q", index, card.Title)
			}
			if owner, exists := seen[card.Title]; exists {
				t.Fatalf("house title %q is shared by %q and %q", card.Title, owner, template.prompt)
			}
			seen[card.Title] = template.prompt
			if len(card.TileIndices) != VibeCardTileCount {
				t.Fatalf("house card %q claims %d fragments", card.Title, len(card.TileIndices))
			}
			for _, tileIndex := range card.TileIndices {
				if tileIndex < 0 || tileIndex >= VibePracticeTileCount {
					t.Fatalf("house card %q reaches fragment %d, outside the practice board", card.Title, tileIndex)
				}
			}
		}
	}
}
