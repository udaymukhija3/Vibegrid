package vibegrid

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
)

func (server *Server) handleRobots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	fmt.Fprintf(w,
		"User-agent: *\nAllow: /\nDisallow: /admin\nDisallow: /api/\nDisallow: /crew/\nDisallow: /p/\nDisallow: /claim\nDisallow: /demo\nSitemap: %s/sitemap.xml\n",
		server.publicBaseURL,
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

// handleSitemap contains only product-level public pages. Crew rooms are
// capability links and classic /p links are a compatibility surface, so neither
// belongs in search results.
func (server *Server) handleSitemap(w http.ResponseWriter, r *http.Request) {
	if !server.allowPuzzleRead(w, r) {
		return
	}
	base := server.publicBaseURL
	set := sitemapURLSet{Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9"}
	for _, path := range []string{"/", "/crews", "/privacy", "/terms", "/policy"} {
		set.URLs = append(set.URLs, sitemapURL{Loc: base + path})
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
