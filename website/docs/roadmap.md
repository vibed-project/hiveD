---
sidebar_position: 6
title: Roadmap and known limitations
---

# Roadmap and known limitations

hiveD is pre-1.0 and pre-alpha. The API surface under `proto/hived/v1alpha1`
may still change between minor versions, and all five ADRs are Draft.

Work is organized into milestones. M0 (foundations) is complete and shipped as
v0.1.0. M1 (first colony) is in progress: its job is to make a Run actually
execute.

## M0: shipped in v0.1.0

| Area | What landed |
|---|---|
| Governance | LICENSE, DCO, NOTICE, CONTRIBUTING, SECURITY, `docs/PROJECT.md`, ADR-0001 through ADR-0004 (all Draft) |
| Proto | Colony, Agent, AgentVersion, Run and Event definitions with typed `Apply`/`Get`/`List`/`Watch` services, generated via buf and connect-go, `gen/` checked in |
| Store | Postgres-backed resource store: shared `resources` and `events` tables, optimistic concurrency via `resourceVersion`, `generation` bumped only on spec change, AgentVersion immutability enforced, plus an in-memory implementation for tests. Watch is polling. |
| Keeper | `hived-keeper` with `serve`, `migrate` and `version`; the resource API for all five kinds |
| CLI | `hived` with `apply -f`, `get`, `watch`, `events`, `run` and `version` |
| Dev loop | `deploy/compose` Keeper plus Postgres; a mindD placeholder behind the `mind` profile, not required for a clean `up` |
| CI | build, race, import boundary, proto drift, lint, store integration, and main-only container jobs |
| Release | Tag-triggered releases: multi-arch image, cross-platform binaries, `checksums.txt` |

## M1: landed so far

These also shipped in v0.1.0, ahead of the implementations that will use them.
All are additive proto changes, so building the rest does not require a
breaking API change.

- The **`Tool` resource**: `ToolService` with `Apply`/`Get`/`List`/`Watch`, and
  types MCP, BUILTIN and AGENT. `hived apply`, `get` and `watch` understand
  the kind.
- The **Drone contract**: `DroneService` with `Bootstrap`, `Heartbeat` and
  `Finish`, plus `RunManifest`, `MemoryConfig`, `ToolDescriptor` and
  `ControlSignal`.
- The **Tool Broker contract**: `ToolBrokerService` with `ListRunTools` and
  `CallTool`, including the `(run, step, callId)` idempotency and `replayed`
  semantics that make a tool call survive a Cell restart.
- `RunService.Logs`, which returns `CodeUnimplemented` until the Executor
  exists.
- **Run scheduler fields**: `RunPhase.TIMED_OUT`, `RunSpec.cancel`,
  `RunStatus.attempt`, `RunStatus.observedGeneration`,
  `RunStatus.lastHeartbeatAt`.
- **ADR-0005** (Draft): the Event `type` vocabulary and the Keeper/Drone
  emitter split.

## M1: in scope, not built yet

This is the honest gap. Every item below is planned for the current milestone
and does not exist in v0.1.0.

| Not built | What it will do |
|---|---|
| Scheduler | Reconcile Run objects: resolve the AgentVersion, evaluate policy, mint tokens, build the Cell artifact, select an Executor, track lifecycle, handle resume. Without it a Run stays `PENDING`. |
| Executor interface and `local-docker` | Provision a Cell. `local-docker` speaks the raw Engine API over a unix socket and works on podman and docker. |
| `hived-drone` binary | The per-Cell runtime: the default agent loop, checkpoint and resume, and the Drone side of `DroneService`. |
| Model Gateway | OpenAI-compatible in, OpenAI-compatible out, plus a `fake` provider for tests. |
| Tool Broker implementation | The proto contract exists; the broker, one MCP server and the built-in `spawn_run` do not. |
| Real PASETO tokens | ADR-0002's v4.public issuance and verification. `internal/identity` is a no-op stub today. |
| Real mindD integration | kv and episodic through a vendored proto and a hand-written connect client, per ADR-0004. |
| `hived logs --follow` | The flag parses; the command fails. |
| `examples/hello-agent` | No examples directory exists. |

**Definition of done for M1:** kill the Cell mid-run and the Run resumes from
its last checkpoint in a new Cell.

## Explicitly out of scope until a later milestone

These are not "coming soon". They are deliberately deferred, and a change that
seems to require one of them should be flagged rather than built.

| Deferred | Note |
|---|---|
| The vibeD executor and lane hints | ADR-0003 fixes the contract; the implementation is later. |
| The Policy engine and approval flow | `hived approve` stays a stub. `Tool.spec.riskClass` is stored, not enforced. |
| Sessions as a resource | `Run.spec.sessionRef` is stored as a string with nothing behind it. |
| The Keeper's MCP surface | Driving hiveD from a coding agent. |
| The Python SDK (`hived-py`) | |
| The Helm chart | Compose is the only supported deployment today. |
| Streaming responses in the Model Gateway | |
| A bidirectional Drone control stream | Heartbeat polling is the M1 contract. |
| Postgres `LISTEN`/`NOTIFY` for Watch | The poller is sufficient for the Scheduler. |
| OpenTelemetry export | `internal/obs` wires slog and Prometheus only, and exists as the seam a tracer provider hangs on. |
| The run inspector UI | |
| OpenAPI publication | |

## Known limitations in v0.1.0

Read these before running anything that matters against a v0.1.0 Keeper.

**`List` is not tenant-scoped when `options.colony` is empty.** It returns
every Colony's resources. Authentication is a deliberate no-op stub in this
milestone (`internal/identity`), so there is no privilege boundary to cross
today, but `Principal.Colony` is not read anywhere: enabling real auth will
not close this by itself. Do not expose a v0.1.0 Keeper to untrusted callers.
Treat it as single-tenant. See `SECURITY.md`.

**No Scheduler or Executor exists**, so a Run is created and stays `PENDING`.
`hived logs` and `hived approve` exit non-zero by design.

**Writes do not check that the referenced Colony or Agent exists**, so a typo
creates an orphaned resource.

**`Watch` ignores `labelSelector`** (`List` honours it), holds a pool
connection while delivering, and swallows query errors while continuing to
emit bookmarks. A watcher cannot distinguish "idle" from "database down".

**`hived watch` ignores `--output`** and does not reconnect after a Keeper
restart.

**All five ADRs are Draft.** The contracts they describe may still change.

## Security posture

A v0.1.0 Keeper accepts any bearer token, including none, and returns a fixed
dev principal. hiveD does not sandbox anything itself; that is the Executor's
job, and no Executor ships. ADR-0004 records a further accepted gap for when
mindD integration lands: mindD scopes tokens by a flat tenant claim plus
admin-predeclared namespace globs, so under the interim mapping a compromised
Run token could reach sibling Runs' memory within the same colony. The
`run/<runID>/` key prefix organizes data; it is not a boundary.

Report vulnerabilities privately through GitHub's security advisory form on
the repository, not as a public issue. See `SECURITY.md`.

## Where things are decided

Milestone scope lives in `CLAUDE.md`, the design brief in `docs/PROJECT.md`,
and cross-cutting contracts in `docs/adr/`. A decision that establishes or
alters a contract another component depends on gets an ADR, not just a code
comment.
