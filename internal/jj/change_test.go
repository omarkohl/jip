package jj

import (
	"testing"
)

// --- Layer 1: Pure DAG logic tests ---

func TestBuildDAGs_LinearStack(t *testing.T) {
	changes := []Change{
		{ChangeID: "a", ParentIDs: []string{"base"}},
		{ChangeID: "b", ParentIDs: []string{"a"}},
		{ChangeID: "c", ParentIDs: []string{"b"}},
	}
	dags := mustBuildDAGs(t, changes)
	if len(dags) != 1 {
		t.Fatalf("expected 1 DAG, got %d", len(dags))
	}
	assertOrder(t, dags[0], []string{"a", "b", "c"})
}

func TestBuildDAGs_DiamondMerge(t *testing.T) {
	// A is root, B and C branch from A, D merges B+C.
	changes := []Change{
		{ChangeID: "a", ParentIDs: []string{"base"}},
		{ChangeID: "b", ParentIDs: []string{"a"}},
		{ChangeID: "c", ParentIDs: []string{"a"}},
		{ChangeID: "d", ParentIDs: []string{"b", "c"}},
	}
	dags := mustBuildDAGs(t, changes)
	if len(dags) != 1 {
		t.Fatalf("expected 1 DAG, got %d", len(dags))
	}
	dag := dags[0]
	if len(dag.Changes) != 4 {
		t.Fatalf("expected 4 changes, got %d", len(dag.Changes))
	}
	// a must come before b, c, and d. d must come after b and c.
	assertBefore(t, dag, "a", "b")
	assertBefore(t, dag, "a", "c")
	assertBefore(t, dag, "b", "d")
	assertBefore(t, dag, "c", "d")
}

func TestBuildDAGs_IndependentBranches(t *testing.T) {
	changes := []Change{
		{ChangeID: "a", ParentIDs: []string{"base1"}},
		{ChangeID: "b", ParentIDs: []string{"a"}},
		{ChangeID: "x", ParentIDs: []string{"base2"}},
		{ChangeID: "y", ParentIDs: []string{"x"}},
	}
	dags := mustBuildDAGs(t, changes)
	if len(dags) != 2 {
		t.Fatalf("expected 2 DAGs, got %d", len(dags))
	}
	// Each DAG should have 2 changes.
	for i, dag := range dags {
		if len(dag.Changes) != 2 {
			t.Errorf("DAG %d: expected 2 changes, got %d", i, len(dag.Changes))
		}
	}
}

func TestBuildDAGs_SingleChange(t *testing.T) {
	changes := []Change{
		{ChangeID: "a", ParentIDs: []string{"base"}},
	}
	dags := mustBuildDAGs(t, changes)
	if len(dags) != 1 {
		t.Fatalf("expected 1 DAG, got %d", len(dags))
	}
	if len(dags[0].Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(dags[0].Changes))
	}
	if dags[0].Changes[0].ChangeID != "a" {
		t.Errorf("expected change 'a', got %q", dags[0].Changes[0].ChangeID)
	}
}

func TestBuildDAGs_Empty(t *testing.T) {
	dags := mustBuildDAGs(t, nil)
	if len(dags) != 0 {
		t.Fatalf("expected 0 DAGs, got %d", len(dags))
	}
}

func TestBuildDAGs_ByIDIndex(t *testing.T) {
	changes := []Change{
		{ChangeID: "a", CommitID: "ca", ParentIDs: []string{"base"}},
		{ChangeID: "b", CommitID: "cb", ParentIDs: []string{"a"}},
	}
	dags := mustBuildDAGs(t, changes)
	dag := dags[0]
	for _, c := range dag.Changes {
		looked, ok := dag.ByID[c.ChangeID]
		if !ok {
			t.Errorf("ByID missing %q", c.ChangeID)
		}
		if looked != c {
			t.Errorf("ByID[%q] points to wrong change", c.ChangeID)
		}
	}
}

func TestBuildDAGs_MultipleRoots(t *testing.T) {
	// Two roots that converge into one merge — should be one DAG.
	changes := []Change{
		{ChangeID: "a", ParentIDs: []string{"base1"}},
		{ChangeID: "b", ParentIDs: []string{"base2"}},
		{ChangeID: "c", ParentIDs: []string{"a", "b"}},
	}
	dags := mustBuildDAGs(t, changes)
	if len(dags) != 1 {
		t.Fatalf("expected 1 DAG, got %d", len(dags))
	}
	assertBefore(t, dags[0], "a", "c")
	assertBefore(t, dags[0], "b", "c")
}

func TestBuildDAGs_DuplicateChangeID(t *testing.T) {
	changes := []Change{
		{ChangeID: "a", ParentIDs: []string{"base"}},
		{ChangeID: "a", ParentIDs: []string{"base"}},
	}
	_, err := BuildDAGs(changes)
	if err == nil {
		t.Fatal("expected error for duplicate change ID")
	}
}

func TestBuildDAGs_Cycle(t *testing.T) {
	changes := []Change{
		{ChangeID: "a", ParentIDs: []string{"b"}},
		{ChangeID: "b", ParentIDs: []string{"a"}},
	}
	_, err := BuildDAGs(changes)
	if err == nil {
		t.Fatal("expected error for cycle")
	}
}

func TestLeafChanges_LinearChain(t *testing.T) {
	changes := []Change{
		{ChangeID: "a", ParentIDs: []string{"base"}},
		{ChangeID: "b", ParentIDs: []string{"a"}},
		{ChangeID: "c", ParentIDs: []string{"b"}},
	}
	dags := mustBuildDAGs(t, changes)
	leaves := dags[0].LeafChanges()
	if len(leaves) != 1 {
		t.Fatalf("expected 1 leaf, got %d", len(leaves))
	}
	if leaves[0].ChangeID != "c" {
		t.Errorf("expected leaf 'c', got %q", leaves[0].ChangeID)
	}
}

