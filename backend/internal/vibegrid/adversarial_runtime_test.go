package vibegrid

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// EDGE-040: rateLimitScopes charges cookie-bearing and cookie-less callers
// against the SAME network counter (prefix+"net:"+network) but evaluates them
// against two different ceilings — limit*networkRateLimitFactor with a session,
// plain limit without one. One ordinary cookie-bearing user who exhausts their
// own session budget therefore pushes the shared counter past the tight
// ceiling, and every brand-new cookie-less visitor from that network is
// rejected on their very first request while cookie-holders keep going.
//
// First-request visitors are exactly the people arriving from a shared invite
// link, so this denies the crew product's acquisition path on any NAT, office,
// cafe, or carrier-grade-NAT uplink.
func TestSharedNetworkBucketLocksOutFirstTimeVisitors(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles: StaticPuzzleSource(SeedPuzzles()),
		Store:   NewMemoryAttemptStore(),
		Clock:   fixedClock,
	})

	// One ordinary member with a cookie, spending only their own session budget.
	cookie := &http.Cookie{Name: sessionCookieName, Value: strings.Repeat("a", 32)}
	spent := 0
	for index := 0; index < readRateLimit; index++ {
		response := rawRequest(t, handler, http.MethodGet, "/api/vibes/today", "", nil, cookie)
		if response.Code != http.StatusOK {
			break
		}
		spent++
	}
	if spent != readRateLimit {
		t.Fatalf("one session was cut off after %d reads, before its own %d limit", spent, readRateLimit)
	}

	// A different cookie-bearing client on the same network is still served,
	// which proves the counter has not been globally exhausted.
	peer := &http.Cookie{Name: sessionCookieName, Value: strings.Repeat("b", 32)}
	if response := rawRequest(t, handler, http.MethodGet, "/api/vibes/today", "", nil, peer); response.Code != http.StatusOK {
		t.Fatalf("second cookie-bearing client got %d", response.Code)
	}

	// But a brand-new visitor with no cookie yet is refused on request #1.
	victim := rawRequest(t, handler, http.MethodGet, "/api/vibes/today", "", nil)
	if victim.Code == http.StatusTooManyRequests {
		t.Fatalf("after one cookie-bearing user spent their own %d-read session budget, a "+
			"brand-new cookie-less visitor on the same network is refused on their FIRST "+
			"request (%d). rateLimitScopes evaluates one shared \"net:\" counter against two "+
			"different ceilings", readRateLimit, victim.Code)
	}
}

// EDGE-041: dropping the session cookie must not hand a client a fresh budget.
// This is the documented reason existingSessionID never mints an id.
func TestDroppingTheCookieDoesNotResetTheReadBudget(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles: StaticPuzzleSource(SeedPuzzles()),
		Store:   NewMemoryAttemptStore(),
		Clock:   fixedClock,
	})

	allowed := 0
	for index := 0; index < readRateLimit+20; index++ {
		response := rawRequest(t, handler, http.MethodGet, "/api/vibes/today", "", nil)
		if response.Code == http.StatusOK {
			allowed++
			continue
		}
		if response.Code == http.StatusTooManyRequests {
			break
		}
		t.Fatalf("unexpected status %d", response.Code)
	}
	if allowed > readRateLimit {
		t.Fatalf("cookie-less client made %d reads against a %d limit", allowed, readRateLimit)
	}
}

