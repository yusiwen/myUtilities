package downloader

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDownloadForceResumeAfterETagChange covers --force-resume: the validators
// mismatch, but the user insists on continuing with the recorded blocks.
func TestDownloadForceResumeAfterETagChange(t *testing.T) {
	payload := testPayload(2 << 20)
	srv := newFakeServer(t, payload, func(f *fakeServer) { f.abortFirstAt = 1 << 20 })
	out := filepath.Join(t.TempDir(), "out.bin")

	opts := testOptions(srv.URL(), out)
	opts.Threads = 1
	opts.Retries = 0
	opts.BlockSize = 1 << 20

	if _, err := Download(context.Background(), opts); err == nil {
		t.Fatalf("expected the first run to fail (ranges: %v)", srv.seenRanges())
	}

	srv.setETag(`"v2"`)
	srv.resetRanges()
	opts.ForceResume = true

	res, err := Download(context.Background(), opts)
	if err != nil {
		t.Fatalf("forced resume: %v", err)
	}
	if !res.Resumed {
		t.Error("expected Resumed = true with --force-resume")
	}
	if got := readFile(t, out); !bytes.Equal(got, payload) {
		t.Error("downloaded content differs after a forced resume")
	}
	for _, header := range srv.seenRanges() {
		if header == "bytes=0-1048575" {
			t.Error("block 0 was re-downloaded instead of resumed")
		}
	}
}

// TestDownloadNoResumeStartsOver covers --no-resume: existing state and partial
// file are discarded and every block is fetched again.
func TestDownloadNoResumeStartsOver(t *testing.T) {
	payload := testPayload(2 << 20)
	srv := newFakeServer(t, payload, func(f *fakeServer) { f.abortFirstAt = (1 << 20) + (512 << 10) })
	out := filepath.Join(t.TempDir(), "out.bin")

	opts := testOptions(srv.URL(), out)
	opts.Threads = 1
	opts.Retries = 0
	opts.BlockSize = 1 << 20

	if _, err := Download(context.Background(), opts); err == nil {
		t.Fatalf("expected the first run to fail (ranges: %v)", srv.seenRanges())
	}
	if st, err := LoadState(out); err != nil || st == nil {
		t.Fatalf("expected resume state after the failed run (state=%v, err=%v)", st, err)
	}

	srv.resetRanges()
	opts.Resume = false

	res, err := Download(context.Background(), opts)
	if err != nil {
		t.Fatalf("Download with --no-resume: %v", err)
	}
	if res.Resumed {
		t.Error("--no-resume must not resume")
	}
	sawFirstBlock := false
	for _, header := range srv.seenRanges() {
		if header == "bytes=0-1048575" {
			sawFirstBlock = true
		}
	}
	if !sawFirstBlock {
		t.Errorf("expected the first block to be re-fetched, got %v", srv.seenRanges())
	}
	if got := readFile(t, out); !bytes.Equal(got, payload) {
		t.Error("downloaded content differs")
	}
}

// TestDownloadFollowsRedirects makes sure the probe and the ranged requests work
// through a redirecting URL.
func TestDownloadFollowsRedirects(t *testing.T) {
	payload := testPayload(1 << 20)
	srv := newFakeServer(t, payload)

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL(), http.StatusFound)
	}))
	t.Cleanup(redirector.Close)

	out := filepath.Join(t.TempDir(), "out.bin")
	res, err := Download(context.Background(), testOptions(redirector.URL+"/start", out))
	if err != nil {
		t.Fatalf("Download through redirect: %v", err)
	}
	if res.SingleStream {
		t.Error("expected a ranged download after following the redirect")
	}
	if got := readFile(t, out); !bytes.Equal(got, payload) {
		t.Error("downloaded content differs")
	}
}

// TestDownloadInterruptSingleStream covers Ctrl-C in single-stream mode, where
// no resume state is written but the partial file must survive.
func TestDownloadInterruptSingleStream(t *testing.T) {
	payload := testPayload(2 << 20)
	srv := newFakeServer(t, payload, func(f *fakeServer) {
		f.ignoreRange = true
		f.throttle = 5 * time.Millisecond
	})
	out := filepath.Join(t.TempDir(), "out.bin")

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := Download(ctx, testOptions(srv.URL(), out))
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("err = %v, want ErrInterrupted", err)
	}
	if _, err := os.Stat(PartPath(out)); err != nil {
		t.Errorf("partial file should survive an interrupt: %v", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Error("no output file should be published on interrupt")
	}
}

// TestDownloadResumeRejectsTruncatedPartial covers the integrity hole in the
// resume path: when the .part file is shorter than the blocks the state file
// records as done, resuming would re-create the file with zero-filled holes and
// publish the corrupt result as a success.
func TestDownloadResumeRejectsTruncatedPartial(t *testing.T) {
	payload := testPayload(2 << 20)
	srv := newFakeServer(t, payload, func(f *fakeServer) { f.abortFirstAt = (1 << 20) + (512 << 10) })
	out := filepath.Join(t.TempDir(), "out.bin")

	opts := testOptions(srv.URL(), out)
	opts.Threads = 1
	opts.Retries = 0
	opts.BlockSize = 1 << 20

	if _, err := Download(context.Background(), opts); err == nil {
		t.Fatalf("expected the first run to fail (ranges: %v)", srv.seenRanges())
	}
	if _, err := os.Stat(PartPath(out)); err != nil {
		t.Fatalf("expected a partial file to resume from: %v", err)
	}

	// Simulate a cleanup script / sparse-unaware copy truncating the partial.
	if err := os.Truncate(PartPath(out), 0); err != nil {
		t.Fatal(err)
	}

	srv.resetRanges()
	res, err := Download(context.Background(), opts)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if res.Resumed {
		t.Error("must not resume from a partial file that is shorter than its recorded blocks")
	}
	if got := readFile(t, out); !bytes.Equal(got, payload) {
		t.Error("downloaded content differs after a truncated partial")
	}
}

// TestDownloadResumeRejectsOversizedPartial covers the mirror case: a partial
// file with stale bytes appended must be discarded instead of resumed (the
// extra bytes would otherwise end up in the published output when the
// preallocation truncate is unavailable).
func TestDownloadResumeRejectsOversizedPartial(t *testing.T) {
	payload := testPayload(2 << 20)
	srv := newFakeServer(t, payload, func(f *fakeServer) { f.abortFirstAt = (1 << 20) + (512 << 10) })
	out := filepath.Join(t.TempDir(), "out.bin")

	opts := testOptions(srv.URL(), out)
	opts.Threads = 1
	opts.Retries = 0
	opts.BlockSize = 1 << 20

	if _, err := Download(context.Background(), opts); err == nil {
		t.Fatalf("expected the first run to fail (ranges: %v)", srv.seenRanges())
	}

	f, err := os.OpenFile(PartPath(out), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(make([]byte, 4096)); err != nil {
		t.Fatal(err)
	}
	f.Close()

	srv.resetRanges()
	res, err := Download(context.Background(), opts)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if res.Resumed {
		t.Error("must not resume from a partial file larger than the remote size")
	}
	got := readFile(t, out)
	if len(got) != len(payload) {
		t.Fatalf("output size = %d, want %d", len(got), len(payload))
	}
	if !bytes.Equal(got, payload) {
		t.Error("downloaded content differs after an oversized partial")
	}
}
