//go:build integration

package cmd

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/omarkohl/jip/internal/jj"
)

func TestIntegration_SendCustomBookmarkStacks(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "tip", true: "explicit"}[explicit], func(t *testing.T) {
			checkJJ(t)
			repo, _ := initTestRepoWithRemote(t)
			mock := newMockService()
			runner := jj.NewRunner(repo)
			writeAndCommit(t, repo, "a.go", "package a", "feat: model")
			jjRun(t, repo, "bookmark", "set", "feature/TASK-1-model", "-r", "@-")
			writeAndCommit(t, repo, "b.go", "package b", "feat: API")
			jjRun(t, repo, "bookmark", "set", "feature/TASK-1-api", "-r", "@-")
			revs := []string{"@-"}
			if explicit {
				jjRun(t, repo, "new", "main")
				writeAndCommit(t, repo, "c.go", "package c", "feat: independent")
				jjRun(t, repo, "bookmark", "set", "jip/manual", "-r", "@-")
				revs = []string{"feature/TASK-1-api", "jip/manual", "feature/TASK-1-model"}
			}
			var out bytes.Buffer
			if err := executeSend(runner, mock, sendOpts{base: "main", remote: "origin", revsets: revs, bookmarkPolicy: "require"}, &out); err != nil {
				t.Fatalf("send: %v\n%s", err, out.String())
			}
			want := map[string]bool{"feature/TASK-1-model": true, "feature/TASK-1-api": true}
			if explicit {
				want["jip/manual"] = true
			}
			if len(mock.prs) != len(want) {
				t.Fatalf("PRs = %d, want %d", len(mock.prs), len(want))
			}
			for _, pr := range mock.prs {
				if !want[pr.HeadRefName] {
					t.Errorf("unexpected branch %q", pr.HeadRefName)
				}
			}
		})
	}
}

func TestIntegration_SendPreflightAllAssignments(t *testing.T) {
	checkJJ(t)
	repo, _ := initTestRepoWithRemote(t)
	runner := jj.NewRunner(repo)
	writeAndCommit(t, repo, "a.go", "package a", "feat: missing one")
	a := getChangeID(t, repo, "@-")
	writeAndCommit(t, repo, "b.go", "package b", "feat: missing two")
	jjRun(t, repo, "new", "main")
	writeAndCommit(t, repo, "c.go", "package c", "feat: ambiguous")
	jjRun(t, repo, "bookmark", "set", "custom-a", "custom-b", "-r", "@-")
	for _, policy := range []string{"auto", "require"} {
		mock := newMockService()
		before, _ := runner.BookmarkList()
		var out bytes.Buffer
		err := executeSend(runner, mock, sendOpts{base: "main", remote: "origin", revsets: []string{a + "+", "@-"}, bookmarkPolicy: policy, rebase: true}, &out)
		if err == nil || !strings.Contains(err.Error(), "ambiguous bookmarks: custom-a, custom-b") {
			t.Fatalf("expected ambiguity error, got %v", err)
		}
		if policy == "require" && (!strings.Contains(err.Error(), "missing one") || !strings.Contains(err.Error(), "missing two")) {
			t.Errorf("missing aggregated errors: %v", err)
		}
		after, _ := runner.BookmarkList()
		if !bytes.Equal(before, after) || len(mock.prs) != 0 || strings.Contains(out.String(), "Rebasing") {
			t.Fatalf("preflight made changes: %s", out.String())
		}
	}
}

func TestIntegration_SendBookmarkExclusions(t *testing.T) {
	checkJJ(t)
	repo, _ := initTestRepoWithRemote(t)
	runner := jj.NewRunner(repo)
	writeAndCommit(t, repo, "a.go", "package a", "feat: feature")
	jjRun(t, repo, "bookmark", "set", "release/stable", "feature/task", "-r", "@-")
	// A name predicate must not exclude other bookmarks on the same commit.
	expression := `normal_target.immutable() || name.starts_with("release/")`
	var out bytes.Buffer
	if err := executeSend(runner, newMockService(), sendOpts{base: "main", remote: "origin", revsets: []string{"@-"}, excludeBookmarks: expression, dryRun: true}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `head="feature/task"`) {
		t.Fatal(out.String())
	}
	for _, rev := range []string{"release/stable"} {
		err := executeSend(runner, newMockService(), sendOpts{base: "main", remote: "origin", revsets: []string{rev}, excludeBookmarks: expression}, &out)
		if err == nil || !strings.Contains(err.Error(), "is excluded") {
			t.Fatalf("expected exclusion error, got %v", err)
		}
	}
	// The default follows the user's jj immutable_heads() alias.
	jjRun(t, repo, "config", "set", "--repo", `revset-aliases."immutable_heads()"`, `trunk() | bookmarks("release/*")`)
	err := executeSend(runner, newMockService(), sendOpts{base: "main", remote: "origin", revsets: []string{"@-"}, bookmarkPolicy: "require"}, &out)
	if err == nil || !strings.Contains(err.Error(), "no eligible bookmark") {
		t.Fatalf("expected immutable exclusion, got %v", err)
	}
	err = executeSend(runner, newMockService(), sendOpts{base: "main", remote: "origin", revsets: []string{"@-"}, excludeBookmarks: "not_a_template()"}, &out)
	if err == nil || !strings.Contains(err.Error(), "exclude-bookmarks") {
		t.Fatalf("expected template error, got %v", err)
	}
}

