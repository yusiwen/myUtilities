// Package downloader implements a multi-connection, resumable HTTP(S) file
// downloader: it probes the remote file, splits it into fixed-size blocks that
// are fetched by several workers, records progress in a sidecar state file so an
// interrupted transfer can continue, and reports live statistics to a Progress
// implementation (the CLI renders them as a progress bar).
package downloader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

const (
	// DefaultThreads is the default number of parallel connections.
	DefaultThreads = 4
	// MaxThreads caps the number of parallel connections.
	MaxThreads = 32

	// DefaultBlockSize is used when the block size cannot be derived from the
	// file size.
	DefaultBlockSize = 8 << 20
	// MinBlockSize is the smallest automatically chosen work unit.
	MinBlockSize = 1 << 20
	// MaxBlockSize is the largest automatically chosen work unit.
	MaxBlockSize = 32 << 20

	readChunkSize   = 64 << 10
	speedWindowSize = 30
)

// Tunables exposed as variables so tests can shorten them.
var (
	reportInterval     = 200 * time.Millisecond
	stateFlushInterval = time.Second
)

// Options controls a download.
type Options struct {
	// URL is the remote file to fetch.
	URL string
	// Output is the target file or directory. Empty means "use the remote
	// filename in the current directory".
	Output string
	// Threads is the number of parallel connections (1..MaxThreads).
	Threads int
	// BlockSize is the work-unit size; 0 selects it from the file size.
	BlockSize int64
	// Resume continues an unfinished download when a state file is present.
	Resume bool
	// ForceResume resumes even when the remote validators changed.
	ForceResume bool
	// Force overwrites an existing output file.
	Force bool
	// Preallocate sizes the partial file up front (sparse on ext4/APFS).
	Preallocate bool
	// Headers are extra `Key: Value` request headers.
	Headers []string
	// Auth sets `Authorization: Bearer <Auth>`.
	Auth string
	// User sets HTTP basic auth as "user:password".
	User string
	// Proxy is an HTTP(S) proxy URL; empty uses the environment/proxy defaults.
	Proxy string
	// Timeout bounds connect/TLS/response-header waits.
	Timeout time.Duration
	// IdleTimeout aborts a connection that stops sending data for this long.
	IdleTimeout time.Duration
	// Retries is the per-block retry count.
	Retries int
	// LimitRate caps the aggregate speed in bytes per second (0 = unlimited).
	LimitRate int64
	// Insecure skips TLS certificate verification.
	Insecure bool
	// CACert is a PEM file whose certificates are added to the system roots.
	CACert string
	// HTTP1 forces HTTP/1.1 (no HTTP/2 connection multiplexing).
	HTTP1 bool
	// SHA256 is the expected checksum; an empty string skips verification.
	SHA256 string
	// Progress receives live transfer snapshots (optional).
	Progress Progress
	// Infof and Warnf receive informational and warning messages (optional).
	Infof func(format string, args ...any)
	Warnf func(format string, args ...any)
}

// Result summarises a finished download.
type Result struct {
	Path         string
	Size         int64
	Bytes        int64
	Elapsed      time.Duration
	AvgSpeed     float64
	Threads      int
	Resumed      bool
	SingleStream bool
	SHA256       string
}

type downloader struct {
	opts   Options
	client *http.Client
	info   *RemoteInfo

	outPath  string
	partPath string

	plan       *plan
	state      *State
	resumed    bool
	single     bool
	writer     *fileWriter
	stats      *stats
	limiter    *rate.Limiter
	readSize   int
	stateMu    sync.Mutex
	stateSaved []int64
}

