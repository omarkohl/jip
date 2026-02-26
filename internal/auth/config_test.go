package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadToken(t *testing.T) {
	ConfigDir = t.TempDir()
	defer func() { ConfigDir = "" }()

	if err := SaveToken("github.com", "ghp_testtoken123"); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	got := cfg["github.com"].OAuthToken
	if got != "ghp_testtoken123" {
		t.Errorf("got token %q, want %q", got, "ghp_testtoken123")
	}
}

func TestSaveTokenPreservesOtherHosts(t *testing.T) {
	ConfigDir = t.TempDir()
	defer func() { ConfigDir = "" }()

	if err := SaveToken("github.com", "token1"); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}
	if err := SaveToken("github.example.com", "token2"); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg["github.com"].OAuthToken != "token1" {
		t.Error("first host token was overwritten")
	}
	if cfg["github.example.com"].OAuthToken != "token2" {
		t.Error("second host token not saved")
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	ConfigDir = t.TempDir()
	defer func() { ConfigDir = "" }()

	_, err := LoadConfig()
	if err == nil {
		t.Error("expected error for missing config file")
	}
}

func TestLoadConfigInvalidJSON(t *testing.T) {
	ConfigDir = t.TempDir()
	defer func() { ConfigDir = "" }()

	dir := filepath.Join(ConfigDir, "jip")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{invalid"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := LoadConfig()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestSaveTokenCreatesDirectory(t *testing.T) {
	ConfigDir = t.TempDir()
	defer func() { ConfigDir = "" }()

	// The jip subdirectory doesn't exist yet
	err := SaveToken("github.com", "tok")
	if err != nil {
		t.Fatalf("SaveToken should create directories: %v", err)
	}

	path := filepath.Join(ConfigDir, "jip", "config.json")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("config file not created: %v", err)
	}
}