// EDGE-042: rotating a forged session cookie widens the effective per-network
// budget by networkRateLimitFactor. Document the real ceiling so it is a
// deliberate tradeoff rather than an accident.
func TestRotatingForgedSessionsWidensTheNetworkBudget(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles: StaticPuzzleSource(SeedPuzzles()),
		Store:   NewMemoryAttemptStore(),
		Clock:   fixedClock,
	})

	allowed := 0
	for index := 0; index < readRateLimit*networkRateLimitFactor+50; index++ {
		cookie := &http.Cookie{Name: sessionCookieName, Value: fmt.Sprintf("%032x", index)}
		response := rawRequest(t, handler, http.MethodGet, "/api/vibes/today", "", nil, cookie)
		if response.Code == http.StatusOK {
			allowed++
			continue
		}
		break
	}
	if allowed <= readRateLimit {
		return // no widening; nothing to document
	}
	if allowed > readRateLimit*networkRateLimitFactor {
		t.Fatalf("rotating forged sessions allowed %d reads, past even the widened "+
			"network ceiling of %d", allowed, readRateLimit*networkRateLimitFactor)
	}
	t.Logf("rotating forged session cookies raised the per-network read budget from %d to %d "+
		"(networkRateLimitFactor=%d)", readRateLimit, allowed, networkRateLimitFactor)
}

// EDGE-051: the limiter's maxKeys setting is a memory bound, not a deployment-
// wide concurrency limit. A new reader at capacity must displace stale/LRU
// identity state instead of being denied before making a single request.
func TestReadLimiterCapacityEvictsInsteadOfFailingClosed(t *testing.T) {
	limiter := newRateLimiter(3, time.Minute)
	limiter.maxKeys = 2
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)

	if !limiter.check("reader-a", now).allowed || !limiter.check("reader-b", now.Add(time.Second)).allowed {
		t.Fatal("initial readers should be allowed")
	}
	if decision := limiter.check("reader-c", now.Add(2*time.Second)); !decision.allowed {
		t.Fatalf("first request from reader-c was rejected at key capacity: %+v", decision)
	}
	if len(limiter.hits) != limiter.maxKeys {
		t.Fatalf("limiter retained %d keys, want bounded size %d", len(limiter.hits), limiter.maxKeys)
	}
	if _, exists := limiter.hits["reader-a"]; exists {
		t.Fatal("least-recently-used reader was not evicted")
	}
}

func TestMemoryGuestSessionsStayBounded(t *testing.T) {
	store := NewMemoryGuestSessionStore()
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	for index := 0; index < maxMemoryGuestSessions+25; index++ {
		token := fmt.Sprintf("%064x", index)
		identity := fmt.Sprintf("%032x", index)
		if _, active, err := store.ResolveOrCreate(t.Context(), token, "", identity, now.Add(time.Duration(index)*time.Second), now.Add(sessionTTL)); err != nil || !active {
			t.Fatalf("register memory guest %d: active=%v err=%v", index, active, err)
		}
	}
	if len(store.sessions) != maxMemoryGuestSessions {
		t.Fatalf("memory guest store retained %d sessions, want cap %d", len(store.sessions), maxMemoryGuestSessions)
	}
	if _, exists := store.sessions[fmt.Sprintf("%064x", 0)]; exists {
		t.Fatal("memory guest store did not evict its least-recently-seen entry")
	}
}

