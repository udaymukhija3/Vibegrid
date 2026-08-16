package vibegrid

import (
	"context"
	"time"
)

func observeStoreOperation(metrics *httpMetrics, component, operation string, started time.Time, err error) {
	status := "ok"
	if err != nil {
		status = "error"
	}
	metrics.observeOperation(component, operation, status, time.Since(started))
}

type observedAttemptStore struct {
	next    Store
	metrics *httpMetrics
}

func observeAttemptStore(next Store, metrics *httpMetrics) Store {
	if next == nil || metrics == nil {
		return next
	}
	return &observedAttemptStore{next: next, metrics: metrics}
}

func (store *observedAttemptStore) GetAttempt(ctx context.Context, puzzle Puzzle, sessionID string, now time.Time) (snapshot AttemptSnapshot, err error) {
	started := time.Now()
	defer func() { observeStoreOperation(store.metrics, "attempts", "get_attempt", started, err) }()
	return store.next.GetAttempt(ctx, puzzle, sessionID, now)
}

func (store *observedAttemptStore) SubmitGuess(ctx context.Context, puzzle Puzzle, sessionID string, request GuessRequest, now time.Time) (submission GuessSubmission, err error) {
	started := time.Now()
	defer func() { observeStoreOperation(store.metrics, "attempts", "submit_guess", started, err) }()
	return store.next.SubmitGuess(ctx, puzzle, sessionID, request, now)
}

type observedPuzzleSource struct {
	next    PuzzleSource
	metrics *httpMetrics
}

func observePuzzleSource(next PuzzleSource, metrics *httpMetrics) PuzzleSource {
	if next == nil || metrics == nil {
		return next
	}
	return &observedPuzzleSource{next: next, metrics: metrics}
}

func (source *observedPuzzleSource) Puzzles(ctx context.Context) (puzzles []Puzzle, err error) {
	started := time.Now()
	defer func() { observeStoreOperation(source.metrics, "puzzles", "list_all", started, err) }()
	return source.next.Puzzles(ctx)
}

func (source *observedPuzzleSource) PublishedPuzzles(ctx context.Context, today string, limit, offset int) (puzzles []Puzzle, err error) {
	started := time.Now()
	defer func() { observeStoreOperation(source.metrics, "puzzles", "list_published", started, err) }()
	return source.next.PublishedPuzzles(ctx, today, limit, offset)
}

func (source *observedPuzzleSource) TodaysPuzzle(ctx context.Context, today string) (puzzle Puzzle, err error) {
	started := time.Now()
	defer func() { observeStoreOperation(source.metrics, "puzzles", "today", started, err) }()
	return source.next.TodaysPuzzle(ctx, today)
}

func (source *observedPuzzleSource) PuzzleByID(ctx context.Context, puzzleID string) (puzzle Puzzle, err error) {
	started := time.Now()
	defer func() { observeStoreOperation(source.metrics, "puzzles", "by_id", started, err) }()
	return source.next.PuzzleByID(ctx, puzzleID)
}

type observedAdminPuzzleStore struct {
	next    AdminPuzzleStore
	metrics *httpMetrics
}

func observeAdminPuzzleStore(next AdminPuzzleStore, metrics *httpMetrics) AdminPuzzleStore {
	if next == nil || metrics == nil {
		return next
	}
	return &observedAdminPuzzleStore{next: next, metrics: metrics}
}

func (store *observedAdminPuzzleStore) CreateDraft(ctx context.Context, input AdminPuzzleInput) (puzzle Puzzle, err error) {
	started := time.Now()
	defer func() { observeStoreOperation(store.metrics, "admin_puzzles", "create_draft", started, err) }()
	return store.next.CreateDraft(ctx, input)
}

func (store *observedAdminPuzzleStore) Publish(ctx context.Context, puzzleID, publishDate string) (err error) {
	started := time.Now()
	defer func() { observeStoreOperation(store.metrics, "admin_puzzles", "publish", started, err) }()
	return store.next.Publish(ctx, puzzleID, publishDate)
}

func (store *observedAdminPuzzleStore) ApproveCommunity(ctx context.Context, puzzleID string) (err error) {
	started := time.Now()
	defer func() { observeStoreOperation(store.metrics, "admin_puzzles", "approve_community", started, err) }()
	return store.next.ApproveCommunity(ctx, puzzleID)
}

