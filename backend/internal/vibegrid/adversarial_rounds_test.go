package vibegrid

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"sync"
	"testing"
	"time"
)

// adversarialFixture is a real-Postgres crew round harness for hostile probes.
// It deliberately uses the same stores the HTTP handlers use, because most of
// the invariants under test (one card per member, one ballot per member,
// no self-vote, frozen board size) are enforced inside store transactions.
type adversarialFixture struct {
	ctx    context.Context
	db     *sql.DB
	crews  *PostgresCrewStore
	rounds *PostgresVibeRoundStore
	crew   Crew
	board  VibeBoard
	now    time.Time
}

func newAdversarialFixture(t *testing.T, date string) *adversarialFixture {
	t.Helper()
	databaseURL := requireTestDatabase(t)
	ctx := context.Background()
	if err := MigrateDB(ctx, databaseURL); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	database, err := OpenDB(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Exec(
		`truncate vibe_votes, vibe_submissions, vibe_crew_boards, vibe_board_expansions,
		 vibe_daily_boards, crew_members, crews restart identity cascade`); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	fixture := &adversarialFixture{
		ctx:    ctx,
		db:     database,
		crews:  NewPostgresCrewStore(database),
		rounds: NewPostgresVibeRoundStore(database),
		now:    time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC),
	}
	board, err := VibeBoardForDate(date)
	if err != nil {
		t.Fatalf("bank board: %v", err)
	}
	stored, err := fixture.rounds.EnsureBoard(ctx, board)
	if err != nil {
		t.Fatalf("ensure board: %v", err)
	}
	fixture.board = stored
	return fixture
}

func requireTestDatabase(t *testing.T) string {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres adversarial test")
	}
	return databaseURL
}

func (fixture *adversarialFixture) createCrew(t *testing.T, name, displayName, session string) Crew {
	t.Helper()
	crew, err := fixture.crews.CreateCrew(fixture.ctx, name, displayName, session, fixture.now)
	if err != nil {
		t.Fatalf("create crew: %v", err)
	}
	fixture.crew = crew
	return crew
}

func (fixture *adversarialFixture) join(t *testing.T, displayName, session string) {
	t.Helper()
	if _, err := fixture.crews.JoinCrew(fixture.ctx, fixture.crew.InviteCode, displayName, session, fixture.now); err != nil {
		t.Fatalf("join %s: %v", displayName, err)
	}
}

func (fixture *adversarialFixture) freeze(t *testing.T, session string) VibeBoard {
	t.Helper()
	projected, err := fixture.rounds.EnsureCrewBoard(fixture.ctx, fixture.crew.ID, session, fixture.board)
	if err != nil {
		t.Fatalf("freeze crew board for %s: %v", session, err)
	}
	return projected
}

func (fixture *adversarialFixture) selection(offset int) []string {
	return []string{
		fixture.board.Tiles[offset].ID,
		fixture.board.Tiles[offset+1].ID,
		fixture.board.Tiles[offset+2].ID,
		fixture.board.Tiles[offset+3].ID,
	}
}

func (fixture *adversarialFixture) submit(t *testing.T, session, clientID, title string, offset int) VibeSubmission {
	t.Helper()
	card, err := fixture.rounds.SubmitVibe(fixture.ctx, fixture.crew.ID, session, VibeSubmissionRequest{
		BoardID:            fixture.board.ID,
		Title:              title,
		SelectedTileIDs:    fixture.selection(offset),
		ClientSubmissionID: clientID,
	}, fixture.now)
	if err != nil {
		t.Fatalf("submit %s: %v", session, err)
	}
	return card
}

func (fixture *adversarialFixture) memberIDs(t *testing.T) map[string]string {
	t.Helper()
	rows, err := fixture.db.Query(
		`select session_id, member_id from crew_members where crew_id = $1`, fixture.crew.ID)
	if err != nil {
		t.Fatalf("member ids: %v", err)
	}
	defer func() { _ = rows.Close() }()
	ids := map[string]string{}
	for rows.Next() {
		var session, memberID string
		if err := rows.Scan(&session, &memberID); err != nil {
			t.Fatalf("scan member id: %v", err)
		}
		ids[session] = memberID
	}
	return ids
}

// EDGE-001: leaving and rejoining a crew mints a fresh crew_members.member_id,
// but vibe_submissions.submitted_by_member and vibe_votes.voter_member_id are
// keyed on that id. The "one card per member per board" and "one ballot per
// member per board" invariants are therefore enforced against a handle the
// player can rotate at will, from their own browser, with no owner involvement.
func TestLeaveAndRejoinDefeatsOneCardPerMember(t *testing.T) {
	fixture := newAdversarialFixture(t, "2026-08-21")
	fixture.createCrew(t, "Night Shift", "Ari", "session-a")
	fixture.join(t, "Bea", "session-b")
	fixture.freeze(t, "session-a")

	first := fixture.submit(t, "session-a", "card-1", "First identity", 0)

	if err := fixture.crews.LeaveCrew(fixture.ctx, fixture.crew.ID, "session-a", false); err != nil {
		t.Fatalf("leave: %v", err)
	}
	fixture.join(t, "Ari", "session-a")

	second, err := fixture.rounds.SubmitVibe(fixture.ctx, fixture.crew.ID, "session-a", VibeSubmissionRequest{
		BoardID:            fixture.board.ID,
		Title:              "Second identity",
		SelectedTileIDs:    fixture.selection(4),
		ClientSubmissionID: "card-2",
	}, fixture.now)
	if errors.Is(err, ErrVibeAlreadySubmitted) {
		return // invariant held
	}
	if err != nil {
		t.Fatalf("unexpected error on rejoin submit: %v", err)
	}

	var cards int
	if err := fixture.db.QueryRow(
		`select count(*) from vibe_submissions where crew_id = $1 and board_id = $2`,
		fixture.crew.ID, fixture.board.ID).Scan(&cards); err != nil {
		t.Fatalf("count cards: %v", err)
	}
	t.Fatalf("one session authored %d cards on one board (ids %s, %s); "+
		"leave+rejoin rotates member_id and defeats vibe_submissions_one_card",
		cards, first.ID, second.ID)
}

