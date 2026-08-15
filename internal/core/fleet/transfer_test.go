package fleet

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveExt(t *testing.T) {
	cases := map[string]string{
		"a.tar.gz":       "tar.gz",
		"a.tgz":          "tar.gz",
		"a.tar":          "tar",
		"a.zip":          "zip",
		"a.txt":          "",
		"a":              "",
		"a.TAR.GZ":       "tar.gz",
		"archive.tar.gz": "tar.gz",
	}
	for name, want := range cases {
		if got := ArchiveExt(name); got != want {
			t.Fatalf("ArchiveExt(%q) = %q, want %q", name, got, want)
		}
	}
}

func writeTarGz(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(content))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gz.Close()
	f.Close()
}

func writeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	zw.Close()
	f.Close()
}

func TestExtractTarGz(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "bundle.tar.gz")
	writeTarGz(t, archive, map[string]string{"app/bin": "binary-content", "deployed.txt": "deployed-ok"})

	target := filepath.Join(dir, "out")
	if err := ExtractArchive(archive, target); err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(target, "deployed.txt"))
	if err != nil || string(data) != "deployed-ok" {
		t.Fatalf("extracted deployed.txt = %q, %v", data, err)
	}
}

func TestExtractZip(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "bundle.zip")
	writeZip(t, archive, map[string]string{"hello.txt": "hello-zip"})

	target := filepath.Join(dir, "out")
	if err := ExtractArchive(archive, target); err != nil {
		t.Fatalf("ExtractArchive: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(target, "hello.txt"))
	if err != nil || string(data) != "hello-zip" {
		t.Fatalf("extracted hello.txt = %q, %v", data, err)
	}
}

func TestExtractRejectsNonArchive(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(plain, []byte("not an archive"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := ExtractArchive(plain, dir); err == nil {
		t.Fatal("expected error for non-archive")
	}
}

func TestComputeSHA256(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	os.WriteFile(p, []byte("hello"), 0644)
	sum, err := ComputeSHA256(p)
	if err != nil {
		t.Fatal(err)
	}
	// sha256("hello") = 2cf24dba...
	if sum != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
		t.Fatalf("unexpected sha256 %q", sum)
	}
}
