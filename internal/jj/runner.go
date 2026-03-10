package jj

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/omarkohl/jip/internal/retry"
)

// logTemplate is the jj template that outputs one JSON object per line.
const logTemplate = "" +
	`"{" ++` +
	`"\"change_id\":" ++ json(change_id) ++` +
	`",\"commit_id\":" ++ json(commit_id) ++` +
	`",\"description\":" ++ json(description) ++` +
	`",\"conflict\":" ++ if(conflict, "true", "false") ++` +
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
	`",\"synced\":" ++ if(remote && tracked, if(synced, "true", "false"), "false") ++` +
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

	// ConfigGet returns the value of a jj configuration key.
	// Returns an error if the key is not set.
	ConfigGet(key string) (string, error)
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
	logCmd("jj", args)
	cmd := exec.Command("jj", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		slog.Debug("jj exec failed", "err", err, "output", strings.TrimSpace(string(out)), "stderr", strings.TrimSpace(stderr.String()))
		return nil, fmt.Errorf("jj log: %w\n%s", err, strings.TrimSpace(stderr.String()))
	}
	if s := strings.TrimSpace(stderr.String()); s != "" {
		slog.Debug("jj log stderr", "stderr", s)
	}
	slog.Debug("jj exec ok", "bytes", len(out))
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
	logCmd("jj", args)
	cmd := exec.Command("jj", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		slog.Debug("jj exec failed", "err", err, "output", strings.TrimSpace(string(out)), "stderr", strings.TrimSpace(stderr.String()))
		return nil, fmt.Errorf("jj bookmark list: %w\n%s", err, strings.TrimSpace(stderr.String()))
	}
	if s := strings.TrimSpace(stderr.String()); s != "" {
		slog.Debug("jj bookmark list stderr", "stderr", s)
	}
	slog.Debug("jj exec ok", "bytes", len(out))
	return out, nil
}

func (r *realRunner) BookmarkSet(name, rev string) error {
	args := []string{
		"bookmark", "set",
		"-R", r.repoDir,
		name,
		"-r", rev,
	}
	logCmd("jj", args)
	cmd := exec.Command("jj", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		slog.Debug("jj exec failed", "err", err, "output", strings.TrimSpace(string(out)))
		return fmt.Errorf("jj bookmark set: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	slog.Debug("jj exec ok", "bytes", len(out))
	return nil
}

func (r *realRunner) GitRemoteList() ([]byte, error) {
	args := []string{"git", "remote", "list", "-R", r.repoDir}
	logCmd("jj", args)
	cmd := exec.Command("jj", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		slog.Debug("jj exec failed", "err", err, "output", strings.TrimSpace(string(out)))
		return nil, fmt.Errorf("jj git remote list: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	slog.Debug("jj exec ok", "bytes", len(out))
	return out, nil
}

func (r *realRunner) GitFetch(remote string) error {
	return retry.Do(func() error {
		args := []string{"git", "fetch", "-R", r.repoDir, "--remote", remote}
		logCmd("jj", args)
		cmd := exec.Command("jj", args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			slog.Debug("jj exec failed", "err", err, "output", strings.TrimSpace(string(out)))
			return fmt.Errorf("jj git fetch: %w\n%s", err, strings.TrimSpace(string(out)))
		}
		slog.Debug("jj exec ok", "bytes", len(out))
		return nil
	})
}

func (r *realRunner) GitPush(bookmarks []string, allowNew bool, remote string) error {
	return retry.Do(func() error {
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
		logCmd("jj", args)
		cmd := exec.Command("jj", args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			slog.Debug("jj exec failed", "err", err, "output", strings.TrimSpace(string(out)))
			return fmt.Errorf("jj git push: %w\n%s", err, strings.TrimSpace(string(out)))
		}
		slog.Debug("jj exec ok", "bytes", len(out))
		return nil
	})
}

func (r *realRunner) Interdiff(from, to string) (string, error) {
	args := []string{
		"interdiff", "--git",
		"-R", r.repoDir,
		"--from", from,
		"--to", to,
	}
	logCmd("jj", args)
	cmd := exec.Command("jj", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		slog.Debug("jj exec failed", "err", err, "output", strings.TrimSpace(string(out)))
		return "", fmt.Errorf("jj interdiff: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	slog.Debug("jj exec ok", "bytes", len(out))
	return string(out), nil
}

func (r *realRunner) ConfigGet(key string) (string, error) {
	args := []string{"config", "get", "-R", r.repoDir, key}
	logCmd("jj", args)
	cmd := exec.Command("jj", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		slog.Debug("jj exec failed", "err", err, "stderr", strings.TrimSpace(stderr.String()))
		return "", fmt.Errorf("jj config get %s: %w\n%s", key, err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

func (r *realRunner) Rebase(revsets []string, destination string) error {
	args := []string{"rebase", "-R", r.repoDir, "-d", destination}
	for _, rev := range revsets {
		args = append(args, "-b", rev)
	}
	logCmd("jj", args)
	cmd := exec.Command("jj", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		slog.Debug("jj exec failed", "err", err, "output", strings.TrimSpace(string(out)))
		return fmt.Errorf("jj rebase: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	slog.Debug("jj exec ok", "bytes", len(out))
	return nil
}

// debugEnabled reports whether debug-level logging is active.
func debugEnabled() bool {
	return slog.Default().Handler().Enabled(context.Background(), slog.LevelDebug)
}

// logCmd prints a copy-pasteable shell command to stderr when debug
// logging is enabled. It writes directly to stderr (bypassing slog)
// because slog.TextHandler escapes backslashes and quotes inside
// values, which makes the output uncopyable.
func logCmd(prog string, args []string) {
	if !debugEnabled() {
		return
	}
	var b strings.Builder
	b.WriteString(prog)
	for _, a := range args {
		b.WriteByte(' ')
		if a == "" || strings.ContainsAny(a, " \t\n\"'\\|&;()<>$`!{}*?[]#~") {
			b.WriteByte('\'')
			b.WriteString(strings.ReplaceAll(a, "'", "'\\''"))
			b.WriteByte('\'')
		} else {
			b.WriteString(a)
		}
	}
	fmt.Fprintf(os.Stderr, "DEBUG $ %s\n", b.String())
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
