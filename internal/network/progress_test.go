package network

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/yusiwen/myUtilities/internal/core/downloader"
)

func sampleSnapshot() downloader.Snapshot {
	return downloader.Snapshot{
		Output:        "file.bin",
		Total:         1000,
		SizeKnown:     true,
		Done:          500,
		Blocks:        10,
		DoneBlocks:    5,
		Workers:       4,
		ActiveWorkers: 3,
		Speed:         5 << 20,
		Elapsed:       20 * time.Second,
		ETA:           30 * time.Second,
		Connections: []downloader.ConnStat{
			{ID: 1, Active: true, Bytes: 250, Speed: 2 << 20},
			{ID: 2, Active: true, Bytes: 150, Speed: 1 << 20},
			{ID: 3, Active: true, Bytes: 100, Speed: 1 << 20},
			{ID: 4, Active: false, Bytes: 0, Speed: 0},
		},
	}
}

func TestProgressRendererTTYLiveRegion(t *testing.T) {
	var buf bytes.Buffer
	r := &progressRenderer{out: &buf, isTTY: true, verbose: true, width: 90}

	r.Update(sampleSnapshot())
	r.Update(sampleSnapshot())
	r.Close()
	out := buf.String()

	for _, want := range []string{"50.0%", "conn 3/4", "ETA 00:30", "█", "░", "file.bin", "MiB/s", "[1]"} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "\x1b[?25l") {
		t.Error("cursor should be hidden while the live region is active")
	}
	// 1 aggregate line + 4 connection lines.
	if !strings.Contains(out, "\x1b[5A") {
		t.Error("the region should be repainted in place with a cursor-up sequence")
	}
	if !strings.Contains(out, "\x1b[?25h") {
		t.Error("cursor should be restored when the renderer closes")
	}
}

func TestProgressRendererPlainMode(t *testing.T) {
	var buf bytes.Buffer
	r := &progressRenderer{out: &buf, isTTY: false, width: 100}

	r.Update(sampleSnapshot())
	r.Update(sampleSnapshot())
	if got := strings.Count(buf.String(), "\n"); got != 1 {
		t.Errorf("plain mode printed %d lines in one interval, want 1", got)
	}
	if strings.Contains(buf.String(), "\x1b") {
		t.Error("plain mode must not emit escape sequences")
	}
	if !strings.Contains(buf.String(), "conn 3/4") {
		t.Errorf("plain line missing the connection count: %s", buf.String())
	}
}

func TestProgressRendererQuietAndUnknownSize(t *testing.T) {
	var buf bytes.Buffer
	quiet := &progressRenderer{out: &buf, isTTY: true, quiet: true}
	quiet.Update(sampleSnapshot())
	quiet.Close()
	quiet.Log("should not appear")
	if buf.Len() != 0 {
		t.Errorf("quiet renderer produced output: %q", buf.String())
	}

	var plain bytes.Buffer
	r := &progressRenderer{out: &plain, isTTY: false}
	snap := sampleSnapshot()
	snap.SizeKnown = false
	snap.Total = -1
	r.Update(snap)
	if strings.Contains(plain.String(), "█") {
		t.Error("an unknown total size must not render a progress bar")
	}
	if !strings.Contains(plain.String(), "500 B") {
		t.Errorf("plain line should still show downloaded bytes: %s", plain.String())
	}
}

func TestProgressRendererColor(t *testing.T) {
	var buf bytes.Buffer
	r := &progressRenderer{out: &buf, isTTY: true, color: true, verbose: true, width: 90}
	r.Update(sampleSnapshot())
	r.Close()
	out := buf.String()

	if !strings.Contains(out, "\x1b[32m") { // aec.GreenF, the filled part of the bar
		t.Errorf("expected a colored progress bar:\n%q", out)
	}
	if !strings.Contains(out, "\x1b[36m") { // aec.CyanF, the percentage
		t.Errorf("expected a colored percentage:\n%q", out)
	}
	if plain := (&progressRenderer{out: &bytes.Buffer{}, isTTY: true, width: 90}); plain.color {
		t.Error("color must be off unless explicitly enabled")
	}
}

func TestProgressRendererLogKeepsRegionConsistent(t *testing.T) {
	var buf bytes.Buffer
	r := &progressRenderer{out: &buf, isTTY: true, width: 90}
	r.Update(sampleSnapshot())
	r.Log("downloading with 1 connection")

	if !strings.Contains(buf.String(), "downloading with 1 connection") {
		t.Errorf("log line missing: %q", buf.String())
	}
	if r.rendered != 0 {
		t.Errorf("rendered = %d, want 0 after a log line cleared the region", r.rendered)
	}
	r.Close()
}

func TestFormatDuration(t *testing.T) {
	cases := map[time.Duration]string{
		30 * time.Second: "00:30",
		59 * time.Second: "00:59",
		90 * time.Second: "01:30",
		time.Hour + 2*time.Minute + 5*time.Second: "1:02:05",
		-5 * time.Second: "00:00",
	}
	for input, want := range cases {
		if got := formatDuration(input); got != want {
			t.Errorf("formatDuration(%s) = %q, want %q", input, got, want)
		}
	}
}

func TestFormatSpeed(t *testing.T) {
	if got := formatSpeed(0); got != "-- /s" {
		t.Errorf("formatSpeed(0) = %q, want %q", got, "-- /s")
	}
	if got := formatSpeed(5 << 20); got != "5.0 MiB/s" {
		t.Errorf("formatSpeed(5MiB) = %q, want %q", got, "5.0 MiB/s")
	}
}
