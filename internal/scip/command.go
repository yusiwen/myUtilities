package scip

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	coregit "github.com/yusiwen/myUtilities/internal/core/git"
	corescip "github.com/yusiwen/myUtilities/internal/core/scip"
	"github.com/yusiwen/myUtilities/internal/core/term"
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

	version := o.Release
	if version == "" {
		// Respect a configured override, then the default pin.
		var err error
		version, err = resolveConfiguredVersion(ix)
		if err != nil {
			return err
		}
	}

	if o.Force {
		os.Remove(tc.BinPathFor(ix, version))
	}

	path, err := tc.Install(ix, version)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "%s\n", term.Faint("binary: "+path))
	return tc.Validate(ix)
}

func (o ListOptions) Run() error {
	tc := corescip.NewToolchain()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "Language\tIndexer\tDist\tConfigured\tPinned\tInstalled")
	fmt.Fprintln(w, "--------\t-------\t----\t----------\t------\t---------")
	for _, ix := range corescip.Registry() {
		dist := string(ix.Install)
		configured, _ := configuredVersion(ix)
		pinned := ix.Version
		if pinned == "" {
			pinned = "latest"
		}
		installed := installedVersions(tc, ix)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", ix.Lang, ix.BinaryName, dist, orDash(configured), pinned, orDash(installed))
	}
	return w.Flush()
}

func (o IndexOptions) Run() error {
	set, err := corescip.EnsureIndex(corescip.EnsureOptions{
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
	if set == nil || len(set.Langs()) == 0 {
		fmt.Fprintln(os.Stderr, term.Faint("No indexable languages detected."))
		return nil
	}
	names := make([]string, 0, len(set.Langs()))
	for _, l := range set.Langs() {
		names = append(names, corescip.LangDisplay(l))
	}
	fmt.Fprintf(os.Stderr, "%s\n", term.Faint("Index ready: "+strings.Join(names, ", ")))
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

// updateIndexers updates the given indexer(s) to their latest release. When
// pin is true the resolved version is persisted to git-config.json.
func (o UpdateOptions) updateIndexers(tc *corescip.Toolchain, langs []string, pin bool) error {
	for _, lang := range langs {
		ix, ok := corescip.LookupLang(lang)
		if !ok {
			fmt.Fprintf(os.Stderr, "%s\n", term.Faint(fmt.Sprintf("skipping %s: no registered indexer", lang)))
			continue
		}
		if ix.Install != corescip.MethodGitHubRelease {
			fmt.Fprintf(os.Stderr, "%s\n", term.Faint(fmt.Sprintf("skipping %s: %s distribution", lang, ix.Install)))
			continue
		}

		latest, err := tc.LatestVersion(ix)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", term.Faint(fmt.Sprintf("skipping %s: %v", lang, err)))
			continue
		}

		current, _ := tc.LookupFor(ix, latest)
		installed := tc.InstalledVersions(ix)
		newestInstalled := ""
		if len(installed) > 0 {
			newestInstalled = installed[0]
		}

		if o.DryRun {
			if newestInstalled != "" && newestInstalled != latest {
				fmt.Printf("%s: %s -> %s\n", ix.BinaryName, newestInstalled, latest)
			} else if current == "" {
				fmt.Printf("%s: not installed -> %s\n", ix.BinaryName, latest)
			} else {
				fmt.Printf("%s: already up to date (%s)\n", ix.BinaryName, latest)
			}
			continue
		}

		if newestInstalled == latest && current != "" {
			fmt.Printf("%s: already up to date (%s)\n", ix.BinaryName, latest)
			continue
		}

		if _, err := tc.Install(ix, latest); err != nil {
			return fmt.Errorf("update %s: %w", lang, err)
		}

		if !o.KeepOld {
			if err := tc.PurgeVersions(ix, latest); err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", term.Faint(fmt.Sprintf("warning: could not remove old %s versions: %v", lang, err)))
			}
		}

		if pin {
			if err := setConfiguredVersion(ix, latest); err != nil {
				return fmt.Errorf("persist version for %s: %w", lang, err)
			}
		}
		fmt.Printf("%s: updated to %s\n", ix.BinaryName, latest)
	}
	return nil
}

func (o UpdateOptions) Run() error {
	tc := corescip.NewToolchain()
	tc.Token = o.Token
	tc.Out = os.Stderr

	langs := []string{}
	if o.Lang != "" {
		langs = append(langs, o.Lang)
	} else {
		for _, ix := range corescip.Registry() {
			if !ix.Disable && ix.Install == corescip.MethodGitHubRelease {
				langs = append(langs, ix.Lang)
			}
		}
	}
	if len(langs) == 0 {
		return fmt.Errorf("no language to update")
	}
	return o.updateIndexers(tc, langs, !o.NoPin)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func installedVersions(tc *corescip.Toolchain, ix *corescip.Indexer) string {
	return strings.Join(tc.InstalledVersions(ix), ", ")
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

// loadGitScipConfig loads git-config.json and returns the review.scip config.
func loadGitScipConfig() (*coregit.ScipConfig, error) {
	gc, err := coregit.LoadGitConfig()
	if err != nil {
		return nil, err
	}
	return &gc.Review.Scip, nil
}

// configuredVersion returns the configured version override for lang, if any.
func configuredVersion(ix *corescip.Indexer) (string, bool) {
	scipCfg, err := loadGitScipConfig()
	if err != nil {
		return "", false
	}
	v, ok := scipCfg.Versions[ix.Lang]
	return v, ok && v != ""
}

// resolveConfiguredVersion returns the configured override for lang, or the
// indexer's default pin, or "" to fall back to latest.
func resolveConfiguredVersion(ix *corescip.Indexer) (string, error) {
	if v, ok := configuredVersion(ix); ok {
		return v, nil
	}
	return ix.Version, nil
}

// setConfiguredVersion persists a version override for lang into
// git-config.json's review.scip.versions.
func setConfiguredVersion(ix *corescip.Indexer, version string) error {
	gc, err := coregit.LoadGitConfig()
	if err != nil {
		return err
	}
	if gc.Review.Scip.Versions == nil {
		gc.Review.Scip.Versions = map[string]string{}
	}
	gc.Review.Scip.Versions[ix.Lang] = version
	return coregit.SaveGitConfig(gc)
}
