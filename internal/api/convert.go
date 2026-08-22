// Package api implements the Keeper's connect-go service handlers: thin
// per-kind adapters between proto messages and internal/store.Resource.
// See docs/PROJECT.md §5.1 and docs/adr/ADR-0001 for why the Keeper never
// does anything beyond that translation plus store calls in M0 — there is
// no Scheduler, no policy evaluation, nothing that looks like reconciling
// a Run.
package api

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1alpha1 "github.com/hived-project/hived/gen/hived/v1alpha1"
	"github.com/hived-project/hived/internal/store"
)

// mapStoreErr translates a store error into the connect error code the
// CLI and any future client should key off of.
func mapStoreErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, store.ErrNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, store.ErrConflict):
		return connect.NewError(connect.CodeAborted, err)
	case errors.Is(err, store.ErrImmutable):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
}

func invalidArgument(msg string) error {
	return connect.NewError(connect.CodeInvalidArgument, errors.New(msg))
}

func toTimestamp(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

func fromTimestamp(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime()
}

// toObjectMeta builds the wire ObjectMeta from a stored Resource.
func toObjectMeta(r store.Resource) *v1alpha1.ObjectMeta {
	m := &v1alpha1.ObjectMeta{
		Name:            r.Name,
		Colony:          r.Colony,
		Uid:             r.UID,
		Generation:      r.Generation,
		ResourceVersion: r.ResourceVersion,
		Labels:          r.Labels,
		Annotations:     r.Annotations,
		CreatedAt:       toTimestamp(r.CreatedAt),
		UpdatedAt:       toTimestamp(r.UpdatedAt),
	}
	if r.DeletedAt != nil {
		m.DeletedAt = toTimestamp(*r.DeletedAt)
	}
	return m
}

// resourceFromMeta extracts the store-level identity fields (kind, colony,
// name, labels, annotations) a client supplies on Apply. Server-assigned
// fields (uid, generation, resource_version, timestamps) are ignored here —
// the store, not the caller, owns them.
func resourceFromMeta(kind string, m *v1alpha1.ObjectMeta) (store.Resource, error) {
	if m == nil || m.Name == "" {
		return store.Resource{}, invalidArgument("api: metadata.name is required")
	}
	return store.Resource{
		Kind:        kind,
		APIVersion:  store.DefaultAPIVersion,
		Colony:      m.Colony,
		Name:        m.Name,
		Labels:      m.Labels,
		Annotations: m.Annotations,
	}, nil
}

func marshalSpec(m proto.Message) ([]byte, error) {
	return protojson.Marshal(m)
}

func unmarshalSpec(data []byte, m proto.Message) error {
	if len(data) == 0 {
		return nil
	}
	return protojson.Unmarshal(data, m)
}

func toWatchEventType(t store.WatchEventType) v1alpha1.WatchEventType {
	switch t {
	case store.WatchAdded:
		return v1alpha1.WatchEventType_WATCH_EVENT_TYPE_ADDED
	case store.WatchModified:
		return v1alpha1.WatchEventType_WATCH_EVENT_TYPE_MODIFIED
	case store.WatchDeleted:
		return v1alpha1.WatchEventType_WATCH_EVENT_TYPE_DELETED
	case store.WatchBookmark:
		return v1alpha1.WatchEventType_WATCH_EVENT_TYPE_BOOKMARK
	default:
		return v1alpha1.WatchEventType_WATCH_EVENT_TYPE_UNSPECIFIED
	}
}

func toListOptions(o *v1alpha1.ListOptions) store.ListOptions {
	if o == nil {
		return store.ListOptions{}
	}
	return store.ListOptions{
		Colony:        o.Colony,
		LabelSelector: o.LabelSelector,
		PageSize:      o.PageSize,
		PageToken:     o.PageToken,
	}
}

func toListMeta(r store.ListResult) *v1alpha1.ListMeta {
	return &v1alpha1.ListMeta{
		NextPageToken:   r.NextPageToken,
		ResourceVersion: r.ResourceVersion,
	}
}

// watchLoop drains a store.WatchEvent channel, converts each event via
// toEvent, and sends it on the connect server stream. It returns when the
// channel closes or ctx is done. Shared by every *Service.Watch handler so
// the ctx/channel-draining logic exists exactly once.
func watchLoop[E any](ctx context.Context, ch <-chan store.WatchEvent, toEvent func(store.WatchEvent) (*E, error), send func(*E) error) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-ch:
			if !ok {
				return nil
			}
			out, err := toEvent(ev)
			if err != nil {
				return connect.NewError(connect.CodeInternal, err)
			}
			if out == nil {
				continue
			}
			if err := send(out); err != nil {
				return err
			}
		}
	}
}