// EDGE-002: identity rotation used to let one browser cast a second ballot and
// back a card it had authored under its previous handle, defeating both the
// one-ballot rule and the no-self-vote rule.
//
// This asserted the exploit while it was open; it now asserts the fix. There is
// deliberately no skip path — an escape hatch here would let the ballot-stuffing
// half of the P1 silently stop being guarded, which is exactly what happened to
// this test once migration 00020 landed.
func TestLeaveAndRejoinCannotStuffBallotsOrSelfVote(t *testing.T) {
	fixture := newAdversarialFixture(t, "2026-08-21")
	fixture.createCrew(t, "Night Shift", "Ari", "session-a")
	fixture.join(t, "Bea", "session-b")
	fixture.join(t, "Cam", "session-c")
	fixture.freeze(t, "session-a")

	ariCard := fixture.submit(t, "session-a", "card-a", "Ari original", 0)
	beaCard := fixture.submit(t, "session-b", "card-b", "Bea card", 4)
	fixture.submit(t, "session-c", "card-c", "Cam card", 8)

	if _, err := fixture.rounds.CastVibeVote(fixture.ctx, fixture.crew.ID, "session-a", VibeVoteRequest{
		BoardID: fixture.board.ID, SubmissionID: ariCard.ID, ClientVoteID: "self-1",
	}, fixture.now); !errors.Is(err, ErrVibeSelfVote) {
		t.Fatalf("baseline self-vote should be rejected, got %v", err)
	}
	if _, err := fixture.rounds.CastVibeVote(fixture.ctx, fixture.crew.ID, "session-a", VibeVoteRequest{
		BoardID: fixture.board.ID, SubmissionID: beaCard.ID, ClientVoteID: "ballot-1",
	}, fixture.now); err != nil {
		t.Fatalf("honest ballot: %v", err)
	}

	if err := fixture.crews.LeaveCrew(fixture.ctx, fixture.crew.ID, "session-a", false); err != nil {
		t.Fatalf("leave: %v", err)
	}
	fixture.join(t, "Ari", "session-a")

	// The rejoined browser must not be able to author a second card...
	if _, err := fixture.rounds.SubmitVibe(fixture.ctx, fixture.crew.ID, "session-a", VibeSubmissionRequest{
		BoardID:            fixture.board.ID,
		Title:              "Ari alias",
		SelectedTileIDs:    fixture.selection(0),
		ClientSubmissionID: "card-a2",
	}, fixture.now); !errors.Is(err, ErrVibeAlreadySubmitted) {
		t.Fatalf("rejoined browser authored a second card (err=%v)", err)
	}
	// ...nor back its own card under the restored handle...
	if _, err := fixture.rounds.CastVibeVote(fixture.ctx, fixture.crew.ID, "session-a", VibeVoteRequest{
		BoardID: fixture.board.ID, SubmissionID: ariCard.ID, ClientVoteID: "self-2",
	}, fixture.now); !errors.Is(err, ErrVibeSelfVote) && !errors.Is(err, ErrVibeAlreadyVoted) {
		t.Fatalf("rejoined browser voted for its own card (err=%v)", err)
	}
	// ...nor cast a second ballot at all.
	if _, err := fixture.rounds.CastVibeVote(fixture.ctx, fixture.crew.ID, "session-a", VibeVoteRequest{
		BoardID: fixture.board.ID, SubmissionID: beaCard.ID, ClientVoteID: "ballot-2",
	}, fixture.now); !errors.Is(err, ErrVibeAlreadyVoted) {
		t.Fatalf("rejoined browser cast a second ballot (err=%v)", err)
	}

	var cards, votes int
	if err := fixture.db.QueryRow(
		`select count(*) from vibe_submissions where crew_id = $1 and board_id = $2`,
		fixture.crew.ID, fixture.board.ID).Scan(&cards); err != nil {
		t.Fatalf("count cards: %v", err)
	}
	if err := fixture.db.QueryRow(
		`select count(*) from vibe_votes where crew_id = $1 and board_id = $2`,
		fixture.crew.ID, fixture.board.ID).Scan(&votes); err != nil {
		t.Fatalf("count votes: %v", err)
	}
	if cards != 3 || votes != 1 {
		t.Fatalf("round holds %d cards and %d ballots, want 3 and 1", cards, votes)
	}
}

