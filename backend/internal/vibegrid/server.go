package vibegrid

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// maxGuessBodyBytes caps the guess payload. A legal guess is four short tile
// ids, so anything large is malformed or hostile.
const maxGuessBodyBytes = 16 << 10 // 16 KiB

const (
	requestTimeout  = 5 * time.Second
	guessRateLimit  = 120
	guessRateWindow = time.Minute

	// Reports/appeals are public, unauthenticated writes into the moderation
	// queue; a generous per-IP cap stops one client from flooding it.
	reportRateLimit  = 30
	reportRateWindow = time.Hour

	// Admin login is a single shared secret, so throttle password attempts per IP.
	adminLoginRateLimit  = 10
	adminLoginRateWindow = time.Minute
)

type ServerConfig struct {
	Puzzles            PuzzleSource
	Store              Store
	AdminPuzzles       AdminPuzzleStore
	Community          CommunityPuzzleStore
	Stats              StatsStore
	RateLimits         RateLimitStore
	Moderation         ModerationStore
	ReadyCheck         func(context.Context) error
	Frontend           http.Handler
	AdminToken         string
	AdminPassword      string
	AdminSessionSecret string
	Clock              func() time.Time
	TimeZone           string
	AllowedOrigins     []string
	SecureCookies      bool
	BlockedTerms       []string
	// Optional runtime metric sources, surfaced on /metrics. Nil in no-database
	// mode, where there is no pool or content cache to observe.
	DBStats          func() sql.DBStats
	PuzzleCacheStats func() CacheStats
}

type Server struct {
	puzzles            PuzzleSource
	store              Store
	adminPuzzles       AdminPuzzleStore
	community          CommunityPuzzleStore
	stats              StatsStore
	rateLimits         RateLimitStore
	moderation         ModerationStore
	readyCheck         func(context.Context) error
	createLimiter      *rateLimiter
	guessLimiter       *rateLimiter
	reportLimiter      *rateLimiter
	loginLimiter       *rateLimiter
	blocklist          *wordBlocklist
	metrics            *httpMetrics
	adminToken         string
	adminPassword      string
	adminSessionSecret string
	clock              func() time.Time
	timeZone           string
	secureCookies      bool
	dbStats            func() sql.DBStats
	puzzleCacheStats   func() CacheStats
}

