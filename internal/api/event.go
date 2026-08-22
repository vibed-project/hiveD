package api

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"

	v1alpha1 "github.com/hived-project/hived/gen/hived/v1alpha1"
	"github.com/hived-project/hived/internal/store"
)

// EventHandler implements hivedv1alpha1connect.EventServiceHandler.
// Append-only: no Apply, no Get, no update, no delete.
type EventHandler struct {
	store store.EventStore
}

func NewEventHandler(s store.EventStore) *EventHandler {
	return &EventHandler{store: s}
}

func eventToStoreEvent(e *v1alpha1.Event) (store.Event, error) {
	if e.GetColony() == "" || e.GetRun() == "" || e.GetType() == "" {
		return store.Event{}, invalidArgument("api: colony, run, and type are required")
	}
	payload, err := marshalSpec(e.GetPayload())
	if err != nil {
		return store.Event{}, err
	}
	return store.Event{
		Colony:  e.GetColony(),
		Run:     e.GetRun(),
		Type:    e.GetType(),
		TS:      fromTimestamp(e.GetTs()),
		Payload: payload,
	}, nil
}

func storeEventToEvent(e store.Event) (*v1alpha1.Event, error) {
	payload := &structpb.Struct{}
	if err := unmarshalSpec(e.Payload, payload); err != nil {
		return nil, err
	}
	return &v1alpha1.Event{
		Colony:          e.Colony,
		Run:             e.Run,
		Seq:             e.Seq,
		Type:            e.Type,
		Ts:              toTimestamp(e.TS),
		Payload:         payload,
		ResourceVersion: e.ResourceVersion,
	}, nil
}

func (h *EventHandler) Append(ctx context.Context, req *connect.Request[v1alpha1.AppendEventRequest]) (*connect.Response[v1alpha1.Event], error) {
	obj := req.Msg.GetObject()
	if obj == nil {
		return nil, invalidArgument("api: object is required")
	}
	e, err := eventToStoreEvent(obj)
	if err != nil {
		return nil, err
	}
	saved, err := h.store.Append(ctx, e)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	out, err := storeEventToEvent(saved)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(out), nil
}

func (h *EventHandler) List(ctx context.Context, req *connect.Request[v1alpha1.ListEventsRequest]) (*connect.Response[v1alpha1.ListEventsResponse], error) {
	events, err := h.store.ListEvents(ctx, req.Msg.Colony, req.Msg.Run, req.Msg.SinceSeq, req.Msg.Limit)
	if err != nil {
		return nil, mapStoreErr(err)
	}
	items := make([]*v1alpha1.Event, 0, len(events))
	for _, e := range events {
		out, err := storeEventToEvent(e)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		items = append(items, out)
	}
	return connect.NewResponse(&v1alpha1.ListEventsResponse{Items: items}), nil
}

func (h *EventHandler) Watch(ctx context.Context, req *connect.Request[v1alpha1.WatchEventsRequest], stream *connect.ServerStream[v1alpha1.Event]) error {
	ch, err := h.store.WatchEvents(ctx, req.Msg.Colony, req.Msg.Run, req.Msg.SinceResourceVersion)
	if err != nil {
		return mapStoreErr(err)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case e, ok := <-ch:
			if !ok {
				return nil
			}
			out, err := storeEventToEvent(e)
			if err != nil {
				return connect.NewError(connect.CodeInternal, err)
			}
			if err := stream.Send(out); err != nil {
				return err
			}
		}
	}
}
