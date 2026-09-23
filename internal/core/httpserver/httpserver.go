// Package httpserver centralizes the HTTP server settings used by mu's local
// services.
//
// net/http's zero-value server has no timeouts at all: a client that opens a
// connection and then trickles headers (slowloris), or starts a body and never
// finishes it, pins a goroutine and a file descriptor until the process runs
// out of them. Every mu server therefore goes through this package instead of
// http.ListenAndServe.
package httpserver

import (
	"net/http"
	"time"
)

// Default timeouts applied by New and ListenAndServe.
//
// The read/write budgets are deliberately generous: several services accept
// archive uploads and serve large responses, and the goal is to bound a stuck
// connection, not to police throughput. ReadHeaderTimeout is the tight one
// because it is what a slowloris client attacks.
const (
	DefaultReadHeaderTimeout = 10 * time.Second
	DefaultReadTimeout       = 5 * time.Minute
	DefaultWriteTimeout      = 5 * time.Minute
	DefaultIdleTimeout       = 2 * time.Minute
)

// Options are the timeouts applied to a server. A zero duration disables that
// particular timeout, matching net/http semantics.
type Options struct {
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

// Default returns the settings used by mu's services.
func Default() Options {
	return Options{
		ReadHeaderTimeout: DefaultReadHeaderTimeout,
		ReadTimeout:       DefaultReadTimeout,
		WriteTimeout:      DefaultWriteTimeout,
		IdleTimeout:       DefaultIdleTimeout,
	}
}

// New builds an *http.Server with the default timeouts. Callers keep their own
// signal handling and shutdown logic.
func New(addr string, handler http.Handler) *http.Server {
	return NewWith(addr, handler, Default())
}

// NewWith builds an *http.Server with explicit timeouts.
func NewWith(addr string, handler http.Handler, o Options) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: o.ReadHeaderTimeout,
		ReadTimeout:       o.ReadTimeout,
		WriteTimeout:      o.WriteTimeout,
		IdleTimeout:       o.IdleTimeout,
	}
}

// ListenAndServe is a drop-in replacement for http.ListenAndServe that applies
// the default timeouts.
func ListenAndServe(addr string, handler http.Handler) error {
	return New(addr, handler).ListenAndServe()
}
