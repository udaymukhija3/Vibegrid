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

func (handler *Handler) resolve(cleanPath string) string {
	if isSharedPuzzlePath(cleanPath) {
		for _, candidate := range []string{"p/__share__/index.html", "p/__share__.html", "p/index.html"} {
			if handler.exists(candidate) {
				return candidate
			}
		}
	}

	if isDemoRoomPath(cleanPath) {
		for _, candidate := range []string{"demo/__room__/index.html", "demo/__room__.html", "demo/index.html"} {
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

func isSharedPuzzlePath(cleanPath string) bool {
	return strings.HasPrefix(cleanPath, "/p/") && cleanPath != "/p/"
}

func isDemoRoomPath(cleanPath string) bool {
	return strings.HasPrefix(cleanPath, "/demo/") && cleanPath != "/demo/"
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
