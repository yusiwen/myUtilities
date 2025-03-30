package main

import (
	"fmt"
	"github.com/alecthomas/kong"
	"os"
	"runtime/debug"
)

const shaLen = 7

//TIP <p>To run your code, right-click the code and select <b>Run</b>.</p> <p>Alternatively, click
// the <icon src="AllIcons.Actions.Execute"/> icon in the gutter and select the <b>Run</b> menu item from here.</p>

func main() {
	if Version == "unknown version" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Sum != "" {
			Version = info.Main.Version
		} else {
			Version = "unknown (built from source)"
		}
	}
	version := fmt.Sprintf("myUtilities version %s", Version)
	if len(CommitSHA) >= shaLen {
		version += " (" + CommitSHA[:shaLen] + ")"
	}
	var mu = &MyUtilities{}
	var ctx = kong.Parse(
		mu,
		kong.Name("myUtilities"),
		kong.Description("myUtilities"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact:             true,
			Summary:             false,
			NoExpandSubcommands: true,
		}),
		kong.Vars{
			"version":       version,
			"versionNumber": Version,
			"versionFull":   Version + " (" + BuildTime + ")",
		})

	if err := ctx.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
