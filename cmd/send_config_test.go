package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

// newSendFlags builds a flag set with the same definitions as `jip send`,
// so config application is tested against the real flag types and defaults.
func newSendFlags() *pflag.FlagSet {
	flags := pflag.NewFlagSet("send", pflag.ContinueOnError)
	sendCmd.Flags().VisitAll(func(f *pflag.Flag) {
		flags.AddFlag(&pflag.Flag{
			Name:      f.Name,
			Shorthand: f.Shorthand,
			Usage:     f.Usage,
			Value:     newValueLike(f),
			DefValue:  f.DefValue,
		})
	})
	return flags
}

// newValueLike creates a fresh pflag.Value of the same type and default as f,
// so tests don't mutate the shared sendCmd flag values.
func newValueLike(f *pflag.Flag) pflag.Value {
	tmp := pflag.NewFlagSet("tmp", pflag.ContinueOnError)
	switch f.Value.Type() {
	case "bool":
		tmp.Bool(f.Name, f.DefValue == "true", "")
	case "stringSlice":
		tmp.StringSlice(f.Name, nil, "")
	default:
		tmp.String(f.Name, f.DefValue, "")
	}
	return tmp.Lookup(f.Name).Value
}

func TestApplySendConfig_SetsUnsetFlags(t *testing.T) {
	flags := newSendFlags()
	cfg := map[string]string{
		"rebase":   "true",
		"base":     "dev",
		"reviewer": "alice,team/backend",
	}
	if err := applySendConfig(flags, cfg); err != nil {
		t.Fatalf("applySendConfig: %v", err)
	}
	if got := flags.Lookup("rebase").Value.String(); got != "true" {
		t.Errorf("rebase = %q, want true", got)
	}
	if got := flags.Lookup("base").Value.String(); got != "dev" {
		t.Errorf("base = %q, want dev", got)
	}
	if got := flags.Lookup("reviewer").Value.String(); got != "[alice,team/backend]" {
		t.Errorf("reviewer = %q, want [alice,team/backend]", got)
	}
}

func TestApplySendConfig_CLIFlagWins(t *testing.T) {
	flags := newSendFlags()
	if err := flags.Set("base", "release"); err != nil {
		t.Fatal(err)
	}
	if err := applySendConfig(flags, map[string]string{"base": "dev"}); err != nil {
		t.Fatalf("applySendConfig: %v", err)
	}
	if got := flags.Lookup("base").Value.String(); got != "release" {
		t.Errorf("base = %q, want release (CLI must override config)", got)
	}
}

func TestApplySendConfig_UnknownKey(t *testing.T) {
	flags := newSendFlags()
	err := applySendConfig(flags, map[string]string{"dry-run": "true"})
	if err == nil {
		t.Fatal("expected error for unsupported key")
	}
	if !strings.Contains(err.Error(), "dry-run") {
		t.Errorf("error should name the key, got: %v", err)
	}
}

func TestApplySendConfig_InvalidValue(t *testing.T) {
	flags := newSendFlags()
	err := applySendConfig(flags, map[string]string{"rebase": "yes"})
	if err == nil {
		t.Fatal("expected error for invalid boolean value")
	}
	if !strings.Contains(err.Error(), "rebase") {
		t.Errorf("error should name the key, got: %v", err)
	}
}

func TestResolveStackMode(t *testing.T) {
	tests := []struct {
		name         string
		stack        string
		stackSet     bool
		noStack      bool
		noStackOnCLI bool
		want         string
		wantErr      bool
	}{
		{name: "default", stack: "default", want: "default"},
		{name: "native", stack: "gh-native", stackSet: true, want: "gh-native"},
		{name: "invalid value", stack: "nope", stackSet: true, wantErr: true},
		{name: "no-stack alone", stack: "default", noStack: true, want: "none"},
		{name: "config stack beats config no-stack", stack: "gh-native", stackSet: true, noStack: true, want: "gh-native"},
		{name: "CLI no-stack beats config stack", stack: "gh-native", stackSet: true, noStack: true, noStackOnCLI: true, want: "none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveStackMode(tt.stack, tt.stackSet, tt.noStack, tt.noStackOnCLI)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// Every key allowed in config must correspond to an actual send flag.
func TestSendConfigKeys_MatchFlags(t *testing.T) {
	for key := range sendConfigKeys {
		if sendCmd.Flags().Lookup(key) == nil {
			t.Errorf("config key %q has no matching send flag", key)
		}
	}
}
