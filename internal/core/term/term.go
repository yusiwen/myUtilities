// Package term provides terminal formatting utilities.
package term

import (
	"os"
	"regexp"

	"github.com/morikuni/aec"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

var noColor bool

func init() {
	if os.Getenv("NO_COLOR") != "" {
		noColor = true
	}
}

// DisableColor disables ANSI color output globally.
func DisableColor() {
	noColor = true
}

// Faint wraps s in ANSI faint (dim) style.
func Faint(s string) string {
	if noColor {
		return s
	}
	return aec.Apply(s, aec.Faint)
}

// Bright wraps s in ANSI white foreground.
func Bright(s string) string {
	if noColor {
		return s
	}
	return aec.Apply(s, aec.WhiteF)
}

// StripANSI removes all ANSI escape sequences from s.
func StripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}