func (store *observedAdminPuzzleStore) Archive(ctx context.Context, puzzleID string) (err error) {
	started := time.Now()
	defer func() { observeStoreOperation(store.metrics, "admin_puzzles", "archive", started, err) }()
	return store.next.Archive(ctx, puzzleID)
}

func (store *observedAdminPuzzleStore) Reinstate(ctx context.Context, puzzleID string) (err error) {
	started := time.Now()
	defer func() { observeStoreOperation(store.metrics, "admin_puzzles", "reinstate", started, err) }()
	return store.next.Reinstate(ctx, puzzleID)
}

func (store *observedAdminPuzzleStore) PersistDaily(ctx context.Context, puzzle Puzzle) (err error) {
	started := time.Now()
	defer func() { observeStoreOperation(store.metrics, "admin_puzzles", "persist_daily", started, err) }()
	return store.next.PersistDaily(ctx, puzzle)
}

type observedCommunityPuzzleStore struct {
	next    CommunityPuzzleStore
	metrics *httpMetrics
}

func observeCommunityPuzzleStore(next CommunityPuzzleStore, metrics *httpMetrics) CommunityPuzzleStore {
	if next == nil || metrics == nil {
		return next
	}
	return &observedCommunityPuzzleStore{next: next, metrics: metrics}
}

func (store *observedCommunityPuzzleStore) CreateCommunityPuzzle(ctx context.Context, input AdminPuzzleInput, claimHash string) (puzzle Puzzle, err error) {
	started := time.Now()
	defer func() { observeStoreOperation(store.metrics, "community_puzzles", "create", started, err) }()
	return store.next.CreateCommunityPuzzle(ctx, input, claimHash)
}

func (store *observedCommunityPuzzleStore) CreatorStatus(ctx context.Context, puzzleID, claimHash string) (status CreatorPuzzleStatus, err error) {
	started := time.Now()
	defer func() { observeStoreOperation(store.metrics, "community_puzzles", "creator_status", started, err) }()
	return store.next.CreatorStatus(ctx, puzzleID, claimHash)
}

func (store *observedCommunityPuzzleStore) WithdrawCommunityPuzzle(ctx context.Context, puzzleID, claimHash string) (status CreatorPuzzleStatus, err error) {
	started := time.Now()
	defer func() { observeStoreOperation(store.metrics, "community_puzzles", "withdraw", started, err) }()
	return store.next.WithdrawCommunityPuzzle(ctx, puzzleID, claimHash)
}

type observedStatsStore struct {
	next    StatsStore
	metrics *httpMetrics
}

func observeStatsStore(next StatsStore, metrics *httpMetrics) StatsStore {
	if next == nil || metrics == nil {
		return next
	}
	return &observedStatsStore{next: next, metrics: metrics}
}

func (store *observedStatsStore) PuzzleStats(ctx context.Context, puzzleID string) (stats PuzzleStats, err error) {
	started := time.Now()
	defer func() { observeStoreOperation(store.metrics, "stats", "puzzle_stats", started, err) }()
	return store.next.PuzzleStats(ctx, puzzleID)
}

func (store *observedStatsStore) WrongGuessGroupings(ctx context.Context, puzzleID string, limit int) (groups []WrongGuessGrouping, err error) {
	started := time.Now()
	defer func() { observeStoreOperation(store.metrics, "stats", "wrong_guess_groupings", started, err) }()
	return store.next.WrongGuessGroupings(ctx, puzzleID, limit)
}

func (store *observedStatsStore) SessionStreak(ctx context.Context, sessionID, today string) (summary StreakSummary, err error) {
	started := time.Now()
	defer func() { observeStoreOperation(store.metrics, "stats", "session_streak", started, err) }()
	return store.next.SessionStreak(ctx, sessionID, today)
}

type observedRateLimitStore struct {
	next    RateLimitStore
	metrics *httpMetrics
}

func observeRateLimitStore(next RateLimitStore, metrics *httpMetrics) RateLimitStore {
	if next == nil || metrics == nil {
		return next
	}
	return &observedRateLimitStore{next: next, metrics: metrics}
}

