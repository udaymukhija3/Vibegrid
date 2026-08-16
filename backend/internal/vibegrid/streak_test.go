package vibegrid

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"
)

// newTestStatsStore mirrors newTestStore: it needs TEST_DATABASE_URL and starts
// from empty tables. It returns the raw handle too, so tests can insert attempts
// with an exact completed_at.
func newTestStatsStore(t *testing.T) (*PostgresStatsStore, *sql.DB) {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres integration tests")
	}

	database, err := OpenDB(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	if _, err := database.Exec(`truncate rate_limit_hits, admin_sessions, moderation_actions, moderation_reports, moderation_appeals, puzzles, attempts, attempt_guesses restart identity cascade`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return NewPostgresStatsStore(database, "UTC"), database
}

// completeAttempt records a finished attempt for puzzleID, completed at
// completedAt. It deliberately does not require the puzzle to exist: the daily
// the product actually serves is bank-synthesized and never persisted.
func completeAttempt(t *testing.T, database *sql.DB, sessionID, puzzleID string, completedAt time.Time) {
	t.Helper()
	_, err := database.Exec(
		`insert into attempts (puzzle_id, session_id, mistakes, guess_count, completed, failed, solved_group_ids, started_at, completed_at)
		 values ($1, $2, 0, 4, true, false, '{}', $3, $3)`,
		puzzleID, sessionID, completedAt.UTC(),
	)
	if err != nil {
		t.Fatalf("insert attempt %s: %v", puzzleID, err)
	}
}

func dailyID(day time.Time) string { return "vibegrid-" + day.Format(dateLayout) }

// TestSessionStreakCountsBankDailies is the regression test for the streak
// reading 0 for every player. SessionStreak inner-joined puzzles, but the daily
// is synthesized from the bank and never written there, so every real
// completion was discarded.
func TestSessionStreakCountsBankDailies(t *testing.T) {
	store, database := newTestStatsStore(t)
	ctx := context.Background()

	today := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	completeAttempt(t, database, "session-a", dailyID(today), today)

	summary, err := store.SessionStreak(ctx, "session-a", today.Format(dateLayout))
	if err != nil {
		t.Fatalf("session streak: %v", err)
	}
	if summary.CurrentStreak != 1 || summary.TotalCompleted != 1 {
		t.Errorf("a completed bank daily did not count: got %+v, want CurrentStreak=1 TotalCompleted=1", summary)
	}
}

func TestSessionStreakCountsConsecutiveDays(t *testing.T) {
	store, database := newTestStatsStore(t)
	ctx := context.Background()

	today := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	for offset := 0; offset < 3; offset++ {
		day := today.AddDate(0, 0, -offset)
		completeAttempt(t, database, "session-a", dailyID(day), day)
	}

	summary, err := store.SessionStreak(ctx, "session-a", today.Format(dateLayout))
	if err != nil {
		t.Fatalf("session streak: %v", err)
	}
	if summary.CurrentStreak != 3 || summary.LongestStreak != 3 || summary.TotalCompleted != 3 {
		t.Errorf("got %+v, want CurrentStreak=3 LongestStreak=3 TotalCompleted=3", summary)
	}
}

// TestSessionStreakIgnoresBackfilledDailies guards the farming hole that a naive
// fix would open. Every past daily is playable from the archive and the bank
// synthesizes any date on demand, so completing 30 old dailies in one sitting
// must not manufacture a 30-day streak.
func TestSessionStreakIgnoresBackfilledDailies(t *testing.T) {
	store, database := newTestStatsStore(t)
	ctx := context.Background()

	today := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	// Today's daily, played today: legitimately counts.
	completeAttempt(t, database, "session-a", dailyID(today), today)
	// Thirty past dailies, all completed today from the archive.
	for offset := 1; offset <= 30; offset++ {
		completeAttempt(t, database, "session-a", dailyID(today.AddDate(0, 0, -offset)), today)
	}

	summary, err := store.SessionStreak(ctx, "session-a", today.Format(dateLayout))
	if err != nil {
		t.Fatalf("session streak: %v", err)
	}
	if summary.CurrentStreak != 1 || summary.TotalCompleted != 1 {
		t.Errorf("backfilled dailies inflated the streak: got %+v, want CurrentStreak=1 TotalCompleted=1", summary)
	}
}

// TestSessionStreakCountsAdminEditorialDaily keeps the original behaviour for
// admin-authored editorial puzzles, whose ids are not date-shaped and whose date
// therefore still comes from puzzles.publish_date.
func TestSessionStreakCountsAdminEditorialDaily(t *testing.T) {
	store, database := newTestStatsStore(t)
	ctx := context.Background()

	today := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	if _, err := database.Exec(
		`insert into puzzles (id, puzzle_number, publish_date, status, difficulty, origin)
		 values ('pzl_editorial_one', 900, $1, 'PUBLISHED', 'MEDIUM', 'EDITORIAL')`,
		today.Format(dateLayout),
	); err != nil {
		t.Fatalf("insert editorial puzzle: %v", err)
	}
	completeAttempt(t, database, "session-a", "pzl_editorial_one", today)

	summary, err := store.SessionStreak(ctx, "session-a", today.Format(dateLayout))
	if err != nil {
		t.Fatalf("session streak: %v", err)
	}
	if summary.CurrentStreak != 1 || summary.TotalCompleted != 1 {
		t.Errorf("admin editorial daily stopped counting: got %+v, want CurrentStreak=1 TotalCompleted=1", summary)
	}
}

// TestSessionStreakIgnoresCommunityPuzzles keeps community play out of the daily
// streak entirely.
func TestSessionStreakIgnoresCommunityPuzzles(t *testing.T) {
	store, database := newTestStatsStore(t)
	ctx := context.Background()

	today := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	if _, err := database.Exec(
		`insert into puzzles (id, puzzle_number, publish_date, status, difficulty, origin)
		 values ('pzl_community_one', 901, $1, 'PUBLISHED', 'MEDIUM', 'COMMUNITY')`,
		today.Format(dateLayout),
	); err != nil {
		t.Fatalf("insert community puzzle: %v", err)
	}
	completeAttempt(t, database, "session-a", "pzl_community_one", today)

	summary, err := store.SessionStreak(ctx, "session-a", today.Format(dateLayout))
	if err != nil {
		t.Fatalf("session streak: %v", err)
	}
	if summary.TotalCompleted != 0 {
		t.Errorf("community puzzle counted toward the daily streak: got %+v", summary)
	}
}
