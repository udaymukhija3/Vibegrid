package vibegrid

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// sitemapMaxURLs caps how many puzzle URLs the sitemap lists. The sitemap spec
// allows up to 50k per file; a daily puzzle will not approach that for years,
// and the cap keeps the build bounded.
const sitemapMaxURLs = 5000

func (server *Server) handleRobots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	fmt.Fprintf(w,
		"User-agent: *\nAllow: /\nDisallow: /admin\nDisallow: /api/\nSitemap: %s/sitemap.xml\n",
		requestBaseURL(r),
	)
}

type sitemapURL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	Xmlns   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

// handleSitemap builds the sitemap from the live data model: the static pages
// plus every editorial puzzle that is published on or before today. Community
// puzzles are link-only by design and deliberately stay out of search indexes,
// and future-dated puzzles are excluded because PublishedPuzzles filters them.
func (server *Server) handleSitemap(w http.ResponseWriter, r *http.Request) {
	base := requestBaseURL(r)
	set := sitemapURLSet{Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9"}
	for _, path := range []string{"/", "/archive", "/create", "/privacy", "/terms", "/policy"} {
		set.URLs = append(set.URLs, sitemapURL{Loc: base + path})
	}

	puzzles, err := server.puzzles.PublishedPuzzles(r.Context(), server.todayString(), sitemapMaxURLs, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not build the sitemap.")
		return
	}
	for _, puzzle := range puzzles {
		set.URLs = append(set.URLs, sitemapURL{
			Loc:     base + "/p/" + url.PathEscape(puzzle.ID),
			LastMod: puzzle.PublishDate,
		})
	}

	body, err := xml.Marshal(set)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Could not build the sitemap.")
		return
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = io.WriteString(w, xml.Header)
	_, _ = w.Write(body)
}
