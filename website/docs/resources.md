---
sidebar_position: 4
title: Resource reference
---

# Resource reference

Five resource kinds exist today: `Colony`, `Agent`, `AgentVersion`, `Run` and
`Tool`, plus the append-only `Event`. All are defined in
`proto/hived/v1alpha1`, which is the source of truth.

**Field names.** The proto uses `snake_case`; the JSON and YAML wire form uses
`lowerCamelCase`. Write `displayName`, `agentRef`, `maxConcurrentRuns` in a
manifest, not `display_name`. Unknown fields are rejected.

**Enums** are written as their full proto name, for example
`type: TOOL_TYPE_MCP`. **Durations** use the protobuf JSON form, for example
`timeout: 300s`. **Money** fields are decimal strings such as `"12.50"`, never
floats, to avoid drift in budget accounting.

## ObjectMeta

Every kind except `Event` embeds `metadata`. `Event` does not: it is not a
reconciled resource, it is a fact that happened.

| Field | Type | Meaning |
|---|---|---|
| `name` | string | Unique within `(kind, colony)`. Immutable after creation. |
| `colony` | string | The owning Colony. Required for colony-scoped kinds; empty for `Colony` itself, which is hive-scoped. |
| `uid` | string | Server-assigned, stable across renames. Renames are not supported today; the field exists for forward compatibility. |
| `generation` | int64 | Increments **only** when the spec changes. A status-only write leaves it alone, which is what makes `status.observedGeneration` meaningful. |
| `resourceVersion` | int64 | Increments on **every** write, spec or status. This is the watch cursor and the optimistic-concurrency token. |
| `labels` | map | Free-form key/value. `List` filters on them; `Watch` currently does not. |
| `annotations` | map | Free-form key/value, not used for selection. |
| `createdAt` | timestamp | Server-assigned. |
| `updatedAt` | timestamp | Server-assigned. |
| `deletedAt` | timestamp | Set on soft delete. A `Watch` stream emits `DELETED` when this transitions from unset to set. No API sets it today: there is no delete operation on any service. |

`uid`, `generation`, `resourceVersion` and the timestamps are server-owned.
Setting them in a manifest does not do anything useful.

### Condition

A shared status building block used by most kinds.

| Field | Type |
|---|---|
| `type` | string |
| `status` | string |
| `reason` | string |
| `message` | string |
| `lastTransitionTime` | timestamp |
| `observedGeneration` | int64 |

Nothing writes conditions in v0.1.0. There are no controllers yet.

## Colony

Hive-scoped. The tenant and isolation boundary. It owns agents, policies,
quotas, tool registrations and a memory scope root.

`spec`:

| Field | Type | Meaning |
|---|---|---|
| `displayName` | string | Human-readable name. |
| `quotas` | Quotas | See below. Stored, not enforced. |
| `policies` | []string | Names of Policy resources evaluated for Runs in this Colony. Policy is not implemented; accepted and stored, never evaluated. |
| `memoryRoot` | string | The mindD path root this Colony's Runs are scoped under. See ADR-0004 for the interim mapping onto mindD's flat tenant claim. |
| `executorAllowlist` | []string | Executors Runs here may use. Stored, not enforced. |
| `modelAllowlist` | []string | Models Runs here may use. Stored, not enforced. |

`spec.quotas`:

| Field | Type | Meaning |
|---|---|---|
| `maxConcurrentRuns` | int64 | |
| `maxRunsPerHour` | int64 | |
| `tokenBudget` | int64 | |
| `costBudget` | string | Decimal string, for example `"12.50"`. |

`status`: `conditions`, `observedGeneration`.

```yaml
apiVersion: hived/v1alpha1
kind: Colony
metadata:
  name: acme
spec:
  displayName: Acme Corp
  memoryRoot: colony/acme
  quotas:
    maxConcurrentRuns: 4
    costBudget: "25.00"
```

## Agent

