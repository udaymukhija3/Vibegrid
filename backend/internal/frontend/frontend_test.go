package frontend

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestHandlerServesSharedPuzzleFallback(t *testing.T) {
	handler := NewHandler(fstest.MapFS{
		"p/__share__/index.html": {Data: []byte("<html>share shell</html>")},
	})

	request := httptest.NewRequest(http.MethodGet, "/p/community-123", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if response.Body.String() != "<html>share shell</html>" {
		t.Fatalf("unexpected body: %q", response.Body.String())
	}
	if got := response.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("expected html no-cache, got %q", got)
	}
}

func TestHandlerServesDemoRoomFallback(t *testing.T) {
	handler := NewHandler(fstest.MapFS{
		"demo/__room__/index.html": {Data: []byte("<html>demo shell</html>")},
	})

	request := httptest.NewRequest(http.MethodGet, "/demo/investor-01", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if response.Body.String() != "<html>demo shell</html>" {
		t.Fatalf("unexpected body: %q", response.Body.String())
	}
}

func TestHandlerServesStaticAssetWithImmutableCache(t *testing.T) {
	handler := NewHandler(fstest.MapFS{
		"_next/static/chunks/app.js": {Data: []byte("console.log('ok')")},
	})

	request := httptest.NewRequest(http.MethodGet, "/_next/static/chunks/app.js", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	if got := response.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("unexpected cache header: %q", got)
	}
}