func NewServer(config ServerConfig) http.Handler {
	clock := config.Clock
	if clock == nil {
		clock = time.Now
	}

	timeZone := config.TimeZone
	if timeZone == "" {
		timeZone = "Asia/Kolkata"
	}

	server := &Server{
		puzzles:            config.Puzzles,
		store:              config.Store,
		adminPuzzles:       config.AdminPuzzles,
		community:          config.Community,
		stats:              config.Stats,
		rateLimits:         config.RateLimits,
		moderation:         config.Moderation,
		readyCheck:         config.ReadyCheck,
		createLimiter:      newRateLimiter(20, time.Hour),
		guessLimiter:       newRateLimiter(guessRateLimit, guessRateWindow),
		reportLimiter:      newRateLimiter(reportRateLimit, reportRateWindow),
		loginLimiter:       newRateLimiter(adminLoginRateLimit, adminLoginRateWindow),
		blocklist:          newWordBlocklist(config.BlockedTerms),
		metrics:            newHTTPMetrics(),
		adminToken:         config.AdminToken,
		adminPassword:      config.AdminPassword,
		adminSessionSecret: config.AdminSessionSecret,
		clock:              clock,
		timeZone:           timeZone,
		secureCookies:      config.SecureCookies,
		dbStats:            config.DBStats,
		puzzleCacheStats:   config.PuzzleCacheStats,
	}
	if server.store == nil {
		server.store = NewMemoryAttemptStore()
	}
	if server.puzzles == nil {
		server.puzzles = StaticPuzzleSource(nil)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.handleHealth)
	mux.HandleFunc("GET /readyz", server.handleReady)
	mux.HandleFunc("GET /metrics", server.handleMetrics)
	mux.HandleFunc("GET /api/puzzles/today", server.handleTodayPuzzle)
	mux.HandleFunc("GET /api/puzzles", server.handlePuzzles)
	mux.HandleFunc("GET /api/puzzles/{id}", server.handleGetPuzzle)
	mux.HandleFunc("GET /api/puzzles/{id}/stats", server.handleStats)
	mux.HandleFunc("GET /api/puzzles/{id}/vibes", server.handlePuzzleVibes)
	mux.HandleFunc("GET /api/puzzle-templates", server.handlePuzzleTemplates)
	mux.HandleFunc("GET /api/session", server.handleSessionStatus)
	mux.HandleFunc("GET /api/og/puzzles/{id}", server.handlePuzzleOGImage)
	mux.HandleFunc("GET /robots.txt", server.handleRobots)
	mux.HandleFunc("GET /sitemap.xml", server.handleSitemap)
	mux.HandleFunc("GET /api/attempts/", server.handleAttempt)
	mux.HandleFunc("GET /api/streak", server.handleStreak)
	mux.HandleFunc("POST /api/guesses", server.handleGuess)
	mux.HandleFunc("POST /api/community/puzzles", server.handleCommunityCreate)
	mux.HandleFunc("POST /api/reports", server.handleCreateReport)
	mux.HandleFunc("POST /api/appeals", server.handleCreateAppeal)

	mux.HandleFunc("GET /api/admin/session", server.handleAdminSessionStatus)
	mux.HandleFunc("POST /api/admin/session", server.handleAdminSession)
	mux.HandleFunc("DELETE /api/admin/session", server.handleAdminLogout)
	mux.HandleFunc("GET /api/admin/puzzles", server.requireAdmin(server.handleAdminListPuzzles))
	mux.HandleFunc("POST /api/admin/puzzles", server.requireAdmin(server.handleAdminCreatePuzzle))
	mux.HandleFunc("POST /api/admin/puzzles/{id}/publish", server.requireAdmin(server.handleAdminPublishPuzzle))
	mux.HandleFunc("POST /api/admin/puzzles/{id}/archive", server.requireAdmin(server.handleAdminArchivePuzzle))
	mux.HandleFunc("POST /api/admin/puzzles/{id}/reinstate", server.requireAdmin(server.handleAdminReinstatePuzzle))
	mux.HandleFunc("GET /api/admin/puzzles/{id}/analytics", server.requireAdmin(server.handleAdminAnalytics))
	mux.HandleFunc("GET /api/admin/moderation/reports", server.requireAdmin(server.handleAdminReports))
	mux.HandleFunc("POST /api/admin/moderation/reports/{id}/resolve", server.requireAdmin(server.handleAdminResolveReport))
	mux.HandleFunc("GET /api/admin/moderation/appeals", server.requireAdmin(server.handleAdminAppeals))
	mux.HandleFunc("POST /api/admin/moderation/appeals/{id}/resolve", server.requireAdmin(server.handleAdminResolveAppeal))
	mux.HandleFunc("GET /api/admin/moderation/audit", server.requireAdmin(server.handleAdminAuditLog))
	if config.Frontend != nil {
		mux.Handle("GET /", server.withFrontendMetadata(config.Frontend))
	}

	handler := withRequestTimeout(mux, requestTimeout)
	handler = withCORS(handler, config.AllowedOrigins)
	handler = withSecurityHeaders(handler)
	handler = withRequestMetrics(handler, server.metrics)
	return withRequestLogging(handler)
}

// todayString is the current daily date in the configured launch timezone — the
// same notion of "today" that selects the daily puzzle.
func (server *Server) todayString() string {
	location, err := time.LoadLocation(server.timeZone)
	if err != nil {
		location = time.UTC
	}
	return server.clock().In(location).Format("2006-01-02")
}

