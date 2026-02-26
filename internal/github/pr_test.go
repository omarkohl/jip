package github

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testAPIResponse = `{
  "data": {
    "repository": {
      "b0": {
        "nodes": [
          {
            "number": 10,
            "state": "OPEN",
            "url": "https://github.com/acme-corp/widgets/pull/10",
            "title": "feat: add widget factory",
            "headRefName": "jip/alice/add-widget-factory/aabbccddee01",
            "baseRefName": "main",
            "isDraft": false
          }
        ]
      },
      "b1": {
        "nodes": [
          {
            "number": 17,
            "state": "OPEN",
            "url": "https://github.com/acme-corp/widgets/pull/17",
            "title": "fix: handle nil pointer in widget renderer",
            "headRefName": "jip/alice/handle-nil-pointer/ffeeddccbb02",
            "baseRefName": "main",
            "isDraft": false
          }
        ]
      }
    }
  }
}`

// newGraphQLTestClient creates a Client whose GraphQL URL points at the given
// test server, with the specified owner/repo.
func newGraphQLTestClient(t *testing.T, server *httptest.Server, owner, repo string) *Client {
	t.Helper()
	remoteURL := fmt.Sprintf("https://github.com/%s/%s", owner, repo)
	client, err := NewClient("test-token", remoteURL, server.URL+"/")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}

func TestLookupPRsByBranch_MatchesBranches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "bearer test-token" {
			t.Errorf("expected 'bearer test-token', got %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("expected 'application/json', got %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(testAPIResponse))
	}))
	defer server.Close()

	client := newGraphQLTestClient(t, server, "acme-corp", "widgets")

	branches := []string{
		"jip/alice/add-widget-factory/aabbccddee01",
		"jip/alice/handle-nil-pointer/ffeeddccbb02",
	}
	prs, err := client.LookupPRsByBranch(branches)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(prs) != 2 {
		t.Fatalf("expected 2 PRs, got %d", len(prs))
	}

	pr1 := prs["jip/alice/add-widget-factory/aabbccddee01"]
	if pr1 == nil {
		t.Fatal("expected PR for add-widget-factory branch")
	}
	if pr1.Number != 10 {
		t.Errorf("expected PR #10, got #%d", pr1.Number)
	}
	if pr1.State != "OPEN" {
		t.Errorf("expected state OPEN, got %q", pr1.State)
	}
	if pr1.URL != "https://github.com/acme-corp/widgets/pull/10" {
		t.Errorf("unexpected URL: %q", pr1.URL)
	}
	if pr1.HeadRefName != "jip/alice/add-widget-factory/aabbccddee01" {
		t.Errorf("unexpected headRefName: %q", pr1.HeadRefName)
	}
	if pr1.BaseRefName != "main" {
		t.Errorf("expected baseRefName main, got %q", pr1.BaseRefName)
	}
	if pr1.IsDraft {
		t.Error("expected isDraft=false")
	}

	pr2 := prs["jip/alice/handle-nil-pointer/ffeeddccbb02"]
	if pr2 == nil {
		t.Fatal("expected PR for handle-nil-pointer branch")
	}
	if pr2.Number != 17 {
		t.Errorf("expected PR #17, got #%d", pr2.Number)
	}
	if pr2.Title != "fix: handle nil pointer in widget renderer" {
		t.Errorf("unexpected title: %q", pr2.Title)
	}
}

func TestLookupPRsByBranch_EmptyBranches(t *testing.T) {
	client, err := NewClient("token", "https://github.com/owner/repo", "")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	prs, err := client.LookupPRsByBranch(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prs) != 0 {
		t.Errorf("expected empty map, got %d entries", len(prs))
	}
}

func TestLookupPRsByBranch_NoPRsFound(t *testing.T) {
	emptyResponse := `{"data":{"repository":{"b0":{"nodes":[]}}}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(emptyResponse))
	}))
	defer server.Close()

	client := newGraphQLTestClient(t, server, "owner", "repo")
	prs, err := client.LookupPRsByBranch([]string{"no-pr-branch"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prs) != 0 {
		t.Errorf("expected 0 PRs, got %d", len(prs))
	}
}

func TestLookupPRsByBranch_MixedResults(t *testing.T) {
	response := `{"data":{"repository":{
		"b0":{"nodes":[{"number":42,"state":"OPEN","url":"https://example.com/pull/42","title":"feat: add caching","headRefName":"has-pr","baseRefName":"main","isDraft":true}]},
		"b1":{"nodes":[]}
	}}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	client := newGraphQLTestClient(t, server, "owner", "repo")
	prs, err := client.LookupPRsByBranch([]string{"has-pr", "no-pr"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(prs))
	}
	pr := prs["has-pr"]
	if pr == nil {
		t.Fatal("expected PR for 'has-pr'")
	}
	if pr.Number != 42 {
		t.Errorf("expected PR #42, got #%d", pr.Number)
	}
	if !pr.IsDraft {
		t.Error("expected isDraft=true")
	}
	if _, ok := prs["no-pr"]; ok {
		t.Error("expected no PR for 'no-pr'")
	}
}

func TestLookupPRsByBranch_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
	}))
	defer server.Close()

	client := newGraphQLTestClient(t, server, "owner", "repo")
	// Override token to test bad auth.
	client.token = "bad-token"
	_, err := client.LookupPRsByBranch([]string{"branch"})
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
}

func TestLookupPRsByBranch_GraphQLError(t *testing.T) {
	response := `{"data":null,"errors":[{"message":"Could not resolve to a Repository"}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	client := newGraphQLTestClient(t, server, "bad", "repo")
	_, err := client.LookupPRsByBranch([]string{"branch"})
	if err == nil {
		t.Fatal("expected error for GraphQL error response")
	}
}

func TestBuildPRQuery_SingleBranch(t *testing.T) {
	q := buildPRQuery([]string{"my-branch"})
	want := `query($owner:String!,$repo:String!){repository(owner:$owner,name:$repo){` +
		`b0:pullRequests(headRefName:"my-branch",first:1,states:[OPEN],orderBy:{field:UPDATED_AT,direction:DESC}){nodes{number state url title body headRefName baseRefName isDraft}}` +
		`}}`
	if q != want {
		t.Errorf("query mismatch:\ngot:  %s\nwant: %s", q, want)
	}
}

func TestBuildPRQuery_MultipleBranches(t *testing.T) {
	q := buildPRQuery([]string{"branch-a", "branch-b", "branch-c"})
	for _, alias := range []string{`b0:pullRequests(headRefName:"branch-a"`, `b1:pullRequests(headRefName:"branch-b"`, `b2:pullRequests(headRefName:"branch-c"`} {
		if !strings.Contains(q, alias) {
			t.Errorf("query missing %q:\n%s", alias, q)
		}
	}
}

func TestBuildPRQuery_EscapesQuotes(t *testing.T) {
	q := buildPRQuery([]string{`branch"with"quotes`})
	if !strings.Contains(q, `branch\"with\"quotes`) {
		t.Errorf("expected escaped quotes in query: %s", q)
	}
}
