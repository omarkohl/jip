package cmd

import (
	"context"
	"fmt"

	"github.com/google/go-github/v68/github"
	"github.com/omarkohl/jip/internal/auth"
	"github.com/spf13/cobra"
)

const defaultHost = "github.com"

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current authentication status",
	RunE:  runAuthStatus,
}

func init() {
	authCmd.AddCommand(authStatusCmd)
}

func runAuthStatus(cmd *cobra.Command, args []string) error {
	token, source := auth.ResolveToken(defaultHost)
	if token == "" {
		return fmt.Errorf("not authenticated. Run 'jip auth login' or 'gh auth login' or set GH_TOKEN")
	}

	client := github.NewClient(nil).WithAuthToken(token)
	user, _, err := client.Users.Get(context.Background(), "")
	if err != nil {
		return fmt.Errorf("token invalid: %w", err)
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Authenticated as %s (via %s)\n", user.GetLogin(), source)
	return err
}