func (server *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleReady is the readiness probe: liveness (handleHealth) means the process
// is up, readiness means it can actually serve — i.e. the database is reachable.
// Platforms route traffic only once this returns 200.
func (server *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if server.readyCheck != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := server.readyCheck(ctx); err != nil {
			writeError(w, http.StatusServiceUnavailable, "Not ready.")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ready": true})
}

func (server *Server) handleTodayPuzzle(w http.ResponseWriter, r *http.Request) {
	puzzle, err := server.puzzles.TodaysPuzzle(r.Context(), server.todayString())
	if err != nil {
		writeError(w, http.StatusNotFound, "Puzzle not found.")
		return
	}

	// The public puzzle payload carries no per-user data, so it is safe to cache
	// at the CDN/edge for a short window. Near midnight, cap the TTL to the next
	// rollover so clients do not keep yesterday's board after today changes.
	w.Header().Set("Cache-Control", server.dailyCacheControl())
	writeJSON(w, http.StatusOK, ToPublicPuzzle(puzzle))
}

func (server *Server) handlePuzzles(w http.ResponseWriter, r *http.Request) {
	// Bound the archive read: it grows by one puzzle a day forever, so without a
	// limit this would eventually load every puzzle's full content in one query.
	limit, offset := archivePagination(r)
	puzzles, err := server.puzzles.PublishedPuzzles(r.Context(), server.todayString(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load puzzles.")
		return
	}

	publicPuzzles := make([]PublicPuzzle, 0, len(puzzles))
	for _, puzzle := range puzzles {
		publicPuzzles = append(publicPuzzles, ToPublicPuzzle(puzzle))
	}

	w.Header().Set("Cache-Control", server.dailyCacheControl())
	writeJSON(w, http.StatusOK, publicPuzzles)
}

func (server *Server) handleAttempt(w http.ResponseWriter, r *http.Request) {
	puzzleID := strings.TrimPrefix(r.URL.Path, "/api/attempts/")
	if puzzleID == "" {
		writeError(w, http.StatusBadRequest, "Puzzle id is required.")
		return
	}

	puzzle, err := server.publicPuzzleByID(r.Context(), puzzleID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Puzzle not found.")
		return
	}

	sessionID := EnsureSessionID(w, r, server.secureCookies)
	attempt, err := server.store.GetAttempt(r.Context(), puzzle, sessionID, server.clock())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not load that attempt.")
		return
	}
	writeJSON(w, http.StatusOK, attempt)
}

func (server *Server) handleGuess(w http.ResponseWriter, r *http.Request) {
	if server.guessLimiter != nil || server.rateLimits != nil {
		decision, err := server.checkRateLimit(r.Context(), "guess:"+clientIP(r), guessRateLimit, guessRateWindow, server.guessLimiter)
		if err != nil {
			// Fail open: a rate-limit backend hiccup must not block gameplay. The
			// guess path is still protected by per-attempt row locking and
			// idempotent client guess ids, so allowing through is safe.
			slog.Warn("guess rate-limit check failed; allowing", "error", err)
		} else if !decision.allowed {
			writeRateLimit(w, "You're guessing too quickly. Slow down for a minute.", decision.retryAfter)
			return
		}
	}

	var request GuessRequest
	if !decodeJSONBody(w, r, maxGuessBodyBytes, &request, "That guess payload is not valid.") {
		return
	}

	if request.PuzzleID == "" || request.ClientGuessID == "" {
		writeError(w, http.StatusBadRequest, "Puzzle id and client guess id are required.")
		return
	}

	puzzle, err := server.publicPuzzleByID(r.Context(), request.PuzzleID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Puzzle not found.")
		return
	}

	sessionID := EnsureSessionID(w, r, server.secureCookies)
	submission, err := server.store.SubmitGuess(r.Context(), puzzle, sessionID, request, server.clock())
	if err != nil {
		status := http.StatusUnprocessableEntity
		switch {
		case errors.Is(err, ErrAttemptFinished):
			status = http.StatusConflict
		case isGuessValidationError(err):
			status = http.StatusUnprocessableEntity
		default:
			// Unexpected (storage/transaction) failures are 500s, not client errors.
			slog.Error("guess submission failed", "error", err, "puzzle_id", request.PuzzleID)
			writeError(w, http.StatusInternalServerError, "Could not record that guess.")
			return
		}

		writeError(w, status, humanError(err))
		return
	}

	response := GuessResponse{
		OK:        true,
		IsCorrect: submission.IsCorrect,
		Group:     submission.Group,
		Attempt:   &submission.Attempt,
		SessionID: sessionID,
	}
	if !submission.IsCorrect {
		response.OneAway = IsOneAway(puzzle, request.SelectedTileIDs)
	}
	if submission.Attempt.Failed {
		response.RevealedGroups = submission.Attempt.RevealedGroups
	}

	writeJSON(w, http.StatusOK, response)
}

// handlePuzzleVibes returns the puzzle's group names ("vibes") with their colour
// index, in a stable colour order. It powers the guided Standard mode, which
// reveals one vibe at a time for the player to match. It deliberately omits the
// tile→group mapping and the explanation, so the answer is never exposed: the
// guess engine remains the only authority on whether four tiles form a group.
func (server *Server) handlePuzzleVibes(w http.ResponseWriter, r *http.Request) {
	puzzle, err := server.publicPuzzleByID(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "Puzzle not found.")
		return
	}

	vibes := make([]VibeHint, 0, len(puzzle.Groups))
	for _, group := range puzzle.Groups {
		vibes = append(vibes, VibeHint{Name: group.Name, ColorIndex: group.ColorIndex})
	}
	sort.Slice(vibes, func(i, j int) bool { return vibes[i].ColorIndex < vibes[j].ColorIndex })

	// Names are static for a puzzle, so cache like the other public reads.
	w.Header().Set("Cache-Control", "public, max-age=300, s-maxage=900")
	writeJSON(w, http.StatusOK, map[string]any{"vibes": vibes})
}

// handlePuzzleTemplates serves the curated starter puzzles for the create page.
// Templates are static, fully-exposed content (separate from the daily bank), so
// it caches like the other public reads.
func (server *Server) handlePuzzleTemplates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "public, max-age=300, s-maxage=900")
	writeJSON(w, http.StatusOK, map[string]any{"templates": PuzzleTemplates()})
}

