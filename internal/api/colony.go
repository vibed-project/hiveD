package api

import (
	"context"

	"connectrpc.com/connect"

	v1alpha1 "github.com/hived-project/hived/gen/hived/v1alpha1"
	"github.com/hived-project/hived/internal/store"
)

// ColonyHandler implements hivedv1alpha1connect.ColonyServiceHandler.
// Colony is hive-scoped: it is its own tenant boundary, so it is stored
// with an empty store.Resource.Colony field (the resource IS the colony).
type ColonyHandler struct {
	store store.ResourceStore
}

func NewColonyHandler(s store.ResourceStore) *ColonyHandler {
	return &ColonyHandler{store: s}
}

func colonyToResource(c *v1alpha1.Colony) (store.Resource, error) {
	r, err := resourceFromMeta("Colony", c.GetMetadata())
	if err != nil {
		return store.Resource{}, err
	}
	spec, err := marshalSpec(c.GetSpec())
	if err != nil {
		return store.Resource{}, err
	}
	r.Spec = spec
	return r, nil
}

func resourceToColony(r store.Resource) (*v1alpha1.Colony, error) {
	spec := &v1alpha1.ColonySpec{}
	if err := unmarshalSpec(r.Spec, spec); err != nil {
		return nil, err
	}
	status := &v1alpha1.ColonyStatus{}
	if err := unmarshalSpec(r.Status, status); err != nil {
		return nil, err
	}
	return &v1alpha1.Colony{
		Metadata: toObjectMeta(r),
		Spec:     spec,
		Status:   status,
	}, nil
}

func (h *ColonyHandler) Apply(ctx context.Context, req *connect.Request[v1alpha1.ApplyColonyRequest]) (*connect.Response[v1alpha1.Colony], error) {
	obj := req.Msg.GetObject()
	if obj == nil {
		return nil, invalidArgument("api: object is required")
	}
	r, err := colonyToResource(obj)
	if err != nil {
		return nil, invalidArgument(err.Error())
	}
	saved, err := h.store.Apply(ctx, r, req.Msg.IfResourceVersion)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	out, err := resourceToColony(saved)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(out), nil
}

func (h *ColonyHandler) Get(ctx context.Context, req *connect.Request[v1alpha1.GetColonyRequest]) (*connect.Response[v1alpha1.Colony], error) {
	r, err := h.store.Get(ctx, "Colony", "", req.Msg.Name)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	out, err := resourceToColony(r)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(out), nil
}

func (h *ColonyHandler) List(ctx context.Context, req *connect.Request[v1alpha1.ListColoniesRequest]) (*connect.Response[v1alpha1.ListColoniesResponse], error) {
	opts := toListOptions(req.Msg.Options)
	opts.Colony = "" // Colony is hive-scoped; ignore any caller-supplied colony filter.
	res, err := h.store.List(ctx, "Colony", opts)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	items := make([]*v1alpha1.Colony, 0, len(res.Items))
	for _, r := range res.Items {
		c, err := resourceToColony(r)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		items = append(items, c)
	}
	return connect.NewResponse(&v1alpha1.ListColoniesResponse{Items: items, ListMeta: toListMeta(res)}), nil
}

func (h *ColonyHandler) Watch(ctx context.Context, req *connect.Request[v1alpha1.WatchColoniesRequest], stream *connect.ServerStream[v1alpha1.ColonyWatchEvent]) error {
	opts := toListOptions(req.Msg.Options)
	opts.Colony = ""
	ch, err := h.store.Watch(ctx, "Colony", opts, req.Msg.SinceResourceVersion)
	if err != nil {
		return mapStoreErr(err)
	}
	return watchLoop(ctx, ch, func(ev store.WatchEvent) (*v1alpha1.ColonyWatchEvent, error) {
		out := &v1alpha1.ColonyWatchEvent{Type: toWatchEventType(ev.Type), ResourceVersion: ev.ResourceVersion}
		if ev.Type != store.WatchBookmark {
			c, err := resourceToColony(ev.Object)
			if err != nil {
				return nil, err
			}
			out.Object = c
		}
		return out, nil
	}, stream.Send)
}
