package cmd

import (
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

// Version is set at build time via goreleaser ldflags:
//
//	-X github.com/omarkohl/jip/cmd.Version=0.2.0
var Version = "dev"

// buildVersion returns the version string shown by --version.
//
// Two build scenarios:
//  1. goreleaser: Version is set via ldflags → return it directly.
//  2. All other builds (go install @version, go build): Version is "dev",
//     debug.BuildInfo provides the module version → strip "v" prefix.
func buildVersion() string {
	if Version != "dev" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		v := info.Main.Version
		if v != "" && v != "(devel)" {
			return strings.TrimPrefix(v, "v")
		}
	}
	return Version
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
