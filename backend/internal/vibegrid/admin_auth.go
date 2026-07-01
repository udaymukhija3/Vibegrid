package vibegrid

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const adminSessionCookieName = "vibegrid_admin"
const adminSessionDuration = 12 * time.Hour
const adminCSRFHeader = "X-CSRF-Token"

type adminSessionRequest struct {
	Password string `json:"password"`
}

type adminSessionResponse struct {
	OK        bool   `json:"ok"`
	CSRFToken string `json:"csrfToken,omitempty"`
}

func (server *Server) handleAdminSession(w http.ResponseWriter, r *http.Request) {
	// Throttle password attempts per IP. Fail closed if the backing store is
	// unavailable: otherwise an outage turns the password endpoint into an
	// unbounded online guessing target.
	if decision, err := server.checkRateLimit(r.Context(), "admin-login:"+server.clientIP(r), adminLoginRateLimit, adminLoginRateWindow, server.loginLimiter); err != nil {
		slog.Error("admin login rate-limit check failed", "error", err)
		writeError(w, http.StatusServiceUnavailable, "Could not check request limits.")
		return
	} else if !decision.allowed {
		writeRateLimit(w, "Too many login attempts. Wait a minute and try again.", decision.retryAfter)
		return
	}

	var request adminSessionRequest
	if !decodeJSONBody(w, r, maxAdminBodyBytes, &request, "That login payload is not valid JSON.") {
		return
	}
	if server.adminPassword == "" || server.adminSessionSecret == "" || server.adminSessions == nil {
		writeError(w, http.StatusServiceUnavailable, "Admin login is not configured.")
		return
	}
	if subtle.ConstantTimeCompare([]byte(request.Password), []byte(server.adminPassword)) != 1 {
		writeError(w, http.StatusUnauthorized, "Admin password is incorrect.")
		return
	}

	expiresAt := server.clock().Add(adminSessionDuration)
	sessionValue := newAdminSessionToken()
	if err := server.adminSessions.Create(r.Context(), hashAdminSessionToken(sessionValue), expiresAt); err != nil {
		slog.Error("create admin session failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Could not create admin session.")
		return
	}
	http.SetCookie(w, adminCookie(sessionValue, expiresAt, expiresAt.Sub(server.clock()), server.secureCookies))
	writeJSON(w, http.StatusOK, adminSessionResponse{
		OK:        true,
		CSRFToken: server.adminCSRFToken(sessionValue),
	})
}

func (server *Server) handleAdminSessionStatus(w http.ResponseWriter, r *http.Request) {
	if sessionValue, _, ok := server.adminSessionValue(r); ok {
		writeJSON(w, http.StatusOK, adminSessionResponse{
			OK:        true,
			CSRFToken: server.adminCSRFToken(sessionValue),
		})
		return
	}
	if server.validAdminBearerToken(r) {
		writeJSON(w, http.StatusOK, adminSessionResponse{OK: true})
		return
	}
	writeError(w, http.StatusUnauthorized, "Admin authorization required.")
}

func (server *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(adminSessionCookieName); err == nil && cookie.Value != "" && server.adminSessions != nil {
		if err := server.adminSessions.Revoke(r.Context(), hashAdminSessionToken(cookie.Value), server.clock()); err != nil {
			slog.Error("revoke admin session failed", "error", err)
			writeError(w, http.StatusInternalServerError, "Could not end admin session.")
			return
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   server.secureCookies,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (server *Server) isAdminRequest(r *http.Request) bool {
	if server.adminPassword != "" && server.adminSessionSecret != "" && server.adminSessions != nil && server.validAdminSession(r) {
		return true
	}
	return server.validAdminBearerToken(r)
}

func (server *Server) validAdminBearerToken(r *http.Request) bool {
	if server.adminToken == "" {
		return false
	}
	token := bearerToken(r)
	return token != "" && subtle.ConstantTimeCompare([]byte(token), []byte(server.adminToken)) == 1
}

func (server *Server) validAdminSession(r *http.Request) bool {
	_, ok := server.adminSessionExpiresAt(r)
	return ok
}

func (server *Server) adminSessionExpiresAt(r *http.Request) (time.Time, bool) {
	_, expiresAt, ok := server.adminSessionValue(r)
	return expiresAt, ok
}

func (server *Server) adminSessionValue(r *http.Request) (string, time.Time, bool) {
	if server.adminPassword == "" || server.adminSessionSecret == "" {
		return "", time.Time{}, false
	}
	cookie, err := r.Cookie(adminSessionCookieName)
	if err != nil {
		return "", time.Time{}, false
	}
	expiresAt, ok, err := server.adminSessions.Active(r.Context(), hashAdminSessionToken(cookie.Value), server.clock())
	if err != nil {
		slog.Error("load admin session failed", "error", err)
		return "", time.Time{}, false
	}
	if !ok {
		return "", time.Time{}, false
	}
	return cookie.Value, expiresAt, true
}

func adminCookie(value string, expiresAt time.Time, maxAge time.Duration, secure bool) *http.Cookie {
	maxAgeSeconds := int(maxAge.Seconds())
	if maxAgeSeconds < 1 {
		maxAgeSeconds = 1
	}
	return &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    value,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   maxAgeSeconds,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	}
}

func (server *Server) validAdminCSRFToken(r *http.Request) bool {
	sessionValue, _, ok := server.adminSessionValue(r)
	if !ok {
		return false
	}
	token := strings.TrimSpace(r.Header.Get(adminCSRFHeader))
	if token == "" {
		return false
	}
	expected := server.adminCSRFToken(sessionValue)
	return subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}

func (server *Server) adminCSRFToken(sessionValue string) string {
	mac := hmac.New(sha256.New, []byte(server.adminSessionSecret))
	_, _ = mac.Write([]byte("csrf."))
	_, _ = mac.Write([]byte(sessionValue))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (server *Server) adminActor(r *http.Request) string {
	if bearerToken(r) != "" {
		return "admin-token"
	}
	return "admin-session:" + server.clientIP(r)
}