// Download fetches o.URL to o.Output and returns a summary. On failure the
// partial file and its resume state are left in place so the transfer can be
// continued with another call.
func Download(ctx context.Context, o Options) (*Result, error) {
	started := time.Now()
	o = o.withDefaults()
	if err := o.validateHeaders(); err != nil {
		return nil, err
	}

	client, err := NewClient(o)
	if err != nil {
		return nil, err
	}
	info, err := Probe(ctx, client, o)
	if err != nil {
		return nil, err
	}

	d := &downloader{opts: o, client: client, info: info, readSize: readChunkSize}
	if err := d.resolvePath(); err != nil {
		return nil, err
	}
	if err := d.prepare(); err != nil {
		return nil, err
	}

	res, err := d.transfer(ctx, started)
	if res != nil {
		res.Elapsed = time.Since(started)
		if res.Elapsed > 0 {
			res.AvgSpeed = float64(res.Bytes) / res.Elapsed.Seconds()
		}
	}
	return res, err
}

func (o Options) withDefaults() Options {
	if o.Threads <= 0 {
		o.Threads = DefaultThreads
	}
	if o.Threads > MaxThreads {
		o.Threads = MaxThreads
	}
	if o.Timeout <= 0 {
		o.Timeout = 30 * time.Second
	}
	if o.IdleTimeout <= 0 {
		o.IdleTimeout = 30 * time.Second
	}
	if o.Retries < 0 {
		o.Retries = 0
	}
	return o
}

func (d *downloader) infof(format string, args ...any) {
	if d.opts.Infof != nil {
		d.opts.Infof(format, args...)
	}
}

func (d *downloader) warnf(format string, args ...any) {
	if d.opts.Warnf != nil {
		d.opts.Warnf(format, args...)
	}
}

// resolvePath decides the final output path and the partial-file location.
func (d *downloader) resolvePath() error {
	out := d.opts.Output
	switch {
	case out == "":
		out = d.remoteFilename()
	case strings.HasSuffix(out, "/") || strings.HasSuffix(out, string(os.PathSeparator)):
		out = filepath.Join(out, d.remoteFilename())
	default:
		if fi, err := os.Stat(out); err == nil && fi.IsDir() {
			out = filepath.Join(out, d.remoteFilename())
		}
	}
	if dir := filepath.Dir(out); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("cannot create output directory %s: %w", dir, err)
		}
	}
	d.outPath = out
	d.partPath = PartPath(out)
	return nil
}

func (d *downloader) remoteFilename() string {
	if d.info.Filename != "" {
		return d.info.Filename
	}
	return "download"
}

// prepare checks the target file and decides whether an existing partial
// download can be resumed.
func (d *downloader) prepare() error {
	if fi, err := os.Stat(d.outPath); err == nil && fi.Mode().IsRegular() {
		if !d.opts.Force {
			return fmt.Errorf("%w: %s (use --force to overwrite)", ErrOutputExists, d.outPath)
		}
	}

	partSize := int64(-1)
	if fi, err := os.Stat(d.partPath); err == nil && fi.Mode().IsRegular() {
		partSize = fi.Size()
	}

	if d.opts.Resume {
		st, err := LoadState(d.outPath)
		switch {
		case err != nil:
			d.warnf("%v; starting over", err)
		case st == nil:
			if partSize > 0 {
				d.warnf("discarding partial file without resume state: %s", d.partPath)
			}
		default:
			reason, ok := d.stateUsable(st, partSize)
			if !ok && !d.opts.ForceResume {
				d.warnf("cannot resume (%s); starting over", reason)
				d.discardPartial()
			} else {
				if !ok {
					d.warnf("resuming despite %s (--force-resume)", reason)
				}
				d.state = st
				d.resumed = len(st.DoneBlocks) > 0
			}
		}
	} else if partSize > 0 {
		d.warnf("discarding partial file (--no-resume): %s", d.partPath)
	}
	return nil
}

