package vibegrid

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestConcurrentFirstBoardReadsConvergeOnOneMaster(t *testing.T) {
	_, database := hostileServer(t)
	store := NewPostgresVibeRoundStore(database)
	const readers = 16
	// Both id and publish_date are unique. First readers must converge even
	// when PostgreSQL detects the primary-key conflict before the date conflict.
	for day := 0; day < 8; day++ {
		date := time.Date(2026, 9, 1+day, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		board, err := VibeBoardForDate(date)
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		type outcome struct {
			board VibeBoard
			err   error
		}
		results := make(chan outcome, readers)
		for index := 0; index < readers; index++ {
			go func(index int) {
				<-start
				candidate := board
				candidate.Prompt = fmt.Sprintf("Build candidate %d.", index)
				candidate.Tiles = append([]Tile(nil), board.Tiles...)
				candidate.Tiles[27].Text = fmt.Sprintf("candidate %d", index)
				stored, err := store.EnsureBoard(context.Background(), candidate)
				results <- outcome{stored, err}
			}(index)
		}
		close(start)
		var winner VibeBoard
		for index := 0; index < readers; index++ {
			result := <-results
			if result.err != nil {
				t.Fatalf("first read for %s: %v", date, result.err)
			}
			if index == 0 {
				winner = result.board
			}
			if !reflect.DeepEqual(result.board, winner) {
				t.Fatalf("readers got different masters for %s", date)
			}
		}
		var candidate int
		if _, err := fmt.Sscanf(winner.Prompt, "Build candidate %d.", &candidate); err != nil {
			t.Fatal(err)
		}
		if winner.Tiles[27].Text != fmt.Sprintf("candidate %d", candidate) {
			t.Fatal("expansion came from another writer")
		}
		stored, err := store.EnsureBoard(context.Background(), board)
		if err != nil || !reflect.DeepEqual(stored, winner) {
			t.Fatalf("later read rewrote immutable master: %v", err)
		}
	}
}

// seedFrozenCrewBoard constructs a preexisting snapshot for store/migration
// fixtures. Production has no standalone freeze API: SubmitVibe owns that write.
func (store *PostgresVibeRoundStore) seedFrozenCrewBoard(ctx context.Context, crewID, sessionID string, board VibeBoard) (VibeBoard, error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return VibeBoard{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var id string
	if err := tx.QueryRowContext(ctx, `select id from crews where id = $1 for update`, crewID).Scan(&id); err != nil {
		return VibeBoard{}, err
	}
	if err := tx.QueryRowContext(ctx, `select member_id from crew_members where crew_id = $1 and session_id = $2`, crewID, sessionID).Scan(&id); err != nil {
		return VibeBoard{}, err
	}
	projected, err := freezeCrewBoard(ctx, tx, crewID, board.ID)
	if err != nil {
		return VibeBoard{}, err
	}
	if err := tx.Commit(); err != nil {
		return VibeBoard{}, err
	}
	return projected, nil
}

func TestIdempotencyRejectsChangedCrewTarget(t *testing.T) {
	handler, database := hostileServer(t)
	owner := newCrewSession(t, handler)
	firstCrew := createCrew(t, handler, owner, "First Room", "Ari")
	secondCrew := createCrew(t, handler, owner, "Second Room", "Ari")
	board := todayVibeBoard(t, handler)
	body, _ := json.Marshal(VibeSubmissionRequest{BoardID: board.ID, Title: "Same card", SelectedTileIDs: []string{board.Tiles[0].ID, board.Tiles[1].ID, board.Tiles[2].ID, board.Tiles[3].ID}, ClientSubmissionID: "same-intent"})
	send := func(invite string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/crews/"+invite+"/submissions", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "cross-crew-key")
		request.Header.Set(guestCSRFHeader, guestCSRFToken(owner.Value))
		request.AddCookie(owner)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	first := send(firstCrew.InviteCode)
	if first.Code != http.StatusCreated {
		t.Fatalf("first: %d %s", first.Code, first.Body.String())
	}
	second := send(secondCrew.InviteCode)
	if second.Code != http.StatusConflict {
		t.Fatalf("changed crew: %d %s", second.Code, second.Body.String())
	}
	replay := send(firstCrew.InviteCode)
	if replay.Code != http.StatusCreated || replay.Body.String() != first.Body.String() || replay.Header().Get(idempotencyReplayHeader) != "true" {
		t.Fatalf("original replay: %d %s", replay.Code, replay.Body.String())
	}
	var count int
	if err := database.QueryRow(`select count(*) from vibe_submissions where crew_id = $1`, crewInternalID(t, database, secondCrew.InviteCode)).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("changed target wrote %d cards", count)
	}
}

func TestFirstAcceptedCardCommitsSizeAndCardTogether(t *testing.T) {
	handler, database := hostileServer(t, func(config *ServerConfig, _ *sql.DB) { config.BlockedTerms = []string{"blocked-card"} })
	owner := newCrewSession(t, handler)
	crew := createCrew(t, handler, owner, "Growing Room", "Ari")
	board := todayVibeBoard(t, handler)
	crewID := crewInternalID(t, database, crew.InviteCode)
	store := NewPostgresVibeRoundStore(database)
	path := "/api/crews/" + crew.InviteCode + "/submissions"
	base := VibeSubmissionRequest{BoardID: board.ID, Title: "Valid title", SelectedTileIDs: []string{board.Tiles[0].ID, board.Tiles[1].ID, board.Tiles[2].ID, board.Tiles[3].ID}, ClientSubmissionID: "first-valid-card"}
	assertUnstarted := func(t *testing.T) {
		t.Helper()
		frozen, err := store.FrozenCrewBoards(context.Background(), crewID, []string{board.ID})
		if err != nil {
			t.Fatal(err)
		}
		var cards int
		if err := database.QueryRow(`select count(*) from vibe_submissions where crew_id = $1`, crewID).Scan(&cards); err != nil {
			t.Fatal(err)
		}
		if len(frozen) != 0 || cards != 0 {
			t.Fatalf("rejected action started round: sizes=%v cards=%d", frozen, cards)
		}
	}
	cases := []struct {
		name   string
		change func(*VibeSubmissionRequest)
	}{
		{"empty title", func(r *VibeSubmissionRequest) { r.Title = "" }},
		{"blocked title", func(r *VibeSubmissionRequest) { r.Title = "blocked-card" }},
		{"duplicate fragments", func(r *VibeSubmissionRequest) {
			r.SelectedTileIDs = []string{board.Tiles[0].ID, board.Tiles[0].ID, board.Tiles[1].ID, board.Tiles[2].ID}
		}},
		{"outside crew projection", func(r *VibeSubmissionRequest) {
			r.SelectedTileIDs = []string{board.Tiles[0].ID, board.Tiles[1].ID, board.Tiles[2].ID, board.Tiles[15].ID}
		}},
		{"invalid replay id", func(r *VibeSubmissionRequest) { r.ClientSubmissionID = "bad key" }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			request := base
			test.change(&request)
			response := crewRequest(t, handler, http.MethodPost, path, owner, request)
			if response.Code != http.StatusUnprocessableEntity {
				t.Fatalf("rejection: %d %s", response.Code, response.Body.String())
			}
			assertUnstarted(t)
		})
	}
	// Force a database error after sizing, independently of handler validation.
	if _, err := database.Exec(`alter table vibe_submissions add constraint test_reject_card check (title <> 'Valid title')`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.Exec(`alter table vibe_submissions drop constraint if exists test_reject_card`)
	})
	failed := crewRequest(t, handler, http.MethodPost, path, owner, base)
	if failed.Code != http.StatusInternalServerError {
		t.Fatalf("insert failure: %d %s", failed.Code, failed.Body.String())
	}
	assertUnstarted(t)
	if _, err := database.Exec(`alter table vibe_submissions drop constraint test_reject_card`); err != nil {
		t.Fatal(err)
	}

	for index := 0; index < 4; index++ {
		member := newCrewSession(t, handler)
		joined := crewRequest(t, handler, http.MethodPost, "/api/crews/"+crew.InviteCode+"/join", member, map[string]string{"displayName": fmt.Sprintf("Friend %d", index)})
		if joined.Code != http.StatusOK {
			t.Fatalf("join: %d %s", joined.Code, joined.Body.String())
		}
	}
	base.SelectedTileIDs = []string{board.Tiles[0].ID, board.Tiles[1].ID, board.Tiles[2].ID, board.Tiles[15].ID}
	accepted := crewRequest(t, handler, http.MethodPost, path, owner, base)
	if accepted.Code != http.StatusCreated {
		t.Fatalf("first valid card: %d %s", accepted.Code, accepted.Body.String())
	}
	frozen, err := store.FrozenCrewBoards(context.Background(), crewID, []string{board.ID})
	if err != nil || frozen[board.ID] != 16 {
		t.Fatalf("first valid card did not size for five members: %v %v", frozen, err)
	}
}
