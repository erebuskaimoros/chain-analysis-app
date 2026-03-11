package ui

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

//go:embed all:dist
var embeddedDist embed.FS

func NewHandler(distDir string, preferFilesystem bool) http.Handler {
	if preferFilesystem && strings.TrimSpace(distDir) != "" {
		if stat, err := os.Stat(distDir); err == nil && stat.IsDir() {
			return spaHandler(http.Dir(distDir))
		}
	}
	if sub, err := fs.Sub(embeddedDist, "dist"); err == nil {
		return spaHandler(http.FS(sub))
	}
	if strings.TrimSpace(distDir) != "" {
		if stat, err := os.Stat(distDir); err == nil && stat.IsDir() {
			return spaHandler(http.Dir(distDir))
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "ui bundle unavailable", http.StatusServiceUnavailable)
	})
}

func spaHandler(filesystem http.FileSystem) http.Handler {
	fileServer := http.FileServer(filesystem)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleanPath := path.Clean("/" + strings.TrimSpace(r.URL.Path))
		if cleanPath == "/" {
			serveIndex(filesystem, fileServer, w, r)
			return
		}
		if strings.HasPrefix(cleanPath, "/api/") || strings.HasPrefix(cleanPath, "/legacy") || strings.HasPrefix(cleanPath, "/static/") {
			http.NotFound(w, r)
			return
		}
		if file, err := filesystem.Open(strings.TrimPrefix(cleanPath, "/")); err == nil {
			_ = file.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		serveIndex(filesystem, fileServer, w, r)
	})
}

func serveIndex(filesystem http.FileSystem, fileServer http.Handler, w http.ResponseWriter, r *http.Request) {
	file, err := filesystem.Open("index.html")
	if err != nil {
		http.Error(w, "ui bundle unavailable", http.StatusServiceUnavailable)
		return
	}
	defer file.Close()

	readSeeker, ok := file.(io.ReadSeeker)
	if !ok {
		http.Error(w, "ui bundle unavailable", http.StatusServiceUnavailable)
		return
	}

	stat, err := file.Stat()
	if err != nil {
		http.Error(w, "ui bundle unavailable", http.StatusServiceUnavailable)
		return
	}

	modTime := time.Time{}
	if stat != nil {
		modTime = stat.ModTime()
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, "index.html", modTime, readSeeker)
}
