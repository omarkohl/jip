//go:build integration

package jj

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// initJJRepoWithRemote creates a jj repo backed by a bare git remote.
// Returns (repoDir, remoteDir). The repo has an initial commit pushed
// to "origin" with a "main" bookmark.
func initJJRepoWithRemote(t *testing.T) (string, string) {
	t.Helper()
	checkJJ(t)

	remoteDir, err := os.MkdirTemp("", "jip-remote-*")
	if err != nil {
		t.Fatalf("creating remote dir: %v", err)
	}

	repoDir, err := os.MkdirTemp("", "jip-integration-*")
	if err != nil {
		t.Fatalf("creating repo dir: %v", err)
	}

	if os.Getenv("JIP_KEEP_REPO") != "" {
		t.Logf("JIP_KEEP_REPO set — repo preserved at: %s", repoDir)
		t.Logf("JIP_KEEP_REPO set — remote preserved at: %s", remoteDir)
	} else {
		t.Cleanup(func() {
			os.RemoveAll(repoDir)
			os.RemoveAll(remoteDir)
		})
	}

	// Create bare git remote.
	gitCmd := exec.Command("git", "init", "--bare", remoteDir)
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}

	// Init jj repo.
	cmd := exec.Command("jj", "git", "init", repoDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("jj git init: %v\n%s", err, out)
	}

	// Configure user for commits.
	jjRun(t, repoDir, "config", "set", "--repo", "user.email", "test@jip.dev")
	jjRun(t, repoDir, "config", "set", "--repo", "user.name", "Test User")

	// Add remote.
	jjRun(t, repoDir, "git", "remote", "add", "origin", remoteDir)

	// Initial commit and push.
	writeAndCommit(t, repoDir, "README.md", "# test repo", "initial commit")
	jjRun(t, repoDir, "bookmark", "set", "main", "-r", "@-")
	jjRun(t, repoDir, "git", "push", "--bookmark", "main")

	t.Logf("repo: %s, remote: %s", repoDir, remoteDir)
	return repoDir, remoteDir
}

// cloneJJRepo creates a second jj clone of the bare remote.
func cloneJJRepo(t *testing.T, remoteDir string) string {
	t.Helper()
	checkJJ(t)

	cloneDir, err := os.MkdirTemp("", "jip-clone-*")
	if err != nil {
		t.Fatalf("creating clone dir: %v", err)
	}
	if os.Getenv("JIP_KEEP_REPO") == "" {
		t.Cleanup(func() { os.RemoveAll(cloneDir) })
	}

	cmd := exec.Command("jj", "git", "clone", remoteDir, cloneDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("jj git clone: %v\n%s", err, out)
	}

	jjRun(t, cloneDir, "config", "set", "--repo", "user.email", "clone@jip.dev")
	jjRun(t, cloneDir, "config", "set", "--repo", "user.name", "Clone User")

	t.Logf("clone: %s", cloneDir)
	return cloneDir
}

func TestIntegration_BookmarkListParsing(t *testing.T) {
	repoDir, _ := initJJRepoWithRemote(t)
	runner := NewRunner(repoDir)

	// Create a bookmark and push it.
	writeAndCommit(t, repoDir, "a.txt", "aaa", "feat: add feature")
	jjRun(t, repoDir, "bookmark", "set", "feat-branch", "-r", "@-")
	jjRun(t, repoDir, "git", "push", "--bookmark", "feat-branch")

	logRepoState(t, repoDir)

	// Parse bookmark list.
	data, err := runner.BookmarkList()
	if err != nil {
		t.Fatalf("BookmarkList: %v", err)
	}
	t.Logf("raw bookmark list:\n%s", string(data))

	bookmarks, err := ParseBookmarkList(data)
	if err != nil {
		t.Fatalf("ParseBookmarkList: %v", err)
	}

	// Should have main and feat-branch (git remote filtered out).
	names := map[string]bool{}
	for _, b := range bookmarks {
		names[b.Name] = true
		t.Logf("bookmark: %s present=%v target=%.12s remotes=%v",
			b.Name, b.Present, b.Target, b.Remotes)
	}

	if !names["main"] {
		t.Error("expected 'main' bookmark")
	}
	if !names["feat-branch"] {
		t.Error("expected 'feat-branch' bookmark")
	}

	// feat-branch should be in-sync with origin (just pushed).
	for _, b := range bookmarks {
		if b.Name == "feat-branch" {
			if s := b.SyncWith("origin"); s != SyncInSync {
				t.Errorf("feat-branch sync: expected SyncInSync, got %v", s)
			}
			if _, ok := b.Remotes["git"]; ok {
				t.Error("git remote should be filtered out")
			}
		}
	}
}

