package api

import (
	"context"

	"connectrpc.com/connect"

	v1alpha1 "github.com/vibed-project/hiveD/gen/hived/v1alpha1"
	"github.com/vibed-project/hiveD/internal/store"
)

// AgentHandler implements hivedv1alpha1connect.AgentServiceHandler. It
// never writes an AgentVersion spec — only AgentVersionHandler does that —
// and Agent.status.current is set via ApplyStatus, not by anything here;
// there is no controller in M0 to set it automatically.
type AgentHandler struct {
	store store.ResourceStore
}

func NewAgentHandler(s store.ResourceStore) *AgentHandler {
	return &AgentHandler{store: s}
}

func agentToResource(a *v1alpha1.Agent) (store.Resource, error) {
	r, err := resourceFromMeta("Agent", a.GetMetadata())
	if err != nil {
		return store.Resource{}, err
	}
	if r.Colony == "" {
		return store.Resource{}, invalidArgument("api: metadata.colony is required")
	}
	spec, err := marshalSpec(a.GetSpec())
	if err != nil {
		return store.Resource{}, err
	}
	r.Spec = spec
	return r, nil
}

func resourceToAgent(r store.Resource) (*v1alpha1.Agent, error) {
	spec := &v1alpha1.AgentSpec{}
	if err := unmarshalSpec(r.Spec, spec); err != nil {
		return nil, err
	}
	status := &v1alpha1.AgentStatus{}
	if err := unmarshalSpec(r.Status, status); err != nil {
		return nil, err
	}
	return &v1alpha1.Agent{Metadata: toObjectMeta(r), Spec: spec, Status: status}, nil
}

func (h *AgentHandler) Apply(ctx context.Context, req *connect.Request[v1alpha1.ApplyAgentRequest]) (*connect.Response[v1alpha1.Agent], error) {
	obj := req.Msg.GetObject()
	if obj == nil {
		return nil, invalidArgument("api: object is required")
	}
	r, err := agentToResource(obj)
	if err != nil {
		return nil, err
	}
	saved, err := h.store.Apply(ctx, r, req.Msg.IfResourceVersion)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	out, err := resourceToAgent(saved)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(out), nil
}

func (h *AgentHandler) Get(ctx context.Context, req *connect.Request[v1alpha1.GetAgentRequest]) (*connect.Response[v1alpha1.Agent], error) {
	r, err := h.store.Get(ctx, "Agent", req.Msg.Colony, req.Msg.Name)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	out, err := resourceToAgent(r)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(out), nil
}

func (h *AgentHandler) List(ctx context.Context, req *connect.Request[v1alpha1.ListAgentsRequest]) (*connect.Response[v1alpha1.ListAgentsResponse], error) {
	res, err := h.store.List(ctx, "Agent", toListOptions(req.Msg.Options))
	if err != nil {
		return nil, mapStoreErr(err)
	}
	items := make([]*v1alpha1.Agent, 0, len(res.Items))
	for _, r := range res.Items {
		a, err := resourceToAgent(r)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		items = append(items, a)
	}
	return connect.NewResponse(&v1alpha1.ListAgentsResponse{Items: items, ListMeta: toListMeta(res)}), nil
}

func (h *AgentHandler) Watch(ctx context.Context, req *connect.Request[v1alpha1.WatchAgentsRequest], stream *connect.ServerStream[v1alpha1.AgentWatchEvent]) error {
	ch, err := h.store.Watch(ctx, "Agent", toListOptions(req.Msg.Options), req.Msg.SinceResourceVersion)
	if err != nil {
		return mapStoreErr(err)
	}
	return watchLoop(ctx, ch, func(ev store.WatchEvent) (*v1alpha1.AgentWatchEvent, error) {
		out := &v1alpha1.AgentWatchEvent{Type: toWatchEventType(ev.Type), ResourceVersion: ev.ResourceVersion}
		if ev.Type != store.WatchBookmark {
			a, err := resourceToAgent(ev.Object)
			if err != nil {
				return nil, err
			}
			out.Object = a
		}
		return out, nil
	}, stream.Send)
}

