package api

import (
	"context"

	"connectrpc.com/connect"

	v1alpha1 "github.com/hived-project/hived/gen/hived/v1alpha1"
	"github.com/hived-project/hived/internal/store"
)

// RunHandler implements hivedv1alpha1connect.RunServiceHandler. Apply
// persists a Run as RUN_PHASE_PENDING; nothing schedules it — there is no
// Scheduler until M1. Callers should expect a Run to stay PENDING forever
// against an M0 Keeper.
type RunHandler struct {
	store store.ResourceStore
}

func NewRunHandler(s store.ResourceStore) *RunHandler {
	return &RunHandler{store: s}
}

func runToResource(r *v1alpha1.Run) (store.Resource, error) {
	res, err := resourceFromMeta("Run", r.GetMetadata())
	if err != nil {
		return store.Resource{}, err
	}
	if res.Colony == "" {
		return store.Resource{}, invalidArgument("api: metadata.colony is required")
	}
	if r.GetSpec().GetAgentRef() == "" {
		return store.Resource{}, invalidArgument("api: spec.agent_ref is required")
	}
	spec, err := marshalSpec(r.GetSpec())
	if err != nil {
		return store.Resource{}, err
	}
	res.Spec = spec

	// A freshly-applied Run always starts PENDING; the caller does not get
	// to set an initial phase. There is no Scheduler in M0 to advance it.
	status, err := marshalSpec(&v1alpha1.RunStatus{Phase: v1alpha1.RunPhase_RUN_PHASE_PENDING})
	if err != nil {
		return store.Resource{}, err
	}
	res.Status = status
	return res, nil
}

func resourceToRun(r store.Resource) (*v1alpha1.Run, error) {
	spec := &v1alpha1.RunSpec{}
	if err := unmarshalSpec(r.Spec, spec); err != nil {
		return nil, err
	}
	status := &v1alpha1.RunStatus{}
	if err := unmarshalSpec(r.Status, status); err != nil {
		return nil, err
	}
	return &v1alpha1.Run{Metadata: toObjectMeta(r), Spec: spec, Status: status}, nil
}

func (h *RunHandler) Apply(ctx context.Context, req *connect.Request[v1alpha1.ApplyRunRequest]) (*connect.Response[v1alpha1.Run], error) {
	obj := req.Msg.GetObject()
	if obj == nil {
		return nil, invalidArgument("api: object is required")
	}
	r, err := runToResource(obj)
	if err != nil {
		return nil, err
	}
	saved, err := h.store.Apply(ctx, r, req.Msg.IfResourceVersion)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	out, err := resourceToRun(saved)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(out), nil
}

func (h *RunHandler) Get(ctx context.Context, req *connect.Request[v1alpha1.GetRunRequest]) (*connect.Response[v1alpha1.Run], error) {
	r, err := h.store.Get(ctx, "Run", req.Msg.Colony, req.Msg.Name)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	out, err := resourceToRun(r)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(out), nil
}

func (h *RunHandler) List(ctx context.Context, req *connect.Request[v1alpha1.ListRunsRequest]) (*connect.Response[v1alpha1.ListRunsResponse], error) {
	res, err := h.store.List(ctx, "Run", toListOptions(req.Msg.Options))
	if err != nil {
		return nil, mapStoreErr(err)
	}
	items := make([]*v1alpha1.Run, 0, len(res.Items))
	for _, r := range res.Items {
		run, err := resourceToRun(r)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		items = append(items, run)
	}
	return connect.NewResponse(&v1alpha1.ListRunsResponse{Items: items, ListMeta: toListMeta(res)}), nil
}

func (h *RunHandler) Watch(ctx context.Context, req *connect.Request[v1alpha1.WatchRunsRequest], stream *connect.ServerStream[v1alpha1.RunWatchEvent]) error {
	ch, err := h.store.Watch(ctx, "Run", toListOptions(req.Msg.Options), req.Msg.SinceResourceVersion)
	if err != nil {
		return mapStoreErr(err)
	}
	return watchLoop(ctx, ch, func(ev store.WatchEvent) (*v1alpha1.RunWatchEvent, error) {
		out := &v1alpha1.RunWatchEvent{Type: toWatchEventType(ev.Type), ResourceVersion: ev.ResourceVersion}
		if ev.Type != store.WatchBookmark {
			run, err := resourceToRun(ev.Object)
			if err != nil {
				return nil, err
			}
			out.Object = run
		}
		return out, nil
	}, stream.Send)
}
