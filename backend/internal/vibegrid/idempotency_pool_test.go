package vibegrid

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"
)

// These tests pin down a pool-safety invariant for idempotent mutations: the
// handler runs as the action callback inside PostgresIdempotencyStore.Execute,
// which already holds a transaction — and therefore a pooled connection. Any
// domain work inside that callback must reuse the transaction rather than
// acquire a second connection from the same pool.
//
// When it does acquire a second one, the pool deadlocks: with MaxOpenConns at N,
// N concurrent mutations each hold a transaction while waiting for a connection
// that only another one of them could release. Nothing unwinds it until
// databaseOperationTimeout expires and every request fails with 503.

// newPoolBoundModerationServer wires a Postgres-backed server whose pool is
// capped at maxConns, so a handler that needs one more connection than its
// enclosing transaction holds deadlocks instead of merely running slowly.
func newPoolBoundModerationServer(t *testing.T, maxConns int) (http.Handler, *sql.DB) {
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

	if _, err := database.Exec(`truncate idempotency_keys, rate_limit_hits, admin_sessions, moderation_actions, moderation_reports, moderation_appeals, puzzles, attempts, attempt_guesses restart identity cascade`); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	// Shrink the pool after migrations so setup is not itself starved.
	database.SetMaxOpenConns(maxConns)
	database.SetMaxIdleConns(maxConns)

	puzzleStore := NewPostgresPuzzleStore(database)
	handler := NewServer(ServerConfig{
		// The production read path is wrapped in the content cache, so exercise
		// that layer too: a cache miss inside a transaction must not escape it.
		Puzzles:     NewCachedPuzzleStore(puzzleStore, 5*time.Minute),
		Store:       NewPostgresAttemptStore(database),
		Community:   puzzleStore,
		Idempotency: NewPostgresIdempotencyStore(database),
		Moderation:  NewPostgresModerationStore(database),
		Clock:       fixedClock,
	})
	return handler, database
}

// insertPlayablePuzzle creates a PUBLISHED editorial puzzle dated on or before
// fixedClock's day, so publicPuzzleByID resolves it.
//
// It is given real groups and tiles on purpose. Loading a puzzle runs three
// queries in sequence, and a transaction can only have one result set open at a
// time — so a full board is what proves the group and tile reads drain and hand
// the connection back rather than colliding on it.
func insertPlayablePuzzle(t *testing.T, database *sql.DB, id string) {
	t.Helper()
	if _, err := database.Exec(
		`insert into puzzles (id, puzzle_number, publish_date, status, difficulty, origin)
		 values ($1, nextval('puzzle_number_seq'), '2026-06-01'::date, 'PUBLISHED', 'MEDIUM', 'EDITORIAL')`,
		id,
	); err != nil {
		t.Fatalf("insert puzzle %s: %v", id, err)
	}

	for groupIndex, group := range validPuzzleInput().Groups {
		groupID := fmt.Sprintf("%s_g%d", id, groupIndex)
		if _, err := database.Exec(
			`insert into puzzle_groups (id, puzzle_id, name, explanation, color_index, sort_order)
			 values ($1, $2, $3, $4, $5, $6)`,
			groupID, id, group.Name, group.Explanation, groupIndex, groupIndex,
		); err != nil {
			t.Fatalf("insert group %s: %v", groupID, err)
		}
		for tileIndex, text := range group.Tiles {
			if _, err := database.Exec(
				`insert into puzzle_tiles (id, puzzle_id, group_id, text, sort_order)
				 values ($1, $2, $3, $4, $5)`,
				fmt.Sprintf("%s_t%d", groupID, tileIndex), id, groupID, text, groupIndex*GroupSize+tileIndex,
			); err != nil {
				t.Fatalf("insert tile for %s: %v", groupID, err)
			}
		}
	}
}

