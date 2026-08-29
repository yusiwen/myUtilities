// Package httpclient provides a minimal HTTP client for ad-hoc API calls.
// It is the business-logic layer behind the `mu http` command.
package httpclient

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/morikuni/aec"
)

var noColor bool

func init() {
	if os.Getenv("NO_COLOR") != "" {
		noColor = true
	}
}

func faint(s string) string {
	if noColor {
		return s
	}
	return aec.Apply(s, aec.Faint)
}

func bold(s string) string {
	if noColor {
		return s
	}
	return aec.Apply(s, aec.Bold)
}

func green(s string) string {
	if noColor {
		return s
	}
	return aec.Apply(s, aec.GreenF)
}

func red(s string) string {
	if noColor {
		return s
	}
	return aec.Apply(s, aec.RedF)
}

// Params describes a single HTTP request.
type Params struct {
	URL      string
	Method   string
	Headers  []string
	Body     string
	Auth     string
	Timeout  time.Duration
	Insecure bool
	NoFollow bool
	JSON     bool
	BodyOnly bool
	Output   string
}

// Result holds the response of a request.
type Result struct {
	StatusCode int
	Status     string
	Header     http.Header
	Body       []byte
	Elapsed    time.Duration
}

// Do executes the request described by p and returns the result.
func Do(p Params) (*Result, error) {
	// Build request.
	req, err := http.NewRequest(p.Method, p.URL, strings.NewReader(p.Body))
	if err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Set custom headers.
	for _, h := range p.Headers {
		idx := strings.IndexByte(h, ':')
		if idx < 0 {
			return nil, fmt.Errorf("invalid header format %q: expected \"Key: Value\"", h)
		}
		key := strings.TrimSpace(h[:idx])
		val := strings.TrimSpace(h[idx+1:])
		req.Header.Set(key, val)
	}

	// Bearer auth.
	if p.Auth != "" {
		req.Header.Set("Authorization", "Bearer "+p.Auth)
	}

	// Set Content-Type only if not already set.
	if p.Body != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// Build client.
	client := &http.Client{
		Timeout: p.Timeout,
	}
	if p.Insecure {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		}
	}
	if p.NoFollow {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	// Execute.
	deadline := time.Now().Add(p.Timeout)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	elapsed := time.Since(deadline.Add(-p.Timeout))
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return &Result{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Header:     resp.Header,
		Body:       respBody,
		Elapsed:    elapsed,
	}, nil
}

// ReadBodyFromStdin returns the stdin contents when not a TTY.
func ReadBodyFromStdin() (string, error) {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return "", nil
	}
	if fi.Mode()&os.ModeCharDevice != 0 {
		return "", nil
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	return string(data), nil
}

// Render formats the result as a human-readable response:
// status line, headers (in faint), and body (pretty-printed if JSON).
// If p.Output is set, the body is written to the file and not printed.
func Render(p Params, r *Result) string {
	var out strings.Builder

	if !p.BodyOnly {
		statusLine := fmt.Sprintf("HTTP/1.1 %s", r.Status)
		switch {
		case r.StatusCode >= 400:
			out.WriteString(red(bold(statusLine + "\n")))
		case r.StatusCode >= 300:
			out.WriteString(bold(statusLine + "\n"))
		default:
			out.WriteString(green(bold(statusLine + "\n")))
		}
		for k, v := range r.Header {
			out.WriteString(faint(fmt.Sprintf("%s: %s\n", k, strings.Join(v, ", "))))
		}
		out.WriteString("\n")
	}

	if p.Output != "" {
		if err := os.WriteFile(p.Output, r.Body, 0644); err != nil {
			out.WriteString(red(fmt.Sprintf("write output file: %v\n", err)))
			return out.String()
		}
		if !p.BodyOnly {
			out.WriteString(faint(fmt.Sprintf("[response saved to %s]\n", p.Output)))
		}
	} else {
		if p.JSON || IsJSON(r.Header.Get("Content-Type"), r.Body) {
			out.WriteString(PrettyJSON(r.Body))
		} else {
			out.Write(r.Body)
		}
	}

	return out.String()
}

// SummaryLine returns the one-line summary printed to stderr.
func SummaryLine(p Params, r *Result) string {
	return faint(fmt.Sprintf("%s %s → %d (%s)", p.Method, p.URL, r.StatusCode, r.Elapsed.Round(time.Millisecond)))
}

// IsJSON reports whether the response looks like JSON.
func IsJSON(contentType string, body []byte) bool {
	if strings.Contains(contentType, "json") {
		return true
	}
	trimmed := bytes.TrimSpace(body)
	return len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[')
}

// PrettyJSON pretty-prints a JSON byte slice; non-JSON passes through.
func PrettyJSON(data []byte) string {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return string(data)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(data)
	}
	return string(out) + "\n"
}

// Execute performs the full request + rendering + output.
func Execute(p Params, stdout, stderr io.Writer) error {
	res, err := Do(p)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprint(stdout, Render(p, res))
	_, _ = fmt.Fprintln(stderr, SummaryLine(p, res))
	return nil
}
