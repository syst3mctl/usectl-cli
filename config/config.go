package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	DefaultAPIURL = "https://usectl.com"
	configDir     = ".usectl"
	configFile    = "config.json"
)

// Config holds persistent CLI configuration.
type Config struct {
	Token       string `json:"token"`
	APIURL      string `json:"api_url"`
	GitHubToken string `json:"github_token,omitempty"`
}

// configPath returns ~/.usectl/config.json
func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, configDir, configFile), nil
}

// Load reads the config from disk. Returns an empty config if the file doesn't exist.
func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return &Config{APIURL: DefaultAPIURL}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{APIURL: DefaultAPIURL}, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.APIURL == "" {
		cfg.APIURL = DefaultAPIURL
	}
	return &cfg, nil
}

// Save writes the config to disk.
func Save(cfg *Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
