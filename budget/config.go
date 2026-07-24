package budget

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/yusiwen/myUtilities/core/llm"
)

type BudgetConfig struct {
	Providers map[string]ProviderConfig `json:"providers"`
	DebugLog  bool                      `json:"debug_log"`
}

type ProviderConfig struct {
	APIKey          string `json:"api_key"`
	AccessKeyID     string `json:"access_key_id"`
	AccessKeySecret string `json:"access_key_secret"`
}

func defaultConfigPath() (string, error) {
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

func loadConfig(configPath string) (*BudgetConfig, error) {
	path := configPath
	if path == "" {
		var err error
		path, err = defaultConfigPath()
		if err != nil {
			return nil, err
		}
	}

	cfg := &BudgetConfig{}
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

func resolveAPIKey(provider string, flagKey string, cfg *BudgetConfig) (string, error) {
	if flagKey != "" {
		return flagKey, nil
	}

	if cfg != nil && cfg.Providers != nil {
		if p, ok := cfg.Providers[provider]; ok && p.APIKey != "" {
			return p.APIKey, nil
		}
	}

	for _, app := range []string{"ask", "commit"} {
		llmCfg, err := llm.LoadConfig(app)
		if err == nil && llmCfg.APIKey != "" {
			return llmCfg.APIKey, nil
		}
	}

	return "", fmt.Errorf(
		"no API key configured for %s\nSet it via:\n"+
			"  - --key flag\n"+
			"  - ~/.config/mu/budget-config.json → providers.%s.api_key\n"+
			"  - ~/.config/mu/ask.json → api_key\n"+
			"  - ~/.config/mu/commit.json → api_key",
		provider, provider,
	)
}
