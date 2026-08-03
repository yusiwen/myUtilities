package k8s

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var kubeconfigDir string

// SetConfigDir overrides the base directory for stored kubeconfigs.
func SetConfigDir(dir string) {
	kubeconfigDir = dir
}

func indexPath() string {
	if kubeconfigDir != "" {
		os.MkdirAll(kubeconfigDir, 0700)
		return filepath.Join(kubeconfigDir, "kubeconfigs.yaml")
	}
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".config", "mu")
	os.MkdirAll(dir, 0700)
	return filepath.Join(dir, "kubeconfigs.yaml")
}

type ConfigIndex struct {
	Active  string            `yaml:"active"`
	Configs map[string]string `yaml:"configs"`
}

func LoadIndex() (*ConfigIndex, error) {
	idx := &ConfigIndex{Configs: make(map[string]string)}
	data, err := os.ReadFile(indexPath())
	if err != nil {
		if os.IsNotExist(err) {
			oldPath := filepath.Join(filepath.Dir(indexPath()), "kubeconfig.yaml")
			oldData, oldErr := os.ReadFile(oldPath)
			if oldErr == nil && len(oldData) > 0 {
				rawCfg, parseErr := clientcmd.NewClientConfigFromBytes(oldData)
				if parseErr == nil {
					cfg, _ := rawCfg.RawConfig()
					name := cfg.CurrentContext
					if name == "" {
						name = "default"
					}
					idx.Active = name
					idx.Configs[name] = string(oldData)
					SaveIndex(idx)
					os.Remove(oldPath)
				}
			}
			return idx, nil
		}
		return nil, err
	}
	if err := yaml.Unmarshal(data, idx); err != nil {
		return nil, err
	}
	if idx.Configs == nil {
		idx.Configs = make(map[string]string)
	}
	return idx, nil
}

func SaveIndex(idx *ConfigIndex) error {
	data, err := yaml.Marshal(idx)
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath(), data, 0644)
}

// LoadClient builds a Kubernetes clientset from kubeconfig content.
func LoadClient(kubeconfigContent string) (*kubernetes.Clientset, error) {
	config, err := clientcmd.NewClientConfigFromBytes([]byte(kubeconfigContent))
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}
	restConfig, err := config.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("create rest config: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client: %w", err)
	}
	return clientset, nil
}

// ClientFromConfig builds a Kubernetes clientset from a rest.Config.
func ClientFromConfig(restConfig *rest.Config) (*kubernetes.Clientset, error) {
	return kubernetes.NewForConfig(restConfig)
}

// LoadClientFromIndex returns the clientset for the active kubeconfig, or an error.
func LoadClientFromIndex() (*kubernetes.Clientset, error) {
	idx, err := LoadIndex()
	if err != nil {
		return nil, err
	}
	if idx.Active == "" || idx.Configs[idx.Active] == "" {
		return nil, fmt.Errorf("kubeconfig not configured; upload or paste your config first")
	}
	return LoadClient(idx.Configs[idx.Active])
}

// ActiveKubeconfig returns the active kubeconfig content.
func ActiveKubeconfig() (string, error) {
	idx, err := LoadIndex()
	if err != nil {
		return "", err
	}
	if idx.Active == "" || idx.Configs[idx.Active] == "" {
		return "", fmt.Errorf("kubeconfig not configured")
	}
	return idx.Configs[idx.Active], nil
}

func ParseKubeconfigMeta(data string) ([]string, string) {
	rawCfg, err := clientcmd.NewClientConfigFromBytes([]byte(data))
	if err != nil {
		return nil, ""
	}
	cfg, err := rawCfg.RawConfig()
	if err != nil {
		return nil, ""
	}
	var contexts []string
	for name := range cfg.Contexts {
		contexts = append(contexts, name)
	}
	return contexts, cfg.CurrentContext
}

// SaveKubeconfig stores a kubeconfig under the name derived from its current-context.
func SaveKubeconfig(content string) error {
	_, currentCtx := ParseKubeconfigMeta(content)
	name := currentCtx
	if name == "" {
		name = "default"
	}
	idx, err := LoadIndex()
	if err != nil {
		return err
	}
	idx.Active = name
	idx.Configs[name] = content
	return SaveIndex(idx)
}
