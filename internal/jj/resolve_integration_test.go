//go:build integration

package jj

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegration_LinearStack(t *testing.T) {
	dir := initJJRepo(t)
	runner := NewRunner(dir)

	// Create 3 commits on top of main.
	writeAndCommit(t, dir, "a.txt", "aaa", "first change")
	writeAndCommit(t, dir, "b.txt", "bbb", "second change")
	writeAndCommit(t, dir, "c.txt", "ccc", "third change")

	logRepoState(t, dir)

	// @- is the last committed change (third change). Resolve the full stack.
	dags, err := ResolveStacks(runner, []string{"@-"}, "main")
	if err != nil {
		t.Fatalf("ResolveStacks: %v", err)
	}
	logDAGs(t, dags)

	if len(dags) != 1 {
		t.Fatalf("expected 1 DAG, got %d", len(dags))
	}
	if len(dags[0].Changes) != 3 {
		t.Fatalf("expected 3 changes, got %d", len(dags[0].Changes))
	}
	// Verify topological order: first committed change should be first.
	if dags[0].Changes[0].Description != "first change" {
		t.Errorf("expected first change at position 0, got %q", dags[0].Changes[0].Description)
	}
}

func TestIntegration_TwoIndependentBranches(t *testing.T) {
	dir := initJJRepo(t)
	runner := NewRunner(dir)

	// Create first branch and capture its change ID.
	writeAndCommit(t, dir, "a.txt", "aaa", "branch-a")
	idA := getChangeID(t, dir, "@-")

	// Go back to main to create an independent branch.
	jjRun(t, dir, "new", "main")
	writeAndCommit(t, dir, "b.txt", "bbb", "branch-b")
	idB := getChangeID(t, dir, "@-")

	logRepoState(t, dir)

	// Resolve both branches by their change IDs.
	dags, err := ResolveStacks(runner, []string{idA, idB}, "main")
	if err != nil {
		t.Fatalf("ResolveStacks: %v", err)
	}
	logDAGs(t, dags)

	if len(dags) != 2 {
		t.Fatalf("expected 2 DAGs, got %d", len(dags))
	}
	// Each DAG should have exactly 1 change.
	for i, dag := range dags {
		if len(dag.Changes) != 1 {
			t.Errorf("DAG %d: expected 1 change, got %d", i, len(dag.Changes))
		}
	}
}

func TestIntegration_SingleCommit(t *testing.T) {
	dir := initJJRepo(t)
	runner := NewRunner(dir)

	writeAndCommit(t, dir, "a.txt", "aaa", "only change")

	logRepoState(t, dir)

	dags, err := ResolveStacks(runner, []string{"@-"}, "main")
	if err != nil {
		t.Fatalf("ResolveStacks: %v", err)
	}
	logDAGs(t, dags)

	if len(dags) != 1 {
		t.Fatalf("expected 1 DAG, got %d", len(dags))
	}
	if len(dags[0].Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(dags[0].Changes))
	}
}

