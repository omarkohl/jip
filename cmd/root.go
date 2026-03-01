package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "jip",
	Short:         "jip " + buildVersion() + " — Stacked PRs for jj and GitHub",
	Version:       buildVersion(),
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() error {
	return rootCmd.Execute()
}
