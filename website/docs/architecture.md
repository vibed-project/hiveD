---
sidebar_position: 3
title: Architecture
---

# Architecture

## System context

```text
                 +-------------------+          +------------------+
   CLI / UI /    |                   |  gRPC    |   Executors      |
   MCP / SDK --->|   hiveD Keeper    |--------->|  vibeD (default) |
                 |  (control plane)  |          |  local-docker    |
                 |                   |          |  (k8s job later) |
                 +----+---------+----+          +--------+---------+
                      |         |                        |
             issues   |         | events/control         | provisions Cell
     capability tokens|         | stream                 v
                      |    +----+--------------------------------+
                      |    |  Cell (sandbox)                     |
                      |    |  hived-drone + agent code           |
                      |    |    |-> model calls  -> hiveD Model Gateway
                      |    |    |-> tool calls   -> hiveD Tool Broker -> MCP servers
                      |    |    |-> memory ops   -> mindD (kv/episodic/semantic/artifact/lease)
                      v    |    |-------------------------------------+
                  mindD <--+
```

Of that picture, v0.1.0 implements the Keeper box and its Postgres-backed
resource API. The Cell, the Drone, the Executors, the Model Gateway and the
Tool Broker are not built. Their wire contracts are defined in
`proto/hived/v1alpha1` so that building them does not require a breaking API
change.

## The Keeper / Drone split

hiveD is two binaries with one hard line between them.

| | Keeper (`hived-keeper`) | Drone (`hived-drone`) |
|---|---|---|
| Where it runs | Control plane, alongside Postgres | Inside every Cell |
| Owns | Resources, scheduling, identity, policy, budgets, audit | The agent loop, checkpoints, tool and model calls |
| Never does | Executes agent logic | Makes a policy decision |
| Status | Implemented | Not built yet |

The reason for the split is that agent code is untrusted. It is arbitrary
user code, often driven by model output, running in a sandbox. If the
component that decides "may this tool be called" also runs that code, then
compromising the code compromises the decision. So the decision lives in a
different process, on a different machine, reachable only over an
authenticated API.

The split is structural, not a runtime check. It is enforced by which binary
the code lives in. A Drone has no code path that evaluates a policy, and the
Keeper has no code path that runs an agent step, so there is nothing to
bypass. See ADR-0001.

## Invariants

These hold across the codebase and should hold across any contribution.

- **Never make policy decisions in the Drone.**
- **Never execute agent logic in the Keeper.**
- **AgentVersion specs are immutable after creation.** Re-applying an
  identical spec is a no-op success. Applying a different spec to an existing
  version is rejected.
- **Events are append-only.** No update, no delete. `EventService` has
  `Append`, `List` and `Watch`, and deliberately no `Apply` and no `Get`.
- **The module stays self-contained.** hiveD imports no sibling
  `github.com/vibed-project/*` module, which includes mindD. Enforced in CI by
  `scripts/check-import-boundary.sh` (`make boundary`).

## Resource model

Every kind except `Event` follows `metadata` / `spec` / `status` conventions
with Kubernetes-like semantics, and no Kubernetes dependency. The Keeper
exposes `Apply`, `Get`, `List` and `Watch` uniformly across all of them.

```text
Colony (hive-scoped)
  |
  +-- Agent                      spec: description
  |     |                        status.current -> an AgentVersion name
  |     +-- AgentVersion         the immutable definition
  |
  +-- Tool                       MCP | BUILTIN | AGENT
  |
  +-- Run                        spec.agentRef -> Agent, spec.version
        |
        +-- Event                append-only, per (colony, run), ordered by seq
```

| Kind | Scope | Mutability |
|---|---|---|
| Colony | Hive | Mutable |
| Agent | Colony | Mutable |
| AgentVersion | Colony | Spec immutable |
| Tool | Colony | Mutable |
| Run | Colony | Spec mutable (`cancel`); status written by the Scheduler |
| Event | Colony + Run | Append-only |

Relationships are by name and are **not** validated in v0.1.0. Writes do not
check that the referenced Colony or Agent exists, so a typo creates an
orphaned resource. Field-level detail is in the
[resource reference](./resources.md).

## How the store works

`internal/store` is one generic implementation shared by every kind, not one
table per kind.

**Shared tables.** All spec/status kinds live in a single `resources` table
keyed by `UNIQUE (kind, colony, name)`, with `spec` and `status` held as
`jsonb`. The store never interprets their contents beyond structural
comparison; the `api` package owns protobuf marshalling. Events live in a
separate append-only `events` table with `UNIQUE (colony, run, seq)`.

**Generation versus resource version.** `generation` increments only when the
spec changes, so a status-only write leaves it alone and
`status.observedGeneration` is meaningful once controllers exist.
`resourceVersion` increments on every write and is both the watch cursor and
the optimistic-concurrency token.

**Optimistic concurrency.** `Apply` takes an optional `ifResourceVersion`. If
it is set and does not match the stored value, the write is rejected with
`CodeAborted`.

**No-op applies stay no-ops.** Only a real change to spec, labels or
annotations advances `resourceVersion`. Re-applying an identical manifest does
not wake every watcher, which otherwise turns a controller that reconciles on
its own watch into a self-sustaining loop.

