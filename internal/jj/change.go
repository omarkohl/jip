package jj

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// Change represents a single jj change in a stack.
type Change struct {
	ChangeID    string   `json:"change_id"`
	CommitID    string   `json:"commit_id"`
	Description string   `json:"description"`
	ParentIDs   []string `json:"parent_ids"`
	Bookmarks   []string `json:"bookmarks"`
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
