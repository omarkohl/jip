package github

import (
	"strings"
	"testing"
)

func TestParseRepoFromURL_HTTPS(t *testing.T) {
	tests := []struct {
		url       string
		wantOwner string
		wantRepo  string
	}{
		{"https://github.com/owner/repo.git", "owner", "repo"},
		{"https://github.com/owner/repo", "owner", "repo"},
		{"https://github.com/example-org/leafy.git", "example-org", "leafy"},
		{"http://github.com/owner/repo.git", "owner", "repo"},
	}
	for _, tt := range tests {
		owner, repo, err := ParseRepoFromURL(tt.url)
		if err != nil {
			t.Errorf("ParseRepoFromURL(%q): unexpected error: %v", tt.url, err)
			continue
		}
		if owner != tt.wantOwner || repo != tt.wantRepo {
			t.Errorf("ParseRepoFromURL(%q) = (%q, %q), want (%q, %q)",
				tt.url, owner, repo, tt.wantOwner, tt.wantRepo)
		}
	}
}

func TestParseRepoFromURL_SSH(t *testing.T) {
	tests := []struct {
		url       string
		wantOwner string
		wantRepo  string
	}{
		{"git@github.com:owner/repo.git", "owner", "repo"},
		{"git@github.com:owner/repo", "owner", "repo"},
		{"git@github.com:example-org/leafy.git", "example-org", "leafy"},
	}
	for _, tt := range tests {
		owner, repo, err := ParseRepoFromURL(tt.url)
		if err != nil {
			t.Errorf("ParseRepoFromURL(%q): unexpected error: %v", tt.url, err)
			continue
		}
		if owner != tt.wantOwner || repo != tt.wantRepo {
			t.Errorf("ParseRepoFromURL(%q) = (%q, %q), want (%q, %q)",
				tt.url, owner, repo, tt.wantOwner, tt.wantRepo)
		}
	}
}

func TestParseRepoFromURL_Invalid(t *testing.T) {
	invalids := []string{
		"",
		"not-a-url",
		"ftp://example.com/owner/repo",
		"/local/path",
	}
	for _, url := range invalids {
		_, _, err := ParseRepoFromURL(url)
		if err == nil {
			t.Errorf("ParseRepoFromURL(%q): expected error, got nil", url)
		}
	}
}

func TestParseRepoFromURL_RejectsNonGitHubHost(t *testing.T) {
	urls := []string{
		"https://ghes.corp/acme/app.git",
		"https://github.example.com/acme/app",
		"git@ghes.corp:acme/app.git",
	}
	for _, raw := range urls {
		_, _, err := ParseRepoFromURL(raw)
		if err == nil {
			t.Errorf("ParseRepoFromURL(%q): expected error, got nil", raw)
			continue
		}
		if !strings.Contains(err.Error(), "not supported") || !strings.Contains(err.Error(), "github.com only") {
			t.Errorf("ParseRepoFromURL(%q): error %q should mention github.com only", raw, err)
		}
		if strings.Contains(err.Error(), "cannot parse") {
			t.Errorf("ParseRepoFromURL(%q): should reject host, not fail to parse: %v", raw, err)
		}
	}
}

func TestParseRepoFromURL_WhitespaceHandling(t *testing.T) {
	owner, repo, err := ParseRepoFromURL("  https://github.com/owner/repo.git  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if owner != "owner" || repo != "repo" {
		t.Errorf("got (%q, %q), want (\"owner\", \"repo\")", owner, repo)
	}
}