// EDGE-003: a result is "official" only with >= 3 cards and >= 2 ballots, and
// only official rounds extend the crew streak. One browser cycling identities
// used to be able to manufacture both. It must now fail at the first extra card,
// so the round stays honestly small and the streak stays at zero.
func TestOneBrowserCannotManufactureAnOfficialResult(t *testing.T) {
	fixture := newAdversarialFixture(t, "2026-08-21")
	fixture.createCrew(t, "Solo", "Keeper", "session-keeper")
	fixture.join(t, "Ari", "session-a")
	fixture.freeze(t, "session-a")

	fixture.submit(t, "session-a", "card-0", "Alias 0", 0)

	// Every subsequent rotation must be refused, not merely discouraged.
	for index := 1; index < 3; index++ {
		if err := fixture.crews.LeaveCrew(fixture.ctx, fixture.crew.ID, "session-a", false); err != nil {
			t.Fatalf("leave %d: %v", index, err)
		}
		fixture.join(t, fmt.Sprintf("Ari-%d", index), "session-a")
		if _, err := fixture.rounds.SubmitVibe(fixture.ctx, fixture.crew.ID, "session-a", VibeSubmissionRequest{
			BoardID:            fixture.board.ID,
			Title:              fmt.Sprintf("Alias %d", index),
			SelectedTileIDs:    fixture.selection(index * 4),
			ClientSubmissionID: fmt.Sprintf("card-%d", index),
		}, fixture.now); !errors.Is(err, ErrVibeAlreadySubmitted) {
			t.Fatalf("rotation %d minted an extra card (err=%v)", index, err)
		}
	}

	snapshot, err := fixture.rounds.CrewSnapshot(fixture.ctx, fixture.crew.ID, []string{fixture.board.ID})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if len(snapshot.Submissions) != 1 {
		t.Fatalf("one browser produced %d cards on one board", len(snapshot.Submissions))
	}

	// The round is nowhere near official, and the streak must reflect that.
	view := buildResultView(fixture.board, "", snapshot.Submissions, snapshot.Votes)
	if view != nil && view.Official {
		t.Fatalf("a round authored by one browser was reported official: %#v", view)
	}
	streak, err := fixture.rounds.CrewStreak(fixture.ctx, fixture.crew.ID, fixture.board.PublishDate)
	if err != nil {
		t.Fatalf("streak: %v", err)
	}
	if streak != 0 {
		t.Fatalf("one browser drove the crew streak to %d", streak)
	}
}

func TestRejoinRestoresStableMemberIdentityAndRoundHistory(t *testing.T) {
	fixture := newAdversarialFixture(t, "2026-08-21")
	fixture.createCrew(t, "Night Shift", "Ari", "session-a")
	fixture.join(t, "Bea", "session-b")
	fixture.freeze(t, "session-a")

	before := fixture.memberIDs(t)["session-a"]
	card := fixture.submit(t, "session-a", "stable-card", "Still mine", 0)
	if err := fixture.crews.LeaveCrew(fixture.ctx, fixture.crew.ID, "session-a", false); err != nil {
		t.Fatalf("leave: %v", err)
	}
	fixture.join(t, "Ari returned", "session-a")
	after := fixture.memberIDs(t)["session-a"]
	if before == "" || before != after || card.MemberID != after {
		t.Fatalf("rejoin changed identity: before=%q after=%q card=%q", before, after, card.MemberID)
	}

	_, err := fixture.rounds.SubmitVibe(fixture.ctx, fixture.crew.ID, "session-a", VibeSubmissionRequest{
		BoardID:            fixture.board.ID,
		Title:              "A second card",
		SelectedTileIDs:    fixture.selection(4),
		ClientSubmissionID: "stable-card-2",
	}, fixture.now)
	if !errors.Is(err, ErrVibeAlreadySubmitted) {
		t.Fatalf("rejoined member should retain the one-card history, got %v", err)
	}

	var identities int
	if err := fixture.db.QueryRow(
		`select count(*) from crew_member_identities where crew_id = $1 and session_id = $2`,
		fixture.crew.ID, "session-a",
	).Scan(&identities); err != nil {
		t.Fatalf("count durable identities: %v", err)
	}
	if identities != 1 {
		t.Fatalf("found %d durable identities for one browser in one crew", identities)
	}
}

func TestDatabaseRejectsImpossibleVoteTargetsAndSelfVotes(t *testing.T) {
	fixture := newAdversarialFixture(t, "2026-08-21")
	fixture.createCrew(t, "Constraint Crew", "Ari", "session-a")
	fixture.join(t, "Bea", "session-b")
	fixture.freeze(t, "session-a")
	ari := fixture.submit(t, "session-a", "db-card-a", "Ari card", 0)
	bea := fixture.submit(t, "session-b", "db-card-b", "Bea card", 4)

	if _, err := fixture.db.Exec(
		`insert into vibe_votes
		 (id, crew_id, board_id, voter_member_id, target_member_id, submission_id, client_vote_id, created_at)
		 values ($1, $2, $3, $4, $4, $5, $6, $7)`,
		"vote_direct_self", fixture.crew.ID, fixture.board.ID, ari.MemberID, ari.ID, "direct-self", fixture.now,
	); err == nil {
		t.Fatal("database accepted a self-vote")
	}

	otherBoard, err := VibeBoardForDate("2026-08-22")
	if err != nil {
		t.Fatalf("other board: %v", err)
	}
	otherBoard, err = fixture.rounds.EnsureBoard(fixture.ctx, otherBoard)
	if err != nil {
		t.Fatalf("ensure other board: %v", err)
	}
	if _, err := fixture.db.Exec(
		`insert into vibe_votes
		 (id, crew_id, board_id, voter_member_id, target_member_id, submission_id, client_vote_id, created_at)
		 values ($1, $2, $3, $4, $5, $6, $7, $8)`,
		"vote_wrong_round", fixture.crew.ID, otherBoard.ID, ari.MemberID, bea.MemberID, bea.ID, "wrong-round", fixture.now,
	); err == nil {
		t.Fatal("database accepted a vote whose target belongs to another board")
	}
}

