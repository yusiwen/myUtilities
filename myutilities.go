package main

import "github.com/yusiwen/myUtilities/installer"

type MyUtilities struct {
	Installer installer.Options `cmd:"" name:"install" help:"Install binary from GitHub release."`
}
