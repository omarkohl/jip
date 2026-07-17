package github

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	gogithub "github.com/google/go-github/v68/github"

	"github.com/omarkohl/jip/internal/retry"
)

// The Stacks REST API (repos/{owner}/{repo}/stacks...) backs GitHub's native
// stacked-PRs private preview. It is not yet documented on docs.github.com;
// endpoints and payload shapes mirror what the official gh-stack CLI
// extension uses and may change before general availability.

// StackPR is a pull request entry within a native GitHub stack.
type StackPR struct {
	Number   int     `json:"number"`
	State    string  `json:"state"` // "open" or "closed"
	Draft    bool    `json:"draft"`
	MergedAt *string `json:"merged_at"`
}

// Merged reports whether the pull request has been merged.
func (p StackPR) Merged() bool { return p.MergedAt != nil && *p.MergedAt != "" }

// StackBase describes the base ref of a stack.
type StackBase struct {
	Ref string `json:"ref"`
}

// Stack is a native GitHub stack of pull requests. Number is the human-facing
// stack number used in API paths; PullRequests is ordered bottom to top.
type Stack struct {
	Number       int       `json:"number"`
	URL          string    `json:"url"`
	Open         bool      `json:"open"`
	Base         StackBase `json:"base"`
	PullRequests []StackPR `json:"pull_requests"`
}

// OpenPRNumbers returns the numbers of the stack's open, unmerged PRs, bottom
// to top. Merged PRs stay listed in a stack after a partial merge, so callers
// comparing against a local stack must ignore them.
func (s *Stack) OpenPRNumbers() []int {
	var nums []int
	for _, p := range s.PullRequests {
		if p.State == "open" && !p.Merged() {
			nums = append(nums, p.Number)
		}
	}
	return nums
}

func (c *Client) stacksPath() string {
	return fmt.Sprintf("repos/%s/%s/stacks", c.owner, c.repo)
}

// isNotFound reports whether err is a GitHub API 404 response.
func isNotFound(err error) bool {
	var ghErr *gogithub.ErrorResponse
	return errors.As(err, &ghErr) && ghErr.Response != nil &&
		ghErr.Response.StatusCode == http.StatusNotFound
}

// StacksEnabled reports whether the stacked-PRs preview is enabled for the
// repository. The stacks endpoints answer 404 when it is not.
func (c *Client) StacksEnabled() (bool, error) {
	slog.Debug("StacksEnabled")
	enabled := true
	err := retry.Do(func() error {
		req, err := c.gh.NewRequest(http.MethodGet, c.stacksPath(), nil)
		if err != nil {
			return err
		}
		var stacks []Stack
		_, apiErr := c.gh.Do(context.Background(), req, &stacks)
		if isNotFound(apiErr) {
			enabled = false // a 404 is an answer, not a transient failure
			return nil
		}
		return apiErr
	})
	if err != nil {
		slog.Debug("StacksEnabled failed", "err", err)
		return false, fmt.Errorf("checking stacked-PRs availability: %w", err)
	}
	slog.Debug("StacksEnabled ok", "enabled", enabled)
	return enabled, nil
}

// FindStackForPR returns the stack containing the given PR, or nil when the
// PR is not part of any stack.
func (c *Client) FindStackForPR(number int) (*Stack, error) {
	slog.Debug("FindStackForPR", "number", number)
	var stacks []Stack
	err := retry.Do(func() error {
		path := fmt.Sprintf("%s?pull_request=%d", c.stacksPath(), number)
		req, err := c.gh.NewRequest(http.MethodGet, path, nil)
		if err != nil {
			return err
		}
		_, apiErr := c.gh.Do(context.Background(), req, &stacks)
		return apiErr
	})
	if err != nil {
		slog.Debug("FindStackForPR failed", "number", number, "err", err)
		return nil, fmt.Errorf("finding stack for PR #%d: %w", number, err)
	}
	if len(stacks) == 0 {
		return nil, nil
	}
	return &stacks[0], nil
}

// stackRequest is the payload for stack create/add calls.
type stackRequest struct {
	PullRequests []int `json:"pull_requests"`
}

// CreateStack creates a native GitHub stack from PR numbers ordered bottom to
// top. The PRs must already form a valid base-to-head chain (each PR based on
// the head branch of the one below), and there must be at least two.
func (c *Client) CreateStack(prNumbers []int) (*Stack, error) {
	slog.Debug("CreateStack", "prs", prNumbers)
	var stack Stack
	err := retry.Do(func() error {
		req, err := c.gh.NewRequest(http.MethodPost, c.stacksPath(), stackRequest{PullRequests: prNumbers})
		if err != nil {
			return err
		}
		_, apiErr := c.gh.Do(context.Background(), req, &stack)
		return apiErr
	})
	if err != nil {
		slog.Debug("CreateStack failed", "err", err)
		return nil, fmt.Errorf("creating stack from PRs %v: %w", prNumbers, err)
	}
	slog.Debug("CreateStack ok", "number", stack.Number)
	return &stack, nil
}

// AddToStack appends pull requests to the top of an existing stack. Only the
// new PR numbers (the delta) are given, ordered from the current top upward.
// The stacks API is append-only: reordering or mid-stack changes require
// Unstack followed by CreateStack.
func (c *Client) AddToStack(stackNumber int, prNumbers []int) (*Stack, error) {
	slog.Debug("AddToStack", "stack", stackNumber, "prs", prNumbers)
	var stack Stack
	err := retry.Do(func() error {
		path := fmt.Sprintf("%s/%d/add", c.stacksPath(), stackNumber)
		req, err := c.gh.NewRequest(http.MethodPost, path, stackRequest{PullRequests: prNumbers})
		if err != nil {
			return err
		}
		_, apiErr := c.gh.Do(context.Background(), req, &stack)
		return apiErr
	})
	if err != nil {
		slog.Debug("AddToStack failed", "stack", stackNumber, "err", err)
		return nil, fmt.Errorf("adding PRs %v to stack #%d: %w", prNumbers, stackNumber, err)
	}
	return &stack, nil
}

// Unstack removes the pull requests from a stack, dissolving it. The PRs
// themselves survive. PRs queued for merge or with auto-merge enabled cannot
// be removed; dissolved is false when any remain (HTTP 200 with the remaining
// stack instead of 204).
func (c *Client) Unstack(stackNumber int) (dissolved bool, err error) {
	slog.Debug("Unstack", "stack", stackNumber)
	err = retry.Do(func() error {
		path := fmt.Sprintf("%s/%d/unstack", c.stacksPath(), stackNumber)
		req, reqErr := c.gh.NewRequest(http.MethodPost, path, nil)
		if reqErr != nil {
			return reqErr
		}
		var remaining Stack
		resp, apiErr := c.gh.Do(context.Background(), req, &remaining)
		if apiErr != nil {
			return apiErr
		}
		dissolved = resp.StatusCode == http.StatusNoContent || len(remaining.PullRequests) == 0
		return nil
	})
	if err != nil {
		slog.Debug("Unstack failed", "stack", stackNumber, "err", err)
		return false, fmt.Errorf("unstacking stack #%d: %w", stackNumber, err)
	}
	slog.Debug("Unstack ok", "stack", stackNumber, "dissolved", dissolved)
	return dissolved, nil
}