// stateUsable reports whether the recorded state still matches the remote file.
func (d *downloader) stateUsable(st *State, partSize int64) (reason string, ok bool) {
	if partSize < 0 {
		return "partial file is missing", false
	}
	if st.Size != d.info.Size {
		return fmt.Sprintf("remote size changed (%d -> %d)", st.Size, d.info.Size), false
	}
	if st.URL != "" && d.opts.URL != "" && st.URL != d.opts.URL {
		return "URL changed", false
	}
	if st.ETag != "" && d.info.ETag != "" && st.ETag != d.info.ETag {
		return fmt.Sprintf("ETag changed (%s -> %s)", st.ETag, d.info.ETag), false
	}
	if st.ETag == "" && st.LastModified != "" && d.info.LastModified != "" && st.LastModified != d.info.LastModified {
		return "Last-Modified changed", false
	}
	if d.opts.BlockSize > 0 && st.BlockSize > 0 && d.opts.BlockSize != st.BlockSize {
		return fmt.Sprintf("block size changed (%d -> %d)", st.BlockSize, d.opts.BlockSize), false
	}
	if st.ETag == "" && st.LastModified == "" {
		d.warnf("remote file has no validator; resuming on size match only")
	}
	return "", true
}

// discardPartial keeps the unusable partial file as `<output>.part.old` (so the
// user can still inspect it) and clears the resume state.
func (d *downloader) discardPartial() {
	if _, err := os.Stat(d.partPath); err == nil {
		old := d.partPath + ".old"
		_ = os.Remove(old)
		_ = os.Rename(d.partPath, old)
	}
	_ = RemoveState(d.outPath)
	d.state = nil
	d.resumed = false
}

// removePartial drops the partial file and its state.
func (d *downloader) removePartial() {
	_ = os.Remove(d.partPath)
	_ = RemoveState(d.outPath)
	d.state = nil
	d.resumed = false
}

func (d *downloader) transfer(ctx context.Context, started time.Time) (*Result, error) {
	if d.info.Size == 0 {
		return d.createEmptyFile()
	}
	if !d.info.Ranged {
		return d.singleStream(ctx, started)
	}
	return d.parallel(ctx, started)
}

