package wol

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strings"
)

//go:embed frontend/dist/*
var frontendFS embed.FS

// frontendHandler serves the embedded Svelte frontend.
// It falls back to index.html for SPA routing.
func frontendHandler() http.Handler {
	subFS, err := fs.Sub(frontendFS, "frontend/dist")
	if err != nil {
		log.Fatalf("failed to get frontend sub filesystem: %v", err)
	}
	fileServer := http.FileServer(http.FS(subFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to serve the requested file
		p := path.Clean(r.URL.Path)
		if p == "/" {
			data, err := fs.ReadFile(subFS, "index.html")
			if err == nil {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write(data)
				return
			}
		} else {
			// Strip leading slash for fs.FS (paths are relative to sub filesystem root)
			subPath := strings.TrimPrefix(p, "/")
			f, err := subFS.Open(subPath)
			if err == nil {
				f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		// Fallback to index.html (SPA routing)
		data, err := fs.ReadFile(subFS, "index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})
}
