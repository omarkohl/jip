package jj

import (
	"testing"
)

// --- ParseBookmarkList tests ---

func TestParseBookmarkList_LocalAndRemote(t *testing.T) {
	jsonl := `{"name":"main","remote":null,"present":true,"target":"abc123","change_id":"xaa","tracked":false,"synced":false}
{"name":"main","remote":"origin","present":true,"target":"abc123","change_id":"xaa","tracked":true,"synced":true}
`
	bookmarks, err := ParseBookmarkList([]byte(jsonl))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bookmarks) != 1 {
		t.Fatalf("expected 1 bookmark, got %d", len(bookmarks))
	}
	b := bookmarks[0]
	if b.Name != "main" {
		t.Errorf("expected name 'main', got %q", b.Name)
	}
	if !b.Present {
		t.Error("expected present=true")
	}
	if b.Target != "abc123" {
		t.Errorf("expected target 'abc123', got %q", b.Target)
	}
	if b.ChangeID != "xaa" {
		t.Errorf("expected change_id 'xaa', got %q", b.ChangeID)
	}
	rs, ok := b.Remotes["origin"]
	if !ok {
		t.Fatal("expected origin remote entry")
	}
	if !rs.Tracked {
		t.Error("expected tracked=true")
	}
	if !rs.Synced {
		t.Error("expected synced=true")
	}
}

func TestParseBookmarkList_FiltersGitRemote(t *testing.T) {
	jsonl := `{"name":"main","remote":null,"present":true,"target":"abc","change_id":"x","tracked":false,"synced":false}
{"name":"main","remote":"git","present":true,"target":"abc","change_id":"x","tracked":true,"synced":true}
{"name":"main","remote":"origin","present":true,"target":"abc","change_id":"x","tracked":true,"synced":true}
`
	bookmarks, err := ParseBookmarkList([]byte(jsonl))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bookmarks) != 1 {
		t.Fatalf("expected 1 bookmark, got %d", len(bookmarks))
	}
	if _, ok := bookmarks[0].Remotes["git"]; ok {
		t.Error("expected git remote to be filtered out")
	}
	if _, ok := bookmarks[0].Remotes["origin"]; !ok {
		t.Error("expected origin remote to be present")
	}
}

func TestParseBookmarkList_MultipleBookmarks(t *testing.T) {
	jsonl := `{"name":"main","remote":null,"present":true,"target":"aaa","change_id":"xa","tracked":false,"synced":false}
{"name":"feat-branch","remote":null,"present":true,"target":"bbb","change_id":"xb","tracked":false,"synced":false}
{"name":"feat-branch","remote":"origin","present":true,"target":"ccc","change_id":"xc","tracked":true,"synced":false}
`
	bookmarks, err := ParseBookmarkList([]byte(jsonl))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bookmarks) != 2 {
		t.Fatalf("expected 2 bookmarks, got %d", len(bookmarks))
	}
	// Check order is preserved.
	if bookmarks[0].Name != "main" {
		t.Errorf("expected first bookmark 'main', got %q", bookmarks[0].Name)
	}
	if bookmarks[1].Name != "feat-branch" {
		t.Errorf("expected second bookmark 'feat-branch', got %q", bookmarks[1].Name)
	}
	// feat-branch origin remote should not be synced (different targets).
	rs := bookmarks[1].Remotes["origin"]
	if rs.Synced {
		t.Error("expected synced=false for feat-branch origin")
	}
}

func TestParseBookmarkList_RemoteOnly(t *testing.T) {
	jsonl := `{"name":"remote-branch","remote":"origin","present":true,"target":"abc","change_id":"x","tracked":false,"synced":false}
`
	bookmarks, err := ParseBookmarkList([]byte(jsonl))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bookmarks) != 1 {
		t.Fatalf("expected 1 bookmark, got %d", len(bookmarks))
	}
	if bookmarks[0].Present {
		t.Error("expected present=false for remote-only bookmark")
	}
}

