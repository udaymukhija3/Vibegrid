package vibegrid

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func postClientError(handler http.Handler, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/client-errors", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestClientErrorReportIsAcceptedAndBounded(t *testing.T) {
	handler := NewServer(ServerConfig{Clock: fixedClock})

	accepted := postClientError(handler, `{"message":"TypeError: x is undefined","stack":"at play","url":"https://vibegrid.example/p/abc"}`)
	if accepted.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for a valid report, got %d: %s", accepted.Code, accepted.Body.String())
	}

	missingMessage := postClientError(handler, `{"stack":"at play"}`)
	if missingMessage.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without a message, got %d", missingMessage.Code)
	}

	unknownField := postClientError(handler, `{"message":"x","extra":"y"}`)
	if unknownField.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown fields, got %d", unknownField.Code)
	}

	oversized := postClientError(handler, `{"message":"`+strings.Repeat("a", int(maxClientErrorBodyBytes))+`"}`)
	if oversized.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for an oversized report, got %d", oversized.Code)
	}

	noContentType := httptest.NewRequest(http.MethodPost, "/api/client-errors", strings.NewReader(`{"message":"x"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, noContentType)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415 without a JSON content type, got %d", rec.Code)
	}
}

func TestClientErrorReportsAreRateLimitedAndFailClosed(t *testing.T) {
	handler := NewServer(ServerConfig{Clock: fixedClock})
	// One report was not consumed above (fresh server); exhaust the budget.
	for i := 0; i < clientErrorRateLimit; i++ {
		postClientError(handler, `{"message":"boom"}`)
	}
	limited := postClientError(handler, `{"message":"boom"}`)
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after %d reports, got %d", clientErrorRateLimit, limited.Code)
	}

	failing := NewServer(ServerConfig{RateLimits: failingRateLimitStore{}, Clock: fixedClock})
	closed := postClientError(failing, `{"message":"boom"}`)
	if closed.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when the shared limiter errors, got %d", closed.Code)
	}
}
