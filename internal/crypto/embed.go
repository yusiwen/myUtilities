package crypto

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strings"
)

//go:embed frontend/dist
var cryptoFrontendFS embed.FS

var cryptoMimeTypes = map[string]string{
	".js":   "application/javascript",
	".css":  "text/css",
	".html": "text/html; charset=utf-8",
	".png":  "image/png",
	".svg":  "image/svg+xml",
}

func FrontendHandler() http.Handler {
	subFS, err := fs.Sub(cryptoFrontendFS, "frontend/dist")
	if err != nil {
		log.Fatalf("failed to get crypto frontend sub filesystem: %v", err)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" || p == "." {
			p = "index.html"
		}
		data, err := fs.ReadFile(subFS, p)
		if err != nil {
			data, err = fs.ReadFile(subFS, "index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(data)
			return
		}
		if ct, ok := cryptoMimeTypes[path.Ext(p)]; ok {
			w.Header().Set("Content-Type", ct)
		}
		w.Write(data)
	})
}