func TestIntegration_SendMultipleExplicitBookmarksRejected(t *testing.T) {
	checkJJ(t)
	repo, _ := initTestRepoWithRemote(t)
	writeAndCommit(t, repo, "a.go", "package a", "feat: feature")
	jjRun(t, repo, "bookmark", "set", "alpha", "beta", "-r", "@-")
	mock := newMockService()
	var out bytes.Buffer
	err := executeSend(jj.NewRunner(repo), mock, sendOpts{base: "main", remote: "origin", revsets: []string{"beta", "alpha"}}, &out)
	if err == nil || !strings.Contains(err.Error(), "ambiguous bookmarks") || len(mock.prs) != 0 {
		t.Fatalf("expected ambiguity, got %v", err)
	}
}

func TestIntegration_SendAmbiguousPRsRequireChoice(t *testing.T) {
	checkJJ(t)
	repo, _ := initTestRepoWithRemote(t)
	writeAndCommit(t, repo, "a.go", "package a", "feat: feature")
	mock := newMockService()
	for _, name := range []string{"alpha", "beta"} {
		jjRun(t, repo, "bookmark", "set", name, "-r", "@-")
		jjRun(t, repo, "git", "push", "-b", name)
		if _, err := mock.CreatePR(name, "main", "old", "untouched", false); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	err := executeSend(jj.NewRunner(repo), mock, sendOpts{base: "main", remote: "origin", revsets: []string{"@-"}}, &out)
	if err == nil || !strings.Contains(err.Error(), "ambiguous bookmarks") {
		t.Fatalf("expected ambiguity, got %v", err)
	}
	for _, pr := range mock.prs {
		if pr.Body != "untouched" {
			t.Fatal("PR changed")
		}
	}
}

func TestIntegration_SendDryRunNativeStack(t *testing.T) {
	checkJJ(t)
	repo, _ := initTestRepoWithRemote(t)
	runner := jj.NewRunner(repo)
	mock := newMockService()
	mock.stacksEnabled = true
	writeAndCommit(t, repo, "a.go", "package a", "feat: bottom")
	writeAndCommit(t, repo, "b.go", "package b", "feat: top")
	var out bytes.Buffer
	opts := sendOpts{base: "main", remote: "origin", revsets: []string{"@-"}, stackMode: stackModeNative}
	if err := executeSend(runner, mock, opts, &out); err != nil {
		t.Fatal(err)
	}
	before := *mock.stacks[1]
	before.PullRequests = append(before.PullRequests[:0:0], before.PullRequests...)
	bottomBefore, topBefore := *mock.prs[1], *mock.prs[2]
	// A changed base requires dissolution and recreation during a real send.
	jjRun(t, repo, "bookmark", "set", "release", "-r", "main")
	opts.base, opts.dryRun = "release", true
	out.Reset()
	if err := executeSend(runner, mock, opts, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"DISSOLVE GitHub stack #1", "base: release", "CREATE GitHub stack #-1"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q:\n%s", want, out.String())
		}
	}
	if !reflect.DeepEqual(*mock.stacks[1], before) || *mock.prs[1] != bottomBefore || *mock.prs[2] != topBefore || mock.unstackCalls != 0 || mock.createStackCalls != 1 {
		t.Fatal("dry-run mutated the native stack")
	}
}

