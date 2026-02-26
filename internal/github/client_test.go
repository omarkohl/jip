package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreatePR(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v3/repos/owner/repo/pulls", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)

		if req["title"] != "feat: my change" {
			t.Errorf("unexpected title: %v", req["title"])
		}
		if req["head"] != "jip/user/my-change/abc123" {
			t.Errorf("unexpected head: %v", req["head"])
		}
		if req["base"] != "main" {
			t.Errorf("unexpected base: %v", req["base"])
		}
		if req["draft"] != true {
			t.Errorf("expected draft=true, got %v", req["draft"])
		}

		w.WriteHeader(201)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"number":   42,
			"state":    "open",
			"html_url": "https://github.com/owner/repo/pull/42",
			"title":    req["title"],
			"body":     req["body"],
			"draft":    true,
			"head":     map[string]any{"ref": req["head"]},
			"base":     map[string]any{"ref": req["base"]},
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server, "owner", "repo")
	pr, err := client.CreatePR("jip/user/my-change/abc123", "main", "feat: my change", "body text", true)
	if err != nil {
		t.Fatalf("CreatePR: %v", err)
	}
	if pr.Number != 42 {
		t.Errorf("expected PR #42, got #%d", pr.Number)
	}
	if pr.IsDraft != true {
		t.Error("expected draft PR")
	}
}

func TestUpdatePR(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /api/v3/repos/owner/repo/pulls/10", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)

		if req["title"] != "updated title" {
			t.Errorf("unexpected title: %v", req["title"])
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"number": 10,
			"title":  req["title"],
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server, "owner", "repo")
	title := "updated title"
	err := client.UpdatePR(10, UpdatePROpts{Title: &title})
	if err != nil {
		t.Fatalf("UpdatePR: %v", err)
	}
}

func TestCommentOnPR(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v3/repos/owner/repo/issues/5/comments", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)

		if req["body"] != "test comment" {
			t.Errorf("unexpected comment body: %v", req["body"])
		}

		w.WriteHeader(201)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":   1,
			"body": req["body"],
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server, "owner", "repo")
	err := client.CommentOnPR(5, "test comment")
	if err != nil {
		t.Fatalf("CommentOnPR: %v", err)
	}
}

func TestRequestReviewers(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v3/repos/owner/repo/pulls/7/requested_reviewers", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)

		reviewers := req["reviewers"].([]any)
		if len(reviewers) != 2 {
			t.Errorf("expected 2 reviewers, got %d", len(reviewers))
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"number": 7,
		})
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server, "owner", "repo")
	err := client.RequestReviewers(7, []string{"alice", "bob"})
	if err != nil {
		t.Fatalf("RequestReviewers: %v", err)
	}
}

// newTestClient creates a Client pointed at a test server.
func newTestClient(t *testing.T, server *httptest.Server, owner, repo string) *Client {
	t.Helper()
	remoteURL := fmt.Sprintf("https://github.com/%s/%s", owner, repo)
	client, err := NewClient("test-token", remoteURL, server.URL+"/")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return client
}
