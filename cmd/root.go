package cmd

import (
	"github.com/spf13/cobra"
)

// version is set at build time via -ldflags "-X github.com/omarkohl/jip/cmd.version=..."
var version = "0.1.0-dev"

var rootCmd = &cobra.Command{
	Use:     "jip",
	Short:   "Stacked PRs for jj and GitHub",
	Version: version,
}

func Execute() error {
	return rootCmd.Execute()
}
