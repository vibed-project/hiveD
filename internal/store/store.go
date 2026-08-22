// Package store implements hiveD's Postgres-backed resource store: the
// apply/get/list/watch semantics shared by Colony, Agent, AgentVersion, and
// Run, plus the append-only Event log. See docs/adr/ADR-0001 for why this
// follows Kubernetes-like resource conventions without a Kubernetes
// dependency, and the M0 plan for why watch is poll-based for now.
package store

import (
	"context"
	"time"
)

const DefaultAPIVersion = "hived/v1alpha1"

// Resource is the storage-layer representation of any spec/status kind
// (Colony, Agent, AgentVersion, Run). Spec and Status are canonical JSON —
// the api package owns marshaling to/from protobuf; the store never
// interprets their contents beyond byte/structural comparison.
type Resource struct {
	Kind             string
	APIVersion       string
	Colony           string
	Name             string
	UID              string
	Generation       int64
	ResourceVersion  int64
	Labels           map[string]string
	Annotations      map[string]string
	Spec             []byte
	Status           []byte
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}

// ListOptions scopes and paginates a List or Watch call.
type ListOptions struct {
	Colony        string
	LabelSelector map[string]string
	PageSize      int32
	PageToken     string
}

// ListResult is a page of resources plus the watermark a caller should pass
// as the `since` cursor to Watch to avoid missing events that landed during
// or after this List.
type ListResult struct {
	Items           []Resource
	NextPageToken   string
	ResourceVersion int64
}

type WatchEventType string

const (
	WatchAdded    WatchEventType = "ADDED"
	WatchModified WatchEventType = "MODIFIED"
	WatchDeleted  WatchEventType = "DELETED"
	WatchBookmark WatchEventType = "BOOKMARK"
)

type WatchEvent struct {
	Type            WatchEventType
	ResourceVersion int64
	Object          Resource
}

// ResourceStore is the generic apply/get/list/watch interface shared by
// every spec/status resource kind. One implementation (postgres.go) backs
// the running Keeper; another (memory.go) backs handler unit tests.
type ResourceStore interface {
	// Apply creates or updates a resource. For kinds in ImmutableKinds
	// (AgentVersion), an update whose spec differs from the stored spec
	// returns ErrImmutable; an identical re-apply is a no-op success. For
	// all other kinds, generation increments only when spec changes.
	// ifResourceVersion, if non-nil, enforces optimistic concurrency
	// against an existing row and returns ErrConflict on mismatch.
	Apply(ctx context.Context, r Resource, ifResourceVersion *int64) (Resource, error)
	Get(ctx context.Context, kind, colony, name string) (Resource, error)
	List(ctx context.Context, kind string, opts ListOptions) (ListResult, error)
	// Watch streams changes to resources of the given kind with
	// resource_version > since. The returned channel is closed when ctx is
	// done or the store is closed.
	Watch(ctx context.Context, kind string, opts ListOptions, since int64) (<-chan WatchEvent, error)
	// ApplyStatus writes only the status subresource; it never changes
	// generation.
	ApplyStatus(ctx context.Context, r Resource) (Resource, error)
}

// Event is a single append-only fact about a Run.
type Event struct {
	Colony          string
	Run             string
	Seq             int64
	Type            string
	TS              time.Time
	Payload         []byte
	ResourceVersion int64
}

type EventStore interface {
	// Append assigns Seq (monotonic per colony/run) and ResourceVersion and
	// persists the event. Events are never updated or deleted.
	Append(ctx context.Context, e Event) (Event, error)
	ListEvents(ctx context.Context, colony, run string, sinceSeq int64, limit int32) ([]Event, error)
	WatchEvents(ctx context.Context, colony, run string, since int64) (<-chan Event, error)
}

// ImmutableKinds are resource kinds whose spec cannot change after
// creation. AgentVersion is the only one in M0 — see docs/PROJECT.md §6.
var ImmutableKinds = map[string]bool{
	"AgentVersion": true,
}

func IsImmutableKind(kind string) bool {
	return ImmutableKinds[kind]
}
