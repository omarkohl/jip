package cmd

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	gh "github.com/omarkohl/jip/internal/github"
	"github.com/omarkohl/jip/internal/jj"
)

type bodyConflictResolver func(number int, reason string) (gh.BodyResolution, error)

func promptBodyConflict(in io.Reader, out io.Writer) bodyConflictResolver {
	scanner := bufio.NewScanner(in)
	return func(number int, reason string) (gh.BodyResolution, error) {
		_, _ = fmt.Fprintf(out, "PR #%d: %s.\nPreserve keeps the old text but can duplicate descriptions and navigation.\nOverwrite deletes the whole description.\n", number, reason)
		for {
			_, _ = fmt.Fprint(out, "[p]reserve / [o]verwrite / [s]kip / [a]bort (default): ")
			if !scanner.Scan() {
				if err := scanner.Err(); err != nil {
					return gh.BodyAbort, err
				}
				return gh.BodyAbort, nil
			}
			switch strings.ToLower(strings.TrimSpace(scanner.Text())) {
			case "p", "preserve":
				return gh.BodyPreserve, nil
			case "o", "overwrite":
				return gh.BodyOverwrite, nil
			case "s", "skip":
				return gh.BodySkip, nil
			case "", "a", "abort":
				return gh.BodyAbort, nil
			}
		}
	}
}

func resolveBodyConflict(s *changeState, opts sendOpts) error {
	reason := gh.PRBodyConflict(s.pr.Body)
	if reason == "" {
		return nil
	}
	if opts.overwritePRBody {
		s.bodyResolution = gh.BodyOverwrite
		return nil
	}
	if opts.resolveBody == nil || opts.dryRun {
		return fmt.Errorf("PR #%d: %s; run in a terminal to choose preserve, overwrite, skip, or abort, or use --overwrite-pr-body to overwrite the whole description", s.pr.Number, reason)
	}
	decision, err := opts.resolveBody(s.pr.Number, reason)
	if err != nil {
		return err
	}
	switch decision {
	case gh.BodyPreserve, gh.BodyOverwrite, gh.BodySkip:
		s.bodyResolution = decision
		return nil
	default:
		return fmt.Errorf("send aborted: PR #%d body conflict", s.pr.Number)
	}
}

func sendTemplate(runner jj.Runner, opts sendOpts, baseBranch string) (string, error) {
	base := opts.base
	if opts.upstream != "" {
		if opts.upstreamRemote == "" {
			return "", fmt.Errorf("PR template lookup requires a local upstream remote; add one and use --upstream=<remote> instead of a URL")
		}
		base = baseBranch + "@" + opts.upstreamRemote
	}
	body, err := runner.PRTemplate(base)
	if err != nil {
		return "", err
	}
	if err := gh.ValidatePRContent(body); err != nil {
		return "", fmt.Errorf("PR template: %w", err)
	}
	if reason := gh.PRBodyConflict(body); reason != "" {
		return "", fmt.Errorf("PR template: %s; remove legacy jip records", reason)
	}
	return body, nil
}

// updateOwnedBody re-reads the PR so edits made during the push are preserved.
// A conflict decision applies only to the exact body approved before pushing.
func updateOwnedBody(client gh.Service, s *changeState, generated string) error {
	current, err := client.GetPR(s.pr.Number)
	if err != nil {
		return fmt.Errorf("code was already pushed; cannot read PR #%d body: %w", s.pr.Number, err)
	}
	if current.Body != s.bodyBeforePush && gh.PRBodyConflict(current.Body) != "" {
		return fmt.Errorf("code was already pushed; PR #%d description changed and now needs a new conflict decision; body left unchanged, run jip send again", s.pr.Number)
	}
	body, err := gh.MergePRBody(current.Body, generated, s.bodyResolution)
	if err != nil {
		return fmt.Errorf("code was already pushed; PR #%d body left unchanged: %w", s.pr.Number, err)
	}
	if body == current.Body {
		return nil
	}
	if err := client.UpdatePR(s.pr.Number, gh.UpdatePROpts{Body: &body}); err != nil {
		return fmt.Errorf("code was already pushed; updating PR #%d body: %w", s.pr.Number, err)
	}
	s.changed = true
	return nil
}
