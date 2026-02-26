package cmd

import (
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage GitHub authentication",
}

func init() {
	rootCmd.AddCommand(authCmd)
}
