package scip

import (
	"fmt"
	"os"
	"text/tabwriter"

	corescip "github.com/yusiwen/myUtilities/core/scip"
	"github.com/yusiwen/myUtilities/core/term"
)

func (o InstallOptions) Run() error {
	ix, ok := corescip.LookupLang(o.Lang)
	if !ok {
		return fmt.Errorf("no indexer registered for language %q (available: %s)", o.Lang, availableLangs())
	}
	if ix.Install != corescip.MethodGitHubRelease {
		return fmt.Errorf("%s indexer uses %q distribution, not auto-installable yet", ix.Lang, ix.Install)
	}

	tc := corescip.NewToolchain()
	tc.Token = o.Token
	tc.Out = os.Stderr

	if o.Force {
		os.Remove(tc.BinPath(ix))
	}

	path, err := tc.Install(ix)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "%s\n", term.Faint("binary: "+path))
	return tc.Validate(ix)
}

func (o ListOptions) Run() error {
	tc := corescip.NewToolchain()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "Language\tIndexer\tDist\tVersion\tStatus")
	fmt.Fprintln(w, "--------\t-------\t----\t-------\t------")
	for _, ix := range corescip.Registry() {
		status := "not installed"
		if _, ok := tc.Lookup(ix); ok {
			status = "installed"
		}
		if ix.Disable {
			status += " (needs build system)"
		}
		dist := string(ix.Install)
		ver := ix.Version
		if ver == "" {
			ver = "latest"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", ix.Lang, ix.BinaryName, dist, ver, status)
	}
	return w.Flush()
}

func (o IndexOptions) Run() error {
	_, err := corescip.EnsureIndex(corescip.EnsureOptions{
		RepoRoot:    ".",
		CacheDir:    o.CacheDir,
		AutoInstall: true,
		Force:       o.Force,
		Token:       o.Token,
		Out:         os.Stderr,
	})
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "SCIP index ready.")
	return nil
}

func (o PurgeOptions) Run() error {
	root := corescip.DefaultCacheRoot()
	if !o.Force {
		fmt.Printf("Remove SCIP cache at %s? [y/N] ", root)
		var ans string
		if _, err := fmt.Scanln(&ans); err != nil || (ans != "y" && ans != "Y") {
			fmt.Println("aborted")
			return nil
		}
	}
	if err := os.RemoveAll(root); err != nil {
		return err
	}
	fmt.Printf("Removed %s\n", root)
	return nil
}

func availableLangs() string {
	out := ""
	for i, ix := range corescip.Registry() {
		if i > 0 {
			out += ", "
		}
		out += ix.Lang
	}
	return out
}