func TestIntegration_BookmarkSyncInSync(t *testing.T) {
	repoDir, _ := initJJRepoWithRemote(t)
	runner := NewRunner(repoDir)

	writeAndCommit(t, repoDir, "a.txt", "aaa", "feat: synced feature")
	jjRun(t, repoDir, "bookmark", "set", "synced", "-r", "@-")
	jjRun(t, repoDir, "git", "push", "--bookmark", "synced")

	data, err := runner.BookmarkList()
	if err != nil {
		t.Fatalf("BookmarkList: %v", err)
	}
	bookmarks, err := ParseBookmarkList(data)
	if err != nil {
		t.Fatalf("ParseBookmarkList: %v", err)
	}

	for _, b := range bookmarks {
		if b.Name == "synced" {
			if s := b.SyncWith("origin"); s != SyncInSync {
				t.Errorf("expected SyncInSync, got %v", s)
			}
			return
		}
	}
	t.Fatal("bookmark 'synced' not found")
}

func TestIntegration_BookmarkSyncAhead(t *testing.T) {
	repoDir, _ := initJJRepoWithRemote(t)
	runner := NewRunner(repoDir)

	// Push, then add a new commit on top — local is strictly ahead of origin.
	writeAndCommit(t, repoDir, "a.txt", "v1", "feat: initial")
	jjRun(t, repoDir, "bookmark", "set", "ahead-branch", "-r", "@-")
	jjRun(t, repoDir, "git", "push", "--bookmark", "ahead-branch")

	// Add another commit and move the bookmark forward.
	writeAndCommit(t, repoDir, "b.txt", "v2", "feat: follow-up")
	jjRun(t, repoDir, "bookmark", "set", "ahead-branch", "-r", "@-")

	logRepoState(t, repoDir)

	data, err := runner.BookmarkList()
	if err != nil {
		t.Fatalf("BookmarkList: %v", err)
	}
	bookmarks, err := ParseBookmarkList(data)
	if err != nil {
		t.Fatalf("ParseBookmarkList: %v", err)
	}

	for _, b := range bookmarks {
		if b.Name == "ahead-branch" {
			s := b.SyncWith("origin")
			t.Logf("ahead-branch: sync=%v, remotes=%+v", s, b.Remotes)
			if s != SyncAhead {
				t.Errorf("expected SyncAhead, got %v", s)
			}
			return
		}
	}
	t.Fatal("bookmark 'ahead-branch' not found")
}

func TestIntegration_BookmarkSyncBehind(t *testing.T) {
	repoDir, _ := initJJRepoWithRemote(t)
	runner := NewRunner(repoDir)

	// Create a stack: A → B, push the bookmark at B.
	writeAndCommit(t, repoDir, "a.txt", "v1", "feat: behind base")
	idA := getChangeID(t, repoDir, "@-")
	writeAndCommit(t, repoDir, "b.txt", "v2", "feat: behind tip")
	jjRun(t, repoDir, "bookmark", "set", "behind-branch", "-r", "@-")
	jjRun(t, repoDir, "git", "push", "--bookmark", "behind-branch")

	// Move the local bookmark backward to A — now local is behind remote.
	jjRun(t, repoDir, "bookmark", "set", "--allow-backwards", "behind-branch", "-r", idA)

	logRepoState(t, repoDir)

	data, err := runner.BookmarkList()
	if err != nil {
		t.Fatalf("BookmarkList: %v", err)
	}
	bookmarks, err := ParseBookmarkList(data)
	if err != nil {
		t.Fatalf("ParseBookmarkList: %v", err)
	}

	for _, b := range bookmarks {
		if b.Name == "behind-branch" {
			s := b.SyncWith("origin")
			t.Logf("behind-branch: sync=%v, remotes=%+v", s, b.Remotes)
			if s != SyncBehind {
				t.Errorf("expected SyncBehind, got %v", s)
			}
			return
		}
	}
	t.Fatal("bookmark 'behind-branch' not found")
}

