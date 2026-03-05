package cmd

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

var debugFlag bool

var rootCmd = &cobra.Command{
	Use:           "jip",
	Short:         "jip " + buildVersion() + " — Stacked PRs for jj and GitHub",
	Version:       buildVersion(),
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRun: func(_ *cobra.Command, _ []string) {
		level := slog.LevelWarn
		if debugFlag || os.Getenv("JIP_DEBUG") != "" {
			level = slog.LevelDebug
		}
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: level,
		})))
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&debugFlag, "debug", false, "enable debug logging to stderr")
}

func Execute() error {
	return rootCmd.Execute()
}
