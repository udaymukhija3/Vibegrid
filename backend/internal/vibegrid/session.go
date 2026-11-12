package vibegrid

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const sessionCookieName = "vibegrid_session"
const guestCSRFHeader = "X-CSRF-Token"

// sessionTTL is how long an anonymous session cookie lives, measured from the
// most recent request. It matches attempt retention so a returning browser
// never silently starts a second durable trail.
const sessionTTL = 24 * time.Hour * 30

const sessionMaxAgeDays = int(sessionTTL / (24 * time.Hour))

const (
	guestSessionAccessLimit  = 600
	guestSessionAccessWindow = time.Hour
)

type SessionStatus struct {
	Mode  string             `json:"mode"`
	Guest GuestSessionStatus `json:"guest"`
	Admin AdminSessionStatus `json:"admin"`
}

type GuestSessionStatus struct {
	Active     bool   `json:"active"`
	Label      string `json:"label"`
	CookieName string `json:"cookieName"`
	MaxAgeDays int    `json:"maxAgeDays"`
	CSRFToken  string `json:"csrfToken"`
}

type AdminSessionStatus struct {
	Authenticated bool    `json:"authenticated"`
	CookieName    string  `json:"cookieName"`
	ExpiresAt     *string `json:"expiresAt,omitempty"`
}

// EnsureSessionID returns the caller's session id, minting one when the request
// carries no valid cookie.
//
// The cookie is rewritten on every call so its expiry slides forward. Stamping
// it only at mint time gave every browser a hard cutoff sessionTTL after its
// first visit: nothing ever extended the original Expires, so a player with a
// sixty-day streak lost their crews on day thirty no matter how faithfully they
// played. Identity is the whole product here — crew membership, submissions and
// votes all hang off this id — so it has to survive continuous use.
func EnsureSessionID(w http.ResponseWriter, r *http.Request, secure bool) string {
	if cookie, err := r.Cookie(sessionCookieName); err == nil && validSessionID(cookie.Value) {
		writeSessionCookie(w, cookie.Value, secure)
		return cookie.Value
	}

	sessionID := randomSessionID()
	writeSessionCookie(w, sessionID, secure)
	return sessionID
}

// ensureSessionID resolves the cookie bearer token to a stable anonymous
// identity. Existing pre-migration cookies bootstrap with identity == token, so
// current crew memberships keep working. Newly minted cookies use separate
// random values, which lets the bearer rotate without rewriting durable rows.
func (server *Server) ensureSessionID(w http.ResponseWriter, r *http.Request) string {
	identity, _ := server.ensureGuestSession(w, r)
	return identity
}

// ensureGuestSession returns both the stable anonymous identity used by domain
// rows and the active bearer token carried by this browser. Keeping those two
// values explicit prevents security responses such as the CSRF bootstrap from
// accidentally being derived from the durable identity instead of the
// revocable browser secret.
func (server *Server) ensureGuestSession(w http.ResponseWriter, r *http.Request) (identity, token string) {
	if server.guestSessions == nil {
		// Some focused unit tests construct a Server directly around one handler.
		// Production and the full router always go through NewServer and therefore
		// always receive a revocable store.
		legacy := EnsureSessionID(w, r, server.secureCookies)
		return legacy, legacy
	}
	now := server.clock().UTC()
	expiresAt := now.Add(sessionTTL)
	if cookie, err := r.Cookie(sessionCookieName); err == nil && validSessionID(cookie.Value) {
		identity, active, resolveErr := server.guestSessions.ResolveOrCreate(
			r.Context(), hashGuestToken(cookie.Value), cookie.Value, guestIdentityForToken(cookie.Value), now, expiresAt,
		)
		if resolveErr == nil && active {
			writeSessionCookie(w, cookie.Value, server.secureCookies)
			return identity, cookie.Value
		}
		if resolveErr != nil {
			slog.Error("guest session resolution failed", "error", resolveErr, "request_id", requestIDFromContext(r.Context()))
		}
	}

	token = randomSessionID()
	identity = guestIdentityForToken(token)
	resolved, active, err := server.guestSessions.ResolveOrCreate(
		r.Context(), hashGuestToken(token), "", identity, now, expiresAt,
	)
	if err != nil || !active {
		// A database-backed deployment cannot use its durable product while that
		// store is unavailable anyway. Keep public/local surfaces working with a
		// fresh identity, but never honor the possibly revoked incoming bearer.
		slog.Error("guest session registration failed", "error", err, "request_id", requestIDFromContext(r.Context()))
		resolved = identity
	}
	writeSessionCookie(w, token, server.secureCookies)
	// The idempotency wrapper and the handler it wraps may both resolve the
	// session during one request. Replace even a revoked/expired cookie on the
	// in-flight request so the second resolution sees the one token we just
	// issued rather than minting another identity and another Set-Cookie value.
	replaceRequestSessionCookie(r, token)
	return resolved, token
}