func (server *Server) handlePuzzleOGImage(w http.ResponseWriter, r *http.Request) {
	puzzleID := strings.TrimSuffix(r.PathValue("id"), ".svg")
	puzzle, err := server.publicPuzzleByID(r.Context(), puzzleID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Puzzle not found.")
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=300, s-maxage=900")
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	_, _ = w.Write([]byte(renderPuzzleOGImage(puzzle)))
}

func (server *Server) publicPuzzleByID(ctx context.Context, puzzleID string) (Puzzle, error) {
	puzzle, err := server.puzzles.PuzzleByID(ctx, puzzleID)
	if err != nil {
		return Puzzle{}, err
	}
	if !PubliclyPlayable(puzzle, server.todayString()) {
		return Puzzle{}, ErrPuzzleNotFound
	}
	return puzzle, nil
}

func (server *Server) dailyCacheControl() string {
	seconds := server.secondsUntilRollover()
	maxAge := minInt(60, seconds)
	sharedMaxAge := minInt(300, seconds)
	return fmt.Sprintf("public, max-age=%d, s-maxage=%d", maxAge, sharedMaxAge)
}

func (server *Server) secondsUntilRollover() int {
	location, err := time.LoadLocation(server.timeZone)
	if err != nil {
		location = time.UTC
	}

	now := server.clock().In(location)
	next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, location)
	seconds := int(next.Sub(now).Seconds())
	if seconds < 0 {
		return 0
	}
	return seconds
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

const (
	defaultArchivePageSize = 30
	maxArchivePageSize     = 100
	maxArchiveOffset       = 1_000_000
)

// archivePagination reads and clamps the limit/offset query params for the
// public archive. Bad or missing values fall back to sane defaults rather than
// erroring, so a hand-edited URL never breaks the page.
func archivePagination(r *http.Request) (limit, offset int) {
	limit = clampedQueryInt(r, "limit", defaultArchivePageSize, 1, maxArchivePageSize)
	offset = clampedQueryInt(r, "offset", 0, 0, maxArchiveOffset)
	return limit, offset
}

func clampedQueryInt(r *http.Request, key string, fallback, minValue, maxValue int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

// isGuessValidationError reports whether err is a client-fixable problem with
// the guess (wrong size, unknown tile, already-solved group) as opposed to a
// storage failure.
func isGuessValidationError(err error) bool {
	return errors.Is(err, ErrInvalidGroupSize) ||
		errors.Is(err, ErrUnknownTile) ||
		errors.Is(err, ErrAlreadySolved)
}

func humanError(err error) string {
	switch {
	case errors.Is(err, ErrInvalidGroupSize):
		return "Pick exactly four unique tiles."
	case errors.Is(err, ErrUnknownTile):
		return "This guess contains a tile that is not in the puzzle."
	case errors.Is(err, ErrAlreadySolved):
		return "That vibe is already locked."
	case errors.Is(err, ErrAttemptFinished):
		return "This attempt is already finished."
	default:
		return "Something went wrong."
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, GuessResponse{
		OK:    false,
		Error: message,
	})
}

type bufferedResponse struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newBufferedResponse() *bufferedResponse {
	return &bufferedResponse{header: http.Header{}}
}

func (response *bufferedResponse) Header() http.Header {
	return response.header
}

func (response *bufferedResponse) WriteHeader(status int) {
	if response.status == 0 {
		response.status = status
	}
}

func (response *bufferedResponse) Write(body []byte) (int, error) {
	if response.status == 0 {
		response.status = http.StatusOK
	}
	return response.body.Write(body)
}

func (server *Server) withFrontendMetadata(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metadata, ok := frontendMetadataFor(r)
		if r.Method != http.MethodGet || !ok {
			next.ServeHTTP(w, r)
			return
		}

		recorder := newBufferedResponse()
		// A document navigation is the first thing both tabs load. Establishing the
		// session cookie here — rather than waiting for the first /api call — means
		// two tabs opened together share one session, so their server-side attempt
		// state cannot diverge into two anonymous sessions. EnsureSessionID is a
		// no-op when the request already carries a valid cookie.
		EnsureSessionID(recorder, r, server.secureCookies)
		next.ServeHTTP(recorder, r)

		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		body := recorder.body.Bytes()
		if status == http.StatusOK && bytes.Contains(body, []byte("<head>")) {
			body = injectFrontendMetadata(body, metadata)
			recorder.header.Del("Content-Length")
		}

		copyHeaders(w.Header(), recorder.header)
		w.WriteHeader(status)
		_, _ = w.Write(body)
	})
}

