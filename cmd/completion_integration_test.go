//go:build integration

package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestIntegration_CompleteJJRevsets(t *testing.T) {
	checkJJ(t)

	repoDir, _ := initTestRepoWithRemote(t)

	// Create two changes with bookmarks.
	writeAndCommit(t, repoDir, "a.go", "package a", "feat: alpha feature")
	writeAndCommit(t, repoDir, "b.go", "package b", "fix: beta bugfix")

	// Push a bookmark so we also get remote bookmark completions.
	jjRun(t, repoDir, "bookmark", "set", "mybranch", "-r", "@-")
	jjRun(t, repoDir, "git", "push", "--bookmark", "mybranch")

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(oldDir) })
	os.Chdir(repoDir)

	completions, directive := completeJJRevsets(nil, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected ShellCompDirectiveNoFileComp, got %v", directive)
	}

	if len(completions) == 0 {
		t.Fatal("expected completions, got none")
	}

	// Should contain the "main" bookmark.
	found := false
	for _, c := range completions {
		if strings.HasPrefix(c, "main\t") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'main' in completions, got: %v", completions)
	}

	// Should contain "mybranch".
	found = false
	for _, c := range completions {
		if strings.HasPrefix(c, "mybranch\t") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'mybranch' in completions, got: %v", completions)
	}

	// Should contain change IDs (short IDs with descriptions).
	hasChangeID := false
	for _, c := range completions {
		// Change IDs are short strings that aren't bookmarks or tags.
		name := strings.SplitN(c, "\t", 2)[0]
		if name != "main" && !strings.Contains(name, "@") && !strings.HasPrefix(name, "v") && name != "mybranch" {
			hasChangeID = true
			break
		}
	}
	if !hasChangeID {
		t.Errorf("expected change ID completions, got: %v", completions)
	}

	// No completions should start with "--".
	for _, c := range completions {
		if strings.HasPrefix(c, "--") {
			t.Errorf("unexpected flag in completions: %s", c)
		}
	}
}

func TestIntegration_CompleteJJRevsetsWithPrefix(t *testing.T) {
	checkJJ(t)

	repoDir, _ := initTestRepoWithRemote(t)
	writeAndCommit(t, repoDir, "a.go", "package a", "feat: something")

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(oldDir) })
	os.Chdir(repoDir)

	completions, directive := completeJJRevsets(nil, nil, "mai")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected ShellCompDirectiveNoFileComp, got %v", directive)
	}

	// Should return only matches for "mai" prefix — at minimum "main".
	if len(completions) == 0 {
		t.Fatal("expected completions for 'mai', got none")
	}
	for _, c := range completions {
		name := strings.SplitN(c, "\t", 2)[0]
		if !strings.HasPrefix(name, "mai") {
			t.Errorf("completion %q does not match prefix 'mai'", name)
		}
	}
}

func TestIntegration_CompleteJJRevsetsWithOperator(t *testing.T) {
	checkJJ(t)

	repoDir, _ := initTestRepoWithRemote(t)
	writeAndCommit(t, repoDir, "a.go", "package a", "feat: something")
	jjRun(t, repoDir, "bookmark", "set", "secondary", "-r", "@-")

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(oldDir) })
	os.Chdir(repoDir)

	// Completing after ".." operator should return revisions.
	completions, directive := completeJJRevsets(nil, nil, "main..sec")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected ShellCompDirectiveNoFileComp, got %v", directive)
	}

	if len(completions) == 0 {
		t.Fatal("expected completions for 'main..sec', got none")
	}

	found := false
	for _, c := range completions {
		if strings.HasPrefix(c, "main..secondary") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'main..secondary' in completions, got: %v", completions)
	}
}

func TestIntegration_CompleteJJBookmarks(t *testing.T) {
	checkJJ(t)

	repoDir, _ := initTestRepoWithRemote(t)

	writeAndCommit(t, repoDir, "a.go", "package a", "feat: alpha")
	jjRun(t, repoDir, "bookmark", "set", "feature-x", "-r", "@-")

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(oldDir) })
	os.Chdir(repoDir)

	completions, directive := completeJJBookmarks(nil, nil, "")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected ShellCompDirectiveNoFileComp, got %v", directive)
	}

	if len(completions) == 0 {
		t.Fatal("expected bookmark completions, got none")
	}

	// Should contain "main" and "feature-x".
	names := make(map[string]bool)
	for _, c := range completions {
		name := strings.SplitN(c, "\t", 2)[0]
		names[name] = true
	}

	if !names["main"] {
		t.Errorf("expected 'main' in bookmark completions, got: %v", completions)
	}
	if !names["feature-x"] {
		t.Errorf("expected 'feature-x' in bookmark completions, got: %v", completions)
	}

	// Should NOT contain remote bookmarks (e.g. main@origin) or change IDs.
	for _, c := range completions {
		name := strings.SplitN(c, "\t", 2)[0]
		if strings.Contains(name, "@") {
			t.Errorf("bookmark completion should not contain remote bookmark: %s", name)
		}
		if strings.HasPrefix(name, "--") {
			t.Errorf("bookmark completion should not contain flags: %s", name)
		}
	}
}

func TestIntegration_CompleteJJBookmarksWithPrefix(t *testing.T) {
	checkJJ(t)

	repoDir, _ := initTestRepoWithRemote(t)

	writeAndCommit(t, repoDir, "a.go", "package a", "feat: alpha")
	jjRun(t, repoDir, "bookmark", "set", "feature-x", "-r", "@-")

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(oldDir) })
	os.Chdir(repoDir)

	completions, directive := completeJJBookmarks(nil, nil, "fea")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected ShellCompDirectiveNoFileComp, got %v", directive)
	}

	if len(completions) == 0 {
		t.Fatal("expected completions for 'fea', got none")
	}

	for _, c := range completions {
		name := strings.SplitN(c, "\t", 2)[0]
		if !strings.HasPrefix(name, "fea") {
			t.Errorf("completion %q does not match prefix 'fea'", name)
		}
	}

	found := false
	for _, c := range completions {
		if strings.HasPrefix(c, "feature-x\t") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'feature-x' in completions, got: %v", completions)
	}
}

func TestIntegration_CompleteJJBookmarksNoMatch(t *testing.T) {
	checkJJ(t)

	repoDir, _ := initTestRepoWithRemote(t)

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(oldDir) })
	os.Chdir(repoDir)

	completions, directive := completeJJBookmarks(nil, nil, "zzz-nonexistent")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected ShellCompDirectiveNoFileComp, got %v", directive)
	}

	if len(completions) != 0 {
		t.Errorf("expected no completions for 'zzz-nonexistent', got: %v", completions)
	}
}

func TestIntegration_CompleteJJRevsetsNoMatch(t *testing.T) {
	checkJJ(t)

	repoDir, _ := initTestRepoWithRemote(t)

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(oldDir) })
	os.Chdir(repoDir)

	completions, directive := completeJJRevsets(nil, nil, "zzz-nonexistent")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected ShellCompDirectiveNoFileComp, got %v", directive)
	}

	if len(completions) != 0 {
		t.Errorf("expected no completions for 'zzz-nonexistent', got: %v", completions)
	}
}