// EDGE-004: blind judging is the product. Cards used to be served in created_at
// order, and during the make phase every member's "submittedToday" flag flips in
// that same order, visible to every other member. Ballot position was therefore
// a positional map from card to author that any member could read for free.
//
// The contract is now: ballot order is keyed on the crypto-random submission id,
// never on creation time. Asserting "the order came out different from creation
// order" would be a coin flip (with n cards it agrees by chance 1/n! of the
// time), so this asserts the deterministic property instead — the ballot is
// exactly the id-sorted order — and a companion unit test below pins the case
// where creation order and id order disagree.
func TestJudgeBallotIsOrderedByIDNotSubmissionTime(t *testing.T) {
	fixture := newAdversarialFixture(t, "2026-08-21")
	fixture.createCrew(t, "Night Shift", "Ari", "session-a")
	fixture.join(t, "Bea", "session-b")
	fixture.join(t, "Cam", "session-c")
	fixture.freeze(t, "session-a")

	// Submissions land in a known wall-clock order, exactly as three humans
	// playing through the day would produce.
	order := []struct {
		session, title string
		offset         int
	}{
		{"session-b", "Second best week", 0},
		{"session-c", "Technically fine", 4},
		{"session-a", "Under control", 8},
	}
	for index, entry := range order {
		if _, err := fixture.rounds.SubmitVibe(fixture.ctx, fixture.crew.ID, entry.session, VibeSubmissionRequest{
			BoardID:            fixture.board.ID,
			Title:              entry.title,
			SelectedTileIDs:    fixture.selection(entry.offset),
			ClientSubmissionID: fmt.Sprintf("ordered-%d", index),
		}, fixture.now.Add(time.Duration(index)*time.Hour)); err != nil {
			t.Fatalf("submit %s: %v", entry.session, err)
		}
	}

	snapshot, err := fixture.rounds.CrewSnapshot(fixture.ctx, fixture.crew.ID, []string{fixture.board.ID})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	// CrewSnapshot hands submissions back in created_at order, which is what the
	// ballot must NOT reuse.
	for index := 1; index < len(snapshot.Submissions); index++ {
		if snapshot.Submissions[index].CreatedAt.Before(snapshot.Submissions[index-1].CreatedAt) {
			t.Fatalf("fixture assumption broken: snapshot is not in created_at order")
		}
	}

	members := fixture.memberIDs(t)
	view := buildJudgeView(fixture.board, members["session-a"], snapshot.Submissions, nil)
	if view == nil || !view.Eligible {
		t.Fatalf("expected an eligible judge view, got %#v", view)
	}

	wantIDs := make([]string, 0, len(snapshot.Submissions))
	for _, submission := range snapshot.Submissions {
		wantIDs = append(wantIDs, submission.ID)
	}
	sort.Strings(wantIDs)
	gotIDs := make([]string, 0, len(view.Cards))
	for _, card := range view.Cards {
		gotIDs = append(gotIDs, card.ID)
	}
	if !slices.Equal(gotIDs, wantIDs) {
		t.Fatalf("ballot order %v is not the id-sorted order %v; ordering still follows "+
			"another key (created_at is the one that leaks authorship)", gotIDs, wantIDs)
	}
}

// EDGE-004b: the deterministic half. Creation order and id order are made to
// disagree explicitly, so this fails if buildJudgeView ever reverts to serving
// submissions in the order CrewSnapshot returned them.
func TestJudgeBallotIgnoresCreationOrderWhenItDiffersFromIDOrder(t *testing.T) {
	board, err := VibeBoardForDate("2026-08-21")
	if err != nil {
		t.Fatalf("board: %v", err)
	}
	tiles := []string{board.Tiles[0].ID, board.Tiles[1].ID, board.Tiles[2].ID, board.Tiles[3].ID}
	// Creation order is c, b, a — the exact reverse of id order.
	submissions := []VibeSubmission{
		{ID: "card_ccc", BoardID: board.ID, MemberID: "member-c", DisplayName: "Cam", Title: "Third to submit", SelectedTileIDs: tiles},
		{ID: "card_bbb", BoardID: board.ID, MemberID: "member-b", DisplayName: "Bea", Title: "Second to submit", SelectedTileIDs: tiles},
		{ID: "card_aaa", BoardID: board.ID, MemberID: "member-a", DisplayName: "Ari", Title: "First to submit", SelectedTileIDs: tiles},
	}

	view := buildJudgeView(board, "member-a", submissions, nil)
	if view == nil || !view.Eligible {
		t.Fatalf("expected an eligible judge view, got %#v", view)
	}
	got := []string{view.Cards[0].ID, view.Cards[1].ID, view.Cards[2].ID}
	want := []string{"card_aaa", "card_bbb", "card_ccc"}
	if !slices.Equal(got, want) {
		t.Fatalf("ballot came back as %v, want id order %v; serving the caller's "+
			"creation order republishes the authorship side channel", got, want)
	}
	for _, card := range view.Cards {
		if card.AuthorName != "" {
			t.Fatalf("blind ballot revealed author %q", card.AuthorName)
		}
	}
}

