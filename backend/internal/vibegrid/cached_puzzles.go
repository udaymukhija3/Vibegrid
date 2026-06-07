package vibegrid

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

// CacheStats is a point-in-time view of puzzle-cache effectiveness, surfaced on
// /metrics so the hit rate of the latency optimization is observable in prod.
type CacheStats struct {
	Hits      uint64
	Misses    uint64
	Evictions uint64
	Entries   int
}

// puzzleBackend is the full puzzle-store surface the server depends on: the
// public read source plus admin authoring and community creation. The Postgres
// store satisfies it, and cachedPuzzleStore wraps it so the per-id content read
// on the hot guess/attempt/stats/OG path does not hit the database every time.
type puzzleBackend interface {
	PuzzleSource
	AdminPuzzleStore
	CommunityPuzzleStore
}

const defaultPuzzleCacheMaxEntries = 2048

// cachedPuzzleStore caches PuzzleByID results in process. Puzzle content
// (number, groups, tiles) is immutable once created; only status and
// publish_date change, and every method that changes them invalidates the
// affected id. A short TTL bounds staleness if an invalidation is ever missed.
//
// Cached puzzles are treated as read-only by every caller — guess evaluation,
// ToPublicPuzzle, and the stats/OG lookups all copy before mutating — so one
// cached value is safely shared across concurrent requests. singleflight
// collapses a burst of concurrent misses for the same id into a single load,
// which matters at the daily rollover when the edge cache is cold.
type cachedPuzzleStore struct {
	inner      puzzleBackend
	ttl        time.Duration
	maxEntries int
	clock      func() time.Time

	mu      sync.Mutex
	byID    map[string]cachedPuzzleEntry
	flights singleflight.Group

	hits      atomic.Uint64
	misses    atomic.Uint64
	evictions atomic.Uint64
}

type cachedPuzzleEntry struct {
	puzzle    Puzzle
	expiresAt time.Time
	cachedAt  time.Time
}

// NewCachedPuzzleStore wraps a puzzle backend with a per-id content cache.
// A non-positive ttl (or nil backend) disables caching and returns the backend
// unchanged, so callers can opt out without branching.
func NewCachedPuzzleStore(inner puzzleBackend, ttl time.Duration) puzzleBackend {
	if inner == nil || ttl <= 0 {
		return inner
	}
	return &cachedPuzzleStore{
		inner:      inner,
		ttl:        ttl,
		maxEntries: defaultPuzzleCacheMaxEntries,
		clock:      time.Now,
		byID:       map[string]cachedPuzzleEntry{},
	}
}

func (store *cachedPuzzleStore) PuzzleByID(ctx context.Context, puzzleID string) (Puzzle, error) {
	if puzzle, ok := store.getCached(puzzleID); ok {
		store.hits.Add(1)
		return puzzle, nil
	}

	result := store.flights.DoChan(puzzleID, func() (any, error) {
		if puzzle, ok := store.getCached(puzzleID); ok {
			store.hits.Add(1)
			return puzzle, nil
		}
		store.misses.Add(1)
		puzzle, err := store.inner.PuzzleByID(ctx, puzzleID)
		if err != nil {
			// Negative lookups are not cached: a community puzzle is shared by link
			// the instant it is created, so a cached "not found" would hide it.
			return Puzzle{}, err
		}
		store.setCached(puzzleID, puzzle)
		return puzzle, nil
	})

	select {
	case loaded := <-result:
		if loaded.Err != nil {
			return Puzzle{}, loaded.Err
		}
		puzzle, ok := loaded.Val.(Puzzle)
		if !ok {
			return Puzzle{}, fmt.Errorf("unexpected puzzle cache value for %s", puzzleID)
		}
		return puzzle, nil
	case <-ctx.Done():
		return Puzzle{}, ctx.Err()
	}
}

// Puzzles, PublishedPuzzles, and TodaysPuzzle are not cached here: the list/admin
// views are not on the per-guess hot path, and the public daily/archive reads are
// already short-cached at the edge via Cache-Control.
func (store *cachedPuzzleStore) Puzzles(ctx context.Context) ([]Puzzle, error) {
	return store.inner.Puzzles(ctx)
}

func (store *cachedPuzzleStore) PublishedPuzzles(ctx context.Context, today string, limit, offset int) ([]Puzzle, error) {
	return store.inner.PublishedPuzzles(ctx, today, limit, offset)
}

