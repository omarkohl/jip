//go:build integration

package jj

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegration_PRTemplate(t *testing.T) {
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("jj", append([]string{"-R", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("jj %v: %v\n%s", args, err, out)
		}
	}
	cmd := exec.Command("jj", "git", "init", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	run("config", "set", "--repo", "user.name", "Test")
	run("config", "set", "--repo", "user.email", "test@example.com")
	write := func(name, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	runner := NewRunner(dir)
	if body, err := runner.PRTemplate("root()"); err != nil || body != "" {
		t.Fatalf("missing: %q, %v", body, err)
	}
	if _, err := runner.PRTemplate("nonexistent_revision"); err == nil {
		t.Fatal("missing revision treated as missing template")
	}
	write(".github/PULL_REQUEST_TEMPLATE/bug.md", "directory ignored")
	write("docs/pull_request_template.md", "docs")
	run("commit", "-m", "docs template")
	if body, err := runner.PRTemplate("@-"); err != nil || body != "docs" {
		t.Fatalf("docs: %q, %v", body, err)
	}
	write("PuLl_ReQuEsT_TeMpLaTe.Md", "root")
	run("commit", "-m", "root template")
	if body, err := runner.PRTemplate("@-"); err != nil || body != "root" {
		t.Fatalf("root: %q, %v", body, err)
	}
	write(".github/PULL_REQUEST_TEMPLATE.md", "github\n")
	run("commit", "-m", "github template")
	write(".github/PULL_REQUEST_TEMPLATE.md", "working copy ignored")
	if body, err := runner.PRTemplate("@-"); err != nil || body != "github\n" {
		t.Fatalf("github: %q, %v", body, err)
	}
	write(".github/pull_request_template.md", "ambiguous")
	run("commit", "-m", "ambiguous templates")
	if _, err := runner.PRTemplate("@-"); err == nil || !strings.Contains(err.Error(), "multiple PR templates") {
		t.Fatalf("ambiguous templates: %v", err)
	}
}
