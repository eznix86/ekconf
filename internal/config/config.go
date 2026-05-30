package config

import (
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"
)

const (
	DirName                 = ".ekube"
	ConfigFileName          = "config.yaml"
	EncryptedConfigFileName = "config.enc"
	DefaultNamespace        = "default"
	tempConfigPattern       = ".ekube-config-*"
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
	return filepath.Join(home, DirName), nil
}

func ConfigPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ConfigFileName), nil
}

func EncPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, EncryptedConfigFileName), nil
}

func EnsureDir() error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	return os.MkdirAll(dir, 0o700)
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

	tmp, err := os.CreateTemp(filepath.Dir(path), tempConfigPattern)
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp config: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
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

func AddContext(name, namespace string) error {
	cfg, err := Load()
	if err != nil {
		return err
	}

	if namespace == "" {
		namespace = DefaultNamespace
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
		return DefaultNamespace
	}
	return entry.Namespace
}
