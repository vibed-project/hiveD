# ADR-0001: Scope and principles

## Status
Draft

## Context

hiveD sits alongside two sibling repositories in this ecosystem — vibeD
(sandbox execution) and mindD/MemorySidecar (durable memory) — and needs a
written boundary for what it is and is not, before any code makes that
boundary implicit and hard to change. Agent frameworks (CrewAI, LangGraph,
ADK, the OpenAI/Anthropic SDKs) already solve the in-process problem of
writing an agent loop; they do not solve where an agent runs, under what
identity, with what budget, how it is paused/resumed/audited/stopped, or
how many agents coordinate without stepping on each other. hiveD exists to
answer that operational problem in a vendor-neutral, self-hostable way.

## Decision

hiveD is an agent **control plane** — not a runtime, not an orchestration
DSL, not a model server, and not a framework. Concretely:

- **Framework-agnostic.** hiveD does not care which library an agent's
  logic uses. It ships a minimal runtime (the Drone) and thin SDKs, and
  treats third-party agent frameworks as user code running inside a Cell.
- **Composable, not monolithic.** hiveD, vibeD, and mindD are three
  repositories with hard API contracts. Each is useful alone. hiveD talks
  to other executors and other memory backends through the same
  interfaces it uses for vibeD and mindD — it does not hard-depend on
  either sibling's internals (enforced by `scripts/check-import-boundary.sh`).
- **Governance is a first-class primitive, not an add-on.** Identity,
  policy, budgets, approval gates, and audit are in the core resource
  model from day one — not retrofitted once something goes wrong.
- **Boring, operable software.** Go binaries, Postgres, OpenTelemetry,
  protobuf/connect-go, a Helm chart. No exotic dependencies, no bespoke
  storage engine.
- **Agent-operable.** Everything a human can do through the CLI or UI can
  also be done by an agent through an MCP interface, so coding agents can
  create, run, and inspect hiveD agents the same way they drive vibeD's
  `deploy_artifact`.
- **Resource-oriented, Kubernetes-like semantics, no Kubernetes
  dependency.** Every resource (Colony, Agent, AgentVersion, Run, Event,
  ...) follows `metadata`/`spec`/`status` conventions with `generation`
  and `resource_version` for optimistic concurrency and watch, and the
  Keeper exposes `apply`/`get`/`list`/`watch` uniformly across them. This
  buys a well-understood reconciliation model without requiring a
  Kubernetes cluster to run hiveD itself.
- **Hard split between control and execution.** The Keeper (control
  plane) never executes agent logic. The Drone (per-Cell runtime) never
  makes policy decisions. This is structural — which binary code lives
  in — not a runtime check to bypass.
- **Executors are pluggable behind one interface** (see ADR-0003 for the
  first concrete implementation, targeting vibeD). hiveD does not own
  sandbox technology.

### Non-goals (v0.x)

- No orchestration DSL, no graph builder, no visual agent builder.
- No own vector database, no own sandbox technology, no own model
  serving — mindD, vibeD, and existing inference providers serve those.
- No Kubernetes CRDs or operator in the core (a `hived-operator` may wrap
  the API later, out of tree).
- No fine-tuning, no evals framework beyond storing traces and exposing
  them.
- No end-user chat product.

## Consequences

- Every new resource kind needs a store schema entry before it needs a
  controller — the API surface (proto + store semantics) is the contract,
  not the Go struct shape behind it.
- Because the Keeper/Drone split is structural, M0 (which has no Drone at
  all) can still validate the Keeper-side half of every invariant that
  matters: no resource kind's handler is allowed to reach into anything
  that looks like agent execution.
- The self-contained-module boundary means hiveD's own dependency surface
  toward vibeD and mindD must be an interface hiveD defines, not whatever
  those repos happen to expose internally — see ADR-0003 and ADR-0004.

## Alternatives considered

- **Wrap an existing framework (e.g. LangGraph) as the core.** Rejected:
  those frameworks solve the in-process loop problem, not the operational
  one hiveD targets, and coupling the control plane to one framework's
  object model would work against the framework-agnostic principle.
- **Build directly on Kubernetes (CRDs + operator) instead of a bespoke
  resource store.** Rejected for the core: it would force every hiveD
  installation to run on Kubernetes, which is a much heavier operational
  requirement than "a Postgres database." A `hived-operator` remains
  possible later as an optional, out-of-tree integration.
