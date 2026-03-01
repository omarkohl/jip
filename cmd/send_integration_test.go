//go:build integration

package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	gh "github.com/omarkohl/jip/internal/github"
	"github.com/omarkohl/jip/internal/jj"
)

// mockService implements gh.Service with in-memory state.
type mockService struct {
	mu        sync.Mutex
	prs       map[int]*gh.PRInfo
	comments  map[int][]string
	reviewers map[int][]string
	nextPR    int
	owner     string
	repo      string
}

func newMockService() *mockService {
	return &mockService{
		prs:       make(map[int]*gh.PRInfo),
		comments:  make(map[int][]string),
		reviewers: make(map[int][]string),
		nextPR:    1,
		owner:     "testowner",
		repo:      "testrepo",
	}
}

func (m *mockService) Owner() string { return m.owner }
func (m *mockService) Repo() string  { return m.repo }

func (m *mockService) GetAuthenticatedUser() (string, error) {
	return "testuser", nil
}

func (m *mockService) CreatePR(head, base, title, body string, draft bool) (*gh.PRInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	num := m.nextPR
	m.nextPR++
	pr := &gh.PRInfo{
		Number:      num,
		State:       "OPEN",
		URL:         fmt.Sprintf("https://github.com/%s/%s/pull/%d", m.owner, m.repo, num),
		Title:       title,
		Body:        body,
		HeadRefName: head,
		BaseRefName: base,
		IsDraft:     draft,
	}
	m.prs[num] = pr
	return pr, nil
}

func (m *mockService) UpdatePR(number int, opts gh.UpdatePROpts) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	pr := m.prs[number]
	if pr != nil {
		if opts.Title != nil {
			pr.Title = *opts.Title
		}
		if opts.Body != nil {
			pr.Body = *opts.Body
		}
		if opts.Base != nil {
			pr.BaseRefName = *opts.Base
		}
	}
	return nil
}

func (m *mockService) CommentOnPR(number int, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.comments[number] = append(m.comments[number], body)
	return nil
}

func (m *mockService) RequestReviewers(number int, reviewers []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reviewers[number] = append(m.reviewers[number], reviewers...)
	return nil
}

func (m *mockService) LookupPRsByBranch(branches []string) (map[string]*gh.PRInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make(map[string]*gh.PRInfo)
	for _, branch := range branches {
		for _, pr := range m.prs {
			if pr.HeadRefName == branch && pr.State == "OPEN" {
				result[branch] = pr
				break
			}
		}
	}
	return result, nil
}

func TestIntegration_SendCreatesNewPRs(t *testing.T) {
	checkJJ(t)

	mock := newMockService()
	repoDir, _ := initTestRepoWithRemote(t)
	runner := jj.NewRunner(repoDir)

	// Create two changes.
	writeAndCommit(t, repoDir, "a.go", "package a", "feat: add feature A")
	writeAndCommit(t, repoDir, "b.go", "package b", "fix: fix bug B")

	var buf bytes.Buffer
	err := executeSend(runner, mock, sendOpts{
		base:    "main",
		remote:  "origin",
		revsets: []string{"@-"},
	}, &buf)
	if err != nil {
		t.Fatalf("send failed: %v\nOutput:\n%s", err, buf.String())
	}

	output := buf.String()
	t.Logf("Output:\n%s", output)

	// Verify PRs were created.
	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.prs) != 2 {
		t.Errorf("expected 2 PRs created, got %d", len(mock.prs))
	}

	// Verify PR bodies contain stack navigation.
	for _, pr := range mock.prs {
		if !strings.Contains(pr.Body, "stacked PR") {
			t.Errorf("PR #%d body missing stack navigation:\n%s", pr.Number, pr.Body)
		}
		if !strings.Contains(pr.Body, "review commit") {
			t.Errorf("PR #%d body missing review commit:\n%s", pr.Number, pr.Body)
		}
	}

	if !strings.Contains(output, "created") {
		t.Error("expected 'created' in output")
	}
	if !strings.Contains(output, "2 PR(s) sent") {
		t.Errorf("expected '2 PR(s) sent' in output, got:\n%s", output)
	}
}

func TestIntegration_SendDryRun(t *testing.T) {
	checkJJ(t)

	mock := newMockService()
	repoDir, _ := initTestRepoWithRemote(t)
	runner := jj.NewRunner(repoDir)

	writeAndCommit(t, repoDir, "a.go", "package a", "feat: dry run test")

	var buf bytes.Buffer
	err := executeSend(runner, mock, sendOpts{
		base:    "main",
		remote:  "origin",
		revsets: []string{"@-"},
		dryRun:  true,
	}, &buf)
	if err != nil {
		t.Fatalf("send --dry-run failed: %v\nOutput:\n%s", err, buf.String())
	}

	output := buf.String()
	t.Logf("Output:\n%s", output)

	if !strings.Contains(output, "Dry run") {
		t.Error("expected 'Dry run' in output")
	}
	if !strings.Contains(output, "CREATE") {
		t.Error("expected 'CREATE' action in dry run output")
	}

	// Verify no PRs were actually created.
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.prs) != 0 {
		t.Errorf("expected 0 PRs in dry run, got %d", len(mock.prs))
	}
}