// AgentVersionHandler implements hivedv1alpha1connect.AgentVersionServiceHandler.
// Apply is create-or-identical-noop only — internal/store enforces that an
// update whose spec differs from an existing (colony, name) is rejected as
// immutable (store.ErrImmutable -> CodeFailedPrecondition).
type AgentVersionHandler struct {
	store store.ResourceStore
}

func NewAgentVersionHandler(s store.ResourceStore) *AgentVersionHandler {
	return &AgentVersionHandler{store: s}
}

func agentVersionToResource(av *v1alpha1.AgentVersion) (store.Resource, error) {
	r, err := resourceFromMeta("AgentVersion", av.GetMetadata())
	if err != nil {
		return store.Resource{}, err
	}
	if r.Colony == "" {
		return store.Resource{}, invalidArgument("api: metadata.colony is required")
	}
	if av.GetSpec().GetAgent() == "" {
		return store.Resource{}, invalidArgument("api: spec.agent is required")
	}
	spec, err := marshalSpec(av.GetSpec())
	if err != nil {
		return store.Resource{}, err
	}
	r.Spec = spec
	return r, nil
}

func resourceToAgentVersion(r store.Resource) (*v1alpha1.AgentVersion, error) {
	spec := &v1alpha1.AgentVersionSpec{}
	if err := unmarshalSpec(r.Spec, spec); err != nil {
		return nil, err
	}
	status := &v1alpha1.AgentVersionStatus{}
	if err := unmarshalSpec(r.Status, status); err != nil {
		return nil, err
	}
	return &v1alpha1.AgentVersion{Metadata: toObjectMeta(r), Spec: spec, Status: status}, nil
}

func (h *AgentVersionHandler) Apply(ctx context.Context, req *connect.Request[v1alpha1.ApplyAgentVersionRequest]) (*connect.Response[v1alpha1.AgentVersion], error) {
	obj := req.Msg.GetObject()
	if obj == nil {
		return nil, invalidArgument("api: object is required")
	}
	r, err := agentVersionToResource(obj)
	if err != nil {
		return nil, err
	}
	saved, err := h.store.Apply(ctx, r, req.Msg.IfResourceVersion)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	out, err := resourceToAgentVersion(saved)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(out), nil
}

func (h *AgentVersionHandler) Get(ctx context.Context, req *connect.Request[v1alpha1.GetAgentVersionRequest]) (*connect.Response[v1alpha1.AgentVersion], error) {
	r, err := h.store.Get(ctx, "AgentVersion", req.Msg.Colony, req.Msg.Name)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	out, err := resourceToAgentVersion(r)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(out), nil
}

func (h *AgentVersionHandler) List(ctx context.Context, req *connect.Request[v1alpha1.ListAgentVersionsRequest]) (*connect.Response[v1alpha1.ListAgentVersionsResponse], error) {
	res, err := h.store.List(ctx, "AgentVersion", toListOptions(req.Msg.Options))
	if err != nil {
		return nil, mapStoreErr(err)
	}
	items := make([]*v1alpha1.AgentVersion, 0, len(res.Items))
	for _, r := range res.Items {
		av, err := resourceToAgentVersion(r)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		items = append(items, av)
	}
	return connect.NewResponse(&v1alpha1.ListAgentVersionsResponse{Items: items, ListMeta: toListMeta(res)}), nil
}

func (h *AgentVersionHandler) Watch(ctx context.Context, req *connect.Request[v1alpha1.WatchAgentVersionsRequest], stream *connect.ServerStream[v1alpha1.AgentVersionWatchEvent]) error {
	ch, err := h.store.Watch(ctx, "AgentVersion", toListOptions(req.Msg.Options), req.Msg.SinceResourceVersion)
	if err != nil {
		return mapStoreErr(err)
	}
	return watchLoop(ctx, ch, func(ev store.WatchEvent) (*v1alpha1.AgentVersionWatchEvent, error) {
		out := &v1alpha1.AgentVersionWatchEvent{Type: toWatchEventType(ev.Type), ResourceVersion: ev.ResourceVersion}
		if ev.Type != store.WatchBookmark {
			av, err := resourceToAgentVersion(ev.Object)
			if err != nil {
				return nil, err
			}
			out.Object = av
		}
		return out, nil
	}, stream.Send)
}
