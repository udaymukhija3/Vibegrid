package vibegrid

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// crewRequest performs a crew API call as a specific browser session. Passing a
// nil cookie models a brand-new browser; the returned recorder carries whatever
// session cookie the server assigned.
func crewRequest(t *testing.T, handler http.Handler, method, path string, cookie *http.Cookie, body any) *httptest.ResponseRecorder {
	t.Helper()

	var reader *bytes.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(payload)
	} else {
		reader = bytes.NewReader(nil)
	}

	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		request.AddCookie(cookie)
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

// newCrewSession returns a fresh guest session cookie.
func newCrewSession(t *testing.T, handler http.Handler) *http.Cookie {
	t.Helper()
	response := crewRequest(t, handler, http.MethodGet, "/api/session", nil, nil)
	cookie := responseCookie(response, sessionCookieName)
	if cookie == nil {
		t.Fatal("session endpoint did not set a guest cookie")
	}
	return cookie
}

func decodeCrewBoard(t *testing.T, response *httptest.ResponseRecorder) crewBoardResponse {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("crew board failed: %d %s", response.Code, response.Body.String())
	}
	var board crewBoardResponse
	if err := json.NewDecoder(response.Body).Decode(&board); err != nil {
		t.Fatal(err)
	}
	return board
}

func createCrew(t *testing.T, handler http.Handler, cookie *http.Cookie, name, displayName string) crewView {
	t.Helper()
	response := crewRequest(t, handler, http.MethodPost, "/api/crews", cookie, crewCreateRequest{
		Name:        name,
		DisplayName: displayName,
	})
	if response.Code != http.StatusCreated {
		t.Fatalf("create crew failed: %d %s", response.Code, response.Body.String())
	}
	var crew crewView
	if err := json.NewDecoder(response.Body).Decode(&crew); err != nil {
		t.Fatal(err)
	}
	return crew
}

// seedTodaysPuzzle puts the daily grid back after the harness truncates. The
// crew board is defined against today's puzzle, so tests that read a board need
// one to exist.
func seedTodaysPuzzle(t *testing.T, store *PostgresPuzzleStore) {
	t.Helper()
	if err := store.Seed(t.Context(), SeedPuzzles()); err != nil {
		t.Fatalf("seed puzzles: %v", err)
	}
}

// solveTodaysPuzzle plays today's daily to completion as the given session.
func solveTodaysPuzzle(t *testing.T, handler http.Handler, cookie *http.Cookie) {
	t.Helper()
	puzzle := SeedPuzzles()[0]
	for index, group := range puzzle.Groups {
		tileIDs := make([]string, 0, GroupSize)
		for _, tile := range group.Tiles {
			tileIDs = append(tileIDs, tile.ID)
		}
		response := postGuess(t, handler, cookie.String(), GuessRequest{
			PuzzleID:        puzzle.ID,
			ClientGuessID:   "solve-" + cookie.Value + "-" + group.ID,
			SelectedTileIDs: tileIDs,
		})
		if response.Code != http.StatusOK {
			t.Fatalf("guess %d failed: %d %s", index, response.Code, response.Body.String())
		}
	}
}

