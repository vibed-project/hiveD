package store

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"
)

// runResourceStoreConformance exercises the ResourceStore contract that
// MemoryStore and PostgresStore must both satisfy identically.
func runResourceStoreConformance(t *testing.T, newStore func(t *testing.T) ResourceStore) {
	t.Helper()

	t.Run("apply create then get", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()

		created, err := s.Apply(ctx, Resource{Kind: "Colony", Colony: "", Name: "acme", Spec: []byte(`{"displayName":"Acme"}`)}, nil)
		if err != nil {
			t.Fatalf("Apply: %v", err)
		}
		if created.Generation != 1 {
			t.Fatalf("Generation = %d, want 1", created.Generation)
		}
		if created.UID == "" {
			t.Fatal("UID not assigned")
		}

		got, err := s.Get(ctx, "Colony", "", "acme")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if got.UID != created.UID {
			t.Fatalf("Get UID = %s, want %s", got.UID, created.UID)
		}
	})

	t.Run("apply spec change bumps generation, status-only does not", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()

		r1, err := s.Apply(ctx, Resource{Kind: "Colony", Name: "acme", Spec: []byte(`{"displayName":"Acme"}`)}, nil)
		if err != nil {
			t.Fatalf("Apply: %v", err)
		}

		r2, err := s.Apply(ctx, Resource{Kind: "Colony", Name: "acme", Spec: []byte(`{"displayName":"Acme Inc"}`)}, nil)
		if err != nil {
			t.Fatalf("Apply (spec change): %v", err)
		}
		if r2.Generation != r1.Generation+1 {
			t.Fatalf("Generation after spec change = %d, want %d", r2.Generation, r1.Generation+1)
		}
		if r2.ResourceVersion <= r1.ResourceVersion {
			t.Fatalf("ResourceVersion did not advance: %d -> %d", r1.ResourceVersion, r2.ResourceVersion)
		}

		r3, err := s.ApplyStatus(ctx, Resource{Kind: "Colony", Name: "acme", Status: []byte(`{"ready":true}`)})
		if err != nil {
			t.Fatalf("ApplyStatus: %v", err)
		}
		if r3.Generation != r2.Generation {
			t.Fatalf("Generation after status-only write = %d, want unchanged %d", r3.Generation, r2.Generation)
		}
		if r3.ResourceVersion <= r2.ResourceVersion {
			t.Fatalf("ResourceVersion did not advance on status write")
		}
	})

	t.Run("apply identical spec is a no-op resource_version-wise for mutable kinds too", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()

		spec := []byte(`{"displayName":"Acme"}`)
		r1, err := s.Apply(ctx, Resource{Kind: "Colony", Name: "acme", Spec: spec}, nil)
		if err != nil {
			t.Fatalf("Apply: %v", err)
		}
		r2, err := s.Apply(ctx, Resource{Kind: "Colony", Name: "acme", Spec: spec}, nil)
		if err != nil {
			t.Fatalf("Apply (identical): %v", err)
		}
		if r2.Generation != r1.Generation {
			t.Fatalf("Generation changed on identical spec re-apply: %d -> %d", r1.Generation, r2.Generation)
		}
		// The test name promised this all along but only Generation was
		// asserted. An unconditional resource_version bump makes every
		// re-applying controller emit a MODIFIED event to every watcher.
		if r2.ResourceVersion != r1.ResourceVersion {
			t.Fatalf("ResourceVersion changed on identical spec re-apply: %d -> %d",
				r1.ResourceVersion, r2.ResourceVersion)
		}
	})

	t.Run("apply conflict on stale resource_version", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()

		r1, err := s.Apply(ctx, Resource{Kind: "Colony", Name: "acme", Spec: []byte(`{}`)}, nil)
		if err != nil {
			t.Fatalf("Apply: %v", err)
		}
		stale := r1.ResourceVersion
		if _, err := s.Apply(ctx, Resource{Kind: "Colony", Name: "acme", Spec: []byte(`{"a":1}`)}, nil); err != nil {
			t.Fatalf("Apply (advance): %v", err)
		}

		_, err = s.Apply(ctx, Resource{Kind: "Colony", Name: "acme", Spec: []byte(`{"a":2}`)}, &stale)
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("Apply with stale if_resource_version: err = %v, want ErrConflict", err)
		}
	})

	t.Run("immutable kind rejects differing update, allows identical re-apply", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()

		spec := []byte(`{"instructions":"be helpful"}`)
		if _, err := s.Apply(ctx, Resource{Kind: "AgentVersion", Name: "v1", Spec: spec}, nil); err != nil {
			t.Fatalf("Apply (create): %v", err)
		}

		if _, err := s.Apply(ctx, Resource{Kind: "AgentVersion", Name: "v1", Spec: spec}, nil); err != nil {
			t.Fatalf("Apply (identical re-apply) should succeed: %v", err)
		}

		_, err := s.Apply(ctx, Resource{Kind: "AgentVersion", Name: "v1", Spec: []byte(`{"instructions":"be different"}`)}, nil)
		if !errors.Is(err, ErrImmutable) {
			t.Fatalf("Apply (differing spec) err = %v, want ErrImmutable", err)
		}
	})

	t.Run("apply preserves caller-supplied initial status on create", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()

		created, err := s.Apply(ctx, Resource{
			Kind: "Run", Colony: "acme", Name: "run-1",
			Spec:   []byte(`{"agentRef":"bot"}`),
			Status: []byte(`{"phase":"RUN_PHASE_PENDING"}`),
		}, nil)
		if err != nil {
			t.Fatalf("Apply: %v", err)
		}
		if !jsonEqual(created.Status, []byte(`{"phase":"RUN_PHASE_PENDING"}`)) {
			t.Fatalf("Apply response status = %s, want RUN_PHASE_PENDING", created.Status)
		}

		got, err := s.Get(ctx, "Run", "acme", "run-1")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if !jsonEqual(got.Status, []byte(`{"phase":"RUN_PHASE_PENDING"}`)) {
			t.Fatalf("Get status = %s, want RUN_PHASE_PENDING (initial status was dropped on create)", got.Status)
		}
	})

	t.Run("get not found", func(t *testing.T) {
		s := newStore(t)
		_, err := s.Get(context.Background(), "Colony", "", "missing")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("Get err = %v, want ErrNotFound", err)
		}
	})

	t.Run("list scopes by colony", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()

		if _, err := s.Apply(ctx, Resource{Kind: "Agent", Colony: "acme", Name: "bot", Spec: []byte(`{}`)}, nil); err != nil {
			t.Fatalf("Apply: %v", err)
		}
		if _, err := s.Apply(ctx, Resource{Kind: "Agent", Colony: "globex", Name: "bot", Spec: []byte(`{}`)}, nil); err != nil {
			t.Fatalf("Apply: %v", err)
		}

		res, err := s.List(ctx, "Agent", ListOptions{Colony: "acme"})
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(res.Items) != 1 || res.Items[0].Colony != "acme" {
			t.Fatalf("List(colony=acme) = %+v, want exactly one acme item", res.Items)
		}
	})

	t.Run("list paginates across colonies without dropping same-named resources", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()

		// name is unique only per (kind, colony). A cursor keyed on name
		// alone skipped every same-named resource in later colonies.
		colonies := []string{"c1", "c2", "c3"}
		for _, c := range colonies {
			if _, err := s.Apply(ctx, Resource{Kind: "Agent", Colony: c, Name: "bot", Spec: []byte(`{}`)}, nil); err != nil {
				t.Fatalf("Apply %s/bot: %v", c, err)
			}
		}

		seen := map[string]bool{}
		token := ""
		for page := 0; page < 10; page++ {
			res, err := s.List(ctx, "Agent", ListOptions{PageSize: 1, PageToken: token})
			if err != nil {
				t.Fatalf("List page %d: %v", page, err)
			}
			for _, item := range res.Items {
				key := item.Colony + "/" + item.Name
				if seen[key] {
					t.Fatalf("page %d returned %s twice", page, key)
				}
				seen[key] = true
			}
			token = res.NextPageToken
			if token == "" {
				break
			}
		}
		if len(seen) != len(colonies) {
			t.Fatalf("paginated over %d resources, want %d (got %v)", len(seen), len(colonies), seen)
		}
	})

	t.Run("list watermark lets a watcher see the first resource ever created", func(t *testing.T) {
		s := newStore(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// meta.proto promises List-then-Watch(since=resource_version) misses
		// nothing. On an empty store the watermark used to equal the version
		// the first resource would go on to receive, so it was never
		// delivered.
		empty, err := s.List(ctx, "Colony", ListOptions{})
		if err != nil {
			t.Fatalf("List (empty): %v", err)
		}

		ch, err := s.Watch(ctx, "Colony", ListOptions{}, empty.ResourceVersion)
		if err != nil {
			t.Fatalf("Watch: %v", err)
		}
		if _, err := s.Apply(ctx, Resource{Kind: "Colony", Name: "first-ever", Spec: []byte(`{}`)}, nil); err != nil {
			t.Fatalf("Apply: %v", err)
		}

		ev := nextNonBookmark(t, ch)
		if ev.Object.Name != "first-ever" {
			t.Fatalf("watch delivered %q, want first-ever", ev.Object.Name)
		}
	})

	t.Run("page size is clamped rather than overflowing", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()

		if _, err := s.Apply(ctx, Resource{Kind: "Colony", Name: "acme", Spec: []byte(`{}`)}, nil); err != nil {
			t.Fatalf("Apply: %v", err)
		}
		// PageSize is int32 off the wire; pageSize+1 used to wrap negative at
		// MaxInt32 and Postgres rejected the resulting LIMIT.
		for _, size := range []int32{math.MaxInt32, math.MaxInt32 - 1, MaxPageSize + 1, -5} {
			res, err := s.List(ctx, "Colony", ListOptions{PageSize: size})
			if err != nil {
				t.Fatalf("List(PageSize=%d): %v", size, err)
			}
			if len(res.Items) != 1 {
				t.Fatalf("List(PageSize=%d) returned %d items, want 1", size, len(res.Items))
			}
		}
	})

	t.Run("malformed page token is rejected as such", func(t *testing.T) {
		s := newStore(t)
		_, err := s.List(context.Background(), "Colony", ListOptions{PageToken: "!!!not-base64!!!"})
		if !errors.Is(err, ErrInvalidPageToken) {
			t.Fatalf("List with bad page token: err = %v, want ErrInvalidPageToken", err)
		}
	})

	t.Run("watch delivers added then modified", func(t *testing.T) {
		s := newStore(t)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		events, err := s.Watch(ctx, "Colony", ListOptions{}, 0)
		if err != nil {
			t.Fatalf("Watch: %v", err)
		}

		if _, err := s.Apply(ctx, Resource{Kind: "Colony", Name: "acme", Spec: []byte(`{"v":1}`)}, nil); err != nil {
			t.Fatalf("Apply: %v", err)
		}

		first := nextNonBookmark(t, events)
		if first.Type != WatchAdded {
			t.Fatalf("first event type = %s, want ADDED", first.Type)
		}

		if _, err := s.Apply(ctx, Resource{Kind: "Colony", Name: "acme", Spec: []byte(`{"v":2}`)}, nil); err != nil {
			t.Fatalf("Apply (update): %v", err)
		}

		second := nextNonBookmark(t, events)
		if second.Type != WatchModified {
			t.Fatalf("second event type = %s, want MODIFIED", second.Type)
		}
	})
}

