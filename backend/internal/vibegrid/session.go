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
