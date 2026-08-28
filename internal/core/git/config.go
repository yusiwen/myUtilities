package git

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yusiwen/myUtilities/internal/core/llm"
)

// Type aliases for shared types defined in core/llm/config.go.
type Provider = llm.Provider
type MultiProvider = llm.MultiProvider

// ModuleConfig holds the per-module (commit or review) configuration.
type ModuleConfig struct {
	Provider   MultiProvider `json:"provider"`
	Lang       string        `json:"lang,omitempty"`
	ReviewsDir string        `json:"reviews_dir,omitempty"`
	Scip       ScipConfig    `json:"scip,omitempty"`
}

func (m *ModuleConfig) ReviewsDirPath() string {
	dir := m.ReviewsDir
	if dir == "" {
		dir = "~/.cache/mu/git_reviews"
	}
	if strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			dir = filepath.Join(home, dir[2:])
		}
	}
	return dir
}

type ScipConfig struct {
	Enabled     *bool             `json:"enabled,omitempty"`      // nil → enabled by default
	AutoInstall *bool             `json:"auto_install,omitempty"` // nil → true
	CacheDir    string            `json:"cache_dir,omitempty"`    // empty → ~/.cache/mu/scip
	Versions    map[string]string `json:"versions,omitempty"`     // lang → release tag override
}

type GitConfig struct {
	Providers []Provider   `json:"providers"`
	Commit    ModuleConfig `json:"commit"`
	Review    ModuleConfig `json:"review"`
}

const configDir = "~/.config/mu"
const configFileName = "git-config.json"
const oldCommitConfigName = "commit-config.json"

func configPath() (string, error) {
	raw := filepath.Join(configDir, configFileName)
	if strings.HasPrefix(raw, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("unable to find home directory: %w", err)
		}
		raw = filepath.Join(home, raw[2:])
	}
	dir := filepath.Dir(raw)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("unable to create config directory: %w", err)
	}
	return raw, nil
}

func oldCommitConfigPath() (string, error) {
	raw := filepath.Join(configDir, oldCommitConfigName)
	if strings.HasPrefix(raw, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("unable to find home directory: %w", err)
		}
		raw = filepath.Join(home, raw[2:])
	}
	return raw, nil
}

func LoadGitConfig() (*GitConfig, error) {
	gc := &GitConfig{}

	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			gc, migrErr := migrateFromOldConfig()
			if migrErr != nil {
				return &GitConfig{}, nil
			}
			return gc, nil
		}
		return nil, fmt.Errorf("failed to read git-config.json: %w", err)
	}

	if err := json.Unmarshal(data, gc); err != nil {
		return nil, fmt.Errorf("failed to parse git-config.json: %w", err)
	}

	return gc, nil
}

func migrateFromOldConfig() (*GitConfig, error) {
	oldPath, err := oldCommitConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(oldPath)
	if err != nil {
		return nil, err
	}

	var oldCfg struct {
		BaseURL string `json:"base_url"`
		APIKey  string `json:"api_key"`
		Model   string `json:"model"`
	}
	if err := json.Unmarshal(data, &oldCfg); err != nil {
		return nil, err
	}

	if oldCfg.BaseURL == "" && oldCfg.APIKey == "" {
		return nil, fmt.Errorf("old commit-config.json is empty")
	}

	if oldCfg.BaseURL == "" {
		oldCfg.BaseURL = "https://api.openai.com/v1"
	}
	if oldCfg.Model == "" {
		oldCfg.Model = "gpt-4o-mini"
	}

	gc := &GitConfig{
		Providers: []Provider{
			{
				Name:    "default",
				BaseURL: oldCfg.BaseURL,
				APIKey:  oldCfg.APIKey,
				Model:   oldCfg.Model,
			},
		},
		Commit: ModuleConfig{
			Provider: MultiProvider{"default"},
			Lang:     "en",
		},
		Review: ModuleConfig{
			Provider: MultiProvider{"default"},
			Lang:     "en",
		},
	}

	if err := SaveGitConfig(gc); err != nil {
		return nil, err
	}

	os.Remove(oldPath)
	fmt.Fprintf(os.Stderr, "Migrated config from %s to git-config.json\n", oldPath)
	return gc, nil
}

func SaveGitConfig(gc *GitConfig) error {
	path, err := configPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(gc, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal git-config.json: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write git-config.json: %w", err)
	}

	return nil
}

// ResolveProvider finds a named provider in the git config.
func ResolveProvider(gc *GitConfig, name string) (*Provider, error) {
	for i := range gc.Providers {
		if gc.Providers[i].Name == name {
			return &gc.Providers[i], nil
		}
	}
	return nil, fmt.Errorf("provider %q not found in git-config.json", name)
}

func GetModuleConfig(gc *GitConfig, module string) (*ModuleConfig, error) {
	switch module {
	case "commit":
		return &gc.Commit, nil
	case "review":
		return &gc.Review, nil
	default:
		return nil, fmt.Errorf("unknown module: %q", module)
	}
}

// ForEachProvider iterates through provider names and calls fn for each.
// fn receives the resolved *Provider. If a provider is not found in the
// config, the next provider is tried; an error is returned only when every
// provider in the chain has been exhausted without success.
func ForEachProvider(gc *GitConfig, providerNames MultiProvider, fn func(*Provider) error) error {
	names := providerNames.Names()
	if len(names) == 0 {
		return fmt.Errorf("no provider configured for this module (set one with 'mu set git <module> --provider <name>')")
	}

	// lastErr tracks errors across the fallback chain. Not-found errors are
	// all accumulated (useful when a provider name is misspelled midway). fn
	// errors are tracked for the final wrap.
	var lastErr error
	for i, name := range names {
		p, err := ResolveProvider(gc, name)
		if err != nil {
			lastErr = fmt.Errorf("provider %q not found in git-config.json: %w", name, err)
			continue
		}
		if err := fn(p); err != nil {
			// If there are more providers to try, record the error and continue.
			if i < len(names)-1 {
				lastErr = fmt.Errorf("provider %q failed: %w", name, err)
				continue
			}
			// This was the last provider — wrap consistently so the outer
			// "all provider(s) failed" message always carries detail.
			return fmt.Errorf("provider %q failed: %w", name, err)
		}
		// Success — stop immediately.
		return nil
	}
	return lastErr
}
