package cmd

import (
	"fmt"

	"github.com/cli/oauth"
	"github.com/omarkohl/jip/internal/auth"
	"github.com/spf13/cobra"
)

// The "jip" OAuth app
// These values are safe to be embedded in version control.
// See also: https://github.com/cli/cli/blob/cf862d65df7f8ff528015e235c8cccd48cea286f/internal/authflow/flow.go#L22
var (
	oauthClientID     = "Ov23liy1wX3wH8zYSRgs"
	oauthClientSecret = "77953a22f359540edb17947bd7ef428af82be4d3"
)

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with GitHub using OAuth device flow",
	RunE:  runAuthLogin,
}

func init() {
	authCmd.AddCommand(authLoginCmd)
}

func runAuthLogin(cmd *cobra.Command, args []string) error {
	host, err := oauth.NewGitHubHost("https://github.com")
	if err != nil {
		return fmt.Errorf("initializing OAuth host: %w", err)
	}

	flow := &oauth.Flow{
		Host:         host,
		ClientID:     oauthClientID,
		ClientSecret: oauthClientSecret,
		Scopes:       []string{"repo"},
	}

	token, err := flow.DetectFlow()
	if err != nil {
		return fmt.Errorf("OAuth device flow failed: %w", err)
	}

	if err := auth.SaveToken(defaultHost, token.Token); err != nil {
		return fmt.Errorf("failed to save token: %w", err)
	}

	_, err = fmt.Fprintln(cmd.OutOrStdout(), "Authentication successful! Token saved.")
	return err
}
