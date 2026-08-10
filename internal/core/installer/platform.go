package installer

import (
	"regexp"
	"strings"
)

var (
	archRe     = regexp.MustCompile(`(arm64|386|686|amd64|x86_64|aarch64|armv[0-9]|\b32\b|\b64\b)`)
	fileExtRe  = regexp.MustCompile(`(\.tar)?(\.[a-z][a-z0-9]+)$`)
	posixOSRe  = regexp.MustCompile(`(darwin|linux|(net|free|open)bsd|mac|osx|windows|win)`)
	checksumRe = regexp.MustCompile(`(checksums|sha256sums)`)
	// versionRe matches version segments like v1.2.3, -1.2.3 or _1.2 so they
	// are not mistaken for the 32/64 numeric arch fallback below.
	versionRe = regexp.MustCompile(`(?i)[-_]?v?\d+(\.\d+)+`)
)

func GetOS(s string) string {
	s = strings.ToLower(s)
	o := posixOSRe.FindString(s)
	if o == "mac" || o == "osx" {
		o = "darwin"
	}
	if o == "win" {
		o = "windows"
	}
	return o
}

func GetArch(s string) string {
	s = strings.ToLower(s)
	// Drop version segments (e.g. "v3.32.0") so a version number is never
	// mistaken for the 32/64 numeric arch fallback below.
	s = versionRe.ReplaceAllString(s, "")
	a := archRe.FindString(s)
	if a == "" {
		// Numeric fallback for names where the arch is glued to the OS token
		// (e.g. "linux64", "win32"). Only trust a bare number when the name
		// actually names an OS, otherwise a version like "64" could win.
		if GetOS(s) != "" {
			switch {
			case strings.Contains(s, "32"):
				a = "32"
			case strings.Contains(s, "64"):
				a = "64"
			}
		}
	}
	if a == "64" || a == "x86_64" || a == "" {
		a = "amd64"
	} else if a == "32" || a == "686" {
		a = "386"
	} else if a == "aarch64" {
		a = "arm64"
	}
	return a
}

func GetFileExt(s string) string {
	return fileExtRe.FindString(s)
}

func SplitHalf(s, by string) (string, string) {
	i := strings.Index(s, by)
	if i == -1 {
		return s, ""
	}
	return s[:i], s[i+len(by):]
}
