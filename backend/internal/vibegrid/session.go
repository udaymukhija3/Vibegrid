package vibegrid

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"
)

const sessionCookieName = "vibegrid_session"

// sessionTTL is how long an anonymous session cookie lives. Long enough that a
// returning player keeps their attempt history and streak across days.
const sessionTTL = 24 * time.Hour * 183

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

func EnsureSessionID(w http.ResponseWriter, r *http.Request, secure bool) string {
	if cookie, err := r.Cookie(sessionCookieName); err == nil && validSessionID(cookie.Value) {
		return cookie.Value
	}

	sessionID := randomSessionID()
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

	return sessionID
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
