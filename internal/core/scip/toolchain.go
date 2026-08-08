package scip

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	coreinst "github.com/yusiwen/myUtilities/internal/core/installer"
	"github.com/yusiwen/myUtilities/internal/core/term"
)

const defaultCacheDir = "~/.cache/mu/scip"

// Toolchain manages the local indexer binaries, installed on demand
// from GitHub releases (treesitter-nvim style).
type Toolchain struct {
	Root    string
	Token   string
	Verbose bool
	Out     io.Writer
}

// NewToolchain returns a Toolchain rooted at the default cache directory.
func NewToolchain() *Toolchain {
	return &Toolchain{Root: expandHome(defaultCacheDir), Out: os.Stderr}
}

// DefaultCacheRoot returns the expanded default cache directory.
func DefaultCacheRoot() string {
	return expandHome(defaultCacheDir)
}

func expandHome(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}

func (tc *Toolchain) logf(format string, args ...any) {
	if tc.Out != nil {
		fmt.Fprintf(tc.Out, format+"\n", args...)
	}
}

func (tc *Toolchain) debugf(format string, args ...any) {
	if tc.Verbose {
		tc.logf(term.Faint(format), args...)
	}
}

func (tc *Toolchain) toolsDir() string {
	return filepath.Join(tc.Root, "tools")
}

// BinPath returns the versioned install path for the indexer binary.
func (tc *Toolchain) BinPath(ix *Indexer) string {
	return tc.BinPathFor(ix, ix.Version)
}

// BinPathFor returns the install path for a specific version.
func (tc *Toolchain) BinPathFor(ix *Indexer, version string) string {
	return filepath.Join(tc.toolsDir(), ix.BinaryName, version, ix.BinaryName)
}

// Lookup returns the installed binary path for the indexer's default version
// and whether it is usable.
func (tc *Toolchain) Lookup(ix *Indexer) (string, bool) {
	return tc.LookupFor(ix, ix.Version)
}

// LookupFor returns the installed binary path for a specific version.
func (tc *Toolchain) LookupFor(ix *Indexer, version string) (string, bool) {
	path := tc.BinPathFor(ix, version)
	if isExecutable(path) {
		return path, true
	}
	return "", false
}

// ResolveVersion determines which release version to use for ix, with the
// priority: config override (map keyed by language) > default pin > latest.
func (tc *Toolchain) ResolveVersion(ix *Indexer, overrides map[string]string) (string, error) {
	if v, ok := overrides[ix.Lang]; ok && v != "" {
		return v, nil
	}
	if ix.Version != "" {
		return ix.Version, nil
	}
	return tc.resolveLatest(ix)
}

// LatestVersion queries GitHub for the latest release tag of the indexer.
func (tc *Toolchain) LatestVersion(ix *Indexer) (string, error) {
	return tc.resolveLatest(ix)
}

// resolveLatest queries GitHub for the latest release tag of the indexer.
func (tc *Toolchain) resolveLatest(ix *Indexer) (string, error) {
	if ix.Install != MethodGitHubRelease {
		return "", fmt.Errorf("indexer %s uses install method %q, not auto-installable yet", ix.Lang, ix.Install)
	}
	user, program := splitRepo(ix.GitHubRepo)
	client := &coreinst.Client{Token: tc.Token}
	tag, err := client.LatestTag(user, program)
	if err != nil {
		return "", fmt.Errorf("failed to resolve latest release of %s: %w", ix.GitHubRepo, err)
	}
	return tag, nil
}

