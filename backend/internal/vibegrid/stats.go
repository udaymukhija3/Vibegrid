package vibegrid

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/lib/pq"
	"golang.org/x/sync/singleflight"
)

const dateLayout = "2006-01-02"

// wrongGuessView is one heatmap row with human-readable tile text.
type wrongGuessView struct {
	Tiles []string `json:"tiles"`
	Count int      `json:"count"`
}

type adminAnalyticsResponse struct {
	Stats        PuzzleStats      `json:"stats"`
	WrongGuesses []wrongGuessView `json:"wrongGuesses"`
}

// handleStats serves public completion stats for any puzzle. Without a database
// it returns empty stats so the UI can simply show nothing.
func (server *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if !server.allowPuzzleRead(w, r) {
		return
	}
	if _, err := server.publicPuzzleByID(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusNotFound, "Puzzle not found.")
		return
	}

	if server.stats == nil {
		writeJSON(w, http.StatusOK, PuzzleStats{})
		return
	}

	stats, err := server.stats.PuzzleStats(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load stats.")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// handleStreak serves the current session's daily-completion streak. Without a
// database it returns an empty summary (streaks need persistence).
func (server *Server) handleStreak(w http.ResponseWriter, r *http.Request) {
	sessionID := EnsureSessionID(w, r, server.secureCookies)
	if server.stats == nil {
		writeJSON(w, http.StatusOK, StreakSummary{})
		return
	}

	summary, err := server.stats.SessionStreak(r.Context(), sessionID, server.todayString())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load streak.")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

// handleAdminAnalytics serves the admin wrong-guess heatmap: stats plus the most
// common incorrect groupings, with tile ids resolved to their text.
func (server *Server) handleAdminAnalytics(w http.ResponseWriter, r *http.Request) {
	if server.stats == nil {
		writeError(w, http.StatusServiceUnavailable, "Analytics require a database.")
		return
	}
	puzzleID := r.PathValue("id")
	if !validPuzzleID(puzzleID) {
		writeError(w, http.StatusBadRequest, "Puzzle id is required.")
		return
	}

	stats, err := server.stats.PuzzleStats(r.Context(), puzzleID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load stats.")
		return
	}

	groupings, err := server.stats.WrongGuessGroupings(r.Context(), puzzleID, 10)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load analytics.")
		return
	}

	tileText := server.tileTextLookup(r.Context(), puzzleID)
	wrongGuesses := make([]wrongGuessView, 0, len(groupings))
	for _, grouping := range groupings {
		tiles := make([]string, 0, len(grouping.TileIDs))
		for _, id := range grouping.TileIDs {
			if text, ok := tileText[id]; ok {
				tiles = append(tiles, text)
			} else {
				tiles = append(tiles, id)
			}
		}
		wrongGuesses = append(wrongGuesses, wrongGuessView{Tiles: tiles, Count: grouping.Count})
	}

	writeJSON(w, http.StatusOK, adminAnalyticsResponse{Stats: stats, WrongGuesses: wrongGuesses})
}

// tileTextLookup builds an id->text map for a puzzle, used to render heatmap rows
// readably. Returns an empty map if the puzzle can't be loaded.
func (server *Server) tileTextLookup(ctx context.Context, puzzleID string) map[string]string {
	lookup := map[string]string{}
	puzzle, err := server.puzzles.PuzzleByID(ctx, puzzleID)
	if err != nil {
		return lookup
	}
	for _, group := range puzzle.Groups {
		for _, tile := range group.Tiles {
			lookup[tile.ID] = tile.Text
		}
	}
	return lookup
}

// PuzzleStats is the public, aggregate view of how a puzzle is going: how many
// people have actually played it and how they're faring. Everything is derived
// from the attempts table, so there is no separate analytics pipeline to keep
// in sync.
type PuzzleStats struct {
	Players            int      `json:"players"`
	SolveRate          float64  `json:"solveRate"` // 0..1, of players who made a guess
	FailRate           float64  `json:"failRate"`
	MedianMistakes     float64  `json:"medianMistakes"`
	MedianSolveSeconds *float64 `json:"medianSolveSeconds,omitempty"`
}

// WrongGuessGrouping is one frequently-submitted incorrect set of four tiles,
// the heart of the admin wrong-guess heatmap.
type WrongGuessGrouping struct {
	TileIDs []string `json:"tileIds"`
	Count   int      `json:"count"`
}

// canonicalTileSet makes selection order irrelevant without mutating the
// caller's slice. New wrong guesses are stored this way, and the analytics query
// applies the same normalization to historical rows written before this rule.
func canonicalTileSet(tileIDs []string) []string {
	canonical := append([]string(nil), tileIDs...)
	sort.Strings(canonical)
	return canonical
}

// StatsStore exposes read-only analytics over attempts and guesses. Only the
// Postgres implementation exists; without a database the server reports empty
// stats rather than failing.
type StatsStore interface {
	PuzzleStats(ctx context.Context, puzzleID string) (PuzzleStats, error)
	WrongGuessGroupings(ctx context.Context, puzzleID string, limit int) ([]WrongGuessGrouping, error)
	SessionStreak(ctx context.Context, sessionID, today string) (StreakSummary, error)
}

type cachedStatsStore struct {
	next       StatsStore
	ttl        time.Duration
	maxEntries int
	clock      func() time.Time

	mu      sync.Mutex
	stats   map[string]cachedStatsEntry
	flights singleflight.Group
}

type cachedStatsEntry struct {
	value     PuzzleStats
	expiresAt time.Time
	cachedAt  time.Time
}

const defaultStatsCacheMaxEntries = 1024

func NewCachedStatsStore(next StatsStore, ttl time.Duration) StatsStore {
	if next == nil || ttl <= 0 {
		return next
	}
	return &cachedStatsStore{
		next:       next,
		ttl:        ttl,
		maxEntries: defaultStatsCacheMaxEntries,
		clock:      time.Now,
		stats:      map[string]cachedStatsEntry{},
	}
}

func (store *cachedStatsStore) PuzzleStats(ctx context.Context, puzzleID string) (PuzzleStats, error) {
	if stats, ok := store.getCached(puzzleID); ok {
		return stats, nil
	}

	result := store.flights.DoChan(puzzleID, func() (any, error) {
		if stats, ok := store.getCached(puzzleID); ok {
			return stats, nil
		}

		stats, err := store.next.PuzzleStats(ctx, puzzleID)
		if err != nil {
			return PuzzleStats{}, err
		}

		store.setCached(puzzleID, stats)
		return stats, nil
	})

	select {
	case loaded := <-result:
		if loaded.Err != nil {
			return PuzzleStats{}, loaded.Err
		}
		stats, ok := loaded.Val.(PuzzleStats)
		if !ok {
			return PuzzleStats{}, fmt.Errorf("unexpected stats cache value for %s", puzzleID)
		}
		return stats, nil
	case <-ctx.Done():
		return PuzzleStats{}, ctx.Err()
	}
}

func (store *cachedStatsStore) getCached(puzzleID string) (PuzzleStats, bool) {
	now := store.clock()

	store.mu.Lock()
	defer store.mu.Unlock()

	entry, ok := store.stats[puzzleID]
	if !ok {
		return PuzzleStats{}, false
	}
	if now.After(entry.expiresAt) {
		delete(store.stats, puzzleID)
		return PuzzleStats{}, false
	}
	return entry.value, true
}

func (store *cachedStatsStore) setCached(puzzleID string, stats PuzzleStats) {
	now := store.clock()

	store.mu.Lock()
	defer store.mu.Unlock()

	if store.maxEntries <= 0 {
		return
	}
	store.pruneExpiredLocked(now)
	if _, exists := store.stats[puzzleID]; !exists && len(store.stats) >= store.maxEntries {
		store.evictOldestLocked()
	}

	store.stats[puzzleID] = cachedStatsEntry{value: stats, expiresAt: now.Add(store.ttl), cachedAt: now}
}

func (store *cachedStatsStore) pruneExpiredLocked(now time.Time) {
	for puzzleID, entry := range store.stats {
		if now.After(entry.expiresAt) {
			delete(store.stats, puzzleID)
		}
	}
}

func (store *cachedStatsStore) evictOldestLocked() {
	var (
		oldestID string
		oldestAt time.Time
	)

	for puzzleID, entry := range store.stats {
		if oldestID == "" || entry.cachedAt.Before(oldestAt) {
			oldestID = puzzleID
			oldestAt = entry.cachedAt
		}
	}
	if oldestID != "" {
		delete(store.stats, oldestID)
	}
}

func (store *cachedStatsStore) WrongGuessGroupings(ctx context.Context, puzzleID string, limit int) ([]WrongGuessGrouping, error) {
	return store.next.WrongGuessGroupings(ctx, puzzleID, limit)
}

func (store *cachedStatsStore) SessionStreak(ctx context.Context, sessionID, today string) (StreakSummary, error) {
	return store.next.SessionStreak(ctx, sessionID, today)
}

type PostgresStatsStore struct {
	db *sql.DB
	// location is the launch timezone. The streak needs it to decide which
	// calendar day an attempt was completed on.
	location *time.Location
}

// NewPostgresStatsStore builds the stats store. timeZone is the launch timezone
// (the same one the daily rollover uses); an unknown name falls back to UTC.
func NewPostgresStatsStore(database *sql.DB, timeZone string) *PostgresStatsStore {
	location, err := time.LoadLocation(timeZone)
	if err != nil {
		location = time.UTC
	}
	return &PostgresStatsStore{db: database, location: location}
}

func (store *PostgresStatsStore) PuzzleStats(ctx context.Context, puzzleID string) (PuzzleStats, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	var (
		players      int
		completed    int
		failed       int
		medMistakes  sql.NullFloat64
		medSolveSecs sql.NullFloat64
	)
	err := store.db.QueryRowContext(ctx,
		`select
		   count(*) filter (where guess_count > 0)                                as players,
		   count(*) filter (where completed)                                      as completed,
		   count(*) filter (where failed)                                         as failed,
		   percentile_cont(0.5) within group (order by mistakes)
		     filter (where guess_count > 0)                                       as median_mistakes,
		   percentile_cont(0.5) within group (order by extract(epoch from (completed_at - started_at)))
		     filter (where completed and completed_at is not null)                as median_solve_seconds
		 from attempts
		 where puzzle_id = $1`,
		puzzleID,
	).Scan(&players, &completed, &failed, &medMistakes, &medSolveSecs)
	if err != nil {
		return PuzzleStats{}, fmt.Errorf("puzzle stats: %w", err)
	}

	stats := PuzzleStats{Players: players}
	if players > 0 {
		stats.SolveRate = float64(completed) / float64(players)
		stats.FailRate = float64(failed) / float64(players)
	}
	if medMistakes.Valid {
		stats.MedianMistakes = medMistakes.Float64
	}
	if medSolveSecs.Valid {
		seconds := medSolveSecs.Float64
		stats.MedianSolveSeconds = &seconds
	}
	return stats, nil
}

func (store *PostgresStatsStore) WrongGuessGroupings(ctx context.Context, puzzleID string, limit int) ([]WrongGuessGrouping, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	rows, err := store.db.QueryContext(ctx,
		`select normalized.selected_tile_ids, count(*) as n
		 from (
		   select array(
		     select tile_id
		     from unnest(g.selected_tile_ids) as tile_id
		     order by tile_id
		   ) as selected_tile_ids
		   from attempt_guesses g
		   join attempts a on a.id = g.attempt_id
		   where a.puzzle_id = $1 and g.is_correct = false
		 ) normalized
		 group by normalized.selected_tile_ids
		 order by n desc
		 limit $2`,
		puzzleID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("wrong guess groupings: %w", err)
	}
	defer rows.Close()

	groupings := []WrongGuessGrouping{}
	for rows.Next() {
		var grouping WrongGuessGrouping
		if err := rows.Scan(pq.Array(&grouping.TileIDs), &grouping.Count); err != nil {
			return nil, fmt.Errorf("scan grouping: %w", err)
		}
		groupings = append(groupings, grouping)
	}
	return groupings, rows.Err()
}

// dailyPuzzleIDDate matches the id the daily is served under, "vibegrid-<date>".
// Bank-synthesized dailies carry their date only in the id — they are never
// written to the puzzles table — so it is the authoritative source for them.
const dailyPuzzleIDDate = `^vibegrid-\d{4}-\d{2}-\d{2}$`

// SessionStreak reads the daily puzzles a session has completed and derives the
// streak summary. "today" is the current daily date in the launch timezone,
// supplied by the caller so the store stays clock-agnostic.
//
// Two things this query has to get right:
//
// It must not require the puzzle to exist in the puzzles table. It used to inner
// join puzzles, but the daily actually served is synthesized from the evergreen
// bank and deliberately never persisted (see bank_source.go), so the join
// discarded every real daily completion and the streak read 0 for everyone. The
// date is taken from the puzzle id, falling back to the joined publish_date for
// admin-authored editorial dailies whose ids are not date-shaped.
//
// It must also only count a daily that was played on its own day. Any past daily
// is playable from the archive and the bank will synthesize any date on demand,
// so counting a completion regardless of when it happened would let anyone farm
// an arbitrary streak in seconds. A day counts when the attempt was completed on
// that same calendar day in the launch timezone.
func (store *PostgresStatsStore) SessionStreak(ctx context.Context, sessionID, today string) (StreakSummary, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	rows, err := store.db.QueryContext(ctx,
		`select
		   coalesce(
		     substring(a.puzzle_id from '^vibegrid-(\d{4}-\d{2}-\d{2})$'),
		     to_char(p.publish_date, 'YYYY-MM-DD')
		   ) as puzzle_date,
		   a.completed_at
		 from attempts a
		 left join puzzles p
		   on p.id = a.puzzle_id
		  and p.origin = 'EDITORIAL'
		  and p.publish_date is not null
		 where a.session_id = $1
		   and a.completed = true
		   and a.completed_at is not null
		   and (a.puzzle_id ~ $2 or p.id is not null)`,
		sessionID, dailyPuzzleIDDate,
	)
	if err != nil {
		return StreakSummary{}, fmt.Errorf("session streak: %w", err)
	}
	defer rows.Close()

	dates := []string{}
	for rows.Next() {
		var (
			puzzleDate  sql.NullString
			completedAt time.Time
		)
		if err := rows.Scan(&puzzleDate, &completedAt); err != nil {
			return StreakSummary{}, fmt.Errorf("scan streak row: %w", err)
		}
		if !puzzleDate.Valid || puzzleDate.String == "" {
			continue
		}
		// Played on its own day, in the launch timezone.
		if completedAt.In(store.location).Format(dateLayout) != puzzleDate.String {
			continue
		}
		dates = append(dates, puzzleDate.String)
	}
	if err := rows.Err(); err != nil {
		return StreakSummary{}, err
	}
	return computeStreak(dates, today), nil
}

// computeStreak derives current/longest/total from a set of completed daily
// dates. The current streak anchors on today, or on yesterday if today is not
// done yet (so a streak isn't shown broken until a full day is actually missed).
func computeStreak(completedDates []string, today string) StreakSummary {
	set := map[string]bool{}
	for _, d := range completedDates {
		if d != "" {
			set[d] = true
		}
	}

	summary := StreakSummary{TotalCompleted: len(set)}
	if len(set) == 0 {
		return summary
	}

	sorted := make([]string, 0, len(set))
	for d := range set {
		sorted = append(sorted, d)
	}
	sort.Strings(sorted)

	longest, run := 0, 0
	var prev time.Time
	havePrev := false
	for _, ds := range sorted {
		d, err := time.Parse(dateLayout, ds)
		if err != nil {
			continue
		}
		if havePrev && d.Equal(prev.AddDate(0, 0, 1)) {
			run++
		} else {
			run = 1
		}
		if run > longest {
			longest = run
		}
		prev, havePrev = d, true
	}
	summary.LongestStreak = longest

	todayT, err := time.Parse(dateLayout, today)
	if err != nil {
		return summary
	}
	anchor := todayT
	if !set[today] {
		yesterday := todayT.AddDate(0, 0, -1)
		if !set[yesterday.Format(dateLayout)] {
			return summary // missed both today and yesterday: streak is broken
		}
		anchor = yesterday
	}
	for d := anchor; set[d.Format(dateLayout)]; d = d.AddDate(0, 0, -1) {
		summary.CurrentStreak++
	}
	return summary
}
