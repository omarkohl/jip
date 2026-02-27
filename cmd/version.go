package cmd

import (
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

// These variables are set at build time via -ldflags.
//
//	go build -ldflags "-X github.com/omarkohl/jip/cmd.Version=0.2.0 -X github.com/omarkohl/jip/cmd.Commit=abc123 -X github.com/omarkohl/jip/cmd.Date=2026-02-27"
var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

// buildVersion returns the full version string shown by --version.
// It tries ldflags first, then falls back to debug.BuildInfo for
// go install / go run builds.
func buildVersion() string {
	commit := Commit
	date := Date
	dirty := false

	// Fall back to debug.BuildInfo when ldflags are not set.
	if commit == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, s := range info.Settings {
				switch s.Key {
				case "vcs.revision":
					commit = s.Value
				case "vcs.time":
					date = s.Value
				case "vcs.modified":
					dirty = s.Value == "true"
				}
			}
		}
	}

	if commit == "" {
		return Version
	}

	// Shorten the commit hash to 7 characters.
	if len(commit) > 7 {
		commit = commit[:7]
	}

	if dirty {
		commit += "-dirty"
	}

	var parts []string
	parts = append(parts, commit)
	if date != "" {
		parts = append(parts, date)
	}

	return fmt.Sprintf("%s (%s)", Version, strings.Join(parts, ", "))
}

func init() {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("jip version " + buildVersion())
		},
	})
}