func TestIntegration_SendExistingOnlySkipsNewPRs(t *testing.T) {
	checkJJ(t)

	mock := newMockService()
	repoDir, _ := initTestRepoWithRemote(t)
	runner := jj.NewRunner(repoDir)

	// Create two changes: A and B.
	writeAndCommit(t, repoDir, "a.go", "package a", "feat: feature A")
	writeAndCommit(t, repoDir, "b.go", "package b", "feat: feature B")

	// Send only A (first change) to create its PR.
	var buf bytes.Buffer
	err := executeSend(runner, mock, sendOpts{
		base:    "main",
		remote:  "origin",
		revsets: []string{"@--"},
	}, &buf)
	if err != nil {
		t.Fatalf("first send failed: %v\nOutput:\n%s", err, buf.String())
	}
	t.Logf("First send:\n%s", buf.String())

	mock.mu.Lock()
	if len(mock.prs) != 1 {
		t.Fatalf("expected 1 PR after first send, got %d", len(mock.prs))
	}
	mock.mu.Unlock()

	// Now send both A and B with --existing: only A should be updated.
	buf.Reset()
	err = executeSend(runner, mock, sendOpts{
		base:     "main",
		remote:   "origin",
		revsets:  []string{"@-"},
		existing: true,
	}, &buf)
	if err != nil {
		t.Fatalf("second send (--existing) failed: %v\nOutput:\n%s", err, buf.String())
	}
	t.Logf("Second send:\n%s", buf.String())

	output := buf.String()

	// Should skip one change without an existing PR.
	if !strings.Contains(output, "Skipping 1 change(s)") {
		t.Errorf("expected skip message in output, got:\n%s", output)
	}

	// Only the existing PR should be updated, no new ones created.
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.prs) != 1 {
		t.Errorf("expected still 1 PR with --existing, got %d", len(mock.prs))
	}

	if !strings.Contains(output, "up-to-date") {
		t.Error("expected 'up-to-date' in output")
	}
	if strings.Contains(output, "created") {
		t.Error("should not contain 'created' with --existing")
	}
}

func TestIntegration_SendExistingOnlyNoExistingPRs(t *testing.T) {
	checkJJ(t)

	mock := newMockService()
	repoDir, _ := initTestRepoWithRemote(t)
	runner := jj.NewRunner(repoDir)

	// Create a change but don't send it first.
	writeAndCommit(t, repoDir, "a.go", "package a", "feat: new feature")

	var buf bytes.Buffer
	err := executeSend(runner, mock, sendOpts{
		base:     "main",
		remote:   "origin",
		revsets:  []string{"@-"},
		existing: true,
	}, &buf)
	if err != nil {
		t.Fatalf("send --existing failed: %v\nOutput:\n%s", err, buf.String())
	}

	output := buf.String()
	t.Logf("Output:\n%s", output)

	if !strings.Contains(output, "No existing PRs to update") {
		t.Errorf("expected 'No existing PRs to update' in output, got:\n%s", output)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.prs) != 0 {
		t.Errorf("expected 0 PRs with --existing, got %d", len(mock.prs))
	}
}

func TestIntegration_SendUpdatesExistingPRs(t *testing.T) {
	checkJJ(t)

	mock := newMockService()
	repoDir, _ := initTestRepoWithRemote(t)
	runner := jj.NewRunner(repoDir)

	// Create a change, send it (creates PR).
	writeAndCommit(t, repoDir, "a.go", "package a", "feat: initial feature")

	var buf bytes.Buffer
	err := executeSend(runner, mock, sendOpts{
		base:    "main",
		remote:  "origin",
		revsets: []string{"@-"},
	}, &buf)
	if err != nil {
		t.Fatalf("first send failed: %v\nOutput:\n%s", err, buf.String())
	}
	t.Logf("First send:\n%s", buf.String())

	mock.mu.Lock()
	if len(mock.prs) != 1 {
		t.Fatalf("expected 1 PR after first send, got %d", len(mock.prs))
	}
	mock.mu.Unlock()

	// Now send again — should detect existing PR and update.
	buf.Reset()
	err = executeSend(runner, mock, sendOpts{
		base:    "main",
		remote:  "origin",
		revsets: []string{"@-"},
	}, &buf)
	if err != nil {
		t.Fatalf("second send failed: %v\nOutput:\n%s", err, buf.String())
	}
	t.Logf("Second send:\n%s", buf.String())

	// No new PRs should have been created.
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.prs) != 1 {
		t.Errorf("expected still 1 PR after second send, got %d", len(mock.prs))
	}

	if !strings.Contains(buf.String(), "up-to-date") {
		t.Error("expected 'up-to-date' in second send output")
	}
}

