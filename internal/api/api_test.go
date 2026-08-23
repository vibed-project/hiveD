package api

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"connectrpc.com/connect"

	v1alpha1 "github.com/vibed-project/hiveD/gen/hived/v1alpha1"
	"github.com/vibed-project/hiveD/internal/store"
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

func TestToolHandler_ApplyValidatesByType(t *testing.T) {
	h := NewToolHandler(store.NewMemoryStore())
	ctx := context.Background()

	tests := []struct {
		name     string
		spec     *v1alpha1.ToolSpec
		wantCode connect.Code
	}{
		{"mcp ok", &v1alpha1.ToolSpec{Type: v1alpha1.ToolType_TOOL_TYPE_MCP, Endpoint: "http://echo:8080/mcp"}, 0},
		{"mcp missing endpoint", &v1alpha1.ToolSpec{Type: v1alpha1.ToolType_TOOL_TYPE_MCP}, connect.CodeInvalidArgument},
		{"builtin ok", &v1alpha1.ToolSpec{Type: v1alpha1.ToolType_TOOL_TYPE_BUILTIN, Builtin: "spawn_run"}, 0},
		{"builtin missing name", &v1alpha1.ToolSpec{Type: v1alpha1.ToolType_TOOL_TYPE_BUILTIN}, connect.CodeInvalidArgument},
		{"agent ok", &v1alpha1.ToolSpec{Type: v1alpha1.ToolType_TOOL_TYPE_AGENT, AgentRef: "summarizer"}, 0},
		{"agent missing ref", &v1alpha1.ToolSpec{Type: v1alpha1.ToolType_TOOL_TYPE_AGENT}, connect.CodeInvalidArgument},
		{"type unspecified", &v1alpha1.ToolSpec{Endpoint: "http://x"}, connect.CodeInvalidArgument},
	}
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.Apply(ctx, connect.NewRequest(&v1alpha1.ApplyToolRequest{
				Object: &v1alpha1.Tool{
					Metadata: &v1alpha1.ObjectMeta{Colony: "acme", Name: fmt.Sprintf("tool-%d", i)},
					Spec:     tc.spec,
				},
			}))
			if tc.wantCode == 0 {
				if err != nil {
					t.Fatalf("Apply: %v", err)
				}
				return
			}
			if connect.CodeOf(err) != tc.wantCode {
				t.Fatalf("err = %v, want %v", err, tc.wantCode)
			}
		})
	}
}

func TestToolHandler_ApplyGetList(t *testing.T) {
	s := store.NewMemoryStore()
	h := NewToolHandler(s)
	ctx := context.Background()

	mk := func(colony, name, endpoint string) *connect.Request[v1alpha1.ApplyToolRequest] {
		return connect.NewRequest(&v1alpha1.ApplyToolRequest{
			Object: &v1alpha1.Tool{
				Metadata: &v1alpha1.ObjectMeta{Colony: colony, Name: name},
				Spec:     &v1alpha1.ToolSpec{Type: v1alpha1.ToolType_TOOL_TYPE_MCP, Endpoint: endpoint, RiskClass: "read"},
			},
		})
	}
	if _, err := h.Apply(ctx, mk("acme", "echo", "http://echo:8080/mcp")); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := h.Apply(ctx, mk("other", "echo", "http://other:8080/mcp")); err != nil {
		t.Fatalf("Apply (other colony): %v", err)
	}

	// Tool is mutable: a changed spec is accepted and bumps generation.
	updated, err := h.Apply(ctx, mk("acme", "echo", "http://echo:9090/mcp"))
	if err != nil {
		t.Fatalf("Apply (update): %v", err)
	}
	if updated.Msg.Metadata.Generation != 2 {
		t.Fatalf("Generation after spec change = %d, want 2", updated.Msg.Metadata.Generation)
	}

	got, err := h.Get(ctx, connect.NewRequest(&v1alpha1.GetToolRequest{Colony: "acme", Name: "echo"}))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Msg.Spec.Endpoint != "http://echo:9090/mcp" || got.Msg.Spec.RiskClass != "read" {
		t.Fatalf("Get spec = %+v", got.Msg.Spec)
	}

	list, err := h.List(ctx, connect.NewRequest(&v1alpha1.ListToolsRequest{Options: &v1alpha1.ListOptions{Colony: "acme"}}))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list.Msg.Items) != 1 {
		t.Fatalf("List(acme) returned %d items, want 1 (colony scoping)", len(list.Msg.Items))
	}

	_, err = h.Get(ctx, connect.NewRequest(&v1alpha1.GetToolRequest{Colony: "acme", Name: "missing"}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("Get(missing) err = %v, want CodeNotFound", err)
	}
}

func TestRunHandler_ApplyResetsAttemptAndSchedulerFields(t *testing.T) {
	h := NewRunHandler(store.NewMemoryStore())
	resp, err := h.Apply(context.Background(), connect.NewRequest(&v1alpha1.ApplyRunRequest{
		Object: &v1alpha1.Run{
			Metadata: &v1alpha1.ObjectMeta{Colony: "acme", Name: "run-1"},
			Spec:     &v1alpha1.RunSpec{AgentRef: "bot"},
			// Scheduler-owned fields supplied by a caller must be dropped.
			Status: &v1alpha1.RunStatus{Attempt: 7, CellRef: "cell-x", Checkpoint: "kv/x@1"},
		},
	}))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	st := resp.Msg.Status
	if st.Attempt != 0 || st.CellRef != "" || st.Checkpoint != "" || st.LastHeartbeatAt != nil {
		t.Fatalf("status not reset: %+v", st)
	}
}

func TestRunHandler_LogsUnimplemented(t *testing.T) {
	h := NewRunHandler(store.NewMemoryStore())
	err := h.Logs(context.Background(), connect.NewRequest(&v1alpha1.RunLogsRequest{Colony: "acme", Name: "run-1"}), nil)
	if connect.CodeOf(err) != connect.CodeUnimplemented {
		t.Fatalf("Logs err = %v, want CodeUnimplemented", err)
	}
}