func TestIntegration_SendDryRunActionsWithoutMutations(t *testing.T) {
	checkJJ(t)
	repo, _ := initTestRepoWithRemote(t)
	runner := jj.NewRunner(repo)
	mock := newMockService()
	writeAndCommit(t, repo, "a.go", "package a", "feat: initial")
	var out bytes.Buffer
	opts := sendOpts{base: "main", remote: "origin", revsets: []string{"@-"}, dryRun: true, draft: true, reviewers: []string{"alice"}}
	before, _ := runner.BookmarkList()
	if err := executeSend(runner, mock, opts, &out); err != nil {
		t.Fatal(err)
	}
	after, _ := runner.BookmarkList()
	if !bytes.Equal(before, after) || len(mock.prs) != 0 {
		t.Fatal("dry-run changed bookmarks or PRs")
	}
	for _, want := range []string{"CREATE bookmark \"jip/initial/", "PUSH bookmark", "CREATE PR #-1 (placeholder)", "draft=true", "title: feat: initial", "body:", "REQUEST reviewers", "alice", "jip:pushed-commit="} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q:\n%s", want, out.String())
		}
	}
	// Create the PR, then preview title/body/comment updates without changing it.
	opts.dryRun = false
	if err := executeSend(runner, mock, opts, &out); err != nil {
		t.Fatal(err)
	}
	id := getChangeID(t, repo, "@-")
	editFile(t, repo, id, "a.go", "package a\nconst Version = 2\n")
	jjRun(t, repo, "describe", "-r", id, "-m", "feat: updated\n\nNew body.")
	original := *mock.prs[1]
	before, _ = runner.BookmarkList()
	opts.dryRun = true
	out.Reset()
	if err := executeSend(runner, mock, opts, &out); err != nil {
		t.Fatal(err)
	}
	after, _ = runner.BookmarkList()
	if !bytes.Equal(before, after) || *mock.prs[1] != original || len(mock.comments[1]) != 0 {
		t.Fatal("preview modified existing state")
	}
	for _, want := range []string{"UPDATE PR #1", "title: feat: updated", "New body.", "COMMENT on PR #1", "Version = 2"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q:\n%s", want, out.String())
		}
	}
	opts.rebase = true
	if err := executeSend(runner, mock, opts, &out); err == nil {
		t.Fatal("dry-run must not perform a rebase")
	}
}

func TestIntegration_SendRebasePreservesSelection(t *testing.T) {
	checkJJ(t)
	for _, selection := range []string{"commit", "remote bookmark"} {
		t.Run(selection, func(t *testing.T) {
			repo, _ := initTestRepoWithRemote(t)
			writeAndCommit(t, repo, "a.go", "package a", "feat: selected")
			id := getChangeID(t, repo, "@-")
			rev := getCommitID(t, repo, id)
			if selection == "remote bookmark" {
				jjRun(t, repo, "bookmark", "set", "feature", "other", "-r", id)
				jjRun(t, repo, "git", "push", "-b", "feature")
				rev = "feature@origin"
			}
			jjRun(t, repo, "new", "main")
			writeAndCommit(t, repo, "base.go", "package base", "feat: advance base")
			jjRun(t, repo, "bookmark", "set", "main", "-r", "@-")
			jjRun(t, repo, "git", "push", "-b", "main")
			jjRun(t, repo, "new", id)
			spy := &spyRunner{Runner: jj.NewRunner(repo)}
			mock := newMockService()
			var out bytes.Buffer
			if err := executeSend(spy, mock, sendOpts{base: "main", remote: "origin", revsets: []string{rev}, rebase: true}, &out); err != nil {
				t.Fatalf("send: %v\n%s", err, out.String())
			}
			if len(mock.prs) != 1 || mock.prs[1].Title != "feat: selected" {
				t.Fatalf("selected change was not sent: %+v\n%s", mock.prs, out.String())
			}
			rebasedCommit := getCommitID(t, repo, id)
			if !strings.Contains(mock.prs[1].Body, "jip:pushed-commit="+rebasedCommit) {
				t.Fatalf("sent the old commit instead of %s: %s", rebasedCommit, mock.prs[1].Body)
			}
			if len(spy.fetchRemotes) != 1 {
				t.Fatalf("fetched %d times, want once", len(spy.fetchRemotes))
			}
		})
	}
}

func TestIntegration_SendExistingSkipsExcludedGeneratedBookmark(t *testing.T) {
	checkJJ(t)
	repo, _ := initTestRepoWithRemote(t)
	writeAndCommit(t, repo, "a.go", "package a", "feat: feature")
	id := getChangeID(t, repo, "@-")
	name := jj.GenerateBookmarkName("feat: feature", id[:8])
	jjRun(t, repo, "bookmark", "set", name, "-r", "@-")
	for _, expression := range []string{"false", `name.starts_with("jip/")`} {
		t.Run(expression, func(t *testing.T) {
			var out bytes.Buffer
			mock := newMockService()
			if err := executeSend(jj.NewRunner(repo), mock, sendOpts{
				base: "main", remote: "origin", revsets: []string{"@-"}, existing: true,
				excludeBookmarks: expression,
			}, &out); err != nil {
				t.Fatalf("changes without PRs must be skipped: %v", err)
			}
			if len(mock.prs) != 0 || !strings.Contains(out.String(), "No existing PRs") {
				t.Fatalf("unexpected result: %s", out.String())
			}
		})
	}
}