func (store *observedRateLimitStore) Check(ctx context.Context, key string, limit int, window time.Duration, now time.Time) (decision rateLimitDecision, err error) {
	started := time.Now()
	defer func() { observeStoreOperation(store.metrics, "rate_limits", "check", started, err) }()
	return store.next.Check(ctx, key, limit, window, now)
}

func (store *observedRateLimitStore) Prune(ctx context.Context) (err error) {
	started := time.Now()
	defer func() { observeStoreOperation(store.metrics, "rate_limits", "prune", started, err) }()
	return store.next.Prune(ctx)
}

type observedIdempotencyStore struct {
	next    IdempotencyStore
	metrics *httpMetrics
}

func observeIdempotencyStore(next IdempotencyStore, metrics *httpMetrics) IdempotencyStore {
	if next == nil || metrics == nil {
		return next
	}
	return &observedIdempotencyStore{next: next, metrics: metrics}
}

func (store *observedIdempotencyStore) Execute(
	ctx context.Context,
	scope, keyHash, requestHash string,
	action func(context.Context) idempotencyResponse,
) (response idempotencyResponse, replayed, conflict bool, err error) {
	started := time.Now()
	defer func() { observeStoreOperation(store.metrics, "idempotency", "execute", started, err) }()
	return store.next.Execute(ctx, scope, keyHash, requestHash, action)
}

func (store *observedIdempotencyStore) PruneExpired(ctx context.Context, before time.Time, limit int) (deleted int64, err error) {
	started := time.Now()
	defer func() { observeStoreOperation(store.metrics, "idempotency", "prune", started, err) }()
	return store.next.PruneExpired(ctx, before, limit)
}

type observedModerationStore struct {
	next    ModerationStore
	metrics *httpMetrics
}

func observeModerationStore(next ModerationStore, metrics *httpMetrics) ModerationStore {
	if next == nil || metrics == nil {
		return next
	}
	return &observedModerationStore{next: next, metrics: metrics}
}

func (store *observedModerationStore) CreateReport(ctx context.Context, input ReportInput, sessionID string) (report ModerationReport, err error) {
	started := time.Now()
	defer func() { observeStoreOperation(store.metrics, "moderation", "create_report", started, err) }()
	return store.next.CreateReport(ctx, input, sessionID)
}

func (store *observedModerationStore) ListReports(ctx context.Context) (reports []ModerationReport, err error) {
	started := time.Now()
	defer func() { observeStoreOperation(store.metrics, "moderation", "list_reports", started, err) }()
	return store.next.ListReports(ctx)
}

func (store *observedModerationStore) ResolveReport(ctx context.Context, reportID, status, note, actor string) (report ModerationReport, err error) {
	started := time.Now()
	defer func() { observeStoreOperation(store.metrics, "moderation", "resolve_report", started, err) }()
	return store.next.ResolveReport(ctx, reportID, status, note, actor)
}

func (store *observedModerationStore) CreateAppeal(ctx context.Context, input AppealInput) (appeal ModerationAppeal, err error) {
	started := time.Now()
	defer func() { observeStoreOperation(store.metrics, "moderation", "create_appeal", started, err) }()
	return store.next.CreateAppeal(ctx, input)
}

func (store *observedModerationStore) ListAppeals(ctx context.Context) (appeals []ModerationAppeal, err error) {
	started := time.Now()
	defer func() { observeStoreOperation(store.metrics, "moderation", "list_appeals", started, err) }()
	return store.next.ListAppeals(ctx)
}

func (store *observedModerationStore) ResolveAppeal(ctx context.Context, appealID, note, actor string) (appeal ModerationAppeal, err error) {
	started := time.Now()
	defer func() { observeStoreOperation(store.metrics, "moderation", "resolve_appeal", started, err) }()
	return store.next.ResolveAppeal(ctx, appealID, note, actor)
}

func (store *observedModerationStore) AddAction(ctx context.Context, action ModerationActionInput) (err error) {
	started := time.Now()
	defer func() { observeStoreOperation(store.metrics, "moderation", "add_action", started, err) }()
	return store.next.AddAction(ctx, action)
}

func (store *observedModerationStore) AuditLog(ctx context.Context, limit int) (actions []ModerationAction, err error) {
	started := time.Now()
	defer func() { observeStoreOperation(store.metrics, "moderation", "audit_log", started, err) }()
	return store.next.AuditLog(ctx, limit)
}
