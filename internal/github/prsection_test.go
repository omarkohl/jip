package github

import (
	"strings"
	"testing"
)

func TestMergePRBody(t *testing.T) {
	oldBlock := SectionBegin + "\n\nold\n\n" + SectionEnd
	newBlock := SectionBegin + "\n\nnew\n\n" + SectionEnd
	for _, tc := range []struct {
		name, body, want string
		resolution       BodyResolution
		conflict         bool
	}{
		{name: "empty", want: newBlock},
		{name: "manual", body: "Manual description", want: "Manual description\n\n" + newBlock},
		{name: "both outside regions", body: "  Before\r\n\r\n" + oldBlock + "\n\nAfter  \n", want: "  Before\r\n\r\n" + newBlock + "\n\nAfter  \n"},
		{name: "legacy", body: WithPushedCommitMarker("manual edits", "abcdef1"), conflict: true},
		{name: "older legacy", body: BuildStackedPRBody("abcdef1", "o/r", 1, []int{1, 2}, "body"), conflict: true},
		{name: "unpaired begin", body: SectionBegin, conflict: true},
		{name: "unpaired end", body: SectionEnd, conflict: true},
		{name: "unclosed code fence", body: "```go\nunclosed", conflict: true},
		{name: "cannot preserve unclosed code fence", body: "```go\nunclosed", resolution: BodyPreserve, conflict: true},
		{name: "duplicate", body: oldBlock + "\n" + oldBlock, conflict: true},
		{name: "misordered", body: SectionEnd + "\n" + SectionBegin, conflict: true},
		{name: "malformed begin", body: "<!-- jip:begin broken\n\n" + SectionEnd, conflict: true},
		{name: "overwrite conflict", body: "important\n" + SectionBegin, resolution: BodyOverwrite, want: newBlock},
		{name: "overwrite valid", body: "important\n\n" + oldBlock, resolution: BodyOverwrite, want: "important\n\n" + newBlock},
		{name: "preserve conflict", body: "important\n" + SectionBegin + "\nold text", resolution: BodyPreserve, want: "important\n\nold text\n\n" + newBlock},
		{name: "inline example", body: "Example: <!-- jip:begin -->", want: "Example: <!-- jip:begin -->\n\n" + newBlock},
		{name: "fenced example", body: "```html\n" + SectionBegin + "\n```", want: "```html\n" + SectionBegin + "\n```\n\n" + newBlock},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := MergePRBody(tc.body, "new", tc.resolution)
			if tc.conflict {
				if err == nil {
					t.Fatal("expected conflict")
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
			again, err := MergePRBody(got, "new", "")
			if err != nil || again != got {
				t.Fatalf("not idempotent: %q, %v", again, err)
			}
		})
	}
}

func TestRecordsUseOwnedSection(t *testing.T) {
	inside := WithPushedCommitMarker(BuildStackedPRBody("aaaaaaa", "o/r", 1, []int{1, 2}, "body"), "aaaaaaa")
	block, err := MergePRBody("", inside, "")
	if err != nil {
		t.Fatal(err)
	}
	outside := WithPushedCommitMarker(BuildStackedPRBody("bbbbbbb", "o/r", 1, []int{1, 2}, "outside"), "bbbbbbb")
	body := outside + "\n\n" + block + "\n\n" + outside
	if got := ParsePushedCommit(body); got != "aaaaaaa" {
		t.Fatalf("pushed commit = %q", got)
	}
	if got := ParseReviewCommit(body); got != "aaaaaaa" {
		t.Fatalf("review commit = %q", got)
	}
	broken := strings.Replace(body, SectionEnd, "", 1)
	if ParsePushedCommit(broken) != "" || ParseReviewCommit(broken) != "" {
		t.Fatal("must not trust records in a broken section")
	}
}

func TestValidatePRContent(t *testing.T) {
	for _, body := range []string{SectionBegin, SectionEnd, "<!-- jip:pushed-commit=aaaaaaa -->", "<!-- jip:begin broken", "```go\nunclosed code"} {
		if err := ValidatePRContent(body); err == nil {
			t.Errorf("accepted reserved comment: %q", body)
		}
	}
	if err := ValidatePRContent("```html\n" + SectionBegin + "\n```\nnormal text"); err != nil {
		t.Fatal(err)
	}
}

func TestFencedLegacyReviewExample(t *testing.T) {
	body := "Example:\n\n```markdown\n" + BuildStackedPRBody("aaaaaaa", "o/r", 1, []int{1, 2}, "description") + "\n```"
	if reason := PRBodyConflict(body); reason != "" {
		t.Fatalf("example reported as conflict: %s", reason)
	}
	if got := ParseReviewCommit(body); got != "" {
		t.Fatalf("example used as commit record: %q", got)
	}
	merged, err := MergePRBody(body, "new", "")
	if err != nil || !strings.HasPrefix(merged, body+"\n\n") {
		t.Fatalf("example not preserved: %q, %v", merged, err)
	}
}
