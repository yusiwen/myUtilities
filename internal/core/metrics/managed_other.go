//go:build !linux

package metrics

import "syscall"

// managedSysProcAttr is a no-op on platforms without parent-death signals.
func managedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}
