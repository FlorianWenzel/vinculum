package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the ~/.vnclm/config.yml shape. Everything here is optional; kube
// context is the primary source of truth for cluster/namespace.
type Config struct {
	CurrentAgent string `yaml:"currentAgent,omitempty"`
	Namespace    string `yaml:"namespace,omitempty"`
	// OperatorService is the ClusterIP Service name for the operator API.
	OperatorService string `yaml:"operatorService,omitempty"`
	OperatorPort   int    `yaml:"operatorPort,omitempty"`
}

func defaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".vnclm", "config.yml"), nil
}

func Load() (Config, string, error) {
	path, err := defaultPath()
	if err != nil {
		return Config{}, "", err
	}
	cfg := Config{
		OperatorService: "vinculum-operator",
		OperatorPort:    8084,
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, path, nil
		}
		return cfg, path, err
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return cfg, path, err
	}
	if cfg.OperatorService == "" {
		cfg.OperatorService = "vinculum-operator"
	}
	if cfg.OperatorPort == 0 {
		cfg.OperatorPort = 8084
	}
	return cfg, path, nil
}

func Save(cfg Config) error {
	path, err := defaultPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}
