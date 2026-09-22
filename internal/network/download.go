package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yusiwen/myUtilities/internal/core/downloader"
)

// DownloadOptions is the CLI surface for `mu network download`.
type DownloadOptions struct {
	URL string `arg:"" name:"url" help:"URL of the file to download."`

	Output string `short:"o" name:"output" help:"Output file or directory (default: remote filename in the current directory)."`

	Threads     int  `short:"n" name:"threads" help:"Parallel connections (1-32)." default:"4"`
	NoResume    bool `name:"no-resume" help:"Discard any partial file and resume state, then start over."`
	ForceResume bool `name:"force-resume" help:"Resume even when the remote ETag/Last-Modified changed."`
	Force       bool `short:"f" name:"force" help:"Overwrite an existing output file."`

	BlockSize     string `name:"block-size" help:"Work unit size, e.g. 8M or 1MiB (default: derived from the file size)."`
	NoPreallocate bool   `name:"no-preallocate" help:"Do not preallocate the output file up front."`

	Insecure bool     `short:"k" name:"insecure" help:"Skip TLS certificate verification."`
	CACert   string   `name:"cacert" help:"PEM file with extra trusted CA certificates (added to the system roots)."`
	Headers  []string `short:"H" name:"header" help:"Extra request header as Key: Value (repeatable)."`
	Auth     string   `short:"A" name:"auth" help:"Bearer token; sets Authorization: Bearer <token>."`
	User     string   `name:"user" help:"HTTP basic auth as user:password."`
	Proxy    string   `name:"proxy" help:"HTTP(S) proxy URL."`

	Timeout     string `short:"t" name:"timeout" help:"Connect/TLS/response-header timeout (e.g. 30s)." default:"30s"`
	IdleTimeout string `name:"idle-timeout" help:"Abort a connection that stops sending data for this long." default:"30s"`
	Retries     int    `name:"retries" help:"Retries per block." default:"3"`
	LimitRate   string `name:"limit-rate" help:"Global speed cap, e.g. 5M or 500k (default: unlimited)."`
	SHA256      string `name:"sha256" help:"Expected SHA-256 checksum; the download fails on mismatch."`
	HTTP1       bool   `name:"http1" help:"Force HTTP/1.1 (disable HTTP/2 connection multiplexing)."`

	NoProgress bool `name:"no-progress" help:"Disable the live progress display."`
	Verbose    bool `name:"verbose" help:"Show one progress line per connection (-v is the global version flag)."`
	Quiet      bool `short:"q" name:"quiet" help:"Only print errors."`
	JSON       bool `name:"json" help:"Print the result as JSON."`
}

// Run handles `mu network download`.
func (o *DownloadOptions) Run() error {
	if o.Threads < 1 {
		return fmt.Errorf("invalid --threads %d: must be at least 1", o.Threads)
	}
	if o.Threads > downloader.MaxThreads {
		fmt.Fprintf(os.Stderr, "warning: --threads %d exceeds the maximum of %d; using %d\n",
			o.Threads, downloader.MaxThreads, downloader.MaxThreads)
		o.Threads = downloader.MaxThreads
	}

	timeout, err := time.ParseDuration(o.Timeout)
	if err != nil {
		return fmt.Errorf("invalid --timeout: %w", err)
	}
	idleTimeout, err := time.ParseDuration(o.IdleTimeout)
	if err != nil {
		return fmt.Errorf("invalid --idle-timeout: %w", err)
	}
	blockSize, err := parseOptionalSize("--block-size", o.BlockSize)
	if err != nil {
		return err
	}
	limitRate, err := parseOptionalSize("--limit-rate", o.LimitRate)
	if err != nil {
		return err
	}

	renderer := newProgressRenderer(os.Stderr, o.Verbose, o.Quiet || o.NoProgress)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	res, err := downloader.Download(ctx, downloader.Options{
		URL:         o.URL,
		Output:      o.Output,
		Threads:     o.Threads,
		BlockSize:   blockSize,
		Resume:      !o.NoResume,
		ForceResume: o.ForceResume,
		Force:       o.Force,
		Preallocate: !o.NoPreallocate,
		Headers:     o.Headers,
		Auth:        o.Auth,
		User:        o.User,
		Proxy:       o.Proxy,
		Timeout:     timeout,
		IdleTimeout: idleTimeout,
		Retries:     o.Retries,
		LimitRate:   limitRate,
		Insecure:    o.Insecure,
		CACert:      o.CACert,
		HTTP1:       o.HTTP1,
		SHA256:      o.SHA256,
		Progress:    renderer,
		Infof:       renderer.Log,
		Warnf:       renderer.Warn,
	})

	// The live region must be cleared before anything else is printed.
	renderer.Close()

	if err != nil {
		if errors.Is(err, downloader.ErrInterrupted) {
			fmt.Fprintln(os.Stderr, "download interrupted; run the same command again to resume")
			// 128 + SIGINT, the shell convention for an interrupted process.
			os.Exit(130)
		}
		return err
	}
	return o.report(res)
}

func (o *DownloadOptions) report(res *downloader.Result) error {
	if o.JSON {
		payload := struct {
			Path         string  `json:"path"`
			Size         int64   `json:"size"`
			Bytes        int64   `json:"bytes"`
			ElapsedMS    int64   `json:"elapsed_ms"`
			AvgSpeedBps  float64 `json:"avg_speed_bps"`
			Threads      int     `json:"threads"`
			Resumed      bool    `json:"resumed"`
			SingleStream bool    `json:"single_stream"`
			SHA256       string  `json:"sha256,omitempty"`
		}{
			Path:         res.Path,
			Size:         res.Size,
			Bytes:        res.Bytes,
			ElapsedMS:    res.Elapsed.Milliseconds(),
			AvgSpeedBps:  res.AvgSpeed,
			Threads:      res.Threads,
			Resumed:      res.Resumed,
			SingleStream: res.SingleStream,
			SHA256:       res.SHA256,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(payload)
	}

	if o.Quiet {
		return nil
	}

	mode := fmt.Sprintf("%d connection(s)", res.Threads)
	if res.SingleStream {
		mode = "single connection"
	}
	extra := ""
	if res.Resumed {
		extra += fmt.Sprintf(", resumed %s", downloader.FormatSize(res.Bytes))
	}
	if res.SHA256 != "" {
		extra += ", sha256 verified"
	}
	savedSize := res.Bytes
	if res.Size >= 0 {
		savedSize = res.Size
	}
	fmt.Fprintf(os.Stderr, "saved %s (%s in %s, %s, %s%s)\n",
		res.Path,
		downloader.FormatSize(savedSize),
		res.Elapsed.Round(time.Millisecond),
		formatSpeed(res.AvgSpeed),
		mode,
		extra,
	)
	return nil
}

// parseOptionalSize parses an optional size flag, blaming the right flag name.
func parseOptionalSize(flag, value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	size, err := downloader.ParseSize(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", flag, err)
	}
	return size, nil
}
