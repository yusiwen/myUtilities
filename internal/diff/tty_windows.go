//go:build windows

package diff

import "os"

// openTTY opens the console input device, so keys can be read while the patch
// itself arrived on stdin.
func openTTY() (*os.File, error) {
	return os.OpenFile("CONIN$", os.O_RDWR, 0)
}
