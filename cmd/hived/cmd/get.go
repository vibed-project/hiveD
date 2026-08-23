package cmd

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"

	v1alpha1 "github.com/vibed-project/hiveD/gen/hived/v1alpha1"
)

func newGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <kind> [name]",
		Short: "Get a single resource by name, or list all resources of a kind.",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			kind, err := resolveKind(args[0])
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			if len(args) == 2 {
				return getOne(ctx, kind, args[1])
			}
			return listAll(ctx, kind)
		},
	}
}

func getOne(ctx context.Context, kind, name string) error {
	switch kind {
	case "Colony":
		resp, err := colonyClient().Get(ctx, connect.NewRequest(&v1alpha1.GetColonyRequest{Name: name}))
		if err != nil {
			return err
		}
		return printObject(resp.Msg)
	case "Agent":
		resp, err := agentClient().Get(ctx, connect.NewRequest(&v1alpha1.GetAgentRequest{Colony: flags.colony, Name: name}))
		if err != nil {
			return err
		}
		return printObject(resp.Msg)
	case "AgentVersion":
		resp, err := agentVersionClient().Get(ctx, connect.NewRequest(&v1alpha1.GetAgentVersionRequest{Colony: flags.colony, Name: name}))
		if err != nil {
			return err
		}
		return printObject(resp.Msg)
	case "Run":
		resp, err := runClient().Get(ctx, connect.NewRequest(&v1alpha1.GetRunRequest{Colony: flags.colony, Name: name}))
		if err != nil {
			return err
		}
		return printObject(resp.Msg)
	case "Tool":
		resp, err := toolClient().Get(ctx, connect.NewRequest(&v1alpha1.GetToolRequest{Colony: flags.colony, Name: name}))
		if err != nil {
			return err
		}
		return printObject(resp.Msg)
	default:
		return fmt.Errorf("unknown kind %q", kind)
	}
}

func listAll(ctx context.Context, kind string) error {
	opts := &v1alpha1.ListOptions{Colony: flags.colony}

	switch kind {
	case "Colony":
		resp, err := colonyClient().List(ctx, connect.NewRequest(&v1alpha1.ListColoniesRequest{Options: opts}))
		if err != nil {
			return err
		}
		if flags.output != "table" {
			for _, item := range resp.Msg.Items {
				if err := printObject(item); err != nil {
					return err
				}
			}
			return nil
		}
		rows := make([][]string, 0, len(resp.Msg.Items))
		for _, c := range resp.Msg.Items {
			rows = append(rows, []string{c.Metadata.Name, c.Spec.DisplayName, fmtInt(c.Metadata.Generation)})
		}
		printTable([]string{"NAME", "DISPLAY NAME", "GENERATION"}, rows)
		return nil

	case "Agent":
		resp, err := agentClient().List(ctx, connect.NewRequest(&v1alpha1.ListAgentsRequest{Options: opts}))
		if err != nil {
			return err
		}
		if flags.output != "table" {
			for _, item := range resp.Msg.Items {
				if err := printObject(item); err != nil {
					return err
				}
			}
			return nil
		}
		rows := make([][]string, 0, len(resp.Msg.Items))
		for _, a := range resp.Msg.Items {
			rows = append(rows, []string{a.Metadata.Colony, a.Metadata.Name, a.Status.Current})
		}
		printTable([]string{"COLONY", "NAME", "CURRENT VERSION"}, rows)
		return nil

	case "AgentVersion":
		resp, err := agentVersionClient().List(ctx, connect.NewRequest(&v1alpha1.ListAgentVersionsRequest{Options: opts}))
		if err != nil {
			return err
		}
		if flags.output != "table" {
			for _, item := range resp.Msg.Items {
				if err := printObject(item); err != nil {
					return err
				}
			}
			return nil
		}
		rows := make([][]string, 0, len(resp.Msg.Items))
		for _, av := range resp.Msg.Items {
			rows = append(rows, []string{av.Metadata.Colony, av.Metadata.Name, av.Spec.Agent, av.Spec.Version})
		}
		printTable([]string{"COLONY", "NAME", "AGENT", "VERSION"}, rows)
		return nil

	case "Run":
		resp, err := runClient().List(ctx, connect.NewRequest(&v1alpha1.ListRunsRequest{Options: opts}))
		if err != nil {
			return err
		}
		if flags.output != "table" {
			for _, item := range resp.Msg.Items {
				if err := printObject(item); err != nil {
					return err
				}
			}
			return nil
		}
		rows := make([][]string, 0, len(resp.Msg.Items))
		for _, r := range resp.Msg.Items {
			rows = append(rows, []string{r.Metadata.Colony, r.Metadata.Name, r.Spec.AgentRef, r.Status.Phase.String()})
		}
		printTable([]string{"COLONY", "NAME", "AGENT", "PHASE"}, rows)
		return nil

	case "Tool":
		resp, err := toolClient().List(ctx, connect.NewRequest(&v1alpha1.ListToolsRequest{Options: opts}))
		if err != nil {
			return err
		}
		if flags.output != "table" {
			for _, item := range resp.Msg.Items {
				if err := printObject(item); err != nil {
					return err
				}
			}
			return nil
		}
		rows := make([][]string, 0, len(resp.Msg.Items))
		for _, t := range resp.Msg.Items {
			rows = append(rows, []string{t.Metadata.Colony, t.Metadata.Name, t.Spec.Type.String(), t.Spec.RiskClass})
		}
		printTable([]string{"COLONY", "NAME", "TYPE", "RISK"}, rows)
		return nil

	default:
		return fmt.Errorf("unknown kind %q", kind)
	}
}

func fmtInt(v int64) string { return fmt.Sprintf("%d", v) }
