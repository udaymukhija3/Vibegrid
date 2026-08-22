package vibegrid

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRobotsTxtPointsAtSitemap(t *testing.T) {
	handler := NewServer(ServerConfig{Puzzles: StaticPuzzleSource(SeedPuzzles()), PublicBaseURL: "https://vibegrid.example"})

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
	for _, private := range []string{"/crew/", "/p/", "/claim"} {
		if !strings.Contains(body, "Disallow: "+private) {
			t.Fatalf("robots.txt should disallow %s:\n%s", private, body)
		}
	}
}

func TestSitemapListsOnlyPublicProductPages(t *testing.T) {
	handler := NewServer(ServerConfig{Puzzles: StaticPuzzleSource(SeedPuzzles()), Clock: fixedClock, PublicBaseURL: "https://vibegrid.example"})

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
	for _, entry := range set.URLs {
		locs[entry.Loc] = true
	}

	for _, want := range []string{
		"https://vibegrid.example/",
		"https://vibegrid.example/crews",
		"https://vibegrid.example/privacy",
	} {
		if !locs[want] {
			t.Fatalf("sitemap missing %q; got %v", want, locs)
		}
	}
	for _, private := range []string{"/p/", "/crew/", "/admin", "/archive", "/create"} {
		for location := range locs {
			if strings.Contains(location, private) {
				t.Fatalf("private or compatibility route %q leaked into sitemap", location)
			}
		}
	}
}

func seoRequest(handler http.Handler, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
