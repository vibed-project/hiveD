package cmd

import (
	"fmt"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	v1alpha1 "github.com/vibed-project/hiveD/gen/hived/v1alpha1"
)

func newEventsCmd() *cobra.Command {
	var sinceSeq int64
	c := &cobra.Command{
		Use:   "events <run>",
		Short: "List events recorded for a Run.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.colony == "" {
				return fmt.Errorf("--colony is required")
			}
			ctx, cancel := commandContext(cmd.Context())
			defer cancel()
			resp, err := eventClient().List(ctx, connect.NewRequest(&v1alpha1.ListEventsRequest{
				Colony:   flags.colony,
				Run:      args[0],
				SinceSeq: sinceSeq,
			}))
			if err != nil {
				return err
			}
			if flags.output != "table" {
				for _, e := range resp.Msg.Items {
					if err := printObject(e); err != nil {
						return err
					}
				}
				return nil
			}
			rows := make([][]string, 0, len(resp.Msg.Items))
			for _, e := range resp.Msg.Items {
				rows = append(rows, []string{fmtInt(e.Seq), e.Type, e.Ts.AsTime().Format("15:04:05.000")})
			}
			printTable([]string{"SEQ", "TYPE", "TIME"}, rows)
			return nil
		},
	}
	c.Flags().Int64Var(&sinceSeq, "since-seq", 0, "only list events with seq greater than this")
	return c
}
