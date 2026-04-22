package jj

import (
	"fmt"
	"strings"
)

// ResolveBaseBranch resolves a jj revset (e.g. "trunk()" or "main") to a
// remote bookmark name suitable for use as a GitHub PR base branch. It
// prefers a bookmark on preferredRemote and falls back to any remote, then
// any local bookmark pointing at the same commit.
func ResolveBaseBranch(runner Runner, revset string, bookmarks []BookmarkInfo, preferredRemote string) (string, error) {
	out, err := runner.Log(revset)
	if err != nil {
		return "", fmt.Errorf("resolving base %q: %w", revset, err)
	}
	changes, err := ParseChanges(out)
	if err != nil {
		return "", fmt.Errorf("parsing base %q: %w", revset, err)
	}
	if len(changes) == 0 {
		return "", fmt.Errorf("base %q resolved to no commits", revset)
	}
	if len(changes) > 1 {
		return "", fmt.Errorf("base %q resolved to %d commits, expected 1", revset, len(changes))
	}
	commitID := changes[0].CommitID

	for _, b := range bookmarks {
		if rs, ok := b.Remotes[preferredRemote]; ok && rs.Target == commitID {
			return b.Name, nil
		}
	}
	for _, b := range bookmarks {
		for _, rs := range b.Remotes {
			if rs.Target == commitID {
				return b.Name, nil
			}
		}
	}
	for _, b := range bookmarks {
		if b.Present && b.Target == commitID {
			return b.Name, nil
		}
	}
	return "", fmt.Errorf("base %q does not match any bookmark — push one to %s or pass --base", revset, preferredRemote)
}

// ResolveStacks resolves one or more revsets against a base branch and returns
// the changes organized into connected DAGs. Each DAG represents an independent
// stack of changes between the base and the given revsets.
func ResolveStacks(runner Runner, revsets []string, base string) ([]*ChangeDAG, error) {
	if len(revsets) == 0 {
		return nil, fmt.Errorf("no revsets provided")
	}
	if base == "" {
		return nil, fmt.Errorf("no base revset provided")
	}

	// Build combined revset: base..(rev1 | rev2 | ...)
	heads := strings.Join(revsets, " | ")
	revset := fmt.Sprintf("(%s)..(%s)", base, heads)

	out, err := runner.Log(revset)
	if err != nil {
		return nil, err
	}

	changes, err := ParseChanges(out)
	if err != nil {
		return nil, err
	}

	return BuildDAGs(changes)
}
