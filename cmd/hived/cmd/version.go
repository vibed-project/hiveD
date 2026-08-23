package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/vibed-project/hiveD/internal/version"
)

// newVersionCmd mirrors `hived-keeper version` so a released CLI binary can
// identify itself. The format is kept identical to the Keeper's so the two
// can be compared at a glance when debugging a version skew.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the hived CLI build information.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "hived %s (commit %s, built %s)\n",
				version.Version, version.Commit, version.BuildDate)
			return err
		},
	}
}