Colony-scoped. The mutable envelope around a versioned definition. The real
spec lives on `AgentVersion`.

`spec`:

| Field | Type | Meaning |
|---|---|---|
| `description` | string | That is the whole spec. Everything else is on the version. |

`status`:

| Field | Type | Meaning |
|---|---|---|
| `current` | string | Name of the active AgentVersion. There is no controller to set this, so it has to be written explicitly. |
| `versions` | []string | Known version names. |
| `conditions` | []Condition | |

## AgentVersion

Colony-scoped, and **the immutable one**. `Apply` is create-or-identical-noop:
re-applying an identical spec succeeds and changes nothing, applying a
different spec to an existing `(colony, name)` is rejected as immutable. To
change an agent, create a new version.

`spec`:

| Field | Type | Meaning |
|---|---|---|
| `agent` | string | Name of the owning Agent. |
| `version` | string | Version label, for example `v1`. |
| `instructions` | string | The system prompt. |
| `model` | ModelSpec | |
| `tools` | []ToolRef | References to Tool resources. |
| `memory` | MemorySpec | |
| `policies` | []string | Policy names. Stored, never evaluated. |
| `runtime` | RuntimeSpec | |
| `limits` | LimitsSpec | |
| `io` | IOSpec | |

`spec.model` (ModelSpec):

| Field | Type |
|---|---|
| `provider` | string |
| `name` | string |
| `params` | map |

`spec.tools[]` (ToolRef):

| Field | Type | Meaning |
|---|---|---|
| `name` | string | The Tool resource's name. |
| `version` | string | |
| `config` | map | Per-tool options. |

`spec.memory` (MemorySpec):

| Field | Type | Meaning |
|---|---|---|
| `root` | string | Memory path root. |
| `blocks` | []string | mindD blocks this version uses: `kv`, `episodic`, `semantic`, `artifact`, `lease`. |

`spec.runtime` (RuntimeSpec):

| Field | Type | Meaning |
|---|---|---|
| `executor` | string | A **hint**, not a binding assignment. See ADR-0003. |
| `image` | string | |
| `env` | map | |

`spec.limits` (LimitsSpec):

| Field | Type | Meaning |
|---|---|---|
| `maxSteps` | int64 | |
| `maxTokens` | int64 | |
| `maxCost` | string | Decimal string. |
| `timeout` | duration | Run wall clock, for example `300s`. |

`spec.io` (IOSpec):

| Field | Type | Meaning |
|---|---|---|
| `inputSchema` | string | JSON Schema, stored as a string. |
| `outputSchema` | string | JSON Schema, stored as a string. |

`status`: `conditions`.

None of `model`, `tools`, `memory`, `runtime`, `limits` or `io` is acted on in
v0.1.0. They are stored faithfully so that the Scheduler, Drone and Tool
Broker can read them without a schema change.

## Run

Colony-scoped. One running instance of an Agent, a "Worker" in prose.

`RunService.Apply` always persists a Run in `RUN_PHASE_PENDING` at `attempt`
0. The Scheduler owns every later transition and is the only writer of
`status`. Since there is no Scheduler, a Run stays `PENDING`.

`spec`:

| Field | Type | Meaning |
|---|---|---|
| `agentRef` | string | Agent to run. |
| `version` | string | AgentVersion to pin. |
| `input` | struct | Arbitrary JSON object passed to the Run. |
| `sessionRef` | string | Session grouping. Session is not a resource yet. |
| `parentRunRef` | string | Set when a Run was spawned by another Run. |
| `executorHint` | string | Maps to an Executor lane per ADR-0003. Consumed by nothing today. |
| `cancel` | bool | Desired state, Kubernetes style: setting it asks the Scheduler to stop the Run and move it to `RUN_PHASE_CANCELLED`. Clearing it has no effect once the Run is terminal. |

`status`:

