package vibegrid

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/pressly/goose/v3"
	"github.com/vibegrid/vibegrid/backend/db"
)

// migrationTestDatabase is deliberately a DIFFERENT database from
// TEST_DATABASE_URL: these tests roll the schema backwards, which would wedge
// every other Postgres test if they shared one.
func migrationTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	databaseURL := os.Getenv("TEST_MIGRATION_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_MIGRATION_DATABASE_URL not set; skipping schema upgrade tests")
	}
	database, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Fatalf("open migration database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Ping(); err != nil {
		t.Fatalf("ping migration database: %v", err)
	}

	goose.SetBaseFS(db.Migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("goose dialect: %v", err)
	}
	goose.SetLogger(goose.NopLogger())
	if err := goose.DownTo(database, "migrations", 0); err != nil {
		t.Fatalf("reset migration database: %v", err)
	}
	return database
}

// EDGE-070: migration 00020 closes the identity-rotation P1 for NEW rows, but a
// real deployment upgrades a database that already holds crews, cards and
// ballots. Its backfill has to carry every existing member handle across
// unchanged — otherwise the upgrade silently reassigns authorship of every card
// in flight, and pre-existing members keep the exploit.
func TestUpgradeFromPreIdentitySchemaPreservesHistoryAndClosesTheExploit(t *testing.T) {
	database := migrationTestDatabase(t)
	ctx := context.Background()

	// Stand the schema up as it shipped before the identity fix.
	if err := goose.UpTo(database, "migrations", 19); err != nil {
		t.Fatalf("migrate to pre-identity schema: %v", err)
	}

	board, err := VibeBoardForDate("2026-08-21")
	if err != nil {
		t.Fatalf("board: %v", err)
	}
	// The v19-era rows are written with raw SQL on purpose: the current Go stores
	// already expect the post-fix schema, so using them here would test nothing
	// about the upgrade path.
	baseTiles, _, err := marshalStoredVibeBoard(board)
	if err != nil {
		t.Fatalf("marshal board: %v", err)
	}
	if _, err := database.Exec(
		`insert into vibe_daily_boards (id, publish_date, board_number, prompt, tiles)
		 values ($1, $2, $3, $4, $5)`,
		board.ID, board.PublishDate, board.BoardNumber, board.Prompt, string(baseTiles)); err != nil {
		t.Fatalf("legacy board: %v", err)
	}
	if _, err := database.Exec(
		`insert into crews (id, invite_code, name, created_by_session)
		 values ('legacy-crew', 'legacy-invite', 'Legacy Crew', 'legacy-session-a')`); err != nil {
		t.Fatalf("legacy crew: %v", err)
	}
	crewID := "legacy-crew"
	legacyHandles := map[string]string{}
	for _, member := range []struct{ name, session string }{
		{"Ari", "legacy-session-a"}, {"Bea", "legacy-session-b"}, {"Cam", "legacy-session-c"},
	} {
		// No explicit member_id: the pre-fix schema minted it from the column
		// default, which is exactly the behaviour the fix had to replace.
		var handle string
		if err := database.QueryRow(
			`insert into crew_members (crew_id, session_id, display_name)
			 values ($1, $2, $3) returning member_id`,
			crewID, member.session, member.name).Scan(&handle); err != nil {
			t.Fatalf("legacy join %s: %v", member.name, err)
		}
		legacyHandles[member.session] = handle
	}
	if _, err := database.Exec(
		`insert into vibe_crew_boards (crew_id, board_id, member_count_snapshot, tile_count)
		 values ($1, $2, 3, 12)`, crewID, board.ID); err != nil {
		t.Fatalf("legacy freeze: %v", err)
	}

	selection := []string{board.Tiles[0].ID, board.Tiles[1].ID, board.Tiles[2].ID, board.Tiles[3].ID}
	cards := map[string]string{}
	for _, session := range []string{"legacy-session-a", "legacy-session-b", "legacy-session-c"} {
		cardID := "card_legacy_" + session
		if _, err := database.Exec(
			`insert into vibe_submissions (id, crew_id, board_id, submitted_by_member, display_name,
			 title, selected_tile_ids, client_submission_id)
			 values ($1, $2, $3, $4, $5, $6, $7, $8)`,
			cardID, crewID, board.ID, legacyHandles[session], session, "Legacy "+session,
			pq.Array(selection), "legacy-"+session); err != nil {
			t.Fatalf("legacy submit %s: %v", session, err)
		}
		cards[session] = cardID
	}

	// One honest ballot, written the way the pre-fix schema stored them: no
	// target_member_id column exists yet.
	if _, err := database.Exec(
		`insert into vibe_votes (id, crew_id, board_id, voter_member_id, submission_id, client_vote_id)
		 values ($1, $2, $3, $4, $5, $6)`,
		"vote_legacy_ok", crewID, board.ID, legacyHandles["legacy-session-a"],
		cards["legacy-session-b"], "legacy-ballot-ok"); err != nil {
		t.Fatalf("legacy honest ballot: %v", err)
	}
	// And one impossible row of the kind the old schema permitted — a self-vote,
	// exactly what the identity-rotation exploit produced in the wild.
	if _, err := database.Exec(
		`insert into vibe_votes (id, crew_id, board_id, voter_member_id, submission_id, client_vote_id)
		 values ($1, $2, $3, $4, $5, $6)`,
		"vote_legacy_self", crewID, board.ID, legacyHandles["legacy-session-c"],
		cards["legacy-session-c"], "legacy-ballot-self"); err != nil {
		t.Fatalf("legacy self ballot: %v", err)
	}

	// The upgrade under test.
	if err := goose.Up(database, "migrations"); err != nil {
		t.Fatalf("upgrade to head failed on a populated database: %v", err)
	}
	// Only now is the current code's schema contract valid.
	rounds := NewPostgresVibeRoundStore(database)
	crews := NewPostgresCrewStore(database)
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)

	// 1. Every existing member handle survives, so no card changes author.
	for session, want := range legacyHandles {
		var mapped, active string
		if err := database.QueryRow(
			`select member_id from crew_member_identities where crew_id = $1 and session_id = $2`,
			crewID, session).Scan(&mapped); err != nil {
			t.Fatalf("no durable identity for %s after upgrade: %v", session, err)
		}
		if err := database.QueryRow(
			`select member_id from crew_members where crew_id = $1 and session_id = $2`,
			crewID, session).Scan(&active); err != nil {
			t.Fatalf("membership for %s vanished in the upgrade: %v", session, err)
		}
		if mapped != want || active != want {
			t.Fatalf("upgrade reassigned %s: was %s, mapping %s, active %s", session, want, mapped, active)
		}
	}

	// 2. Cards survive with their original authorship.
	for session, cardID := range cards {
		var author string
		if err := database.QueryRow(
			`select submitted_by_member from vibe_submissions where id = $1`, cardID).Scan(&author); err != nil {
			t.Fatalf("card %s lost in the upgrade: %v", cardID, err)
		}
		if author != legacyHandles[session] {
			t.Fatalf("card %s changed author from %s to %s", cardID, legacyHandles[session], author)
		}
	}

	// 3. The honest ballot survives and gained a correct target; the impossible
	//    one is repaired away rather than blocking the migration.
	var target string
	if err := database.QueryRow(
		`select target_member_id from vibe_votes where id = 'vote_legacy_ok'`).Scan(&target); err != nil {
		t.Fatalf("honest legacy ballot did not survive the upgrade: %v", err)
	}
	if target != legacyHandles["legacy-session-b"] {
		t.Fatalf("backfilled vote target is %s, want the card's author %s",
			target, legacyHandles["legacy-session-b"])
	}
	var selfVotes int
	if err := database.QueryRow(
		`select count(*) from vibe_votes where id = 'vote_legacy_self'`).Scan(&selfVotes); err != nil {
		t.Fatalf("count repaired ballots: %v", err)
	}
	if selfVotes != 0 {
		t.Fatal("the upgrade kept a self-vote that its own CHECK constraint forbids")
	}

	// 4. The exploit is closed for a member who predates the fix — the case that
	//    a naive "only new joins get stable ids" fix would have missed.
	if err := crews.LeaveCrew(ctx, crewID, "legacy-session-b", false); err != nil {
		t.Fatalf("legacy member leave: %v", err)
	}
	if _, err := crews.JoinCrew(ctx, "legacy-invite", "Bea", "legacy-session-b", now); err != nil {
		t.Fatalf("legacy member rejoin: %v", err)
	}
	var rejoined string
	if err := database.QueryRow(
		`select member_id from crew_members where crew_id = $1 and session_id = $2`,
		crewID, "legacy-session-b").Scan(&rejoined); err != nil {
		t.Fatalf("rejoined legacy member: %v", err)
	}
	if rejoined != legacyHandles["legacy-session-b"] {
		t.Fatalf("a pre-upgrade member rotated their handle on rejoin: %s -> %s",
			legacyHandles["legacy-session-b"], rejoined)
	}
	if _, err := rounds.SubmitVibe(ctx, crewID, "legacy-session-b", VibeSubmissionRequest{
		BoardID: board.ID, Title: "Second card", SelectedTileIDs: selection,
		ClientSubmissionID: "legacy-second",
	}, now); !errors.Is(err, ErrVibeAlreadySubmitted) {
		t.Fatalf("a pre-upgrade member authored a second card after rejoining (err=%v)", err)
	}
}

