package jj

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// SyncState describes how a local bookmark relates to a remote copy.
type SyncState int

const (
	SyncUnknown    SyncState = iota
	SyncInSync               // local and remote point to same commit
	SyncAhead                // local has commits not on remote (remote is behind)
	SyncBehind               // remote has commits not on local (remote is ahead)
	SyncDiverged             // both sides have unique commits
	SyncLocalOnly            // bookmark exists locally only
	SyncRemoteOnly           // bookmark exists on remote only
)

func (s SyncState) String() string {
	switch s {
	case SyncInSync:
		return "in-sync"
	case SyncAhead:
		return "ahead"
	case SyncBehind:
		return "behind"
	case SyncDiverged:
		return "diverged"
	case SyncLocalOnly:
		return "local-only"
	case SyncRemoteOnly:
		return "remote-only"
	default:
		return "unknown"
	}
}

// RemoteBookmarkState holds a bookmark's state on a specific remote.
type RemoteBookmarkState struct {
	Target  string // commit ID on remote
	Tracked bool   // whether this remote ref is tracked by jj
	Ahead   int    // commits the remote is ahead of local
	Behind  int    // commits the remote is behind local
}

// BookmarkInfo holds the full state of a named bookmark across local and remotes.
type BookmarkInfo struct {
	Name     string                         // bookmark name
	Target   string                         // local commit ID ("" if remote-only or conflicted)
	ChangeID string                         // local change ID ("" if remote-only or conflicted)
	Present  bool                           // has local target
	Conflict bool                           // bookmark is in conflicted state (multiple targets)
	Remotes  map[string]RemoteBookmarkState // remote name → state
}

// SyncWith returns the sync state of this bookmark relative to the given remote.
func (b *BookmarkInfo) SyncWith(remote string) SyncState {
	if b.Conflict {
		return SyncDiverged
	}
	rs, ok := b.Remotes[remote]
	if !ok {
		if b.Present {
			return SyncLocalOnly
		}
		return SyncUnknown
	}
	if !b.Present {
		return SyncRemoteOnly
	}
	if rs.Ahead > 0 && rs.Behind > 0 {
		return SyncDiverged
	}
	if rs.Behind > 0 {
		return SyncAhead // local is ahead (remote is behind)
	}
	if rs.Ahead > 0 {
		return SyncBehind // local is behind (remote is ahead)
	}
	return SyncInSync
}

// rawBookmarkEntry is the JSON structure from jj bookmark list template output.
type rawBookmarkEntry struct {
	Name     string  `json:"name"`
	Remote   *string `json:"remote"` // null for local entries
	Present  bool    `json:"present"`
	Conflict bool    `json:"conflict"`
	Target   string  `json:"target"`
	ChangeID string  `json:"change_id"`
	Tracked  bool    `json:"tracked"`
	Ahead    int     `json:"ahead"`
	Behind   int     `json:"behind"`
}

// ParseBookmarkList parses JSONL output from jj bookmark list --all-remotes
// into grouped BookmarkInfo entries. The internal "git" remote is filtered out.
func ParseBookmarkList(data []byte) ([]BookmarkInfo, error) {
	var entries []rawBookmarkEntry
	dec := json.NewDecoder(bytes.NewReader(data))
	for dec.More() {
		var e rawBookmarkEntry
		if err := dec.Decode(&e); err != nil {
			return nil, fmt.Errorf("parsing bookmark entry: %w", err)
		}
		entries = append(entries, e)
	}

	// Group by bookmark name, preserving order of first occurrence.
	type bookmarkState struct {
		info  BookmarkInfo
		order int
	}
	byName := make(map[string]*bookmarkState)
	orderCounter := 0

	for _, e := range entries {
		// Skip jj's internal git remote.
		if e.Remote != nil && *e.Remote == "git" {
			continue
		}

		bs, exists := byName[e.Name]
		if !exists {
			bs = &bookmarkState{
				info: BookmarkInfo{
					Name:    e.Name,
					Remotes: make(map[string]RemoteBookmarkState),
				},
				order: orderCounter,
			}
			orderCounter++
			byName[e.Name] = bs
		}

		if e.Remote == nil {
			// Local entry.
			bs.info.Present = e.Present
			bs.info.Conflict = e.Conflict
			bs.info.Target = e.Target
			bs.info.ChangeID = e.ChangeID
		} else {
			// Remote entry.
			bs.info.Remotes[*e.Remote] = RemoteBookmarkState{
				Target:  e.Target,
				Tracked: e.Tracked,
				Ahead:   e.Ahead,
				Behind:  e.Behind,
			}
			// If no local entry exists, mark as remote-only with remote's change info.
			if !bs.info.Present && bs.info.Target == "" {
				bs.info.Target = ""
				bs.info.ChangeID = ""
			}
		}
	}

	// Build result sorted by first-occurrence order.
	result := make([]BookmarkInfo, len(byName))
	for _, bs := range byName {
		result[bs.order] = bs.info
	}
	return result, nil
}

