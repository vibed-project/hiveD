package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"

	v1alpha1connect "github.com/hived-project/hived/gen/hived/v1alpha1/hivedv1alpha1connect"
	"github.com/hived-project/hived/internal/identity"
	"github.com/hived-project/hived/internal/store"
)

// ReadyFunc reports whether the Keeper is ready to serve (e.g. the
// database pool is reachable). Returning an error fails /readyz.
type ReadyFunc func(ctx context.Context) error

// NewHandler builds the Keeper's API mux: the six resource connect-go
// services plus /healthz and /readyz. /metrics is intentionally not mounted here —
// see internal/obs and cmd/keeper, which serve it on a separate port so
// scraping never shares a listener with application traffic.
//
// The returned handler expects to be served by an *http.Server with its
// Protocols field configured for unencrypted HTTP/2 (see cmd/keeper), so
// gRPC works without TLS in the compose dev stack.
func NewHandler(rs store.ResourceStore, es store.EventStore, verifier identity.Verifier, logger *slog.Logger, ready ReadyFunc) http.Handler {
	interceptor := connect.WithInterceptors(newServerInterceptor(logger, verifier))

	mux := http.NewServeMux()
	mux.Handle(v1alpha1connect.NewColonyServiceHandler(NewColonyHandler(rs), interceptor))
	mux.Handle(v1alpha1connect.NewAgentServiceHandler(NewAgentHandler(rs), interceptor))
	mux.Handle(v1alpha1connect.NewAgentVersionServiceHandler(NewAgentVersionHandler(rs), interceptor))
	mux.Handle(v1alpha1connect.NewRunServiceHandler(NewRunHandler(rs), interceptor))
	mux.Handle(v1alpha1connect.NewToolServiceHandler(NewToolHandler(rs), interceptor))
	mux.Handle(v1alpha1connect.NewEventServiceHandler(NewEventHandler(es), interceptor))

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if ready != nil {
			if err := ready(r.Context()); err != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not ready", "error": err.Error()})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	return mux
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
