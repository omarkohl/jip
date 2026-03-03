package cmd

import (
	"bufio"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

// completeJJRevsets returns a ValidArgsFunction that completes jj revsets
// by delegating to jj's built-in completion protocol.
func completeJJRevsets(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return jjComplete([]string{"log", "-r", toComplete}, "")
}

// completeJJBookmarks returns a ValidArgsFunction that completes jj bookmark
// names by delegating to jj's built-in completion protocol.
func completeJJBookmarks(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return jjComplete([]string{"bookmark", "set", toComplete}, "--")
}

// jjComplete invokes jj with COMPLETE=fish and parses the tab-separated
// output into cobra completions. The reason for using fish is that the output
// is easier to parse. Lines starting with filterPrefix are excluded (e.g. "--"
// to skip flag suggestions).
func jjComplete(simArgs []string, filterPrefix string) ([]string, cobra.ShellCompDirective) {
	cmdArgs := append([]string{"--"}, append([]string{"jj"}, simArgs...)...)
	cmd := exec.Command("jj", cmdArgs...)
	cmd.Env = append(os.Environ(), "COMPLETE=fish")

	out, err := cmd.Output()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	var completions []string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		if filterPrefix != "" && strings.HasPrefix(line, filterPrefix) {
			continue
		}
		completions = append(completions, line)
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}
