package api

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/vibed-project/hiveD/internal/identity"
)

var rpcTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "hived_keeper_rpc_total",
	Help: "Total connect-go RPCs handled by the Keeper, by procedure and status code.",
}, []string{"procedure", "code"})

// serverInterceptor is the single connect.Interceptor mounted on every
// Keeper service: it authenticates (via the stub identity.Verifier in
// M0 — see internal/identity), recovers panics as CodeInternal instead of
// crashing the process, logs, and records the hived_keeper_rpc_total
// metric. One implementation covers unary and streaming RPCs so this
// logic exists exactly once.
type serverInterceptor struct {
	logger   *slog.Logger
	verifier identity.Verifier
}

func newServerInterceptor(logger *slog.Logger, verifier identity.Verifier) *serverInterceptor {
	return &serverInterceptor{logger: logger, verifier: verifier}
}

func (si *serverInterceptor) authenticate(ctx context.Context, authHeader string) (context.Context, error) {
	token := strings.TrimPrefix(authHeader, "Bearer ")
	principal, err := si.verifier.Verify(ctx, token)
	if err != nil {
		return ctx, connect.NewError(connect.CodeUnauthenticated, err)
	}
	return identity.WithPrincipal(ctx, principal), nil
}

func (si *serverInterceptor) observe(procedure string, err error) {
	code := "ok"
	if err != nil {
		code = connect.CodeOf(err).String()
	}
	rpcTotal.WithLabelValues(procedure, code).Inc()
}

func (si *serverInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (resp connect.AnyResponse, err error) {
		procedure := req.Spec().Procedure
		defer func() {
			if r := recover(); r != nil {
				err = connect.NewError(connect.CodeInternal, fmt.Errorf("panic: %v", r))
				si.logger.Error("panic in rpc handler", "procedure", procedure, "panic", r)
			}
			si.observe(procedure, err)
		}()

		ctx, authErr := si.authenticate(ctx, req.Header().Get("Authorization"))
		if authErr != nil {
			return nil, authErr
		}

		start := time.Now()
		resp, err = next(ctx, req)
		si.logger.Debug("rpc", "procedure", procedure, "duration", time.Since(start), "error", err)
		return resp, err
	}
}

func (si *serverInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next // this Keeper never acts as a connect client; passthrough satisfies the interface.
}

func (si *serverInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) (err error) {
		procedure := conn.Spec().Procedure
		defer func() {
			if r := recover(); r != nil {
				err = connect.NewError(connect.CodeInternal, fmt.Errorf("panic: %v", r))
				si.logger.Error("panic in streaming rpc handler", "procedure", procedure, "panic", r)
			}
			si.observe(procedure, err)
		}()

		ctx, authErr := si.authenticate(ctx, conn.RequestHeader().Get("Authorization"))
		if authErr != nil {
			return authErr
		}

		start := time.Now()
		err = next(ctx, conn)
		si.logger.Debug("rpc stream ended", "procedure", procedure, "duration", time.Since(start), "error", err)
		return err
	}
}

var _ connect.Interceptor = (*serverInterceptor)(nil)
