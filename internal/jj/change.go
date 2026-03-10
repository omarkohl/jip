package jj

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Change represents a single jj change in a stack.
type Change struct {
	ChangeID    string   `json:"change_id"`
	CommitID    string   `json:"commit_id"`
	Description string   `json:"description"`
	Conflict    bool     `json:"conflict"`
	ParentIDs   []string `json:"parent_ids"`
	Bookmarks   []string `json:"bookmarks"`
}

// Title returns the first line of the description (the commit subject).
func (c *Change) Title() string {
	if i := strings.Index(c.Description, "\n"); i >= 0 {
		return c.Description[:i]
	}
	return c.Description
}

// Body returns everything after the first blank line separator in the
// description, trimmed of leading/trailing whitespace. Returns "" if there
// is no body.
func (c *Change) Body() string {
	// Convention: title, blank line, body.
	idx := strings.Index(c.Description, "\n\n")
	if idx < 0 {
		return ""
	}
	return strings.TrimSpace(c.Description[idx+2:])
}

// ChangeDAG is a connected DAG of changes. Changes are topologically sorted
// with roots (closest to base) first.
type ChangeDAG struct {
	Changes []*Change
	ByID    map[string]*Change
}

// ParseChanges parses JSONL output from jj log into a slice of Changes.
func ParseChanges(data []byte) ([]Change, error) {
	var changes []Change
	dec := json.NewDecoder(bytes.NewReader(data))
	for dec.More() {
		var c Change
		if err := dec.Decode(&c); err != nil {
			return nil, fmt.Errorf("parsing change: %w", err)
		}
		// jj's description template includes a trailing newline; trim it.
		c.Description = strings.TrimRight(c.Description, "\n")
		changes = append(changes, c)
	}
	return changes, nil
}

// BuildDAGs splits a flat list of changes into connected components and
// returns each as a topologically sorted ChangeDAG.
// Parent IDs that don't appear in the input are ignored (they reference
// changes outside the resolved range, e.g. the base branch).
// The returned DAGs reference the original Change values in the input slice;
// the caller must not modify the input after calling BuildDAGs.
func BuildDAGs(changes []Change) ([]*ChangeDAG, error) {
	if len(changes) == 0 {
		return nil, nil
	}

	// Index all known change IDs.
	known := make(map[string]int, len(changes)) // change_id -> index
	for i := range changes {
		if _, exists := known[changes[i].ChangeID]; exists {
			return nil, fmt.Errorf("duplicate change ID %q", changes[i].ChangeID)
		}
		known[changes[i].ChangeID] = i
	}

	// Build undirected adjacency for connected component detection.
	// We use union-find.
	parent := make([]int, len(changes))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[ra] = rb
		}
	}

	// Union changes that share a parent-child edge within the known set.
	for i := range changes {
		for _, pid := range changes[i].ParentIDs {
			if j, ok := known[pid]; ok {
				union(i, j)
			}
		}
	}

	// Group by connected component.
	components := make(map[int][]int) // root index -> member indices
	for i := range changes {
		r := find(i)
		components[r] = append(components[r], i)
	}

	// For deterministic output, sort component roots.
	roots := make([]int, 0, len(components))
	for r := range components {
		roots = append(roots, r)
	}
	sort.Ints(roots)

	// Build a ChangeDAG per component with topological sort.
	dags := make([]*ChangeDAG, 0, len(components))
	for _, r := range roots {
		members := components[r]
		dag, err := topoSort(changes, members, known)
		if err != nil {
			return nil, err
		}
		dags = append(dags, dag)
	}
	return dags, nil
}

// LeafChanges returns changes that have no children within this DAG (the "tips").
func (dag *ChangeDAG) LeafChanges() []*Change {
	hasChild := make(map[string]bool)
	for _, c := range dag.Changes {
		for _, pid := range c.ParentIDs {
			if _, ok := dag.ByID[pid]; ok {
				hasChild[pid] = true
			}
		}
	}
	var leaves []*Change
	for _, c := range dag.Changes {
		if !hasChild[c.ChangeID] {
			leaves = append(leaves, c)
		}
	}
	return leaves
}

