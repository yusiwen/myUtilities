//go:build !windows

package diff

import "os"

// openTTY opens the controlling terminal, so keys can be read while the patch
// itself arrived on stdin.
func openTTY() (*os.File, error) {
	return os.OpenFile("/dev/tty", os.O_RDWR, 0)
}