func replaceRequestSessionCookie(r *http.Request, token string) {
	cookies := r.Cookies()
	r.Header.Del("Cookie")
	for _, cookie := range cookies {
		if cookie.Name != sessionCookieName {
			r.AddCookie(&http.Cookie{Name: cookie.Name, Value: cookie.Value})
		}
	}
	r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
}

// existingSessionID returns the session the request already carries, if any. It
// never mints one, which is what makes it safe to use for rate limiting: a
// caller that minted an id would hand every cookie-less request its own fresh
// budget, and dropping the cookie would become a way to bypass the limit.
func existingSessionID(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || !validSessionID(cookie.Value) {
		return "", false
	}
	return cookie.Value, true
}

// writeSessionCookie stamps a fresh sessionTTL window on the session cookie.
//
// It is a no-op when this response already carries one, because a handler may
// resolve the session more than once (an idempotency wrapper and the handler it
// wraps both do) and a browser should not receive the same Set-Cookie twice.
//
// Every session-bearing response now carries Set-Cookie, so this also defaults
// the response to private caching. A shared cache must never store one
// browser's session cookie and replay it to the next visitor. Handlers that
// choose their own Cache-Control keep it.
func writeSessionCookie(w http.ResponseWriter, sessionID string, secure bool) {
	header := w.Header()
	for _, existing := range header.Values("Set-Cookie") {
		if strings.HasPrefix(existing, sessionCookieName+"=") {
			return
		}
	}
	if header.Get("Cache-Control") == "" {
		header.Set("Cache-Control", "private, no-store")
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
		Expires:  time.Now().Add(sessionTTL),
	})
}

func (server *Server) handleSessionStatus(w http.ResponseWriter, r *http.Request) {
	if !server.allowGuestSessionAccess(w, r) {
		return
	}
	identity, token := server.ensureGuestSession(w, r)
	admin := AdminSessionStatus{CookieName: adminSessionCookieName}
	if expiresAt, ok := server.adminSessionExpiresAt(r); ok {
		formatted := expiresAt.Format(time.RFC3339)
		admin.Authenticated = true
		admin.ExpiresAt = &formatted
	}

	writeJSON(w, http.StatusOK, SessionStatus{
		Mode: "guest",
		Guest: GuestSessionStatus{
			Active:     true,
			Label:      guestSessionLabel(identity),
			CookieName: sessionCookieName,
			MaxAgeDays: sessionMaxAgeDays,
			CSRFToken:  guestCSRFToken(token),
		},
		Admin: admin,
	})
}