func TestIntegration_SendDiamondDAG(t *testing.T) {
	checkJJ(t)

	mock := newMockService()
	repoDir, _ := initTestRepoWithRemote(t)
	runner := jj.NewRunner(repoDir)

	// Build diamond merge topology:
	//
	//   main
	//   ├── A "feat: add user authentication"
	//   │   └── B "feat: add password validation"
	//   │                                         \
	//   └── C "refactor: extract email service" ── D "feat: integrate auth with email notifications"

	// Linear chain: main → A → B
	writeAndCommit(t, repoDir, "auth.go",
		"package auth\n\nfunc Login() {}",
		"feat: add user authentication")
	writeAndCommit(t, repoDir, "validate.go",
		"package auth\n\nfunc ValidatePassword(p string) bool { return len(p) >= 8 }",
		"feat: add password validation")
	idB := getChangeID(t, repoDir, "@-")

	// C branching off main independently
	jjRun(t, repoDir, "new", "main")
	writeAndCommit(t, repoDir, "email.go",
		"package email\n\nfunc SendWelcome(addr string) error { return nil }",
		"refactor: extract email service")
	idC := getChangeID(t, repoDir, "@-")

	// D merging B and C
	jjRun(t, repoDir, "new", idB, idC)
	writeAndCommit(t, repoDir, "notify.go",
		"package notify\n\nfunc OnLogin(user string) {}",
		"feat: integrate auth with email notifications")

	var buf bytes.Buffer
	err := executeSend(runner, mock, sendOpts{
		base:    "main",
		remote:  "origin",
		revsets: []string{"@-"},
	}, &buf)
	if err != nil {
		t.Fatalf("send failed: %v\nOutput:\n%s", err, buf.String())
	}

	output := buf.String()
	t.Logf("Output:\n%s", output)

	mock.mu.Lock()
	defer mock.mu.Unlock()

	// All 4 changes should produce 4 PRs.
	if len(mock.prs) != 4 {
		t.Fatalf("expected 4 PRs, got %d", len(mock.prs))
	}

	// Verify each expected title exists.
	titles := make(map[string]bool)
	for _, pr := range mock.prs {
		titles[pr.Title] = true
	}
	for _, want := range []string{
		"feat: add user authentication",
		"feat: add password validation",
		"refactor: extract email service",
		"feat: integrate auth with email notifications",
	} {
		if !titles[want] {
			t.Errorf("missing PR with title %q", want)
		}
	}

	// All PRs target main.
	for _, pr := range mock.prs {
		if pr.BaseRefName != "main" {
			t.Errorf("PR #%d base is %q, expected \"main\"", pr.Number, pr.BaseRefName)
		}
	}

	// Build title → PR lookup for precise stack assertions.
	prByTitle := make(map[string]*gh.PRInfo)
	for _, pr := range mock.prs {
		prByTitle[pr.Title] = pr
	}
	prA := prByTitle["feat: add user authentication"]
	prB := prByTitle["feat: add password validation"]
	prC := prByTitle["refactor: extract email service"]
	prD := prByTitle["feat: integrate auth with email notifications"]

	// Every PR body should have stacked PR navigation.
	for _, pr := range mock.prs {
		if !strings.Contains(pr.Body, "stacked PR") {
			t.Errorf("PR #%d (%s) body missing 'stacked PR':\n%s", pr.Number, pr.Title, pr.Body)
		}
		if !strings.Contains(pr.Body, "review commit") {
			t.Errorf("PR #%d (%s) body missing 'review commit':\n%s", pr.Number, pr.Title, pr.Body)
		}
	}

	// A's stack: A, B, D (not C — C is an unrelated branch).
	assertPRRefsInBody(t, prA, []*gh.PRInfo{prA, prB, prD}, []*gh.PRInfo{prC})
	// B's stack: A, B, D (not C).
	assertPRRefsInBody(t, prB, []*gh.PRInfo{prA, prB, prD}, []*gh.PRInfo{prC})
	// C's stack: C, D (not A or B).
	assertPRRefsInBody(t, prC, []*gh.PRInfo{prC, prD}, []*gh.PRInfo{prA, prB})
	// D's stack: all four (it merges both branches).
	assertPRRefsInBody(t, prD, []*gh.PRInfo{prA, prB, prC, prD}, nil)

	if !strings.Contains(output, "4 PR(s) sent") {
		t.Errorf("expected '4 PR(s) sent' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "created") {
		t.Error("expected 'created' in output")
	}
}

func TestIntegration_SendPostsInterdiffComment(t *testing.T) {
	checkJJ(t)

	mock := newMockService()
	repoDir, _ := initTestRepoWithRemote(t)
	runner := jj.NewRunner(repoDir)

	// Create a change with a simple request handler.
	writeAndCommit(t, repoDir, "handler.go",
		"package api\n\nfunc Handle() error {\n\treturn nil\n}",
		"feat: add request handler")
	changeID := getChangeID(t, repoDir, "@-")

	// First send — creates the PR and pushes the bookmark to the remote.
	var buf bytes.Buffer
	err := executeSend(runner, mock, sendOpts{
		base:    "main",
		remote:  "origin",
		revsets: []string{"@-"},
	}, &buf)
	if err != nil {
		t.Fatalf("first send failed: %v\nOutput:\n%s", err, buf.String())
	}
	t.Logf("First send:\n%s", buf.String())

	mock.mu.Lock()
	if len(mock.prs) != 1 {
		t.Fatalf("expected 1 PR after first send, got %d", len(mock.prs))
	}
	var prNumber int
	for n := range mock.prs {
		prNumber = n
	}
	mock.mu.Unlock()

	// Modify the change: add input validation to the handler.
	jjRun(t, repoDir, "edit", changeID)
	if err := os.WriteFile(
		filepath.Join(repoDir, "handler.go"),
		[]byte("package api\n\nimport \"fmt\"\n\nfunc Handle(input string) error {\n\tif input == \"\" {\n\t\treturn fmt.Errorf(\"input must not be empty\")\n\t}\n\treturn nil\n}"),
		0644,
	); err != nil {
		t.Fatalf("writing updated file: %v", err)
	}
	// Move working copy off the edited change so @- resolves to it.
	jjRun(t, repoDir, "new", changeID)

	// Second send — should detect the changed commit and post an interdiff comment.
	buf.Reset()
	err = executeSend(runner, mock, sendOpts{
		base:    "main",
		remote:  "origin",
		revsets: []string{"@-"},
	}, &buf)
	if err != nil {
		t.Fatalf("second send failed: %v\nOutput:\n%s", err, buf.String())
	}
	t.Logf("Second send:\n%s", buf.String())

	mock.mu.Lock()
	defer mock.mu.Unlock()

	// No new PR should have been created.
	if len(mock.prs) != 1 {
		t.Errorf("expected still 1 PR, got %d", len(mock.prs))
	}

	// Verify the interdiff comment was posted on the PR.
	comments := mock.comments[prNumber]
	if len(comments) == 0 {
		t.Fatal("expected an interdiff comment, got none")
	}
	if len(comments) != 1 {
		t.Errorf("expected exactly 1 comment, got %d", len(comments))
	}

	comment := comments[0]

	// The comment should have the standard interdiff header.
	if !strings.Contains(comment, "Changes since last push") {
		t.Errorf("comment missing header:\n%s", comment)
	}

	// The comment should reference the changed file.
	if !strings.Contains(comment, "handler.go") {
		t.Errorf("comment missing reference to 'handler.go':\n%s", comment)
	}

	// The comment should contain the actual diff content showing the added validation.
	if !strings.Contains(comment, "input") {
		t.Errorf("comment missing diff content (expected 'input'):\n%s", comment)
	}

	// The comment should have a GitHub compare link.
	if !strings.Contains(comment, "View the diff on") {
		t.Errorf("comment missing GitHub compare link:\n%s", comment)
	}

	// The output should indicate the PR was updated, not created.
	if !strings.Contains(buf.String(), "updated") {
		t.Error("expected 'updated' in second send output")
	}
}

func TestIntegration_SendCrossForkPrefixesHead(t *testing.T) {
	checkJJ(t)

	mock := newMockService()
	repoDir, _ := initTestRepoWithRemote(t)
	runner := jj.NewRunner(repoDir)

	writeAndCommit(t, repoDir, "a.go", "package a", "feat: fork feature")

	var buf bytes.Buffer
	err := executeSend(runner, mock, sendOpts{
		base:      "main",
		remote:    "origin",
		pushOwner: "forkuser",
		revsets:   []string{"@-"},
	}, &buf)
	if err != nil {
		t.Fatalf("send failed: %v\nOutput:\n%s", err, buf.String())
	}

	t.Logf("Output:\n%s", buf.String())

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.prs) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(mock.prs))
	}

	for _, pr := range mock.prs {
		if !strings.HasPrefix(pr.HeadRefName, "forkuser:") {
			t.Errorf("expected head to start with 'forkuser:', got %q", pr.HeadRefName)
		}
	}
}

