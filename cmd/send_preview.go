package cmd

import (
	"fmt"
	"io"
	"slices"

	gh "github.com/omarkohl/jip/internal/github"
	"github.com/omarkohl/jip/internal/jj"
)

// Preview uses the normal send pipeline, but replaces every mutation with a
// recorded action. Reads still use jj and GitHub. This keeps the plan aligned
// with title/body/comment and native-stack reconciliation logic.
type previewRunner struct {
	jj.Runner
	w io.Writer
}

func (r *previewRunner) GitFetch(remote string) error {
	_, _ = fmt.Fprintf(r.w, "  SKIP fetch %s (preview uses cached refs)\n", remote)
	return nil
}

func (r *previewRunner) BookmarkSet(name, rev string) error {
	_, _ = fmt.Fprintf(r.w, "  CREATE bookmark %q at %s\n", name, rev)
	return nil
}

func (r *previewRunner) GitPush(bookmarks []string, remote string) error {
	for _, name := range bookmarks {
		_, _ = fmt.Fprintf(r.w, "  PUSH bookmark %q to %s (jj checks remote safety; unchanged refs are no-ops)\n", name, remote)
	}
	return nil
}

func (r *previewRunner) Rebase(_ []string, _ string) error {
	return fmt.Errorf("cannot rebase during dry-run")
}

type previewService struct {
	gh.Service
	w         io.Writer
	nextPR    int
	nextStack int
	stacks    map[int]*gh.Stack
	removed   map[int]bool
}

func newPreviewService(service gh.Service, w io.Writer) *previewService {
	return &previewService{Service: service, w: w, nextPR: -1, nextStack: -1, stacks: make(map[int]*gh.Stack), removed: make(map[int]bool)}
}

func (s *previewService) LookupPRsByBranch(branches []string) (map[string]*gh.PRInfo, error) {
	prs, err := s.Service.LookupPRsByBranch(branches)
	if err != nil {
		return nil, err
	}
	result := make(map[string]*gh.PRInfo, len(prs))
	for name, pr := range prs {
		if pr != nil {
			copy := *pr
			result[name] = &copy
		}
	}
	return result, nil
}

func (s *previewService) CreatePR(head, base, title, body string, draft bool) (*gh.PRInfo, error) {
	number := s.nextPR
	s.nextPR--
	_, _ = fmt.Fprintf(s.w, "  CREATE PR #%d (placeholder) head=%q base=%q draft=%t\n    title: %s\n    body:\n%s\n", number, head, base, draft, title, body)
	return &gh.PRInfo{Number: number, State: "OPEN", HeadRefName: head, BaseRefName: base, Title: title, Body: body, IsDraft: draft}, nil
}

func (s *previewService) UpdatePR(number int, opts gh.UpdatePROpts) error {
	_, _ = fmt.Fprintf(s.w, "  UPDATE PR #%d\n", number)
	if opts.Title != nil {
		_, _ = fmt.Fprintf(s.w, "    title: %s\n", *opts.Title)
	}
	if opts.Base != nil {
		_, _ = fmt.Fprintf(s.w, "    base: %s\n", *opts.Base)
	}
	if opts.Body != nil {
		_, _ = fmt.Fprintf(s.w, "    body:\n%s\n", *opts.Body)
	}
	return nil
}

func (s *previewService) CommentOnPR(number int, body string) error {
	_, _ = fmt.Fprintf(s.w, "  COMMENT on PR #%d:\n%s\n", number, body)
	return nil
}

func (s *previewService) RequestReviewers(number int, reviewers []string) error {
	_, _ = fmt.Fprintf(s.w, "  REQUEST reviewers on PR #%d: %v\n", number, reviewers)
	return nil
}

func (s *previewService) FindStackForPR(number int) (*gh.Stack, error) {
	st, err := s.Service.FindStackForPR(number)
	if err != nil || st == nil {
		return st, err
	}
	if s.removed[st.Number] {
		return nil, nil
	}
	if copy, ok := s.stacks[st.Number]; ok {
		return copy, nil
	}
	copy := *st
	copy.PullRequests = slices.Clone(st.PullRequests)
	s.stacks[st.Number] = &copy
	return &copy, nil
}

func (s *previewService) Unstack(number int) (bool, error) {
	_, _ = fmt.Fprintf(s.w, "  DISSOLVE GitHub stack #%d (subject to GitHub merge-queue/auto-merge restrictions)\n", number)
	s.removed[number] = true
	return true, nil
}

func (s *previewService) CreateStack(numbers []int) (*gh.Stack, error) {
	number := s.nextStack
	s.nextStack--
	_, _ = fmt.Fprintf(s.w, "  CREATE GitHub stack #%d (placeholder), PRs bottom-to-top: %v\n", number, numbers)
	return &gh.Stack{Number: number}, nil
}

func (s *previewService) AddToStack(number int, numbers []int) (*gh.Stack, error) {
	_, _ = fmt.Fprintf(s.w, "  APPEND PRs %v to GitHub stack #%d\n", numbers, number)
	return s.stacks[number], nil
}
