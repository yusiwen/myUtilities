package serve

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type Options struct {
	Dir     string `arg:"" optional:"" name:"dir" help:"Directory to serve." default:"."`
	Port    int    `short:"p" help:"Port to listen on." default:"8080"`
	CORS    bool   `help:"Enable CORS headers for all origins."`
	Verbose bool   `help:"Log requests to stderr."`
}

func (o *Options) Run() error {
	absDir, err := filepath.Abs(o.Dir)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	fi, err := os.Stat(absDir)
	if err != nil {
		return fmt.Errorf("access %s: %w", absDir, err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", absDir)
	}

	var h http.Handler = http.FileServer(http.Dir(absDir))

	if o.CORS {
		h = corsMiddleware(h)
	}

	if o.Verbose {
		h = loggingMiddleware(h)
	}

	addr := fmt.Sprintf(":%d", o.Port)
	fmt.Printf("Serving %s on http://localhost%s\n", absDir, addr)
	return http.ListenAndServe(addr, h)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(lw, r)
		log.Printf("[%s] %s %s %d %v",
			start.Format("2006-01-02 15:04:05"),
			r.Method, r.URL.Path, lw.statusCode, time.Since(start))
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lw *loggingResponseWriter) WriteHeader(code int) {
	lw.statusCode = code
	lw.ResponseWriter.WriteHeader(code)
}
