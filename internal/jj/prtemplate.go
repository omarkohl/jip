package jj

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path"
	"slices"
	"strings"
)

// PRTemplate reads a single-file PR template from a committed revision.
// Missing templates are allowed; revision and file read failures are not.
func (r *realRunner) PRTemplate(revision string) (string, error) {
	args := []string{"file", "list", "-R", r.repoDir, "-r", revision, "-T", `json(path) ++ "\n"`}
	logCmd("jj", args)
	cmd := exec.Command("jj", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("listing PR templates at %s: %w\n%s", revision, err, strings.TrimSpace(stderr.String()))
	}
	var paths []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		var name string
		if err := json.Unmarshal([]byte(line), &name); err != nil {
			return "", fmt.Errorf("parsing template path: %w", err)
		}
		paths = append(paths, name)
	}
	slices.Sort(paths)
	for _, dir := range []string{".github", ".", "docs"} {
		var matches []string
		for _, name := range paths {
			if path.Dir(name) == dir && strings.EqualFold(path.Base(name), "pull_request_template.md") {
				matches = append(matches, name)
			}
		}
		if len(matches) > 1 {
			return "", fmt.Errorf("multiple PR templates in %s at %s: %s", dir, revision, strings.Join(matches, ", "))
		}
		if len(matches) == 0 {
			continue
		}
		args = []string{"file", "show", "-R", r.repoDir, "-r", revision, "--", matches[0]}
		logCmd("jj", args)
		cmd = exec.Command("jj", args...)
		cmd.Dir = r.repoDir
		stderr.Reset()
		cmd.Stderr = &stderr
		out, err = cmd.Output()
		if err != nil {
			return "", fmt.Errorf("reading PR template %s: %w\n%s", matches[0], err, strings.TrimSpace(stderr.String()))
		}
		return string(out), nil
	}
	return "", nil
}