func TestIntegration_SendNoPrefixWithoutUpstream(t *testing.T) {
	checkJJ(t)

	mock := newMockService()
	repoDir, _ := initTestRepoWithRemote(t)
	runner := jj.NewRunner(repoDir)

	writeAndCommit(t, repoDir, "a.go", "package a", "feat: normal feature")

	var buf bytes.Buffer
	err := executeSend(runner, mock, sendOpts{
		base:    "main",
		remote:  "origin",
		revsets: []string{"@-"},
	}, &buf)
	if err != nil {
		t.Fatalf("send failed: %v\nOutput:\n%s", err, buf.String())
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()

	for _, pr := range mock.prs {
		if strings.Contains(pr.HeadRefName, ":") {
			t.Errorf("expected no owner prefix in head, got %q", pr.HeadRefName)
		}
	}
}

func TestIntegration_SendPassesRemoteToGitPush(t *testing.T) {
	checkJJ(t)

	mock := newMockService()
	repoDir, _ := initTestRepoWithRemote(t)
	spy := &spyRunner{Runner: jj.NewRunner(repoDir)}

	writeAndCommit(t, repoDir, "a.go", "package a", "feat: remote test")

	var buf bytes.Buffer
	err := executeSend(spy, mock, sendOpts{
		base:    "main",
		remote:  "origin",
		revsets: []string{"@-"},
	}, &buf)
	if err != nil {
		t.Fatalf("send failed: %v\nOutput:\n%s", err, buf.String())
	}

	if spy.pushRemote != "origin" {
		t.Errorf("expected GitPush remote 'origin', got %q", spy.pushRemote)
	}
}

func TestIntegration_SendFetchesRemote(t *testing.T) {
	checkJJ(t)

	mock := newMockService()
	repoDir, _ := initTestRepoWithRemote(t)
	spy := &spyRunner{Runner: jj.NewRunner(repoDir)}

	writeAndCommit(t, repoDir, "a.go", "package a", "feat: fetch test")

	var buf bytes.Buffer
	err := executeSend(spy, mock, sendOpts{
		base:    "main",
		remote:  "origin",
		revsets: []string{"@-"},
	}, &buf)
	if err != nil {
		t.Fatalf("send failed: %v\nOutput:\n%s", err, buf.String())
	}

	if len(spy.fetchRemotes) != 1 || spy.fetchRemotes[0] != "origin" {
		t.Errorf("expected fetch of [origin], got %v", spy.fetchRemotes)
	}
}

func TestIntegration_SendFetchesUpstreamRemote(t *testing.T) {
	checkJJ(t)

	mock := newMockService()
	repoDir, remoteDir := initTestRepoWithRemote(t)

	// Add a second bare remote to act as upstream.
	upstreamDir := t.TempDir()
	gitCmd := exec.Command("git", "init", "--bare", upstreamDir)
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare upstream: %v\n%s", err, out)
	}
	jjRun(t, repoDir, "git", "remote", "add", "upstream", upstreamDir)
	// Push main to upstream so fetch succeeds.
	jjRun(t, repoDir, "git", "push", "--remote", "upstream", "--bookmark", "main", "--allow-new")

	spy := &spyRunner{Runner: jj.NewRunner(repoDir)}

	writeAndCommit(t, repoDir, "a.go", "package a", "feat: upstream fetch test")

	var buf bytes.Buffer
	err := executeSend(spy, mock, sendOpts{
		base:           "main",
		remote:         "origin",
		upstreamRemote: "upstream",
		revsets:        []string{"@-"},
	}, &buf)
	if err != nil {
		t.Fatalf("send failed: %v\nOutput:\n%s", err, buf.String())
	}

	if len(spy.fetchRemotes) != 2 {
		t.Fatalf("expected 2 fetches, got %v", spy.fetchRemotes)
	}
	if spy.fetchRemotes[0] != "origin" || spy.fetchRemotes[1] != "upstream" {
		t.Errorf("expected fetch [origin, upstream], got %v", spy.fetchRemotes)
	}

	_ = remoteDir // used by initTestRepoWithRemote cleanup
}

