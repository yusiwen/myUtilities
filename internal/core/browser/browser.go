// Package browser starts the operating system's default web browser.
//
// It exists so commands that start a local server can show the result instead of
// asking the user to copy a URL out of the terminal.
package browser

import (
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
)

// Open launches the default browser for rawURL and returns once the launcher has
// been started. It does not wait for the browser to exit.
func Open(rawURL string) error {
	argv, err := command(runtime.GOOS, exec.LookPath, rawURL)
	if err != nil {
		return err
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", argv[0], err)
	}
	// Reap the launcher so it does not stay a zombie. The browser itself keeps
	// running as a separate process.
	go func() { _ = cmd.Wait() }()
	return nil
}

// command returns the argv that opens rawURL with the platform launcher.
//
// It is separate from Open so each platform's mapping is testable without
// starting a browser. lookPath is passed in for the same reason.
func command(goos string, lookPath func(string) (string, error), rawURL string) ([]string, error) {
	switch goos {
	case "darwin":
		path, err := lookPath("open")
		if err != nil {
			return nil, fmt.Errorf("find open: %w", err)
		}
		return []string{path, rawURL}, nil

	case "windows":
		// rundll32 avoids cmd.exe quoting: a URL containing & or ^ would be
		// mangled by `cmd /c start`.
		path, err := lookPath("rundll32")
		if err != nil {
			return nil, fmt.Errorf("find rundll32: %w", err)
		}
		return []string{path, "url.dll,FileProtocolHandler", rawURL}, nil

	default:
		for _, c := range []struct {
			name  string
			extra []string
		}{
			{"xdg-open", nil},
			{"gio", []string{"open"}},
			{"wslview", nil},
			{"sensible-browser", nil},
		} {
			path, err := lookPath(c.name)
			if err != nil {
				continue
			}
			argv := append([]string{path}, c.extra...)
			return append(argv, rawURL), nil
		}
		return nil, errors.New("no browser launcher found (tried xdg-open, gio, wslview, sensible-browser)")
	}
}
