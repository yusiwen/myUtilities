package downloader

import (
	"sync/atomic"
	"time"
)

// Progress receives periodic transfer snapshots. Implementations must tolerate
// being called from the reporter goroutine only (never concurrently).
type Progress interface {
	Update(Snapshot)
}

// ConnStat describes one worker/connection.
type ConnStat struct {
	ID     int     // 1-based worker number
	Active bool    // a request is currently in flight
	Bytes  int64   // bytes received by this worker since the run started
	Speed  float64 // bytes per second over the sliding window
}

// Snapshot is a point-in-time view of the transfer, used by the CLI renderer.
type Snapshot struct {
	Output        string
	Total         int64 // remote size; meaningless when SizeKnown is false
	SizeKnown     bool
	Done          int64 // bytes received so far
	Blocks        int64
	DoneBlocks    int64
	Workers       int
	ActiveWorkers int
	Speed         float64 // aggregate bytes per second
	Elapsed       time.Duration
	ETA           time.Duration // zero when not estimable
	Connections   []ConnStat
	SingleStream  bool
	Resumed       bool
}

// stats aggregates byte counters across workers using atomics so the reporter
// can read them without blocking the transfer.
type stats struct {
	total   atomic.Int64
	perConn []atomic.Int64
	active  []atomic.Int32
}

func newStats(conns int) *stats {
	if conns < 1 {
		conns = 1
	}
	return &stats{
		perConn: make([]atomic.Int64, conns),
		active:  make([]atomic.Int32, conns),
	}
}

// add adds n bytes to both the aggregate and the worker counter.
func (s *stats) add(conn int, n int64) {
	s.total.Add(n)
	if conn >= 0 && conn < len(s.perConn) {
		s.perConn[conn].Add(n)
	}
}

// sub removes bytes that belonged to a failed attempt and will be re-fetched.
func (s *stats) sub(conn int, n int64) {
	if n == 0 {
		return
	}
	s.add(conn, -n)
}

// setActive tracks whether a worker currently has a request in flight.
func (s *stats) setActive(conn int, active bool) {
	if conn < 0 || conn >= len(s.active) {
		return
	}
	if active {
		s.active[conn].Store(1)
		return
	}
	s.active[conn].Store(0)
}

// speedWindow computes a sliding-window rate from periodic (time, bytes)
// samples, so the displayed speed reacts to real changes instead of decaying
// from a whole-run average.
type speedWindow struct {
	samples []speedSample
	next    int
	count   int
}

type speedSample struct {
	at    time.Time
	bytes int64
}

func newSpeedWindow(size int) *speedWindow {
	if size < 2 {
		size = 2
	}
	return &speedWindow{samples: make([]speedSample, size)}
}

// add records a sample and returns the current rate in bytes per second.
func (w *speedWindow) add(at time.Time, bytes int64) float64 {
	w.samples[w.next] = speedSample{at: at, bytes: bytes}
	w.next = (w.next + 1) % len(w.samples)
	if w.count < len(w.samples) {
		w.count++
	}
	if w.count < 2 {
		return 0
	}
	oldest := w.samples[(w.next-w.count+len(w.samples))%len(w.samples)]
	newest := w.samples[(w.next-1+len(w.samples))%len(w.samples)]
	elapsed := newest.at.Sub(oldest.at).Seconds()
	if elapsed <= 0 {
		return 0
	}
	speed := float64(newest.bytes-oldest.bytes) / elapsed
	if speed < 0 {
		return 0
	}
	return speed
}
