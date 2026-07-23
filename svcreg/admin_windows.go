//go:build windows

package svcreg

import "os/exec"

func setIndependent(cmd *exec.Cmd) {}
