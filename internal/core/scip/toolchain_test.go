package scip

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectLangs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	langs, err := DetectLangs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(langs) != 1 || langs[0] != "go" {
		t.Fatalf("expected [go], got %v", langs)
	}
}

func TestDetectLangsNone(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	langs, err := DetectLangs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(langs) != 0 {
		t.Fatalf("expected no languages, got %v", langs)
	}
}

func makeTarGz(t *testing.T, entries map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "pkg.tar.gz")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	for name, content := range entries {
		hdr := &tar.Header{Name: name, Mode: 0755, Size: int64(len(content))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestExtractBinary(t *testing.T) {
	archive := makeTarGz(t, map[string]string{
		"scip-go-linux-amd64/scip-go": "#!binary\n",
		"LICENSE":                     "license text",
	})

	outDir := t.TempDir()
	found, err := extractBinary(archive, outDir)
	if err != nil {
		t.Fatal(err)
	}
	if found == "" {
		t.Fatal("expected a binary to be extracted")
	}
	if filepath.Base(found) != "scip-go" {
		t.Fatalf("expected scip-go binary, got %q", found)
	}
	data, err := os.ReadFile(found)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "#!binary\n" {
		t.Fatalf("unexpected extracted content: %q", string(data))
	}
}

func TestBinaryCandidate(t *testing.T) {
	cases := map[string]bool{
		"scip-go":       true,
		"scip-go.exe":   false,
		"README.md":     false,
		".gitignore":    false,
		"scip-go_linux": true,
		"index.scip":    false,
		"scip-go.bin":   true,
	}
	for name, want := range cases {
		if got := binaryCandidate(filepath.Base(name)); got != want {
			t.Errorf("binaryCandidate(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestJavaRegistryEnabled(t *testing.T) {
	ix, ok := LookupLang("java")
	if !ok {
		t.Fatal("java indexer missing from registry")
	}
	if ix.Disable {
		t.Fatal("java should be enabled")
	}
	if !ix.FailHard {
		t.Fatal("java should be FailHard")
	}
	if ix.AssetName != "scip-java-v0.13.1" {
		t.Fatalf("unexpected java AssetName: %q", ix.AssetName)
	}
	if strings.Join(ix.Prefix, " ") != "index" || ix.OutputFlag != "--output" {
		t.Fatalf("unexpected java invocation fields: prefix=%v output=%q", ix.Prefix, ix.OutputFlag)
	}
	// java must be detectable
	langs, err := DetectLangs(t.TempDir())
	if err != nil || len(langs) != 0 {
		t.Fatalf("empty dir should detect nothing, got %v (err=%v)", langs, err)
	}
}
