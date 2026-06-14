package github

import (
	"fmt"
	"strings"
)

// collapseThreshold is the maximum total diff lines before collapsing
// file sections by default.
const collapseThreshold = 20

// pushedCommitMarkerPrefix is an invisible HTML-comment marker embedded in
// every PR body by jip. It records the commit jip pushed so that a later
// `send --diff-since-jip` can recover the previous jip push as the interdiff
// base, independent of what others pushed to the branch directly.
const pushedCommitMarkerPrefix = "<!-- jip:pushed-commit="

// pushedCommitMarker renders the marker for the given commit hash.
func pushedCommitMarker(hash string) string {
	return pushedCommitMarkerPrefix + hash + " -->"
}

// WithPushedCommitMarker ensures the PR body contains exactly one pushed-commit
// marker reflecting commit. If the body already has that exact commit, it is
// returned unchanged. Otherwise any existing markers are stripped and the new
// marker is appended.
func WithPushedCommitMarker(body, commit string) string {
	if commit == "" {
		return body
	}
	if ParsePushedCommit(body) == commit {
		return body
	}
	body = stripPushedCommitMarkers(body)
	marker := pushedCommitMarker(commit)
	if body == "" {
		return marker
	}
	return body + "\n\n" + marker
}

// stripPushedCommitMarkers removes all pushed-commit markers (and the \n\n
// separator that WithPushedCommitMarker prepends) from body.
func stripPushedCommitMarkers(body string) string {
	for {
		idx := strings.Index(body, pushedCommitMarkerPrefix)
		if idx == -1 {
			break
		}
		rest := body[idx+len(pushedCommitMarkerPrefix):]
		end := strings.Index(rest, "-->")
		if end == -1 {
			break
		}
		markerEnd := idx + len(pushedCommitMarkerPrefix) + end + len("-->")
		markerStart := idx
		if markerStart >= 2 && body[markerStart-2:markerStart] == "\n\n" {
			markerStart -= 2
		}
		body = body[:markerStart] + body[markerEnd:]
	}
	return body
}

// ParsePushedCommit extracts the commit hash from a jip pushed-commit marker
// in a PR body, or "" if the body has no marker or the value is not a valid
// hex hash. Uses LastIndex so that if multiple markers exist the newest one
// wins.
func ParsePushedCommit(commentBody string) string {
	idx := strings.LastIndex(commentBody, pushedCommitMarkerPrefix)
	if idx == -1 {
		return ""
	}
	rest := commentBody[idx+len(pushedCommitMarkerPrefix):]
	end := strings.Index(rest, "-->")
	if end == -1 {
		return ""
	}
	value := strings.TrimSpace(rest[:end])
	if len(value) < 7 {
		return ""
	}
	for i := range len(value) {
		if !isHexChar(value[i]) {
			return ""
		}
	}
	return value
}

// ParseReviewCommit extracts the commit hash from the "Only review commit"
// link that BuildStackedPRBody writes into a stacked PR's body, or "" if the
// body has no such link (e.g. a standalone, non-stacked PR).
//
// Only the line that starts with "Only review commit" is searched, and the
// hash is extracted from the exact /pull/<number>/commits/<hash> URL shape to
// avoid matching unrelated URLs in user-written descriptions.
func ParseReviewCommit(prBody string) string {
	const lineMarker = "Only review commit "
	idx := strings.Index(prBody, lineMarker)
	if idx == -1 {
		return ""
	}
	// Restrict search to the single line that contains the marker.
	line := prBody[idx:]
	if nl := strings.IndexByte(line, '\n'); nl != -1 {
		line = line[:nl]
	}
	const urlMarker = "/commits/"
	ci := strings.Index(line, urlMarker)
	if ci == -1 {
		return ""
	}
	rest := line[ci+len(urlMarker):]
	end := 0
	for end < len(rest) && isHexChar(rest[end]) {
		end++
	}
	if end < 7 {
		return ""
	}
	return rest[:end]
}

func isHexChar(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

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
			if i == 0 {
				// Current PR is the bottom of the stack.
				fmt.Fprintf(&b, "* ➡️ #%d (this PR, base of the stack — can be merged first)\n", num)
			} else {
				fmt.Fprintf(&b, "* ➡️ #%d (this PR, depends on the ones below ⬇️)\n", num)
			}
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
		b.WriteString("\n---\n\n## Description\n\n")
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
// using collapsible sections for each file. When sinceJip is true the header
// reads "Changes since last jip send" (the base is jip's own previous send
// rather than the current remote head).
func BuildDiffComment(codeDiff, repoName, baseBranch, oldCommit, newCommit string, sinceJip bool) string {
	footer := rangeDiffFooter(repoName, baseBranch, oldCommit, newCommit)
	header := "### Changes since last push\n"
	if sinceJip {
		header = "### Changes since last jip send\n"
	}

	if strings.TrimSpace(codeDiff) == "" {
		return header + "\n**No code changes** (likely just a rebase).\n" + footer
	}

	files := parseGitDiff(codeDiff)

	var b strings.Builder
	b.WriteString(header)

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
		fence := codeFence(f.body)
		fmt.Fprintf(&b, "\n<details%s>\n<summary><code>%s</code> (+%d, -%d)</summary>\n\n%sdiff\n%s\n%s\n\n</details>\n",
			openAttr, f.header, added, removed, fence, f.body, fence)
	}

	b.WriteString(footer)
	return b.String()
}

// BuildUnavailableDiffComment generates a PR comment for the case where
// --diff-since-jip knows the previous jip-pushed commit but cannot find it
// locally (e.g. it was pushed from another machine and not fetched). It
// documents that the diff could not be generated and points at the remote.
func BuildUnavailableDiffComment(repoName, baseBranch, oldCommit, newCommit string) string {
	oldShort := oldCommit[:minInt(7, len(oldCommit))]
	var b strings.Builder
	b.WriteString("### Changes since last jip send\n\n")
	fmt.Fprintf(&b,
		"⚠️ Could not generate the diff: the previously pushed commit `%s` is not "+
			"available locally (it may have been pushed from another machine). "+
			"Fetch the remote to inspect it.\n",
		oldShort)
	b.WriteString(rangeDiffFooter(repoName, baseBranch, oldCommit, newCommit))
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

// codeFence returns a backtick fence long enough to safely wrap content.
// If the content contains N consecutive backticks, the fence uses N+1.
func codeFence(content string) string {
	maxRun := 0
	run := 0
	for _, c := range content {
		if c == '`' {
			run++
			if run > maxRun {
				maxRun = run
			}
		} else {
			run = 0
		}
	}
	n := 3
	if maxRun >= n {
		n = maxRun + 1
	}
	return strings.Repeat("`", n)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