type frontendMetadata struct {
	title       string
	description string
	pageURL     string
	imageURL    string
	imageType   string
}

func frontendMetadataFor(r *http.Request) (frontendMetadata, bool) {
	baseURL := requestBaseURL(r)
	if puzzleID := sharedPuzzleID(r.URL.Path); puzzleID != "" {
		escapedID := url.PathEscape(puzzleID)
		return frontendMetadata{
			title:       "VibeGrid shared puzzle",
			description: "A four-by-four vibe puzzle. Find the hidden groups.",
			pageURL:     baseURL + "/p/" + escapedID,
			imageURL:    baseURL + "/api/og/puzzles/" + escapedID + ".svg",
			imageType:   "image/svg+xml",
		}, true
	}

	if !isFrontendDocumentRoute(r.URL.Path) {
		return frontendMetadata{}, false
	}
	return frontendMetadata{
		title:       "VibeGrid",
		description: "Group the words. Guess the vibe. Try not to overthink it.",
		pageURL:     baseURL + r.URL.Path,
		imageURL:    baseURL + "/vibegrid-mark.svg",
		imageType:   "image/svg+xml",
	}, true
}

func isFrontendDocumentRoute(route string) bool {
	if route == "/" {
		return true
	}
	cleaned := strings.Trim(route, "/")
	if cleaned == "" || strings.HasPrefix(cleaned, "_next/") || strings.HasPrefix(cleaned, "api/") {
		return false
	}
	return !strings.Contains(cleaned, ".")
}

