package jarinfo

import (
	"fmt"
	"os"
	"sort"

	"github.com/yusiwen/myUtilities/core/jarinfo"
	"golang.org/x/term"
)

type Options struct {
	Info InfoOptions `cmd:"" name:"info" help:"Analyze a JAR file."`
}

type InfoOptions struct {
	File    string `arg:"" name:"file" help:"Path to JAR file."`
	Verbose bool   `help:"Show detailed version breakdown."`
}

func (o *InfoOptions) Run() error {
	f, err := os.Open(o.File)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat file: %w", err)
	}

	isTerm := term.IsTerminal(int(os.Stderr.Fd()))
	var progress func(current, total int)
	if isTerm && fi.Size() > 10*1024*1024 {
		progress = func(current, total int) {
			fmt.Fprintf(os.Stderr, "\rProcessing: %d/%d", current, total)
			if current == total {
				fmt.Fprintf(os.Stderr, "\n")
			}
		}
	}

	info, err := jarinfo.ParseJar(f, fi.Size(), progress)
	if err != nil {
		return fmt.Errorf("parse jar: %w", err)
	}

	fmt.Printf("Target JDK: %s\n", info.MinJDKVersion)
	fmt.Printf("Classes:    %d\n", info.ClassCount)

	if o.Verbose {
		fmt.Println("Version breakdown:")
		var majors []int
		for major := range info.VersionHistogram {
			majors = append(majors, major)
		}
		sort.Ints(majors)
		for _, major := range majors {
			count := info.VersionHistogram[major]
			fmt.Printf("  Java %-4s (%d): %d\n", jarinfo.JDKVersionString(major), major, count)
		}
	}

	return nil
}
