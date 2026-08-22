package cmd

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"

	v1alpha1 "github.com/hived-project/hived/gen/hived/v1alpha1"
)

func newApplyCmd() *cobra.Command {
	var file string
	c := &cobra.Command{
		Use:   "apply -f FILE|DIR|-",
		Short: "Create or update resources from a manifest (apiVersion/kind/metadata/spec).",
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return fmt.Errorf("-f is required")
			}
			manifests, err := readManifests(file)
			if err != nil {
				return err
			}
			if len(manifests) == 0 {
				return fmt.Errorf("%s: no manifests found", file)
			}
			for _, m := range manifests {
				if err := applyOne(cmd.Context(), m); err != nil {
					return fmt.Errorf("%s %q: %w", m.Kind, m.JSON, err)
				}
			}
			return nil
		},
	}
	c.Flags().StringVarP(&file, "filename", "f", "", "manifest file, directory, or - for stdin")
	return c
}

var unmarshalOpts = protojson.UnmarshalOptions{DiscardUnknown: true}

func applyOne(ctx context.Context, m manifest) error {
	switch m.Kind {
	case "Colony":
		obj := &v1alpha1.Colony{}
		if err := unmarshalOpts.Unmarshal(m.JSON, obj); err != nil {
			return err
		}
		resp, err := colonyClient().Apply(ctx, connect.NewRequest(&v1alpha1.ApplyColonyRequest{Object: obj}))
		if err != nil {
			return err
		}
		return printObject(resp.Msg)

	case "Agent":
		obj := &v1alpha1.Agent{}
		if err := unmarshalOpts.Unmarshal(m.JSON, obj); err != nil {
			return err
		}
		resp, err := agentClient().Apply(ctx, connect.NewRequest(&v1alpha1.ApplyAgentRequest{Object: obj}))
		if err != nil {
			return err
		}
		return printObject(resp.Msg)

	case "AgentVersion":
		obj := &v1alpha1.AgentVersion{}
		if err := unmarshalOpts.Unmarshal(m.JSON, obj); err != nil {
			return err
		}
		resp, err := agentVersionClient().Apply(ctx, connect.NewRequest(&v1alpha1.ApplyAgentVersionRequest{Object: obj}))
		if err != nil {
			return err
		}
		return printObject(resp.Msg)

	case "Run":
		obj := &v1alpha1.Run{}
		if err := unmarshalOpts.Unmarshal(m.JSON, obj); err != nil {
			return err
		}
		resp, err := runClient().Apply(ctx, connect.NewRequest(&v1alpha1.ApplyRunRequest{Object: obj}))
		if err != nil {
			return err
		}
		return printObject(resp.Msg)

	case "Tool":
		obj := &v1alpha1.Tool{}
		if err := unmarshalOpts.Unmarshal(m.JSON, obj); err != nil {
			return err
		}
		resp, err := toolClient().Apply(ctx, connect.NewRequest(&v1alpha1.ApplyToolRequest{Object: obj}))
		if err != nil {
			return err
		}
		return printObject(resp.Msg)

	default:
		return fmt.Errorf("unknown kind %q (want Colony, Agent, AgentVersion, Run, or Tool)", m.Kind)
	}
}
