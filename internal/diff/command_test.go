package diff

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

func writeTempFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestReadSourceFileValid(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "a.txt", []byte("alpha\nbeta\n"))

	data, err := readSourceFile(path)
	if err != nil {
		t.Fatalf("readSourceFile: %v", err)
	}
	if string(data) != "alpha\nbeta\n" {
		t.Errorf("data = %q", data)
	}
}

func TestReadSourceFileMissing(t *testing.T) {
	_, err := readSourceFile(filepath.Join(t.TempDir(), "missing.txt"))
	if err == nil || !strings.Contains(err.Error(), "access") {
		t.Fatalf("err = %v, want an access error", err)
	}
}

func TestReadSourceFileDirectory(t *testing.T) {
	_, err := readSourceFile(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("err = %v, want a regular-file error", err)
	}
}

func TestReadSourceFileBinary(t *testing.T) {
	path := writeTempFile(t, t.TempDir(), "bin.dat", []byte("text\x00more"))
	_, err := readSourceFile(path)
	if err == nil || !strings.Contains(err.Error(), "binary") {
		t.Fatalf("err = %v, want a binary-file error", err)
	}
}

func TestReadSourceFileInvalidUTF8(t *testing.T) {
	// 0xff is invalid UTF-8 but not NUL, so it is reported as an encoding problem.
	path := writeTempFile(t, t.TempDir(), "gbk.txt", []byte{0xff, 0xfe, 'a'})
	_, err := readSourceFile(path)
	if err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("err = %v, want a UTF-8 error", err)
	}
}

func TestReadSourceFileTooLarge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.txt")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := f.Truncate(maxSourceFileBytes + 1); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	_, err = readSourceFile(path)
	if err == nil || !strings.Contains(err.Error(), "5 MiB") {
		t.Fatalf("err = %v, want a size-limit error", err)
	}
}

func TestSourceLoadPayload(t *testing.T) {
	dir := t.TempDir()
	a := writeTempFile(t, dir, "a.txt", []byte("one\n"))
	b := writeTempFile(t, dir, "b.txt", []byte("two\n"))

	payload, err := newSource([]string{a, b}).load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if payload.File1 != "a.txt" || payload.File2 != "b.txt" {
		t.Errorf("names = %q, %q; want base names a.txt, b.txt", payload.File1, payload.File2)
	}
	if payload.Text1 != "one\n" || payload.Text2 != "two\n" {
		t.Errorf("texts = %q, %q", payload.Text1, payload.Text2)
	}
}

func TestNewSourceRequiresPair(t *testing.T) {
	if newSource(nil) != nil {
		t.Error("newSource(nil) should be nil")
	}
	if newSource([]string{"a"}) != nil {
		t.Error("newSource with one file should be nil")
	}
}

func TestHandleSourceOK(t *testing.T) {
	dir := t.TempDir()
	a := writeTempFile(t, dir, "a.txt", []byte("alpha\n"))
	b := writeTempFile(t, dir, "b.txt", []byte("beta\n"))

	mux := http.NewServeMux()
	RegisterHandlersWithSource(mux, newSource([]string{a, b}))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/diff/source", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	var payload sourcePayload
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v (body %q)", err, rec.Body.String())
	}
	if payload.File1 != "a.txt" || payload.Text1 != "alpha\n" || payload.Text2 != "beta\n" {
		t.Errorf("payload = %+v", payload)
	}
}

func TestHandleSourceWithoutPair(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHandlers(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/diff/source", nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestHandleSourceRejectsPost(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHandlers(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/diff/source", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	if got := rec.Header().Get("Allow"); got != "GET, HEAD" {
		t.Errorf("Allow = %q, want GET, HEAD", got)
	}
}

func TestHandleSourceReportsUnreadableFile(t *testing.T) {
	dir := t.TempDir()
	a := writeTempFile(t, dir, "a.txt", []byte("alpha\n"))
	b := writeTempFile(t, dir, "b.txt", []byte("beta\n"))
	if err := os.Remove(a); err != nil {
		t.Fatalf("remove: %v", err)
	}

	mux := http.NewServeMux()
	RegisterHandlersWithSource(mux, newSource([]string{a, b}))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/diff/source", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "a.txt") {
		t.Errorf("body = %q, want the failing path", rec.Body.String())
	}
}

func TestFilePair(t *testing.T) {
	o := &ServeOptions{}
	if files, err := o.filePair(); err != nil || files != nil {
		t.Errorf("no files: files = %v, err = %v", files, err)
	}

	o = &ServeOptions{Files: []string{"a", "b"}}
	if files, err := o.filePair(); err != nil || len(files) != 2 {
		t.Errorf("two files: files = %v, err = %v", files, err)
	}

	for _, n := range []int{1, 3} {
		o = &ServeOptions{Files: make([]string, n)}
		for i := range o.Files {
			o.Files[i] = "f"
		}
		if _, err := o.filePair(); err == nil {
			t.Errorf("%d files: expected an error", n)
		}
	}
}

func TestServeAddressDefaults(t *testing.T) {
	withFiles := &ServeOptions{Files: []string{"a", "b"}}
	if got := withFiles.listenHost(); got != "127.0.0.1" {
		t.Errorf("listenHost with files = %q, want 127.0.0.1", got)
	}
	if !withFiles.loopback() {
		t.Error("files mode should be loopback by default")
	}

	withoutFiles := &ServeOptions{}
	if got := withoutFiles.listenHost(); got != "" {
		t.Errorf("listenHost without files = %q, want all interfaces", got)
	}
	if withoutFiles.loopback() {
		t.Error("no-files mode should not be loopback")
	}
	if got := withoutFiles.displayHost(); got != "localhost" {
		t.Errorf("displayHost = %q, want localhost", got)
	}

	exposed := &ServeOptions{Files: []string{"a", "b"}, Host: "0.0.0.0"}
	if exposed.loopback() {
		t.Error("--host 0.0.0.0 should not count as loopback")
	}
	if got := exposed.displayHost(); got != "localhost" {
		t.Errorf("displayHost for 0.0.0.0 = %q, want localhost", got)
	}

	if got := (&ServeOptions{Files: []string{"a", "b"}, Port: 0}).listenAddr(); !strings.HasSuffix(got, ":0") {
		t.Errorf("listenAddr = %q, want port 0", got)
	}
}

// TestKongParsesServeFiles pins the CLI contract: files are positional, --open
// defaults to true and --no-open turns it off.
func TestKongParsesServeFiles(t *testing.T) {
	parse := func(args ...string) *ServeOptions {
		t.Helper()
		root := struct {
			Serve ServeOptions `cmd:"" name:"serve"`
		}{}
		parser, err := kong.New(&root)
		if err != nil {
			t.Fatalf("kong.New: %v", err)
		}
		if _, err := parser.Parse(append([]string{"serve"}, args...)); err != nil {
			t.Fatalf("parse %v: %v", args, err)
		}
		return &root.Serve
	}

	o := parse("a.txt", "b.txt")
	if len(o.Files) != 2 || o.Files[0] != "a.txt" || o.Files[1] != "b.txt" {
		t.Errorf("Files = %v", o.Files)
	}
	if o.Port != 8088 {
		t.Errorf("Port = %d, want 8088", o.Port)
	}
	if !o.Open {
		t.Error("Open should default to true")
	}

	if o := parse("a.txt", "b.txt", "--no-open"); o.Open {
		t.Error("--no-open left Open enabled")
	}
	if o := parse(); len(o.Files) != 0 {
		t.Errorf("Files = %v, want none", o.Files)
	}
}