func sharedPuzzleID(route string) string {
	trimmed := strings.Trim(route, "/")
	if !strings.HasPrefix(trimmed, "p/") {
		return ""
	}
	puzzleID := strings.TrimPrefix(trimmed, "p/")
	if puzzleID == "" || strings.Contains(puzzleID, "/") {
		return ""
	}
	decoded, err := url.PathUnescape(puzzleID)
	if err != nil {
		return ""
	}
	return decoded
}

func injectFrontendMetadata(body []byte, metadata frontendMetadata) []byte {
	tags := strings.Join([]string{
		`<meta property="og:type" content="website">`,
		fmt.Sprintf(`<meta property="og:title" content="%s">`, html.EscapeString(metadata.title)),
		fmt.Sprintf(`<meta property="og:description" content="%s">`, html.EscapeString(metadata.description)),
		fmt.Sprintf(`<meta property="og:url" content="%s">`, html.EscapeString(metadata.pageURL)),
		fmt.Sprintf(`<meta property="og:image" content="%s">`, html.EscapeString(metadata.imageURL)),
		fmt.Sprintf(`<meta property="og:image:type" content="%s">`, html.EscapeString(metadata.imageType)),
		`<meta name="twitter:card" content="summary_large_image">`,
		fmt.Sprintf(`<meta name="twitter:title" content="%s">`, html.EscapeString(metadata.title)),
		fmt.Sprintf(`<meta name="twitter:description" content="%s">`, html.EscapeString(metadata.description)),
		fmt.Sprintf(`<meta name="twitter:image" content="%s">`, html.EscapeString(metadata.imageURL)),
	}, "")

	return bytes.Replace(body, []byte("<head>"), []byte("<head>"+tags), 1)
}

func requestBaseURL(r *http.Request) string {
	scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}

	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	return scheme + "://" + host
}

func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func renderPuzzleOGImage(puzzle Puzzle) string {
	const (
		width    = 1200
		height   = 630
		gridLeft = 710
		gridTop  = 120
		tileSize = 92
		tileGap  = 14
	)
	colors := []string{"#2ec4b6", "#f9c74f", "#ff6b6b", "#6d5dfc"}
	title := fmt.Sprintf("VibeGrid #%d", puzzle.PuzzleNumber)
	subtitle := "Shared grid"
	if puzzle.PublishDate != "" {
		subtitle = puzzle.PublishDate
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, width, height, width, height))
	builder.WriteString(`<rect width="1200" height="630" fill="#f8fafc"/>`)
	builder.WriteString(`<path d="M0 0h1200v630H0z" fill="url(#grid)"/><defs><pattern id="grid" width="40" height="40" patternUnits="userSpaceOnUse"><path d="M40 0H0v40" fill="none" stroke="#171717" stroke-opacity=".05"/></pattern></defs>`)
	builder.WriteString(`<rect x="56" y="56" width="1088" height="518" rx="28" fill="#fff" stroke="#171717" stroke-width="5"/>`)
	builder.WriteString(fmt.Sprintf(`<text x="96" y="174" font-family="Inter,Arial,sans-serif" font-size="76" font-weight="900" fill="#171717">%s</text>`, html.EscapeString(title)))
	builder.WriteString(fmt.Sprintf(`<text x="100" y="232" font-family="Inter,Arial,sans-serif" font-size="30" font-weight="800" fill="#6b7280">%s</text>`, html.EscapeString(subtitle)))
	builder.WriteString(`<text x="100" y="312" font-family="Inter,Arial,sans-serif" font-size="42" font-weight="900" fill="#171717">Find the four hidden vibes.</text>`)
	builder.WriteString(`<text x="100" y="368" font-family="Inter,Arial,sans-serif" font-size="26" font-weight="700" fill="#4b5563">Four groups of four. No hints, just pattern recognition.</text>`)

	tileIndex := 0
	for groupIndex, group := range puzzle.Groups {
		fill := colors[groupIndex%len(colors)]
		if group.ColorIndex >= 0 {
			fill = colors[group.ColorIndex%len(colors)]
		}
		for _, tile := range group.Tiles {
			row := tileIndex / 4
			col := tileIndex % 4
			x := gridLeft + col*(tileSize+tileGap)
			y := gridTop + row*(tileSize+tileGap)
			builder.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" rx="14" fill="%s" stroke="#171717" stroke-width="3"/>`, x, y, tileSize, tileSize, fill))
			builder.WriteString(fmt.Sprintf(`<text x="%d" y="%d" text-anchor="middle" dominant-baseline="middle" font-family="Inter,Arial,sans-serif" font-size="18" font-weight="900" fill="#171717">%s</text>`, x+tileSize/2, y+tileSize/2, html.EscapeString(truncateOGText(tile.Text, 11))))
			tileIndex++
		}
	}
	builder.WriteString(`</svg>`)
	return builder.String()
}

