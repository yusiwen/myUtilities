package installer

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/yusiwen/myUtilities/installer/templates"
	"io"
	"net/http"
	"sort"
	"strings"
	"text/template"
	"time"
)

var (
	errNotFound = errors.New("not found")
)

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
	//detect if we have a native m1 asset
	for _, a := range as {
		if a.IsMacM1() {
			return true
		}
	}
	return false
}

func (o Options) get(url string, v interface{}) error {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if o.Token != "" {
		req.Header.Set("Authorization", "token "+o.Token)
	}
	resp, err := http.Get(url)
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

type Query struct {
	User, Program, Release       string
	AsProgram, Select            string
	MoveToPath, Search, Insecure bool
	SudoMove                     bool   // deprecated: not used, now automatically detected
	OS, Arch                     string // override OS and Arch
}

type QueryResult struct {
	Query
	ResolvedRelease string
	Timestamp       time.Time
	Assets          Assets
	M1Asset         bool
}

func (o Options) Run() error {
	script := ""
	// type specific error response
	switch o.Output {
	case "json":
		script = ""
	case "shell":
		script = string(templates.Shell)
	default:
		return fmt.Errorf("unknown type: %s", o.Output)
	}
	q := Query{
		User:      "",
		Program:   "",
		Release:   "",
		Insecure:  o.Insecure,
		AsProgram: o.AsProgram,
		Select:    o.Select,
		OS:        o.Os,
		Arch:      o.Arch,
	}
	if o.Move {
		q.MoveToPath = true // also allow move=1 if bang in urls cause issues
	}
	var rest string
	q.User, rest = splitHalf(o.Repo, "/")
	q.Program, q.Release = splitHalf(rest, "@")
	// no program? treat first part as program, use default user
	if q.Program == "" {
		q.Program = q.User
		q.Search = true
	}
	if q.Release == "" {
		q.Release = "latest"
	}
	// fetch assets
	result, err := o.query(q)
	if err != nil {
		return fmt.Errorf("query failed: %s", err)
	}
	// no render script? just output as json
	if script == "" {
		b, _ := json.MarshalIndent(result, "", "  ")
		fmt.Printf("%s\n", b)
		return nil
	}
	// load template
	t, err := template.New("installer").Parse(script)
	if err != nil {
		return fmt.Errorf("template.New() error: %s", err)
	}
	// execute template
	buff := bytes.Buffer{}
	if err := t.Execute(&buff, result); err != nil {
		return fmt.Errorf("template.execute() error: %s", err)
	}
	fmt.Printf("%s\n", buff.Bytes())
	return nil
}

func (o Options) query(q Query) (QueryResult, error) {
	ts := time.Now()
	release, assets, err := o.getAssets(q)
	if err == nil {
		//didn't need search
		q.Search = false
	} else if errors.Is(err, errNotFound) && q.Search {
		//use ddg/google to auto-detect user...
		user, program, gerr := imFeelingLuck(q.Program)
		if gerr == nil {
			q.Program = program
			q.User = user
			//retry assets...
			release, assets, err = o.getAssets(q)
		}
	}
	if err != nil {
		return QueryResult{}, err
	}
	//success
	if q.Release == "" && release != "" {
		q.Release = release
	}
	result := QueryResult{
		Timestamp:       ts,
		Query:           q,
		ResolvedRelease: release,
		Assets:          assets,
		M1Asset:         assets.HasM1(),
	}
	return result, nil
}

func (o Options) getAssets(q Query) (string, Assets, error) {
	user := q.User
	repo := q.Program
	release := q.Release
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", user, repo)
	ghas := ghAssets{}
	if release == "" || release == "latest" {
		url += "/latest"
		ghr := ghRelease{}
		if err := o.get(url, &ghr); err != nil {
			return release, nil, err
		}
		release = ghr.TagName //discovered
		ghas = ghr.Assets
	} else {
		ghrs := []ghRelease{}
		if err := o.get(url, &ghrs); err != nil {
			return release, nil, err
		}
		found := false
		for _, ghr := range ghrs {
			if ghr.TagName == release {
				found = true
				if err := o.get(ghr.AssetsURL, &ghas); err != nil {
					return release, nil, err
				}
				ghas = ghr.Assets
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
	sumIndex, _ := ghas.getSumIndex()
	index := map[string]Asset{}
	for _, ga := range ghas {
		url := ga.BrowserDownloadURL
		//only binary containers are supported
		//TODO deb,rpm etc
		fext := getFileExt(url)
		if fext == "" && ga.Size > 1024*1024 {
			fext = ".bin" // +1MB binary
		}
		switch fext {
		case ".bin", ".zip", ".tar.bz", ".tar.bz2", ".bz2", ".gz", ".tar.gz", ".tgz":
			// valid
		default:
			continue
		}
		//match
		os := getOS(ga.Name)
		arch := getArch(ga.Name)
		//windows not supported yet
		if os == "windows" {
			//TODO: powershell
			// EG: iwr https://deno.land/x/install/install.ps1 -useb | iex
			continue
		}
		//unknown os, cant use
		if os == "" {
			continue
		}
		// user selecting a particular asset?
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
		//there can only be 1 file for each OS/Arch
		key := asset.Key()
		other, exists := index[key]
		if exists {
			gnu := func(s string) bool { return strings.Contains(s, "gnu") }
			musl := func(s string) bool { return strings.Contains(s, "musl") }
			g2m := gnu(other.Name) && !musl(other.Name) && !gnu(asset.Name) && musl(asset.Name)
			// prefer musl over glib for portability, override with select=gnu
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

type ghAssets []ghAsset

func (as ghAssets) getSumIndex() (map[string]string, error) {
	url := ""
	for _, ga := range as {
		//is checksum file?
		if ga.IsChecksumFile() {
			url = ga.BrowserDownloadURL
			break
		}
	}
	if url == "" {
		return nil, errors.New("no sum file found")
	}
	resp, err := http.DefaultClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// take each line and insert into the index
	index := map[string]string{}
	s := bufio.NewScanner(resp.Body)
	for s.Scan() {
		fs := strings.Fields(s.Text())
		if len(fs) != 2 {
			continue
		}
		index[fs[1]] = fs[0]
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
	return checksumRe.MatchString(strings.ToLower(g.Name)) && g.Size < 64*1024 //maximum file size 64KB
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
