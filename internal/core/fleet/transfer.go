package fleet

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// safeJoin joins a name from an archive onto dir and rejects anything that
// escapes it. A plain strings.HasPrefix check is not enough: it accepts a
// sibling directory whose name merely starts with dir (for example
// "/data/files-evil" when dir is "/data/files").
func safeJoin(dir, name string) (string, error) {
	target := filepath.Join(dir, filepath.Clean(name))
	rel, err := filepath.Rel(dir, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe path %q", name)
	}
	return target, nil
}

// ComputeSHA256 returns the hex SHA-256 of the file at path.
func ComputeSHA256(path string) (string, error) {
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

// ArchiveExt detects the archive type from a file name: "tar.gz"/"tgz",
// "tar", or "zip". It returns "" for non-archives.
func ArchiveExt(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return "tar.gz"
	case strings.HasSuffix(lower, ".tar"):
		return "tar"
	case strings.HasSuffix(lower, ".zip"):
		return "zip"
	}
	return ""
}

// ExtractArchive extracts a tar, tar.gz, tgz, or zip archive into targetDir.
func ExtractArchive(sourcePath, targetDir string) error {
	f, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open archive %s: %w", sourcePath, err)
	}
	defer f.Close()

	switch ArchiveExt(sourcePath) {
	case "tar.gz":
		return extractTarGz(f, targetDir)
	case "tar":
		return extractTar(f, targetDir)
	case "zip":
		return extractZip(f, targetDir)
	default:
		return fmt.Errorf("unsupported archive format: %s", sourcePath)
	}
}

func extractTarGz(f *os.File, targetDir string) error {
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()
	return extractTar(gz, targetDir)
}

func extractTar(r io.Reader, targetDir string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}
		target, err := safeJoin(targetDir, hdr.Name)
		if err != nil {
			return fmt.Errorf("tar: %w", err)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			out, err := os.Create(target)
			if err != nil {
				return err
			}
			_, cpErr := io.Copy(out, tr)
			closeErr := out.Close()
			if cpErr != nil {
				return cpErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
	}
	return nil
}

func extractZip(f *os.File, targetDir string) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(f, info.Size())
	if err != nil {
		return fmt.Errorf("zip: %w", err)
	}
	for _, zf := range zr.File {
		if strings.HasSuffix(zf.Name, "/") {
			continue
		}
		target, err := safeJoin(targetDir, zf.Name)
		if err != nil {
			return fmt.Errorf("zip: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return err
		}
		_, cpErr := io.Copy(out, rc)
		rc.Close()
		closeErr := out.Close()
		if cpErr != nil {
			return cpErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
