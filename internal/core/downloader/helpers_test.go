package downloader

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewLimiterKeepsReadSizeWithinBurst(t *testing.T) {
	cases := []int64{1, 1024, 64 << 10, 1 << 20, 100 << 20}
	for _, rate := range cases {
		limiter, size := newLimiter(rate)
		if size < 4096 {
			t.Errorf("rate %d: read size = %d, want >= 4096", rate, size)
		}
		if limiter.Burst() < size {
			t.Errorf("rate %d: burst %d < read size %d (WaitN would fail)", rate, limiter.Burst(), size)
		}
	}
}

func TestRetryDelay(t *testing.T) {
	cases := map[int]time.Duration{
		0:  time.Second,
		1:  time.Second,
		2:  time.Second,
		3:  2 * time.Second,
		4:  4 * time.Second,
		6:  16 * time.Second,
		20: 30 * time.Second,
	}
	for attempt, want := range cases {
		if got := retryDelay(attempt); got != want {
			t.Errorf("retryDelay(%d) = %s, want %s", attempt, got, want)
		}
	}
}

func TestWatchIdleCancelsStalledAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var lastActivity atomic.Int64
	var stalled atomic.Bool
	lastActivity.Store(time.Now().UnixNano())

	go watchIdle(ctx, cancel, &lastActivity, &stalled, 100*time.Millisecond)

	select {
	case <-ctx.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("watchdog did not cancel a stalled attempt")
	}
	if !stalled.Load() {
		t.Error("stalled flag was not set")
	}
}

func TestWatchIdleKeepsActiveAttemptAlive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var lastActivity atomic.Int64
	var stalled atomic.Bool
	lastActivity.Store(time.Now().UnixNano())

	go watchIdle(ctx, cancel, &lastActivity, &stalled, 200*time.Millisecond)

	// Keep the connection "busy" for longer than the timeout.
	for i := 0; i < 6; i++ {
		time.Sleep(50 * time.Millisecond)
		lastActivity.Store(time.Now().UnixNano())
	}
	if ctx.Err() != nil {
		t.Errorf("watchdog cancelled an active attempt: %v", ctx.Err())
	}
	if stalled.Load() {
		t.Error("stalled flag was set while data kept arriving")
	}
	cancel()
	// Give the watchdog a moment to observe cancellation and exit.
	time.Sleep(50 * time.Millisecond)
}
