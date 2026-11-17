//go:build integration

package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gh "github.com/omarkohl/jip/internal/github"
	"github.com/omarkohl/jip/internal/jj"
)

func TestIntegration_SendTemplateAndOutsideEdits(t *testing.T) {
	checkJJ(t)
	dir, _ := initTestRepoWithRemote(t)
	if err := os.MkdirAll(filepath.Join(dir, ".github"), 0755); err != nil {
		t.Fatal(err)
	}
	writeAndCommit(t, dir, ".github/PULL_REQUEST_TEMPLATE.md", "## Checklist\n\n- [ ] Tested\n", "add template")
	jjRun(t, dir, "bookmark", "set", "main", "-r", "@-")
	jjRun(t, dir, "git", "push", "--bookmark", "main")
	writeAndCommit(t, dir, "feature.go", "package feature\n", "feat: add feature\n\nCommit description.")
	// An uncommitted template must not replace the template in the base.
	if err := os.WriteFile(filepath.Join(dir, ".github/PULL_REQUEST_TEMPLATE.md"), []byte("WRONG TEMPLATE"), 0644); err != nil {
		t.Fatal(err)
	}
	mock := newMockService()
	runner := jj.NewRunner(dir)
	opts := sendOpts{base: "main", remote: "origin", revsets: []string{"@-"}}
	var out bytes.Buffer
	if err := executeSend(runner, mock, opts, &out); err != nil {
		t.Fatalf("send: %v\n%s", err, &out)
	}
	pr := mock.prs[1]
	if !strings.HasPrefix(pr.Body, "## Checklist\n\n- [ ] Tested\n") || strings.Contains(pr.Body, "WRONG TEMPLATE") || !strings.Contains(pr.Body, "Commit description.") {
		t.Fatalf("wrong initial body: %s", pr.Body)
	}
	pr.Body = strings.Replace(pr.Body, "[ ]", "[x]", 1) + "\n\nAfter section.  \n"
	want := pr.Body
	if err := executeSend(runner, mock, opts, &out); err != nil {
		t.Fatal(err)
	}
	if pr.Body != want {
		t.Fatalf("outside text changed:\ngot %q\nwant %q", pr.Body, want)
	}
}

func TestIntegration_SendAdoptedPR(t *testing.T) {
	checkJJ(t)
	dir, _ := initTestRepoWithRemote(t)
	writeAndCommit(t, dir, "feature.go", "package feature\n", "feat: local title\n\nLocal description.")
	jjRun(t, dir, "bookmark", "set", "feature", "-r", "@-")
	jjRun(t, dir, "git", "push", "--bookmark", "feature")
	mock := newMockService()
	pr, err := mock.CreatePR("feature", "main", "Someone else's title", "Someone else's description.\n\n- [x] Reviewed", false)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := executeSend(jj.NewRunner(dir), mock, sendOpts{base: "main", remote: "origin", revsets: []string{"feature"}}, &out); err != nil {
		t.Fatal(err)
	}
	if pr.Title != "feat: local title" || !strings.HasPrefix(pr.Body, "Someone else's description.\n\n- [x] Reviewed\n\n"+gh.SectionBegin) || !strings.Contains(pr.Body, "Local description.") {
		t.Fatalf("wrong adopted PR: %+v", pr)
	}
}

func TestIntegration_SendPreservesFencedExamples(t *testing.T) {
	checkJJ(t)
	dir, _ := initTestRepoWithRemote(t)
	template := "## Example\n\n```markdown\n" + gh.BuildStackedPRBody("aaaaaaa", "o/r", 1, []int{1, 2}, "example") + "\n```\n"
	writeAndCommit(t, dir, "pull_request_template.md", template, "add template example")
	jjRun(t, dir, "bookmark", "set", "main", "-r", "@-")
	jjRun(t, dir, "git", "push", "--bookmark", "main")
	description := "Metadata example:\n\n```html\n<!-- jip:pushed-commit=bbbbbbb -->\n```"
	writeAndCommit(t, dir, "feature.go", "package feature\n", "feat: example\n\n"+description)
	mock := newMockService()
	runner := jj.NewRunner(dir)
	opts := sendOpts{base: "main", remote: "origin", revsets: []string{"@-"}}
	var out bytes.Buffer
	for range 2 {
		if err := executeSend(runner, mock, opts, &out); err != nil {
			t.Fatalf("send: %v\n%s", err, &out)
		}
		body := mock.prs[1].Body
		if !strings.HasPrefix(body, template) || !strings.Contains(body, description) {
			t.Fatalf("fenced examples changed: %s", body)
		}
		if got, want := gh.ParsePushedCommit(body), getCommitID(t, dir, "@-"); got != want {
			t.Fatalf("pushed commit = %q, want %q", got, want)
		}
	}
}

