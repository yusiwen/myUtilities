package llm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MultiProvider represents the provider field in module config.
// It supports both a single string (for backward compatibility) and an
// array of strings (for provider fallback).
//
// JSON examples:
//
//	"provider": "default"            → single provider (legacy)
//	"provider": ["default", "backup"] → fallback chain
type MultiProvider []string

// UnmarshalJSON supports both a single string and an array of strings.
func (m *MultiProvider) UnmarshalJSON(data []byte) error {
	// Try array first
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		*m = arr
		return nil
	}
	// Fall back to single string
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	if s != "" {
		*m = []string{s}
	} else {
		*m = nil
	}
	return nil
}

// MarshalJSON always outputs an array for consistency.
func (m MultiProvider) MarshalJSON() ([]byte, error) {
	if len(m) == 0 {
		return []byte(`null`), nil
	}
	return json.Marshal([]string(m))
}

// Names returns the list of provider names.
func (m MultiProvider) Names() []string {
	if m == nil || len(m) == 0 {
		return nil
	}
	return []string(m)
}

// Contains reports whether the given name is in the list.
func (m MultiProvider) Contains(name string) bool {
	for _, n := range m {
		if n == name {
			return true
		}
	}
	return false
}

// Provider represents a named LLM provider configuration.
type Provider struct {
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

// Config is the ask module configuration file structure.
//
// Legacy mode: flat fields (base_url, api_key, model) are used directly.
//
// New mode: providers[] defines named providers, and provider[] references
// them by name, enabling fallback chains.
//
// On load, flat fields are auto-migrated into a "default" provider if
// providers[] is not set. On save, both formats coexist so existing
// tooling continues to work.
type Config struct {
	BaseURL      string        `json:"base_url,omitempty"`
	APIKey       string        `json:"api_key,omitempty"`
	Model        string        `json:"model,omitempty"`
	SearchAPIKey string        `json:"search_api_key,omitempty"`
	Providers    []Provider    `json:"providers,omitempty"`
	Provider     MultiProvider `json:"provider,omitempty"`
}

const configDir = "~/.config/mu"

// expandHome expands a leading "~/" to the current user's home directory.
// Paths without that prefix are returned unchanged.
func expandHome(path string) (string, error) {
	if !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("unable to find home directory: %w", err)
	}
	return filepath.Join(home, path[2:]), nil
}

func configFilePath(appName string) (string, error) {
	path, err := expandHome(filepath.Join(configDir, appName+".json"))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", fmt.Errorf("unable to create config directory: %w", err)
	}
	return path, nil
}

func LoadConfig(appName string) (*Config, error) {
	path, err := configFilePath(appName + "-config")
	if err != nil {
		return nil, err
	}
	return LoadConfigFromPath(path)
}

// LoadConfigFromPath loads config from an explicit path instead of the default
// ~/.config/mu/<app>-config.json location. A missing file yields the defaults.
func LoadConfigFromPath(path string) (*Config, error) {
	cfg := &Config{
		BaseURL: "https://api.openai.com/v1",
		Model:   "gpt-4o-mini",
	}

	resolved, err := expandHome(path)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	migrateConfig(cfg)
	return cfg, nil
}

// migrateConfig upgrades a legacy flat-field config into the provider model.
//
// Legacy mode (flat base_url/api_key/model, no providers) is migrated to a
// single "default" provider so existing configs keep working unchanged.
//
// It deliberately does NOT invent a "default" reference when the config only
// defines named providers (e.g. after `provider add`): pointing provider[] at a
// nonexistent provider would break every subsequent call. In that case the
// module is left unset and surfaces a clear "no provider configured" error.
func migrateConfig(cfg *Config) {
	if len(cfg.Providers) == 0 && len(cfg.Provider) == 0 &&
		(cfg.BaseURL != "" || cfg.APIKey != "" || cfg.Model != "") {
		cfg.Providers = []Provider{{
			Name:    "default",
			BaseURL: cfg.BaseURL,
			APIKey:  cfg.APIKey,
			Model:   cfg.Model,
		}}
	}
	if len(cfg.Provider) == 0 && len(cfg.Providers) > 0 {
		if _, err := ResolveProvider(cfg, "default"); err == nil {
			cfg.Provider = MultiProvider{"default"}
		}
	}
}

func SaveConfig(appName string, cfg *Config) error {
	// Keep the path consistent with LoadConfig, which reads <app>-config.json.
	path, err := configFilePath(appName + "-config")
	if err != nil {
		return err
	}
	return SaveConfigToPath(path, cfg)
}

func SaveConfigToPath(path string, cfg *Config) error {
	resolved, err := expandHome(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0700); err != nil {
		return fmt.Errorf("unable to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(resolved, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// ParseProviderList parses a comma-separated provider name list, trimming
// whitespace and dropping empty entries. Empty input yields nil.
func ParseProviderList(s string) (MultiProvider, error) {
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	result := make(MultiProvider, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("provider list is empty after trimming whitespace")
	}
	return result, nil
}

// ResolveProvider finds a named provider in the config.
func ResolveProvider(cfg *Config, name string) (*Provider, error) {
	for i := range cfg.Providers {
		if cfg.Providers[i].Name == name {
			return &cfg.Providers[i], nil
		}
	}
	return nil, fmt.Errorf("provider %q not found in ask-config.json", name)
}

// EffectiveClientConfig returns the effective base URL, API key, and model
// to use for an LLM call, merging provider settings with flag overrides.
func EffectiveClientConfig(cfg *Config, p *Provider, flagBaseURL, flagAPIKey, flagModel string) (baseURL, apiKey, model string) {
	baseURL = p.BaseURL
	apiKey = p.APIKey
	model = p.Model
	if flagBaseURL != "" {
		baseURL = flagBaseURL
	}
	if flagAPIKey != "" {
		apiKey = flagAPIKey
	}
	if flagModel != "" {
		model = flagModel
	}
	return
}
