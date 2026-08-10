package installer

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

var errNotFound = errors.New("not found")

// defaultHTTP is used for all requests in this package. The timeout prevents
// hanging requests against the GitHub API or release asset hosts.
var defaultHTTP = &http.Client{Timeout: 30 * time.Second}

// perPage is the number of releases/assetes requested per API page. The
// GitHub API caps this at 100; it is a variable so tests can shrink it.
var perPage = 100

// apiBase is the GitHub REST API root. It is a variable so tests can point it
// at a local server.
var apiBase = "https://api.github.com"

type Asset struct {
	Name, OS, Arch, URL, Type, SHA256 string
}

func (a Asset) Key() string {
	return a.OS + "/" + a.Arch
}

func (a Asset) Is32Bit() bool {
	return a.Arch == "386"
}

func (a Asset) IsMac() bool {
	return a.OS == "darwin"
}

func (a Asset) IsMacM1() bool {
	return a.IsMac() && a.Arch == "arm64"
}

type Assets []Asset

func (as Assets) HasM1() bool {
	for _, a := range as {
		if a.IsMacM1() {
			return true
		}
	}
	return false
}

// Query describes a GitHub release query.
type Query struct {
	User, Program, Release       string
	AsProgram, Select            string
	MoveToPath, Search, Insecure bool
	SudoMove                     bool // deprecated
	OS, Arch                     string
}

type QueryResult struct {
	Query
	ResolvedRelease string
	Timestamp       time.Time
	Assets          Assets
	M1Asset         bool
}

// Client fetches GitHub release assets.
type Client struct {
	Token string
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	if c.Token != "" {
		req.Header.Set("Authorization", "token "+c.Token)
	}
	return defaultHTTP.Do(req)
}

// get performs a JSON GET, attaching the configured token to the request. The
// token is only sent on requests issued through this client, never by the
// http package defaults.
func (c *Client) get(url string, v interface{}) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("build request failed: %s: %s", url, err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	resp, err := c.do(req)
	if err != nil {
		return fmt.Errorf("request failed: %s: %s", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return fmt.Errorf("%w: url %s", errNotFound, url)
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return errors.New(http.StatusText(resp.StatusCode) + " " + string(b))
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("download failed: %s: %s", url, err)
	}
	return nil
}

// QueryAssets resolves a release query and returns the matching assets.
func (c *Client) QueryAssets(q Query) (QueryResult, error) {
	ts := time.Now()
	release, assets, err := c.getAssets(q)
	if err == nil {
		q.Search = false
	} else if errors.Is(err, errNotFound) && q.Search {
		user, program, gerr := ImFeelingLuck(q.Program)
		if gerr == nil {
			q.Program = program
			q.User = user
			release, assets, err = c.getAssets(q)
		}
	}
	if err != nil {
		return QueryResult{}, err
	}
	if q.Release == "" && release != "" {
		q.Release = release
	}
	return QueryResult{
		Timestamp:       ts,
		Query:           q,
		ResolvedRelease: release,
		Assets:          assets,
		M1Asset:         assets.HasM1(),
	}, nil
}

func (c *Client) getAssets(q Query) (string, Assets, error) {
	user := q.User
	repo := q.Program
	release := q.Release
	url := fmt.Sprintf("%s/repos/%s/%s/releases", apiBase, user, repo)
	ghas := ghAssets{}
	if release == "" || release == "latest" {
		url += "/latest"
		ghr := ghRelease{}
		if err := c.get(url, &ghr); err != nil {
			return release, nil, err
		}
		release = ghr.TagName
		ghas = ghr.Assets
	} else {
		ghrs := []ghRelease{}
		for page := 1; ; page++ {
			listURL := fmt.Sprintf("%s?per_page=%d&page=%d", url, perPage, page)
			pageGHs := []ghRelease{}
			if err := c.get(listURL, &pageGHs); err != nil {
				return release, nil, err
			}
			if len(pageGHs) == 0 {
				break
			}
			ghrs = append(ghrs, pageGHs...)
			if len(pageGHs) < perPage {
				break
			}
		}
		found := false
		for _, ghr := range ghrs {
			if ghr.TagName == release {
				found = true
				// Fetch the full asset list via assets_url; the release list
				// representation can truncate assets.
				if err := c.get(ghr.AssetsURL, &ghas); err != nil {
					return release, nil, err
				}
				break
			}
		}
		if !found {
			return release, nil, fmt.Errorf("release tag '%s' not found", release)
		}
	}
	if len(ghas) == 0 {
		return release, nil, errors.New("no assets found")
	}
	sumIndex, _ := c.getSumIndex(ghas)
	index := map[string]Asset{}
	for _, ga := range ghas {
		url := ga.BrowserDownloadURL
		fext := GetFileExt(url)
		if fext == "" && ga.Size > 1024*1024 {
			fext = ".bin"
		}
		switch fext {
		case ".bin", ".zip", ".tar.bz", ".tar.bz2", ".bz2", ".gz", ".tar.gz", ".tgz":
		default:
			continue
		}
		os := GetOS(ga.Name)
		arch := GetArch(ga.Name)
		if os == "windows" {
			continue
		}
		if os == "" {
			continue
		}
		if q.Select != "" && !strings.Contains(ga.Name, q.Select) {
			continue
		}
		asset := Asset{
			OS:     os,
			Arch:   arch,
			Name:   ga.Name,
			URL:    url,
			Type:   fext,
			SHA256: sumIndex[ga.Name],
		}
		key := asset.Key()
		other, exists := index[key]
		if exists {
			gnu := func(s string) bool { return strings.Contains(s, "gnu") }
			musl := func(s string) bool { return strings.Contains(s, "musl") }
			g2m := gnu(other.Name) && !musl(other.Name) && !gnu(asset.Name) && musl(asset.Name)
			if !g2m {
				continue
			}
		}
		index[key] = asset
	}
	if len(index) == 0 {
		return release, nil, errors.New("no downloads found for this release")
	}
	assets := Assets{}
	for _, a := range index {
		assets = append(assets, a)
	}
	sort.Slice(assets, func(i, j int) bool {
		return assets[i].Key() < assets[j].Key()
	})
	return release, assets, nil
}

// LatestTag returns the tag name of the latest published release for the
// repo, without any asset matching. Useful for cross-platform launchers whose
// asset names do not encode the target platform.
func (c *Client) LatestTag(user, program string) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", apiBase, user, program)
	var ghr ghRelease
	if err := c.get(url, &ghr); err != nil {
		return "", err
	}
	if ghr.TagName == "" {
		return "", fmt.Errorf("latest release has no tag for %s/%s", user, program)
	}
	return ghr.TagName, nil
}

