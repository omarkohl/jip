package jj

import (
	"fmt"
	"os/exec"
	"strings"
)

// logTemplate is the jj template that outputs one JSON object per line.
const logTemplate = "" +
	`"{" ++` +
	`"\"change_id\":" ++ json(change_id) ++` +
	`",\"commit_id\":" ++ json(commit_id) ++` +
	`",\"description\":" ++ json(description.first_line()) ++` +
	`",\"parent_ids\":[" ++ parents.map(|c| json(c.change_id())).join(",") ++ "]" ++` +
	`",\"bookmarks\":[" ++ local_bookmarks.map(|r| json(r.name())).join(",") ++ "]" ++` +
	`"}\n"`

// bookmarkListTemplate outputs one JSON object per bookmark entry (local or remote).
// Local entries have remote=null; remote entries have the remote name.
// The "git" internal remote is filtered out during parsing.
const bookmarkListTemplate = "" +
	`"{" ++` +
	`"\"name\":" ++ json(name) ++` +
	`",\"remote\":" ++ if(remote, json(remote), "null") ++` +
	`",\"present\":" ++ if(present, "true", "false") ++` +
	`",\"conflict\":" ++ if(conflict, "true", "false") ++` +
	`",\"target\":" ++ if(present && !conflict, json(normal_target.commit_id()), "\"\"") ++` +
	`",\"change_id\":" ++ if(present && !conflict, json(normal_target.change_id()), "\"\"") ++` +
	`",\"tracked\":" ++ if(remote && tracked, "true", "false") ++` +
	`",\"ahead\":" ++ if(remote && tracked, if(tracking_ahead_count.exact(), tracking_ahead_count.exact(), "0"), "0") ++` +
	`",\"behind\":" ++ if(remote && tracked, if(tracking_behind_count.exact(), tracking_behind_count.exact(), "0"), "0") ++` +
	`"}\n"`

// Runner executes jj commands and returns their output.
type Runner interface {
	// Log runs jj log with the given revset and returns raw JSONL output.
	Log(revset string) ([]byte, error)

	// BookmarkList runs jj bookmark list --all-remotes and returns raw JSONL output.
	BookmarkList() ([]byte, error)

	// BookmarkSet creates or moves a bookmark to the given revision.
	BookmarkSet(name, rev string) error

	// GitRemoteList returns the output of jj git remote list.
	GitRemoteList() ([]byte, error)

	// GitFetch fetches from the given remote.
	GitFetch(remote string) error

	// GitPush pushes the given bookmarks. remote optionally specifies the
	// push target (empty = jj default). allowNew permits new remote branches.
	GitPush(bookmarks []string, allowNew bool, remote string) error

	// Interdiff returns the diff between two revisions using jj interdiff --git.
	Interdiff(from, to string) (string, error)

	// Rebase rebases the given revsets onto the destination revision.
	Rebase(revsets []string, destination string) error
}

// NewRunner creates a Runner that executes jj in the given repository directory.
func NewRunner(repoDir string) Runner {
	return &realRunner{repoDir: repoDir}
}

type realRunner struct {
	repoDir string
}

func (r *realRunner) Log(revset string) ([]byte, error) {
	args := []string{
		"log",
		"--no-graph",
		"-R", r.repoDir,
		"-r", revset,
		"-T", logTemplate,
	}
	cmd := exec.Command("jj", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("jj log: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func (r *realRunner) BookmarkList() ([]byte, error) {
	args := []string{
		"bookmark", "list",
		"--all-remotes",
		"--quiet",
		"-R", r.repoDir,
		"-T", bookmarkListTemplate,
	}
	cmd := exec.Command("jj", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("jj bookmark list: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func (r *realRunner) BookmarkSet(name, rev string) error {
	args := []string{
		"bookmark", "set",
		"-R", r.repoDir,
		name,
		"-r", rev,
	}
	cmd := exec.Command("jj", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("jj bookmark set: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (r *realRunner) GitRemoteList() ([]byte, error) {
	cmd := exec.Command("jj", "git", "remote", "list", "-R", r.repoDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("jj git remote list: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func (r *realRunner) GitFetch(remote string) error {
	args := []string{"git", "fetch", "-R", r.repoDir, "--remote", remote}
	cmd := exec.Command("jj", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("jj git fetch: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (r *realRunner) GitPush(bookmarks []string, allowNew bool, remote string) error {
	args := []string{"git", "push", "-R", r.repoDir}
	if remote != "" {
		args = append(args, "--remote", remote)
	}
	for _, b := range bookmarks {
		args = append(args, "-b", b)
	}
	if allowNew {
		args = append(args, "--allow-new")
	}
	cmd := exec.Command("jj", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("jj git push: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (r *realRunner) Interdiff(from, to string) (string, error) {
	args := []string{
		"interdiff", "--git",
		"-R", r.repoDir,
		"--from", from,
		"--to", to,
	}
	cmd := exec.Command("jj", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("jj interdiff: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func (r *realRunner) Rebase(revsets []string, destination string) error {
	args := []string{"rebase", "-R", r.repoDir, "-d", destination}
	for _, rev := range revsets {
		args = append(args, "-b", rev)
	}
	cmd := exec.Command("jj", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("jj rebase: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ParseRemoteList parses the output of jj git remote list into a map
// of remote name → URL.
func ParseRemoteList(data []byte) map[string]string {
	remotes := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 {
			remotes[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return remotes
}