func nextNonBookmark(t *testing.T, ch <-chan WatchEvent) WatchEvent {
	t.Helper()
	for {
		select {
		case e, ok := <-ch:
			if !ok {
				t.Fatal("watch channel closed unexpectedly")
			}
			if e.Type == WatchBookmark {
				continue
			}
			return e
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for watch event")
		}
	}
}

func runEventStoreConformance(t *testing.T, newStore func(t *testing.T) EventStore) {
	t.Helper()

	t.Run("append assigns monotonic per-run seq", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()

		e1, err := s.Append(ctx, Event{Colony: "acme", Run: "run-1", Type: "RunStarted", Payload: []byte(`{}`)})
		if err != nil {
			t.Fatalf("Append: %v", err)
		}
		e2, err := s.Append(ctx, Event{Colony: "acme", Run: "run-1", Type: "ModelCalled", Payload: []byte(`{}`)})
		if err != nil {
			t.Fatalf("Append: %v", err)
		}
		if e1.Seq != 1 || e2.Seq != 2 {
			t.Fatalf("seqs = %d, %d, want 1, 2", e1.Seq, e2.Seq)
		}

		// A different run starts its own sequence.
		eOther, err := s.Append(ctx, Event{Colony: "acme", Run: "run-2", Type: "RunStarted", Payload: []byte(`{}`)})
		if err != nil {
			t.Fatalf("Append: %v", err)
		}
		if eOther.Seq != 1 {
			t.Fatalf("run-2 first seq = %d, want 1", eOther.Seq)
		}
	})

	t.Run("concurrent appends to one run all persist with unique seq", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()

		// seq was assigned as MAX(seq)+1 with no serialization, so concurrent
		// appenders all computed the same value and UNIQUE (colony, run, seq)
		// rejected all but one. Nothing retried, so the events were simply
		// lost. Measured before the fix: 200 concurrent appends, 78 persisted.
		const appenders = 16
		const perAppender = 10

		var wg sync.WaitGroup
		errCh := make(chan error, appenders*perAppender)
		for i := 0; i < appenders; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < perAppender; j++ {
					_, err := s.Append(ctx, Event{Colony: "acme", Run: "r1", Type: "test.event"})
					if err != nil {
						errCh <- err
						return
					}
				}
			}()
		}
		wg.Wait()
		close(errCh)
		for err := range errCh {
			t.Fatalf("Append: %v", err)
		}

		got, err := s.ListEvents(ctx, "acme", "r1", 0, MaxPageSize)
		if err != nil {
			t.Fatalf("ListEvents: %v", err)
		}
		want := appenders * perAppender
		if len(got) != want {
			t.Fatalf("persisted %d events, want %d", len(got), want)
		}
		seqs := map[int64]bool{}
		for _, e := range got {
			if seqs[e.Seq] {
				t.Fatalf("duplicate seq %d", e.Seq)
			}
			seqs[e.Seq] = true
		}
		for i := int64(1); i <= int64(want); i++ {
			if !seqs[i] {
				t.Fatalf("seq %d missing; sequence has a gap", i)
			}
		}
	})

	t.Run("append preserves a caller-supplied timestamp", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()

		// ts was omitted from the INSERT column list, so a Worker replaying
		// buffered events after a partition had every timestamp rewritten to
		// ingestion time.
		want := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
		got, err := s.Append(ctx, Event{Colony: "acme", Run: "r1", Type: "test.event", TS: want})
		if err != nil {
			t.Fatalf("Append: %v", err)
		}
		if !got.TS.UTC().Equal(want) {
			t.Fatalf("TS = %s, want %s", got.TS.UTC(), want)
		}
	})

	t.Run("append defaults the timestamp when the caller leaves it unset", func(t *testing.T) {
		s := newStore(t)
		before := time.Now().Add(-time.Minute)

		got, err := s.Append(context.Background(), Event{Colony: "acme", Run: "r1", Type: "test.event"})
		if err != nil {
			t.Fatalf("Append: %v", err)
		}
		if got.TS.Before(before) {
			t.Fatalf("TS = %s, want a server-assigned time after %s", got.TS, before)
		}
	})

	t.Run("list events since seq", func(t *testing.T) {
		s := newStore(t)
		ctx := context.Background()

		for i := 0; i < 3; i++ {
			if _, err := s.Append(ctx, Event{Colony: "acme", Run: "run-1", Type: "Step", Payload: []byte(`{}`)}); err != nil {
				t.Fatalf("Append: %v", err)
			}
		}

		events, err := s.ListEvents(ctx, "acme", "run-1", 1, 0)
		if err != nil {
			t.Fatalf("ListEvents: %v", err)
		}
		if len(events) != 2 {
			t.Fatalf("ListEvents(since=1) returned %d events, want 2", len(events))
		}
		if events[0].Seq != 2 || events[1].Seq != 3 {
			t.Fatalf("unexpected seqs: %d, %d", events[0].Seq, events[1].Seq)
		}
	})
}
