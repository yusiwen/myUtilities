package httpclient

import (
	"bytes"
	"context"
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

// Options holds the CLI flags for `mu http`.
type Options struct {
	URL      string   `arg:"" name:"url" help:"Target URL." required:""`
	Method   string   `short:"X" name:"method" help:"HTTP method." default:"GET" enum:"GET,POST,PUT,PATCH,DELETE,HEAD,OPTIONS"`
	Headers  []string `short:"H" name:"header" help:"Request headers as Key: Value (repeatable)."`
	Data     string   `short:"d" name:"data" help:"Request body (or pipe from stdin)."`
	Auth     string   `short:"A" name:"auth" help:"Bearer token for Authorization header."`
	Timeout  string   `short:"t" name:"timeout" help:"Request timeout (e.g. 30s, 2m)." default:"30s"`
	Insecure bool     `short:"k" name:"insecure" help:"Skip TLS certificate verification."`
	NoFollow bool     `short:"N" name:"no-follow" help:"Do not follow redirects."`
	JSON     bool     `short:"j" name:"json" help:"Force pretty-print JSON response."`
	BodyOnly bool     `short:"b" name:"body" help:"Print only the response body (no headers/status)."`
	Output   string   `short:"o" name:"output" help:"Write response body to file instead of stdout."`
}

// Run executes the HTTP request and prints the result.
func (o *Options) Run() error {
	timeout, err := time.ParseDuration(o.Timeout)
	if err != nil {
		return fmt.Errorf("invalid timeout %q: %w", o.Timeout, err)
	}

	// Read body from stdin if -d was not provided.
	body := o.Data
	if body == "" {
		if fi, _ := os.Stdin.Stat(); fi != nil && (fi.Mode()&os.ModeCharDevice) == 0 {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("read stdin: %w", err)
			}
			body = string(data)
		}
	}

	// Build request.
	req, err := http.NewRequest(o.Method, o.URL, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("invalid request: %w", err)
	}

	// Set custom headers.
	for _, h := range o.Headers {
		idx := strings.IndexByte(h, ':')
		if idx < 0 {
			return fmt.Errorf("invalid header format %q: expected \"Key: Value\"", h)
		}
		key := strings.TrimSpace(h[:idx])
		val := strings.TrimSpace(h[idx+1:])
		req.Header.Set(key, val)
	}

	// Bearer auth.
	if o.Auth != "" {
		req.Header.Set("Authorization", "Bearer "+o.Auth)
	}

	// Set Content-Type only if not already set.
	if body != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// Build client.
	client := &http.Client{
		Timeout: timeout,
	}
	if o.Insecure {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		}
	}
	if o.NoFollow {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	// Execute.
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	elapsed := time.Since(start)
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	// Write body to file if -o specified.
	if o.Output != "" {
		if err := os.WriteFile(o.Output, respBody, 0644); err != nil {
			return fmt.Errorf("write output file: %w", err)
		}
		if o.BodyOnly {
			return nil
		}
	}

	// Determine if body is JSON.
	isJSON := o.JSON || isJSONContent(resp.Header.Get("Content-Type"), respBody)

	// Build output.
	var out strings.Builder

	if !o.BodyOnly {
		statusLine := fmt.Sprintf("HTTP/1.1 %s", resp.Status)
		switch {
		case resp.StatusCode >= 400:
			out.WriteString(red(bold(statusLine + "\n")))
		case resp.StatusCode >= 300:
			out.WriteString(bold(statusLine + "\n"))
		default:
			out.WriteString(green(bold(statusLine + "\n")))
		}
		for k, v := range resp.Header {
			out.WriteString(faint(fmt.Sprintf("%s: %s\n", k, strings.Join(v, ", "))))
		}
		out.WriteString("\n")
	}

	if o.Output != "" && !o.BodyOnly {
		out.WriteString(faint(fmt.Sprintf("[response saved to %s]\n", o.Output)))
	} else {
		if isJSON {
			out.WriteString(prettyJSON(respBody))
		} else {
			out.Write(respBody)
		}
	}

	// Summary to stderr.
	fmt.Fprintln(os.Stderr, faint(
		fmt.Sprintf("%s %s → %d (%s)", o.Method, o.URL, resp.StatusCode, elapsed.Round(time.Millisecond)),
	))

	fmt.Print(out.String())
	return nil
}

func isJSONContent(contentType string, body []byte) bool {
	if strings.Contains(contentType, "json") {
		return true
	}
	trimmed := bytes.TrimSpace(body)
	return len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[')
}

func prettyJSON(data []byte) string {
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
