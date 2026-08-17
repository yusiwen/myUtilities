//go:build linux

package metrics

import "syscall"

// managedSysProcAttr wires the child to the parent lifecycle: if the gateway
// is hard-killed (SIGKILL), the managed subprocess dies with it.
func managedSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
}