// createEmptyFile handles a zero-byte remote file.
func (d *downloader) createEmptyFile() (*Result, error) {
	f, err := os.OpenFile(d.outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	_ = RemoveState(d.outPath)
	return &Result{Path: d.outPath, Size: 0, Threads: d.opts.Threads}, nil
}

// blockSizeFor picks the work-unit size, preferring the size recorded in a
// resume state so that a resumed transfer uses the same block layout.
func (d *downloader) blockSizeFor() int64 {
	if d.state != nil && d.state.BlockSize > 0 {
		return d.state.BlockSize
	}
	if d.opts.BlockSize > 0 {
		return d.opts.BlockSize
	}
	return autoBlockSize(d.info.Size, d.opts.Threads)
}

// autoBlockSize spreads the file over roughly four blocks per connection,
// rounded to 1 MiB and clamped to [MinBlockSize, MaxBlockSize].
func autoBlockSize(size int64, threads int) int64 {
	if size <= 0 {
		return DefaultBlockSize
	}
	if threads < 1 {
		threads = 1
	}
	blockSize := size / int64(threads*4)
	blockSize = ((blockSize + MinBlockSize - 1) / MinBlockSize) * MinBlockSize
	if blockSize < MinBlockSize {
		blockSize = MinBlockSize
	}
	if blockSize > MaxBlockSize {
		blockSize = MaxBlockSize
	}
	if blockSize > size {
		blockSize = size
	}
	return blockSize
}

// newLimiter builds the shared rate limiter and the largest safe read size
// (a WaitN call must not exceed the limiter's burst).
func newLimiter(bytesPerSec int64) (*rate.Limiter, int) {
	burst := bytesPerSec / 4
	if burst < 8<<10 {
		burst = 8 << 10
	}
	if burst > 1<<20 {
		burst = 1 << 20
	}
	limiter := rate.NewLimiter(rate.Limit(bytesPerSec), int(burst))
	size := readChunkSize
	if int64(size) > burst {
		size = int(burst)
	}
	if size < 4096 {
		size = 4096
	}
	return limiter, size
}

// parallel runs the block-queue transfer.
func (d *downloader) parallel(ctx context.Context, started time.Time) (*Result, error) {
	blockSize := d.blockSizeFor()
	var doneBlocks []int64
	if d.state != nil {
		doneBlocks = d.state.DoneBlocks
	}
	d.plan = newPlan(d.info.Size, blockSize, doneBlocks)

	w, err := openWriter(d.partPath, d.info.Size, !d.resumed, d.opts.Preallocate, d.warnf)
	if err != nil {
		return nil, err
	}
	d.writer = w
	defer w.Close() //nolint:errcheck // closed explicitly on the success path

	d.stats = newStats(d.opts.Threads)
	resumedBytes := d.plan.doneBytes()
	d.stats.total.Store(resumedBytes)
	if d.opts.LimitRate > 0 {
		d.limiter, d.readSize = newLimiter(d.opts.LimitRate)
	}

	_, totalBlocks := d.plan.progress()
	if d.resumed {
		d.infof("resuming %s: %s of %s already complete (%d/%d blocks)",
			d.outPath, FormatSize(resumedBytes), FormatSize(d.info.Size), len(doneBlocks), totalBlocks)
	}
	d.infof("downloading %s with %d connection(s): %d block(s) of %s",
		d.info.FinalURL, d.opts.Threads, totalBlocks, FormatSize(blockSize))
	if err := d.saveState(true); err != nil {
		d.warnf("cannot write resume state: %v", err)
	}

	reportCtx, stopReport := context.WithCancel(ctx)
	var reportWG sync.WaitGroup
	d.startReporter(reportCtx, &reportWG, started)

	runCtx, cancelRun := context.WithCancel(ctx)
	var flushWG sync.WaitGroup
	startStateFlusher(runCtx, &flushWG, d)

	var workerWG sync.WaitGroup
	errCh := make(chan error, d.opts.Threads)
	for i := 0; i < d.opts.Threads; i++ {
		workerWG.Add(1)
		go func(id int) {
			defer workerWG.Done()
			if err := d.worker(runCtx, id); err != nil {
				select {
				case errCh <- err:
				default:
				}
				cancelRun()
			}
		}(i)
	}

	workerWG.Wait()
	cancelRun()
	flushWG.Wait()
	stopReport()
	reportWG.Wait()

	runErr := firstError(errCh)
	if ctx.Err() != nil {
		runErr = ErrInterrupted
	}

	if runErr != nil {
		if err := d.saveState(true); err != nil {
			d.warnf("cannot write resume state: %v", err)
		}
		if errors.Is(runErr, errRangeIgnored) {
			d.warnf("server ignored the range request; falling back to a single connection")
			_ = w.Close()
			d.plan = nil
			d.removePartial()
			return d.singleStream(ctx, started)
		}
		return nil, runErr
	}

	if err := w.Sync(); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	sum, err := d.verifyChecksum()
	if err != nil {
		_ = d.saveState(true)
		return nil, err
	}
	if err := d.publish(); err != nil {
		return nil, err
	}
	_ = RemoveState(d.outPath)

	return &Result{
		Path:    d.outPath,
		Size:    d.info.Size,
		Bytes:   d.info.Size - resumedBytes,
		Threads: d.opts.Threads,
		Resumed: d.resumed,
		SHA256:  sum,
	}, nil
}

// worker pulls and writes blocks until the plan is exhausted or ctx is done.
func (d *downloader) worker(ctx context.Context, id int) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		idx, ok := d.plan.next()
		if !ok {
			return nil
		}
		if err := d.fetchBlockWithRetry(ctx, idx, id); err != nil {
			return err
		}
		d.plan.mark(idx)
	}
}

// fetchBlockWithRetry downloads one block, retrying transient failures with
// exponential backoff. Bytes written by failed attempts are subtracted from the
// progress counters because they will be fetched again.
func (d *downloader) fetchBlockWithRetry(ctx context.Context, idx int64, conn int) error {
	attempts := d.opts.Retries + 1
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 {
			delay := retryDelay(attempt)
			d.warnf("block %d: attempt %d/%d failed (%v); retrying in %s",
				idx+1, attempt-1, attempts, lastErr, delay)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
		written, err := d.fetchBlock(ctx, idx, conn)
		if err == nil {
			return nil
		}
		d.stats.sub(conn, written)
		if errors.Is(err, errRangeIgnored) {
			return err
		}
		if ctx.Err() != nil {
			return err
		}
		lastErr = err
	}
	return lastErr
}