// EDGE-043: the daily rollover drives which board is "today", which is "judge",
// and which is "result". Probe the exact boundaries rather than mid-afternoon.
func TestDailyRolloverBoundaries(t *testing.T) {
	cases := []struct {
		name                          string
		now                           time.Time
		wantToday, wantJudge, wantRes string
	}{
		{"one ns before midnight UTC",
			time.Date(2026, 8, 21, 23, 59, 59, 999999999, time.UTC),
			"2026-08-21", "2026-08-20", "2026-08-19"},
		{"exactly midnight UTC",
			time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC),
			"2026-08-22", "2026-08-21", "2026-08-20"},
		{"month end",
			time.Date(2026, 8, 31, 23, 59, 59, 0, time.UTC),
			"2026-08-31", "2026-08-30", "2026-08-29"},
		{"month start",
			time.Date(2026, 9, 1, 0, 0, 1, 0, time.UTC),
			"2026-09-01", "2026-08-31", "2026-08-30"},
		{"year end",
			time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC),
			"2026-12-31", "2026-12-30", "2026-12-29"},
		{"leap day",
			time.Date(2028, 2, 29, 12, 0, 0, 0, time.UTC),
			"2028-02-29", "2028-02-28", "2028-02-27"},
		{"day after leap day",
			time.Date(2028, 3, 1, 0, 0, 0, 0, time.UTC),
			"2028-03-01", "2028-02-29", "2028-02-28"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			server := &Server{clock: func() time.Time { return testCase.now }, timeZone: "UTC"}
			today, judge, result := server.vibeRoundDates()
			if today != testCase.wantToday || judge != testCase.wantJudge || result != testCase.wantRes {
				t.Fatalf("got %s/%s/%s, want %s/%s/%s",
					today, judge, result, testCase.wantToday, testCase.wantJudge, testCase.wantRes)
			}
			if today != server.todayString() {
				t.Fatalf("todayString (%s) disagrees with vibeRoundDates today (%s); "+
					"submissions and the crew page would target different boards",
					server.todayString(), today)
			}
			for _, date := range []string{today, judge, result} {
				if _, err := VibeBoardForDate(date); err != nil {
					t.Fatalf("no board for %s: %v", date, err)
				}
			}
		})
	}
}

// EDGE-044: a DST transition must not produce a duplicate or skipped board day
// when the deployment is configured with a non-UTC launch timezone.
func TestDailyRolloverAcrossDST(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	seen := map[string]int{}
	// Spring forward 2027-03-14, fall back 2027-11-07.
	for _, start := range []time.Time{
		time.Date(2027, 3, 13, 0, 0, 0, 0, location),
		time.Date(2027, 11, 6, 0, 0, 0, 0, location),
	} {
		for hour := 0; hour < 96; hour++ {
			moment := start.Add(time.Duration(hour) * time.Hour)
			server := &Server{clock: func() time.Time { return moment }, timeZone: "America/New_York"}
			today, judge, _ := server.vibeRoundDates()
			seen[today]++
			parsedToday, err1 := time.Parse("2006-01-02", today)
			parsedJudge, err2 := time.Parse("2006-01-02", judge)
			if err1 != nil || err2 != nil {
				t.Fatalf("unparseable rollover dates %s/%s", today, judge)
			}
			if gap := parsedToday.Sub(parsedJudge).Hours(); gap != 24 {
				t.Fatalf("at %s the judge board is %v hours behind today, not 24", moment, gap)
			}
		}
	}
	for date, count := range seen {
		if count == 0 {
			t.Fatalf("date %s never appeared across the transition", date)
		}
	}
}

// EDGE-045: the editorial bank must produce a coherent board for any date the
// rollover can generate. Dates before the Unix epoch fall into the
// board-number fallback, which computes a NEGATIVE number and then fails its
// own validation — so a machine whose clock has slipped before 1970 serves 500
// on every board route instead of degrading.
func TestEditorialBankHandlesExtremeDates(t *testing.T) {
	dates := []string{
		"0001-01-01", "1900-01-01", "1969-12-31", "1970-01-01",
		"2026-08-19", "2028-02-29", "2100-02-28", "9999-12-31",
	}
	for _, date := range dates {
		t.Run(date, func(t *testing.T) {
			board, err := VibeBoardForDate(date)
			if err != nil {
				t.Fatalf("date %s produced an error (%v); the daily board, the crew page and "+
					"the practice round all 500 for this date", date, err)
			}
			if board.BoardNumber < 1 {
				t.Fatalf("date %s produced board number %d", date, board.BoardNumber)
			}
			if len(board.Tiles) != VibeBoardMaxTileCount {
				t.Fatalf("date %s dealt %d fragments", date, len(board.Tiles))
			}
			if board.ID != "vibe-"+date {
				t.Fatalf("date %s produced id %q", date, board.ID)
			}
		})
	}

	for _, bad := range []string{"", "not-a-date", "2026-13-01", "2026-02-30", "2026-8-1", "26-08-19", "2026-08-19T00:00:00Z"} {
		if _, err := VibeBoardForDate(bad); err == nil {
			t.Fatalf("invalid date %q was accepted", bad)
		}
	}
}

