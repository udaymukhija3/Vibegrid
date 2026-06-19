package vibegrid

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
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
	// Throttle password attempts per IP. Fail open: a rate-limit backend hiccup
	// must never lock the only admin out of their own console.
	if decision, err := server.checkRateLimit(r.Context(), "admin-login:"+clientIP(r), adminLoginRateLimit, adminLoginRateWindow, server.loginLimiter); err != nil {
		slog.Warn("admin login rate-limit check failed; allowing", "error", err)
	} else if !decision.allowed {
		writeRateLimit(w, "Too many login attempts. Wait a minute and try again.", decision.retryAfter)
		return
	}

	var request adminSessionRequest
	if !decodeJSONBody(w, r, maxAdminBodyBytes, &request, "That login payload is not valid JSON.") {
		return
	}
	if server.adminPassword == "" || server.adminSessionSecret == "" {
		writeError(w, http.StatusServiceUnavailable, "Admin login is not configured.")
		return
	}
	if subtle.ConstantTimeCompare([]byte(request.Password), []byte(server.adminPassword)) != 1 {
		writeError(w, http.StatusUnauthorized, "Admin password is incorrect.")
		return
	}

	expiresAt := server.clock().Add(adminSessionDuration)
	sessionValue := signedAdminSession(expiresAt, server.adminSessionSecret)
	http.SetCookie(w, adminCookie(sessionValue, expiresAt, server.secureCookies))
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

func (server *Server) handleAdminLogout(w http.ResponseWriter, _ *http.Request) {
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
	if server.adminPassword != "" && server.adminSessionSecret != "" && server.validAdminSession(r) {
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
	expiresAt, ok := verifyAdminSession(cookie.Value, server.adminSessionSecret)
	if !ok || !server.clock().Before(expiresAt) {
		return "", time.Time{}, false
	}
	return cookie.Value, expiresAt, true
}

func adminCookie(value string, expiresAt time.Time, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    value,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	}
}

func signedAdminSession(expiresAt time.Time, secret string) string {
	expires := strconv.FormatInt(expiresAt.UTC().Unix(), 10)
	signature := adminSignature(expires, secret)
	return expires + "." + signature
}

func verifyAdminSession(value, secret string) (time.Time, bool) {
	expires, signature, ok := strings.Cut(value, ".")
	if !ok || expires == "" || signature == "" {
		return time.Time{}, false
	}
	expected := adminSignature(expires, secret)
	if subtle.ConstantTimeCompare([]byte(signature), []byte(expected)) != 1 {
		return time.Time{}, false
	}
	unix, err := strconv.ParseInt(expires, 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(unix, 0).UTC(), true
}

func adminSignature(expires, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(expires))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
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

func adminActor(r *http.Request) string {
	if bearerToken(r) != "" {
		return "admin-token"
	}
	return fmt.Sprintf("admin-session:%s", clientIP(r))
}
