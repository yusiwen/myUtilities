package serve

import (
	"crypto/subtle"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileHandler serves one regular file over HTTP.
//
// The file is available at "/" so the URL encoded in a QR code stays short, and
// again under its own name ("/<name>") so the link works when copied by hand.
// Requests are answered with http.ServeContent, which supplies Content-Length,
// Range (resumable downloads), If-Modified-Since and HEAD for free.
type FileHandler struct {
	// Path is the absolute path of the file to serve.
	Path string
	// Name is the download name shown to clients. It defaults to the base name
	// of Path and is not required to match Path, which keeps the URL short.
	Name string
	// ContentType overrides the MIME type derived from the file extension.
	ContentType string
	// Token, when set, must be the first path segment of every request. All
	// other paths answer 404, so only someone holding the link can download.
	Token string
	// ExpiresAt, when non-zero, makes every request after that instant answer
	// 410 Gone.
	ExpiresAt time.Time
	// OnComplete runs once the whole file has been written to a client. A
	// partial Range request or an aborted transfer does not call it, so a
	// caller can implement "one successful download, then stop".
	OnComplete func()
}

// NewFileHandler checks that path is a readable regular file and builds a
// handler for it. It fails on directories so a caller never serves one by
// accident.
func NewFileHandler(path string) (*FileHandler, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("access %s: %w", abs, err)
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", abs)
	}

	name := filepath.Base(abs)
	return &FileHandler{
		Path:        abs,
		Name:        name,
		ContentType: contentTypeFor(name),
	}, nil
}

// ServeHTTP implements http.Handler.
func (h *FileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.expired() {
		http.Error(w, "link expired", http.StatusGone)
		return
	}

	switch r.Method {
	case http.MethodGet, http.MethodHead:
	default:
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check the token before touching the file system: a wrong path and a
	// missing file answer the same way, so probing reveals nothing.
	if !h.matches(r.URL.Path) {
		http.NotFound(w, r)
		return
	}

	f, err := os.Open(h.Path)
	if err != nil {
		http.Error(w, "file unavailable", http.StatusNotFound)
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		http.Error(w, "file unavailable", http.StatusNotFound)
		return
	}

	// attachment makes phones download instead of trying to render the file;
	// mime.FormatMediaType adds the RFC 2231 filename* form for non-ASCII names.
	w.Header().Set("Content-Disposition", attachmentDisposition(h.Name))
	w.Header().Set("Content-Type", h.ContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")

	cw := &countingResponseWriter{ResponseWriter: w, status: http.StatusOK}

	// ServeContent handles Range, If-Range, Content-Length and HEAD. Its name
	// argument is only used for extension sniffing, which Content-Type above
	// already settles.
	http.ServeContent(cw, r, h.Name, fi.ModTime(), f)

	// Only a transfer that delivered the whole file counts. A partial Range
	// request or a client that disconnected mid-way leaves the link usable.
	if h.OnComplete != nil && r.Method == http.MethodGet &&
		cw.status >= 200 && cw.status < 300 && cw.written == fi.Size() {
		h.OnComplete()
	}
}

// expired reports whether the link's lifetime has elapsed.
func (h *FileHandler) expired() bool {
	return !h.ExpiresAt.IsZero() && !time.Now().Before(h.ExpiresAt)
}

// matches reports whether the request path addresses the served file.
func (h *FileHandler) matches(path string) bool {
	if h.Token == "" {
		return path == "/" || path == "/"+h.Name
	}

	rest, ok := h.withoutToken(path)
	if !ok {
		return false
	}
	// "/<token>", "/<token>/" and "/<token>/<name>" all address the file.
	return rest == "" || rest == "/" || rest == "/"+h.Name
}

// withoutToken strips the leading "/<token>" segment from path and reports
// whether it matched. The comparison is constant time so a wrong token cannot
// be refined byte by byte through timing.
func (h *FileHandler) withoutToken(path string) (string, bool) {
	if !strings.HasPrefix(path, "/") {
		return "", false
	}
	rest := path[1:]
	segment := rest
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		segment = rest[:i]
	}
	if subtle.ConstantTimeCompare([]byte(segment), []byte(h.Token)) != 1 {
		return "", false
	}
	return rest[len(segment):], true
}

// countingResponseWriter records the status and the number of body bytes
// actually handed to the client, which is how a complete transfer is detected.
type countingResponseWriter struct {
	http.ResponseWriter
	status  int
	written int64
}

func (c *countingResponseWriter) WriteHeader(code int) {
	c.status = code
	c.ResponseWriter.WriteHeader(code)
}

func (c *countingResponseWriter) Write(p []byte) (int, error) {
	n, err := c.ResponseWriter.Write(p)
	c.written += int64(n)
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (c *countingResponseWriter) Unwrap() http.ResponseWriter {
	return c.ResponseWriter
}

// attachmentDisposition builds the Content-Disposition header for name.
func attachmentDisposition(name string) string {
	if v := mime.FormatMediaType("attachment", map[string]string{"filename": name}); v != "" {
		return v
	}
	return "attachment"
}

// contentTypeFor returns the MIME type for a file name. Anything unknown is
// served as a binary download rather than guessed.
func contentTypeFor(name string) string {
	if ct := mime.TypeByExtension(filepath.Ext(name)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}
