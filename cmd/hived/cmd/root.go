// Package cmd implements the hived CLI's command tree with Cobra.
package cmd

import (
	"time"

	"github.com/spf13/cobra"
)

// flags holds the persistent flag values shared by every subcommand. A
// package-level struct is simplest for a CLI this size; see client.go for
// how it's turned into connect clients.
var flags struct {
	server  string
	colony  string
	token   string
	output  string
	timeout time.Duration
}

func Root() *cobra.Command {
	root := &cobra.Command{
		Use:   "hived",
		Short: "hived drives a hiveD Keeper: apply resources, inspect Runs, watch events.",
		// main() prints the error and sets the exit code; leaving Cobra to
		// print it as well produced every message twice.
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			return validateOutput()
		},
	}

	root.PersistentFlags().StringVar(&flags.server, "server", "http://localhost:8080", "hiveD Keeper base URL")
	root.PersistentFlags().StringVar(&flags.colony, "colony", "", "colony to scope colony-scoped commands to")
	root.PersistentFlags().StringVar(&flags.token, "token", "", "bearer token (ignored by an M0 Keeper's stub auth, but the flag is wired for M1)")
	root.PersistentFlags().StringVarP(&flags.output, "output", "o", "table", "output format: table, json, or yaml")
	root.PersistentFlags().DurationVar(&flags.timeout, "timeout", 30*time.Second,
		"per-request timeout; 0 disables it. Streaming commands (watch) are not bounded by this, only their initial response.")

	root.AddCommand(
		newApplyCmd(),
		newGetCmd(),
		newWatchCmd(),
		newEventsCmd(),
		newRunCmd(),
		newLogsCmd(),
		newApproveCmd(),
		newVersionCmd(),
	)
	return root
}
