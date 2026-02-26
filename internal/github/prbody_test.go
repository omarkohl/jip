package github

import (
	"fmt"
	"strings"
	"testing"
)

func TestBuildStackBlock_SinglePR(t *testing.T) {
	result := BuildStackBlock([]int{1}, 1)
	if result != "" {
		t.Errorf("expected empty for single PR, got %q", result)
	}
}

func TestBuildStackBlock_MultiplePRs(t *testing.T) {
	result := BuildStackBlock([]int{1, 2, 3}, 2)
	if !strings.Contains(result, "PRs:") {
		t.Errorf("expected 'PRs:' header, got:\n%s", result)
	}
	if !strings.Contains(result, "* ➡️ #2") {
		t.Errorf("expected current PR arrow marker, got:\n%s", result)
	}
	if !strings.Contains(result, "* #1\n") {
		t.Errorf("expected #1 in stack, got:\n%s", result)
	}
	if !strings.Contains(result, "* #3\n") {
		t.Errorf("expected #3 in stack, got:\n%s", result)
	}
	// #3 should appear before #1 (top-to-bottom = newest first).
	idx3 := strings.Index(result, "#3")
	idx1 := strings.Index(result, "#1")
	if idx3 > idx1 {
		t.Errorf("expected #3 before #1 (newest first), got:\n%s", result)
	}
}

func TestBuildStackedPRBody_WithStack(t *testing.T) {
	body := BuildStackedPRBody("abcdef1234567890", "owner/repo", 2, []int{1, 2, 3}, "Some description")
	if !strings.Contains(body, "stacked PR") {
		t.Error("expected stacked PR intro")
	}
	if !strings.Contains(body, "[abcdef1](https://github.com/owner/repo/pull/2/commits/abcdef1234567890)") {
		t.Errorf("expected commit link, got:\n%s", body)
	}
	if !strings.Contains(body, "PRs:") {
		t.Error("expected stack block")
	}
	if !strings.Contains(body, "Some description") {
		t.Error("expected commit body")
	}
	if !strings.Contains(body, "[^1]:") {
		t.Error("expected footnote")
	}
	if strings.Contains(body, "Managed by") {
		t.Error("should not contain old jip footer")
	}
}

func TestBuildStackedPRBody_NoStack(t *testing.T) {
	body := BuildStackedPRBody("abc123", "owner/repo", 1, []int{1}, "my body")
	if strings.Contains(body, "stacked PR") {
		t.Error("expected no stacked PR intro for single PR")
	}
	if body != "my body" {
		t.Errorf("expected plain commit body for single PR, got %q", body)
	}
}

func TestBuildStackedPRBody_NoStack_EmptyBody(t *testing.T) {
	body := BuildStackedPRBody("abc123", "owner/repo", 1, []int{1}, "")
	if body != "" {
		t.Errorf("expected empty body for single PR with no commit body, got %q", body)
	}
}

func TestBuildDiffComment_EmptyDiff(t *testing.T) {
	result := BuildDiffComment("", "owner/repo", "main", "aaa111", "bbb222")
	if !strings.Contains(result, "Changes since last push") {
		t.Errorf("expected 'Changes since last push' header, got:\n%s", result)
	}
	if !strings.Contains(result, "**None.**") {
		t.Errorf("expected 'None' message for empty diff, got:\n%s", result)
	}
}

func TestBuildDiffComment_WithDiff(t *testing.T) {
	diff := `diff --git a/foo.go b/foo.go
--- a/foo.go
+++ b/foo.go
@@ -1,3 +1,4 @@
 package foo
+// added line
 func Bar() {}
-// old comment
`
	result := BuildDiffComment(diff, "owner/repo", "main", "old1234567890ab", "new4567890abcde")
	if !strings.Contains(result, "Changes since last push") {
		t.Error("expected 'Changes since last push' header")
	}
	if !strings.Contains(result, "<details") {
		t.Error("expected collapsible section")
	}
	if !strings.Contains(result, "```diff") {
		t.Error("expected diff code block")
	}
	if !strings.Contains(result, "+1, -1") {
		t.Errorf("expected +1, -1 stats, got:\n%s", result)
	}
	// Should have open attribute since diff is small.
	if !strings.Contains(result, "<details open>") {
		t.Errorf("expected open details for small diff, got:\n%s", result)
	}
	// Should have compare link.
	if !strings.Contains(result, "Compare on GitHub") {
		t.Errorf("expected GitHub compare link, got:\n%s", result)
	}
	if !strings.Contains(result, "range-diff") {
		t.Errorf("expected range-diff hint, got:\n%s", result)
	}
}

func TestBuildDiffComment_LargeDiff_CollapsedByDefault(t *testing.T) {
	// Build a diff with enough lines to exceed the collapse threshold.
	var diffLines []string
	diffLines = append(diffLines, "diff --git a/big.go b/big.go")
	diffLines = append(diffLines, "--- a/big.go")
	diffLines = append(diffLines, "+++ b/big.go")
	diffLines = append(diffLines, "@@ -1,5 +1,30 @@")
	for i := 0; i < 25; i++ {
		diffLines = append(diffLines, fmt.Sprintf("+line %d", i))
	}
	diff := strings.Join(diffLines, "\n")

	result := BuildDiffComment(diff, "owner/repo", "main", "old123", "new456")
	if strings.Contains(result, "<details open>") {
		t.Errorf("expected collapsed details for large diff, got:\n%s", result)
	}
	if !strings.Contains(result, "<details>") {
		t.Errorf("expected details element, got:\n%s", result)
	}
}

func TestBuildDiffComment_EmptyDiff_WithFooter(t *testing.T) {
	result := BuildDiffComment("", "owner/repo", "main", "aaa111222333", "bbb444555666")
	if !strings.Contains(result, "Compare on GitHub") {
		t.Errorf("expected compare link even for empty diff, got:\n%s", result)
	}
}

func TestParseGitDiff_MultipleFiles(t *testing.T) {
	diff := `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1 +1,2 @@
 package a
+// new
diff --git a/b.go b/b.go
--- a/b.go
+++ b/b.go
@@ -1 +0,0 @@
-package b
`
	files := parseGitDiff(diff)
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if files[0].header != "a.go" {
		t.Errorf("expected header 'a.go', got %q", files[0].header)
	}
	if files[1].header != "b.go" {
		t.Errorf("expected header 'b.go', got %q", files[1].header)
	}
}

func TestDiffStats(t *testing.T) {
	chunk := `--- a/file.go
+++ b/file.go
@@ -1,3 +1,4 @@
 unchanged
+added1
+added2
-removed1
`
	added, removed := diffStats(chunk)
	if added != 2 {
		t.Errorf("expected 2 added, got %d", added)
	}
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}
}

func TestRangeDiffFooter_Empty(t *testing.T) {
	result := rangeDiffFooter("", "main", "old", "new")
	if result != "" {
		t.Errorf("expected empty footer with no repo name, got %q", result)
	}
}

func TestRangeDiffFooter_WithData(t *testing.T) {
	result := rangeDiffFooter("owner/repo", "main", "old1234567890", "new4567890123")
	if !strings.Contains(result, "https://github.com/owner/repo/compare/old1234567890..new4567890123") {
		t.Errorf("expected compare URL, got:\n%s", result)
	}
	if !strings.Contains(result, "git range-diff main old1234 new4567") {
		t.Errorf("expected range-diff command, got:\n%s", result)
	}
}
