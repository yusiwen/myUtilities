//go:build !windows

package svcreg

import (
	"os/exec"
	"syscall"
)

func setIndependent(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
