package vibegrid

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"time"
)

const sessionCookieName = "vibegrid_session"

// sessionTTL is how long an anonymous session cookie lives, measured from the
// most recent request. It matches attempt retention so a returning browser
// never silently starts a second durable trail.
const sessionTTL = 24 * time.Hour * 30

const sessionMaxAgeDays = int(sessionTTL / (24 * time.Hour))

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
	sessionID := EnsureSessionID(w, r, server.secureCookies)
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
			Label:      guestSessionLabel(sessionID),
			CookieName: sessionCookieName,
			MaxAgeDays: sessionMaxAgeDays,
		},
		Admin: admin,
	})
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

func validSessionID(value string) bool {
	if len(value) != 32 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