func TestIntegration_BookmarkSyncDiverged(t *testing.T) {
	repoDir, remoteDir := initJJRepoWithRemote(t)
	runner := NewRunner(repoDir)

	// Push a bookmark.
	writeAndCommit(t, repoDir, "a.txt", "v1", "feat: diverge test")
	changeID := getChangeID(t, repoDir, "@-")
	jjRun(t, repoDir, "bookmark", "set", "diverged-branch", "-r", "@-")
	jjRun(t, repoDir, "git", "push", "--bookmark", "diverged-branch")

	// Advance from clone and push.
	cloneDir := cloneJJRepo(t, remoteDir)
	jjRun(t, cloneDir, "bookmark", "track", "diverged-branch@origin")
	jjRun(t, cloneDir, "new", "diverged-branch@origin")
	writeAndCommit(t, cloneDir, "b.txt", "clone-v2", "feat: clone advance")
	jjRun(t, cloneDir, "bookmark", "set", "diverged-branch", "-r", "@-")
	jjRun(t, cloneDir, "git", "push", "--bookmark", "diverged-branch")

	// Also advance locally by adding a commit and moving the bookmark.
	jjRun(t, repoDir, "new", changeID)
	writeAndCommit(t, repoDir, "c.txt", "local-v2", "feat: local advance")
	jjRun(t, repoDir, "bookmark", "set", "diverged-branch", "-r", "@-")

	// Fetch to see the remote update.
	jjRun(t, repoDir, "git", "fetch")

	logRepoState(t, repoDir)

	data, err := runner.BookmarkList()
	if err != nil {
		t.Fatalf("BookmarkList: %v", err)
	}
	bookmarks, err := ParseBookmarkList(data)
	if err != nil {
		t.Fatalf("ParseBookmarkList: %v", err)
	}

	for _, b := range bookmarks {
		if b.Name == "diverged-branch" {
			s := b.SyncWith("origin")
			t.Logf("diverged-branch: sync=%v, remotes=%+v", s, b.Remotes)
			if s != SyncDiverged {
				t.Errorf("expected SyncDiverged, got %v", s)
			}
			return
		}
	}
	t.Fatal("bookmark 'diverged-branch' not found")
}

