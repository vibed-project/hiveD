package store

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"
)

// MemoryStore is an in-process ResourceStore + EventStore for handler unit
// tests that don't need a real Postgres instance. Its Apply/Get/List/Watch
// semantics mirror PostgresStore's as closely as practical; see
// store_test.go for the conformance tests both implementations share.
type MemoryStore struct {
	mu sync.Mutex

	resources map[resKey]Resource
	log       []logEntry
	seq       int64

	events   []Event
	eventSeq map[string]int64 // "colony/run" -> next seq
}

type resKey struct {
	Kind, Colony, Name string
}

type logEntry struct {
	Kind, Colony, Name string
	ResourceVersion    int64
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		resources: make(map[resKey]Resource),
		eventSeq:  make(map[string]int64),
	}
}

func (m *MemoryStore) nextSeq() int64 {
	m.seq++
	return m.seq
}

func cloneResource(r Resource) Resource {
	r.Labels = maps.Clone(r.Labels)
	r.Annotations = maps.Clone(r.Annotations)
	r.Spec = slices.Clone(r.Spec)
	r.Status = slices.Clone(r.Status)
	if r.DeletedAt != nil {
		t := *r.DeletedAt
		r.DeletedAt = &t
	}
	return r
}

func (m *MemoryStore) Apply(_ context.Context, r Resource, ifResourceVersion *int64) (Resource, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := resKey{r.Kind, r.Colony, r.Name}
	existing, exists := m.resources[key]

	if IsImmutableKind(r.Kind) && exists {
		if jsonEqual(existing.Spec, r.Spec) {
			return cloneResource(existing), nil
		}
		return Resource{}, fmt.Errorf("%w: %s/%s/%s", ErrImmutable, r.Kind, r.Colony, r.Name)
	}

	if ifResourceVersion != nil {
		if !exists || existing.ResourceVersion != *ifResourceVersion {
			return Resource{}, fmt.Errorf("%w: %s/%s/%s", ErrConflict, r.Kind, r.Colony, r.Name)
		}
	}

	now := time.Now().UTC()
	out := cloneResource(r)
	if out.APIVersion == "" {
		out.APIVersion = DefaultAPIVersion
	}

	// Mirror PostgresStore: only a real change advances resource_version (and
	// therefore only a real change wakes watchers). Re-applying an identical
	// manifest must be invisible on the watch stream.
	changed := true
	if exists {
		changed = !jsonEqual(existing.Spec, r.Spec) ||
			!maps.Equal(existing.Labels, r.Labels) ||
			!maps.Equal(existing.Annotations, r.Annotations)
	}

	if exists {
		out.UID = existing.UID
		out.CreatedAt = existing.CreatedAt
		if jsonEqual(existing.Spec, r.Spec) {
			out.Generation = existing.Generation
		} else {
			out.Generation = existing.Generation + 1
		}
		if out.Status == nil {
			out.Status = existing.Status
		}
		if changed {
			out.ResourceVersion = m.nextSeq()
			out.UpdatedAt = now
		} else {
			out.ResourceVersion = existing.ResourceVersion
			out.UpdatedAt = existing.UpdatedAt
		}
	} else {
		out.UID = newUID()
		out.CreatedAt = now
		out.Generation = 1
		out.ResourceVersion = m.nextSeq()
		out.UpdatedAt = now
	}

	m.resources[key] = out
	if changed {
		m.log = append(m.log, logEntry{r.Kind, r.Colony, r.Name, out.ResourceVersion})
	}
	return cloneResource(out), nil
}

func (m *MemoryStore) Get(_ context.Context, kind, colony, name string) (Resource, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	r, ok := m.resources[resKey{kind, colony, name}]
	if !ok || r.DeletedAt != nil {
		return Resource{}, fmt.Errorf("%w: %s/%s/%s", ErrNotFound, kind, colony, name)
	}
	return cloneResource(r), nil
}

func (m *MemoryStore) List(_ context.Context, kind string, opts ListOptions) (ListResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var items []Resource
	for _, r := range m.resources {
		if r.Kind != kind || r.DeletedAt != nil {
			continue
		}
		if opts.Colony != "" && r.Colony != opts.Colony {
			continue
		}
		if !matchesLabels(r.Labels, opts.LabelSelector) {
			continue
		}
		items = append(items, cloneResource(r))
	}
	// Sort and paginate on (colony, name) exactly as PostgresStore does.
	// This store previously ignored PageSize/PageToken completely, so the
	// conformance suite could not exercise pagination at all and the
	// name-only cursor bug in PostgresStore went unnoticed.
	slices.SortFunc(items, func(a, b Resource) int {
		if c := strings.Compare(a.Colony, b.Colony); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})

	afterColony, afterName, err := decodePageToken(opts.PageToken)
	if err != nil {
		return ListResult{}, err
	}
	if opts.PageToken != "" {
		items = slices.DeleteFunc(items, func(r Resource) bool {
			return r.Colony < afterColony || (r.Colony == afterColony && r.Name <= afterName)
		})
	}

	pageSize := clampPageSize(opts.PageSize)
	var nextPageToken string
	if int32(len(items)) > pageSize {
		last := items[pageSize-1]
		nextPageToken = encodePageToken(last.Colony, last.Name)
		items = items[:pageSize]
	}

	return ListResult{Items: items, NextPageToken: nextPageToken, ResourceVersion: m.seq}, nil
}