// allowGuestSessionAccess places one non-widened ceiling on the source network
// before an unknown bearer can create a durable row. The ordinary read limiter
// intentionally gives cookie-bearing traffic a wider shared-network budget;
// that is correct for puzzle polling but too generous for identity creation.
func (server *Server) allowGuestSessionAccess(w http.ResponseWriter, r *http.Request) bool {
	network := server.clientIdentity.networkKey(r)
	if network == "" {
		network = server.clientIP(r)
	}
	decision, err := server.checkRateLimit(
		r.Context(), "guest-session:net:"+network,
		guestSessionAccessLimit, guestSessionAccessWindow, server.createLimiter,
	)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "Could not check session limits.")
		return false
	}
	if !decision.allowed {
		writeRateLimit(w, "Too many browser sessions are starting from this network. Try again later.", decision.retryAfter)
		return false
	}
	return true
}

type guestSessionRotationResponse struct {
	OK        bool   `json:"ok"`
	CSRFToken string `json:"csrfToken"`
}

type guestSessionRotationRequest struct {
	ReplacementToken string `json:"replacementToken"`
}

// handleGuestSessionRotate invalidates the current bearer token while keeping
// the anonymous identity—and therefore all crew memberships and history—the
// same. This is the recovery path for a copied cookie or a shared device.
func (server *Server) handleGuestSessionRotate(w http.ResponseWriter, r *http.Request) {
	if !server.allowGuestSessionAccess(w, r) {
		return
	}
	oldToken, ok := existingSessionID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "Guest session required.")
		return
	}
	var request guestSessionRotationRequest
	if !decodeJSONBody(w, r, 1024, &request, "That session rotation request is not valid JSON.") {
		return
	}
	if !validSessionID(request.ReplacementToken) || request.ReplacementToken == oldToken {
		writeError(w, http.StatusUnprocessableEntity, "A fresh guest session token is required.")
		return
	}
	now := server.clock().UTC()
	expiresAt := now.Add(sessionTTL)
	_, err := server.guestSessions.Rotate(
		r.Context(), hashGuestToken(oldToken), hashGuestToken(request.ReplacementToken), now, expiresAt,
	)
	if errors.Is(err, ErrGuestSessionInactive) {
		writeError(w, http.StatusUnauthorized, "That guest session is no longer active.")
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "Could not secure this browser session.")
		return
	}
	// A handler earlier in the chain may have refreshed the old cookie. Replace,
	// rather than append after, that Set-Cookie value.
	w.Header().Del("Set-Cookie")
	writeSessionCookie(w, request.ReplacementToken, server.secureCookies)
	writeJSON(w, http.StatusOK, guestSessionRotationResponse{OK: true, CSRFToken: guestCSRFToken(request.ReplacementToken)})
}

// guestCSRFToken is safe to expose to same-origin JavaScript: it is a one-way
// derivation of the 128-bit HttpOnly session secret, not the secret itself. A
// cross-site page can cause the browser to send the cookie but cannot read this
// response or manufacture the required custom header without passing CORS.
func guestCSRFToken(sessionID string) string {
	digest := sha256.Sum256([]byte("vibegrid.guest.csrf." + sessionID))
	return hex.EncodeToString(digest[:])
}

func (server *Server) requireGuestCSRF(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID, ok := existingSessionID(r)
		provided := strings.TrimSpace(r.Header.Get(guestCSRFHeader))
		if !ok || provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(guestCSRFToken(sessionID))) != 1 {
			writeError(w, http.StatusForbidden, "Refresh the page before changing crew data.")
			return
		}
		next(w, r)
	}
}

func guestSessionLabel(sessionID string) string {
	if len(sessionID) < 6 {
		return "Guest session"
	}
	return "Guest " + sessionID[:6]
}

func randomSessionID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		panic("crypto/rand failed while generating session id: " + err.Error())
	}

	return hex.EncodeToString(bytes)
}

func hashGuestToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

// guestIdentityForToken deterministically gives concurrent tabs that share a
// newly minted document cookie the same stable identity without persisting the
// bearer itself. The domain separator ensures the identity is not the token
// hash used for authentication.
func guestIdentityForToken(token string) string {
	digest := sha256.Sum256([]byte("vibegrid.guest.identity." + token))
	return hex.EncodeToString(digest[:16])
}

func validSessionID(value string) bool {
	if len(value) != 32 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
