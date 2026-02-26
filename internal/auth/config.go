package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// HostConfig holds auth credentials for a single GitHub host.
type HostConfig struct {
	OAuthToken string `json:"oauth_token"`
}

// Config maps hostnames to their auth config.
type Config map[string]HostConfig

// ConfigDir overrides the config directory for testing.
// If empty, os.UserConfigDir() is used.
var ConfigDir string

func configPath() (string, error) {
	dir := ConfigDir
	if dir == "" {
		var err error
		dir, err = os.UserConfigDir()
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(dir, "jip", "config.json"), nil
}

// LoadConfig reads the jip config file.
func LoadConfig() (Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// SaveToken stores an OAuth token for the given host.
func SaveToken(host, token string) error {
	cfg, err := LoadConfig()
	if err != nil {
		cfg = make(Config)
	}

	cfg[host] = HostConfig{OAuthToken: token}

	path, err := configPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o600)
}
