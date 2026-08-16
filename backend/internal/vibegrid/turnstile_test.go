package vibegrid

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTurnstileVerifierValidatesProviderResponse(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.Form.Get("secret") != "server-secret" || r.Form.Get("response") != "browser-token" {
			t.Fatalf("unexpected credentials: %#v", r.Form)
		}
		if r.Form.Get("remoteip") != "198.51.100.9" {
			t.Fatalf("remote ip missing: %#v", r.Form)
		}
		return jsonResponse(`{"success":true,"action":"community_create","hostname":"vibegrid.example"}`), nil
	})}

	verifier := NewTurnstileVerifier("server-secret", "vibegrid.example")
	verifier.endpoint = "https://provider.invalid/siteverify"
	verifier.client = client
	if err := verifier.Verify(context.Background(), "browser-token", "198.51.100.9", "community_create"); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestTurnstileVerifierRejectsMismatchedBinding(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(`{"success":true,"action":"report_create","hostname":"attacker.example"}`), nil
	})}

	verifier := NewTurnstileVerifier("server-secret", "vibegrid.example")
	verifier.endpoint = "https://provider.invalid/siteverify"
	verifier.client = client
	if err := verifier.Verify(context.Background(), "browser-token", "", "community_create"); err == nil {
		t.Fatal("expected mismatched action/hostname to be rejected")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

type rejectingBotVerifier struct{}

func (rejectingBotVerifier) Verify(context.Context, string, string, string) error {
	return ErrBotVerificationRejected
}

func TestCommunityCreateFailsClosedWhenBotVerificationRejects(t *testing.T) {
	handler := NewServer(ServerConfig{
		Puzzles:       StaticPuzzleSource(SeedPuzzles()),
		Community:     fakeCommunityStore{},
		BotVerifier:   rejectingBotVerifier{},
		Clock:         fixedClock,
		SecureCookies: true,
	})

	body, err := json.Marshal(validPuzzleInput())
	if err != nil {
		t.Fatalf("marshal puzzle: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/community/puzzles", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "turnstile-rejection-1")
	request.Header.Set(turnstileTokenHeader, "rejected-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", response.Code, response.Body.String())
	}
}
