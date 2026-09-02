//go:build !darwin

package termshot

import "fmt"

// SaveToClipboard is not supported on this platform.
func SaveToClipboard(scaffold Scaffold) error {
	return fmt.Errorf("clipboard is not supported on this platform")
}