// retryDelay returns 1s, 2s, 4s, ... capped at 30s for attempt >= 2.
func retryDelay(attempt int) time.Duration {
	if attempt < 2 {
		return time.Second
	}
	delay := time.Second << uint(attempt-2)
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	return delay
}

// fetchBlock downloads a single block range and writes it at its file offset.
// On error it returns the number of bytes already written.
func (d *downloader) fetchBlock(ctx context.Context, idx int64, conn int) (int64, error) {
	start, end := d.plan.blockRange(idx)

	attemptCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var lastActivity atomic.Int64
	var stalled atomic.Bool
	lastActivity.Store(time.Now().UnixNano())
	if d.opts.IdleTimeout > 0 {
		go watchIdle(attemptCtx, cancel, &lastActivity, &stalled, d.opts.IdleTimeout)
	}

	req, err := d.opts.newRequest(attemptCtx, http.MethodGet, d.info.FinalURL)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

	d.stats.setActive(conn, true)
	defer d.stats.setActive(conn, false)

	resp, err := d.client.Do(req)
	if err != nil {
		if stalled.Load() {
			return 0, fmt.Errorf("connection stalled (no data for %s)", d.opts.IdleTimeout)
		}
		return 0, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusPartialContent:
		gotStart, _, _, ok := parseContentRange(resp.Header.Get("Content-Range"))
		if ok && gotStart != start {
			return 0, fmt.Errorf("unexpected Content-Range %q (wanted offset %d)", resp.Header.Get("Content-Range"), start)
		}
	case http.StatusOK:
		// The server ignored the Range header. That is only acceptable when the
		// requested block is the entire file.
		if start != 0 || end != d.info.Size-1 {
			return 0, errRangeIgnored
		}
	case http.StatusRequestedRangeNotSatisfiable:
		// The block is already present server-side; treat it as done.
		return 0, nil
	default:
		return 0, fmt.Errorf("unexpected HTTP status %s", resp.Status)
	}

	offset := start
	written, err := d.copyStream(attemptCtx, resp.Body, conn, func(p []byte) error {
		if _, werr := d.writer.WriteAt(p, offset); werr != nil {
			return werr
		}
		offset += int64(len(p))
		return nil
	}, &lastActivity, &stalled)
	if err != nil {
		return written, err
	}
	if want := end - start + 1; written != want {
		return written, fmt.Errorf("short read: got %d of %d bytes", written, want)
	}
	return written, nil
}

