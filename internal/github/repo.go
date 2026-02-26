package github

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	httpsRe = regexp.MustCompile(`https?://[^/]+/([^/]+)/([^/.]+)`)
	sshRe   = regexp.MustCompile(`[^@]+@[^:]+:([^/]+)/([^/.]+)`)
)

// ParseRepoFromURL extracts owner and repo name from a GitHub remote URL.
// Supports both HTTPS and SSH formats.
func ParseRepoFromURL(url string) (owner, repo string, err error) {
	url = strings.TrimSpace(url)
	url = strings.TrimSuffix(url, ".git")

	if m := httpsRe.FindStringSubmatch(url); m != nil {
		return m[1], m[2], nil
	}
	if m := sshRe.FindStringSubmatch(url); m != nil {
		return m[1], m[2], nil
	}
	return "", "", fmt.Errorf("cannot parse owner/repo from URL: %s", url)
}
