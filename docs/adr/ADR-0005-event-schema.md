# ADR-0005: Event schema

## Status
Draft

## Context

`Event` (proto/hived/v1alpha1/event.proto) is an append-only, per-Run fact
with a free-form `type` string and a `payload` Struct. M0 defined the
envelope but no vocabulary, because nothing emitted events yet. M1 adds
two emitters with different trust levels: the Keeper (Scheduler) and the
Drone (running inside a Cell). `hived events`, the future run inspector,
and any SIEM export all key off `type`, so the vocabulary needs to be
fixed before the first emitter ships, and the split of who may emit what
needs to be explicit.

## Decision

`Event.type` is a closed vocabulary of UpperCamelCase names. Adding a name
is an additive change recorded here; renaming or removing one is a new
ADR.

**Keeper-emitted** (the Scheduler is the only writer; carries the
Keeper's view of the Run):

| type | when | payload |
|---|---|---|
| `RunScheduled` | PENDING → SCHEDULING; AgentVersion resolved, policy passed | `agent_version`, `executor` |
| `CellProvisioned` | Executor returned a Cell handle | `cell_ref`, `attempt` |
| `CellLost` | Cell exited or vanished without `Finish`, or heartbeats stopped | `cell_ref`, `attempt`, `reason` |
| `RunTimedOut` | limits.timeout elapsed | `attempt`, `step` |
| `RunCancelled` | spec.cancel honoured | `attempt`, `step` |

**Drone-emitted** (authenticated with the Run token; the Keeper's
interceptor rejects any Event whose `colony`/`run` differ from the token's
claims, so a Drone can only write its own Run's log):

| type | when | payload |
|---|---|---|
| `RunStarted` | first step of attempt 1 | `attempt` |
| `RunResumed` | first step of attempt > 1, after loading the checkpoint | `attempt`, `from_step`, `checkpoint_ref` |
| `ModelCalled` | a Model Gateway call returned | `step`, `model`, `tokens{input,output,total}`, `latency_ms` |
| `ToolCalled` | a Tool Broker call returned | `step`, `call_id`, `tool`, `is_error`, `replayed`, `latency_ms` |
| `RunCheckpointed` | checkpoint written | `step`, `checkpoint_ref` |
| `RunFinished` | `Finish` accepted | `phase`, `step`, `tokens` |

Rules:

- Payloads are summaries, never transcripts. Prompt text, tool arguments
  and tool results live in mindD's episodic block (ADR-0004); Events carry
  identifiers and counts so the Postgres event log stays small and free
  of user data by default.
- Every payload includes `attempt` where a Cell is involved, so a
  consumer can reconstruct the incarnation timeline without joining
  against Run status history.
- `seq` ordering per Run is authoritative; `ts` is informational and may
  be skewed between Keeper and Drone clocks.

## Consequences

- `internal/events` (M1) exposes one typed constructor per name above so
  the string never appears at an emit site.
- `hived events` renders `type` and a one-line summary of the payload;
  unknown types render raw, so a newer Keeper does not break an older CLI.
- Policy and approval events (`ApprovalRequested`, `ApprovalDecided`,
  `PolicyDenied`) are deliberately absent: they arrive with the Policy
  engine in M2 as an additive update to this ADR.

## Alternatives considered

- **Typed Event messages per kind (oneof).** Rejected for now: the
  vocabulary is still moving and a proto change per new event type is
  friction the run inspector does not need. Revisit if consumers want
  compile-time payload shapes.
- **Full transcripts in Event payloads.** Rejected: duplicates mindD's
  episodic block, grows Postgres unboundedly, and puts user content in the
  audit log by default. Consumers that need content read episodic with a
  scoped token.