**A transactional resource_version counter.** `resourceVersion` comes from a
single-row counter updated inside the writing transaction, not from a Postgres
sequence. Sequences are deliberately non-transactional: `nextval()` takes
effect immediately and never rolls back, so version order need not match
commit order. Writer A could take version 878 and commit slowly while writer B
takes 879 and commits first; a watcher polling in between would deliver 879,
advance its cursor past 878, and lose that row permanently. With a counter,
`UPDATE ... RETURNING` holds the row lock until commit, so a transaction
cannot obtain version N+1 until the holder of N has committed. The trade-off
is that all version allocation serializes on one row, which is sub-millisecond
and far above what a control plane needs.

**Event sequencing.** `seq` is assigned under a transaction-scoped advisory
lock keyed on `(colony, run)`, so concurrent appends to the same Run do not
collide on the unique constraint. Appends to different Runs never contend.

**Pagination.** `List` uses an opaque keyset page token over `(colony, name)`,
because name is unique only per `(kind, colony)`. Page size is clamped to
1000.

**Watch is polling.** The Postgres watch implementation polls at 250ms and
emits a `BOOKMARK` every 30 seconds so an idle watcher can still checkpoint
its cursor. Postgres `LISTEN`/`NOTIFY` is a planned replacement behind the
same since-cursor contract; callers will not need to change. There are real
rough edges today: `Watch` ignores `labelSelector` (`List` honours it), holds
a pool connection while delivering, and swallows query errors while continuing
to emit bookmarks, so a watcher cannot distinguish "idle" from "database
down".

**List then Watch.** A `List` response carries
`listMeta.resourceVersion`, the highest version observed in that response. A
client that then calls `Watch(sinceResourceVersion = that value)` sees every
subsequent change exactly once, with no gap and no duplicate.

## API surface

The Keeper serves connect-go on one port, which gives gRPC, gRPC-Web and
HTTP/JSON from the same handlers. `proto/hived/v1alpha1` is the source of
truth and `gen/` is generated and checked in; CI fails on drift, and nothing
under `gen/` is ever hand-edited.

One server interceptor covers every service: it authenticates, recovers panics
as `CodeInternal`, logs, and records the `hived_keeper_rpc_total` metric.
Authentication in v0.1.0 is `identity.StubVerifier`, which accepts any token
and returns a fixed dev principal. `/healthz` and `/readyz` are on the API
port; `/metrics` is on a separate port so scraping never shares a listener
with application traffic.

## Relationship to vibeD and mindD

hiveD, vibeD and mindD are three repositories with hard API contracts, not one
system in three directories.

- **vibeD** provisions Cells. It is the default Executor and the best-supported
  one, but it reaches hiveD only through the `Executor` interface
  (`Capabilities`, `Provision`, `Status`, `Signal`, `Logs`, `Destroy`).
  ADR-0003 fixes the contract: artifact layout (`/drone`, `/app`,
  `/run.json`), the `HIVED_*` environment variables the Drone bootstraps
  from, secrets injected by reference and never in the clear, deny-by-default
  egress enforced by the Executor and never by the Drone, and the lane hint
  being a hint the Keeper or Executor may override.
- **mindD** holds durable memory: `kv` for working state and checkpoints,
  `episodic` for the transcript and tool-call log, `semantic` for retrieval,
  `artifact` for large outputs, `lease` for coordination between sibling Runs.
  ADR-0004 records the mapping and one honest gap: mindD scopes capability
  tokens by a flat tenant string plus admin-predeclared namespace globs, not
  by a per-run path, so under the interim mapping a leaked Run token can reach
  sibling Runs' memory within the same colony. The `run/<runID>/` key prefix
  is a naming convention, not a security boundary, until that changes.

**hiveD never imports either module directly.** Integration goes through an
interface that hiveD defines plus a generated or hand-written client, so
hiveD's security posture does not implicitly depend on a sibling repository's
internals. `scripts/check-import-boundary.sh` enforces this in CI. This also
means hiveD's dependency surface toward vibeD and mindD is something hiveD
controls, rather than whatever those repos happen to expose.

Neither integration ships in v0.1.0. The compose file's mindD service is a
profile-gated placeholder, and no Executor exists at all.

## Repository layout

```text
cmd/keeper/            hived-keeper binary: serve | migrate | version
cmd/hived/             hived CLI (Cobra)
internal/store/        Postgres resource store: apply/get/list/watch, migrations
internal/api/          Keeper API handlers (connect-go), one file per kind
internal/identity/     token issuance, stub only today
internal/config/       env-driven Keeper config
internal/obs/          slog + Prometheus wiring
internal/version/      ldflags-injected build info
proto/hived/v1alpha1/  resource + service definitions, source of truth
gen/                   generated Go code from proto/, never hand-edited
scripts/               import boundary check, containerized toolchain wrappers
deploy/compose/        local dev docker-compose stack
docs/adr/              architecture decision records
```

## Architecture decision records

Five ADRs exist, all in Draft status, which means the contracts they describe
may still change.

| # | Title |
|---|---|
| 0001 | Scope and principles |
| 0002 | Identity and capability tokens (PASETO v4.public, Keeper as sole issuer) |
| 0003 | vibeD executor contract |
| 0004 | mindD memory usage |
| 0005 | Event schema |
