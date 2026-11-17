package github

import (
	"fmt"
	"strings"
)

const (
	legacyBodyConflict = "legacy jip body has no ownership boundary"

	SectionBegin = "<!-- jip:begin (generated from the commit message; edits here are overwritten) -->"
	SectionEnd   = "<!-- jip:end -->"
)

// BodyResolution is an explicit decision about a body with uncertain ownership.
type BodyResolution string

const (
	BodyPreserve  BodyResolution = "preserve"
	BodyOverwrite BodyResolution = "overwrite"
	BodySkip      BodyResolution = "skip"
	BodyAbort     BodyResolution = "abort"
)

type bodySection struct {
	start, end int
	found      bool
	conflict   string
}

// unfencedLines visits lines outside fenced code blocks and reports whether
// all fences are closed. Offsets exclude each line's final newline.
func unfencedLines(body string, visit func(start, end int)) bool {
	offset, fenceLen := 0, 0
	var fenceChar byte
	for line := range strings.SplitSeq(body, "\n") {
		text := strings.TrimSpace(line)
		if len(text) >= 3 && (text[0] == '`' || text[0] == '~') {
			n := 0
			for n < len(text) && text[n] == text[0] {
				n++
			}
			if fenceLen == 0 && n >= 3 {
				fenceChar, fenceLen = text[0], n
			} else if fenceLen > 0 && text[0] == fenceChar && n >= fenceLen && strings.TrimSpace(text[n:]) == "" {
				fenceLen = 0
			}
		} else if fenceLen == 0 {
			visit(offset, offset+len(line))
		}
		offset += len(line) + 1
	}
	return fenceLen == 0
}

// metadataLines visits standalone jip comments outside fenced code blocks.
// Offsets include the comment line, but not its final newline.
func metadataLines(body string, visit func(start, end int, kind string)) bool {
	return unfencedLines(body, func(start, end int) {
		text := strings.TrimSpace(body[start:end])
		for _, kind := range []string{"begin", "end", "pushed-commit="} {
			prefix := "<!-- jip:" + kind
			if strings.HasPrefix(text, prefix) && (kind == "pushed-commit=" || len(text) == len(prefix) || strings.ContainsRune(" \t\r", rune(text[len(prefix)]))) {
				visit(start, end, kind)
				break
			}
		}
	})
}

func findBodySection(body string) bodySection {
	var section bodySection
	begins, ends, pushed := 0, 0, false
	closedFences := metadataLines(body, func(start, end int, kind string) {
		switch kind {
		case "begin":
			begins++
			section.start = start
			line := strings.TrimSpace(body[start:end])
			if line != SectionBegin && line != "<!-- jip:begin -->" {
				section.conflict = "broken jip section markers"
			}
		case "end":
			ends++
			section.end = end
			if strings.TrimSpace(body[start:end]) != SectionEnd {
				section.conflict = "broken jip section markers"
			}
		case "pushed-commit=":
			pushed = true
		}
	})
	if begins == 0 && ends == 0 {
		if pushed || parseReviewCommit(body) != "" {
			section.conflict = legacyBodyConflict
		} else if !closedFences {
			section.conflict = "unclosed Markdown code fence"
		}
		return section
	}
	if begins != 1 || ends != 1 || section.start >= section.end {
		section.conflict = "broken jip section markers"
	}
	section.found = section.conflict == ""
	return section
}

// PRBodyConflict returns why the body requires an explicit user decision.
func PRBodyConflict(body string) string { return findBodySection(body).conflict }

// ValidatePRContent rejects reserved comments in user-supplied generated text.
func ValidatePRContent(body string) error {
	reserved := false
	metadataLines(body, func(_, _ int, _ string) { reserved = true })
	if reserved {
		return fmt.Errorf("reserved jip comments in template or commit message: remove them or put examples in a fenced code block")
	}
	if !findBodySection(SectionBegin + "\n\n" + body + "\n\n" + SectionEnd).found {
		return fmt.Errorf("unclosed Markdown code fence in template or commit message: close it before sending")
	}
	return nil
}

// MergePRBody replaces only a valid owned section, preserving both outside
// regions byte-for-byte. Without a section, it appends one at the bottom.
// A resolution is used only when ownership is uncertain.
func MergePRBody(body, generated string, resolution BodyResolution) (string, error) {
	section := findBodySection(body)
	block := SectionBegin + "\n\n" + generated + "\n\n" + SectionEnd
	if section.conflict != "" {
		switch resolution {
		case BodyOverwrite:
			return block, nil
		case BodyPreserve:
			var clean strings.Builder
			cursor := 0
			metadataLines(body, func(start, end int, _ string) {
				clean.WriteString(body[cursor:start])
				cursor = end
			})
			clean.WriteString(body[cursor:])
			body = clean.String()
		default:
			return "", fmt.Errorf("%s", section.conflict)
		}
	} else if section.found {
		return body[:section.start] + block + body[section.end:], nil
	}
	if body == "" {
		return block, nil
	}
	separator := "\n\n"
	if strings.HasSuffix(body, "\n\n") || strings.HasSuffix(body, "\r\n\r\n") {
		separator = ""
	} else if strings.HasSuffix(body, "\n") {
		separator = "\n"
	}
	merged := body + separator + block
	if !findBodySection(merged).found {
		return "", fmt.Errorf("cannot append a jip section after an unclosed Markdown code fence; close the fence, or choose overwrite, skip, or abort")
	}
	return merged, nil
}

// recordedBody prevents outside comments from overriding the owned record.
func recordedBody(body string) string {
	section := findBodySection(body)
	if section.conflict != "" && section.conflict != legacyBodyConflict {
		return ""
	}
	if section.found {
		return body[section.start:section.end]
	}
	return body
}