// singleStream downloads without ranges (servers that ignore Range headers).
// Resuming is impossible in this mode.
func (d *downloader) singleStream(ctx context.Context, started time.Time) (*Result, error) {
	if d.state != nil {
		d.warnf("server does not support range requests; restarting from scratch")
		d.state = nil
		d.resumed = false
	}
	if d.opts.LimitRate > 0 {
		d.limiter, d.readSize = newLimiter(d.opts.LimitRate)
	}

	w, err := openWriter(d.partPath, d.info.Size, true, d.opts.Preallocate && d.info.Size > 0, d.warnf)
	if err != nil {
		return nil, err
	}
	defer w.Close() //nolint:errcheck // closed explicitly on the success path
	d.writer = w
	d.single = true
	d.stats = newStats(1)

	attemptCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var lastActivity atomic.Int64
	var stalled atomic.Bool
	lastActivity.Store(time.Now().UnixNano())
	if d.opts.IdleTimeout > 0 {
		go watchIdle(attemptCtx, cancel, &lastActivity, &stalled, d.opts.IdleTimeout)
	}

	req, err := d.opts.newRequest(attemptCtx, http.MethodGet, d.info.FinalURL)
	if err != nil {
		return nil, err
	}

	d.stats.setActive(0, true)
	resp, err := d.client.Do(req)
	if err != nil {
		d.stats.setActive(0, false)
		if ctx.Err() != nil {
			return nil, ErrInterrupted
		}
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		d.stats.setActive(0, false)
		return nil, fmt.Errorf("unexpected HTTP status %s", resp.Status)
	}
	if d.info.Size < 0 && resp.ContentLength >= 0 {
		d.info.Size = resp.ContentLength
	}
	d.infof("downloading %s with a single connection (server does not support ranges)", d.info.FinalURL)

	reportCtx, stopReport := context.WithCancel(ctx)
	var reportWG sync.WaitGroup
	d.startReporter(reportCtx, &reportWG, started)

	written, err := d.copyStream(attemptCtx, resp.Body, 0, func(p []byte) error {
		_, werr := w.Write(p)
		return werr
	}, &lastActivity, &stalled)
	d.stats.setActive(0, false)
	stopReport()
	reportWG.Wait()

	if err != nil {
		if ctx.Err() != nil {
			return nil, ErrInterrupted
		}
		return nil, err
	}
	if d.info.Size >= 0 && written != d.info.Size {
		return nil, fmt.Errorf("short read: got %d of %d bytes", written, d.info.Size)
	}

	if err := w.Sync(); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	sum, err := d.verifyChecksum()
	if err != nil {
		return nil, err
	}
	if err := d.publish(); err != nil {
		return nil, err
	}
	_ = RemoveState(d.outPath)

	return &Result{
		Path:         d.outPath,
		Size:         d.info.Size,
		Bytes:        written,
		Threads:      1,
		SingleStream: true,
		SHA256:       sum,
	}, nil
}

// copyStream pumps body into sink, applying the rate limit and refreshing the
// idle-timeout watchdog.
func (d *downloader) copyStream(ctx context.Context, body io.Reader, conn int, sink func([]byte) error, lastActivity *atomic.Int64, stalled *atomic.Bool) (int64, error) {
	buf := make([]byte, d.readSize)
	var written int64
	for {
		n, readErr := body.Read(buf)
		if n > 0 {
			if lastActivity != nil {
				lastActivity.Store(time.Now().UnixNano())
			}
			if d.limiter != nil {
				if waitErr := d.limiter.WaitN(ctx, n); waitErr != nil {
					return written, waitErr
				}
			}
			if sinkErr := sink(buf[:n]); sinkErr != nil {
				return written, sinkErr
			}
			written += int64(n)
			d.stats.add(conn, int64(n))
		}
		if readErr != nil {
			if readErr == io.EOF {
				return written, nil
			}
			if stalled != nil && stalled.Load() {
				return written, fmt.Errorf("connection stalled (no data for %s)", d.opts.IdleTimeout)
			}
			return written, readErr
		}
	}
}

// watchIdle cancels the attempt when no data arrives for timeout.
func watchIdle(ctx context.Context, cancel context.CancelFunc, lastActivity *atomic.Int64, stalled *atomic.Bool, timeout time.Duration) {
	interval := timeout / 4
	if interval < 100*time.Millisecond {
		interval = 100 * time.Millisecond
	}
	if interval > time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if now.Sub(time.Unix(0, lastActivity.Load())) > timeout {
				stalled.Store(true)
				cancel()
				return
			}
		}
	}
}

// publish moves the verified partial file into place.
func (d *downloader) publish() error {
	if _, err := os.Stat(d.outPath); err == nil {
		if !d.opts.Force {
			return fmt.Errorf("%w: %s appeared during the download", ErrOutputExists, d.outPath)
		}
		// os.Rename does not overwrite on Windows.
		if err := os.Remove(d.outPath); err != nil {
			return err
		}
	}
	return os.Rename(d.partPath, d.outPath)
}

