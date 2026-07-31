package budget

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Providers map[string]ProviderConfig `json:"providers"`
	DebugLog  bool                      `json:"debug_log"`
}

type ProviderConfig struct {
	APIKey          string `json:"api_key"`
	AccessKeyID     string `json:"access_key_id"`
	AccessKeySecret string `json:"access_key_secret"`
	TopUpURL        string `json:"top_up_url,omitempty"`
}

func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("unable to find home directory: %w", err)
	}
	dir := filepath.Join(home, ".config", "mu")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("unable to create config directory: %w", err)
	}
	return filepath.Join(dir, "budget-config.json"), nil
}

func LoadConfig(configPath string) (*Config, error) {
	path := configPath
	if path == "" {
		var err error
		path, err = DefaultConfigPath()
		if err != nil {
			return nil, err
		}
	}

	cfg := &Config{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read budget config: %w", err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse budget config: %w", err)
	}

	return cfg, nil
}

func ResolveAPIKey(provider string, flagKey string, cfg *Config) (string, error) {
	if flagKey != "" {
		return flagKey, nil
	}

	if cfg != nil && cfg.Providers != nil {
		if p, ok := cfg.Providers[provider]; ok && p.APIKey != "" {
			return p.APIKey, nil
		}
	}

	return "", fmt.Errorf(
		"no API key configured for %s\nSet it via:\n"+
			"  - --key flag\n"+
			"  - ~/.config/mu/budget-config.json → providers.%s.api_key",
		provider, provider,
	)
}