// TestCrewShareGridUsesRealGroupColours guards the quiet failure mode of the
// crew board: colorByTile[id] yields 0 for an unknown tile, so a broken mapping
// would paint every square green and still look plausible. This pins each
// square to the colour of the group the tile actually belongs to.
func TestCrewShareGridUsesRealGroupColours(t *testing.T) {
	puzzle := SeedPuzzles()[0]
	colors := tileColorIndex(puzzle)

	if len(colors) != len(puzzle.Groups)*GroupSize {
		t.Fatalf("expected every tile mapped, got %d of %d", len(colors), len(puzzle.Groups)*GroupSize)
	}

	// One guess made of the first tile of each group must produce one square per
	// colour — not four of the same.
	guess := make([]string, 0, len(puzzle.Groups))
	want := ""
	for _, group := range puzzle.Groups {
		guess = append(guess, group.Tiles[0].ID)
		want += crewShareSquares[group.ColorIndex%len(crewShareSquares)]
	}

	grid := buildCrewShareGrid([][]string{guess}, colors)
	if len(grid) != 1 {
		t.Fatalf("expected one row, got %d", len(grid))
	}
	if grid[0] != want {
		t.Fatalf("share grid row = %q, want %q", grid[0], want)
	}

	// A correct guess (four tiles of one group) is a single-colour row.
	solved := make([]string, 0, GroupSize)
	for _, tile := range puzzle.Groups[2].Tiles {
		solved = append(solved, tile.ID)
	}
	square := crewShareSquares[puzzle.Groups[2].ColorIndex%len(crewShareSquares)]
	grid = buildCrewShareGrid([][]string{solved}, colors)
	if grid[0] != strings.Repeat(square, GroupSize) {
		t.Fatalf("solved row = %q, want %q", grid[0], strings.Repeat(square, GroupSize))
	}
}

func TestCrewCreateJoinAndBoard(t *testing.T) {
	handler, puzzles := newAdminTestServer(t)
	seedTodaysPuzzle(t, puzzles)

	founder := newCrewSession(t, handler)
	crew := createCrew(t, handler, founder, "Sunday Crew", "Uday")
	if crew.JoinPath != "/crew/"+crew.InviteCode {
		t.Fatalf("expected an invite path, got %q", crew.JoinPath)
	}

	// A second browser opens the invite link and joins with its own name.
	friend := newCrewSession(t, handler)
	joined := crewRequest(t, handler, http.MethodPost, "/api/crews/"+crew.InviteCode+"/join", friend, crewJoinRequest{DisplayName: "Ada"})
	if joined.Code != http.StatusOK {
		t.Fatalf("join failed: %d %s", joined.Code, joined.Body.String())
	}

	board := decodeCrewBoard(t, crewRequest(t, handler, http.MethodGet, "/api/crews/"+crew.InviteCode, friend, nil))
	if !board.IsMember {
		t.Fatal("a member must be recognised on the board")
	}
	if len(board.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(board.Members))
	}

	names := map[string]bool{}
	you := 0
	for _, member := range board.Members {
		names[member.DisplayName] = true
		if member.IsYou {
			you++
			if member.DisplayName != "Ada" {
				t.Errorf("isYou marked the wrong member: %q", member.DisplayName)
			}
		}
	}
	if !names["Uday"] || !names["Ada"] {
		t.Fatalf("crew board is missing members: %#v", board.Members)
	}
	if you != 1 {
		t.Fatalf("exactly one member should be marked isYou, got %d", you)
	}

	// The board must never carry session ids: they are the identity cookie.
	if strings.Contains(board.Members[0].DisplayName, founder.Value) {
		t.Fatal("crew board leaked a session id")
	}

	// Rejoining with a new name is an update, not a duplicate member.
	rejoined := crewRequest(t, handler, http.MethodPost, "/api/crews/"+crew.InviteCode+"/join", friend, crewJoinRequest{DisplayName: "Ada L"})
	if rejoined.Code != http.StatusOK {
		t.Fatalf("rejoin failed: %d %s", rejoined.Code, rejoined.Body.String())
	}
	board = decodeCrewBoard(t, crewRequest(t, handler, http.MethodGet, "/api/crews/"+crew.InviteCode, friend, nil))
	if len(board.Members) != 2 {
		t.Fatalf("rejoining duplicated a member: %d members", len(board.Members))
	}
}

