package auth

import (
	"testing"
)

func TestResolveTokenFromEnvVar(t *testing.T) {
	// Isolate from gh CLI config by pointing GH_CONFIG_DIR to empty dir
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	t.Setenv("GH_TOKEN", "ghp_envtoken")

	token, source := ResolveToken("github.com")
	if token != "ghp_envtoken" {
		t.Errorf("got token %q, want %q", token, "ghp_envtoken")
	}
	if source != "GH_TOKEN" {
		t.Errorf("got source %q, want %q", source, "GH_TOKEN")
	}
}

func TestResolveTokenFromGitHubTokenEnv(t *testing.T) {
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "ghp_github_env")

	token, source := ResolveToken("github.com")
	if token != "ghp_github_env" {
		t.Errorf("got token %q, want %q", token, "ghp_github_env")
	}
	if source != "GITHUB_TOKEN" {
		t.Errorf("got source %q, want %q", source, "GITHUB_TOKEN")
	}
}

func TestResolveTokenGHTokenTakesPrecedence(t *testing.T) {
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	t.Setenv("GH_TOKEN", "ghp_first")
	t.Setenv("GITHUB_TOKEN", "ghp_second")

	token, _ := ResolveToken("github.com")
	if token != "ghp_first" {
		t.Errorf("GH_TOKEN should take precedence, got %q", token)
	}
}

func TestResolveTokenFromJipConfig(t *testing.T) {
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_PATH", "/nonexistent") // prevent shelling out to gh

	ConfigDir = t.TempDir()
	defer func() { ConfigDir = "" }()

	if err := SaveToken("github.com", "ghp_jip_token"); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}

	token, source := ResolveToken("github.com")
	if token != "ghp_jip_token" {
		t.Errorf("got token %q, want %q", token, "ghp_jip_token")
	}
	if source != "jip config" {
		t.Errorf("got source %q, want %q", source, "jip config")
	}
}

func TestResolveTokenReturnsEmptyWhenNothingConfigured(t *testing.T) {
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_PATH", "/nonexistent") // prevent shelling out to gh

	ConfigDir = t.TempDir()
	defer func() { ConfigDir = "" }()

	token, source := ResolveToken("github.com")
	if token != "" {
		t.Errorf("expected empty token, got %q", token)
	}
	if source != "" {
		t.Errorf("expected empty source, got %q", source)
	}
}
