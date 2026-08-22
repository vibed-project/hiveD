package cmd

import "github.com/spf13/cobra"

func newApproveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "approve <run>",
		Short: "Approve a pending tool call for a Run. Not implemented until M1 (no Approval resource or Tool Broker yet).",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return notImplementedM1("approve")
		},
	}
}
