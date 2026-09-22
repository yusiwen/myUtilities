package downloader

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeServer is a configurable HTTP file server used by the tests. It supports
// byte ranges, HEAD rejection, range-ignoring mode, chunked (unknown-length)
// responses and one-shot connection aborts to exercise resume and retries.
type fakeServer struct {
	data           []byte
	etag           string
	lastModified   string
	filename       string
	ignoreRange    bool
	lieAboutRanges bool
	rejectHEAD     bool
	chunked        bool
	throttle       time.Duration
	abortFirstAt   int64 // byte offset; when > 0 the first range covering it is cut short
	abortPending   atomic.Bool

	mu     sync.Mutex
	ranges []string
	srv    *httptest.Server
}

func newFakeServer(t *testing.T, data []byte, opts ...func(*fakeServer)) *fakeServer {
	t.Helper()
	f := &fakeServer{
		data:         data,
		etag:         `"v1"`,
		lastModified: "Wed, 21 Oct 2026 07:28:00 GMT",
		filename:     "payload.bin",
	}
	for _, opt := range opts {
		opt(f)
	}
	if f.abortFirstAt > 0 {
		f.abortPending.Store(true)
	}
	f.srv = httptest.NewServer(f)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeServer) URL() string { return f.srv.URL + "/" + f.filename }

func (f *fakeServer) seenRanges() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.ranges...)
}

func (f *fakeServer) resetRanges() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ranges = nil
}

func (f *fakeServer) setETag(etag string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.etag = etag
}