// FindPrivateChanges queries jj for the git.private-commits config and
// returns the set of change IDs from the given DAGs that match the configured
// private revset. Returns an empty set if git.private-commits is not configured.
func FindPrivateChanges(runner Runner, dags []*ChangeDAG) (map[string]bool, error) {
	privateRevset, err := runner.ConfigGet("git.private-commits")
	if err != nil || privateRevset == "" {
		return nil, nil // not configured → no private commits
	}

	// Build a revset of all change IDs across all DAGs.
	var ids []string
	for _, dag := range dags {
		for _, c := range dag.Changes {
			ids = append(ids, c.ChangeID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}

	combined := "(" + strings.Join(ids, " | ") + ") & (" + privateRevset + ")"
	data, err := runner.Log(combined)
	if err != nil {
		return nil, fmt.Errorf("evaluating private commits: %w", err)
	}
	changes, err := ParseChanges(data)
	if err != nil {
		return nil, fmt.Errorf("parsing private commits: %w", err)
	}

	result := make(map[string]bool, len(changes))
	for _, c := range changes {
		result[c.ChangeID] = true
	}
	return result, nil
}

// FilterDAG returns a new ChangeDAG excluding changes whose IDs are keys in skip.
// Returns nil if all changes are skipped.
func FilterDAG[T any](dag *ChangeDAG, skip map[string]T) *ChangeDAG {
	var filtered []*Change
	byID := make(map[string]*Change)
	for _, c := range dag.Changes {
		if _, skipped := skip[c.ChangeID]; !skipped {
			filtered = append(filtered, c)
			byID[c.ChangeID] = c
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return &ChangeDAG{Changes: filtered, ByID: byID}
}

// topoSort performs Kahn's algorithm on the subset of changes identified
// by memberIndices, returning a ChangeDAG with changes ordered roots-first.
func topoSort(all []Change, memberIndices []int, known map[string]int) (*ChangeDAG, error) {
	// Build local index for this component.
	inComponent := make(map[string]bool, len(memberIndices))
	for _, i := range memberIndices {
		inComponent[all[i].ChangeID] = true
	}

	// Compute in-degree (number of parents within the component).
	inDegree := make(map[string]int, len(memberIndices))
	children := make(map[string][]string) // parent -> children within component
	for _, i := range memberIndices {
		c := &all[i]
		inDegree[c.ChangeID] = 0 // ensure entry exists
		for _, pid := range c.ParentIDs {
			if inComponent[pid] {
				inDegree[c.ChangeID]++
				children[pid] = append(children[pid], c.ChangeID)
			}
		}
	}

	// Kahn's algorithm.
	// Seed with zero in-degree nodes, sorted for determinism.
	var queue []string
	for _, i := range memberIndices {
		if inDegree[all[i].ChangeID] == 0 {
			queue = append(queue, all[i].ChangeID)
		}
	}
	sort.Strings(queue)

	sorted := make([]*Change, 0, len(memberIndices))
	byID := make(map[string]*Change, len(memberIndices))
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		idx := known[id]
		ch := &all[idx]
		sorted = append(sorted, ch)
		byID[id] = ch

		// Sort children for deterministic processing.
		kids := children[id]
		sort.Strings(kids)
		for _, kid := range kids {
			inDegree[kid]--
			if inDegree[kid] == 0 {
				queue = append(queue, kid)
			}
		}
	}

	if len(sorted) != len(memberIndices) {
		return nil, fmt.Errorf("cycle detected in change graph (%d of %d changes sorted)", len(sorted), len(memberIndices))
	}

	return &ChangeDAG{
		Changes: sorted,
		ByID:    byID,
	}, nil
}