// EDGE-005: the crew board size must freeze on first member open and must not be
// rewritten by later joins. Two members opening simultaneously while a third
// joins must converge on exactly one frozen size.
func TestConcurrentFirstOpenAndJoinFreezeOneSize(t *testing.T) {
	fixture := newAdversarialFixture(t, "2026-08-21")
	fixture.createCrew(t, "Night Shift", "Ari", "session-a")
	for index := 0; index < 3; index++ {
		fixture.join(t, fmt.Sprintf("M%d", index), fmt.Sprintf("session-m%d", index))
	}

	var wait sync.WaitGroup
	sizes := make([]int, 4)
	errs := make([]error, 8)
	for index := 0; index < 4; index++ {
		wait.Add(1)
		go func(slot int) {
			defer wait.Done()
			session := "session-a"
			if slot > 0 {
				session = fmt.Sprintf("session-m%d", slot-1)
			}
			projected, err := fixture.rounds.EnsureCrewBoard(context.Background(), fixture.crew.ID, session, fixture.board)
			errs[slot] = err
			sizes[slot] = len(projected.Tiles)
		}(index)
	}
	for index := 0; index < 4; index++ {
		wait.Add(1)
		go func(slot int) {
			defer wait.Done()
			_, err := fixture.crews.JoinCrew(context.Background(), fixture.crew.InviteCode,
				fmt.Sprintf("Late%d", slot), fmt.Sprintf("session-late%d", slot), fixture.now)
			errs[4+slot] = err
		}(index)
	}
	wait.Wait()

	for slot, err := range errs {
		if err != nil {
			t.Fatalf("concurrent operation %d failed: %v", slot, err)
		}
	}
	first := sizes[0]
	for _, size := range sizes {
		if size != first {
			t.Fatalf("concurrent first-open produced different frozen sizes: %v", sizes)
		}
	}

	var frozenRows, tileCount int
	if err := fixture.db.QueryRow(
		`select count(*), coalesce(max(tile_count), 0) from vibe_crew_boards
		 where crew_id = $1 and board_id = $2`,
		fixture.crew.ID, fixture.board.ID).Scan(&frozenRows, &tileCount); err != nil {
		t.Fatalf("frozen rows: %v", err)
	}
	if frozenRows != 1 {
		t.Fatalf("expected exactly one frozen crew board row, got %d", frozenRows)
	}
	if tileCount != first {
		t.Fatalf("returned projection (%d tiles) disagrees with the frozen row (%d)", first, tileCount)
	}
}

// EDGE-006: two identical submissions racing (the classic double-click / retry)
// must produce exactly one card and both callers must see the same winner.
func TestConcurrentIdenticalSubmissionsProduceOneCard(t *testing.T) {
	fixture := newAdversarialFixture(t, "2026-08-21")
	fixture.createCrew(t, "Night Shift", "Ari", "session-a")
	fixture.freeze(t, "session-a")

	const racers = 8
	var wait sync.WaitGroup
	results := make([]VibeSubmission, racers)
	errs := make([]error, racers)
	start := make(chan struct{})
	for index := 0; index < racers; index++ {
		wait.Add(1)
		go func(slot int) {
			defer wait.Done()
			<-start
			card, err := fixture.rounds.SubmitVibe(context.Background(), fixture.crew.ID, "session-a", VibeSubmissionRequest{
				BoardID:            fixture.board.ID,
				Title:              "Double clicked",
				SelectedTileIDs:    fixture.selection(0),
				ClientSubmissionID: "same-replay-key",
			}, fixture.now)
			results[slot], errs[slot] = card, err
		}(index)
	}
	close(start)
	wait.Wait()

	winner := ""
	for slot, err := range errs {
		if err != nil {
			t.Fatalf("racer %d failed a safe replay: %v", slot, err)
		}
		if winner == "" {
			winner = results[slot].ID
		}
		if results[slot].ID != winner {
			t.Fatalf("racers disagreed on the winning card: %s vs %s", winner, results[slot].ID)
		}
	}
	var cards int
	if err := fixture.db.QueryRow(
		`select count(*) from vibe_submissions where crew_id = $1 and board_id = $2`,
		fixture.crew.ID, fixture.board.ID).Scan(&cards); err != nil {
		t.Fatalf("count cards: %v", err)
	}
	if cards != 1 {
		t.Fatalf("a retried submission produced %d cards", cards)
	}
}