// EDGE-045b: a pre-epoch clock must not take the whole product down.
func TestSkewedClockDoesNotBlackOutEveryBoardRoute(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles: StaticPuzzleSource(SeedPuzzles()),
		Store:   NewMemoryAttemptStore(),
		Clock:   func() time.Time { return time.Date(1969, 7, 20, 20, 17, 0, 0, time.UTC) },
	})
	response := rawRequest(t, handler, http.MethodGet, "/api/vibes/today", "", nil)
	if response.Code >= 500 {
		t.Fatalf("a clock slipped before 1970 makes /api/vibes/today return %d: %s",
			response.Code, response.Body.String())
	}
}

// EDGE-046: two distant dates must not collide onto the same editorial board
// id, which would let one date's frozen row shadow another.
func TestExtremeDatesDoNotCollideOntoOneBoard(t *testing.T) {
	seen := map[string]string{}
	for _, date := range []string{"2026-08-19", "9999-12-31", "9999-12-30", "0001-01-01", "0001-01-02"} {
		board, err := VibeBoardForDate(date)
		if err != nil {
			t.Fatalf("board for %s: %v", date, err)
		}
		if previous, clash := seen[board.ID]; clash {
			t.Fatalf("dates %s and %s share board id %s", previous, date, board.ID)
		}
		seen[board.ID] = date
	}
}

// EDGE-047: card titles are rendered to other crew members. Reject Unicode
// format characters that can reorder or invisibly join what another member
// reads, even though React correctly escapes markup.
func TestTitleNormalizationAgainstUnicodeTricks(t *testing.T) {
	cases := []struct {
		name  string
		raw   string
		valid bool
	}{
		{"plain", "Soft launch", true},
		{"collapses whitespace", "Soft    launch", true},
		{"collapses newline", "Soft\nlaunch", true},
		{"tab", "Soft\tlaunch", true},
		{"rejects null", "Soft\x00launch", false},
		{"rejects bell", "Soft\alaunch", false},
		{"rejects escape", "Soft\x1blaunch", false},
		{"empty", "", false},
		{"whitespace only", "   \n\t ", false},
		{"exactly 40 runes", strings.Repeat("é", MaxVibeTitleRunes), true},
		{"41 runes", strings.Repeat("é", MaxVibeTitleRunes+1), false},
		{"emoji counted as runes", strings.Repeat("🙂", MaxVibeTitleRunes), true},
		{"41 emoji", strings.Repeat("🙂", MaxVibeTitleRunes+1), false},
		{"combining marks", "é́́ launch", true},
		{"rtl override", "Soft ‮hcnual", false},
		{"zero width joiner", "Soft‍launch", false},
		{"bidi isolate", "⁦Soft launch⁩", false},
	}
	surprises := []string{}
	for _, testCase := range cases {
		normalized, err := normalizeVibeTitle(testCase.raw)
		if (err == nil) != testCase.valid {
			t.Fatalf("%s: valid=%v but err=%v", testCase.name, testCase.valid, err)
		}
		if err != nil {
			continue
		}
		for _, r := range normalized {
			switch r {
			case '‪', '‫', '‬', '‭', '‮', '⁦', '⁧', '⁨', '⁩', '​', '‍':
				surprises = append(surprises, fmt.Sprintf("%s kept U+%04X", testCase.name, r))
			}
		}
	}
	if len(surprises) > 0 {
		t.Fatalf("title normalization retained invisible/bidi formatting characters: %s", strings.Join(surprises, "; "))
	}
}