func TestIntegration_SendSkipsFetchForUpstreamURL(t *testing.T) {
	checkJJ(t)

	mock := newMockService()
	repoDir, _ := initTestRepoWithRemote(t)
	spy := &spyRunner{Runner: jj.NewRunner(repoDir)}

	writeAndCommit(t, repoDir, "a.go", "package a", "feat: url upstream test")

	var buf bytes.Buffer
	err := executeSend(spy, mock, sendOpts{
		base:     "main",
		remote:   "origin",
		upstream: "https://github.com/other/repo.git",
		// upstreamRemote is empty — upstream was a URL, not a remote name
		pushOwner: "myuser",
		revsets:   []string{"@-"},
	}, &buf)
	if err != nil {
		t.Fatalf("send failed: %v\nOutput:\n%s", err, buf.String())
	}

	// Should only fetch origin, not the URL.
	if len(spy.fetchRemotes) != 1 || spy.fetchRemotes[0] != "origin" {
		t.Errorf("expected fetch of [origin] only, got %v", spy.fetchRemotes)
	}
}

func TestIntegration_SendNoStack(t *testing.T) {
	checkJJ(t)

	mock := newMockService()
	repoDir, _ := initTestRepoWithRemote(t)
	runner := jj.NewRunner(repoDir)

	// Create two changes (a linear stack).
	writeAndCommit(t, repoDir, "a.go", "package a", "feat: add feature A")
	writeAndCommit(t, repoDir, "b.go", "package b", "feat: add feature B")

	var buf bytes.Buffer
	err := executeSend(runner, mock, sendOpts{
		base:    "main",
		remote:  "origin",
		revsets: []string{"@-"},
		noStack: true,
	}, &buf)
	if err != nil {
		t.Fatalf("send --no-stack failed: %v\nOutput:\n%s", err, buf.String())
	}

	output := buf.String()
	t.Logf("Output:\n%s", output)

	mock.mu.Lock()
	defer mock.mu.Unlock()

	// Only 1 PR should be created (the tip), not 2.
	if len(mock.prs) != 1 {
		t.Fatalf("expected 1 PR with --no-stack, got %d", len(mock.prs))
	}

	// PR title should match the tip commit's description.
	for _, pr := range mock.prs {
		if pr.Title != "feat: add feature B" {
			t.Errorf("expected PR title 'feat: add feature B', got %q", pr.Title)
		}
		// PR body should NOT contain stack navigation.
		if strings.Contains(pr.Body, "stacked PR") {
			t.Errorf("PR body should not contain stack navigation with --no-stack:\n%s", pr.Body)
		}
	}

	if !strings.Contains(output, "1 PR(s) sent") {
		t.Errorf("expected '1 PR(s) sent' in output, got:\n%s", output)
	}
}