// EDGE-007: two DIFFERENT cards racing from one member must leave exactly one
// card, and the loser must be told it lost rather than silently succeeding.
func TestConcurrentDistinctSubmissionsLeaveOneCard(t *testing.T) {
	fixture := newAdversarialFixture(t, "2026-08-21")
	fixture.createCrew(t, "Night Shift", "Ari", "session-a")
	fixture.freeze(t, "session-a")

	const racers = 6
	var wait sync.WaitGroup
	errs := make([]error, racers)
	start := make(chan struct{})
	for index := 0; index < racers; index++ {
		wait.Add(1)
		go func(slot int) {
			defer wait.Done()
			<-start
			_, err := fixture.rounds.SubmitVibe(context.Background(), fixture.crew.ID, "session-a", VibeSubmissionRequest{
				BoardID:            fixture.board.ID,
				Title:              fmt.Sprintf("Attempt %d", slot),
				SelectedTileIDs:    fixture.selection(slot % 3 * 4),
				ClientSubmissionID: fmt.Sprintf("distinct-%d", slot),
			}, fixture.now)
			errs[slot] = err
		}(index)
	}
	close(start)
	wait.Wait()

	succeeded := 0
	for slot, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrVibeAlreadySubmitted), errors.Is(err, ErrVibeReplayConflict):
		default:
			t.Fatalf("racer %d got an unexpected error: %v", slot, err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("%d of %d distinct concurrent submissions were accepted", succeeded, racers)
	}
	var cards int
	if err := fixture.db.QueryRow(
		`select count(*) from vibe_submissions where crew_id = $1 and board_id = $2`,
		fixture.crew.ID, fixture.board.ID).Scan(&cards); err != nil {
		t.Fatalf("count cards: %v", err)
	}
	if cards != 1 {
		t.Fatalf("concurrent distinct submissions stored %d cards", cards)
	}
}

// EDGE-008: concurrent ballots from one member must leave exactly one ballot.
func TestConcurrentBallotsLeaveOneVote(t *testing.T) {
	fixture := newAdversarialFixture(t, "2026-08-21")
	fixture.createCrew(t, "Night Shift", "Ari", "session-a")
	fixture.join(t, "Bea", "session-b")
	fixture.join(t, "Cam", "session-c")
	fixture.freeze(t, "session-a")

	fixture.submit(t, "session-a", "vc-a", "Ari", 0)
	beaCard := fixture.submit(t, "session-b", "vc-b", "Bea", 4)
	camCard := fixture.submit(t, "session-c", "vc-c", "Cam", 8)

	targets := []string{beaCard.ID, camCard.ID}
	const racers = 8
	var wait sync.WaitGroup
	errs := make([]error, racers)
	start := make(chan struct{})
	for index := 0; index < racers; index++ {
		wait.Add(1)
		go func(slot int) {
			defer wait.Done()
			<-start
			_, err := fixture.rounds.CastVibeVote(context.Background(), fixture.crew.ID, "session-a", VibeVoteRequest{
				BoardID:      fixture.board.ID,
				SubmissionID: targets[slot%len(targets)],
				ClientVoteID: fmt.Sprintf("race-vote-%d", slot),
			}, fixture.now)
			errs[slot] = err
		}(index)
	}
	close(start)
	wait.Wait()

	succeeded := 0
	for slot, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrVibeAlreadyVoted), errors.Is(err, ErrVibeReplayConflict):
		default:
			t.Fatalf("ballot racer %d got an unexpected error: %v", slot, err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("%d of %d concurrent ballots were accepted", succeeded, racers)
	}
	var votes int
	if err := fixture.db.QueryRow(
		`select count(*) from vibe_votes where crew_id = $1 and board_id = $2`,
		fixture.crew.ID, fixture.board.ID).Scan(&votes); err != nil {
		t.Fatalf("count votes: %v", err)
	}
	if votes != 1 {
		t.Fatalf("concurrent ballots stored %d votes", votes)
	}
}

// EDGE-009: a member of crew A must not be able to vote for a card that lives
// in crew B, even when they hold both invite codes and a valid card in each.
func TestCrossCrewVoteIsRejected(t *testing.T) {
	fixture := newAdversarialFixture(t, "2026-08-21")
	alpha := fixture.createCrew(t, "Alpha", "Ari", "session-a")
	fixture.join(t, "Bea", "session-b")
	fixture.freeze(t, "session-a")
	fixture.submit(t, "session-a", "x-a", "Alpha card", 0)
	beaCard := fixture.submit(t, "session-b", "x-b", "Bea card", 4)

	beta, err := fixture.crews.CreateCrew(fixture.ctx, "Beta", "Ari", "session-a", fixture.now)
	if err != nil {
		t.Fatalf("create beta: %v", err)
	}
	fixture.crew = beta
	fixture.freeze(t, "session-a")
	fixture.submit(t, "session-a", "x-c", "Beta card", 8)

	_, err = fixture.rounds.CastVibeVote(fixture.ctx, beta.ID, "session-a", VibeVoteRequest{
		BoardID: fixture.board.ID, SubmissionID: beaCard.ID, ClientVoteID: "cross-crew",
	}, fixture.now)
	if !errors.Is(err, ErrVibeSubmissionNotFound) {
		t.Fatalf("cross-crew vote from crew %s to card in crew %s returned %v, want not-found",
			beta.ID, alpha.ID, err)
	}
	var leaked int
	if err := fixture.db.QueryRow(
		`select count(*) from vibe_votes v join vibe_submissions s on s.id = v.submission_id
		 where v.crew_id <> s.crew_id`).Scan(&leaked); err != nil {
		t.Fatalf("cross-crew vote scan: %v", err)
	}
	if leaked != 0 {
		t.Fatalf("%d votes point at a card in a different crew", leaked)
	}
}