| Field | Type | Meaning |
|---|---|---|
| `phase` | RunPhase | See below. |
| `cellRef` | string | The Cell this attempt runs in. |
| `executor` | string | Which Executor provisioned it. |
| `identity` | string | Opaque reference to the Run's issued token. Never populated today; `internal/identity` is a stub. See ADR-0002. |
| `startedAt` / `finishedAt` | timestamp | |
| `steps` | int64 | |
| `tokens` | TokenUsage | `input`, `output`, `total`. |
| `cost` | string | Decimal string. |
| `checkpoint` | string | Last checkpoint reference. |
| `result` | struct | Terminal result. |
| `message` | string | Human-readable status detail. |
| `conditions` | []Condition | |
| `attempt` | int32 | Counts Cell incarnations. 0 until the first Cell is provisioned, incremented each time the Scheduler re-provisions after a lost Cell. Drone reports carry the attempt they belong to, so a stale Cell cannot overwrite its successor's status. |
| `observedGeneration` | int64 | |
| `lastHeartbeatAt` | timestamp | |

`RunPhase`:

| Value | Meaning |
|---|---|
| `RUN_PHASE_UNSPECIFIED` | Zero value. |
| `RUN_PHASE_PENDING` | Created, not yet picked up. Every Run in v0.1.0. |
| `RUN_PHASE_SCHEDULING` | The provisioning window: Cell requested, Drone not yet bootstrapped. |
| `RUN_PHASE_RUNNING` | |
| `RUN_PHASE_PAUSED` | |
| `RUN_PHASE_SUCCEEDED` | Terminal. |
| `RUN_PHASE_FAILED` | Terminal. |
| `RUN_PHASE_CANCELLED` | Terminal. |
| `RUN_PHASE_TIMED_OUT` | Terminal. The Keeper stopped the Run because `AgentVersion.spec.limits.timeout` elapsed. |

## Tool

Colony-scoped registration of something an Agent may call. An AgentVersion
references Tools by name in `spec.tools`.

The design point: the Tool Broker resolves those references and is the only
component that ever contacts `spec.endpoint`. The Drone never sees a Tool's
endpoint or credentials. The Broker is not implemented, so today a Tool is a
record and nothing more.

`spec`:

| Field | Type | Meaning |
|---|---|---|
| `type` | ToolType | See below. |
| `description` | string | |
| `endpoint` | string | MCP server URL, for `TOOL_TYPE_MCP`. |
| `builtin` | string | Names the broker-internal implementation, for `TOOL_TYPE_BUILTIN`. |
| `agentRef` | string | Agent to spawn, for `TOOL_TYPE_AGENT`. |
| `riskClass` | string | `read`, `write`, `destructive` or `external-money`. Feeds policy. Stored, **not enforced**: policy evaluation is a later milestone. |

`ToolType`:

| Value | Meaning |
|---|---|
| `TOOL_TYPE_UNSPECIFIED` | Zero value. |
| `TOOL_TYPE_MCP` | A Model Context Protocol server reached at `spec.endpoint`. |
| `TOOL_TYPE_BUILTIN` | Implemented inside the Tool Broker, for example `spawn_run` or `wait_run`. `spec.endpoint` is empty. |
| `TOOL_TYPE_AGENT` | Calling the tool spawns a child Run of `spec.agentRef`. |

`status`: `conditions`, `observedGeneration`.

```yaml
apiVersion: hived/v1alpha1
kind: Tool
metadata:
  name: docs-search
  colony: acme
spec:
  type: TOOL_TYPE_MCP
  description: search the internal docs corpus
  endpoint: https://mcp.internal.example/docs
  riskClass: read
```

## Event

Append-only and per Run. No `metadata`, no spec/status split, no update, no
delete.