func TestIntegration_BookmarkCreation(t *testing.T) {
	repoDir, _ := initJJRepoWithRemote(t)
	runner := NewRunner(repoDir)

	// Create changes and resolve a DAG.
	writeAndCommit(t, repoDir, "a.txt", "aaa", "feat: add auth")
	writeAndCommit(t, repoDir, "b.txt", "bbb", "fix: handle error")

	logRepoState(t, repoDir)

	dags, err := ResolveStacks(runner, []string{"@-"}, "main")
	if err != nil {
		t.Fatalf("ResolveStacks: %v", err)
	}
	if len(dags) != 1 {
		t.Fatalf("expected 1 DAG, got %d", len(dags))
	}

	// No bookmarks on these changes yet.
	data, err := runner.BookmarkList()
	if err != nil {
		t.Fatalf("BookmarkList: %v", err)
	}
	bookmarks, err := ParseBookmarkList(data)
	if err != nil {
		t.Fatalf("ParseBookmarkList: %v", err)
	}

	// EnsureBookmarks should create new bookmarks.
	results, err := EnsureBookmarks(runner, dags[0], bookmarks, "origin", nil, true)
	if err != nil {
		t.Fatalf("EnsureBookmarks: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		t.Logf("  %s → %s (new=%v, sync=%v)", r.ChangeID, r.Bookmark, r.IsNew, r.SyncState)
		if !r.IsNew {
			t.Errorf("expected IsNew=true for %s", r.ChangeID)
		}
		if r.SyncState != SyncLocalOnly {
			t.Errorf("expected SyncLocalOnly for new bookmark, got %v", r.SyncState)
		}
		if !strings.HasPrefix(r.Bookmark, "jip/") {
			t.Errorf("expected jip/ prefix, got %q", r.Bookmark)
		}
	}

	// Verify the bookmarks actually exist in jj.
	logRepoState(t, repoDir)
	for _, r := range results {
		out := jjRun(t, repoDir, "bookmark", "list", r.Bookmark)
		if !strings.Contains(out, r.Bookmark) {
			t.Errorf("bookmark %q not found in jj bookmark list output: %s", r.Bookmark, out)
		}
	}
}

func TestIntegration_BookmarkReuse(t *testing.T) {
	repoDir, _ := initJJRepoWithRemote(t)
	runner := NewRunner(repoDir)

	// Create a change with a pre-existing bookmark.
	writeAndCommit(t, repoDir, "a.txt", "aaa", "feat: existing branch test")
	jjRun(t, repoDir, "bookmark", "set", "my-existing-branch", "-r", "@-")
	jjRun(t, repoDir, "git", "push", "--bookmark", "my-existing-branch")

	dags, err := ResolveStacks(runner, []string{"@-"}, "main")
	if err != nil {
		t.Fatalf("ResolveStacks: %v", err)
	}

	data, err := runner.BookmarkList()
	if err != nil {
		t.Fatalf("BookmarkList: %v", err)
	}
	bookmarks, err := ParseBookmarkList(data)
	if err != nil {
		t.Fatalf("ParseBookmarkList: %v", err)
	}

	// shouldUseExisting always returns true → reuse existing bookmark.
	results, err := EnsureBookmarks(runner, dags[0], bookmarks, "origin",
		func(changeID, bookmark string) bool { return true }, true)
	if err != nil {
		t.Fatalf("EnsureBookmarks: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.IsNew {
		t.Error("expected IsNew=false for existing bookmark")
	}
	if r.Bookmark != "my-existing-branch" {
		t.Errorf("expected 'my-existing-branch', got %q", r.Bookmark)
	}
	if r.SyncState != SyncInSync {
		t.Errorf("expected SyncInSync, got %v", r.SyncState)
	}
}

func TestIntegration_BookmarkSelectiveReuse(t *testing.T) {
	repoDir, _ := initJJRepoWithRemote(t)
	runner := NewRunner(repoDir)

	// Create a change with a non-jip bookmark.
	writeAndCommit(t, repoDir, "a.txt", "aaa", "feat: selective test")
	jjRun(t, repoDir, "bookmark", "set", "foreign-branch", "-r", "@-")

	dags, err := ResolveStacks(runner, []string{"@-"}, "main")
	if err != nil {
		t.Fatalf("ResolveStacks: %v", err)
	}

	data, err := runner.BookmarkList()
	if err != nil {
		t.Fatalf("BookmarkList: %v", err)
	}
	bookmarks, err := ParseBookmarkList(data)
	if err != nil {
		t.Fatalf("ParseBookmarkList: %v", err)
	}

	// shouldUseExisting rejects non-jip bookmarks → should create a new one.
	results, err := EnsureBookmarks(runner, dags[0], bookmarks, "origin",
		func(changeID, bookmark string) bool {
			return strings.HasPrefix(bookmark, "jip/")
		}, true)
	if err != nil {
		t.Fatalf("EnsureBookmarks: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if !r.IsNew {
		t.Error("expected IsNew=true since foreign-branch was rejected")
	}
	if !strings.HasPrefix(r.Bookmark, "jip/selective-test/") {
		t.Errorf("expected jip/selective-test/ prefix, got %q", r.Bookmark)
	}
}

func TestIntegration_BookmarkMatchToChanges(t *testing.T) {
	repoDir, _ := initJJRepoWithRemote(t)
	runner := NewRunner(repoDir)

	// Create two changes with bookmarks.
	writeAndCommit(t, repoDir, "a.txt", "aaa", "feat: change A")
	jjRun(t, repoDir, "bookmark", "set", "branch-a", "-r", "@-")

	writeAndCommit(t, repoDir, "b.txt", "bbb", "feat: change B")
	jjRun(t, repoDir, "bookmark", "set", "branch-b", "-r", "@-")

	logRepoState(t, repoDir)

	dags, err := ResolveStacks(runner, []string{"@-"}, "main")
	if err != nil {
		t.Fatalf("ResolveStacks: %v", err)
	}

	data, err := runner.BookmarkList()
	if err != nil {
		t.Fatalf("BookmarkList: %v", err)
	}
	bookmarks, err := ParseBookmarkList(data)
	if err != nil {
		t.Fatalf("ParseBookmarkList: %v", err)
	}

	matched := MatchBookmarksToChanges(dags[0], bookmarks)
	t.Logf("matched: %v", matched)

	// Each change should have exactly one matching bookmark.
	for _, change := range dags[0].Changes {
		bms, ok := matched[change.ChangeID]
		if !ok {
			t.Errorf("change %s (%s) has no matched bookmarks", change.ChangeID[:12], change.Description)
			continue
		}
		if len(bms) != 1 {
			t.Errorf("change %s: expected 1 bookmark, got %d", change.ChangeID[:12], len(bms))
		}
	}

	// Verify the correct bookmark matched the correct change.
	changeA := dags[0].Changes[0] // topological order: A is first
	changeB := dags[0].Changes[1]
	if bms := matched[changeA.ChangeID]; bms[0].Name != "branch-a" {
		t.Errorf("expected change A matched to branch-a, got %s", bms[0].Name)
	}
	if bms := matched[changeB.ChangeID]; bms[0].Name != "branch-b" {
		t.Errorf("expected change B matched to branch-b, got %s", bms[0].Name)
	}
}