// TestCrewBoardHidesGridsUntilTheViewerFinishes is the spoiler contract. A
// member still playing may see how far everyone has got, but never which tiles
// anyone guessed — otherwise the crew board hands out the answers.
func TestCrewBoardHidesGridsUntilTheViewerFinishes(t *testing.T) {
	handler, puzzles := newAdminTestServer(t)
	seedTodaysPuzzle(t, puzzles)

	founder := newCrewSession(t, handler)
	crew := createCrew(t, handler, founder, "Spoiler Test", "Finisher")
	friend := newCrewSession(t, handler)
	if response := crewRequest(t, handler, http.MethodPost, "/api/crews/"+crew.InviteCode+"/join", friend, crewJoinRequest{DisplayName: "Watcher"}); response.Code != http.StatusOK {
		t.Fatalf("join failed: %d %s", response.Code, response.Body.String())
	}

	// The founder finishes today's grid; the friend has not started.
	solveTodaysPuzzle(t, handler, founder)

	watching := crewRequest(t, handler, http.MethodGet, "/api/crews/"+crew.InviteCode, friend, nil)
	board := decodeCrewBoard(t, watching)
	if board.SpoilersUnlocked {
		t.Fatal("spoilers must stay locked for a member who has not finished")
	}
	for _, member := range board.Members {
		if len(member.Grid) != 0 {
			t.Fatalf("%s's grid leaked to a player who is still going: %v", member.DisplayName, member.Grid)
		}
	}
	// Not merely stripped from the struct — absent from the wire format too.
	if strings.Contains(watching.Body.String(), "grid") {
		t.Fatalf("locked board serialized a grid field: %s", watching.Body.String())
	}

	// Progress itself is not a spoiler and stays visible.
	var finisher CrewBoardEntry
	for _, member := range board.Members {
		if member.DisplayName == "Finisher" {
			finisher = member
		}
	}
	if !finisher.Solved || finisher.SolvedCount != PuzzleGroupCount {
		t.Fatalf("a finished member should show as solved: %#v", finisher)
	}
	if finisher.ElapsedSeconds == nil {
		t.Fatal("a finished member should report a time")
	}

	// Once the viewer finishes too, the grids unlock.
	solveTodaysPuzzle(t, handler, friend)
	board = decodeCrewBoard(t, crewRequest(t, handler, http.MethodGet, "/api/crews/"+crew.InviteCode, friend, nil))
	if !board.SpoilersUnlocked {
		t.Fatal("spoilers should unlock once the viewer has finished")
	}
	grids := 0
	for _, member := range board.Members {
		if len(member.Grid) > 0 {
			grids++
			for _, row := range member.Grid {
				if strings.ContainsAny(row, "abcdefghijklmnopqrstuvwxyz") {
					t.Fatalf("share grid must be coloured squares only, got %q", row)
				}
			}
		}
	}
	if grids != 2 {
		t.Fatalf("expected both finished members to expose a grid, got %d", grids)
	}
}

// TestCrewBoardIsReadableByNonMembers lets someone open an invite link and see
// what they are joining, while still being told they are not in it yet.
func TestCrewBoardIsReadableByNonMembers(t *testing.T) {
	handler, puzzles := newAdminTestServer(t)
	seedTodaysPuzzle(t, puzzles)

	founder := newCrewSession(t, handler)
	crew := createCrew(t, handler, founder, "Open Invite", "Uday")

	stranger := newCrewSession(t, handler)
	board := decodeCrewBoard(t, crewRequest(t, handler, http.MethodGet, "/api/crews/"+crew.InviteCode, stranger, nil))
	if board.IsMember {
		t.Fatal("a stranger must not be reported as a member")
	}
	if board.SpoilersUnlocked {
		t.Fatal("a non-member can never have spoilers unlocked")
	}
	for _, member := range board.Members {
		if len(member.Grid) != 0 {
			t.Fatal("a non-member must not see any grid")
		}
	}
}

func TestCrewRejectsDuplicateDisplayNames(t *testing.T) {
	handler, _ := newAdminTestServer(t)

	founder := newCrewSession(t, handler)
	crew := createCrew(t, handler, founder, "Name Clash", "Sam")

	friend := newCrewSession(t, handler)
	clash := crewRequest(t, handler, http.MethodPost, "/api/crews/"+crew.InviteCode+"/join", friend, crewJoinRequest{DisplayName: "sam"})
	if clash.Code != http.StatusConflict {
		t.Fatalf("expected 409 for a duplicate display name, got %d: %s", clash.Code, clash.Body.String())
	}
}

