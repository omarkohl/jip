package jj

import (
	"testing"
)

func TestParseRemoteList_Basic(t *testing.T) {
	data := "origin https://github.com/owner/repo.git\n"
	remotes := ParseRemoteList([]byte(data))
	if len(remotes) != 1 {
		t.Fatalf("expected 1 remote, got %d", len(remotes))
	}
	if remotes["origin"] != "https://github.com/owner/repo.git" {
		t.Errorf("expected origin URL, got %q", remotes["origin"])
	}
}

func TestParseRemoteList_MultipleRemotes(t *testing.T) {
	data := "origin git@github.com:owner/repo.git\nupstream https://github.com/upstream/repo.git\n"
	remotes := ParseRemoteList([]byte(data))
	if len(remotes) != 2 {
		t.Fatalf("expected 2 remotes, got %d", len(remotes))
	}
	if remotes["origin"] != "git@github.com:owner/repo.git" {
		t.Errorf("origin: got %q", remotes["origin"])
	}
	if remotes["upstream"] != "https://github.com/upstream/repo.git" {
		t.Errorf("upstream: got %q", remotes["upstream"])
	}
}

func TestParseRemoteList_Empty(t *testing.T) {
	remotes := ParseRemoteList([]byte(""))
	if len(remotes) != 0 {
		t.Errorf("expected 0 remotes, got %d", len(remotes))
	}
}

func TestParseRemoteList_BlankLines(t *testing.T) {
	data := "origin https://example.com/repo.git\n\n\n"
	remotes := ParseRemoteList([]byte(data))
	if len(remotes) != 1 {
		t.Fatalf("expected 1 remote, got %d", len(remotes))
	}
}