func truncateOGText(value string, maxRunes int) string {
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	if maxRunes <= 1 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-1]) + "..."
}

func writeRateLimit(w http.ResponseWriter, message string, retryAfter time.Duration) {
	w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfterSeconds(retryAfter)))
	writeError(w, http.StatusTooManyRequests, message)
}

func retryAfterSeconds(duration time.Duration) int {
	if duration <= 0 {
		return 1
	}
	seconds := int((duration + time.Second - time.Nanosecond) / time.Second)
	if seconds < 1 {
		return 1
	}
	return seconds
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, maxBytes int64, target any, message string) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, message)
		return false
	}

	var trailing struct{}
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, message)
		return false
	}
	return true
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (recorder *statusRecorder) WriteHeader(status int) {
	if recorder.status == 0 {
		recorder.status = status
		recorder.ResponseWriter.WriteHeader(status)
	}
}

func (recorder *statusRecorder) Write(body []byte) (int, error) {
	if recorder.status == 0 {
		recorder.WriteHeader(http.StatusOK)
	}
	return recorder.ResponseWriter.Write(body)
}

func withRequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}

		next.ServeHTTP(recorder, r)

		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		slog.Info(
			"http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", status,
			"duration_ms", time.Since(started).Milliseconds(),
			"client_ip", clientIP(r),
			"user_agent", r.UserAgent(),
		)
	})
}

func withRequestTimeout(next http.Handler, timeout time.Duration) http.Handler {
	return http.TimeoutHandler(next, timeout, `{"ok":false,"error":"Request timed out."}`+"\n")
}

func withCORS(next http.Handler, origins []string) http.Handler {
	if len(origins) == 0 {
		origins = []string{
			"http://localhost:3000",
			"http://localhost:3001",
			"http://localhost:3002",
		}
	}

	allowedOrigins := make(map[string]bool, len(origins))
	for _, origin := range origins {
		allowedOrigins[origin] = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowedOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Vary", "Origin")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy(r.URL.Path))
		next.ServeHTTP(w, r)
	})
}

func contentSecurityPolicy(route string) string {
	if isAPIRoute(route) {
		return "default-src 'none'; frame-ancestors 'none'; base-uri 'none'"
	}
	return strings.Join([]string{
		"default-src 'self'",
		"script-src 'self' 'unsafe-inline'",
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data:",
		"font-src 'self' data:",
		"connect-src 'self'",
		"object-src 'none'",
		"frame-ancestors 'none'",
		"base-uri 'self'",
		"form-action 'self'",
	}, "; ")
}

func isAPIRoute(route string) bool {
	return route == "/healthz" || route == "/readyz" || route == "/metrics" || route == "/api" || strings.HasPrefix(route, "/api/")
}