func reportRequest(t *testing.T, handler http.Handler, puzzleID, idempotencyKey string) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(ReportInput{PuzzleID: puzzleID, Reason: "SPAM", Details: "spam grid"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/reports", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(idempotencyHeader, idempotencyKey)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

// TestIdempotentReportCreateUsesOneConnection is the deterministic form of the
// deadlock. With a single-connection pool, the idempotency transaction holds the
// only connection, so a report create that reaches for a second one can never
// finish — it blocks until databaseOperationTimeout and returns 503.
func TestIdempotentReportCreateUsesOneConnection(t *testing.T) {
	handler, database := newPoolBoundModerationServer(t, 1)
	insertPlayablePuzzle(t, database, "pzl_pool_single")

	recorder := reportRequest(t, handler, "pzl_pool_single", "pool-single-conn-key")
	if recorder.Code != http.StatusCreated {
		t.Fatalf("report create on a single-connection pool: got %d, want 201 (body %s)", recorder.Code, recorder.Body.String())
	}
}

// TestCachedPuzzleReadInsideTransactionStaysOnItsTransaction covers the cache
// layer directly. Two properties matter, and the single-connection pool proves
// the first: the read can only complete if it ran on the caller's transaction.
//
// The second is why the cache is bypassed rather than merely populated. The
// cache collapses concurrent misses through singleflight, and singleflight runs
// only the *leader's* closure. A transactional read that joined a flight led by
// an ordinary reader would therefore wait on a load that needs its own pooled
// connection — reintroducing the deadlock whenever report traffic overlaps with
// plain reads of the same puzzle. Not sharing the flight in either direction is
// what keeps the two apart, and leaving the cache unwritten follows from it: a
// value read inside a transaction must not outlive a rollback.
func TestCachedPuzzleReadInsideTransactionStaysOnItsTransaction(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres integration tests")
	}

	ctx := context.Background()
	database, err := OpenDB(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.Exec(`truncate puzzles restart identity cascade`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	insertPlayablePuzzle(t, database, "pzl_cache_tx")
	database.SetMaxOpenConns(1)

	cache, ok := NewCachedPuzzleStore(NewPostgresPuzzleStore(database), 5*time.Minute).(*cachedPuzzleStore)
	if !ok {
		t.Fatal("NewCachedPuzzleStore did not return a caching store")
	}

	// Claim the only connection, so any read that leaves the transaction hangs.
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	puzzle, err := cache.PuzzleByID(contextWithTransaction(ctx, tx), "pzl_cache_tx")
	if err != nil {
		t.Fatalf("cached read inside a transaction: %v", err)
	}
	if puzzle.ID != "pzl_cache_tx" {
		t.Errorf("loaded puzzle id: got %q, want %q", puzzle.ID, "pzl_cache_tx")
	}
	// The board proves the follow-up group and tile queries also ran on the
	// transaction: on a one-connection pool they had nowhere else to run.
	if len(puzzle.Groups) != len(validPuzzleInput().Groups) {
		t.Fatalf("loaded groups: got %d, want %d", len(puzzle.Groups), len(validPuzzleInput().Groups))
	}
	for _, group := range puzzle.Groups {
		if len(group.Tiles) != GroupSize {
			t.Errorf("group %q tiles: got %d, want %d", group.Name, len(group.Tiles), GroupSize)
		}
	}
	if _, _, cached := cache.getCached("pzl_cache_tx"); cached {
		t.Error("a read inside a transaction populated the shared cache; it must not outlive a rollback")
	}
}

// TestIdempotentAppealCreateUsesOneConnection covers the sibling handler. Its
// puzzle lookup goes through CreatorStatus, which already reuses the enclosing
// transaction; this keeps it that way, since the same single-connection pool
// makes any regression an immediate hang rather than a silent slowdown.
func TestIdempotentAppealCreateUsesOneConnection(t *testing.T) {
	handler, database := newPoolBoundModerationServer(t, 1)

	// An appeal contests a takedown, so only an ARCHIVED creator-owned grid is
	// appealable.
	claimSecret := newCreatorClaimSecret()
	if _, err := database.Exec(
		`insert into puzzles (id, puzzle_number, status, difficulty, origin, creator_claim_hash)
		 values ($1, nextval('puzzle_number_seq'), 'ARCHIVED', 'MEDIUM', 'COMMUNITY', $2)`,
		"pzl_appeal_pool", hashCreatorClaimSecret(claimSecret),
	); err != nil {
		t.Fatalf("insert archived community puzzle: %v", err)
	}

	payload, err := json.Marshal(AppealInput{PuzzleID: "pzl_appeal_pool", Message: "Please re-review this grid."})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/appeals", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(creatorClaimHeader, claimSecret)
	request.Header.Set(idempotencyHeader, "pool-appeal-single-conn-key")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("appeal create on a single-connection pool: got %d, want 201 (body %s)", recorder.Code, recorder.Body.String())
	}
}

// TestConcurrentIdempotentReportCreatesDoNotExhaustPool reproduces the reported
// production shape: enough concurrent report creates to hold every pooled
// connection in an idempotency transaction at once.
func TestConcurrentIdempotentReportCreatesDoNotExhaustPool(t *testing.T) {
	const maxConns = 2
	const concurrency = 8

	handler, database := newPoolBoundModerationServer(t, maxConns)
	insertPlayablePuzzle(t, database, "pzl_pool_concurrent")

	var waitGroup sync.WaitGroup
	codes := make([]int, concurrency)
	bodies := make([]string, concurrency)
	start := make(chan struct{})

	for index := 0; index < concurrency; index++ {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			<-start
			// Distinct keys so each request is a real mutation rather than a
			// replay of the first one.
			recorder := reportRequest(t, handler, "pzl_pool_concurrent", fmt.Sprintf("pool-concurrent-key-%02d", index))
			codes[index] = recorder.Code
			bodies[index] = recorder.Body.String()
		}(index)
	}
	close(start)
	waitGroup.Wait()

	for index, code := range codes {
		if code != http.StatusCreated {
			t.Errorf("concurrent report create %d: got %d, want 201 (body %s)", index, code, bodies[index])
		}
	}

	var reports int
	if err := database.QueryRow(`select count(*) from moderation_reports`).Scan(&reports); err != nil {
		t.Fatalf("count reports: %v", err)
	}
	if reports != concurrency {
		t.Errorf("stored reports: got %d, want %d", reports, concurrency)
	}
}