// AssetByURL returns the browser download URL and SHA256 checksum for a release
// asset by exact name, without OS/arch filtering. The checksum is read from a
// sibling "<name>.sha256" asset when present; it is empty otherwise. Used for
// cross-platform launchers whose release assets do not encode the target
// platform.
func (c *Client) AssetByURL(user, program, release, name string) (string, string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/tags/%s", apiBase, user, program, release)
	var ghr ghRelease
	if err := c.get(url, &ghr); err != nil {
		return "", "", err
	}
	var downloadURL string
	found := false
	for _, a := range ghr.Assets {
		if a.Name == name {
			downloadURL = a.BrowserDownloadURL
			found = true
			break
		}
	}
	if !found {
		return "", "", fmt.Errorf("asset %q not found in %s/%s release %s", name, user, program, release)
	}

	sha := ""
	for _, a := range ghr.Assets {
		if a.Name == name+".sha256" {
			sha = c.fetchChecksum(a.BrowserDownloadURL)
			break
		}
	}
	return downloadURL, sha, nil
}

// fetchChecksum downloads a sha256sum-format file and returns the hex digest.
func (c *Client) fetchChecksum(url string) string {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return ""
	}
	resp, err := c.do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return ""
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

type ghAssets []ghAsset

func (c *Client) getSumIndex(as ghAssets) (map[string]string, error) {
	url := ""
	for _, ga := range as {
		if ga.IsChecksumFile() {
			url = ga.BrowserDownloadURL
			break
		}
	}
	if url == "" {
		return nil, errors.New("no sum file found")
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	index := map[string]string{}
	s := bufio.NewScanner(resp.Body)
	for s.Scan() {
		fs := strings.Fields(s.Text())
		if len(fs) != 2 {
			continue
		}
		name := strings.TrimPrefix(fs[1], "*")
		index[name] = fs[0]
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return index, nil
}

type ghAsset struct {
	BrowserDownloadURL string `json:"browser_download_url"`
	ContentType        string `json:"content_type"`
	CreatedAt          string `json:"created_at"`
	DownloadCount      int    `json:"download_count"`
	ID                 int    `json:"id"`
	Label              string `json:"label"`
	Name               string `json:"name"`
	Size               int    `json:"size"`
	State              string `json:"state"`
	UpdatedAt          string `json:"updated_at"`
	Uploader           struct {
		ID    int    `json:"id"`
		Login string `json:"login"`
	} `json:"uploader"`
	URL string `json:"url"`
}

func (g ghAsset) IsChecksumFile() bool {
	return checksumRe.MatchString(strings.ToLower(g.Name)) && g.Size < 64*1024
}

type ghRelease struct {
	Assets    []ghAsset `json:"assets"`
	AssetsURL string    `json:"assets_url"`
	Author    struct {
		ID    int    `json:"id"`
		Login string `json:"login"`
	} `json:"author"`
	Body            string      `json:"body"`
	CreatedAt       string      `json:"created_at"`
	Draft           bool        `json:"draft"`
	HTMLURL         string      `json:"html_url"`
	ID              int         `json:"id"`
	Name            interface{} `json:"name"`
	Prerelease      bool        `json:"prerelease"`
	PublishedAt     string      `json:"published_at"`
	TagName         string      `json:"tag_name"`
	TarballURL      string      `json:"tarball_url"`
	TargetCommitish string      `json:"target_commitish"`
	UploadURL       string      `json:"upload_url"`
	URL             string      `json:"url"`
	ZipballURL      string      `json:"zipball_url"`
}