// verifyChecksum checks the finished partial file against the expected SHA-256.
// It returns the computed digest ("" when no check was requested).
func (d *downloader) verifyChecksum() (string, error) {
	if d.opts.SHA256 == "" {
		return "", nil
	}
	d.infof("verifying SHA-256 of %s", d.outPath)
	sum, err := fileSHA256(d.partPath)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(sum, strings.TrimSpace(d.opts.SHA256)) {
		return sum, fmt.Errorf("%w: expected %s, got %s (kept %s)",
			ErrChecksumMismatch, strings.ToLower(strings.TrimSpace(d.opts.SHA256)), sum, d.partPath)
	}
	return sum, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// startStateFlusher periodically persists resume progress while workers run.
func startStateFlusher(ctx context.Context, wg *sync.WaitGroup, d *downloader) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(stateFlushInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := d.saveState(false); err != nil {
					d.warnf("cannot write resume state: %v", err)
				}
			}
		}
	}()
}

// saveState writes the resume state when the completed-block set changed
// (or unconditionally when force is set).
func (d *downloader) saveState(force bool) error {
	if d.plan == nil {
		return nil
	}
	done := d.plan.doneList()

	d.stateMu.Lock()
	defer d.stateMu.Unlock()
	if !force && slices.Equal(done, d.stateSaved) {
		return nil
	}
	now := time.Now()
	st := &State{
		Version:      stateVersion,
		URL:          d.opts.URL,
		FinalURL:     d.info.FinalURL,
		Size:         d.info.Size,
		ETag:         d.info.ETag,
		LastModified: d.info.LastModified,
		BlockSize:    d.plan.blockSize,
		TotalBlocks:  d.plan.blocks,
		DoneBlocks:   done,
		Output:       d.outPath,
		UpdatedAt:    now,
	}
	if d.state != nil && !d.state.StartedAt.IsZero() {
		st.StartedAt = d.state.StartedAt
	} else {
		st.StartedAt = now
	}
	if err := st.Save(); err != nil {
		return err
	}
	d.stateSaved = done
	return nil
}

// startReporter emits progress snapshots at a fixed rate.
func (d *downloader) startReporter(ctx context.Context, wg *sync.WaitGroup, started time.Time) {
	if d.opts.Progress == nil {
		return
	}
	wg.Add(1)
	conns := len(d.stats.perConn)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(reportInterval)
		defer ticker.Stop()
		aggregate := newSpeedWindow(speedWindowSize)
		windows := make([]*speedWindow, conns)
		for i := range windows {
			windows[i] = newSpeedWindow(speedWindowSize)
		}
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				d.opts.Progress.Update(d.snapshot(now, started, aggregate, windows))
			}
		}
	}()
}

// snapshot builds the current progress view.
func (d *downloader) snapshot(now, started time.Time, aggregate *speedWindow, windows []*speedWindow) Snapshot {
	done := d.stats.total.Load()
	speed := aggregate.add(now, done)

	snap := Snapshot{
		Output:       filepath.Base(d.outPath),
		Total:        d.info.Size,
		SizeKnown:    d.info.Size >= 0,
		Done:         done,
		Workers:      d.opts.Threads,
		Speed:        speed,
		Elapsed:      now.Sub(started),
		SingleStream: d.single,
		Resumed:      d.resumed,
	}
	if d.single {
		snap.Workers = 1
	}
	if d.plan != nil {
		snap.DoneBlocks, snap.Blocks = d.plan.progress()
	}

	active := 0
	snap.Connections = make([]ConnStat, 0, len(windows))
	for i := range windows {
		isActive := d.stats.active[i].Load() > 0
		if isActive {
			active++
		}
		snap.Connections = append(snap.Connections, ConnStat{
			ID:     i + 1,
			Active: isActive,
			Bytes:  d.stats.perConn[i].Load(),
			Speed:  windows[i].add(now, d.stats.perConn[i].Load()),
		})
	}
	snap.ActiveWorkers = active

	if snap.SizeKnown && speed > 0 && done < d.info.Size {
		snap.ETA = time.Duration(float64(d.info.Size-done) / speed * float64(time.Second))
	}
	return snap
}

// firstError drains the worker error channel (non-blocking).
func firstError(ch chan error) error {
	select {
	case err := <-ch:
		return err
	default:
		return nil
	}
}
