package diff

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/yusiwen/myUtilities/internal/core/browser"
	"github.com/yusiwen/myUtilities/internal/core/httpserver"
	"github.com/yusiwen/myUtilities/internal/core/patch"
	"golang.org/x/term"
)

// maxSourceFileBytes caps one preloaded file. The web UI receives both files in
// one JSON response and CodeMirror's merge view degrades badly on multi-megabyte
// documents, so anything larger is rejected instead of truncated: a truncated
// diff would silently compare the wrong thing.
const maxSourceFileBytes = 5 << 20

type Options struct {
	// Bare is the hidden default command Kong selects when `mu diff` is called
	// without a subcommand. It makes `mu diff a.txt b.txt` and `mu diff patchfile`
	// work, which is also the shape `git difftool` invokes. The name is internal;
	// hidden commands do not appear in help.
	Bare BareOptions `cmd:"" name:"bare" hidden:"" default:"withargs"`

	File  FileOptions  `cmd:"" name:"file" aliases:"f" help:"Compare two files."`
	Text  TextOptions  `cmd:"" name:"text" aliases:"t" help:"Compare two text strings."`
	Serve ServeOptions `cmd:"" name:"serve" help:"Start diff tool HTTP server."`
	Merge MergeOptions `cmd:"" name:"merge" help:"Three-way merge of text files."`
	Patch PatchOptions `cmd:"" name:"patch" help:"Render a unified diff from stdin or a file (same as the default mode)."`
}

// BareOptions is the default `mu diff` mode:
//
//	mu diff a.txt b.txt      compare two files
//	mu diff change.patch     render a unified diff file
//	mu diff                  render a unified diff from stdin
//
// The third form is what a pager needs; the first is what `git difftool` passes
// ($LOCAL and $REMOTE).
type BareOptions struct {
	Files       []string `arg:"" optional:"" name:"file" help:"Two files to compare, or one unified diff file (default: stdin)."`
	Color       string   `help:"Color output: auto, always, never." enum:"auto,always,never" default:"auto"`
	LineNumbers bool     `negatable:"" default:"true" help:"Show old and new line numbers."`
	TUI         bool     `help:"Open an interactive file list and diff viewer (requires a terminal)."`
	Theme       string   `help:"Color theme." enum:"default,nord" default:"nord"`
}

func (o *BareOptions) Run() error {
	switch len(o.Files) {
	case 0:
		// A terminal stdin means no patch was piped in; reading it would hang.
		if term.IsTerminal(int(os.Stdin.Fd())) {
			return errors.New("no input: pass two files to compare, a unified diff file, or pipe a diff on stdin (run 'mu diff --help')")
		}
		return o.runPatch("")
	case 1:
		return o.runPatch(o.Files[0])
	case 2:
		files, err := compareFiles(o.Files[0], o.Files[1])
		if err != nil {
			return err
		}
		if len(files) == 0 {
			return nil
		}
		return renderFiles(files, o.renderConfig(), os.Stdin, os.Stdout)
	default:
		return fmt.Errorf("expected two files to compare, a single unified diff file, or stdin; got %d arguments", len(o.Files))
	}
}

// runPatch reuses the patch renderer, including its exit code of 2 for a read or
// parse error.
func (o *BareOptions) runPatch(file string) error {
	return (&PatchOptions{
		File:        file,
		Color:       o.Color,
		LineNumbers: o.LineNumbers,
		TUI:         o.TUI,
		Theme:       o.Theme,
	}).Run()
}

func (o *BareOptions) renderConfig() renderConfig {
	return renderConfig{
		color:       o.Color,
		lineNumbers: o.LineNumbers,
		tui:         o.TUI,
		theme:       patchTheme(o.Theme),
		title:       "mu diff",
	}
}

// compareFiles reads both sides and builds the unified diff model.
func compareFiles(oldPath, newPath string) ([]patch.File, error) {
	oldText, err := readDiffInput(oldPath)
	if err != nil {
		return nil, err
	}
	newText, err := readDiffInput(newPath)
	if err != nil {
		return nil, err
	}
	return patch.FromTexts(oldPath, newPath, oldText, newText), nil
}

