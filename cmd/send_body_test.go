package cmd

import (
	"bytes"
	"strings"
	"testing"

	gh "github.com/omarkohl/jip/internal/github"
	"github.com/omarkohl/jip/internal/jj"
)

func TestPromptBodyConflict(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  gh.BodyResolution
	}{
		{"\n", gh.BodyAbort}, {"", gh.BodyAbort}, {"p\n", gh.BodyPreserve},
		{"overwrite\n", gh.BodyOverwrite}, {"S\n", gh.BodySkip}, {"invalid\na\n", gh.BodyAbort},
	} {
		var out bytes.Buffer
		got, err := promptBodyConflict(strings.NewReader(tc.input), &out)(42, "broken markers")
		if err != nil || got != tc.want {
			t.Errorf("%q: got %q, %v; want %q", tc.input, got, err, tc.want)
		}
	}
}

func TestResolveBodyConflict(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts sendOpts
		want gh.BodyResolution
		err  bool
	}{
		{name: "noninteractive", err: true},
		{name: "overwrite flag", opts: sendOpts{overwritePRBody: true}, want: gh.BodyOverwrite},
		{name: "interactive preserve", opts: sendOpts{resolveBody: func(int, string) (gh.BodyResolution, error) { return gh.BodyPreserve, nil }}, want: gh.BodyPreserve},
		{name: "interactive skip", opts: sendOpts{resolveBody: func(int, string) (gh.BodyResolution, error) { return gh.BodySkip, nil }}, want: gh.BodySkip},
		{name: "interactive abort", opts: sendOpts{resolveBody: func(int, string) (gh.BodyResolution, error) { return gh.BodyAbort, nil }}, err: true},
		{name: "dry run never prompts", opts: sendOpts{dryRun: true, resolveBody: func(int, string) (gh.BodyResolution, error) { t.Fatal("prompt in dry run"); return "", nil }}, err: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := changeState{pr: &gh.PRInfo{Number: 42, Body: "important\n" + gh.SectionBegin}}
			err := resolveBodyConflict(&s, tc.opts)
			if (err != nil) != tc.err || s.bodyResolution != tc.want {
				t.Fatalf("got %q, %v; want %q, error %v", s.bodyResolution, err, tc.want, tc.err)
			}
		})
	}
}

type bodyService struct {
	gh.Service
	current string
	updates int
}

type templateRunner struct {
	jj.Runner
	revision string
}

func (r *templateRunner) PRTemplate(revision string) (string, error) {
	r.revision = revision
	return "template", nil
}

func TestSendTemplateBase(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts sendOpts
		want string
	}{
		{name: "configured base", opts: sendOpts{base: "release"}, want: "release"},
		{name: "upstream base", opts: sendOpts{base: "release", upstream: "upstream", upstreamRemote: "upstream", stackMode: stackModeNative}, want: "release@upstream"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &templateRunner{}
			body, err := sendTemplate(runner, tc.opts, "release")
			if err != nil || body != "template" || runner.revision != tc.want {
				t.Fatalf("body %q, revision %q, error %v", body, runner.revision, err)
			}
		})
	}
}

func (s *bodyService) GetPR(number int) (*gh.PRInfo, error) {
	return &gh.PRInfo{Number: number, Body: s.current}, nil
}

func (s *bodyService) UpdatePR(_ int, opts gh.UpdatePROpts) error {
	s.updates++
	s.current = *opts.Body
	return nil
}

func TestUpdateOwnedBodyConcurrentEdits(t *testing.T) {
	old, _ := gh.MergePRBody("description", "old", "")
	for _, tc := range []struct {
		name, current string
		resolution    gh.BodyResolution
		err           bool
	}{
		{name: "outside edits", current: "new checklist\n\n" + old},
		{name: "new conflict", current: "edited description\n" + gh.SectionBegin, err: true},
		{name: "overwrite flag cannot approve later edits", current: "edited description\n" + gh.SectionBegin, resolution: gh.BodyOverwrite, err: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &bodyService{current: tc.current}
			s := changeState{pr: &gh.PRInfo{Number: 42}, bodyBeforePush: old, bodyResolution: tc.resolution}
			err := updateOwnedBody(service, &s, "new")
			if (err != nil) != tc.err {
				t.Fatalf("error = %v", err)
			}
			if tc.err {
				if service.updates != 0 || !strings.Contains(err.Error(), "already pushed") {
					t.Fatalf("unsafe conflict handling: updates %d, error %v", service.updates, err)
				}
				return
			}
			if !strings.HasPrefix(service.current, "new checklist\n\ndescription") {
				t.Fatal("concurrent outside edits lost")
			}
			if err := updateOwnedBody(service, &s, "new"); err != nil || service.updates != 1 {
				t.Fatalf("unchanged body updated: %d, %v", service.updates, err)
			}
		})
	}
}
