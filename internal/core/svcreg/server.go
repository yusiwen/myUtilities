package svcreg

import (
	"context"
	"log"
	"net/http"
	"time"
)

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(sw, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, sw.status, time.Since(start))
	})
}

type Server struct {
	httpServer *http.Server
	handler    *svcHandler
	store      Store
}

func NewServer(addr string, store Store) *Server {
	handler := newHandler(store, addr)
	mux := http.NewServeMux()
	handler.registerRoutes(mux)
	return &Server{
		httpServer: &http.Server{
			Addr:    addr,
			Handler: loggingMiddleware(mux),
		},
		handler: handler,
		store:   store,
	}
}

func (s *Server) ListenAndServe() error {
	log.Printf("Service Registry server listening on %s", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return err
	}
	return s.store.Close()
}

func NewHandler(store Store, listenAddr string) *svcHandler {
	return newHandler(store, listenAddr)
}

func (h *svcHandler) RegisterRoutes(mux *http.ServeMux) {
	h.registerRoutes(mux)
}

func LoggingMiddleware(next http.Handler) http.Handler {
	return loggingMiddleware(next)
}
