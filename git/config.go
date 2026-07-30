package git

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Provider struct {
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

type ModuleConfig struct {
	Provider string `json:"provider"`
	Lang     string `json:"lang,omitempty"`
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
				// Return empty config if migration fails (e.g. no old config either)
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
			Provider: "default",
			Lang:     "en",
		},
		Review: ModuleConfig{
			Provider: "default",
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
