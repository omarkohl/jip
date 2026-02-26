package jj

import (
	"fmt"
	"strings"
)

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
