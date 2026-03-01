package github

import (
	"fmt"
	"strings"
)

// collapseThreshold is the maximum total diff lines before collapsing
// file sections by default.
const collapseThreshold = 20

// BuildStackBlock generates a markdown stack navigation block showing
// the current PR's position in the stack.
func BuildStackBlock(prNumbers []int, current int) string {
	if len(prNumbers) <= 1 {
		return ""
	}

	var b strings.Builder
	b.WriteString("PRs:\n")
	// Display top-to-bottom (newest first).
	for i := len(prNumbers) - 1; i >= 0; i-- {
		num := prNumbers[i]
		if num == current {
			fmt.Fprintf(&b, "* ➡️ #%d\n", num)
		} else {
			fmt.Fprintf(&b, "* #%d\n", num)
		}
	}
	return b.String()
}

// BuildStackedPRBody generates the full PR body for a stacked PR.
// For a single PR (len(allPRs) <= 1), only the commitBody is returned.
func BuildStackedPRBody(commitHash, repoFullName string, prNumber int, allPRs []int, commitBody string) string {
	// Single PR: just use the commit body directly.
	if len(allPRs) <= 1 {
		return commitBody
	}

	shortHash := commitHash[:minInt(7, len(commitHash))]
	commitLink := fmt.Sprintf("https://github.com/%s/pull/%d/commits/%s", repoFullName, prNumber, commitHash)

	var b strings.Builder
	fmt.Fprintf(&b, "This is a stacked PR[^1]. Only review commit [%s](%s).\n\n", shortHash, commitLink)

	b.WriteString(BuildStackBlock(allPRs, prNumber))

	if commitBody != "" {
		b.WriteString("\n---\n\n")
		b.WriteString(commitBody)
		b.WriteString("\n")
	}

	b.WriteString("\n[^1]: A stacked PR is a pull request that depends on other pull requests. ")
	b.WriteString("The current PR depends on the ones listed below it and MUST NOT be merged before they are merged. ")
	b.WriteString("The PRs listed above the current one in turn depend on it and won't be merged until the current one is. ")
	b.WriteString("Learn more about [why](https://github.com/omarkohl/jip/blob/main/docs/why.md) and [how to review](https://github.com/omarkohl/jip/blob/main/docs/reviewing.md).\n")

	return b.String()
}

// fileDiff represents a single file's diff section.
type fileDiff struct {
	header string // the diff --git a/... b/... line and hunks header
	body   string // the actual diff content
}

// BuildDiffComment generates a PR comment with interdiff output,
// using collapsible sections for each file.
func BuildDiffComment(codeDiff, repoName, baseBranch, oldCommit, newCommit string) string {
	footer := rangeDiffFooter(repoName, baseBranch, oldCommit, newCommit)

	if strings.TrimSpace(codeDiff) == "" {
		return "### Changes since last push\n\n**No code changes** (likely just a rebase).\n" + footer
	}

	files := parseGitDiff(codeDiff)

	var b strings.Builder
	b.WriteString("### Changes since last push\n")

	totalLines := 0
	for _, f := range files {
		totalLines += len(strings.Split(f.body, "\n"))
	}
	expand := totalLines <= collapseThreshold

	for _, f := range files {
		added, removed := diffStats(f.body)
		openAttr := ""
		if expand {
			openAttr = " open"
		}
		fmt.Fprintf(&b, "\n<details%s>\n<summary><code>%s</code> (+%d, -%d)</summary>\n\n```diff\n%s\n```\n\n</details>\n",
			openAttr, f.header, added, removed, f.body)
	}

	b.WriteString(footer)
	return b.String()
}

// rangeDiffFooter builds a footer with a GitHub compare link and a local
// range-diff command hint.
func rangeDiffFooter(repoName, baseBranch, oldCommit, newCommit string) string {
	if oldCommit == "" || newCommit == "" || repoName == "" {
		return ""
	}
	oldShort := oldCommit[:minInt(7, len(oldCommit))]
	newShort := newCommit[:minInt(7, len(newCommit))]
	compareURL := fmt.Sprintf("https://github.com/%s/compare/%s..%s", repoName, oldCommit, newCommit)
	return fmt.Sprintf(
		"\n---\n<sub>View the diff on [GitHub](%s) "+
			"(may include unrelated changes due to rebases since GitHub does not currently implement `git range-diff`).\n"+
			"View the diff locally (will only work if you fetched the older commit at some point):\n"+
			"`git range-diff %s %s %s`\n"+
			"`jj interdiff -f %s -t %s`\n"+
			"</sub>\n",
		compareURL, baseBranch, oldShort, newShort, oldShort, newShort,
	)
}

// parseGitDiff splits a unified diff into per-file sections.
func parseGitDiff(diff string) []fileDiff {
	var files []fileDiff
	lines := strings.Split(diff, "\n")
	var current *fileDiff

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			if current != nil {
				current.body = strings.TrimRight(current.body, "\n")
				files = append(files, *current)
			}
			// Extract file path from "diff --git a/path b/path"
			header := line
			parts := strings.SplitN(line, " b/", 2)
			if len(parts) == 2 {
				header = parts[1]
			}
			current = &fileDiff{header: header}
			continue
		}
		if current != nil {
			current.body += line + "\n"
		}
	}
	if current != nil {
		current.body = strings.TrimRight(current.body, "\n")
		files = append(files, *current)
	}
	return files
}

// diffStats counts added and removed lines in a diff chunk.
func diffStats(chunk string) (added, removed int) {
	for _, line := range strings.Split(chunk, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			added++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			removed++
		}
	}
	return
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