func (f *fakeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	etag, lastModified, filename := f.etag, f.lastModified, f.filename
	f.mu.Unlock()

	h := w.Header()
	h.Set("ETag", etag)
	h.Set("Last-Modified", lastModified)
	h.Set("Content-Disposition", `attachment; filename="`+filename+`"`)

	if r.Method == http.MethodHead {
		if f.rejectHEAD {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !f.chunked {
			h.Set("Content-Length", strconv.Itoa(len(f.data)))
		}
		if !f.ignoreRange || f.lieAboutRanges {
			h.Set("Accept-Ranges", "bytes")
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	rangeHeader := r.Header.Get("Range")
	f.mu.Lock()
	f.ranges = append(f.ranges, rangeHeader)
	f.mu.Unlock()

	start, end := int64(0), int64(len(f.data))-1
	if rangeHeader != "" && !f.ignoreRange {
		s, e, ok := parseTestRange(rangeHeader, int64(len(f.data)))
		if !ok {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		start, end = s, e
		h.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(f.data)))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		if !f.chunked {
			h.Set("Content-Length", strconv.Itoa(len(f.data)))
		}
		w.WriteHeader(http.StatusOK)
	}

	if f.chunked {
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}

	body := f.data[start : end+1]

	if f.abortPending.Load() && start <= f.abortFirstAt && f.abortFirstAt <= end && f.abortPending.CompareAndSwap(true, false) {
		cut := f.abortFirstAt - start + 1
		if cut > int64(len(body)) {
			cut = int64(len(body))
		}
		// Flush the response head first: otherwise the transport considers the
		// request untouched and retries it transparently, hiding the failure.
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		f.writeBody(w, body[:cut])
		// Abort the connection the way a crashed peer would.
		panic(http.ErrAbortHandler)
	}
	f.writeBody(w, body)
}

func (f *fakeServer) writeBody(w http.ResponseWriter, body []byte) {
	if f.throttle <= 0 {
		w.Write(body) //nolint:errcheck // test server
		return
	}
	const chunk = 16 << 10
	for len(body) > 0 {
		n := chunk
		if n > len(body) {
			n = len(body)
		}
		if _, err := w.Write(body[:n]); err != nil {
			return
		}
		body = body[n:]
		time.Sleep(f.throttle)
	}
}

func parseTestRange(header string, size int64) (int64, int64, bool) {
	spec := strings.TrimPrefix(header, "bytes=")
	startPart, endPart, found := strings.Cut(spec, "-")
	if !found {
		return 0, 0, false
	}
	start, err := strconv.ParseInt(startPart, 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, false
	}
	end := size - 1
	if endPart != "" {
		end, err = strconv.ParseInt(endPart, 10, 64)
		if err != nil || end < start {
			return 0, 0, false
		}
		if end >= size {
			end = size - 1
		}
	}
	return start, end, true
}

func testPayload(n int) []byte {
	data := make([]byte, n)
	for i := range data {
		data[i] = byte(i*7 + i/251)
	}
	return data
}

func testOptions(url, output string) Options {
	return Options{
		URL:         url,
		Output:      output,
		Threads:     4,
		BlockSize:   256 << 10,
		Resume:      true,
		Preallocate: true,
		Timeout:     10 * time.Second,
		IdleTimeout: 10 * time.Second,
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func TestParseSize(t *testing.T) {
	cases := map[string]int64{
		"1024":  1024,
		"8M":    8 << 20,
		"8MiB":  8 << 20,
		"1MiB":  1 << 20,
		"512k":  512 << 10,
		"500kB": 500000,
		"1GB":   1000000000,
		"2g":    2 << 30,
	}
	for input, want := range cases {
		got, err := ParseSize(input)
		if err != nil {
			t.Fatalf("ParseSize(%q): %v", input, err)
		}
		if got != want {
			t.Errorf("ParseSize(%q) = %d, want %d", input, got, want)
		}
	}
	for _, bad := range []string{"", "abc", "-1M"} {
		if _, err := ParseSize(bad); err == nil {
			t.Errorf("ParseSize(%q) expected an error", bad)
		}
	}
}

func TestFormatSize(t *testing.T) {
	cases := map[int64]string{
		512:            "512 B",
		1024:           "1.0 KiB",
		1536:           "1.5 KiB",
		1 << 20:        "1.0 MiB",
		3 << 30:        "3.0 GiB",
		10 * (1 << 20): "10.0 MiB",
	}
	for input, want := range cases {
		if got := FormatSize(input); got != want {
			t.Errorf("FormatSize(%d) = %q, want %q", input, got, want)
		}
	}
}

func TestAutoBlockSize(t *testing.T) {
	if got := autoBlockSize(100<<20, 4); got != 7<<20 {
		t.Errorf("autoBlockSize(100MiB, 4) = %d, want %d", got, 7<<20)
	}
	if got := autoBlockSize(100<<20, 32); got != MinBlockSize {
		t.Errorf("autoBlockSize(100MiB, 32) = %d, want %d", got, MinBlockSize)
	}
	if got := autoBlockSize(4<<30, 4); got != MaxBlockSize {
		t.Errorf("autoBlockSize(4GiB, 4) = %d, want %d", got, MaxBlockSize)
	}
	if got := autoBlockSize(300<<10, 4); got != 300<<10 {
		t.Errorf("autoBlockSize(300KiB, 4) = %d, want %d (single block)", got, 300<<10)
	}
}

func TestDownloadParallel(t *testing.T) {
	payload := testPayload(3 << 20)
	srv := newFakeServer(t, payload)
	out := filepath.Join(t.TempDir(), "out.bin")

	res, err := Download(context.Background(), testOptions(srv.URL(), out))
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if res.SingleStream {
		t.Error("expected a parallel download")
	}
	if res.Bytes != int64(len(payload)) {
		t.Errorf("Bytes = %d, want %d", res.Bytes, len(payload))
	}
	if got := readFile(t, out); !bytes.Equal(got, payload) {
		t.Errorf("downloaded content differs (got %d bytes)", len(got))
	}
	if res.Threads != 4 {
		t.Errorf("Threads = %d, want 4", res.Threads)
	}
	if ranges := srv.seenRanges(); len(ranges) == 0 {
		t.Error("expected ranged requests")
	}
	for _, header := range srv.seenRanges() {
		if header == "" {
			t.Fatal("parallel download issued an unranged request")
		}
	}
	if _, err := os.Stat(PartPath(out)); !os.IsNotExist(err) {
		t.Errorf("partial file should be gone after success, stat err = %v", err)
	}
	if _, err := os.Stat(StatePath(out)); !os.IsNotExist(err) {
		t.Errorf("state file should be gone after success, stat err = %v", err)
	}
}

func TestDownloadSingleStreamFallback(t *testing.T) {
	payload := testPayload(1 << 20)
	srv := newFakeServer(t, payload, func(f *fakeServer) { f.ignoreRange = true })
	out := filepath.Join(t.TempDir(), "out.bin")

	res, err := Download(context.Background(), testOptions(srv.URL(), out))
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if !res.SingleStream {
		t.Error("expected single-stream mode")
	}
	if got := readFile(t, out); !bytes.Equal(got, payload) {
		t.Error("downloaded content differs")
	}
}

func TestDownloadUnknownLength(t *testing.T) {
	payload := testPayload(300 << 10)
	srv := newFakeServer(t, payload, func(f *fakeServer) { f.chunked = true })
	out := filepath.Join(t.TempDir(), "out.bin")

	res, err := Download(context.Background(), testOptions(srv.URL(), out))
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if !res.SingleStream {
		t.Error("expected single-stream mode for unknown length")
	}
	if res.Size != -1 {
		t.Errorf("Size = %d, want -1 (unknown)", res.Size)
	}
	if got := readFile(t, out); !bytes.Equal(got, payload) {
		t.Error("downloaded content differs")
	}
}

func TestDownloadHeadRejected(t *testing.T) {
	payload := testPayload(512 << 10)
	srv := newFakeServer(t, payload, func(f *fakeServer) { f.rejectHEAD = true })
	out := filepath.Join(t.TempDir(), "out.bin")

	res, err := Download(context.Background(), testOptions(srv.URL(), out))
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if res.SingleStream {
		t.Error("ranged download expected when HEAD is rejected but ranges work")
	}
	if got := readFile(t, out); !bytes.Equal(got, payload) {
		t.Error("downloaded content differs")
	}
}

func TestDownloadRetriesTransientAbort(t *testing.T) {
	payload := testPayload(1 << 20)
	srv := newFakeServer(t, payload, func(f *fakeServer) { f.abortFirstAt = 700 << 10 })
	out := filepath.Join(t.TempDir(), "out.bin")

	opts := testOptions(srv.URL(), out)
	opts.Threads = 1
	opts.Retries = 3
	opts.BlockSize = 1 << 20

	res, err := Download(context.Background(), opts)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if res.Bytes != int64(len(payload)) {
		t.Errorf("Bytes = %d, want %d", res.Bytes, len(payload))
	}
	if got := readFile(t, out); !bytes.Equal(got, payload) {
		t.Error("downloaded content differs")
	}
}

func TestDownloadResume(t *testing.T) {
	payload := testPayload(3 << 20)
	srv := newFakeServer(t, payload, func(f *fakeServer) { f.abortFirstAt = (1 << 20) + (512 << 10) })
	dir := t.TempDir()
	out := filepath.Join(dir, "out.bin")

	opts := testOptions(srv.URL(), out)
	opts.Threads = 1
	opts.Retries = 0
	opts.BlockSize = 1 << 20

	if _, err := Download(context.Background(), opts); err == nil {
		t.Fatal("expected the first run to fail")
	}
	st, err := LoadState(out)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if st == nil {
		t.Fatal("expected resume state to be written")
	}
	if len(st.DoneBlocks) != 1 || st.DoneBlocks[0] != 0 {
		t.Fatalf("DoneBlocks = %v, want [0]", st.DoneBlocks)
	}
	if _, err := os.Stat(PartPath(out)); err != nil {
		t.Fatalf("partial file missing: %v", err)
	}

	srv.resetRanges()
	res, err := Download(context.Background(), opts)
	if err != nil {
		t.Fatalf("resumed Download: %v", err)
	}
	if !res.Resumed {
		t.Error("expected Resumed = true")
	}
	if res.Bytes != 2<<20 {
		t.Errorf("Bytes = %d, want %d (only the two missing blocks)", res.Bytes, 2<<20)
	}
	if res.Size != int64(len(payload)) {
		t.Errorf("Size = %d, want %d", res.Size, len(payload))
	}
	if got := readFile(t, out); !bytes.Equal(got, payload) {
		t.Error("downloaded content differs after resume")
	}
	for _, header := range srv.seenRanges() {
		if header == "bytes=0-1048575" {
			t.Error("block 0 was re-downloaded instead of resumed")
		}
	}
	if len(srv.seenRanges()) != 2 {
		t.Errorf("expected only the two missing blocks to be fetched, got %v", srv.seenRanges())
	}
}

func TestDownloadResumeInvalidatedByETagChange(t *testing.T) {
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
	res, err := Download(context.Background(), opts)
	if err != nil {
		t.Fatalf("Download after ETag change: %v", err)
	}
	if res.Resumed {
		t.Error("resume must be refused when the ETag changed")
	}
	if len(srv.seenRanges()) != 2 {
		t.Errorf("expected a full re-download, got ranges %v", srv.seenRanges())
	}
	if got := readFile(t, out); !bytes.Equal(got, payload) {
		t.Error("downloaded content differs")
	}
	if _, err := os.Stat(PartPath(out) + ".old"); err != nil {
		t.Errorf("expected the stale partial to be kept as .old: %v", err)
	}
}

func TestDownloadCorruptStateStartsOver(t *testing.T) {
	payload := testPayload(512 << 10)
	srv := newFakeServer(t, payload)
	out := filepath.Join(t.TempDir(), "out.bin")

	if err := os.WriteFile(PartPath(out), []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(StatePath(out), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Download(context.Background(), testOptions(srv.URL(), out))
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if res.Resumed {
		t.Error("corrupt state must not be resumed")
	}
	if got := readFile(t, out); !bytes.Equal(got, payload) {
		t.Error("downloaded content differs")
	}
}

func TestDownloadChecksum(t *testing.T) {
	payload := testPayload(256 << 10)
	sum := sha256.Sum256(payload)
	good := hex.EncodeToString(sum[:])
	srv := newFakeServer(t, payload)
	dir := t.TempDir()

	opts := testOptions(srv.URL(), filepath.Join(dir, "good.bin"))
	opts.SHA256 = strings.ToUpper(good)
	res, err := Download(context.Background(), opts)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if !strings.EqualFold(res.SHA256, good) {
		t.Errorf("SHA256 = %q, want %q", res.SHA256, good)
	}

	badOut := filepath.Join(dir, "bad.bin")
	badOpts := testOptions(srv.URL(), badOut)
	badOpts.SHA256 = strings.Repeat("0", 64)
	if _, err := Download(context.Background(), badOpts); !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("err = %v, want ErrChecksumMismatch", err)
	}
	if _, err := os.Stat(PartPath(badOut)); err != nil {
		t.Errorf("partial file should be kept after a checksum mismatch: %v", err)
	}
	if _, err := os.Stat(badOut); !os.IsNotExist(err) {
		t.Error("output file must not be published after a checksum mismatch")
	}
}

func TestDownloadOutputExists(t *testing.T) {
	payload := testPayload(128 << 10)
	srv := newFakeServer(t, payload)
	out := filepath.Join(t.TempDir(), "out.bin")
	if err := os.WriteFile(out, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Download(context.Background(), testOptions(srv.URL(), out)); !errors.Is(err, ErrOutputExists) {
		t.Fatalf("err = %v, want ErrOutputExists", err)
	}

	opts := testOptions(srv.URL(), out)
	opts.Force = true
	if _, err := Download(context.Background(), opts); err != nil {
		t.Fatalf("Download with --force: %v", err)
	}
	if got := readFile(t, out); !bytes.Equal(got, payload) {
		t.Error("downloaded content differs")
	}
}

func TestDownloadOutputDirectory(t *testing.T) {
	payload := testPayload(64 << 10)
	srv := newFakeServer(t, payload, func(f *fakeServer) { f.filename = "from-header.bin" })
	dir := t.TempDir()

	res, err := Download(context.Background(), testOptions(srv.URL(), dir))
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if want := filepath.Join(dir, "from-header.bin"); res.Path != want {
		t.Errorf("Path = %q, want %q", res.Path, want)
	}
	if got := readFile(t, res.Path); !bytes.Equal(got, payload) {
		t.Error("downloaded content differs")
	}
}

func TestDownloadEmptyFile(t *testing.T) {
	srv := newFakeServer(t, []byte{})
	out := filepath.Join(t.TempDir(), "empty.bin")

	res, err := Download(context.Background(), testOptions(srv.URL(), out))
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if res.Size != 0 {
		t.Errorf("Size = %d, want 0", res.Size)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("expected an empty output file: %v", err)
	}
}

func TestDownloadRetriesExhausted(t *testing.T) {
	payload := testPayload(1 << 20)
	srv := newFakeServer(t, payload, func(f *fakeServer) { f.abortFirstAt = 512 << 10 })
	out := filepath.Join(t.TempDir(), "out.bin")

	opts := testOptions(srv.URL(), out)
	opts.Threads = 1
	opts.BlockSize = 1 << 20
	opts.Retries = 0
	if _, err := Download(context.Background(), opts); err == nil {
		t.Fatal("expected the download to fail")
	}
	if _, err := os.Stat(PartPath(out)); err != nil {
		t.Errorf("partial file should be kept on failure: %v", err)
	}
	if _, err := LoadState(out); err != nil {
		t.Errorf("resume state should be readable: %v", err)
	}
}

func TestDownloadInterrupted(t *testing.T) {
	payload := testPayload(4 << 20)
	srv := newFakeServer(t, payload, func(f *fakeServer) { f.throttle = 5 * time.Millisecond })
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
	if st, err := LoadState(out); err != nil || st == nil {
		t.Errorf("resume state should survive an interrupt (state=%v, err=%v)", st, err)
	}
}

func TestDownloadLimitRate(t *testing.T) {
	payload := testPayload(64 << 10)
	srv := newFakeServer(t, payload)
	out := filepath.Join(t.TempDir(), "out.bin")

	opts := testOptions(srv.URL(), out)
	opts.LimitRate = 128 << 10

	start := time.Now()
	if _, err := Download(context.Background(), opts); err != nil {
		t.Fatalf("Download: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Errorf("limited download finished in %s, expected it to be throttled", elapsed)
	}
	if got := readFile(t, out); !bytes.Equal(got, payload) {
		t.Error("downloaded content differs")
	}
}

// snapshotCollector records progress snapshots for assertions.
type snapshotCollector struct {
	mu        sync.Mutex
	snapshots []Snapshot
}

func (c *snapshotCollector) Update(s Snapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.snapshots = append(c.snapshots, s)
}

func (c *snapshotCollector) all() []Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Snapshot(nil), c.snapshots...)
}

func TestDownloadProgressSnapshots(t *testing.T) {
	oldInterval := reportInterval
	reportInterval = 5 * time.Millisecond
	t.Cleanup(func() { reportInterval = oldInterval })

	payload := testPayload(2 << 20)
	srv := newFakeServer(t, payload, func(f *fakeServer) { f.throttle = 2 * time.Millisecond })
	out := filepath.Join(t.TempDir(), "out.bin")

	collector := &snapshotCollector{}
	opts := testOptions(srv.URL(), out)
	opts.Progress = collector

	if _, err := Download(context.Background(), opts); err != nil {
		t.Fatalf("Download: %v", err)
	}

	snapshots := collector.all()
	if len(snapshots) == 0 {
		t.Fatal("expected at least one progress snapshot")
	}
	last := snapshots[len(snapshots)-1]
	if !last.SizeKnown || last.Total != int64(len(payload)) {
		t.Errorf("snapshot total = %d (known=%v), want %d", last.Total, last.SizeKnown, len(payload))
	}
	if last.Done <= 0 {
		t.Errorf("snapshot Done = %d, want > 0", last.Done)
	}
	if len(last.Connections) != opts.Threads {
		t.Errorf("snapshot has %d connections, want %d", len(last.Connections), opts.Threads)
	}
	maxBlocks := int64(0)
	sawActive := false
	for _, s := range snapshots {
		if s.DoneBlocks > maxBlocks {
			maxBlocks = s.DoneBlocks
		}
		if s.ActiveWorkers > 0 {
			sawActive = true
		}
		if s.Blocks == 0 {
			t.Errorf("snapshot reported 0 blocks (total %d)", s.Blocks)
		}
	}
	if maxBlocks == 0 {
		t.Error("no snapshot recorded a completed block")
	}
	if !sawActive {
		t.Error("no snapshot reported an active worker")
	}
}

func TestDownloadRangeIgnoredAtRuntime(t *testing.T) {
	payload := testPayload(1 << 20)
	srv := newFakeServer(t, payload, func(f *fakeServer) {
		f.ignoreRange = true
		f.lieAboutRanges = true
	})
	out := filepath.Join(t.TempDir(), "out.bin")

	res, err := Download(context.Background(), testOptions(srv.URL(), out))
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if !res.SingleStream {
		t.Error("expected a fallback to single-stream mode")
	}
	if got := readFile(t, out); !bytes.Equal(got, payload) {
		t.Error("downloaded content differs")
	}
}
