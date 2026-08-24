package vibegrid

import (
	"errors"
	"net/http"
	"time"
)

const (
	maxPushBodyBytes      = 4 << 10 // 4 KiB; a subscription is a URL and two keys
	pushWriteRateLimit    = 20
	pushWriteRateWindow   = time.Hour
	pushSubscribeRateName = "push-subscribe:"
)

type pushSubscribeRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

type pushUnsubscribeRequest struct {
	Endpoint string `json:"endpoint"`
}

type pushConfigResponse struct {
	Enabled   bool   `json:"enabled"`
	PublicKey string `json:"publicKey,omitempty"`
}

// pushEnabled reports whether reminders can be served at all. Like crews, push
// is durable and multi-session, and the VAPID keys are deployment config that
// may legitimately be absent — a deploy without them degrades to no reminders
// rather than refusing to boot, which is the lesson this codebase already
// learned the hard way with Turnstile.
func (server *Server) pushEnabled() bool {
	return server.pushSubscriptions != nil && server.vapidKeys.configured()
}

// handlePushConfig tells the browser whether to offer reminders, and hands over
// the public half of the VAPID key it needs to subscribe. The private half
// never leaves the server.
func (server *Server) handlePushConfig(w http.ResponseWriter, r *http.Request) {
	if !server.allowPuzzleRead(w, r) {
		return
	}
	if !server.pushEnabled() {
		w.Header().Set("Cache-Control", "private, no-store")
		writeJSON(w, http.StatusOK, pushConfigResponse{Enabled: false})
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, pushConfigResponse{Enabled: true, PublicKey: server.vapidKeys.Public})
}

func (server *Server) handlePushSubscribe(w http.ResponseWriter, r *http.Request) {
	if !server.pushEnabled() {
		writeError(w, http.StatusServiceUnavailable, "Reminders are not available.")
		return
	}
	if !server.allowCrewWrite(w, r, pushSubscribeRateName, pushWriteRateLimit, pushWriteRateWindow,
		"You're changing reminders too quickly. Try again later.") {
		return
	}
	sessionID := EnsureSessionID(w, r, server.secureCookies)

	var request pushSubscribeRequest
	if !decodeJSONBody(w, r, maxPushBodyBytes, &request, "That subscription is not valid JSON.") {
		return
	}
	subscription := PushSubscription{
		SessionID: sessionID,
		Endpoint:  request.Endpoint,
		P256dh:    request.Keys.P256dh,
		Auth:      request.Keys.Auth,
	}
	if err := server.pushSubscriptions.SaveSubscription(r.Context(), subscription, server.clock()); err != nil {
		if errors.Is(err, ErrPushSubscriptionInvalid) {
			writeError(w, http.StatusUnprocessableEntity, "That subscription is not valid.")
			return
		}
		writeError(w, http.StatusInternalServerError, "Could not save that reminder.")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusCreated, map[string]bool{"subscribed": true})
}

// handlePushUnsubscribe forgets one endpoint. It deliberately does not require
// the endpoint to belong to the caller's session: the endpoint is an opaque,
// high-entropy secret issued by the push service to that browser, and a person
// whose cookie has rotated must still be able to turn their own reminders off.
func (server *Server) handlePushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	if !server.pushEnabled() {
		writeError(w, http.StatusServiceUnavailable, "Reminders are not available.")
		return
	}
	if !server.allowCrewWrite(w, r, pushSubscribeRateName, pushWriteRateLimit, pushWriteRateWindow,
		"You're changing reminders too quickly. Try again later.") {
		return
	}
	var request pushUnsubscribeRequest
	if !decodeJSONBody(w, r, maxPushBodyBytes, &request, "That subscription is not valid JSON.") {
		return
	}
	if err := validatePushEndpoint(request.Endpoint); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "That subscription is not valid.")
		return
	}
	if err := server.pushSubscriptions.DeleteSubscription(r.Context(), request.Endpoint); err != nil {
		writeError(w, http.StatusInternalServerError, "Could not remove that reminder.")
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, map[string]bool{"subscribed": false})
}
