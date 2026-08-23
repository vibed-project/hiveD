package api

import (
	"context"

	"connectrpc.com/connect"

	v1alpha1 "github.com/vibed-project/hiveD/gen/hived/v1alpha1"
	"github.com/vibed-project/hiveD/internal/store"
)

// ToolHandler implements hivedv1alpha1connect.ToolServiceHandler. Tool is a
// plain mutable, colony-scoped registration; nothing here contacts
// spec.endpoint — only the Tool Broker ever does that.
type ToolHandler struct {
	store store.ResourceStore
}

func NewToolHandler(s store.ResourceStore) *ToolHandler {
	return &ToolHandler{store: s}
}

func toolToResource(t *v1alpha1.Tool) (store.Resource, error) {
	r, err := resourceFromMeta("Tool", t.GetMetadata())
	if err != nil {
		return store.Resource{}, err
	}
	if r.Colony == "" {
		return store.Resource{}, invalidArgument("api: metadata.colony is required")
	}
	spec := t.GetSpec()
	switch spec.GetType() {
	case v1alpha1.ToolType_TOOL_TYPE_MCP:
		if spec.GetEndpoint() == "" {
			return store.Resource{}, invalidArgument("api: spec.endpoint is required for TOOL_TYPE_MCP")
		}
	case v1alpha1.ToolType_TOOL_TYPE_BUILTIN:
		if spec.GetBuiltin() == "" {
			return store.Resource{}, invalidArgument("api: spec.builtin is required for TOOL_TYPE_BUILTIN")
		}
	case v1alpha1.ToolType_TOOL_TYPE_AGENT:
		if spec.GetAgentRef() == "" {
			return store.Resource{}, invalidArgument("api: spec.agent_ref is required for TOOL_TYPE_AGENT")
		}
	default:
		return store.Resource{}, invalidArgument("api: spec.type is required")
	}
	specJSON, err := marshalSpec(spec)
	if err != nil {
		return store.Resource{}, err
	}
	r.Spec = specJSON
	return r, nil
}

func resourceToTool(r store.Resource) (*v1alpha1.Tool, error) {
	spec := &v1alpha1.ToolSpec{}
	if err := unmarshalSpec(r.Spec, spec); err != nil {
		return nil, err
	}
	status := &v1alpha1.ToolStatus{}
	if err := unmarshalSpec(r.Status, status); err != nil {
		return nil, err
	}
	return &v1alpha1.Tool{Metadata: toObjectMeta(r), Spec: spec, Status: status}, nil
}

func (h *ToolHandler) Apply(ctx context.Context, req *connect.Request[v1alpha1.ApplyToolRequest]) (*connect.Response[v1alpha1.Tool], error) {
	obj := req.Msg.GetObject()
	if obj == nil {
		return nil, invalidArgument("api: object is required")
	}
	r, err := toolToResource(obj)
	if err != nil {
		return nil, err
	}
	saved, err := h.store.Apply(ctx, r, req.Msg.IfResourceVersion)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	out, err := resourceToTool(saved)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(out), nil
}

func (h *ToolHandler) Get(ctx context.Context, req *connect.Request[v1alpha1.GetToolRequest]) (*connect.Response[v1alpha1.Tool], error) {
	r, err := h.store.Get(ctx, "Tool", req.Msg.Colony, req.Msg.Name)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	out, err := resourceToTool(r)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(out), nil
}

func (h *ToolHandler) List(ctx context.Context, req *connect.Request[v1alpha1.ListToolsRequest]) (*connect.Response[v1alpha1.ListToolsResponse], error) {
	res, err := h.store.List(ctx, "Tool", toListOptions(req.Msg.Options))
	if err != nil {
		return nil, mapStoreErr(err)
	}
	items := make([]*v1alpha1.Tool, 0, len(res.Items))
	for _, r := range res.Items {
		t, err := resourceToTool(r)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		items = append(items, t)
	}
	return connect.NewResponse(&v1alpha1.ListToolsResponse{Items: items, ListMeta: toListMeta(res)}), nil
}

func (h *ToolHandler) Watch(ctx context.Context, req *connect.Request[v1alpha1.WatchToolsRequest], stream *connect.ServerStream[v1alpha1.ToolWatchEvent]) error {
	ch, err := h.store.Watch(ctx, "Tool", toListOptions(req.Msg.Options), req.Msg.SinceResourceVersion)
	if err != nil {
		return mapStoreErr(err)
	}
	return watchLoop(ctx, ch, func(ev store.WatchEvent) (*v1alpha1.ToolWatchEvent, error) {
		out := &v1alpha1.ToolWatchEvent{Type: toWatchEventType(ev.Type), ResourceVersion: ev.ResourceVersion}
		if ev.Type != store.WatchBookmark {
			t, err := resourceToTool(ev.Object)
			if err != nil {
				return nil, err
			}
			out.Object = t
		}
		return out, nil
	}, stream.Send)
}
