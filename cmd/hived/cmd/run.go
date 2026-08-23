package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/structpb"

	v1alpha1 "github.com/vibed-project/hiveD/gen/hived/v1alpha1"
)

func newRunCmd() *cobra.Command {
	var name, input string
	c := &cobra.Command{
		Use:   "run <agent>",
		Short: "Create a Run for an Agent. It will stay PENDING: there is no Scheduler until M1.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.colony == "" {
				return fmt.Errorf("--colony is required")
			}
			if name == "" {
				return fmt.Errorf("--name is required (Run names must be unique within a colony)")
			}

			var inputStruct *structpb.Struct
			if input != "" {
				var m map[string]any
				if err := json.Unmarshal([]byte(input), &m); err != nil {
					return fmt.Errorf("--input is not valid JSON: %w", err)
				}
				s, err := structpb.NewStruct(m)
				if err != nil {
					return err
				}
				inputStruct = s
			}

			resp, err := runClient().Apply(cmd.Context(), connect.NewRequest(&v1alpha1.ApplyRunRequest{
				Object: &v1alpha1.Run{
					Metadata: &v1alpha1.ObjectMeta{Colony: flags.colony, Name: name},
					Spec:     &v1alpha1.RunSpec{AgentRef: args[0], Input: inputStruct},
				},
			}))
			if err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "warning: Run created as PENDING but nothing will schedule it — there is no Scheduler until M1")
			return printObject(resp.Msg)
		},
	}
	c.Flags().StringVar(&name, "name", "", "name for the new Run (required)")
	c.Flags().StringVar(&input, "input", "", "JSON object passed as the Run's input")
	return c
}