func (store *cachedPuzzleStore) TodaysPuzzle(ctx context.Context, today string) (Puzzle, error) {
	return store.inner.TodaysPuzzle(ctx, today)
}

// CreateDraft and CreateCommunityPuzzle mint a new id that nothing has cached yet
// (negatives are not cached), so no invalidation is needed.
func (store *cachedPuzzleStore) CreateDraft(ctx context.Context, input AdminPuzzleInput) (Puzzle, error) {
	return store.inner.CreateDraft(ctx, input)
}

func (store *cachedPuzzleStore) CreateCommunityPuzzle(ctx context.Context, input AdminPuzzleInput) (Puzzle, error) {
	return store.inner.CreateCommunityPuzzle(ctx, input)
}

// Publish, Archive, and Reinstate change a puzzle's status/publish_date, so they
// evict the affected id after the write commits. Archive in particular must take
// effect promptly: it is how moderation pulls an abusive community puzzle.
func (store *cachedPuzzleStore) Publish(ctx context.Context, puzzleID, publishDate string) error {
	err := store.inner.Publish(ctx, puzzleID, publishDate)
	if err == nil {
		store.invalidate(puzzleID)
	}
	return err
}

func (store *cachedPuzzleStore) Archive(ctx context.Context, puzzleID string) error {
	err := store.inner.Archive(ctx, puzzleID)
	if err == nil {
		store.invalidate(puzzleID)
	}
	return err
}

func (store *cachedPuzzleStore) Reinstate(ctx context.Context, puzzleID string) error {
	err := store.inner.Reinstate(ctx, puzzleID)
	if err == nil {
		store.invalidate(puzzleID)
	}
	return err
}

func (store *cachedPuzzleStore) getCached(puzzleID string) (Puzzle, bool) {
	now := store.clock()

	store.mu.Lock()
	defer store.mu.Unlock()

	entry, ok := store.byID[puzzleID]
	if !ok {
		return Puzzle{}, false
	}
	if now.After(entry.expiresAt) {
		delete(store.byID, puzzleID)
		return Puzzle{}, false
	}
	return entry.puzzle, true
}

func (store *cachedPuzzleStore) setCached(puzzleID string, puzzle Puzzle) {
	now := store.clock()

	store.mu.Lock()
	defer store.mu.Unlock()

	if store.maxEntries <= 0 {
		return
	}
	store.pruneExpiredLocked(now)
	if _, exists := store.byID[puzzleID]; !exists && len(store.byID) >= store.maxEntries {
		store.evictOldestLocked()
	}
	store.byID[puzzleID] = cachedPuzzleEntry{puzzle: puzzle, expiresAt: now.Add(store.ttl), cachedAt: now}
}

func (store *cachedPuzzleStore) invalidate(puzzleID string) {
	store.mu.Lock()
	delete(store.byID, puzzleID)
	store.mu.Unlock()
	// Drop any in-flight load so a guess that raced the write cannot repopulate
	// the cache with the pre-write row.
	store.flights.Forget(puzzleID)
}

func (store *cachedPuzzleStore) pruneExpiredLocked(now time.Time) {
	for puzzleID, entry := range store.byID {
		if now.After(entry.expiresAt) {
			delete(store.byID, puzzleID)
		}
	}
}

func (store *cachedPuzzleStore) evictOldestLocked() {
	var (
		oldestID string
		oldestAt time.Time
	)
	for puzzleID, entry := range store.byID {
		if oldestID == "" || entry.cachedAt.Before(oldestAt) {
			oldestID = puzzleID
			oldestAt = entry.cachedAt
		}
	}
	if oldestID != "" {
		delete(store.byID, oldestID)
		store.evictions.Add(1)
	}
}

// CacheStats reports cumulative hit/miss/eviction counters and the current entry
// count. Safe to call concurrently with reads and writes.
func (store *cachedPuzzleStore) CacheStats() CacheStats {
	store.mu.Lock()
	entries := len(store.byID)
	store.mu.Unlock()
	return CacheStats{
		Hits:      store.hits.Load(),
		Misses:    store.misses.Load(),
		Evictions: store.evictions.Load(),
		Entries:   entries,
	}
}

var _ puzzleBackend = (*cachedPuzzleStore)(nil)
