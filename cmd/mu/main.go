package main

import (
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/yusiwen/myUtilities/internal/core/config"
	"github.com/yusiwen/myUtilities/internal/core/version"
	"github.com/yusiwen/myUtilities/internal/gateway"
)

const shaLen = 7

//TIP <p>To run your code, right-click the code and select <b>Run</b>.</p> <p>Alternatively, click
// the <icon src="AllIcons.Actions.Execute"/> icon in the gutter and select the <b>Run</b> menu item from here.</p>

func main() {
	if len(os.Args) > 1 && os.Args[1] == "set" {
		os.Exit(runSet(os.Args[2:]))
	}

	versionStr := fmt.Sprintf("myUtilities version %s", version.Version)
	displayVersion := version.Version
	if len(version.CommitSHA) >= shaLen {
		versionStr += " (" + version.CommitSHA[:shaLen] + ")"
		displayVersion += " (" + version.CommitSHA[:shaLen] + ")"
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
			"version":       versionStr,
			"versionNumber": version.Version,
			"versionFull":   version.Version + " (" + version.BuildTime + ")",
		})
	gateway.SetVersion(displayVersion)

	if err := ctx.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runSet(args []string) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Println("Usage: mu set <module> [flags]")
		fmt.Println("Update module configurations.")
		fmt.Println("\nModules:")
		for _, m := range config.All() {
			fmt.Printf("  %s\n", m.Name())
		}
		return 0
	}
	name := args[0]
	m := config.Get(name)
	if m == nil {
		names := ""
		for _, v := range config.All() {
			names += " " + v.Name()
		}
		fmt.Fprintf(os.Stderr, "unknown module: %s. Available:%s\n", name, names)
		return 1
	}
	if err := m.Set(args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
