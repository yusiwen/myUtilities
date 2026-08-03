package mock

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type ConfigFile struct {
	Port      int                `json:"port"`
	Endpoints []*ManagedEndpoint `json:"endpoints"`
}

// LoadConfig reads a dynamic server config file, resolving relative body file paths.
func LoadConfig(path string) ([]*ManagedEndpoint, int, error) {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, fmt.Errorf("read config file %s failed: %w", path, err)
	}

	var cfg ConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, 0, fmt.Errorf("parse config file %s failed: %v", path, err)
	}

	if cfg.Endpoints == nil {
		cfg.Endpoints = []*ManagedEndpoint{}
	}

	baseDir := filepath.Dir(path)
	for _, ep := range cfg.Endpoints {
		ep.Method = strings.ToUpper(ep.Method)
		if ep.Status == 0 {
			ep.Status = http.StatusOK
		}
		if ep.ID == "" {
			ep.ID = GenerateID()
		}
		if ep.Body != "" && !strings.HasPrefix(ep.Body, "{") && !strings.HasPrefix(ep.Body, "[") {
			bodyPath := ep.Body
			if !filepath.IsAbs(bodyPath) {
				bodyPath = filepath.Join(baseDir, bodyPath)
			}
			if b, err := os.ReadFile(bodyPath); err == nil {
				ep.Body = string(b)
			}
		}
	}

	return cfg.Endpoints, cfg.Port, nil
}

// SaveConfig persists endpoints to a config file, preserving the existing port.
func SaveConfig(path string, endpoints []*ManagedEndpoint) error {
	port := 8084
	if data, err := os.ReadFile(path); err == nil {
		var existing struct {
			Port int `json:"port"`
		}
		if json.Unmarshal(data, &existing) == nil && existing.Port != 0 {
			port = existing.Port
		}
	}

	out := ConfigFile{Port: port, Endpoints: endpoints}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config failed: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config failed: %w", err)
	}
	return nil
}

func GenerateID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
