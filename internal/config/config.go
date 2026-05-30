package config

import (
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"
)

type ContextEntry struct {
	Namespace string `json:"namespace,omitempty"`
}

type Config struct {
	Keychain bool                    `json:"keychain"`
	Current  string                  `json:"current,omitempty"`
	Contexts map[string]ContextEntry `json:"contexts,omitempty"`
	YAML     *YAMLConfig             `json:"yaml,omitempty"`
}

type YAMLConfig struct {
	Colorize bool `json:"colorize"`
}

func DefaultConfig() *Config {
	return &Config{
		Keychain: false,
		Contexts: make(map[string]ContextEntry),
	}
}

func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".ekube"), nil
}

func ConfigPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

func EncPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.enc"), nil
}

func EnsureDir() error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	return os.MkdirAll(dir, 0700)
}

func Load() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Contexts == nil {
		cfg.Contexts = make(map[string]ContextEntry)
	}

	return cfg, nil
}

func Save(cfg *Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}

	if err := EnsureDir(); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

func SaveCurrent(contextName string) error {
	cfg, err := Load()
	if err != nil {
		return err
	}

	cfg.Current = contextName
	return Save(cfg)
}

func SetNamespace(contextName, namespace string) error {
	cfg, err := Load()
	if err != nil {
		return err
	}

	entry := cfg.Contexts[contextName]
	entry.Namespace = namespace
	cfg.Contexts[contextName] = entry
	return Save(cfg)
}

func AddContext(name string, namespace string) error {
	cfg, err := Load()
	if err != nil {
		return err
	}

	if namespace == "" {
		namespace = "default"
	}

	cfg.Contexts[name] = ContextEntry{Namespace: namespace}
	return Save(cfg)
}

func RemoveContext(name string) error {
	cfg, err := Load()
	if err != nil {
		return err
	}

	delete(cfg.Contexts, name)
	if cfg.Current == name {
		cfg.Current = ""
	}
	return Save(cfg)
}

func (c *Config) ContextExists(name string) bool {
	_, ok := c.Contexts[name]
	return ok
}

func (c *Config) GetNamespace(name string) string {
	entry, ok := c.Contexts[name]
	if !ok || entry.Namespace == "" {
		return "default"
	}
	return entry.Namespace
}