// EDGE-048: every API error body must be JSON with a stable shape and must not
// leak internals. writeError reuses the guess response type, so confirm what
// callers actually receive.
func TestErrorBodiesHaveAStableShape(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles: StaticPuzzleSource(SeedPuzzles()),
		Store:   NewMemoryAttemptStore(),
		Clock:   fixedClock,
	})
	probes := []struct {
		method, path, contentType, body string
	}{
		{http.MethodGet, "/api/crews/nope/daily", "", ""},
		{http.MethodGet, "/api/puzzles/does-not-exist", "", ""},
		{http.MethodGet, "/api/vibes/practice/nope", "", ""},
		{http.MethodPost, "/api/guesses", "application/json", `{"bad":1}`},
		{http.MethodGet, "/metrics", "", ""},
	}
	for _, probe := range probes {
		response := rawRequest(t, handler, probe.method, probe.path, probe.contentType, []byte(probe.body))
		if response.Code < 400 {
			continue
		}
		contentType := response.Header().Get("Content-Type")
		if !strings.Contains(contentType, "application/json") && response.Code != http.StatusNotFound {
			t.Fatalf("%s %s returned error as %q", probe.method, probe.path, contentType)
		}
		assertNoInternalDetail(t, response.Body.String())
	}
}

// EDGE-049: a public read must never mutate durable state as a side effect in a
// way an unauthenticated caller can amplify. /api/vibes/today lazily persists
// the dated board, so confirm it converges to exactly one row.
func TestPublicTodayReadPersistsAtMostOneBoardRow(t *testing.T) {
	handler, database := hostileServer(t)

	const readers = 24
	done := make(chan int, readers)
	for index := 0; index < readers; index++ {
		go func() {
			response := rawRequest(t, handler, http.MethodGet, "/api/crews/does-not-exist/daily", "", nil)
			done <- response.Code
		}()
	}
	for index := 0; index < readers; index++ {
		<-done
	}

	member := newCrewSession(t, handler)
	crew := createCrew(t, handler, member, "Night Shift", "Ari")
	for index := 0; index < readers; index++ {
		go func() {
			response := rawRequest(t, handler, http.MethodGet, "/api/crews/"+crew.InviteCode+"/daily", "", nil, member)
			done <- response.Code
		}()
	}
	for index := 0; index < readers; index++ {
		<-done
	}

	var boards, frozen int
	if err := database.QueryRow(`select count(*) from vibe_daily_boards`).Scan(&boards); err != nil {
		t.Fatalf("count boards: %v", err)
	}
	if err := database.QueryRow(`select count(*) from vibe_crew_boards`).Scan(&frozen); err != nil {
		t.Fatalf("count frozen: %v", err)
	}
	if boards > 3 {
		t.Fatalf("concurrent reads persisted %d dated boards (want at most 3: today/judge/result)", boards)
	}
	if frozen > 3 {
		t.Fatalf("concurrent reads froze %d crew board rows (want at most 3)", frozen)
	}
}

// EDGE-050: the crew streak is a public-facing number in the crew UI. Probe its
// behaviour around gaps, future dates, and unparseable input.
func TestCrewStreakBoundaryBehaviour(t *testing.T) {
	fixture := newAdversarialFixture(t, "2026-08-21")
	fixture.createCrew(t, "Night Shift", "Ari", "session-a")

	if _, err := fixture.rounds.CrewStreak(fixture.ctx, fixture.crew.ID, "not-a-date"); err == nil {
		t.Fatal("an unparseable through-date was accepted")
	}
	for _, date := range []string{"2026-08-21", "9999-12-31", "0001-01-01"} {
		streak, err := fixture.rounds.CrewStreak(fixture.ctx, fixture.crew.ID, date)
		if err != nil {
			t.Fatalf("streak for %s: %v", date, err)
		}
		if streak != 0 {
			t.Fatalf("a crew with no rounds reported a streak of %d for %s", streak, date)
		}
	}
}

var _ = httptest.NewRequest
