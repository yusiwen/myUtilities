package downloader

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
)

// RemoteInfo describes what the server is willing to give us.
type RemoteInfo struct {
	URL          string // original URL
	FinalURL     string // URL after following redirects
	Size         int64  // total size in bytes, -1 when unknown
	Ranged       bool   // server honours Range requests
	ETag         string
	LastModified string
	Filename     string // suggested name from Content-Disposition or the URL path
}

// Probe discovers the remote file's size, resume validators and whether the
// server supports byte ranges.
//
// Strategy: try HEAD first; when it is rejected or omits the range capability,
// confirm with a one-byte ranged GET (`Range: bytes=0-0`). A server that
// answers that with 200 is treated as single-stream only.
func Probe(ctx context.Context, client *http.Client, o Options) (*RemoteInfo, error) {
	info := &RemoteInfo{URL: o.URL, Size: -1}

	req, err := o.newRequest(ctx, http.MethodHead, o.URL)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, err
		}
		// Some servers reset or hang on HEAD; fall back to a ranged GET.
		return probeWithGET(ctx, client, o, info)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		fillFromResponse(info, resp, true)
		if info.Size < 0 {
			// Unknown length (chunked): no ranges, no resume.
			return info, nil
		}
		if strings.EqualFold(strings.TrimSpace(resp.Header.Get("Accept-Ranges")), "bytes") {
			info.Ranged = true
			return info, nil
		}
	}
	return probeWithGET(ctx, client, o, info)
}

// probeWithGET confirms range support with a single-byte ranged request.
func probeWithGET(ctx context.Context, client *http.Client, o Options, info *RemoteInfo) (*RemoteInfo, error) {
	req, err := o.newRequest(ctx, http.MethodGet, o.URL)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Range", "bytes=0-0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// Never read the body: for a range-ignoring server this would be the whole
	// file. Closing early also releases the connection.
	io.Copy(io.Discard, io.LimitReader(resp.Body, 0)) //nolint:errcheck // best-effort drain

	switch resp.StatusCode {
	case http.StatusPartialContent:
		fillFromResponse(info, resp, false)
		info.Ranged = true
		if _, _, total, ok := parseContentRange(resp.Header.Get("Content-Range")); ok && total >= 0 {
			info.Size = total
		}
		return info, nil
	case http.StatusOK:
		fillFromResponse(info, resp, true)
		info.Ranged = false
		return info, nil
	default:
		return nil, fmt.Errorf("unexpected HTTP status %s for %s", resp.Status, o.URL)
	}
}

// fillFromResponse copies resume validators and naming hints out of a response.
// useContentLength must be false for 206 responses, where Content-Length
// describes the range and not the whole file.
func fillFromResponse(info *RemoteInfo, resp *http.Response, useContentLength bool) {
	if resp.Request != nil && resp.Request.URL != nil {
		info.FinalURL = resp.Request.URL.String()
	}
	if info.FinalURL == "" {
		info.FinalURL = info.URL
	}
	if etag := resp.Header.Get("ETag"); etag != "" {
		info.ETag = etag
	}
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		info.LastModified = lm
	}
	if name := filenameFromDisposition(resp.Header.Get("Content-Disposition")); name != "" {
		info.Filename = name
	} else if info.Filename == "" {
		info.Filename = filenameFromURL(info.FinalURL)
	}
	if useContentLength && resp.ContentLength >= 0 {
		info.Size = resp.ContentLength
	}
}

// filenameFromDisposition extracts and sanitizes the filename parameter.
func filenameFromDisposition(value string) string {
	if value == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(value)
	if err != nil {
		return ""
	}
	name := params["filename"]
	if name == "" {
		return ""
	}
	// Never let a remote header escape the target directory.
	return sanitizeFilename(name)
}

// filenameFromURL derives a filename from the URL path.
func filenameFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	base := path.Base(u.Path)
	if base == "." || base == "/" || base == "" {
		return ""
	}
	return sanitizeFilename(base)
}

// sanitizeFilename strips any directory component and control characters.
func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Base(name)
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "." || name == ".." {
		return ""
	}
	return name
}

// parseContentRange parses `bytes <start>-<end>/<total>` (total may be `*`).
func parseContentRange(value string) (start, end, total int64, ok bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, 0, -1, false
	}
	value = strings.TrimPrefix(value, "bytes ")
	rangePart, totalPart, found := strings.Cut(value, "/")
	if !found {
		return 0, 0, -1, false
	}
	startPart, endPart, found := strings.Cut(rangePart, "-")
	if !found {
		return 0, 0, -1, false
	}
	start, err := strconv.ParseInt(strings.TrimSpace(startPart), 10, 64)
	if err != nil {
		return 0, 0, -1, false
	}
	end, err = strconv.ParseInt(strings.TrimSpace(endPart), 10, 64)
	if err != nil {
		return 0, 0, -1, false
	}
	total = -1
	if strings.TrimSpace(totalPart) != "*" {
		if parsed, err := strconv.ParseInt(strings.TrimSpace(totalPart), 10, 64); err == nil {
			total = parsed
		}
	}
	return start, end, total, true
}
