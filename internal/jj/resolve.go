package jj

import (
	"fmt"
	"slices"
	"strings"
)

// ResolveBaseBranch resolves a jj revset (e.g. "trunk()" or "main") to a
// remote bookmark name suitable for use as a GitHub PR base branch.
//
// A revset that literally names a bookmark ("release" or "release@origin"), or
// a revset alias defined as one (jj git clone sets trunk() to "main@origin"),
// yields that bookmark directly. Otherwise the revset is resolved to a commit
// and matched against bookmarks, preferring preferredRemote, then any remote,
// then local bookmarks. When several bookmarks match, one that is also a local bookmark
// on the commit wins; all matches are returned as candidates so the caller
// can report the choice.
func ResolveBaseBranch(runner Runner, revset string, bookmarks []BookmarkInfo, preferredRemote string) (branch string, candidates []string, err error) {
	if name, ok := literalBookmark(revset, bookmarks); ok {
		return name, nil, nil
	}
	// An unset alias errors; fall through to resolving the revset.
	if alias, err := runner.ConfigGet(`revset-aliases."` + revset + `"`); err == nil {
		if name, ok := literalBookmark(alias, bookmarks); ok {
			return name, nil, nil
		}
	}

	out, err := runner.Log(revset)
	if err != nil {
		return "", nil, fmt.Errorf("resolving base %q: %w", revset, err)
	}
	changes, err := ParseChanges(out)
	if err != nil {
		return "", nil, fmt.Errorf("parsing base %q: %w", revset, err)
	}
	if len(changes) == 0 {
		return "", nil, fmt.Errorf("base %q resolved to no commits", revset)
	}
	if len(changes) > 1 {
		return "", nil, fmt.Errorf("base %q resolved to %d commits, expected 1", revset, len(changes))
	}
	change := changes[0]

	tiers := []func(b BookmarkInfo) bool{
		func(b BookmarkInfo) bool {
			rs, ok := b.Remotes[preferredRemote]
			return ok && rs.Target == change.CommitID
		},
		func(b BookmarkInfo) bool {
			for _, rs := range b.Remotes {
				if rs.Target == change.CommitID {
					return true
				}
			}
			return false
		},
		func(b BookmarkInfo) bool { return b.Present && b.Target == change.CommitID },
	}
	for _, match := range tiers {
		for _, b := range bookmarks {
			if match(b) {
				candidates = append(candidates, b.Name)
			}
		}
		if len(candidates) > 0 {
			break
		}
	}
	if len(candidates) == 0 {
		return "", nil, fmt.Errorf("base %q does not match any bookmark — push one to %s or pass --base", revset, preferredRemote)
	}

	branch = candidates[0]
	for _, c := range candidates {
		if slices.Contains(change.Bookmarks, c) {
			branch = c
			break
		}
	}
	return branch, candidates, nil
}

// literalBookmark reports whether revset is a known bookmark name, optionally
// qualified with a remote ("name@remote"), and returns the bare name.
func literalBookmark(revset string, bookmarks []BookmarkInfo) (string, bool) {
	for _, b := range bookmarks {
		if revset == b.Name {
			return b.Name, true
		}
		for remote := range b.Remotes {
			if revset == b.Name+"@"+remote {
				return b.Name, true
			}
		}
	}
	return "", false
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
