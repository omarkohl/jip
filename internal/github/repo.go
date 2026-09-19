package github

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	httpsRe = regexp.MustCompile(`https?://([^/]+)/([^/]+)/([^/.]+)`)
	sshRe   = regexp.MustCompile(`[^@]+@([^:]+):([^/]+)/([^/.]+)`)
)

// ParseRepoFromURL extracts owner and repo name from a github.com remote URL.
// Supports both HTTPS and SSH formats. Non-github.com hosts (including GitHub
// Enterprise Server) are rejected — see https://github.com/omarkohl/jip/issues/49.
func ParseRepoFromURL(raw string) (owner, repo string, err error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimSuffix(raw, ".git")

	var host string
	if m := httpsRe.FindStringSubmatch(raw); m != nil {
		host, owner, repo = m[1], m[2], m[3]
	} else if m := sshRe.FindStringSubmatch(raw); m != nil {
		host, owner, repo = m[1], m[2], m[3]
	} else {
		return "", "", fmt.Errorf("cannot parse owner/repo from URL: %s", raw)
	}
	if err := requireGitHubHost(host); err != nil {
		return "", "", err
	}
	return owner, repo, nil
}

func requireGitHubHost(host string) error {
	host = strings.ToLower(strings.TrimSpace(host))
	if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	if host == "github.com" {
		return nil
	}
	return fmt.Errorf("remote host %q is not supported; jip works with github.com only (see https://github.com/omarkohl/jip/issues/49)", host)
}