func TestLeafChanges_BranchingDAG(t *testing.T) {
	// A is root, B and C branch from A — two leaves.
	changes := []Change{
		{ChangeID: "a", ParentIDs: []string{"base"}},
		{ChangeID: "b", ParentIDs: []string{"a"}},
		{ChangeID: "c", ParentIDs: []string{"a"}},
	}
	dags := mustBuildDAGs(t, changes)
	leaves := dags[0].LeafChanges()
	if len(leaves) != 2 {
		t.Fatalf("expected 2 leaves, got %d", len(leaves))
	}
	ids := map[string]bool{}
	for _, l := range leaves {
		ids[l.ChangeID] = true
	}
	if !ids["b"] || !ids["c"] {
		t.Errorf("expected leaves {b, c}, got %v", ids)
	}
}

func TestLeafChanges_SingleChange(t *testing.T) {
	changes := []Change{
		{ChangeID: "a", ParentIDs: []string{"base"}},
	}
	dags := mustBuildDAGs(t, changes)
	leaves := dags[0].LeafChanges()
	if len(leaves) != 1 {
		t.Fatalf("expected 1 leaf, got %d", len(leaves))
	}
	if leaves[0].ChangeID != "a" {
		t.Errorf("expected leaf 'a', got %q", leaves[0].ChangeID)
	}
}

// --- Layer 2: Parse + build tests ---

func TestParseChanges_Valid(t *testing.T) {
	jsonl := `{"change_id":"aaa","commit_id":"c1","description":"first","parent_ids":["base"],"bookmarks":["feat-1"]}
{"change_id":"bbb","commit_id":"c2","description":"second","parent_ids":["aaa"],"bookmarks":[]}
`
	changes, err := ParseChanges([]byte(jsonl))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(changes))
	}
	if changes[0].ChangeID != "aaa" {
		t.Errorf("expected change_id 'aaa', got %q", changes[0].ChangeID)
	}
	if changes[0].Bookmarks[0] != "feat-1" {
		t.Errorf("expected bookmark 'feat-1', got %q", changes[0].Bookmarks[0])
	}
	if changes[1].Description != "second" {
		t.Errorf("expected description 'second', got %q", changes[1].Description)
	}
}

func TestParseChanges_MalformedJSON(t *testing.T) {
	jsonl := `not json at all`
	_, err := ParseChanges([]byte(jsonl))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestParseChanges_Empty(t *testing.T) {
	changes, err := ParseChanges([]byte(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("expected 0 changes, got %d", len(changes))
	}
}

func TestParseChanges_BlankLines(t *testing.T) {
	jsonl := `
{"change_id":"aaa","commit_id":"c1","description":"first","parent_ids":[],"bookmarks":[]}

{"change_id":"bbb","commit_id":"c2","description":"second","parent_ids":["aaa"],"bookmarks":[]}

`
	changes, err := ParseChanges([]byte(jsonl))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(changes))
	}
}

func TestParseAndBuildDAGs_RoundTrip(t *testing.T) {
	jsonl := `{"change_id":"a","commit_id":"ca","description":"root","parent_ids":["base"],"bookmarks":[]}
{"change_id":"b","commit_id":"cb","description":"middle","parent_ids":["a"],"bookmarks":["my-branch"]}
{"change_id":"c","commit_id":"cc","description":"tip","parent_ids":["b"],"bookmarks":[]}
`
	changes, err := ParseChanges([]byte(jsonl))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dags := mustBuildDAGs(t, changes)
	if len(dags) != 1 {
		t.Fatalf("expected 1 DAG, got %d", len(dags))
	}
	assertOrder(t, dags[0], []string{"a", "b", "c"})
	if dags[0].ByID["b"].Bookmarks[0] != "my-branch" {
		t.Errorf("expected bookmark preserved")
	}
}

// --- Test helpers ---

// mustBuildDAGs calls BuildDAGs and fails the test on error.
func mustBuildDAGs(t *testing.T, changes []Change) []*ChangeDAG {
	t.Helper()
	dags, err := BuildDAGs(changes)
	if err != nil {
		t.Fatalf("BuildDAGs: %v", err)
	}
	return dags
}

// assertOrder checks that the DAG's changes appear in exactly the given order.
func assertOrder(t *testing.T, dag *ChangeDAG, expected []string) {
	t.Helper()
	if len(dag.Changes) != len(expected) {
		t.Fatalf("expected %d changes, got %d", len(expected), len(dag.Changes))
	}
	for i, id := range expected {
		if dag.Changes[i].ChangeID != id {
			t.Errorf("position %d: expected %q, got %q", i, id, dag.Changes[i].ChangeID)
		}
	}
}

// assertBefore checks that change with id 'before' appears before 'after' in the DAG.
func assertBefore(t *testing.T, dag *ChangeDAG, before, after string) {
	t.Helper()
	posB, posA := -1, -1
	for i, c := range dag.Changes {
		if c.ChangeID == before {
			posB = i
		}
		if c.ChangeID == after {
			posA = i
		}
	}
	if posB == -1 {
		t.Fatalf("change %q not found in DAG", before)
	}
	if posA == -1 {
		t.Fatalf("change %q not found in DAG", after)
	}
	if posB >= posA {
		t.Errorf("expected %q (pos %d) before %q (pos %d)", before, posB, after, posA)
	}
}
