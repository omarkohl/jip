package github

import (
	"context"
	"fmt"
	"strings"

	gogithub "github.com/google/go-github/v68/github"

	"github.com/omarkohl/jip/internal/retry"
)

// Service defines the GitHub operations needed by the send pipeline.
type Service interface {
	CreatePR(head, base, title, body string, draft bool) (*PRInfo, error)
	UpdatePR(number int, opts UpdatePROpts) error
	CommentOnPR(number int, body string) error
	GetAuthenticatedUser() (string, error)
	RequestReviewers(number int, reviewers []string) error
	LookupPRsByBranch(branches []string) (map[string]*PRInfo, error)
	Owner() string
	Repo() string
}

// Client wraps go-github for PR mutations and GraphQL queries.
type Client struct {
	gh         *gogithub.Client
	owner      string
	repo       string
	token      string
	graphqlURL string
}

// NewClient creates a GitHub client for the given repository.
// remoteURL is the git remote URL (e.g. https://github.com/owner/repo.git),
// from which owner and repo are parsed.
// If apiURL is non-empty, it is used as the GitHub API base URL
// (for GitHub Enterprise or testing).
func NewClient(token, remoteURL, apiURL string) (*Client, error) {
	owner, repo, err := ParseRepoFromURL(remoteURL)
	if err != nil {
		return nil, fmt.Errorf("parsing remote URL: %w", err)
	}

	gh := gogithub.NewClient(nil).WithAuthToken(token)
	if apiURL != "" {
		gh, _ = gh.WithEnterpriseURLs(apiURL, apiURL)
	}

	graphqlURL := "https://api.github.com/graphql"
	if apiURL != "" {
		graphqlURL = strings.TrimSuffix(apiURL, "/") + "/graphql"
	}

	return &Client{
		gh:         gh,
		owner:      owner,
		repo:       repo,
		token:      token,
		graphqlURL: graphqlURL,
	}, nil
}

// Owner returns the repository owner.
func (c *Client) Owner() string { return c.owner }

// Repo returns the repository name.
func (c *Client) Repo() string { return c.repo }

// UpdatePROpts contains optional fields for updating a PR.
type UpdatePROpts struct {
	Title *string
	Body  *string
	Base  *string
	Draft *bool
}

// CreatePR creates a new pull request and returns its info.
func (c *Client) CreatePR(head, base, title, body string, draft bool) (*PRInfo, error) {
	var pr *gogithub.PullRequest
	err := retry.Do(func() error {
		var apiErr error
		pr, _, apiErr = c.gh.PullRequests.Create(context.Background(), c.owner, c.repo, &gogithub.NewPullRequest{
			Title: &title,
			Head:  &head,
			Base:  &base,
			Body:  &body,
			Draft: &draft,
		})
		return apiErr
	})
	if err != nil {
		return nil, fmt.Errorf("creating PR: %w", err)
	}
	return &PRInfo{
		Number:      pr.GetNumber(),
		State:       pr.GetState(),
		URL:         pr.GetHTMLURL(),
		Title:       pr.GetTitle(),
		Body:        pr.GetBody(),
		HeadRefName: pr.GetHead().GetRef(),
		BaseRefName: pr.GetBase().GetRef(),
		IsDraft:     pr.GetDraft(),
	}, nil
}

// UpdatePR updates fields on an existing pull request.
func (c *Client) UpdatePR(number int, opts UpdatePROpts) error {
	update := &gogithub.PullRequest{}
	if opts.Title != nil {
		update.Title = opts.Title
	}
	if opts.Body != nil {
		update.Body = opts.Body
	}
	if opts.Base != nil {
		update.Base = &gogithub.PullRequestBranch{Ref: opts.Base}
	}
	err := retry.Do(func() error {
		_, _, apiErr := c.gh.PullRequests.Edit(context.Background(), c.owner, c.repo, number, update)
		return apiErr
	})
	if err != nil {
		return fmt.Errorf("updating PR #%d: %w", number, err)
	}
	return nil
}

// CommentOnPR posts a comment on a pull request.
func (c *Client) CommentOnPR(number int, body string) error {
	err := retry.Do(func() error {
		_, _, apiErr := c.gh.Issues.CreateComment(context.Background(), c.owner, c.repo, number, &gogithub.IssueComment{
			Body: &body,
		})
		return apiErr
	})
	if err != nil {
		return fmt.Errorf("commenting on PR #%d: %w", number, err)
	}
	return nil
}

// GetAuthenticatedUser returns the login of the authenticated user.
func (c *Client) GetAuthenticatedUser() (string, error) {
	var user *gogithub.User
	err := retry.Do(func() error {
		var apiErr error
		user, _, apiErr = c.gh.Users.Get(context.Background(), "")
		return apiErr
	})
	if err != nil {
		return "", fmt.Errorf("getting authenticated user: %w", err)
	}
	return user.GetLogin(), nil
}

// RequestReviewers adds reviewers to a pull request.
func (c *Client) RequestReviewers(number int, reviewers []string) error {
	err := retry.Do(func() error {
		_, _, apiErr := c.gh.PullRequests.RequestReviewers(context.Background(), c.owner, c.repo, number, gogithub.ReviewersRequest{
			Reviewers: reviewers,
		})
		return apiErr
	})
	if err != nil {
		return fmt.Errorf("requesting reviewers on PR #%d: %w", number, err)
	}
	return nil
}
