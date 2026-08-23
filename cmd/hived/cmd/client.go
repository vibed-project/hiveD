package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"connectrpc.com/connect"

	v1alpha1connect "github.com/vibed-project/hiveD/gen/hived/v1alpha1/hivedv1alpha1connect"
)

// authInterceptor attaches --token as a bearer token on every outbound
// request when set. An M0 Keeper's stub identity.Verifier ignores it.
//
// This implements connect.Interceptor rather than using
// connect.UnaryInterceptorFunc: that helper's WrapStreamingClient is a no-op,
// so the header was silently absent on watch streams and `hived watch` would
// have started failing auth on day one of M1.
type authInterceptor struct{ token string }

func (i authInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		req.Header().Set("Authorization", "Bearer "+i.token)
		return next(ctx, req)
	}
}

func (i authInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		conn.RequestHeader().Set("Authorization", "Bearer "+i.token)
		return conn
	}
}

func (i authInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next // client-side only
}

func clientOptions() []connect.ClientOption {
	if flags.token == "" {
		return nil
	}
	return []connect.ClientOption{connect.WithInterceptors(authInterceptor{token: flags.token})}
}

// httpClient bounds how long the CLI waits on a server that accepts the
// connection and then says nothing. Every client previously used
// http.DefaultClient (Timeout: 0) with a background context, so any command
// against a wedged Keeper hung forever, which is fatal in CI or cron.
//
// The bound is on dial and response headers rather than the whole exchange:
// a total timeout would also kill long-lived `watch` streams, which are
// supposed to stay open. Unary commands additionally get a context deadline
// (see commandContext), which does bound the full request.
func httpClient() *http.Client {
	if flags.timeout <= 0 {
		return http.DefaultClient
	}
	return &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: flags.timeout}).DialContext,
			ResponseHeaderTimeout: flags.timeout,
			ForceAttemptHTTP2:     true,
		},
	}
}

// commandContext applies --timeout to a unary command. Streaming commands
// (watch) deliberately do not use it; they rely on httpClient's
// response-header bound plus Ctrl-C.
func commandContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if flags.timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, flags.timeout)
}

func colonyClient() v1alpha1connect.ColonyServiceClient {
	return v1alpha1connect.NewColonyServiceClient(httpClient(), flags.server, clientOptions()...)
}

func agentClient() v1alpha1connect.AgentServiceClient {
	return v1alpha1connect.NewAgentServiceClient(httpClient(), flags.server, clientOptions()...)
}

func agentVersionClient() v1alpha1connect.AgentVersionServiceClient {
	return v1alpha1connect.NewAgentVersionServiceClient(httpClient(), flags.server, clientOptions()...)
}

func runClient() v1alpha1connect.RunServiceClient {
	return v1alpha1connect.NewRunServiceClient(httpClient(), flags.server, clientOptions()...)
}

func toolClient() v1alpha1connect.ToolServiceClient {
	return v1alpha1connect.NewToolServiceClient(httpClient(), flags.server, clientOptions()...)
}

func eventClient() v1alpha1connect.EventServiceClient {
	return v1alpha1connect.NewEventServiceClient(httpClient(), flags.server, clientOptions()...)
}

func notImplementedM1(what string) error {
	return fmt.Errorf("%s is not implemented until M1 (no Scheduler yet) — see CLAUDE.md for the current milestone scope", what)
}