// MatchBookmarksToChanges returns a map from change ID to bookmarks that point
// to that change. Matching is done via commit ID (local target).
func MatchBookmarksToChanges(dag *ChangeDAG, bookmarks []BookmarkInfo) map[string][]*BookmarkInfo {
	// Build commit ID → change ID lookup from the DAG.
	commitToChange := make(map[string]string, len(dag.Changes))
	for _, c := range dag.Changes {
		commitToChange[c.CommitID] = c.ChangeID
	}

	result := make(map[string][]*BookmarkInfo)
	for i := range bookmarks {
		b := &bookmarks[i]
		if !b.Present || b.Target == "" {
			continue
		}
		if changeID, ok := commitToChange[b.Target]; ok {
			result[changeID] = append(result[changeID], b)
		}
	}
	return result
}

// ChangeBookmark represents the bookmark assignment for a change.
type ChangeBookmark struct {
	ChangeID  string
	Bookmark  string
	IsNew     bool      // bookmark was created (not pre-existing)
	SyncState SyncState // sync state relative to the push remote
	Conflict  bool      // bookmark has conflicting targets (true divergence)
	Displaced bool      // bookmark exists but no longer points to this change's commit
}

// EnsureBookmarks assigns a bookmark to each change in the DAG. For changes
// that already have a matching bookmark, it is reused (subject to the
// shouldUseExisting callback). For changes without a bookmark, a new one is
// created using the jip naming convention.
//
// shouldUseExisting is called for each existing bookmark on a change and returns
// true if that bookmark should be used for the PR. This is the extension point
// for GitHub API integration (e.g., checking if a PR already exists for that branch).
// If nil, all existing bookmarks are accepted.
func EnsureBookmarks(
	runner Runner,
	dag *ChangeDAG,
	bookmarks []BookmarkInfo,
	pushRemote string,
	shouldUseExisting func(changeID, bookmark string) bool,
	createNew bool,
) ([]ChangeBookmark, error) {
	matched := MatchBookmarksToChanges(dag, bookmarks)

	// Build name lookup for detecting bookmarks that exist but point to a
	// different commit (e.g. after a fetch fast-forwarded the local bookmark).
	bookmarkByName := make(map[string]*BookmarkInfo, len(bookmarks))
	for i := range bookmarks {
		bookmarkByName[bookmarks[i].Name] = &bookmarks[i]
	}

	var result []ChangeBookmark
	for _, change := range dag.Changes {
		existing := matched[change.ChangeID]

		// Try to find a usable existing bookmark.
		var chosen *BookmarkInfo
		for _, b := range existing {
			if shouldUseExisting == nil || shouldUseExisting(change.ChangeID, b.Name) {
				chosen = b
				break
			}
		}

		if chosen != nil {
			result = append(result, ChangeBookmark{
				ChangeID:  change.ChangeID,
				Bookmark:  chosen.Name,
				IsNew:     false,
				SyncState: chosen.SyncWith(pushRemote),
				Conflict:  chosen.Conflict,
			})
			continue
		}

		// No existing bookmark matched by commit ID. Generate the name and
		// check if a bookmark with that name already exists (can happen when
		// a fetch fast-forwarded the bookmark to a remote commit).
		shortID := change.ChangeID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		name := GenerateBookmarkName(change.Description, shortID)

		if bi, exists := bookmarkByName[name]; exists {
			// Bookmark exists but points to a different commit than our change.
			// This typically means a fetch fast-forwarded it to a remote commit.
			result = append(result, ChangeBookmark{
				ChangeID:  change.ChangeID,
				Bookmark:  name,
				IsNew:     false,
				SyncState: bi.SyncWith(pushRemote),
				Conflict:  bi.Conflict,
				Displaced: true,
			})
			continue
		}

		if !createNew {
			continue
		}

		if err := runner.BookmarkSet(name, change.ChangeID); err != nil {
			return nil, fmt.Errorf("creating bookmark for %s: %w", change.ChangeID, err)
		}
		result = append(result, ChangeBookmark{
			ChangeID:  change.ChangeID,
			Bookmark:  name,
			IsNew:     true,
			SyncState: SyncLocalOnly,
		})
	}
	return result, nil
}

// GenerateBookmarkName creates a bookmark name following the jip convention:
// jip/<slugified-description>/<short-change-id>
func GenerateBookmarkName(description, shortChangeID string) string {
	slug := slugify(description)
	if slug == "" {
		slug = "change"
	}
	return fmt.Sprintf("jip/%s/%s", slug, shortChangeID)
}

// conventionalPrefixRe matches conventional commit prefixes like "feat:", "fix(scope):", etc.
var conventionalPrefixRe = regexp.MustCompile(`^[a-zA-Z]+(\([^)]*\))?!?:\s*`)

// nonAlnumRe matches runs of non-alphanumeric characters.
var nonAlnumRe = regexp.MustCompile(`[^a-z0-9]+`)

const maxSlugLen = 30

// slugify converts a commit description into a bookmark-safe slug.
// It strips conventional commit prefixes, lowercases, replaces non-alphanumeric
// characters with hyphens, and truncates to maxSlugLen.
func slugify(s string) string {
	// Strip conventional commit prefix.
	s = conventionalPrefixRe.ReplaceAllString(s, "")
	s = strings.ToLower(s)
	s = nonAlnumRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")

	// Truncate at word boundary if possible.
	if len(s) > maxSlugLen {
		s = s[:maxSlugLen]
		if i := strings.LastIndex(s, "-"); i > maxSlugLen/2 {
			s = s[:i]
		}
	}
	return s
}