// InstalledVersions returns the installed version directories for ix, newest
// first (empty when nothing is installed).
func (tc *Toolchain) InstalledVersions(ix *Indexer) []string {
	dir := filepath.Join(tc.toolsDir(), ix.BinaryName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var vers []string
	for _, e := range entries {
		if e.IsDir() {
			vers = append(vers, e.Name())
		}
	}
	sort.Slice(vers, func(i, j int) bool { return versionGreater(vers[i], vers[j]) })
	return vers
}

// PurgeVersions removes installed version directories for ix, keeping the
// given versions (which should be absolute paths or version dirs to keep).
func (tc *Toolchain) PurgeVersions(ix *Indexer, keep ...string) error {
	keepSet := map[string]bool{}
	for _, k := range keep {
		if k == "" {
			continue
		}
		keepSet[filepath.Base(k)] = true
	}
	for _, v := range tc.InstalledVersions(ix) {
		if keepSet[v] {
			continue
		}
		if err := os.RemoveAll(filepath.Join(tc.toolsDir(), ix.BinaryName, v)); err != nil {
			return err
		}
	}
	return nil
}

// Install resolves and downloads the indexer binary for ix at the given
// version, returning its path.
func (tc *Toolchain) Install(ix *Indexer, version string) (string, error) {
	if path, ok := tc.LookupFor(ix, version); ok {
		return path, nil
	}
	if ix.Install != MethodGitHubRelease {
		return "", fmt.Errorf("indexer %s uses install method %q, not auto-installable yet", ix.Lang, ix.Install)
	}
	if runtime.GOOS == "windows" {
		return "", fmt.Errorf("automatic indexer installation is not supported on windows yet")
	}

	user, program := splitRepo(ix.GitHubRepo)
	client := &coreinst.Client{Token: tc.Token}
	q := coreinst.Query{
		User:    user,
		Program: program,
		Release: version,
	}
	if q.Release == "" {
		q.Release = "latest"
	}

	binDir := filepath.Dir(tc.BinPathFor(ix, version))
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return "", err
	}

	var asset coreinst.Asset
	if assetName := ix.AssetNameFor(version); assetName != "" {
		// Cross-platform launcher asset (no OS/arch in the name): resolve the
		// browser download URL by exact name and install it directly.
		tc.logf(term.Faint("Resolving %s release %s ..."), ix.GitHubRepo, q.Release)
		url, sha, err := client.AssetByURL(user, program, q.Release, assetName)
		if err != nil {
			return "", fmt.Errorf("failed to resolve %s asset: %w", assetName, err)
		}
		asset = coreinst.Asset{Name: assetName, URL: url, Type: ".bin", SHA256: sha}
	} else {
		tc.logf(term.Faint("Resolving %s release %s ..."), ix.GitHubRepo, q.Release)
		result, err := client.QueryAssets(q)
		if err != nil {
			return "", fmt.Errorf("failed to query %s assets: %w", ix.GitHubRepo, err)
		}
		var ok bool
		asset, ok = matchAsset(result.Assets)
		if !ok {
			return "", fmt.Errorf("no %s asset for %s/%s", ix.BinaryName, runtime.GOOS, runtime.GOARCH)
		}
		tc.debugf("asset: %s (%s) %s", asset.Name, asset.Key(), asset.URL)
	}

	if err := tc.installAsset(asset, tc.BinPathFor(ix, version)); err != nil {
		return "", err
	}
	if err := os.Chmod(tc.BinPathFor(ix, version), 0755); err != nil {
		return "", err
	}

	tc.logf("%s %s installed to %s", term.Faint(ix.BinaryName), term.Faint(q.Release), term.Bright(tc.BinPathFor(ix, version)))
	return tc.BinPathFor(ix, version), nil
}

func splitRepo(repo string) (user, program string) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 {
		return repo, repo
	}
	return parts[0], parts[1]
}

func matchAsset(assets coreinst.Assets) (coreinst.Asset, bool) {
	for _, a := range assets {
		if a.OS == runtime.GOOS && a.Arch == runtime.GOARCH {
			return a, true
		}
	}
	return coreinst.Asset{}, false
}

func (tc *Toolchain) installAsset(a coreinst.Asset, dest string) error {
	switch a.Type {
	case ".bin", "", ".exe":
		return tc.downloadVerified(a.URL, a.SHA256, dest)
	default:
		return tc.downloadExtract(a, dest)
	}
}

// downloadVerified downloads url into dest and checks sha (if non-empty).
func (tc *Toolchain) downloadVerified(url, sha, dest string) error {
	tc.logf(term.Faint("Downloading %s ..."), filepath.Base(url))
	resp, err := download(url)
	if err != nil {
		return err
	}
	defer resp.Close()

	h := sha256.New()
	body := io.TeeReader(resp, h)

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, body); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}

	if sha != "" {
		got := hex.EncodeToString(h.Sum(nil))
		if !strings.EqualFold(got, sha) {
			os.Remove(dest)
			return fmt.Errorf("sha256 mismatch: got %s, want %s", got, sha)
		}
	}
	return nil
}

// downloadExtract downloads an archive, extracts it and moves the matching
// binary to dest.
func (tc *Toolchain) downloadExtract(a coreinst.Asset, dest string) error {
	tmpDir, err := os.MkdirTemp("", "scip-install-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, filepath.Base(a.Name))
	if err := tc.downloadVerified(a.URL, a.SHA256, archivePath); err != nil {
		return err
	}

	found, err := extractBinary(archivePath, tmpDir)
	if err != nil {
		return err
	}
	if found == "" {
		return fmt.Errorf("no binary matching %q found in archive", dest)
	}

	data, err := os.ReadFile(found)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0755)
}

// extractBinary extracts archive (by extension) into dir and returns the path
// of the most likely indexer binary. A .gz archive is treated as a tar.gz
// first; when the stream is not a tar (e.g. rust-analyzer ships a bare gzipped
// single binary), it falls back to decompressing it directly.
func extractBinary(archivePath, dir string) (string, error) {
	lower := strings.ToLower(archivePath)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return extractZip(archivePath, dir)
	case strings.HasSuffix(lower, ".gz"):
		if p, err := extractTar(archivePath, dir); err == nil && p != "" {
			return p, nil
		}
		return extractRawGzip(archivePath, dir)
	default:
		return extractTar(archivePath, dir)
	}
}

