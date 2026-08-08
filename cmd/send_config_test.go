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
			Name:        f.Name,
			Shorthand:   f.Shorthand,
			Usage:       f.Usage,
			Value:       newValueLike(f),
			DefValue:    f.DefValue,
			NoOptDefVal: f.NoOptDefVal,
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

func TestApplySendConfig_NegationDropsConfigValue(t *testing.T) {
	cfg := map[string]string{
		"draft":          "true",
		"rebase":         "true",
		"diff-since-jip": "true",
		"upstream":       "git@github.com:fork/project.git",
		"reviewer":       "alice,team/backend",
	}
	// The value a key is left at when its negation is given...
	dropped := map[string]string{
		"draft":          "false",
		"rebase":         "false",
		"diff-since-jip": "false",
		"upstream":       "",
		"reviewer":       "[]",
	}
	// ...and when it is not, so the config value still applies.
	kept := map[string]string{
		"draft":          "true",
		"rebase":         "true",
		"diff-since-jip": "true",
		"upstream":       "git@github.com:fork/project.git",
		"reviewer":       "[alice,team/backend]",
	}
	for key, neg := range sendNegations {
		t.Run(key, func(t *testing.T) {
			flags := newSendFlags()
			if err := flags.Parse([]string{"--" + neg}); err != nil {
				t.Fatalf("parse --%s: %v", neg, err)
			}
			if err := applySendConfig(flags, cfg); err != nil {
				t.Fatalf("applySendConfig: %v", err)
			}
			if got := flags.Lookup(key).Value.String(); got != dropped[key] {
				t.Errorf("%s = %q, want %q", key, got, dropped[key])
			}
			// Only the negated key is dropped; the rest still come from config.
			for other := range cfg {
				if other == key {
					continue
				}
				if got := flags.Lookup(other).Value.String(); got != kept[other] {
					t.Errorf("%s = %q, want the configured %q", other, got, kept[other])
				}
			}
		})
	}
}

// --no-<key>=false is not a negation, so the config value still applies.
func TestApplySendConfig_ExplicitFalseNegation(t *testing.T) {
	flags := newSendFlags()
	if err := flags.Parse([]string{"--no-rebase=false"}); err != nil {
		t.Fatal(err)
	}
	if err := applySendConfig(flags, map[string]string{"rebase": "true"}); err != nil {
		t.Fatalf("applySendConfig: %v", err)
	}
	if got := flags.Lookup("rebase").Value.String(); got != "true" {
		t.Errorf("rebase = %q, want true", got)
	}
}

func TestCheckSendNegations(t *testing.T) {
	for key, neg := range sendNegations {
		t.Run(key, func(t *testing.T) {
			flags := newSendFlags()
			if err := flags.Parse([]string{"--" + neg}); err != nil {
				t.Fatal(err)
			}
			if err := checkSendNegations(flags); err != nil {
				t.Fatalf("--%s alone should be accepted: %v", neg, err)
			}
			if err := flags.Set(key, flagTestValue(flags, key)); err != nil {
				t.Fatal(err)
			}
			err := checkSendNegations(flags)
			if err == nil {
				t.Fatalf("expected error for --%s combined with --%s", key, neg)
			}
			if !strings.Contains(err.Error(), key) || !strings.Contains(err.Error(), neg) {
				t.Errorf("error should name both flags, got: %v", err)
			}
		})
	}
}

// flagTestValue returns a non-default value accepted by the named flag.
func flagTestValue(flags *pflag.FlagSet, name string) string {
	if flags.Lookup(name).Value.Type() == "bool" {
		return "true"
	}
	return "someone"
}

// Every negation must refer to a real config key and a real --no-<key> flag.
func TestSendNegations_MatchFlags(t *testing.T) {
	for key, neg := range sendNegations {
		if !sendConfigKeys[key] {
			t.Errorf("negation %q targets %q, which is not a config key", neg, key)
		}
		if sendCmd.Flags().Lookup(key) == nil {
			t.Errorf("negation %q targets %q, which has no send flag", neg, key)
		}
		if f := sendCmd.Flags().Lookup(neg); f == nil {
			t.Errorf("negation flag --%s is not registered", neg)
		} else if f.Value.Type() != "bool" {
			t.Errorf("negation flag --%s is %s, want bool", neg, f.Value.Type())
		}
	}
}

// Conversely, a config key whose "off" state has no value to type needs a
// negation, or it can only be cancelled as --key=false or an empty argument.
// no-stack is exempt: it is the deprecated alias for --stack=none, not a
// negation, and is cancelled by passing --stack.
func TestSendNegations_CoverTogglableConfigKeys(t *testing.T) {
	if neg, ok := sendNegations["stack"]; ok {
		t.Errorf("stack must have no negation, got %q: --no-stack is an alias for --stack=none", neg)
	}
	for key := range sendConfigKeys {
		f := sendCmd.Flags().Lookup(key)
		if key == "no-stack" || f == nil {
			continue // missing flags are reported by TestSendConfigKeys_MatchFlags
		}
		switch f.Value.Type() {
		case "bool", "stringSlice":
			if sendNegations[key] == "" {
				t.Errorf("config key %q is a %s and needs a --no-%s flag", key, f.Value.Type(), key)
			}
		}
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