func matchesLabels(labels, selector map[string]string) bool {
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}

func (m *MemoryStore) ApplyStatus(_ context.Context, r Resource) (Resource, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := resKey{r.Kind, r.Colony, r.Name}
	existing, ok := m.resources[key]
	if !ok || existing.DeletedAt != nil {
		return Resource{}, fmt.Errorf("%w: %s/%s/%s", ErrNotFound, r.Kind, r.Colony, r.Name)
	}

	existing.Status = slices.Clone(r.Status)
	existing.ResourceVersion = m.nextSeq()
	existing.UpdatedAt = time.Now().UTC()
	m.resources[key] = existing
	m.log = append(m.log, logEntry{r.Kind, r.Colony, r.Name, existing.ResourceVersion})
	return cloneResource(existing), nil
}

func (m *MemoryStore) Watch(ctx context.Context, kind string, opts ListOptions, since int64) (<-chan WatchEvent, error) {
	out := make(chan WatchEvent, 16)
	go func() {
		defer close(out)
		cursor := since
		seen := make(map[string]bool)
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()
		bookmark := time.NewTicker(bookmarkInterval)
		defer bookmark.Stop()

		emit := func() bool {
			m.mu.Lock()
			var due []logEntry
			for _, e := range m.log {
				if e.Kind == kind && e.ResourceVersion > cursor {
					if opts.Colony == "" || e.Colony == opts.Colony {
						due = append(due, e)
					}
				}
			}
			resources := make(map[resKey]Resource, len(due))
			for _, e := range due {
				resources[resKey{e.Kind, e.Colony, e.Name}] = m.resources[resKey{e.Kind, e.Colony, e.Name}]
			}
			m.mu.Unlock()

			for _, e := range due {
				r := resources[resKey{e.Kind, e.Colony, e.Name}]
				select {
				case out <- WatchEvent{Type: classifyWatchEvent(r, seen), ResourceVersion: e.ResourceVersion, Object: cloneResource(r)}:
				case <-ctx.Done():
					return false
				}
				if e.ResourceVersion > cursor {
					cursor = e.ResourceVersion
				}
			}
			return true
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !emit() {
					return
				}
			case <-bookmark.C:
				select {
				case out <- WatchEvent{Type: WatchBookmark, ResourceVersion: cursor}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

func (m *MemoryStore) Append(_ context.Context, e Event) (Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := e.Colony + "/" + e.Run
	m.eventSeq[key]++
	e.Seq = m.eventSeq[key]
	e.ResourceVersion = m.nextSeq()
	if e.TS.IsZero() {
		e.TS = time.Now().UTC()
	}
	e.Payload = slices.Clone(e.Payload)
	m.events = append(m.events, e)
	return e, nil
}

func (m *MemoryStore) ListEvents(_ context.Context, colony, run string, sinceSeq int64, limit int32) ([]Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var out []Event
	for _, e := range m.events {
		if e.Colony == colony && e.Run == run && e.Seq > sinceSeq {
			out = append(out, e)
			if int32(len(out)) >= clampPageSize(limit) {
				break
			}
		}
	}
	return out, nil
}

func (m *MemoryStore) WatchEvents(ctx context.Context, colony, run string, since int64) (<-chan Event, error) {
	out := make(chan Event, 16)
	go func() {
		defer close(out)
		cursor := since
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		emit := func() bool {
			m.mu.Lock()
			var due []Event
			for _, e := range m.events {
				if e.Colony == colony && e.Run == run && e.ResourceVersion > cursor {
					due = append(due, e)
				}
			}
			m.mu.Unlock()

			for _, e := range due {
				select {
				case out <- e:
				case <-ctx.Done():
					return false
				}
				if e.ResourceVersion > cursor {
					cursor = e.ResourceVersion
				}
			}
			return true
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !emit() {
					return
				}
			}
		}
	}()
	return out, nil
}

var (
	_ ResourceStore = (*MemoryStore)(nil)
	_ EventStore    = (*MemoryStore)(nil)
)