// EDGE-010: a card must only ever contain fragments from that crew's FROZEN
// projection. A member of a small crew (12 fragments) must not be able to hand
// the API a fragment id from row 4+ of the 28-fragment master palette, which is
// public via /api/vibes/practice and trivially derivable from the board id.
func TestSubmissionCannotUseFragmentsOutsideTheFrozenProjection(t *testing.T) {
	fixture := newAdversarialFixture(t, "2026-08-21")
	fixture.createCrew(t, "Small", "Ari", "session-a")
	projected := fixture.freeze(t, "session-a")
	if len(projected.Tiles) != VibeBoardMinTileCount {
		t.Fatalf("expected a %d-fragment freeze for a solo crew, got %d",
			VibeBoardMinTileCount, len(projected.Tiles))
	}

	beyond := []string{
		fixture.board.Tiles[0].ID,
		fixture.board.Tiles[1].ID,
		fixture.board.Tiles[2].ID,
		fixture.board.Tiles[VibeBoardMaxTileCount-1].ID, // row 7, not dealt to this crew
	}
	_, err := fixture.rounds.SubmitVibe(fixture.ctx, fixture.crew.ID, "session-a", VibeSubmissionRequest{
		BoardID:            fixture.board.ID,
		Title:              "Reaching past the freeze",
		SelectedTileIDs:    beyond,
		ClientSubmissionID: "beyond-1",
	}, fixture.now)
	if !errors.Is(err, ErrVibeRequestInvalid) {
		t.Fatalf("submission using an undealt fragment returned %v, want ErrVibeRequestInvalid", err)
	}
}

// EDGE-011: removing a member must not orphan or rewrite that member's card,
// and must not let the owner delete crew history through the member handle.
func TestRemovedMemberKeepsCardAndCannotBeResubmitted(t *testing.T) {
	fixture := newAdversarialFixture(t, "2026-08-21")
	fixture.createCrew(t, "Night Shift", "Ari", "session-a")
	fixture.join(t, "Bea", "session-b")
	fixture.freeze(t, "session-a")
	beaCard := fixture.submit(t, "session-b", "rm-b", "Bea card", 4)

	members := fixture.memberIDs(t)
	if err := fixture.crews.RemoveMember(fixture.ctx, fixture.crew.ID, members["session-b"], "session-a"); err != nil {
		t.Fatalf("remove member: %v", err)
	}

	var title, displayName string
	if err := fixture.db.QueryRow(
		`select title, display_name from vibe_submissions where id = $1`, beaCard.ID,
	).Scan(&title, &displayName); err != nil {
		t.Fatalf("removed member's card vanished: %v", err)
	}
	if displayName != "Bea" {
		t.Fatalf("author snapshot was rewritten to %q", displayName)
	}

	if _, err := fixture.rounds.SubmitVibe(fixture.ctx, fixture.crew.ID, "session-b", VibeSubmissionRequest{
		BoardID:            fixture.board.ID,
		Title:              "Back anyway",
		SelectedTileIDs:    fixture.selection(8),
		ClientSubmissionID: "rm-b2",
	}, fixture.now); !errors.Is(err, ErrNotCrewMember) {
		t.Fatalf("a removed member submitted anyway: %v", err)
	}
}

// EDGE-012: after an explicit destructive acknowledgement, the last member
// leaving deletes the crew and cascades every card and ballot in that crew's
// history. Confirm the blast radius is real and bounded to that crew, and that
// the shared dated board survives.
func TestLastMemberLeavingDestroysCrewHistory(t *testing.T) {
	fixture := newAdversarialFixture(t, "2026-08-21")
	fixture.createCrew(t, "Doomed", "Ari", "session-a")
	fixture.join(t, "Bea", "session-b")
	fixture.freeze(t, "session-a")
	fixture.submit(t, "session-a", "d-a", "Ari card", 0)
	fixture.submit(t, "session-b", "d-b", "Bea card", 4)

	for index, session := range []string{"session-b", "session-a"} {
		confirmDelete := index == 1
		if err := fixture.crews.LeaveCrew(fixture.ctx, fixture.crew.ID, session, confirmDelete); err != nil {
			t.Fatalf("leave %s: %v", session, err)
		}
	}

	var cards, votes, boards int
	if err := fixture.db.QueryRow(`select count(*) from vibe_submissions`).Scan(&cards); err != nil {
		t.Fatalf("count cards: %v", err)
	}
	if err := fixture.db.QueryRow(`select count(*) from vibe_votes`).Scan(&votes); err != nil {
		t.Fatalf("count votes: %v", err)
	}
	if err := fixture.db.QueryRow(`select count(*) from vibe_daily_boards`).Scan(&boards); err != nil {
		t.Fatalf("count boards: %v", err)
	}
	if boards != 1 {
		t.Fatalf("the shared editorial board was destroyed with the crew (%d remain)", boards)
	}
	if cards != 0 || votes != 0 {
		t.Fatalf("expected cascade to clear crew history, got %d cards / %d votes", cards, votes)
	}
}

