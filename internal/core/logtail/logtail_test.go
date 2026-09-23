package logtail

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// follow starts Follow on a single file and returns the channel plus a stop
// function.
func follow(t *testing.T, path string) (<-chan string, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan string, 32)
	go NewTailer([]string{path}, 20*time.Millisecond).Follow(ctx, ch)
	return ch, cancel
}

func appendTo(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

// TestFollowReassemblesPartialLines covers the partial-write case: a line
// without a trailing newline must not be emitted, and must be emitted exactly
// once after the writer finishes it.
func TestFollowReassemblesPartialLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	ch, stop := follow(t, path)
	defer stop()

	// Let Follow capture its starting offset (the end of the empty file) before
	// anything is written, otherwise the first write is skipped as "already
	// seen".
	time.Sleep(100 * time.Millisecond)

	// A partial write (no newline yet) must not be emitted.
	appendTo(t, path, "partial")
	select {
	case line := <-ch:
		t.Fatalf("emitted %q for an incomplete line", line)
	case <-time.After(150 * time.Millisecond):
	}

	// Completing the line emits it once, with the earlier part included.
	appendTo(t, path, " line\n")

	select {
	case line := <-ch:
		if line != "partial line" {
			t.Fatalf("line = %q, want %q", line, "partial line")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the completed line")
	}

	select {
	case line := <-ch:
		t.Fatalf("line emitted twice: %q", line)
	case <-time.After(150 * time.Millisecond):
	}
}

// TestFollowHandlesLongLines covers the oversized-line case: the old scanner
// buffer capped lines at 1 MiB and failed the whole batch, so the file could
// never make progress.
func TestFollowHandlesLongLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.log")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	ch, stop := follow(t, path)
	defer stop()

	big := strings.Repeat("x", 2<<20) // 2 MiB, twice the old limit
	appendTo(t, path, big+"\n")

	select {
	case line := <-ch:
		if len(line) != len(big) {
			t.Fatalf("line length = %d, want %d", len(line), len(big))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the oversized line")
	}

	// The next line must still be delivered: the offset advanced past the long
	// one instead of getting stuck on it.
	appendTo(t, path, "after\n")
	select {
	case line := <-ch:
		if line != "after" {
			t.Fatalf("line = %q, want %q", line, "after")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the line after the oversized one")
	}
}
