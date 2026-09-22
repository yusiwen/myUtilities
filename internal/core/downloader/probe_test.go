package downloader

import "testing"

func TestParseContentRange(t *testing.T) {
	cases := []struct {
		name       string
		in         string
		start, end int64
		total      int64
		ok         bool
	}{
		{name: "full", in: "bytes 0-0/123", start: 0, end: 0, total: 123, ok: true},
		{name: "unknown total", in: "bytes 10-19/*", start: 10, end: 19, total: -1, ok: true},
		{name: "empty", in: "", ok: false},
		{name: "garbage", in: "garbage", ok: false},
		{name: "missing range part", in: "bytes 5/10", ok: false},
		{name: "missing slash", in: "bytes 0-5", ok: false},
		{name: "non numeric", in: "bytes a-b/10", ok: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start, end, total, ok := parseContentRange(c.in)
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v", ok, c.ok)
			}
			if !ok {
				return
			}
			if start != c.start || end != c.end || total != c.total {
				t.Errorf("got (%d, %d, %d), want (%d, %d, %d)", start, end, total, c.start, c.end, c.total)
			}
		})
	}
}

func TestSanitizeFilename(t *testing.T) {
	cases := map[string]string{
		"file.bin":                        "file.bin",
		"../../etc/passwd":                "passwd",
		`..\..\windows\system32\evil.dll`: "evil.dll",
		"dir/file.txt":                    "file.txt",
		"  spaced.txt  ":                  "spaced.txt",
		"with\x00null":                    "withnull",
		"..":                              "",
		".":                               "",
		"/":                               "",
		"":                                "",
	}
	for in, want := range cases {
		if got := sanitizeFilename(in); got != want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFilenameFromURL(t *testing.T) {
	cases := map[string]string{
		"https://example.com/a/b/file.iso": "file.iso",
		"https://example.com/a/b/":         "b", // path.Base drops the trailing slash, like curl -O
		"https://example.com/":             "",
		"https://example.com":              "",
		"https://example.com/file.bin?x=1": "file.bin",
	}
	for in, want := range cases {
		if got := filenameFromURL(in); got != want {
			t.Errorf("filenameFromURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFilenameFromDisposition(t *testing.T) {
	cases := map[string]string{
		`attachment; filename="report.pdf"`:                 "report.pdf",
		"attachment; filename*=UTF-8''r%C3%A9sum%C3%A9.pdf": "résumé.pdf",
		`attachment; filename="../evil.sh"`:                 "evil.sh",
		`attachment; filename="sub/dir/file.txt"`:           "file.txt",
		"inline":                  "",
		"":                        "",
		`attachment; filename=""`: "",
	}
	for in, want := range cases {
		if got := filenameFromDisposition(in); got != want {
			t.Errorf("filenameFromDisposition(%q) = %q, want %q", in, got, want)
		}
	}
}