func TestIntegration_SendRebase(t *testing.T) {
	checkJJ(t)

	mock := newMockService()
	repoDir, _ := initTestRepoWithRemote(t)
	spy := &spyRunner{Runner: jj.NewRunner(repoDir)}

	// Create a change on top of main.
	writeAndCommit(t, repoDir, "a.go", "package a", "feat: rebase test")

	var buf bytes.Buffer
	err := executeSend(spy, mock, sendOpts{
		base:    "main",
		remote:  "origin",
		revsets: []string{"@-"},
		rebase:  true,
	}, &buf)
	if err != nil {
		t.Fatalf("send --rebase failed: %v\nOutput:\n%s", err, buf.String())
	}

	output := buf.String()
	t.Logf("Output:\n%s", output)

	// Verify rebase was called with the correct arguments.
	if len(spy.rebaseCalls) != 1 {
		t.Fatalf("expected 1 rebase call, got %d", len(spy.rebaseCalls))
	}
	rc := spy.rebaseCalls[0]
	if len(rc.revsets) != 1 || rc.revsets[0] != "@-" {
		t.Errorf("expected rebase revsets [@-], got %v", rc.revsets)
	}
	if rc.destination != "main" {
		t.Errorf("expected rebase destination 'main', got %q", rc.destination)
	}

	// Output should mention rebasing.
	if !strings.Contains(output, "Rebasing onto main") {
		t.Errorf("expected 'Rebasing onto main' in output, got:\n%s", output)
	}

	// PR should still be created successfully.
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.prs) != 1 {
		t.Errorf("expected 1 PR, got %d", len(mock.prs))
	}
}

func TestIntegration_SendNoRebaseByDefault(t *testing.T) {
	checkJJ(t)

	mock := newMockService()
	repoDir, _ := initTestRepoWithRemote(t)
	spy := &spyRunner{Runner: jj.NewRunner(repoDir)}

	writeAndCommit(t, repoDir, "a.go", "package a", "feat: no rebase test")

	var buf bytes.Buffer
	err := executeSend(spy, mock, sendOpts{
		base:    "main",
		remote:  "origin",
		revsets: []string{"@-"},
	}, &buf)
	if err != nil {
		t.Fatalf("send failed: %v\nOutput:\n%s", err, buf.String())
	}

	// Rebase should NOT have been called.
	if len(spy.rebaseCalls) != 0 {
		t.Errorf("expected 0 rebase calls without --rebase, got %d", len(spy.rebaseCalls))
	}
}

func TestIntegration_SendSkipsBehindBookmark(t *testing.T) {
	checkJJ(t)

	mock := newMockService()
	repoDir, remoteDir := initTestRepoWithRemote(t)
	runner := jj.NewRunner(repoDir)

	// Create a change and send it (creates PR + pushes bookmark).
	writeAndCommit(t, repoDir, "a.go", "package a", "feat: behind test")
	changeID := getChangeID(t, repoDir, "@-")

	var buf bytes.Buffer
	err := executeSend(runner, mock, sendOpts{
		base:    "main",
		remote:  "origin",
		revsets: []string{"@-"},
	}, &buf)
	if err != nil {
		t.Fatalf("first send failed: %v\nOutput:\n%s", err, buf.String())
	}
	t.Logf("First send:\n%s", buf.String())

	// Find the bookmark name that was created.
	bmName := findBookmarkForChange(t, runner, changeID)

	// Advance the remote branch independently using plain git (simulates
	// another person pushing to the same branch).
	altDir := t.TempDir()
	gitRun(t, "", "clone", remoteDir, altDir)
	gitRun(t, altDir, "checkout", bmName)
	gitRun(t, altDir, "config", "user.email", "other@jip.dev")
	gitRun(t, altDir, "config", "user.name", "Other User")
	writeFile(t, altDir, "extra.go", "package extra")
	gitRun(t, altDir, "add", "extra.go")
	gitRun(t, altDir, "commit", "-m", "remote-only change")
	gitRun(t, altDir, "push", "origin", bmName)

	// Re-send without local changes: bookmark is now behind remote.
	buf.Reset()
	err = executeSend(runner, mock, sendOpts{
		base:    "main",
		remote:  "origin",
		revsets: []string{"@-"},
	}, &buf)

	output := buf.String()
	t.Logf("Second send:\n%s", output)

	// Should return an error.
	if err == nil {
		t.Fatal("expected error from send with behind bookmark, got nil")
	}

	// Error message should mention skipped changes.
	if !strings.Contains(err.Error(), "skipped") {
		t.Errorf("expected 'skipped' in error, got: %v", err)
	}

	// Output should mention that remote is ahead.
	if !strings.Contains(output, "remote is ahead") {
		t.Errorf("expected 'remote is ahead' in output, got:\n%s", output)
	}

	// No new PRs should have been created (only the original one).
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.prs) != 1 {
		t.Errorf("expected still 1 PR, got %d", len(mock.prs))
	}
}

