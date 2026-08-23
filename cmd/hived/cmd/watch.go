package cmd

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	v1alpha1 "github.com/vibed-project/hiveD/gen/hived/v1alpha1"
)

func newWatchCmd() *cobra.Command {
	var since int64
	c := &cobra.Command{
		Use:   "watch <kind>",
		Short: "Stream ADDED/MODIFIED/DELETED events for a resource kind.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			kind, err := resolveKind(args[0])
			if err != nil {
				return err
			}
			return watchKind(cmd.Context(), kind, since)
		},
	}
	c.Flags().Int64Var(&since, "since-resource-version", 0, "only stream events after this resource_version")
	return c
}

func watchKind(ctx context.Context, kind string, since int64) error {
	opts := &v1alpha1.ListOptions{Colony: flags.colony}

	switch kind {
	case "Colony":
		stream, err := colonyClient().Watch(ctx, connect.NewRequest(&v1alpha1.WatchColoniesRequest{Options: opts, SinceResourceVersion: since}))
		if err != nil {
			return err
		}
		for stream.Receive() {
			ev := stream.Msg()
			name := ""
			if ev.Object != nil {
				name = ev.Object.Metadata.Name
			}
			fmt.Printf("%s\t%d\t%s\n", ev.Type, ev.ResourceVersion, name)
		}
		return stream.Err()

	case "Agent":
		stream, err := agentClient().Watch(ctx, connect.NewRequest(&v1alpha1.WatchAgentsRequest{Options: opts, SinceResourceVersion: since}))
		if err != nil {
			return err
		}
		for stream.Receive() {
			ev := stream.Msg()
			name := ""
			if ev.Object != nil {
				name = ev.Object.Metadata.Colony + "/" + ev.Object.Metadata.Name
			}
			fmt.Printf("%s\t%d\t%s\n", ev.Type, ev.ResourceVersion, name)
		}
		return stream.Err()

	case "AgentVersion":
		stream, err := agentVersionClient().Watch(ctx, connect.NewRequest(&v1alpha1.WatchAgentVersionsRequest{Options: opts, SinceResourceVersion: since}))
		if err != nil {
			return err
		}
		for stream.Receive() {
			ev := stream.Msg()
			name := ""
			if ev.Object != nil {
				name = ev.Object.Metadata.Colony + "/" + ev.Object.Metadata.Name
			}
			fmt.Printf("%s\t%d\t%s\n", ev.Type, ev.ResourceVersion, name)
		}
		return stream.Err()

	case "Run":
		stream, err := runClient().Watch(ctx, connect.NewRequest(&v1alpha1.WatchRunsRequest{Options: opts, SinceResourceVersion: since}))
		if err != nil {
			return err
		}
		for stream.Receive() {
			ev := stream.Msg()
			name := ""
			if ev.Object != nil {
				name = ev.Object.Metadata.Colony + "/" + ev.Object.Metadata.Name
			}
			fmt.Printf("%s\t%d\t%s\n", ev.Type, ev.ResourceVersion, name)
		}
		return stream.Err()

	case "Tool":
		stream, err := toolClient().Watch(ctx, connect.NewRequest(&v1alpha1.WatchToolsRequest{Options: opts, SinceResourceVersion: since}))
		if err != nil {
			return err
		}
		for stream.Receive() {
			ev := stream.Msg()
			name := ""
			if ev.Object != nil {
				name = ev.Object.Metadata.Colony + "/" + ev.Object.Metadata.Name
			}
			fmt.Printf("%s\t%d\t%s\n", ev.Type, ev.ResourceVersion, name)
		}
		return stream.Err()

	default:
		return fmt.Errorf("unknown kind %q", kind)
	}
}