// EDGE-071: the additive-migration promise is that a release can be rolled back
// and that the schema can be rebuilt from nothing. Exercise the whole chain in
// both directions — every Down, then every Up again — because a Down step that
// only ever runs during a bad night is exactly the one nobody has run.
func TestMigrationsRollBackAndForwardCleanly(t *testing.T) {
	database := migrationTestDatabase(t)

	if err := goose.Up(database, "migrations"); err != nil {
		t.Fatalf("initial migrate to head: %v", err)
	}
	head, err := goose.GetDBVersion(database)
	if err != nil {
		t.Fatalf("read head version: %v", err)
	}

	// Step 1: the realistic rollback — back to the last schema the pre-identity
	// binary ran on. This is pinned to a named version rather than "head minus
	// N": a relative offset silently stops short of the identity release as soon
	// as a later migration (00022 push subscriptions) lands on top of it.
	const preIdentitySchema = 19
	if err := goose.DownTo(database, "migrations", preIdentitySchema); err != nil {
		t.Fatalf("rollback to the pre-identity schema failed: %v", err)
	}
	var newerTables int
	if err := database.QueryRow(
		`select count(*) from information_schema.tables
		 where table_name in ('crew_member_identities', 'guest_sessions', 'push_subscriptions')`).Scan(&newerTables); err != nil {
		t.Fatalf("inspect rolled-back schema: %v", err)
	}
	if newerTables != 0 {
		t.Fatalf("rollback left %d of the newer tables behind", newerTables)
	}

	// The previous release must still be able to write a membership, which is
	// what "the old binary stays bootable" actually means.
	if _, err := database.Exec(
		`insert into crews (id, invite_code, name, created_by_session) values ('c1','i1','Rollback Crew','s1')`); err != nil {
		t.Fatalf("rolled-back schema rejected a crew insert: %v", err)
	}
	if _, err := database.Exec(
		`insert into crew_members (crew_id, session_id, display_name) values ('c1','s1','Ari')`); err != nil {
		t.Fatalf("rolled-back schema could not mint a membership handle: %v", err)
	}

	// Step 2: roll forward again over that data.
	if err := goose.Up(database, "migrations"); err != nil {
		t.Fatalf("roll forward after rollback failed: %v", err)
	}
	var restored string
	if err := database.QueryRow(
		`select member_id from crew_member_identities where crew_id = 'c1' and session_id = 's1'`).Scan(&restored); err != nil {
		t.Fatalf("roll-forward did not adopt the row written by the old release: %v", err)
	}
	if restored == "" {
		t.Fatal("adopted identity is empty")
	}

	// Step 3: the full teardown. Every Down step has to run, or the schema can
	// never be rebuilt from scratch and a disaster-recovery path is a dead end.
	// This is the step that caught 00002 dropping a constraint that migration
	// 00010's Up had already removed.
	if err := goose.DownTo(database, "migrations", 0); err != nil {
		t.Fatalf("full teardown failed, so this schema cannot be rebuilt: %v", err)
	}
	var remaining int
	if err := database.QueryRow(
		`select count(*) from information_schema.tables
		 where table_schema = 'public' and table_name not like 'goose%'`).Scan(&remaining); err != nil {
		t.Fatalf("inspect torn-down schema: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("full teardown left %d tables behind", remaining)
	}

	// Step 4: and a clean rebuild from zero has to work.
	if err := goose.Up(database, "migrations"); err != nil {
		t.Fatalf("rebuild from an empty schema failed: %v", err)
	}
	rebuilt, err := goose.GetDBVersion(database)
	if err != nil {
		t.Fatalf("read rebuilt version: %v", err)
	}
	if rebuilt != head {
		t.Fatalf("rebuild landed on version %d, want %d", rebuilt, head)
	}
}
