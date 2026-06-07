package vibegrid

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRobotsTxtPointsAtSitemap(t *testing.T) {
	handler := NewServer(ServerConfig{Puzzles: StaticPuzzleSource(SeedPuzzles())})

	rec := seoRequest(handler, "/robots.txt")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("unexpected content type %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Sitemap: https://vibegrid.example/sitemap.xml") {
		t.Fatalf("robots.txt missing sitemap line:\n%s", body)
	}
	if !strings.Contains(body, "Disallow: /admin") {
		t.Fatalf("robots.txt should disallow /admin:\n%s", body)
	}
}

func TestSitemapListsPublishedPuzzlesOnly(t *testing.T) {
	handler := NewServer(ServerConfig{Puzzles: StaticPuzzleSource(SeedPuzzles()), Clock: fixedClock})

	rec := seoRequest(handler, "/sitemap.xml")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/xml") {
		t.Fatalf("unexpected content type %q", ct)
	}

	var set struct {
		URLs []struct {
			Loc string `xml:"loc"`
		} `xml:"url"`
	}
	if err := xml.Unmarshal(rec.Body.Bytes(), &set); err != nil {
		t.Fatalf("sitemap is not valid XML: %v", err)
	}

	locs := map[string]bool{}
	puzzleURLs := 0
	for _, entry := range set.URLs {
		locs[entry.Loc] = true
		if strings.Contains(entry.Loc, "/p/") {
			puzzleURLs++
		}
	}

	for _, want := range []string{
		"https://vibegrid.example/",
		"https://vibegrid.example/archive",
		"https://vibegrid.example/p/vibegrid-2026-06-02",
	} {
		if !locs[want] {
			t.Fatalf("sitemap missing %q; got %v", want, locs)
		}
	}
	// As of the fixed clock (2026-06-02) only one editorial puzzle is live; the
	// future-dated seed puzzles must not leak into the sitemap.
	if puzzleURLs != 1 {
		t.Fatalf("expected exactly one published puzzle URL, got %d", puzzleURLs)
	}
}

func seoRequest(handler http.Handler, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Host = "vibegrid.example"
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
