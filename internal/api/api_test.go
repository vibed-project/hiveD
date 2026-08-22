package api

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"

	v1alpha1 "github.com/hived-project/hived/gen/hived/v1alpha1"
	"github.com/hived-project/hived/internal/store"
)

func TestColonyHandler_ApplyGetList(t *testing.T) {
	s := store.NewMemoryStore()
	h := NewColonyHandler(s)
	ctx := context.Background()

	applyReq := connect.NewRequest(&v1alpha1.ApplyColonyRequest{
		Object: &v1alpha1.Colony{
			Metadata: &v1alpha1.ObjectMeta{Name: "acme"},
			Spec:     &v1alpha1.ColonySpec{DisplayName: "Acme"},
		},
	})
	applied, err := h.Apply(ctx, applyReq)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if applied.Msg.Metadata.Generation != 1 {
		t.Fatalf("Generation = %d, want 1", applied.Msg.Metadata.Generation)
	}

	got, err := h.Get(ctx, connect.NewRequest(&v1alpha1.GetColonyRequest{Name: "acme"}))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Msg.Spec.DisplayName != "Acme" {
		t.Fatalf("DisplayName = %q, want Acme", got.Msg.Spec.DisplayName)
	}

	list, err := h.List(ctx, connect.NewRequest(&v1alpha1.ListColoniesRequest{}))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list.Msg.Items) != 1 {
		t.Fatalf("List returned %d items, want 1", len(list.Msg.Items))
	}
}

func TestColonyHandler_ApplyRequiresName(t *testing.T) {
	h := NewColonyHandler(store.NewMemoryStore())
	_, err := h.Apply(context.Background(), connect.NewRequest(&v1alpha1.ApplyColonyRequest{
		Object: &v1alpha1.Colony{Metadata: &v1alpha1.ObjectMeta{}},
	}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("err = %v, want CodeInvalidArgument", err)
	}
}

func TestAgentVersionHandler_ImmutableRejectsAsFailedPrecondition(t *testing.T) {
	s := store.NewMemoryStore()
	h := NewAgentVersionHandler(s)
	ctx := context.Background()

	mk := func(instructions string) *connect.Request[v1alpha1.ApplyAgentVersionRequest] {
		return connect.NewRequest(&v1alpha1.ApplyAgentVersionRequest{
			Object: &v1alpha1.AgentVersion{
				Metadata: &v1alpha1.ObjectMeta{Colony: "acme", Name: "bot-v1"},
				Spec:     &v1alpha1.AgentVersionSpec{Agent: "bot", Version: "v1", Instructions: instructions},
			},
		})
	}

	if _, err := h.Apply(ctx, mk("be helpful")); err != nil {
		t.Fatalf("Apply (create): %v", err)
	}
	if _, err := h.Apply(ctx, mk("be helpful")); err != nil {
		t.Fatalf("Apply (identical re-apply): %v", err)
	}
	_, err := h.Apply(ctx, mk("be different"))
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("err = %v, want CodeFailedPrecondition", err)
	}
	if !errors.Is(err, store.ErrImmutable) {
		t.Fatalf("err does not wrap store.ErrImmutable: %v", err)
	}
}

func TestRunHandler_ApplyForcesPendingPhase(t *testing.T) {
	s := store.NewMemoryStore()
	h := NewRunHandler(s)
	ctx := context.Background()

	resp, err := h.Apply(ctx, connect.NewRequest(&v1alpha1.ApplyRunRequest{
		Object: &v1alpha1.Run{
			Metadata: &v1alpha1.ObjectMeta{Colony: "acme", Name: "run-1"},
			Spec:     &v1alpha1.RunSpec{AgentRef: "bot"},
			// A caller-supplied phase must be ignored; Apply always starts PENDING.
			Status: &v1alpha1.RunStatus{Phase: v1alpha1.RunPhase_RUN_PHASE_SUCCEEDED},
		},
	}))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if resp.Msg.Status.Phase != v1alpha1.RunPhase_RUN_PHASE_PENDING {
		t.Fatalf("Phase = %v, want PENDING", resp.Msg.Status.Phase)
	}

	got, err := h.Get(ctx, connect.NewRequest(&v1alpha1.GetRunRequest{Colony: "acme", Name: "run-1"}))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Msg.Status.Phase != v1alpha1.RunPhase_RUN_PHASE_PENDING {
		t.Fatalf("Get Phase = %v, want PENDING (nothing schedules a Run in M0)", got.Msg.Status.Phase)
	}
}

func TestEventHandler_AppendAndList(t *testing.T) {
	s := store.NewMemoryStore()
	h := NewEventHandler(s)
	ctx := context.Background()

	for _, typ := range []string{"RunStarted", "ModelCalled"} {
		_, err := h.Append(ctx, connect.NewRequest(&v1alpha1.AppendEventRequest{
			Object: &v1alpha1.Event{Colony: "acme", Run: "run-1", Type: typ},
		}))
		if err != nil {
			t.Fatalf("Append(%s): %v", typ, err)
		}
	}

	list, err := h.List(ctx, connect.NewRequest(&v1alpha1.ListEventsRequest{Colony: "acme", Run: "run-1"}))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list.Msg.Items) != 2 {
		t.Fatalf("List returned %d events, want 2", len(list.Msg.Items))
	}
	if list.Msg.Items[0].Seq != 1 || list.Msg.Items[1].Seq != 2 {
		t.Fatalf("unexpected seqs: %d, %d", list.Msg.Items[0].Seq, list.Msg.Items[1].Seq)
	}
}
