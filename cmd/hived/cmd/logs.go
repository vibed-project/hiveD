package cmd

import "github.com/spf13/cobra"

func newLogsCmd() *cobra.Command {
	var follow bool
	c := &cobra.Command{
		Use:   "logs <run>",
		Short: "Stream a Run's Cell logs. Not implemented until M1.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return notImplementedM1("logs")
		},
	}
	c.Flags().BoolVar(&follow, "follow", false, "follow the log stream (reserved for M1)")
	return c
}
