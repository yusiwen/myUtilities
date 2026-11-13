package serve

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	gqrcode "github.com/skip2/go-qrcode"
	"github.com/yusiwen/myUtilities/internal/core/qr"
	coreserve "github.com/yusiwen/myUtilities/internal/core/serve"
	"golang.org/x/term"
)

// Options is the `mu serve` command.
//
// The positional argument accepts either a directory (static file server) or a
// single regular file (one-off download for phones on the same LAN).
type Options struct {
	Path  string `arg:"" optional:"" name:"path" help:"Directory or single file to serve." default:"."`
	Port  int    `short:"p" help:"Port to listen on." default:"8080"`
	Host  string `help:"Address to bind to (default: all interfaces)."`
	Iface string `help:"Network interface whose address is advertised in the QR code."`

	// The temporary-share options only apply to a single file.
	Token       string        `help:"Require this token as the URL path (single-file mode)."`
	RandomToken bool          `name:"random-token" help:"Generate a random token instead of using --token (single-file mode)."`
	Once        bool          `help:"Exit after the first complete download (single-file mode)."`
	Expire      time.Duration `help:"Expire the link after this duration, e.g. 10m (single-file mode)." default:"0"`

	QR      bool `negatable:"" default:"true" help:"Draw a QR code for the served URL (terminal only)."`
	CORS    bool `help:"Enable CORS headers for all origins."`
	Verbose bool `help:"Log requests to stderr."`
}

// targetKind says whether the positional argument is a directory or one file.
type targetKind int

const (
	targetDir targetKind = iota
	targetFile
)

// target is the resolved thing to serve.
type target struct {
	path string
	kind targetKind
	size int64
}

func (t target) isDir() bool { return t.kind == targetDir }

// resolveTarget turns the command-line path into an absolute target and rejects
// anything that is neither a directory nor a regular file.
func resolveTarget(path string) (target, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return target{}, fmt.Errorf("resolve path: %w", err)
	}

	fi, err := os.Stat(abs)
	if err != nil {
		return target{}, fmt.Errorf("access %s: %w", abs, err)
	}

	switch {
	case fi.IsDir():
		return target{path: abs, kind: targetDir}, nil
	case fi.Mode().IsRegular():
		return target{path: abs, kind: targetFile, size: fi.Size()}, nil
	default:
		return target{}, fmt.Errorf("%s is neither a regular file nor a directory", abs)
	}
}

func (o *Options) Run() error {
	t, err := resolveTarget(o.Path)
	if err != nil {
		return err
	}
	if t.isDir() {
		return o.runDir(t)
	}
	return o.runFile(t)
}

// runDir serves a directory tree.
func (o *Options) runDir(t target) error {
	if err := o.validateDirMode(); err != nil {
		return err
	}

	ln, port, err := o.listen()
	if err != nil {
		return err
	}

	primary := o.announce(t, port, "")
	o.printQR(primary)

	// Downloads get no wall-clock cap, so a large file is not truncated.
	server := coreserve.NewDownloadServer(o.addr(), o.wrap(http.FileServer(http.Dir(t.path))))
	return server.Serve(ln)
}

// runFile serves one file at "/" and at "/<name>", behind the token, expiry and
// single-use rules when they are enabled.
func (o *Options) runFile(t target) error {
	if o.Expire < 0 {
		return fmt.Errorf("--expire must not be negative")
	}
	token, err := o.resolveToken()
	if err != nil {
		return err
	}

	handler, err := coreserve.NewFileHandler(t.path)
	if err != nil {
		return err
	}
	handler.Token = token
	if o.Expire > 0 {
		handler.ExpiresAt = time.Now().Add(o.Expire)
	}

	ln, port, err := o.listen()
	if err != nil {
		return err
	}

	primary := o.announce(t, port, token)
	o.printQR(primary)

	// done is only ever closed when the link is single use; otherwise the
	// server keeps serving after a download.
	done := make(chan struct{})
	handler.OnComplete = o.onComplete(done)

	server := coreserve.NewDownloadServer(o.addr(), o.wrap(handler, token))
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(ln) }()

	var expired <-chan time.Time
	if o.Expire > 0 {
		expired = time.After(o.Expire)
	}

	select {
	case err := <-serveErr:
		return err
	case <-done:
		fmt.Println("Download complete; shutting down.")
	case <-expired:
		fmt.Println("Link expired; shutting down.")
	}

	o.shutdown(server)
	<-serveErr
	return nil
}