| Field | Type | Meaning |
|---|---|---|
| `colony` | string | |
| `run` | string | |
| `seq` | int64 | Monotonic per `(colony, run)`, assigned by the store on `Append`. This ordering is authoritative. |
| `type` | string | See the vocabulary below. |
| `ts` | timestamp | Informational, and may be skewed between Keeper and Drone clocks. A caller-supplied value is preserved. |
| `payload` | struct | A summary, never a transcript. |
| `resourceVersion` | int64 | Shares the same sequence space as resources' watch cursor, so a single cursor orders both. |

### Event type vocabulary

ADR-0005 fixes a closed vocabulary of UpperCamelCase names and splits it by
emitter. **Nothing emits any of these in v0.1.0**, because neither emitter
exists. The vocabulary is fixed in advance so that `hived events`, a future
run inspector and any SIEM export can key off `type` without churn.

Keeper-emitted, where the Scheduler is the only writer:

| Type | When |
|---|---|
| `RunScheduled` | PENDING to SCHEDULING; AgentVersion resolved, policy passed |
| `CellProvisioned` | Executor returned a Cell handle |
| `CellLost` | Cell exited or vanished without `Finish`, or heartbeats stopped |
| `RunTimedOut` | `limits.timeout` elapsed |
| `RunCancelled` | `spec.cancel` honoured |

Drone-emitted, authenticated with the Run token:

| Type | When |
|---|---|
| `RunStarted` | First step of attempt 1 |
| `RunResumed` | First step of attempt > 1, after loading the checkpoint |
| `ModelCalled` | A Model Gateway call returned |
| `ToolCalled` | A Tool Broker call returned |
| `RunCheckpointed` | Checkpoint written |
| `RunFinished` | `Finish` accepted |

Payloads carry identifiers and counts, never prompt text, tool arguments or
tool results. Those belong in mindD's episodic block, which keeps the Postgres
event log small and free of user data by default. Approval and policy events
(`ApprovalRequested`, `ApprovalDecided`, `PolicyDenied`) are deliberately
absent; they arrive with the Policy engine.

## List and Watch

`ListOptions`, accepted by both `List` and `Watch`:

| Field | Type | Meaning |
|---|---|---|
| `colony` | string | Scopes the list. Ignored for `Colony`, which is hive-scoped. |
| `labelSelector` | map | Exact-match label filter. Honoured by `List`, **ignored by `Watch`**. |
| `pageSize` | int32 | Clamped to a maximum of 1000. |
| `pageToken` | string | Opaque keyset cursor. A malformed token is reported as `InvalidArgument`. |

`ListMeta`, returned by every `List`:

| Field | Type | Meaning |
|---|---|---|
| `nextPageToken` | string | Empty when the page is the last one. |
| `resourceVersion` | int64 | The highest `resourceVersion` observed in this response. |

### The List then Watch handoff

This is the contract that lets a client build a complete, gap-free view.

1. `List` the kind you care about. Page through until `nextPageToken` is
   empty. Keep `listMeta.resourceVersion`.
2. `Watch(sinceResourceVersion = listMeta.resourceVersion)`.
3. Every change after the List arrives on the stream exactly once. Nothing is
   missed and nothing is double-processed.
4. While idle, the stream emits `BOOKMARK` events every 30 seconds. They carry
   no object, only a newer `resourceVersion`, so a watcher can advance its
   saved cursor without a real change.

`WatchEventType` is `ADDED`, `MODIFIED`, `DELETED` or `BOOKMARK`, mirroring
Kubernetes' watch semantics.

The handoff depends on `resourceVersion` order matching commit order, which is
why the store allocates it from a transactional counter rather than a
sequence. See [Architecture](./architecture.md#how-the-store-works).

## Kinds that do not exist yet

`docs/PROJECT.md` describes several more resources. None of them is
implemented, and none has a proto message:

`Session`, `Policy`, `ModelBinding`, `Executor`, `Credential`, `Approval`.

`Run.spec.sessionRef` and the `policies` fields on Colony and AgentVersion
reference kinds that have no resource behind them yet. They are stored as
strings.