// extractRawGzip decompresses a bare gzip-compressed single binary (no tar
// container) into dir and returns the extracted file path.
func extractRawGzip(archivePath, dir string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()

	name := filepath.Base(archivePath)
	name = strings.TrimSuffix(name, ".gz")
	dest := filepath.Join(dir, name)
	if err := writeFileFrom(dest, gz); err != nil {
		return "", err
	}
	return dest, nil
}

func extractTar(archivePath, dir string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var r io.Reader = f
	switch {
	case strings.HasSuffix(strings.ToLower(archivePath), ".bz2"):
		r = bzip2.NewReader(f)
	case strings.HasSuffix(strings.ToLower(archivePath), ".gz"):
		gz, err := gzip.NewReader(f)
		if err != nil {
			return "", err
		}
		defer gz.Close()
		r = gz
	}

	tr := tar.NewReader(r)
	best := ""
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name := filepath.Base(hdr.Name)
		if !binaryCandidate(name) {
			continue
		}
		dest := filepath.Join(dir, name)
		if err := writeFileFrom(dest, tr); err != nil {
			return "", err
		}
		if candidate, ok := pickCandidate(dir, dest); ok {
			return candidate, nil
		}
		if best == "" {
			best = dest
		}
	}
	return best, nil
}

func extractZip(archivePath, dir string) (string, error) {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer zr.Close()

	best := ""
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		name := filepath.Base(zf.Name)
		if !binaryCandidate(name) {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return "", err
		}
		dest := filepath.Join(dir, name)
		if err := writeFileFrom(dest, rc); err != nil {
			rc.Close()
			return "", err
		}
		rc.Close()
		if candidate, ok := pickCandidate(dir, dest); ok {
			best = candidate
		} else if best == "" {
			best = dest
		}
	}
	return best, nil
}

func writeFileFrom(dest string, src io.Reader) error {
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, src); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// binaryCandidate reports whether an archive member name looks like the
// indexer executable (no extension, not a known text/license file).
func binaryCandidate(name string) bool {
	if name == "" || strings.HasPrefix(name, ".") {
		return false
	}
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".exe") || strings.HasSuffix(lower, ".dll") || strings.HasSuffix(lower, ".so") {
		return false
	}
	if nonBinaryNames[lower] {
		return false
	}
	switch filepath.Ext(lower) {
	case ".md", ".txt", ".json", ".yaml", ".yml", ".toml", ".html", ".htm",
		".d", ".version", ".manifest", ".sig", ".asc", ".zip", ".gz", ".bz2":
		return false
	}
	ext := filepath.Ext(lower)
	return ext == "" || ext == ".bin"
}

var nonBinaryNames = map[string]bool{
	"license": true, "licence": true, "copying": true, "notice": true,
	"readme": true, "changelog": true, "authors": true, "contributors": true,
	"makefile": true, "install": true,
}

// pickCandidate prefers a binary that matches GOOS/GOARCH in its name.
func pickCandidate(dir, dest string) (string, bool) {
	name := filepath.Base(dest)
	lower := strings.ToLower(name)
	if strings.Contains(lower, runtime.GOOS) || strings.Contains(lower, runtime.GOARCH) {
		return dest, true
	}
	return "", false
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir() && info.Mode()&0111 != 0
}

// versionGreater reports whether a > b, comparing dotted numeric versions.
// Tags may have a "v" prefix (e.g. "v0.13.1") or be date-style (e.g.
// "2026-07-27"); non-numeric segments are compared lexically as a tiebreak.
func versionGreater(a, b string) bool {
	av, bv := parseVersion(a), parseVersion(b)
	for i := 0; i < len(av) && i < len(bv); i++ {
		if av[i] != bv[i] {
			return av[i] > bv[i]
		}
	}
	return len(av) > len(bv)
}

func parseVersion(v string) []int {
	fields := strings.FieldsFunc(strings.TrimPrefix(v, "v"), func(r rune) bool {
		return r == '.' || r == '-'
	})
	out := make([]int, 0, len(fields))
	for _, f := range fields {
		if n, err := strconv.Atoi(f); err == nil {
			out = append(out, n)
		} else {
			// Lexical segment: approximate by length + first char for
			// deterministic ordering in date-like tags.
			out = append(out, len(f)*1000+int(f[0]))
		}
	}
	return out
}

func download(url string) (io.ReadCloser, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		resp.Body.Close()
		return nil, fmt.Errorf("download %s: %s", url, resp.Status)
	}
	return resp.Body, nil
}

// Validate runs the binary with --version and reports whether it executes.
func (tc *Toolchain) Validate(ix *Indexer) error {
	path, ok := tc.Lookup(ix)
	if !ok {
		return fmt.Errorf("indexer %s is not installed", ix.Lang)
	}
	cmd := exec.Command(path, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s failed to run: %w (%s)", ix.BinaryName, err, strings.TrimSpace(string(out)))
	}
	tc.debugf("%s --version: %s", ix.BinaryName, strings.TrimSpace(string(out)))
	return nil
}