func TestIntegration_SendSkipsDescendantsOfBehind(t *testing.T) {
	checkJJ(t)

	mock := newMockService()
	repoDir, remoteDir := initTestRepoWithRemote(t)
	runner := jj.NewRunner(repoDir)

	// Create root change and send it.
	writeAndCommit(t, repoDir, "a.go", "package a", "feat: root change")
	rootChangeID := getChangeID(t, repoDir, "@-")

	var buf bytes.Buffer
	err := executeSend(runner, mock, sendOpts{
		base:    "main",
		remote:  "origin",
		revsets: []string{"@-"},
	}, &buf)
	if err != nil {
		t.Fatalf("first send failed: %v\nOutput:\n%s", err, buf.String())
	}
	t.Logf("First send:\n%s", buf.String())

	// Find the root's bookmark name.
	rootBmName := findBookmarkForChange(t, runner, rootChangeID)

	// Add a child change on top.
	writeAndCommit(t, repoDir, "b.go", "package b", "feat: child change")

	// Advance the remote branch for root independently using plain git.
	altDir := t.TempDir()
	gitRun(t, "", "clone", remoteDir, altDir)
	gitRun(t, altDir, "checkout", rootBmName)
	gitRun(t, altDir, "config", "user.email", "other@jip.dev")
	gitRun(t, altDir, "config", "user.name", "Other User")
	writeFile(t, altDir, "extra.go", "package extra")
	gitRun(t, altDir, "add", "extra.go")
	gitRun(t, altDir, "commit", "-m", "remote-only change on root")
	gitRun(t, altDir, "push", "origin", rootBmName)

	// Re-send both changes: root is behind, child skipped as descendant.
	buf.Reset()
	err = executeSend(runner, mock, sendOpts{
		base:    "main",
		remote:  "origin",
		revsets: []string{"@-"},
	}, &buf)

	output := buf.String()
	t.Logf("Second send:\n%s", output)

	if err == nil {
		t.Fatal("expected error from send with behind bookmark, got nil")
	}

	// Should skip 2 changes.
	if !strings.Contains(output, "Skipped 2 change(s)") {
		t.Errorf("expected 'Skipped 2 change(s)' in output, got:\n%s", output)
	}

	// Output should mention that remote is ahead for root.
	if !strings.Contains(output, "remote is ahead") {
		t.Errorf("expected 'remote is ahead' in output, got:\n%s", output)
	}

	// Output should mention ancestor skip for child.
	if !strings.Contains(output, "ancestor") {
		t.Errorf("expected 'ancestor' in output, got:\n%s", output)
	}
}

func TestIntegration_SendSkipsConflictedChanges(t *testing.T) {
	checkJJ(t)

	mock := newMockService()
	repoDir, _ := initTestRepoWithRemote(t)
	runner := jj.NewRunner(repoDir)

	// Create two branches that modify the same file, then merge them to
	// produce a content conflict.
	//
	//   main
	//   ├── A (modifies shared.go one way)
	//   └── B (modifies shared.go another way)
	//        \
	//         C (merge of A and B — has conflict)

	// Branch A: modify shared.go
	writeAndCommit(t, repoDir, "shared.go", "package shared\n\nvar X = 1", "feat: branch A")
	idA := getChangeID(t, repoDir, "@-")

	// Branch B: modify shared.go differently (off main)
	jjRun(t, repoDir, "new", "main")
	writeAndCommit(t, repoDir, "shared.go", "package shared\n\nvar X = 2", "feat: branch B")
	idB := getChangeID(t, repoDir, "@-")

	// Merge A and B — this creates a conflict in shared.go.
	jjRun(t, repoDir, "new", idA, idB)
	jjRun(t, repoDir, "commit", "-m", "feat: merge with conflict")

	var buf bytes.Buffer
	err := executeSend(runner, mock, sendOpts{
		base:    "main",
		remote:  "origin",
		revsets: []string{"@-"},
	}, &buf)

	output := buf.String()
	t.Logf("Output:\n%s", output)

	// Should return an error because of skipped changes.
	if err == nil {
		t.Fatal("expected error from send with conflicted change, got nil")
	}

	// The conflicted merge change (and possibly descendants) should be skipped.
	if !strings.Contains(output, "has conflicts") {
		t.Errorf("expected 'has conflicts' in output, got:\n%s", output)
	}

	// A and B should still be sent (they have no conflicts).
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.prs) != 2 {
		t.Errorf("expected 2 PRs (non-conflicted changes), got %d", len(mock.prs))
	}
}

func TestIntegration_SendSkipsDescendantsOfConflicted(t *testing.T) {
	checkJJ(t)

	mock := newMockService()
	repoDir, _ := initTestRepoWithRemote(t)
	runner := jj.NewRunner(repoDir)

	// Create a conflicted change with a descendant.
	//
	//   main
	//   ├── A (modifies shared.go one way)
	//   └── B (modifies shared.go another way)
	//        \
	//         C (merge — conflicted) → D (descendant of conflicted)

	writeAndCommit(t, repoDir, "shared.go", "package shared\n\nvar X = 1", "feat: branch A")
	idA := getChangeID(t, repoDir, "@-")

	jjRun(t, repoDir, "new", "main")
	writeAndCommit(t, repoDir, "shared.go", "package shared\n\nvar X = 2", "feat: branch B")
	idB := getChangeID(t, repoDir, "@-")

	// Merge A and B (conflict).
	jjRun(t, repoDir, "new", idA, idB)
	jjRun(t, repoDir, "commit", "-m", "feat: conflicted merge")

	// Add a descendant of the conflicted merge.
	writeAndCommit(t, repoDir, "extra.go", "package extra", "feat: descendant of conflict")

	var buf bytes.Buffer
	err := executeSend(runner, mock, sendOpts{
		base:    "main",
		remote:  "origin",
		revsets: []string{"@-"},
	}, &buf)

	output := buf.String()
	t.Logf("Output:\n%s", output)

	if err == nil {
		t.Fatal("expected error from send with conflicted change, got nil")
	}

	// Should skip 2 changes (the conflicted merge + its descendant).
	if !strings.Contains(output, "Skipped 2 change(s)") {
		t.Errorf("expected 'Skipped 2 change(s)' in output, got:\n%s", output)
	}

	// The conflicted change should mention conflicts.
	if !strings.Contains(output, "has conflicts") {
		t.Errorf("expected 'has conflicts' in output, got:\n%s", output)
	}

	// The descendant should mention ancestor skip.
	if !strings.Contains(output, "ancestor") {
		t.Errorf("expected 'ancestor' in output, got:\n%s", output)
	}

	// A and B should still be sent.
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.prs) != 2 {
		t.Errorf("expected 2 PRs (non-conflicted changes), got %d", len(mock.prs))
	}
}

