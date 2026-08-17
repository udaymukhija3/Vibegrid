package frontend

import (
	"bytes"
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed all:out
var embeddedFiles embed.FS

func Embedded() fs.FS {
	files, err := fs.Sub(embeddedFiles, "out")
	if err != nil {
		return emptyFS{}
	}
	return files
}

type emptyFS struct{}

func (emptyFS) Open(string) (fs.File, error) {
	return nil, fs.ErrNotExist
}

type Handler struct {
	files fs.FS
}

func NewHandler(files fs.FS) http.Handler {
	return &Handler{files: files}
}

func (handler *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cleanPath := path.Clean("/" + r.URL.Path)
	if strings.HasPrefix(cleanPath, "/api/") || cleanPath == "/api" {
		http.NotFound(w, r)
		return
	}

	target := handler.resolve(cleanPath)
	if target == "" {
		handler.serveNotFound(w, r)
		return
	}

	handler.serveFile(w, r, target)
}

// dynamicRoutes maps each client-side dynamic route onto the static shell Next
// exported for it. Production is a static export, so /p/abc, /demo/xyz and
// /crew/123 have no file of their own: each falls back to one placeholder
// document that reads the real id out of the URL at runtime.
var dynamicRoutes = []struct {
	prefix     string
	candidates []string
}{
	{"/p/", []string{"p/__share__/index.html", "p/__share__.html", "p/index.html"}},
	{"/demo/", []string{"demo/__room__/index.html", "demo/__room__.html", "demo/index.html"}},
	{"/crew/", []string{"crew/__crew__/index.html", "crew/__crew__.html", "crew/index.html"}},
}

func (handler *Handler) resolve(cleanPath string) string {
	for _, route := range dynamicRoutes {
		if !isDynamicRoutePath(cleanPath, route.prefix) {
			continue
		}
		for _, candidate := range route.candidates {
			if handler.exists(candidate) {
				return candidate
			}
		}
	}

	if cleanPath == "/" {
		if handler.exists("index.html") {
			return "index.html"
		}
		return ""
	}

	candidate := strings.TrimPrefix(cleanPath, "/")
	for _, name := range []string{candidate, path.Join(candidate, "index.html"), candidate + ".html"} {
		if handler.exists(name) {
			return name
		}
	}

	return ""
}

func (handler *Handler) serveNotFound(w http.ResponseWriter, r *http.Request) {
	if handler.exists("404.html") {
		handler.serveFileWithStatus(w, r, "404.html", http.StatusNotFound)
		return
	}
	http.NotFound(w, r)
}

func (handler *Handler) serveFile(w http.ResponseWriter, r *http.Request, name string) {
	handler.serveFileWithStatus(w, r, name, http.StatusOK)
}

func (handler *Handler) serveFileWithStatus(w http.ResponseWriter, r *http.Request, name string, status int) {
	data, err := fs.ReadFile(handler.files, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Cache-Control", cacheControlFor(name))
	if status != http.StatusOK {
		w.WriteHeader(status)
		if r.Method != http.MethodHead {
			_, _ = w.Write(data)
		}
		return
	}
	http.ServeContent(w, r, path.Base(name), modTime(handler.files, name), bytes.NewReader(data))
}

func (handler *Handler) exists(name string) bool {
	if name == "" || !fs.ValidPath(name) {
		return false
	}
	info, err := fs.Stat(handler.files, name)
	return err == nil && !info.IsDir()
}

// isDynamicRoutePath matches "/prefix/<id>" but not the bare "/prefix/", which
// carries no id and should fall through to a real file or the 404 page.
func isDynamicRoutePath(cleanPath, prefix string) bool {
	return strings.HasPrefix(cleanPath, prefix) && cleanPath != prefix
}

func cacheControlFor(name string) string {
	if strings.HasPrefix(name, "_next/static/") {
		return "public, max-age=31536000, immutable"
	}
	if isStaticAsset(name) {
		return "public, max-age=3600"
	}
	return "no-cache"
}

func isStaticAsset(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".avif", ".css", ".gif", ".ico", ".jpg", ".jpeg", ".js", ".json", ".png", ".svg", ".txt", ".webp", ".woff", ".woff2":
		return true
	default:
		return false
	}
}

func modTime(files fs.FS, name string) time.Time {
	info, err := fs.Stat(files, name)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}
