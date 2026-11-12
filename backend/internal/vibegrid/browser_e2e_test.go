package vibegrid

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vibegrid/vibegrid/backend/internal/frontend"
)

// The clock control exists only in this test binary. All crew actions still
// pass through the shipped frontend, HTTP handlers, and real Postgres stores.
func TestBrowserCrewLifecycle(t *testing.T) {
	databaseURL := os.Getenv("TEST_BROWSER_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("run npm run test:e2e for the isolated browser lifecycle suite")
	}
	t.Setenv("TEST_DATABASE_URL", databaseURL)
	var day atomic.Int64
	start := time.Now().UTC().Truncate(24 * time.Hour).Add(12 * time.Hour)
	handler, _ := hostileServer(t, func(config *ServerConfig, _ *sql.DB) {
		config.Clock = func() time.Time { return start.AddDate(0, 0, int(day.Load())) }
		config.TimeZone = "UTC"
		config.Frontend = frontend.NewHandler(frontend.Embedded())
	})
	token := newVibeObjectID("clock")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__test/next-day" {
			if r.Method != http.MethodPost || r.Header.Get("X-Test-Clock") != token {
				http.NotFound(w, r)
				return
			}
			day.Add(1)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "node", "scripts/browser-e2e.mjs", server.URL, token)
	command.Dir = root
	output, err := command.CombinedOutput()
	t.Log(string(output))
	if err != nil {
		t.Fatalf("browser lifecycle: %v", err)
	}
}
