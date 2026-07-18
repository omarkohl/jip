package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setGlobalConfig points the global config dir at a temp dir and writes the
// given config.toml content (skipped when content is empty).
func setGlobalConfig(t *testing.T, content string) {
	t.Helper()
	dir := t.TempDir()
	old := Dir
	Dir = dir
	t.Cleanup(func() { Dir = old })
	if content != "" {
		jipDir := filepath.Join(dir, "jip")
		if err := os.MkdirAll(jipDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(jipDir, "config.toml"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// writeGlobalLocalConfig writes config.local.toml next to the global config.
// It must be called after setGlobalConfig, which points Dir at a temp dir.
func writeGlobalLocalConfig(t *testing.T, content string) {
	t.Helper()
	globalPath, err := GlobalPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(globalPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(localSibling(globalPath), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// writeRepoConfig creates a temp repo root containing a .jip.toml.
func writeRepoConfig(t *testing.T, content string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".jip.toml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

// writeRepoLocalConfig writes .jip.local.toml into an existing repo root.
func writeRepoLocalConfig(t *testing.T, root, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, ".jip.local.toml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_MissingFiles(t *testing.T) {
	setGlobalConfig(t, "")
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg) != 0 {
		t.Errorf("expected empty config, got %v", cfg)
	}
}

func TestLoad_RepoOverridesGlobal(t *testing.T) {
	setGlobalConfig(t, "rebase = true\nbase = \"main\"\n")
	root := writeRepoConfig(t, "base = \"dev\"\ndraft = true\n")

	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := map[string]string{
		"rebase": "true", // global only
		"base":   "dev",  // repo overrides global
		"draft":  "true", // repo only
	}
	for k, v := range want {
		if cfg[k] != v {
			t.Errorf("cfg[%q] = %q, want %q", k, cfg[k], v)
		}
	}
}

func TestLoad_GlobalLocalOverridesGlobal(t *testing.T) {
	setGlobalConfig(t, "base = \"main\"\ndraft = false\n")
	writeGlobalLocalConfig(t, "base = \"dev\"\nrebase = true\n")
	root := writeRepoConfig(t, "base = \"release\"\n")

	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := map[string]string{
		"base":   "release", // repo overrides global local
		"draft":  "false",   // global only
		"rebase": "true",    // global local only
	}
	for k, v := range want {
		if cfg[k] != v {
			t.Errorf("cfg[%q] = %q, want %q", k, cfg[k], v)
		}
	}
}

// TestLoad_Precedence pins the full layering: each location overrides the one
// before it, and a .local. file overrides its own sibling.
func TestLoad_Precedence(t *testing.T) {
	setGlobalConfig(t, "base = \"a\"\nremote = \"a\"\nupstream = \"a\"\ndraft = true\n")
	writeGlobalLocalConfig(t, "base = \"b\"\nremote = \"b\"\nupstream = \"b\"\n")
	root := writeRepoConfig(t, "base = \"c\"\nremote = \"c\"\n")
	writeRepoLocalConfig(t, root, "base = \"d\"\n")

	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := map[string]string{
		"base":     "d",    // repo local wins over all
		"remote":   "c",    // repo wins over global local
		"upstream": "b",    // global local wins over global
		"draft":    "true", // global only
	}
	for k, v := range want {
		if cfg[k] != v {
			t.Errorf("cfg[%q] = %q, want %q", k, cfg[k], v)
		}
	}
}

func TestLocalSibling(t *testing.T) {
	tests := []struct{ path, want string }{
		{filepath.Join("cfg", "config.toml"), filepath.Join("cfg", "config.local.toml")},
		{filepath.Join("repo", ".jip.toml"), filepath.Join("repo", ".jip.local.toml")},
	}
	for _, tt := range tests {
		if got := localSibling(tt.path); got != tt.want {
			t.Errorf("localSibling(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestLoad_ValueTypes(t *testing.T) {
	setGlobalConfig(t, "")
	root := writeRepoConfig(t, `
rebase = true
base = "dev"
reviewer = ["alice", "team/backend"]
`)
	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg["rebase"] != "true" {
		t.Errorf("rebase = %q, want %q", cfg["rebase"], "true")
	}
	if cfg["base"] != "dev" {
		t.Errorf("base = %q, want %q", cfg["base"], "dev")
	}
	if cfg["reviewer"] != "alice,team/backend" {
		t.Errorf("reviewer = %q, want %q", cfg["reviewer"], "alice,team/backend")
	}
}

// TestLoad_GlobalConfigDirUnresolvable simulates an environment where
// os.UserConfigDir() cannot resolve a directory (e.g. $HOME unset in a
// minimal container). Load should degrade to "no global config" instead of
// failing outright, since the global config is optional.
func TestLoad_GlobalConfigDirUnresolvable(t *testing.T) {
	old := Dir
	Dir = ""
	t.Cleanup(func() { Dir = old })

	for _, key := range []string{"HOME", "XDG_CONFIG_HOME", "APPDATA"} {
		if val, ok := os.LookupEnv(key); ok {
			if err := os.Unsetenv(key); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.Setenv(key, val); err != nil {
					t.Fatal(err)
				}
			})
		}
	}

	if _, err := os.UserConfigDir(); err == nil {
		t.Skip("os.UserConfigDir() did not fail with env vars unset; cannot simulate on this platform")
	}

	root := writeRepoConfig(t, "base = \"dev\"\n")
	cfg, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg["base"] != "dev" {
		t.Errorf("base = %q, want %q", cfg["base"], "dev")
	}
}

func TestLoad_InvalidTOML(t *testing.T) {
	setGlobalConfig(t, "")
	root := writeRepoConfig(t, "rebase = \n")
	_, err := Load(root)
	if err == nil {
		t.Fatal("expected error for invalid TOML")
	}
	if !strings.Contains(err.Error(), ".jip.toml") {
		t.Errorf("error should name the file, got: %v", err)
	}
}

func TestLoad_UnsupportedValueType(t *testing.T) {
	setGlobalConfig(t, "")
	root := writeRepoConfig(t, "[section]\nkey = \"value\"\n")
	_, err := Load(root)
	if err == nil {
		t.Fatal("expected error for nested table")
	}
}
