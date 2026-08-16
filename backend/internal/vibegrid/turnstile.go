package vibegrid

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrBotVerificationRejected = errors.New("bot verification rejected")

const (
	turnstileTokenHeader = "X-VibeGrid-Turnstile"
	maxTurnstileTokenLen = 2048
	turnstileSiteverify  = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
)

// BotVerifier is the server-side trust boundary for public UGC mutations.
type BotVerifier interface {
	Verify(ctx context.Context, token, remoteIP, expectedAction string) error
}

type TurnstileVerifier struct {
	secret           string
	expectedHostname string
	endpoint         string
	client           *http.Client
}

type turnstileResponse struct {
	Success    bool     `json:"success"`
	Action     string   `json:"action"`
	Hostname   string   `json:"hostname"`
	ErrorCodes []string `json:"error-codes"`
}

func NewTurnstileVerifier(secret, expectedHostname string) *TurnstileVerifier {
	return &TurnstileVerifier{
		secret:           strings.TrimSpace(secret),
		expectedHostname: strings.ToLower(strings.TrimSpace(expectedHostname)),
		endpoint:         turnstileSiteverify,
		client:           &http.Client{Timeout: 3 * time.Second},
	}
}

func (verifier *TurnstileVerifier) Verify(ctx context.Context, token, remoteIP, expectedAction string) error {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > maxTurnstileTokenLen {
		return fmt.Errorf("%w: missing or invalid token", ErrBotVerificationRejected)
	}
	form := url.Values{"secret": {verifier.secret}, "response": {token}}
	if remoteIP = strings.TrimSpace(remoteIP); remoteIP != "" {
		form.Set("remoteip", remoteIP)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, verifier.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("create Turnstile request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := verifier.client.Do(req)
	if err != nil {
		return fmt.Errorf("verify Turnstile token: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Turnstile siteverify returned %s", response.Status)
	}
	var result turnstileResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode Turnstile response: %w", err)
	}
	if !result.Success {
		return fmt.Errorf("%w: provider rejected token", ErrBotVerificationRejected)
	}
	if result.Action != expectedAction {
		return fmt.Errorf("%w: action did not match", ErrBotVerificationRejected)
	}
	if verifier.expectedHostname != "" && !strings.EqualFold(result.Hostname, verifier.expectedHostname) {
		return fmt.Errorf("%w: hostname did not match", ErrBotVerificationRejected)
	}
	return nil
}

func (server *Server) verifyBot(w http.ResponseWriter, r *http.Request, action string) bool {
	if server.botVerifier == nil {
		return true
	}
	token := r.Header.Get(turnstileTokenHeader)
	if strings.TrimSpace(token) == "" || len(token) > maxTurnstileTokenLen {
		writeError(w, http.StatusUnprocessableEntity, "Complete the bot check and try again.")
		return false
	}
	if err := server.botVerifier.Verify(r.Context(), token, server.clientIP(r), action); err != nil {
		if errors.Is(err, ErrBotVerificationRejected) {
			writeError(w, http.StatusUnprocessableEntity, "Bot check failed or expired. Complete it again.")
			return false
		}
		slog.Error("Turnstile verification unavailable", "action", action, "error", err)
		writeError(w, http.StatusServiceUnavailable, "Bot verification is temporarily unavailable. Try again shortly.")
		return false
	}
	return true
}