// EDGE-013: the identity fix must hold for the OWNER-DRIVEN variant too. My
// original probes only covered a member leaving voluntarily; an owner removing
// someone deletes the same crew_members row, so removal + rejoin is the same
// attack with a different actor.
func TestRemoveAndRejoinCannotMintASecondCard(t *testing.T) {
	fixture := newAdversarialFixture(t, "2026-08-21")
	fixture.createCrew(t, "Night Shift", "Ari", "session-a")
	fixture.join(t, "Bea", "session-b")
	fixture.freeze(t, "session-a")

	first := fixture.submit(t, "session-b", "rr-1", "Original card", 0)
	beforeIDs := fixture.memberIDs(t)

	if err := fixture.crews.RemoveMember(fixture.ctx, fixture.crew.ID, beforeIDs["session-b"], "session-a"); err != nil {
		t.Fatalf("remove member: %v", err)
	}
	fixture.join(t, "Bea", "session-b")

	afterIDs := fixture.memberIDs(t)
	if afterIDs["session-b"] != beforeIDs["session-b"] {
		t.Fatalf("removal + rejoin rotated the durable member handle: %s -> %s",
			beforeIDs["session-b"], afterIDs["session-b"])
	}

	if _, err := fixture.rounds.SubmitVibe(fixture.ctx, fixture.crew.ID, "session-b", VibeSubmissionRequest{
		BoardID:            fixture.board.ID,
		Title:              "Second card after readmission",
		SelectedTileIDs:    fixture.selection(4),
		ClientSubmissionID: "rr-2",
	}, fixture.now); !errors.Is(err, ErrVibeAlreadySubmitted) {
		t.Fatalf("a readmitted member authored a second card (err=%v)", err)
	}

	var cards int
	if err := fixture.db.QueryRow(
		`select count(*) from vibe_submissions where crew_id = $1 and board_id = $2`,
		fixture.crew.ID, fixture.board.ID).Scan(&cards); err != nil {
		t.Fatalf("count cards: %v", err)
	}
	if cards != 1 {
		t.Fatalf("removal + rejoin produced %d cards, want 1 (original %s)", cards, first.ID)
	}
}

// EDGE-014: the round rules must now be enforced by the SCHEMA, not only by the
// Go transaction. These write straight to Postgres, bypassing the application
// entirely — which is the point: a future code path, a backfill, or a manual
// repair must not be able to create an impossible round.
func TestSchemaRejectsImpossibleVotesWithoutTheApplication(t *testing.T) {
	fixture := newAdversarialFixture(t, "2026-08-21")
	fixture.createCrew(t, "Night Shift", "Ari", "session-a")
	fixture.join(t, "Bea", "session-b")
	fixture.freeze(t, "session-a")
	ari := fixture.submit(t, "session-a", "sc-a", "Ari card", 0)
	bea := fixture.submit(t, "session-b", "sc-b", "Bea card", 4)
	members := fixture.memberIDs(t)

	// A self-vote, written directly.
	_, err := fixture.db.Exec(
		`insert into vibe_votes (id, crew_id, board_id, voter_member_id, submission_id, target_member_id, client_vote_id)
		 values ($1, $2, $3, $4, $5, $6, $7)`,
		"vote_selfraw", fixture.crew.ID, fixture.board.ID, members["session-a"], ari.ID, members["session-a"], "raw-self")
	if err == nil {
		t.Fatal("the schema accepted a self-vote written outside the application")
	}

	// A vote whose target member does not match the submission's real author.
	_, err = fixture.db.Exec(
		`insert into vibe_votes (id, crew_id, board_id, voter_member_id, submission_id, target_member_id, client_vote_id)
		 values ($1, $2, $3, $4, $5, $6, $7)`,
		"vote_mismatch", fixture.crew.ID, fixture.board.ID, members["session-a"], bea.ID, members["session-a"], "raw-mismatch")
	if err == nil {
		t.Fatal("the schema accepted a vote whose target member is not the card's author")
	}

	// A vote pointing at a card on a different board.
	otherBoard, boardErr := VibeBoardForDate("2026-08-20")
	if boardErr != nil {
		t.Fatalf("other board: %v", boardErr)
	}
	if _, boardErr = fixture.rounds.EnsureBoard(fixture.ctx, otherBoard); boardErr != nil {
		t.Fatalf("ensure other board: %v", boardErr)
	}
	_, err = fixture.db.Exec(
		`insert into vibe_votes (id, crew_id, board_id, voter_member_id, submission_id, target_member_id, client_vote_id)
		 values ($1, $2, $3, $4, $5, $6, $7)`,
		"vote_crossboard", fixture.crew.ID, otherBoard.ID, members["session-a"], bea.ID, members["session-b"], "raw-cross")
	if err == nil {
		t.Fatal("the schema accepted a vote pointing at a card on a different board")
	}

	var votes int
	if err := fixture.db.QueryRow(`select count(*) from vibe_votes`).Scan(&votes); err != nil {
		t.Fatalf("count votes: %v", err)
	}
	if votes != 0 {
		t.Fatalf("%d impossible votes were persisted", votes)
	}
}

// EDGE-015: crew_members.member_id must no longer be self-minting. Inserting an
// active membership with an arbitrary handle has to fail, otherwise the durable
// mapping is advisory rather than enforced.
func TestActiveMembershipCannotInventItsOwnHandle(t *testing.T) {
	fixture := newAdversarialFixture(t, "2026-08-21")
	fixture.createCrew(t, "Night Shift", "Ari", "session-a")

	_, err := fixture.db.Exec(
		`insert into crew_members (crew_id, session_id, display_name, member_id)
		 values ($1, $2, $3, gen_random_uuid())`,
		fixture.crew.ID, "session-forged", "Forged")
	if err == nil {
		t.Fatal("an active membership row invented its own member_id with no durable mapping")
	}
}