// onComplete returns the callback that stops the server after the first
// complete download, or nil when the link is not single use. sync.Once keeps
// two concurrent downloads from closing done twice.
func (o *Options) onComplete(done chan struct{}) func() {
	if !o.Once {
		return nil
	}
	var once sync.Once
	return func() { once.Do(func() { close(done) }) }
}

// validateDirMode rejects the single-file options in directory mode instead of
// silently ignoring them.
func (o *Options) validateDirMode() error {
	switch {
	case o.Token != "" || o.RandomToken:
		return fmt.Errorf("--token and --random-token apply to a single file, not a directory")
	case o.Once:
		return fmt.Errorf("--once applies to a single file, not a directory")
	case o.Expire > 0:
		return fmt.Errorf("--expire applies to a single file, not a directory")
	}
	return nil
}

// resolveToken returns the download token, generating one when requested.
func (o *Options) resolveToken() (string, error) {
	if o.Token != "" && o.RandomToken {
		return "", fmt.Errorf("use either --token or --random-token, not both")
	}
	if o.Token != "" {
		if err := coreserve.ValidateToken(o.Token); err != nil {
			return "", fmt.Errorf("--token: %w", err)
		}
		return o.Token, nil
	}
	if o.RandomToken {
		token, err := coreserve.NewToken()
		if err != nil {
			return "", err
		}
		return token, nil
	}
	return "", nil
}

// shutdown stops the server, giving in-flight transfers up to five seconds to
// finish before the connections are forced closed.
func (o *Options) shutdown(server *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		server.Close()
	}
}

// wrap applies the optional CORS and request logging middleware. Secrets are
// redacted from the log because a token in a request path is not something to
// leave lying around in a terminal scrollback.
func (o *Options) wrap(h http.Handler, secrets ...string) http.Handler {
	if o.CORS {
		h = corsMiddleware(h)
	}
	if o.Verbose {
		h = loggingMiddleware(h, secrets...)
	}
	return h
}

// addr is the listen address handed to the server.
func (o *Options) addr() string {
	return net.JoinHostPort(o.Host, strconv.Itoa(o.Port))
}

// listen binds the port before anything is announced, so a busy port fails
// without printing URLs that do not work. It also reports the port actually
// bound, which differs from the flag when --port 0 is used.
func (o *Options) listen() (net.Listener, int, error) {
	ln, err := net.Listen("tcp", o.addr())
	if err != nil {
		return nil, 0, fmt.Errorf("listen on %s: %w", o.addr(), err)
	}
	port := o.Port
	if tcp, ok := ln.Addr().(*net.TCPAddr); ok {
		port = tcp.Port
	}
	return ln, port, nil
}

// announce prints where the server can be reached and returns the URL a QR code
// should encode, or "" when no reachable LAN address exists.
func (o *Options) announce(t target, port int, token string) string {
	name := filepath.Base(t.path)

	if t.isDir() {
		fmt.Printf("Serving %s on http://localhost:%d\n", t.path, port)
	} else {
		fmt.Printf("Serving %s (%s)\n", t.path, formatBytes(t.size))
	}

	addrs, err := coreserve.Advertise(o.Iface)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}

	primary, localOnly, rest := choosePrimary(o.Host, addrs, port)
	local := withToken(urlFor("localhost", port), token)
	switch {
	case primary == "":
		fmt.Fprintf(os.Stderr, "warning: no LAN address found; only this machine can reach the server\n")
	case localOnly:
		fmt.Fprintf(os.Stderr, "warning: bound to a loopback address, a phone cannot reach the server\n")
		local = withToken(primary, token)
	}

	qrURL := ""
	if primary != "" && !localOnly {
		qrURL = withToken(primary, token)
		fmt.Printf("  URL:   %s\n", qrURL)
		if !t.isDir() {
			fmt.Printf("  same:  %s\n", joinURL(qrURL, url.PathEscape(name)))
		}
		for _, a := range rest {
			fmt.Printf("  alt:   %s (%s)\n", withToken(urlFor(a.IP.String(), port), token), a.Interface)
		}
	}

	if !t.isDir() {
		fmt.Printf("  local: %s\n", local)
		fmt.Println("Press Ctrl+C to stop.")
	}

	// State the access policy so the banner matches what the server enforces.
	if token != "" {
		fmt.Println("  Link requires the token; other paths answer 404.")
	}
	if o.Once {
		fmt.Println("  Single use: the server stops after one complete download.")
	}
	if o.Expire > 0 {
		fmt.Printf("  Expires at %s (in %s).\n", time.Now().Add(o.Expire).Format("15:04:05"), o.Expire)
	}

	return qrURL
}