func TestIntegration_SendBodyConflictsBeforePush(t *testing.T) {
	checkJJ(t)
	for _, kind := range []string{"broken", "legacy"} {
		for _, decision := range []string{"noninteractive", "abort", "preserve", "overwrite", "skip", "overwrite-flag", "dry-overwrite-flag"} {
			t.Run(kind+"/"+decision, func(t *testing.T) {
				dir, _ := initTestRepoWithRemote(t)
				writeAndCommit(t, dir, "feature.go", "package feature\n", "feat: feature\n\nCurrent description.")
				jjRun(t, dir, "bookmark", "set", "feature", "-r", "@-")
				jjRun(t, dir, "git", "push", "--bookmark", "feature")
				oldCommit := getCommitID(t, dir, "feature@origin")
				mock := newMockService()
				oldBody := "Keep this checklist\n\n- [x] Tested\n\n" + gh.SectionBegin
				if kind == "legacy" {
					oldBody = gh.WithPushedCommitMarker("Keep this checklist\n\n- [x] Tested", oldCommit)
				}
				pr, err := mock.CreatePR("feature", "main", "old title", oldBody, false)
				if err != nil {
					t.Fatal(err)
				}
				change := getChangeID(t, dir, "feature")
				editFile(t, dir, change, "feature.go", "package feature\nconst V = 2\n")
				opts := sendOpts{base: "main", remote: "origin", revsets: []string{"feature"}}
				if decision == "overwrite-flag" || decision == "dry-overwrite-flag" {
					opts.overwritePRBody = true
					opts.dryRun = decision == "dry-overwrite-flag"
				} else if decision != "noninteractive" {
					opts.resolveBody = func(int, string) (gh.BodyResolution, error) { return gh.BodyResolution(decision), nil }
				}
				var out bytes.Buffer
				err = executeSend(jj.NewRunner(dir), mock, opts, &out)
				success := decision == "preserve" || decision == "overwrite" || decision == "overwrite-flag" || decision == "dry-overwrite-flag"
				if (err == nil) != success {
					t.Fatalf("unexpected error: %v\n%s", err, &out)
				}
				if !success || opts.dryRun {
					if pr.Body != oldBody || pr.Title != "old title" || getCommitID(t, dir, "feature@origin") != oldCommit {
						t.Fatal("remote changed before conflict was resolved")
					}
					return
				}
				if gh.PRBodyConflict(pr.Body) != "" || strings.Count(pr.Body, gh.SectionBegin) != 1 || !strings.Contains(pr.Body, "Current description.") {
					t.Fatalf("bad resolved body: %s", pr.Body)
				}
				if strings.Contains(pr.Body, "Keep this checklist") != (decision == "preserve") {
					t.Fatalf("wrong preservation result: %s", pr.Body)
				}
				if err := executeSend(jj.NewRunner(dir), mock, opts, &out); err != nil {
					t.Fatalf("resolved body failed on resend: %v", err)
				}
			})
		}
	}
}

func TestIntegration_SendBodySkipKeepsIndependentChanges(t *testing.T) {
	checkJJ(t)
	dir, _ := initTestRepoWithRemote(t)
	writeAndCommit(t, dir, "a.go", "package a\n", "feat: A")
	a := getChangeID(t, dir, "@-")
	jjRun(t, dir, "bookmark", "set", "feature-a", "-r", a)
	jjRun(t, dir, "git", "push", "--bookmark", "feature-a")
	writeAndCommit(t, dir, "b.go", "package b\n", "feat: B")
	b := getChangeID(t, dir, "@-")
	jjRun(t, dir, "new", "main")
	writeAndCommit(t, dir, "c.go", "package c\n", "feat: C")
	c := getChangeID(t, dir, "@-")
	mock := newMockService()
	_, err := mock.CreatePR("feature-a", "main", "A", gh.SectionBegin, false)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err = executeSend(jj.NewRunner(dir), mock, sendOpts{
		base: "main", remote: "origin", revsets: []string{b, c},
		resolveBody: func(int, string) (gh.BodyResolution, error) { return gh.BodySkip, nil },
	}, &out)
	if err == nil || !strings.Contains(err.Error(), "2 change(s) skipped") {
		t.Fatalf("expected A and B skipped, got %v\n%s", err, &out)
	}
	if len(mock.prs) != 2 || mock.prs[2].Title != "feat: C" || mock.prs[1].Body != gh.SectionBegin {
		t.Fatalf("unexpected PRs: %+v\n%s", mock.prs, &out)
	}
}

func TestIntegration_SendTemplateReadErrorBeforePush(t *testing.T) {
	checkJJ(t)
	dir, _ := initTestRepoWithRemote(t)
	writeAndCommit(t, dir, "feature.go", "package feature\n", "feat: feature")
	runner := &templateErrorRunner{Runner: jj.NewRunner(dir)}
	var out bytes.Buffer
	err := executeSend(runner, newMockService(), sendOpts{base: "main", remote: "origin", revsets: []string{"@-"}, overwritePRBody: true}, &out)
	if err == nil || !strings.Contains(err.Error(), "cannot read template") || strings.Contains(out.String(), "Pushing") {
		t.Fatalf("template failure ignored: %v\n%s", err, &out)
	}
}

type templateErrorRunner struct{ jj.Runner }

func (*templateErrorRunner) PRTemplate(string) (string, error) {
	return "", fmt.Errorf("cannot read template")
}