// findBookmarkForChange returns the bookmark name associated with a change ID.
func findBookmarkForChange(t *testing.T, runner jj.Runner, changeID string) string {
	t.Helper()
	bookmarkData, err := runner.BookmarkList()
	if err != nil {
		t.Fatalf("listing bookmarks: %v", err)
	}
	bookmarks, err := jj.ParseBookmarkList(bookmarkData)
	if err != nil {
		t.Fatalf("parsing bookmarks: %v", err)
	}
	for _, b := range bookmarks {
		if b.ChangeID == changeID {
			return b.Name
		}
	}
	t.Fatalf("bookmark not found for change %s", changeID)
	return ""
}

// gitRun runs a plain git command in the given directory (or without -C if dir is empty).
func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// writeFile writes content to a file without committing.
func writeFile(t *testing.T, dir, filename, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatalf("writing %s: %v", filename, err)
	}
}

// spyRunner wraps a real Runner and records remotes passed to GitFetch/GitPush/Rebase.
type spyRunner struct {
	jj.Runner
	fetchRemotes []string
	pushRemote   string
	rebaseCalls  []rebaseCall
}

type rebaseCall struct {
	revsets     []string
	destination string
}

func (s *spyRunner) GitFetch(remote string) error {
	s.fetchRemotes = append(s.fetchRemotes, remote)
	return s.Runner.GitFetch(remote)
}

func (s *spyRunner) GitPush(bookmarks []string, allowNew bool, remote string) error {
	s.pushRemote = remote
	return s.Runner.GitPush(bookmarks, allowNew, remote)
}

func (s *spyRunner) Rebase(revsets []string, destination string) error {
	s.rebaseCalls = append(s.rebaseCalls, rebaseCall{revsets: revsets, destination: destination})
	return s.Runner.Rebase(revsets, destination)
}

// --- Test helpers ---

func checkJJ(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not found in PATH, skipping integration test")
	}
}

func initTestRepoWithRemote(t *testing.T) (string, string) {
	t.Helper()

	remoteDir, err := os.MkdirTemp("", "jip-remote-*")
	if err != nil {
		t.Fatalf("creating remote dir: %v", err)
	}
	repoDir, err := os.MkdirTemp("", "jip-integration-*")
	if err != nil {
		t.Fatalf("creating repo dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(repoDir)
		os.RemoveAll(remoteDir)
	})

	// Bare git remote.
	gitCmd := exec.Command("git", "init", "--bare", remoteDir)
	if out, err := gitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v\n%s", err, out)
	}

	// Init jj repo.
	cmd := exec.Command("jj", "git", "init", repoDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("jj git init: %v\n%s", err, out)
	}

	jjRun(t, repoDir, "config", "set", "--repo", "user.email", "test@jip.dev")
	jjRun(t, repoDir, "config", "set", "--repo", "user.name", "Test User")
	jjRun(t, repoDir, "git", "remote", "add", "origin", remoteDir)

	// Initial commit and push.
	writeAndCommit(t, repoDir, "README.md", "# test repo", "initial commit")
	jjRun(t, repoDir, "bookmark", "set", "main", "-r", "@-")
	jjRun(t, repoDir, "git", "push", "--bookmark", "main")

	return repoDir, remoteDir
}

func jjRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("jj", append([]string{"-R", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("jj %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func getChangeID(t *testing.T, dir, rev string) string {
	t.Helper()
	out := jjRun(t, dir, "log", "--no-graph", "-r", rev, "-T", "change_id")
	return strings.TrimSpace(out)
}

func assertPRRefsInBody(t *testing.T, pr *gh.PRInfo, shouldRef, shouldNotRef []*gh.PRInfo) {
	t.Helper()
	for _, ref := range shouldRef {
		s := fmt.Sprintf("#%d", ref.Number)
		if !strings.Contains(pr.Body, s) {
			t.Errorf("PR #%d (%s) body should reference %s but doesn't:\n%s",
				pr.Number, pr.Title, s, pr.Body)
		}
	}
	for _, ref := range shouldNotRef {
		s := fmt.Sprintf("#%d", ref.Number)
		if strings.Contains(pr.Body, s) {
			t.Errorf("PR #%d (%s) body should NOT reference %s but does:\n%s",
				pr.Number, pr.Title, s, pr.Body)
		}
	}
}

func writeAndCommit(t *testing.T, dir, filename, content, message string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatalf("writing %s: %v", filename, err)
	}
	jjRun(t, dir, "commit", "-m", message)
}
