package cmd

import (
	"context"
	"fmt"
	"net/http"

	"connectrpc.com/connect"

	v1alpha1connect "github.com/vibed-project/hiveD/gen/hived/v1alpha1/hivedv1alpha1connect"
)

// clientOptions attaches --token as a bearer token on every outbound
// request when set. An M0 Keeper's stub identity.Verifier ignores it; the
// flag and this interceptor exist so nothing changes client-side once M1
// ships real token issuance and verification.
func clientOptions() []connect.ClientOption {
	if flags.token == "" {
		return nil
	}
	return []connect.ClientOption{
		connect.WithInterceptors(connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				req.Header().Set("Authorization", "Bearer "+flags.token)
				return next(ctx, req)
			}
		})),
	}
}

func colonyClient() v1alpha1connect.ColonyServiceClient {
	return v1alpha1connect.NewColonyServiceClient(http.DefaultClient, flags.server, clientOptions()...)
}

func agentClient() v1alpha1connect.AgentServiceClient {
	return v1alpha1connect.NewAgentServiceClient(http.DefaultClient, flags.server, clientOptions()...)
}

func agentVersionClient() v1alpha1connect.AgentVersionServiceClient {
	return v1alpha1connect.NewAgentVersionServiceClient(http.DefaultClient, flags.server, clientOptions()...)
}

func runClient() v1alpha1connect.RunServiceClient {
	return v1alpha1connect.NewRunServiceClient(http.DefaultClient, flags.server, clientOptions()...)
}

func toolClient() v1alpha1connect.ToolServiceClient {
	return v1alpha1connect.NewToolServiceClient(http.DefaultClient, flags.server, clientOptions()...)
}

func eventClient() v1alpha1connect.EventServiceClient {
	return v1alpha1connect.NewEventServiceClient(http.DefaultClient, flags.server, clientOptions()...)
}

func notImplementedM1(what string) error {
	return fmt.Errorf("%s is not implemented until M1 (no Scheduler yet) — see CLAUDE.md for the current milestone scope", what)
}