func TestParseBookmarkList_Empty(t *testing.T) {
	bookmarks, err := ParseBookmarkList([]byte(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bookmarks) != 0 {
		t.Fatalf("expected 0 bookmarks, got %d", len(bookmarks))
	}
}

func TestParseBookmarkList_MalformedJSON(t *testing.T) {
	_, err := ParseBookmarkList([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestParseBookmarkList_MultipleRemotes(t *testing.T) {
	jsonl := `{"name":"main","remote":null,"present":true,"target":"aaa","change_id":"xa","tracked":false,"synced":false}
{"name":"main","remote":"origin","present":true,"target":"aaa","change_id":"xa","tracked":true,"synced":true}
{"name":"main","remote":"upstream","present":true,"target":"bbb","change_id":"xb","tracked":true,"synced":false}
`
	bookmarks, err := ParseBookmarkList([]byte(jsonl))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bookmarks) != 1 {
		t.Fatalf("expected 1 bookmark, got %d", len(bookmarks))
	}
	b := bookmarks[0]
	if len(b.Remotes) != 2 {
		t.Fatalf("expected 2 remotes, got %d", len(b.Remotes))
	}
	if b.Remotes["upstream"].Synced {
		t.Error("expected upstream synced=false")
	}
}

// --- SyncWith tests ---

func TestSyncWith_InSync(t *testing.T) {
	b := &BookmarkInfo{
		Present: true,
		Target:  "abc",
		Remotes: map[string]RemoteBookmarkState{
			"origin": {Target: "abc", Tracked: true, Synced: true},
		},
	}
	if s := b.SyncWith("origin"); s != SyncInSync {
		t.Errorf("expected SyncInSync, got %v", s)
	}
}

func TestSyncWith_Ahead(t *testing.T) {
	b := &BookmarkInfo{
		Present: true,
		Target:  "abc",
		Remotes: map[string]RemoteBookmarkState{
			"origin": {Target: "old", Tracked: true, Synced: false},
		},
	}
	if s := b.SyncWith("origin"); s != SyncAhead {
		t.Errorf("expected SyncAhead, got %v", s)
	}
}

func TestSyncWith_TargetsDiffer_NoConflict(t *testing.T) {
	// When targets differ and there's no conflict, local is authoritative (pushable).
	// This covers rebases, amends, and any case where the local moved.
	b := &BookmarkInfo{
		Present: true,
		Target:  "abc",
		Remotes: map[string]RemoteBookmarkState{
			"origin": {Target: "xyz", Tracked: true, Synced: false},
		},
	}
	if s := b.SyncWith("origin"); s != SyncAhead {
		t.Errorf("expected SyncAhead, got %v", s)
	}
}

func TestSyncWith_LocalOnly(t *testing.T) {
	b := &BookmarkInfo{
		Present: true,
		Target:  "abc",
		Remotes: map[string]RemoteBookmarkState{},
	}
	if s := b.SyncWith("origin"); s != SyncLocalOnly {
		t.Errorf("expected SyncLocalOnly, got %v", s)
	}
}

func TestSyncWith_RemoteOnly(t *testing.T) {
	b := &BookmarkInfo{
		Present: false,
		Remotes: map[string]RemoteBookmarkState{
			"origin": {Target: "abc", Tracked: false},
		},
	}
	if s := b.SyncWith("origin"); s != SyncRemoteOnly {
		t.Errorf("expected SyncRemoteOnly, got %v", s)
	}
}

func TestSyncWith_UnknownRemote(t *testing.T) {
	b := &BookmarkInfo{
		Present: true,
		Remotes: map[string]RemoteBookmarkState{
			"origin": {Target: "abc", Tracked: true},
		},
	}
	if s := b.SyncWith("upstream"); s != SyncLocalOnly {
		t.Errorf("expected SyncLocalOnly for unknown remote, got %v", s)
	}
}

func TestSyncWith_NotPresent_NoRemote(t *testing.T) {
	b := &BookmarkInfo{
		Present: false,
		Remotes: map[string]RemoteBookmarkState{},
	}
	if s := b.SyncWith("origin"); s != SyncUnknown {
		t.Errorf("expected SyncUnknown, got %v", s)
	}
}

func TestSyncWith_Scenario1_FullySynced(t *testing.T) {
	b := &BookmarkInfo{
		Present: true,
		Target:  "abc123",
		Remotes: map[string]RemoteBookmarkState{
			"origin": {Target: "abc123", Tracked: true, Synced: true},
		},
	}
	if s := b.SyncWith("origin"); s != SyncInSync {
		t.Errorf("expected SyncInSync, got %v", s)
	}
}

func TestSyncWith_Scenario2_LocalAheadFastForward(t *testing.T) {
	// New commits on top of bookmarked commit. Push would fast-forward.
	b := &BookmarkInfo{
		Present: true,
		Target:  "new456",
		Remotes: map[string]RemoteBookmarkState{
			"origin": {Target: "abc123", Tracked: true, Synced: false},
		},
	}
	if s := b.SyncWith("origin"); s != SyncAhead {
		t.Errorf("expected SyncAhead, got %v", s)
	}
}

func TestSyncWith_Scenario3_LocalRewritten(t *testing.T) {
	// Bookmark was rebased/amended. Same change-id, different commit-id.
	// Not synced, no conflict — pushable.
	b := &BookmarkInfo{
		Present: true,
		Target:  "rebased789",
		Remotes: map[string]RemoteBookmarkState{
			"origin": {Target: "original123", Tracked: true, Synced: false},
		},
	}
	if s := b.SyncWith("origin"); s != SyncAhead {
		t.Errorf("expected SyncAhead, got %v", s)
	}
}

func TestSyncWith_Scenario4_LocalMovedUnrelated(t *testing.T) {
	// Bookmark pointed to completely different commit via jj bookmark set.
	b := &BookmarkInfo{
		Present: true,
		Target:  "unrelated999",
		Remotes: map[string]RemoteBookmarkState{
			"origin": {Target: "original123", Tracked: true, Synced: false},
		},
	}
	if s := b.SyncWith("origin"); s != SyncAhead {
		t.Errorf("expected SyncAhead, got %v", s)
	}
}

func TestSyncWith_Scenario5_RemoteAheadAfterFetch(t *testing.T) {
	// After fetch, local fast-forwards to match remote — synced.
	b := &BookmarkInfo{
		Present: true,
		Target:  "abc123",
		Remotes: map[string]RemoteBookmarkState{
			"origin": {Target: "abc123", Tracked: true, Synced: true},
		},
	}
	if s := b.SyncWith("origin"); s != SyncInSync {
		t.Errorf("expected SyncInSync, got %v", s)
	}
}

func TestSyncWith_Scenario6_LocalOnlyNeverPushed(t *testing.T) {
	b := &BookmarkInfo{
		Present: true,
		Target:  "abc123",
		Remotes: map[string]RemoteBookmarkState{},
	}
	if s := b.SyncWith("origin"); s != SyncLocalOnly {
		t.Errorf("expected SyncLocalOnly, got %v", s)
	}
}

func TestSyncWith_Scenario7_RemoteOnlyUntracked(t *testing.T) {
	b := &BookmarkInfo{
		Present: false,
		Remotes: map[string]RemoteBookmarkState{
			"origin": {Target: "abc123", Tracked: false},
		},
	}
	if s := b.SyncWith("origin"); s != SyncRemoteOnly {
		t.Errorf("expected SyncRemoteOnly, got %v", s)
	}
}

func TestSyncWith_Scenario8_RemoteOnlyTracked(t *testing.T) {
	b := &BookmarkInfo{
		Present: false,
		Remotes: map[string]RemoteBookmarkState{
			"origin": {Target: "abc123", Tracked: true},
		},
	}
	if s := b.SyncWith("origin"); s != SyncRemoteOnly {
		t.Errorf("expected SyncRemoteOnly, got %v", s)
	}
}

func TestSyncWith_Scenario9_LocalDeletedRemoteExists(t *testing.T) {
	b := &BookmarkInfo{
		Present: false,
		Remotes: map[string]RemoteBookmarkState{
			"origin": {Target: "abc123", Tracked: true},
		},
	}
	if s := b.SyncWith("origin"); s != SyncRemoteOnly {
		t.Errorf("expected SyncRemoteOnly, got %v", s)
	}
}

func TestSyncWith_Scenario11_BothMovedDiverged(t *testing.T) {
	// Both local and remote moved independently → conflict.
	b := &BookmarkInfo{
		Present:  true,
		Target:   "",
		Conflict: true,
		Remotes: map[string]RemoteBookmarkState{
			"origin": {Target: "remote456", Tracked: true},
		},
	}
	if s := b.SyncWith("origin"); s != SyncDiverged {
		t.Errorf("expected SyncDiverged, got %v", s)
	}
}

func TestSyncWith_Scenario12_BothForcePushed(t *testing.T) {
	// Both sides rewrote history → conflict.
	b := &BookmarkInfo{
		Present:  true,
		Target:   "",
		Conflict: true,
		Remotes: map[string]RemoteBookmarkState{
			"origin": {Target: "remote789", Tracked: true},
		},
	}
	if s := b.SyncWith("origin"); s != SyncDiverged {
		t.Errorf("expected SyncDiverged, got %v", s)
	}
}

func TestSyncWith_Scenario14_StaleRemote(t *testing.T) {
	// Haven't fetched, remote moved. Templates look normal — local is ahead.
	// The actual push failure is caught at push time by force-with-lease.
	b := &BookmarkInfo{
		Present: true,
		Target:  "local123",
		Remotes: map[string]RemoteBookmarkState{
			"origin": {Target: "stale_old", Tracked: true, Synced: false},
		},
	}
	if s := b.SyncWith("origin"); s != SyncAhead {
		t.Errorf("expected SyncAhead, got %v", s)
	}
}

func TestSyncWith_Scenario15_ConflictedRemoteAgreesOneSide(t *testing.T) {
	b := &BookmarkInfo{
		Present:  true,
		Target:   "",
		Conflict: true,
		Remotes: map[string]RemoteBookmarkState{
			"origin": {Target: "one_side", Tracked: true},
		},
	}
	if s := b.SyncWith("origin"); s != SyncDiverged {
		t.Errorf("expected SyncDiverged, got %v", s)
	}
}

// --- slugify tests ---

func TestSlugify_ConventionalPrefix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"feat: add auth module", "add-auth-module"},
		{"fix: handle nil pointer", "handle-nil-pointer"},
		{"refactor(auth): extract token", "extract-token"},
		{"docs: update README", "update-readme"},
		{"feat!: breaking change", "breaking-change"},
	}
	for _, tt := range tests {
		got := slugify(tt.input)
		if got != tt.want {
			t.Errorf("slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSlugify_SpecialCharacters(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello world", "hello-world"},
		{"Hello  World!", "hello-world"},
		{"one/two/three", "one-two-three"},
		{"CamelCase", "camelcase"},
		{"dots.and.dashes-here", "dots-and-dashes-here"},
		{"  leading trailing  ", "leading-trailing"},
	}
	for _, tt := range tests {
		got := slugify(tt.input)
		if got != tt.want {
			t.Errorf("slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSlugify_Empty(t *testing.T) {
	if got := slugify(""); got != "" {
		t.Errorf("slugify(\"\") = %q, want empty", got)
	}
}

func TestSlugify_Truncation(t *testing.T) {
	long := "this is a very long commit description that should be truncated at a reasonable length"
	got := slugify(long)
	if len(got) > maxSlugLen {
		t.Errorf("slugify result too long: %d > %d: %q", len(got), maxSlugLen, got)
	}
	// Should truncate at a word boundary.
	if got[len(got)-1] == '-' {
		t.Errorf("slug should not end with hyphen: %q", got)
	}
}

// --- GenerateBookmarkName tests ---

func TestGenerateBookmarkName_Basic(t *testing.T) {
	name := GenerateBookmarkName("feat: add auth module", "xyzklmno")
	want := "jip/add-auth-module/xyzklmno"
	if name != want {
		t.Errorf("got %q, want %q", name, want)
	}
}

func TestGenerateBookmarkName_EmptyDescription(t *testing.T) {
	name := GenerateBookmarkName("", "abc12345")
	want := "jip/change/abc12345"
	if name != want {
		t.Errorf("got %q, want %q", name, want)
	}
}

// --- MatchBookmarksToChanges tests ---

func TestMatchBookmarksToChanges_Basic(t *testing.T) {
	dag := &ChangeDAG{
		Changes: []*Change{
			{ChangeID: "xa", CommitID: "c1"},
			{ChangeID: "xb", CommitID: "c2"},
			{ChangeID: "xc", CommitID: "c3"},
		},
	}
	bookmarks := []BookmarkInfo{
		{Name: "branch-a", Present: true, Target: "c1", Remotes: map[string]RemoteBookmarkState{}},
		{Name: "branch-b", Present: true, Target: "c2", Remotes: map[string]RemoteBookmarkState{}},
		{Name: "unrelated", Present: true, Target: "c99", Remotes: map[string]RemoteBookmarkState{}},
	}

	matched := MatchBookmarksToChanges(dag, bookmarks)
	if len(matched["xa"]) != 1 || matched["xa"][0].Name != "branch-a" {
		t.Errorf("expected xa matched to branch-a, got %v", matched["xa"])
	}
	if len(matched["xb"]) != 1 || matched["xb"][0].Name != "branch-b" {
		t.Errorf("expected xb matched to branch-b, got %v", matched["xb"])
	}
	if _, ok := matched["xc"]; ok {
		t.Error("expected xc to have no matched bookmarks")
	}
}

func TestMatchBookmarksToChanges_MultipleBookmarksOnSameChange(t *testing.T) {
	dag := &ChangeDAG{
		Changes: []*Change{
			{ChangeID: "xa", CommitID: "c1"},
		},
	}
	bookmarks := []BookmarkInfo{
		{Name: "branch-1", Present: true, Target: "c1", Remotes: map[string]RemoteBookmarkState{}},
		{Name: "branch-2", Present: true, Target: "c1", Remotes: map[string]RemoteBookmarkState{}},
	}

	matched := MatchBookmarksToChanges(dag, bookmarks)
	if len(matched["xa"]) != 2 {
		t.Errorf("expected 2 bookmarks matched to xa, got %d", len(matched["xa"]))
	}
}

func TestMatchBookmarksToChanges_SkipsNonPresent(t *testing.T) {
	dag := &ChangeDAG{
		Changes: []*Change{
			{ChangeID: "xa", CommitID: "c1"},
		},
	}
	bookmarks := []BookmarkInfo{
		{Name: "remote-only", Present: false, Target: "", Remotes: map[string]RemoteBookmarkState{
			"origin": {Target: "c1"},
		}},
	}

	matched := MatchBookmarksToChanges(dag, bookmarks)
	if _, ok := matched["xa"]; ok {
		t.Error("expected non-present bookmarks to be skipped")
	}
}

// --- SyncState String tests ---

func TestSyncStateString(t *testing.T) {
	tests := []struct {
		state SyncState
		want  string
	}{
		{SyncInSync, "in-sync"},
		{SyncAhead, "ahead"},
		{SyncBehind, "behind"},
		{SyncDiverged, "diverged"},
		{SyncLocalOnly, "local-only"},
		{SyncRemoteOnly, "remote-only"},
		{SyncUnknown, "unknown"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("SyncState(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}
