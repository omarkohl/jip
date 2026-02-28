package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/omarkohl/jip/internal/retry"
)

// PRInfo holds the essential fields of a pull request.
type PRInfo struct {
	Number      int    `json:"number"`
	State       string `json:"state"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	HeadRefName string `json:"headRefName"`
	BaseRefName string `json:"baseRefName"`
	IsDraft     bool   `json:"isDraft"`
}

type graphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

type prNodes struct {
	Nodes []PRInfo `json:"nodes"`
}

// LookupPRsByBranch queries GitHub's GraphQL API for open PRs matching the
// given head branch names. Returns a map from branch name to PRInfo for
// branches that have an open PR.
func (c *Client) LookupPRsByBranch(branches []string) (map[string]*PRInfo, error) {
	if len(branches) == 0 {
		return map[string]*PRInfo{}, nil
	}

	query := buildPRQuery(branches)
	reqBody := graphQLRequest{
		Query: query,
		Variables: map[string]any{
			"owner": c.owner,
			"repo":  c.repo,
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequest("POST", c.graphqlURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Authorization", "bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	var resp *http.Response
	var rawBody []byte
	err = retry.Do(func() error {
		// Reset the request body for each attempt.
		req.Body = io.NopCloser(bytes.NewReader(body))

		var doErr error
		resp, doErr = http.DefaultClient.Do(req)
		if doErr != nil {
			return doErr
		}

		rawBody, doErr = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if doErr != nil {
			return doErr
		}

		// Retry on server errors (5xx); don't retry client errors (4xx).
		if resp.StatusCode >= 500 {
			return fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(rawBody))
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(rawBody))
	}

	// Parse the GraphQL response envelope.
	var result struct {
		Data struct {
			Repository map[string]prNodes
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(rawBody, &result); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("GraphQL errors: %s", result.Errors[0].Message)
	}

	out := make(map[string]*PRInfo, len(branches))
	for i, branch := range branches {
		alias := fmt.Sprintf("b%d", i)
		if nodes, ok := result.Data.Repository[alias]; ok && len(nodes.Nodes) > 0 {
			pr := nodes.Nodes[0]
			out[branch] = &pr
		}
	}

	return out, nil
}

func buildPRQuery(branches []string) string {
	var b strings.Builder
	b.WriteString("query($owner:String!,$repo:String!){repository(owner:$owner,name:$repo){")
	for i, branch := range branches {
		alias := fmt.Sprintf("b%d", i)
		escaped := strings.ReplaceAll(branch, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		fmt.Fprintf(&b,
			`%s:pullRequests(headRefName:"%s",first:1,states:[OPEN],orderBy:{field:UPDATED_AT,direction:DESC}){nodes{number state url title body headRefName baseRefName isDraft}}`,
			alias, escaped)
	}
	b.WriteString("}}")
	return b.String()
}