func TestIntegration_OverlappingRevsetsThreeDAGs(t *testing.T) {
	dir := initJJRepo(t)
	runner := NewRunner(dir)

	// Build repo structure:
	//   main (initial commit)
	//   ├── A "feat: add auth module"
	//   │   ├── B "feat: add login endpoint"
	//   │   │   └── C "feat: add session handling"
	//   │   └── D "feat: add oauth provider" [bookmark: oauth-support]
	//   ├── E "fix: database connection pool"
	//   │   └── F "fix: database retry logic"
	//   └── G "refactor: extract config parser"

	// Linear chain A → B → C
	writeAndCommit(t, dir, "auth.go", "package auth", "feat: add auth module")
	idA := getChangeID(t, dir, "@-")
	writeAndCommit(t, dir, "login.go", "package auth", "feat: add login endpoint")
	writeAndCommit(t, dir, "session.go", "package auth", "feat: add session handling")

	// Branch off A to create D
	jjRun(t, dir, "new", idA)
	writeAndCommit(t, dir, "oauth.go", "package auth", "feat: add oauth provider")
	jjRun(t, dir, "bookmark", "set", "oauth-support", "-r", "@-")

	// Independent stack E → F from main
	jjRun(t, dir, "new", "main")
	writeAndCommit(t, dir, "pool.go", "package db", "fix: database connection pool")
	writeAndCommit(t, dir, "retry.go", "package db", "fix: database retry logic")

	// Independent single commit G from main
	jjRun(t, dir, "new", "main")
	writeAndCommit(t, dir, "config.go", "package config", "refactor: extract config parser")

	logRepoState(t, dir)

	// 4 revsets using different jj revset features.
	// Revsets 1 and 2 overlap: both resolve to changes in the A-B-C-D subgraph.
	// Note: jj description() defaults to exact matching; use substring:/glob: for patterns.
	revsets := []string{
		`description(substring:"session handling")`,                     // substring match → C
		`bookmarks(exact:"oauth-support")`,                              // bookmark → D (shares ancestor A with C)
		`heads(description(substring:"database"))`,                      // composition → F (head of {E,F})
		`description(glob:"*config*") & description(glob:"*refactor*")`, // intersection + glob → G
	}

	dags, err := ResolveStacks(runner, revsets, "main")
	if err != nil {
		t.Fatalf("ResolveStacks: %v", err)
	}

	logDAGs(t, dags)

	if len(dags) != 3 {
		t.Fatalf("expected 3 DAGs, got %d", len(dags))
	}

	// Identify each DAG by size (all three have distinct sizes).
	dagsBySize := map[int]*ChangeDAG{}
	for _, dag := range dags {
		dagsBySize[len(dag.Changes)] = dag
	}

	authDAG := dagsBySize[4]
	dbDAG := dagsBySize[2]
	configDAG := dagsBySize[1]

	if authDAG == nil || dbDAG == nil || configDAG == nil {
		t.Fatalf("expected DAGs of sizes 4, 2, 1; got sizes %v",
			func() []int {
				var s []int
				for _, d := range dags {
					s = append(s, len(d.Changes))
				}
				return s
			}())
	}

	// Auth DAG: A is the root (only change with no in-set parents).
	if authDAG.Changes[0].Description != "feat: add auth module" {
		t.Errorf("auth DAG: expected root to be A, got %q", authDAG.Changes[0].Description)
	}
	// C depends on B which depends on A, so C is always last.
	if authDAG.Changes[3].Description != "feat: add session handling" {
		t.Errorf("auth DAG: expected C at position 3, got %q", authDAG.Changes[3].Description)
	}
	// Verify all 4 descriptions are present.
	authDescs := map[string]bool{}
	for _, c := range authDAG.Changes {
		authDescs[c.Description] = true
	}
	for _, want := range []string{
		"feat: add auth module",
		"feat: add login endpoint",
		"feat: add session handling",
		"feat: add oauth provider",
	} {
		if !authDescs[want] {
			t.Errorf("auth DAG: missing %q", want)
		}
	}

	// Database DAG: E → F in topological order.
	if dbDAG.Changes[0].Description != "fix: database connection pool" {
		t.Errorf("db DAG: expected E first, got %q", dbDAG.Changes[0].Description)
	}
	if dbDAG.Changes[1].Description != "fix: database retry logic" {
		t.Errorf("db DAG: expected F second, got %q", dbDAG.Changes[1].Description)
	}

	// Config DAG: single change G.
	if configDAG.Changes[0].Description != "refactor: extract config parser" {
		t.Errorf("config DAG: expected G, got %q", configDAG.Changes[0].Description)
	}
}

// logRepoState logs the jj log output for manual inspection.
func logRepoState(t *testing.T, dir string) {
	t.Helper()
	t.Log("--- jj log ---")
	t.Log("\n" + jjRun(t, dir, "log", "-r", "::"))
}

// logDAGs logs the resolved DAGs for manual comparison.
func logDAGs(t *testing.T, dags []*ChangeDAG) {
	t.Helper()
	t.Logf("--- %d DAG(s) ---", len(dags))
	for i, dag := range dags {
		t.Logf("DAG %d (%d changes):", i, len(dag.Changes))
		for j, c := range dag.Changes {
			parents := strings.Join(c.ParentIDs, ", ")
			if parents == "" {
				parents = "(root)"
			}
			t.Logf("  [%d] %.12s %s  parents=[%s]", j, c.ChangeID, c.Description, parents)
		}
	}
}

// --- Test helpers ---

// initJJRepo creates a temporary jj repo with an initial commit on a "main"
// bookmark, simulating a realistic remote-backed repo. The working copy is
// left on top of main, ready for test commits.
// Set JIP_KEEP_REPO=1 to preserve the repo directory after the test.
func initJJRepo(t *testing.T) string {
	t.Helper()
	checkJJ(t)
	dir, err := os.MkdirTemp("", "jip-integration-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	if os.Getenv("JIP_KEEP_REPO") != "" {
		t.Logf("JIP_KEEP_REPO set — repo preserved at: %s", dir)
	} else {
		t.Cleanup(func() { os.RemoveAll(dir) })
	}
	// jj git init takes destination as positional arg, not via -R.
	cmd := exec.Command("jj", "git", "init", dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("jj git init: %v\n%s", err, out)
	}
	// Create an initial commit with a "main" bookmark to mimic a real repo.
	writeAndCommit(t, dir, "README.md", "# test repo", "initial commit")
	jjRun(t, dir, "bookmark", "set", "main", "-r", "@-")
	t.Logf("repo: %s", dir)
	return dir
}

func checkJJ(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not found in PATH, skipping integration test")
	}
}

func jjRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("jj", append([]string{"-R", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("jj %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func getChangeID(t *testing.T, dir, rev string) string {
	t.Helper()
	out := jjRun(t, dir, "log", "--no-graph", "-r", rev, "-T", "change_id")
	return strings.TrimSpace(out)
}

func writeAndCommit(t *testing.T, dir, filename, content, message string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatalf("writing %s: %v", filename, err)
	}
	jjRun(t, dir, "commit", "-m", message)
}
