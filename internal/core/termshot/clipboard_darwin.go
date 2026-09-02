//go:build darwin

package termshot

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
)

const osascript = "/usr/bin/osascript"

// hasOsascript checks if /usr/bin/osascript exists and is executable.
func hasOsascript() bool {
	if fi, err := os.Stat(osascript); err == nil {
		return fi.Mode()&0111 != 0
	}
	return false
}

// SaveToClipboard writes the rendered scaffold image into the OS clipboard
// using AppleScript, when the osascript binary is available.
func SaveToClipboard(scaffold Scaffold) error {
	if !hasOsascript() {
		return fmt.Errorf("clipboard is not supported: %s not found", osascript)
	}

	var buf bytes.Buffer
	if _, err := buf.WriteString("set the clipboard to «data PICT"); err != nil {
		return err
	}

	if err := scaffold.WritePNG(hex.NewEncoder(&buf)); err != nil {
		return err
	}

	if _, err := buf.WriteString("»"); err != nil {
		return err
	}

	cmd := exec.Command(osascript, "-e", buf.String()) // #nosec G204
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprint(os.Stderr, string(out))
		return err
	}

	return nil
}
