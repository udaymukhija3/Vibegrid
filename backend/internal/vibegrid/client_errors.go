package vibegrid

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Browser-reported errors land in the structured server log, so frontend
// breakage is visible in the platform log stream without a third-party error
// tracker. Reports are best-effort diagnostics: they are logged, never stored,
// and never echoed back.
const (
	maxClientErrorBodyBytes = 8 << 10 // 8 KiB
	maxClientErrorField     = 2000

	// Unauthenticated public write: a tight per-IP cap keeps a hostile client
	// from turning the log into a firehose while still catching real breakage
	// (a genuinely broken page produces a handful of distinct errors, not 20+).
	clientErrorRateLimit  = 20
	clientErrorRateWindow = time.Hour
)

type ClientErrorReport struct {
	Message string `json:"message"`
	Stack   string `json:"stack,omitempty"`
	URL     string `json:"url,omitempty"`
}

func (server *Server) handleClientError(w http.ResponseWriter, r *http.Request) {
	// Fail closed like the other anonymous writes: an unavailable shared limiter
	// must not turn this into an unlimited log-write path.
	decision, err := server.checkRateLimit(r.Context(), "client-error:"+server.clientIP(r), clientErrorRateLimit, clientErrorRateWindow, server.clientErrorLimiter)
	if err != nil {
		slog.Error("client-error rate-limit check failed", "error", err)
		writeError(w, http.StatusServiceUnavailable, "Could not check request limits.")
		return
	}
	if !decision.allowed {
		writeRateLimit(w, "Too many error reports from this address.", decision.retryAfter)
		return
	}

	var report ClientErrorReport
	if !decodeJSONBody(w, r, maxClientErrorBodyBytes, &report, "That error report is not valid.") {
		return
	}
	if strings.TrimSpace(report.Message) == "" {
		writeError(w, http.StatusBadRequest, "An error message is required.")
		return
	}

	slog.Warn("client error report",
		"message", truncateRunes(report.Message, maxClientErrorField),
		"url", truncateRunes(report.URL, maxClientErrorField),
		"stack", truncateRunes(report.Stack, maxClientErrorField),
		"user_agent", r.UserAgent(),
		"request_id", requestIDFromContext(r.Context()),
	)
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

func truncateRunes(value string, maxRunes int) string {
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}