// readDiffInput reads one side of a comparison. The size cap matches the patch
// renderer because the whole diff is held in memory.
func readDiffInput(path string) ([]byte, error) {
	if isNullDevice(path) {
		return nil, nil
	}
	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if fi.IsDir() {
		return nil, fmt.Errorf("%s is a directory; comparing two trees is not supported", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(data) > maxPatchInputBytes {
		return nil, fmt.Errorf("%s is larger than the %d MiB diff limit", path, maxPatchInputBytes>>20)
	}
	return data, nil
}

// isNullDevice reports whether path names the platform null device, which is how
// git difftool represents an added or deleted file.
func isNullDevice(path string) bool {
	if path == os.DevNull || path == "/dev/null" {
		return true
	}
	return runtime.GOOS == "windows" && strings.EqualFold(path, "NUL")
}

type FileOptions struct {
	File1 string `arg:"" name:"file1" help:"First file."`
	File2 string `arg:"" name:"file2" help:"Second file."`
	Theme string `help:"Color theme." enum:"default,nord" default:"nord"`
}

type TextOptions struct {
	Text1 string `arg:"" name:"text1" help:"First text."`
	Text2 string `arg:"" optional:"" name:"text2" help:"Second text (omit for stdin)."`
	Theme string `help:"Color theme." enum:"default,nord" default:"nord"`
}

type ServeOptions struct {
	Files []string `arg:"" optional:"" name:"files" help:"Two files to compare in the browser."`
	Port  int      `help:"Port to listen on (0 picks a free port)." default:"8088"`
	Host  string   `help:"Address to bind to (default: 127.0.0.1 with files, all interfaces without)."`
	Open  bool     `negatable:"" default:"true" help:"Open the default browser when files are given."`
}

func (o *FileOptions) Run() error {
	a, err := os.ReadFile(o.File1)
	if err != nil {
		return fmt.Errorf("read %s: %w", o.File1, err)
	}
	b, err := os.ReadFile(o.File2)
	if err != nil {
		return fmt.Errorf("read %s: %w", o.File2, err)
	}
	fmt.Print(diffText(string(a), string(b), patchTheme(o.Theme)))
	return nil
}

func (o *TextOptions) Run() error {
	text1 := o.Text1
	text2 := o.Text2
	if text2 == "" && !term.IsTerminal(int(os.Stdin.Fd())) {
		data, _ := io.ReadAll(os.Stdin)
		text2 = strings.TrimRight(string(data), "\n\r")
	}
	fmt.Print(diffText(text1, text2, patchTheme(o.Theme)))
	return nil
}

func (o *ServeOptions) Run() error {
	files, err := o.filePair()
	if err != nil {
		return err
	}
	src := newSource(files)
	// Fail before binding the port: a typo in a path should not start a server.
	if src != nil {
		if _, err := src.load(); err != nil {
			return err
		}
	}

	addr := o.listenAddr()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	pageURL := "http://" + net.JoinHostPort(o.displayHost(), strconv.Itoa(port)) + "/"

	mux := http.NewServeMux()
	mux.Handle("/", FrontendHandler())
	RegisterHandlersWithSource(mux, src)

	fmt.Printf("Diff tool server listening on %s\n", pageURL)
	if src != nil {
		fmt.Printf("Comparing %s and %s\n", src.Files[0], src.Files[1])
		if !o.loopback() {
			fmt.Fprintln(os.Stderr, "warning: bound to a non-loopback address; anyone who can reach this port can read the compared files")
		}
	}
	fmt.Println("Press Ctrl+C to stop.")

	// Open only after the listener is bound, so the first request cannot race a
	// server that is not accepting yet.
	if src != nil && o.Open {
		if err := browser.Open(pageURL); err != nil {
			fmt.Fprintf(os.Stderr, "warning: cannot open the browser: %v\n", err)
		}
	}

	return httpserver.New(addr, mux).Serve(ln)
}

// filePair validates the positional arguments: no files (empty editor) or
// exactly two files to compare.
func (o *ServeOptions) filePair() ([]string, error) {
	switch len(o.Files) {
	case 0:
		return nil, nil
	case 2:
		return o.Files, nil
	default:
		return nil, fmt.Errorf("expected two files to compare, got %d", len(o.Files))
	}
}

func (o *ServeOptions) listenAddr() string {
	return net.JoinHostPort(o.listenHost(), strconv.Itoa(o.Port))
}

// listenHost defaults to loopback once local file contents are served: binding
// every interface would publish the files to the whole network.
func (o *ServeOptions) listenHost() string {
	if o.Host != "" {
		return o.Host
	}
	if len(o.Files) == 2 {
		return "127.0.0.1"
	}
	return ""
}

func (o *ServeOptions) displayHost() string {
	switch h := o.listenHost(); h {
	case "", "0.0.0.0", "::":
		return "localhost"
	default:
		return h
	}
}

func (o *ServeOptions) loopback() bool {
	switch o.listenHost() {
	case "127.0.0.1", "::1", "localhost":
		return true
	}
	return false
}

// Source is the pair of files `mu diff serve` preloads into the web UI.
type Source struct {
	Files []string
}

func newSource(files []string) *Source {
	if len(files) != 2 {
		return nil
	}
	return &Source{Files: files}
}

// sourcePayload is the JSON body of GET /api/diff/source.
type sourcePayload struct {
	File1 string `json:"file1"`
	File2 string `json:"file2"`
	Text1 string `json:"text1"`
	Text2 string `json:"text2"`
}

// load reads both files. They are re-read per request, so reloading the page
// after editing them shows the new content.
func (s *Source) load() (*sourcePayload, error) {
	if s == nil {
		return nil, nil
	}
	texts := make([]string, len(s.Files))
	for i, path := range s.Files {
		data, err := readSourceFile(path)
		if err != nil {
			return nil, err
		}
		texts[i] = string(data)
	}
	return &sourcePayload{
		File1: filepath.Base(s.Files[0]),
		File2: filepath.Base(s.Files[1]),
		Text1: texts[0],
		Text2: texts[1],
	}, nil
}

// readSourceFile rejects input that cannot render in a text merge view.
func readSourceFile(path string) ([]byte, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("access %s: %w", path, err)
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	if fi.Size() > maxSourceFileBytes {
		return nil, fmt.Errorf("%s is %.1f MiB; the web diff view is limited to %d MiB",
			path, float64(fi.Size())/(1<<20), maxSourceFileBytes>>20)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	// The file may have grown between Stat and ReadFile.
	if int64(len(data)) > maxSourceFileBytes {
		return nil, fmt.Errorf("%s is larger than the %d MiB web diff limit", path, maxSourceFileBytes>>20)
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return nil, fmt.Errorf("%s looks like a binary file, not text", path)
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("%s is not valid UTF-8; convert it first, for example: iconv -f GBK -t UTF-8 %s", path, path)
	}
	return data, nil
}

// RegisterHandlers registers the diff API without preloaded content. The gateway
// uses it; `mu diff serve` uses RegisterHandlersWithSource.
func RegisterHandlers(mux *http.ServeMux) {
	RegisterHandlersWithSource(mux, nil)
}

// RegisterHandlersWithSource registers the diff API and preloads src, if any.
func RegisterHandlersWithSource(mux *http.ServeMux, src *Source) {
	mux.HandleFunc("/api/diff", handleDiff)
	mux.HandleFunc("/api/diff/source", handleSource(src))
}

func handleSource(src *Source) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "GET required", http.StatusMethodNotAllowed)
			return
		}
		if src == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		payload, err := src.load()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(payload)
	}
}

func handleDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Text1 string `json:"text1"`
		Text2 string `json:"text2"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	dmp := diffmatchpatch.New()
	rawDiffs := dmp.DiffMain(req.Text1, req.Text2, true)
	dmp.DiffCleanupSemantic(rawDiffs)

	type diffEntry struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	diffs := make([]diffEntry, 0, len(rawDiffs))
	for _, d := range rawDiffs {
		typ := "equal"
		switch d.Type {
		case diffmatchpatch.DiffInsert:
			typ = "insert"
		case diffmatchpatch.DiffDelete:
			typ = "delete"
		}
		diffs = append(diffs, diffEntry{Type: typ, Text: d.Text})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"diffs": diffs,
	})
}

func diffText(a, b string, theme patch.Theme) string {
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(a, b, true)
	dmp.DiffCleanupSemantic(diffs)

	var sb strings.Builder
	for _, d := range diffs {
		switch d.Type {
		case diffmatchpatch.DiffInsert:
			sb.WriteString(patch.Paint(theme, patch.LineAdd, d.Text))
		case diffmatchpatch.DiffDelete:
			sb.WriteString(patch.Paint(theme, patch.LineDelete, d.Text))
		default:
			sb.WriteString(d.Text)
		}
	}
	return sb.String()
}

// patchTheme maps the --theme flag to the palette used by the renderers.
func patchTheme(name string) patch.Theme {
	if name == "default" {
		return patch.ThemeDefault
	}
	return patch.ThemeNord
}
