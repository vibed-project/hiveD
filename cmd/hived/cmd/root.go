// Package cmd implements the hived CLI's command tree with Cobra.
package cmd

import (
	"github.com/spf13/cobra"
)

// flags holds the persistent flag values shared by every subcommand. A
// package-level struct is simplest for a CLI this size; see client.go for
// how it's turned into connect clients.
var flags struct {
	server string
	colony string
	token  string
	output string
}

func Root() *cobra.Command {
	root := &cobra.Command{
		Use:           "hived",
		Short:         "hived drives a hiveD Keeper: apply resources, inspect Runs, watch events.",
		SilenceUsage:  true,
		SilenceErrors: false,
	}

	root.PersistentFlags().StringVar(&flags.server, "server", "http://localhost:8080", "hiveD Keeper base URL")
	root.PersistentFlags().StringVar(&flags.colony, "colony", "", "colony to scope colony-scoped commands to")
	root.PersistentFlags().StringVar(&flags.token, "token", "", "bearer token (ignored by an M0 Keeper's stub auth, but the flag is wired for M1)")
	root.PersistentFlags().StringVarP(&flags.output, "output", "o", "table", "output format: table, json, or yaml")

	root.AddCommand(
		newApplyCmd(),
		newGetCmd(),
		newWatchCmd(),
		newEventsCmd(),
		newRunCmd(),
		newLogsCmd(),
		newApproveCmd(),
	)
	return root
}
