//go:build integration

package jj

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIntegration_WorkspaceRoot(t *testing.T) {
	dir := initJJRepo(t)

	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	// jj canonicalizes the root path, so compare with symlinks resolved
	// (e.g. /tmp vs /private/tmp on macOS).
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}

	for _, from := range []string{dir, sub} {
		got, err := WorkspaceRoot(from)
		if err != nil {
			t.Fatalf("WorkspaceRoot(%q): %v", from, err)
		}
		resolved, err := filepath.EvalSymlinks(got)
		if err != nil {
			t.Fatal(err)
		}
		if resolved != want {
			t.Errorf("WorkspaceRoot(%q) = %q, want %q", from, got, want)
		}
	}

	got, err := WorkspaceRoot(t.TempDir())
	if err != nil {
		t.Fatalf("WorkspaceRoot outside a repo: %v", err)
	}
	if got != "" {
		t.Errorf("WorkspaceRoot outside a repo = %q, want empty", got)
	}
}
