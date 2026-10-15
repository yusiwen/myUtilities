package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// GlobalConfigFileName is the shared provider store read by every module.
const GlobalConfigFileName = "llm-config.json"

// GlobalConfigPath returns the default location of the shared catalog
// (~/.config/mu/llm-config.json), creating the directory when needed.
func GlobalConfigPath() (string, error) {
	return configFilePath("llm-config")
}

// LoadGlobalCatalog reads the shared catalog. An empty path means the default
// location, and a missing file is not an error — it yields an empty catalog, the
// same contract the per-module loaders follow.
func LoadGlobalCatalog(path string) (*Catalog, error) {
	resolved, err := resolveConfigPath(path, GlobalConfigPath)
	if err != nil {
		return nil, err
	}

	cat := NewCatalog(filepath.Base(resolved))
	data, err := os.ReadFile(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return cat, nil
		}
		return nil, fmt.Errorf("failed to read %s: %w", filepath.Base(resolved), err)
	}
	if err := json.Unmarshal(data, cat); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", filepath.Base(resolved), err)
	}
	return cat, nil
}

// SaveGlobalCatalog writes the shared catalog with 0600 permissions. An empty
// path means the default location.
func SaveGlobalCatalog(path string, cat *Catalog) error {
	resolved, err := resolveConfigPath(path, GlobalConfigPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o700); err != nil {
		return fmt.Errorf("unable to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cat, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", filepath.Base(resolved), err)
	}
	if err := os.WriteFile(resolved, data, 0600); err != nil {
		return fmt.Errorf("failed to write %s: %w", filepath.Base(resolved), err)
	}
	return nil
}

// EffectiveCatalog loads the shared catalog and merges a module's own providers
// on top of it. See MergeCatalogs for the precedence rule.
func EffectiveCatalog(globalPath string, module *Catalog) (*Catalog, error) {
	global, err := LoadGlobalCatalog(globalPath)
	if err != nil {
		return nil, err
	}
	return MergeCatalogs(global, module), nil
}

// DependentReferences keeps only the references a module cannot satisfy from its
// own providers, i.e. the ones that need the shared catalog to supply the name.
// A module entry that is not self-sufficient (an override that only sets the model)
// counts as a dependency too, so removing the shared entry cannot leave a module
// with a provider that has no endpoint.
func DependentReferences(module *Catalog, refs []Reference) []Reference {
	var out []Reference
	for _, r := range refs {
		var names MultiProvider
		for _, n := range r.Names.Names() {
			if module != nil && module.SelfSufficient(n) {
				continue
			}
			names = append(names, n)
		}
		if len(names) > 0 {
			out = append(out, Reference{Module: r.Module, Names: names})
		}
	}
	return out
}

// ValidateProviderAdd checks a provider addition before it is written. A name that
// already exists in the module is a duplicate; a name the merged catalog does not
// know yet needs both a base URL and an API key. A name that exists only in the
// shared catalog may be added with any subset of fields — that is how a module
// overrides just the model of a shared endpoint.
func ValidateProviderAdd(module *Catalog, name, baseURL, apiKey, globalPath string) error {
	if name == "" {
		return errors.New("provider name is required")
	}
	if module != nil && module.Contains(name) {
		return fmt.Errorf("provider %q already exists", name)
	}

	catalog, err := EffectiveCatalog(globalPath, module)
	if err != nil {
		return fmt.Errorf("failed to load %s: %w", GlobalConfigFileName, err)
	}
	if _, err := catalog.Resolve(name); err != nil && (baseURL == "" || apiKey == "") {
		return fmt.Errorf("provider %q is new: --base-url and --api-key are required", name)
	}
	return nil
}

// InheritedNames returns the provider names a module leans on the shared catalog
// for: the ones it does not define self-sufficiently itself.
func InheritedNames(module *Catalog, globalPath string) []string {
	global, err := LoadGlobalCatalog(globalPath)
	if err != nil {
		return nil
	}
	var names []string
	for _, p := range global.Providers {
		if module != nil && module.SelfSufficient(p.Name) {
			continue
		}
		names = append(names, p.Name)
	}
	return names
}

// ExplainInheritedProvider returns an error when name is not defined by the
// module but is inherited from the shared catalog. Removing it through a module
// command would fail with a bare "not found"; this says where it actually lives.
func ExplainInheritedProvider(name string, module *Catalog, globalPath string) error {
	if module != nil && module.Contains(name) {
		return nil
	}
	global, err := LoadGlobalCatalog(globalPath)
	if err != nil || !global.Contains(name) {
		return nil
	}
	return fmt.Errorf("provider %q is defined in %s; remove it there with 'mu set llm provider rm %s'",
		name, GlobalConfigFileName, name)
}

// resolveConfigPath expands an explicit path and falls back to fallback() when it
// is empty, mirroring the --config convention of the per-module configs.
func resolveConfigPath(path string, fallback func() (string, error)) (string, error) {
	if path == "" {
		return fallback()
	}
	return expandHome(path)
}
