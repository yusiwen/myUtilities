package main

import (
	"fmt"
	"github.com/alecthomas/kong"
	"os"
)

//TIP <p>To run your code, right-click the code and select <b>Run</b>.</p> <p>Alternatively, click
// the <icon src="AllIcons.Actions.Execute"/> icon in the gutter and select the <b>Run</b> menu item from here.</p>

func main() {
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
			"version":       Version,
			"versionNumber": Version,
			"versionFull":   Version + " (" + BuildTime + ")",
		})

	if err := ctx.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
