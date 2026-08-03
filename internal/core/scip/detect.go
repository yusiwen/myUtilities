package scip

import (
	"os"
	"path"
	"path/filepath"
)

// matchSignals reports whether any of the Detect signals for an indexer
// match the repository root. Plain names match files/dirs at the root;
// globs (containing '*') are matched against root entries with path.Match.
func matchSignals(repoRoot string, signals []string) (bool, error) {
	entries, err := os.ReadDir(repoRoot)
	if err != nil {
		return false, err
	}

	for _, sig := range signals {
		if isGlob(sig) {
			for _, e := range entries {
				ok, err := path.Match(sig, e.Name())
				if err != nil {
					continue
				}
				if ok {
					return true, nil
				}
			}
			continue
		}
		info, err := os.Stat(filepath.Join(repoRoot, sig))
		if err == nil {
			_ = info
			return true, nil
		}
	}
	return false, nil
}

func isGlob(s string) bool {
	for _, r := range s {
		if r == '*' || r == '?' || r == '[' {
			return true
		}
	}
	return false
}

// DetectLangs returns the languages detected in repoRoot, based on the
// enabled (auto-detectable) indexers only.
func DetectLangs(repoRoot string) ([]string, error) {
	var langs []string
	for _, ix := range Enabled() {
		ok, err := matchSignals(repoRoot, ix.Detect)
		if err != nil {
			continue
		}
		if ok {
			langs = append(langs, ix.Lang)
		}
	}
	return langs, nil
}
