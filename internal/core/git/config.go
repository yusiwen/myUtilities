package git

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yusiwen/myUtilities/internal/core/llm"
)

// ModuleConfig holds the per-module (commit or review) configuration.
type ModuleConfig struct {
	Provider   llm.MultiProvider `json:"provider"`
	Lang       string            `json:"lang,omitempty"`
	ReviewsDir string            `json:"reviews_dir,omitempty"`
	Scip       ScipConfig        `json:"scip,omitempty"`
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

// GitConfig is the git module configuration file structure. The embedded
// llm.Catalog holds this file's own providers; providers from the shared
// llm-config.json are merged underneath them (see llm.EffectiveCatalog).
type GitConfig struct {
	llm.Catalog

	Commit ModuleConfig `json:"commit"`
	Review ModuleConfig `json:"review"`
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
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	return loadGitConfigFile(path, true)
}

// LoadGitConfigFrom loads from path when it is non-empty, otherwise from the
// default ~/.config/mu/git-config.json.
func LoadGitConfigFrom(path string) (*GitConfig, error) {
	if path == "" {
		return LoadGitConfig()
	}
	return loadGitConfigFile(path, false)
}

// loadGitConfigFile reads one git config file. migrate is only meaningful for
// the default location, where the legacy commit-config.json is folded in.
func loadGitConfigFile(path string, migrate bool) (*GitConfig, error) {
	gc := &GitConfig{}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if migrate {
				if migrated, migrErr := migrateFromOldConfig(); migrErr == nil {
					return migrated, nil
				}
			}
			return gc, nil
		}
		return nil, fmt.Errorf("failed to read git-config.json: %w", err)
	}

	if err := json.Unmarshal(data, gc); err != nil {
		return nil, fmt.Errorf("failed to parse git-config.json: %w", err)
	}

	// Name the file so a provider lookup failure can point at it.
	gc.Source = filepath.Base(path)

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
		Catalog: llm.Catalog{
			Providers: []llm.Provider{
				{
					Name:    "default",
					BaseURL: oldCfg.BaseURL,
					APIKey:  oldCfg.APIKey,
					Model:   oldCfg.Model,
				},
			},
			Source: configFileName,
		},
		Commit: ModuleConfig{
			Provider: llm.MultiProvider{"default"},
			Lang:     "en",
		},
		Review: ModuleConfig{
			Provider: llm.MultiProvider{"default"},
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
	return saveGitConfigFile(path, gc)
}

// SaveGitConfigTo saves to path when it is non-empty, otherwise to the default
// ~/.config/mu/git-config.json.
func SaveGitConfigTo(path string, gc *GitConfig) error {
	if path == "" {
		return SaveGitConfig(gc)
	}
	return saveGitConfigFile(path, gc)
}

func saveGitConfigFile(path string, gc *GitConfig) error {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("unable to create config directory: %w", err)
		}
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

// References returns the provider lists this config points at, so a provider
// removal can be refused while commit or review still names it.
func (gc *GitConfig) References() []llm.Reference {
	return []llm.Reference{
		{Module: "commit", Names: gc.Commit.Provider},
		{Module: "review", Names: gc.Review.Provider},
	}
}