// printQR renders url as a QR code when the flag allows it and stdout is a
// terminal, so pipes and log files never receive escape sequences.
func (o *Options) printQR(rawURL string) {
	cols := 0
	if c, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		cols = c
	}
	o.printQRTo(os.Stdout, rawURL, isTerminal(os.Stdout), cols)
}

// printQRTo is printQR with the terminal facts passed in, which keeps the
// drawing rules testable without a pseudo-terminal.
func (o *Options) printQRTo(w io.Writer, rawURL string, tty bool, cols int) {
	if !o.QR || rawURL == "" || !tty {
		return
	}

	rendered, err := qr.Terminal(rawURL, gqrcode.Medium)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: cannot render QR code: %v\n", err)
		return
	}
	// A wrapped QR code cannot be scanned, and terminals wrap by width.
	if cols > 0 && rendered.Width > cols {
		fmt.Fprintf(os.Stderr, "warning: QR code needs %d columns but the terminal has %d; widen the window\n", rendered.Width, cols)
		fmt.Fprintf(os.Stderr, "or write a PNG instead: mu qrcode gen -o qr.png %q\n", rawURL)
		return
	}

	fmt.Fprintln(w, "Scan with your phone:")
	fmt.Fprint(w, rendered.Art)
}

// choosePrimary decides which URL a QR code should encode. An explicit bind
// address wins because it is the only address guaranteed to be listened on;
// otherwise the best LAN address is used.
func choosePrimary(host string, addrs []coreserve.Addr, port int) (primary string, localOnly bool, rest []coreserve.Addr) {
	if strings.EqualFold(host, "localhost") {
		return urlFor("localhost", port), true, nil
	}
	if ip := net.ParseIP(host); ip != nil && !ip.IsUnspecified() {
		return urlFor(ip.String(), port), ip.IsLoopback(), nil
	}
	if len(addrs) > 0 {
		return urlFor(addrs[0].IP.String(), port), false, addrs[1:]
	}
	return "", true, nil
}

// urlFor builds a base URL for host:port.
func urlFor(host string, port int) string {
	return fmt.Sprintf("http://%s/", net.JoinHostPort(host, strconv.Itoa(port)))
}

// withToken appends the token segment to a base URL when one is in use.
func withToken(base, token string) string {
	if token == "" {
		return base
	}
	return joinURL(base, token)
}

// joinURL appends a segment to a base URL without doubling the separator.
func joinURL(base, segment string) string {
	return strings.TrimSuffix(base, "/") + "/" + segment
}

// formatBytes renders a byte count in IEC units for the startup banner.
func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	value := float64(n)
	idx := -1
	for value >= unit && idx < len(units)-1 {
		value /= unit
		idx++
	}
	return fmt.Sprintf("%.1f %s", value, units[idx])
}

// isTerminal reports whether f is a terminal.
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler, secrets ...string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &loggingResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(lw, r)
		log.Printf("[%s] %s %s %d %v",
			start.Format("2006-01-02 15:04:05"),
			r.Method, redact(r.URL.Path, secrets), lw.statusCode, time.Since(start))
	})
}

// redact replaces secrets in a logged path.
func redact(path string, secrets []string) string {
	for _, s := range secrets {
		if s != "" {
			path = strings.ReplaceAll(path, s, "<token>")
		}
	}
	return path
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (lw *loggingResponseWriter) WriteHeader(code int) {
	lw.statusCode = code
	lw.ResponseWriter.WriteHeader(code)
}