func TestCrewRejectsBadNames(t *testing.T) {
	handler, _ := newAdminTestServer(t)
	session := newCrewSession(t, handler)

	for _, testCase := range []struct {
		name        string
		crewName    string
		displayName string
	}{
		{"empty crew name", "   ", "Uday"},
		{"empty display name", "Crew", ""},
		{"overlong crew name", strings.Repeat("a", maxCrewNameLength+1), "Uday"},
		{"overlong display name", "Crew", strings.Repeat("a", maxDisplayNameLength+1)},
		{"newline in name", "Crew\nInjected", "Uday"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			response := crewRequest(t, handler, http.MethodPost, "/api/crews", session, crewCreateRequest{
				Name:        testCase.crewName,
				DisplayName: testCase.displayName,
			})
			if response.Code != http.StatusUnprocessableEntity {
				t.Fatalf("expected 422, got %d: %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestCrewJoinRejectsUnknownCrew(t *testing.T) {
	handler, _ := newAdminTestServer(t)
	session := newCrewSession(t, handler)

	missing := crewRequest(t, handler, http.MethodPost, "/api/crews/aGVsbG8tdGhlcmU/join", session, crewJoinRequest{DisplayName: "Uday"})
	if missing.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for an unknown crew, got %d: %s", missing.Code, missing.Body.String())
	}

	malformed := crewRequest(t, handler, http.MethodPost, "/api/crews/not%20a%20crew/join", session, crewJoinRequest{DisplayName: "Uday"})
	if malformed.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a malformed crew id, got %d: %s", malformed.Code, malformed.Body.String())
	}
}

func TestMyCrewsListsOnlyYourCrews(t *testing.T) {
	handler, _ := newAdminTestServer(t)

	mine := newCrewSession(t, handler)
	theirs := newCrewSession(t, handler)
	crew := createCrew(t, handler, mine, "Mine", "Uday")
	createCrew(t, handler, theirs, "Theirs", "Someone")

	response := crewRequest(t, handler, http.MethodGet, "/api/crews", mine, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("my crews failed: %d %s", response.Code, response.Body.String())
	}
	var crews []crewView
	if err := json.NewDecoder(response.Body).Decode(&crews); err != nil {
		t.Fatal(err)
	}
	if len(crews) != 1 || crews[0].InviteCode != crew.InviteCode {
		t.Fatalf("expected exactly my own crew, got %#v", crews)
	}
}

// TestCrewInviteRotationRevokesOldLinks is the point of having an invite code
// separate from the crew id: a link that leaked must stop working, while the
// people already inside keep their access.
func TestCrewInviteRotationRevokesOldLinks(t *testing.T) {
	handler, puzzles := newAdminTestServer(t)
	seedTodaysPuzzle(t, puzzles)

	owner := newCrewSession(t, handler)
	crew := createCrew(t, handler, owner, "Leaky Crew", "Owner")
	if !crew.IsOwner {
		t.Fatal("the founder must be reported as the owner")
	}
	leaked := crew.InviteCode

	member := newCrewSession(t, handler)
	if response := crewRequest(t, handler, http.MethodPost, "/api/crews/"+leaked+"/join", member, crewJoinRequest{DisplayName: "Friend"}); response.Code != http.StatusOK {
		t.Fatalf("join failed: %d %s", response.Code, response.Body.String())
	}

	rotated := crewRequest(t, handler, http.MethodPost, "/api/crews/"+leaked+"/rotate", owner, nil)
	if rotated.Code != http.StatusOK {
		t.Fatalf("rotate failed: %d %s", rotated.Code, rotated.Body.String())
	}
	var refreshed crewView
	if err := json.NewDecoder(rotated.Body).Decode(&refreshed); err != nil {
		t.Fatal(err)
	}
	if refreshed.InviteCode == leaked {
		t.Fatal("rotation did not change the invite code")
	}

	// The leaked link is dead for reading and for joining.
	stranger := newCrewSession(t, handler)
	if board := crewRequest(t, handler, http.MethodGet, "/api/crews/"+leaked, stranger, nil); board.Code != http.StatusNotFound {
		t.Fatalf("a rotated-away link must stop reading, got %d", board.Code)
	}
	rejoin := crewRequest(t, handler, http.MethodPost, "/api/crews/"+leaked+"/join", stranger, crewJoinRequest{DisplayName: "Gatecrasher"})
	if rejoin.Code != http.StatusNotFound {
		t.Fatalf("a rotated-away link must stop admitting people, got %d", rejoin.Code)
	}

	// Existing members are unaffected and find the crew through their own list.
	listed := crewRequest(t, handler, http.MethodGet, "/api/crews", member, nil)
	var crews []crewView
	if err := json.NewDecoder(listed.Body).Decode(&crews); err != nil {
		t.Fatal(err)
	}
	if len(crews) != 1 || crews[0].InviteCode != refreshed.InviteCode {
		t.Fatalf("a member's crew list must carry the current invite code, got %#v", crews)
	}
	if crews[0].IsOwner {
		t.Fatal("a non-owner member must not be reported as the owner")
	}
}

func TestOnlyOwnerCanRotateOrRemove(t *testing.T) {
	handler, puzzles := newAdminTestServer(t)
	seedTodaysPuzzle(t, puzzles)

	owner := newCrewSession(t, handler)
	crew := createCrew(t, handler, owner, "Owned Crew", "Owner")

	member := newCrewSession(t, handler)
	if response := crewRequest(t, handler, http.MethodPost, "/api/crews/"+crew.InviteCode+"/join", member, crewJoinRequest{DisplayName: "Friend"}); response.Code != http.StatusOK {
		t.Fatalf("join failed: %d %s", response.Code, response.Body.String())
	}

	if response := crewRequest(t, handler, http.MethodPost, "/api/crews/"+crew.InviteCode+"/rotate", member, nil); response.Code != http.StatusForbidden {
		t.Fatalf("a member must not rotate the invite, got %d: %s", response.Code, response.Body.String())
	}

	// A member's board carries no member ids at all, so there is no handle to
	// replay even if they guessed the route.
	memberBoard := decodeCrewBoard(t, crewRequest(t, handler, http.MethodGet, "/api/crews/"+crew.InviteCode, member, nil))
	for _, entry := range memberBoard.Members {
		if entry.MemberID != "" {
			t.Fatalf("a non-owner was handed a member id for %q", entry.DisplayName)
		}
	}

	ownerBoard := decodeCrewBoard(t, crewRequest(t, handler, http.MethodGet, "/api/crews/"+crew.InviteCode, owner, nil))
	var friendID, ownID string
	for _, entry := range ownerBoard.Members {
		if entry.IsYou {
			ownID = entry.MemberID
			continue
		}
		friendID = entry.MemberID
	}
	if friendID == "" {
		t.Fatal("the owner needs a member id to act on")
	}
	if ownID != "" {
		t.Fatal("the owner must not be given a handle to remove themselves")
	}

	// A member cannot evict anyone, even holding a valid member id.
	if response := crewRequest(t, handler, http.MethodPost, "/api/crews/"+crew.InviteCode+"/members/"+friendID+"/remove", member, nil); response.Code != http.StatusForbidden {
		t.Fatalf("a member must not remove anyone, got %d: %s", response.Code, response.Body.String())
	}

	removed := crewRequest(t, handler, http.MethodPost, "/api/crews/"+crew.InviteCode+"/members/"+friendID+"/remove", owner, nil)
	if removed.Code != http.StatusOK {
		t.Fatalf("owner remove failed: %d %s", removed.Code, removed.Body.String())
	}
	after := decodeCrewBoard(t, crewRequest(t, handler, http.MethodGet, "/api/crews/"+crew.InviteCode, owner, nil))
	if len(after.Members) != 1 {
		t.Fatalf("expected the crew down to one member, got %d", len(after.Members))
	}

	// The evicted session is no longer a member and its crew list is empty.
	evicted := decodeCrewBoard(t, crewRequest(t, handler, http.MethodGet, "/api/crews/"+crew.InviteCode, member, nil))
	if evicted.IsMember {
		t.Fatal("a removed member must not still count as a member")
	}
}

// TestOwnerLeavingTransfersTheCrew keeps a crew from becoming ownerless — and
// therefore permanently unrotatable — when the founder walks away.
func TestOwnerLeavingTransfersTheCrew(t *testing.T) {
	handler, puzzles := newAdminTestServer(t)
	seedTodaysPuzzle(t, puzzles)

	owner := newCrewSession(t, handler)
	crew := createCrew(t, handler, owner, "Succession", "Founder")

	heir := newCrewSession(t, handler)
	if response := crewRequest(t, handler, http.MethodPost, "/api/crews/"+crew.InviteCode+"/join", heir, crewJoinRequest{DisplayName: "Heir"}); response.Code != http.StatusOK {
		t.Fatalf("join failed: %d %s", response.Code, response.Body.String())
	}

	if response := crewRequest(t, handler, http.MethodPost, "/api/crews/"+crew.InviteCode+"/leave", owner, nil); response.Code != http.StatusOK {
		t.Fatalf("owner leave failed: %d %s", response.Code, response.Body.String())
	}

	board := decodeCrewBoard(t, crewRequest(t, handler, http.MethodGet, "/api/crews/"+crew.InviteCode, heir, nil))
	if len(board.Members) != 1 || !board.Crew.IsOwner {
		t.Fatalf("ownership should have passed to the remaining member: %#v", board)
	}
	if response := crewRequest(t, handler, http.MethodPost, "/api/crews/"+crew.InviteCode+"/rotate", heir, nil); response.Code != http.StatusOK {
		t.Fatalf("the new owner must be able to rotate, got %d: %s", response.Code, response.Body.String())
	}
}

// TestLastMemberLeavingDeletesTheCrew stops empty crews accumulating forever.
func TestLastMemberLeavingDeletesTheCrew(t *testing.T) {
	handler, puzzles := newAdminTestServer(t)
	seedTodaysPuzzle(t, puzzles)

	owner := newCrewSession(t, handler)
	crew := createCrew(t, handler, owner, "Solo", "Only")

	if response := crewRequest(t, handler, http.MethodPost, "/api/crews/"+crew.InviteCode+"/leave", owner, nil); response.Code != http.StatusOK {
		t.Fatalf("leave failed: %d %s", response.Code, response.Body.String())
	}
	if board := crewRequest(t, handler, http.MethodGet, "/api/crews/"+crew.InviteCode, owner, nil); board.Code != http.StatusNotFound {
		t.Fatalf("an emptied crew should be gone, got %d", board.Code)
	}
}

func TestLeavingRequiresMembership(t *testing.T) {
	handler, puzzles := newAdminTestServer(t)
	seedTodaysPuzzle(t, puzzles)

	owner := newCrewSession(t, handler)
	crew := createCrew(t, handler, owner, "Members Only", "Owner")

	stranger := newCrewSession(t, handler)
	if response := crewRequest(t, handler, http.MethodPost, "/api/crews/"+crew.InviteCode+"/leave", stranger, nil); response.Code != http.StatusForbidden {
		t.Fatalf("a non-member leaving should be refused, got %d: %s", response.Code, response.Body.String())
	}
}

// TestCrewsDegradeWithoutDatabase keeps no-database local runs honest: the
// feature reports itself unavailable instead of half-working in one process.
func TestCrewsDegradeWithoutDatabase(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles: StaticPuzzleSource(SeedPuzzles()),
		Store:   NewMemoryAttemptStore(),
		Clock:   fixedClock,
	})

	for _, path := range []string{"/api/crews", "/api/crews/abc"} {
		response := crewRequest(t, handler, http.MethodGet, path, nil, nil)
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s: expected 503 without a database, got %d", path, response.Code)
		}
	}
}
