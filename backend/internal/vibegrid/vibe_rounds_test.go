package vibegrid

import (
	"context"
	"errors"
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
	if len(first.Tiles) != VibeBoardTileCount {
		t.Fatalf("got %d tiles, want %d", len(first.Tiles), VibeBoardTileCount)
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
	input := adminVibeBoardInput{
		PublishDate: "2026-08-22",
		Prompt:      "  Build   the friend who says five more minutes. ",
		Tiles: []string{
			"one shoe", "phone at 2%", "already outside", "just showered",
			"wrong train", "tiny coffee", "sent a pin", "lost keys",
			"almost there", "called twice", "new excuse", "door still locked",
		},
	}
	board, err := input.toBoard("2026-08-21")
	if err != nil {
		t.Fatalf("to board: %v", err)
	}
	if board.Prompt != "Build the friend who says five more minutes." || len(board.Tiles) != 12 {
		t.Fatalf("board was not normalized: %#v", board)
	}

	input.PublishDate = "2026-08-20"
	if _, err := input.toBoard("2026-08-21"); !errors.Is(err, ErrVibeBoardInvalid) {
		t.Fatalf("past board should fail, got %v", err)
	}
	input.PublishDate = "2026-08-22"
	input.Tiles[11] = input.Tiles[0]
	if _, err := input.toBoard("2026-08-21"); !errors.Is(err, ErrVibeBoardInvalid) {
		t.Fatalf("duplicate fragment should fail, got %v", err)
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
	if winners != 2 {
		t.Fatalf("tie should preserve both winners, got %d", winners)
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
}
