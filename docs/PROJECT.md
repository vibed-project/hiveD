# hiveD: Agent Control Plane and Runtime
Design brief for hiveD: vocabulary, architecture, resource model and technology decisions. This document is normative for terminology; see `docs/adr/` for the decisions behind individual contracts.
Status: pre-alpha. The M0 foundations are implemented; see README.md for what actually runs today.
---
## 1. One-line pitch
hiveD is a standalone, open source control plane that defines, schedules, governs and observes AI agents, running them inside isolated sandboxes (vibeD by default) and giving them durable memory (mindD by default). vibeD builds the cells, mindD keeps the memory, hiveD runs the colony.
## 2. Why this exists
Agent frameworks (CrewAI, LangGraph, ADK, OpenAI/Anthropic SDKs) solve the in-process problem: how to write an agent loop. They do not solve the operational problem: where does the agent run, under what identity, with which tools and budget, how is it paused, resumed, audited, and stopped, and how do many agents coordinate without stepping on each other. Hyperscalers solve that with proprietary managed runtimes. hiveD is the vendor-neutral, self-hostable answer, built from first principles rather than by wrapping an existing framework.
Design principles:
1. Framework-agnostic. hiveD does not care which library an agent's logic uses. It ships a minimal runtime and thin SDKs, and it treats third-party frameworks as user code inside the sandbox.
2. Composable, not monolithic. hiveD, vibeD and mindD are three repositories with hard API contracts. Each is useful alone. hiveD can use other executors and other memory backends through the same interfaces.
3. Governance is a first-class primitive, not an add-on. Identity, policy, budgets, approval gates and audit are in the core data model from day one.
4. Boring, operable software. Go binaries, Postgres, OpenTelemetry, OpenAPI and gRPC, Helm chart. No exotic dependencies.
5. Agent-operable. Everything a human can do through the CLI or UI can be done by an agent through an MCP interface, so coding agents can create, run and inspect agents.
## 3. Vocabulary (use consistently in code, docs and UI)
| Term | Meaning |
|---|---|
| Hive | A hiveD installation (control plane instance). Multi-tenant. |
| Colony | Tenant / isolation boundary. Owns agents, policies, quotas, tool registrations, memory scope root. |
| Agent | A versioned, immutable definition of an agent: instructions, model requirements, tools, memory scopes, policies, resource hints. |
| Worker | One running instance of an Agent, i.e. a Run. "Worker" in prose, `Run` in the API. |
| Cell | The isolated sandbox a Worker runs in. Provisioned by an Executor (vibeD general lane, vibeD fast lane, local docker, ...). |
| Session | Optional grouping of Runs that share conversational context and an episodic memory scope. |
| Tool | A registered capability a Worker may call: an MCP server, a built-in, or a child-agent spawn. |
| Policy | Rules attached to a Colony or Agent: allowlists, budgets, egress, approval gates, data classes. |
| Keeper | The hiveD control plane process itself (API server + controllers). Only used in internal naming, e.g. `hived-keeper`. |
| Drone | The runtime binary injected into every Cell that executes the agent loop and talks to the Keeper. Binary name `hived-drone`. |
## 4. System context
```
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
The Keeper never executes agent logic. The Drone never makes policy decisions. mindD holds all durable state of a Run so a Cell can be killed and the Run resumed elsewhere.
## 5. Components
### 5.1 Keeper (control plane), Go
Single deployable binary with subcommands, backed by Postgres. Internally organized as an API layer plus reconciling controllers over a versioned resource store (Kubernetes-like semantics without Kubernetes).
Services inside the Keeper:
- **Resource API**: CRUD and list/watch for all resources in section 6. gRPC first (buf, protobuf), REST generated from it (grpc-gateway or connect-go), OpenAPI published.
- **Scheduler**: reconciles `Run` objects. Resolves the Agent version, evaluates Policy, mints identity and mindD capability token, builds the Cell artifact (drone + agent code + run manifest), selects an Executor by hint and availability, provisions the Cell, tracks lifecycle (Pending, Provisioning, Running, WaitingForApproval, Suspended, Succeeded, Failed, Cancelled, TimedOut), enforces budgets and deadlines, and handles resume.
- **Executor manager**: plugin registry for execution backends behind one interface (section 7).
- **Identity service**: issues short-lived Run identity tokens (PASETO v4, matching mindD's lean) bound to colony/agent/run/session, plus mindD capability tokens scoped to the memory paths the Agent declares. Verifies Drone connections. Rotates keys. Designed so it can later be backed by SPIFFE/SPIRE.
- **Model Gateway**: OpenAI-compatible chat/completions endpoint for Drones, provider adapters outbound (OpenAI, Anthropic, Google, OpenAI-compatible self-hosted like vLLM/Ollama). Per-Run token accounting, budget enforcement, model resolution from capability requirements (`ModelBinding`), retries, structured output passthrough, streaming.
- **Tool Broker**: MCP-aware proxy. Registers Tools, resolves per-Run allowlists, brokers credentials so agent code never sees secrets, classifies risk, enforces approval gates (blocks the call, emits ApprovalRequested, resumes on decision), and records every call. Also exposes built-in tools: `spawn_run` (child agent), `wait_run`, `memory_*` convenience wrappers, `request_approval`.
- **Event bus and trace store**: typed, ordered event stream per Run (RunStarted, ModelCalled, ToolCalled, ApprovalRequested, MemoryWritten, RunCheckpointed, RunFinished, ...). Stored in Postgres, exported via OpenTelemetry (GenAI semantic conventions where they exist), streamed to CLI/UI over gRPC/SSE.
- **MCP surface**: the Keeper is itself an MCP server (`create_agent`, `run_agent`, `get_run`, `list_runs`, `approve`, `stream_events`), so a coding agent can drive hiveD the same way it drives vibeD's `deploy_artifact`.
### 5.2 Drone (runtime), Go
Small static binary injected into every Cell. Responsibilities, and nothing else:
1. Read the Run manifest (env/file): Run ID, Keeper endpoint, identity token, mindD endpoint and capability token, model gateway endpoint, tool broker endpoint, agent package location.
2. Open a bidirectional gRPC stream to the Keeper for events (up) and control signals (down: pause, resume, cancel, approval decisions, budget warnings).
3. Restore state from mindD if the Run is a resume (kv working state, episodic transcript pointer).
4. Execute the agent loop. Default loop: build context (instructions + retrieved memory + transcript window), call model through the gateway, parse tool calls, execute them through the broker, append to episodic memory, checkpoint working state to kv after every step, repeat until final answer, max steps, budget, or cancel.
5. Support user-provided loop code via a local IPC contract (section 8), so an agent written with any framework can run inside the Cell while the Drone still owns identity, checkpointing and eventing.
6. Coordinate with sibling Runs through mindD leases when the Agent declares shared scopes.
7. Exit with a typed result and a final checkpoint.
The Drone must be resumable: any Run can be killed at any step and restarted in a fresh Cell from its last checkpoint. This is hiveD's durable execution story and it depends only on mindD.
### 5.3 SDKs
- **hived-py**: Python. Defines agents (typed models mirroring the Agent resource), tool bindings, optional custom loop hooks, local dev runner that speaks the same Drone contract, client for the Keeper API. Published as `hived`.
- **hived-ts**: TypeScript. Same scope, second priority.
- Both are thin. No orchestration DSL. Multi-agent is done via `spawn_run` and shared memory scopes, not in-process graphs.
### 5.4 CLI: `hived`
kubectl-style verbs: `hived apply -f agent.yaml`, `hived run <agent> --input ...`, `hived get runs`, `hived logs <run> --follow`, `hived approve <run>`, `hived cancel`, `hived colony create`, `hived tool register`, `hived executor list`. Also `hived dev` for running an agent locally against a local Keeper with the docker executor.
### 5.5 UI
Deferred. Read-only run inspector first (timeline of events, tool calls, model calls with tokens and cost, memory diffs, approval queue). Built as a separate SPA talking to the REST/SSE API.
## 6. Resource model
All resources are colony-scoped unless noted, have `metadata` (name, uid, labels, annotations, createdAt, generation), and follow spec/status conventions.
- **Colony** (hive-scoped): tenant. Quotas, default policies, memory root path in mindD, executor allowlist, model allowlist.
- **Agent**: `spec` is immutable per version. Fields: `description`, `instructions` (system prompt, templated), `model` (capability requirements: modality, context window min, tool calling, structured output, cost class, or a pinned ModelBinding name), `tools` (list of Tool refs with per-tool options), `memory` (declared scopes and blocks: e.g. `kv: private`, `episodic: session`, `semantic: colony/knowledge/*` read-only, `lease: colony/shared-plan`), `policies` (Policy refs), `runtime` (loop: `default` or `custom`, package: OCI ref or inline files, entrypoint, lane hint: `fast`|`general`, resources), `limits` (max steps, max tokens, wall clock, cost cap), `io` (input JSON schema, output JSON schema). Versions are immutable; `AgentVersion` is a sub-resource; `Agent` has a `current` pointer.
- **Run**: `spec`: agentRef + version, input, sessionRef, parentRunRef, executorHint, overrides within policy. `status`: phase, cellRef, executor, identity, startedAt, finishedAt, steps, tokens, cost, lastCheckpoint, result or error, approvals pending.
- **Session**: groups Runs, holds the episodic memory scope, TTL.
- **Tool**: `type`: mcp | builtin | agent. For mcp: endpoint or an Executor-deployable package (tools can themselves be deployed as vibeD artifacts), auth requirement (none, colony credential ref, per-user delegated), schema cache, risk class (read, write, destructive, external-money), default approval requirement.
- **Policy**: rules evaluated by the Keeper: tool allow/deny by name, tag or risk class; budget caps (tokens, cost, wall clock) per Run and per Session; approval gates (which risk classes require human approval, who may approve); egress rules passed to the Executor; memory data classes; model allow/deny; max concurrency and child depth. Start with a typed YAML rule schema; keep an extension point for OPA/Rego later.
- **ModelBinding**: named concrete model endpoint with provider, credentials ref, price table, capability tags. Colony or hive scoped.
- **Executor**: registered execution backend with type, endpoint, capabilities (lanes, max duration, GPU, egress control), health.
- **Credential**: reference to a secret (Keeper-managed encrypted store first, external secret managers later). Never returned in the clear over the API.
- **Approval**: pending decision object with requester Run, tool call, context, decision, decider, timestamps.
- **Event**: append-only, per Run, typed. Not editable.
## 7. Executor interface
Executors are how hiveD stays independent from vibeD while making vibeD the best experience.
```go
type Executor interface {
    Capabilities(ctx) (Capabilities, error)      // lanes, max duration, gpu, egress control, warm start
    Provision(ctx, CellSpec) (CellHandle, error)  // artifact (drone + agent pkg + manifest), lane, resources, env, egress policy
    Status(ctx, CellHandle) (CellStatus, error)
    Signal(ctx, CellHandle, Signal) error         // stop, kill
    Logs(ctx, CellHandle) (io.ReadCloser, error)
    Destroy(ctx, CellHandle) error
}
```
Implementations, in order:
1. `local-docker`: for development and CI. Runs the Drone in a container on the developer machine.
2. `vibed`: calls vibeD's HTTP/MCP API (`deploy_artifact` and status), maps `lane` to vibeD's classifier hint (fast lane for workerd/static, general lane for Kata + Firecracker), waits for `running`, wires the Cell URL/handle back. Uses vibeD's warm pools for start latency. Contract details (artifact packaging, env injection, network policy, log access) go into ADR-0003.
3. `process`: bare subprocess, for `hived dev` without Docker.
4. Later: `kubernetes-job`, remote sandbox providers.
The Drone binary and its IPC contract are the only things every Executor must be able to run.
## 8. Drone IPC contract for custom loops
To let user code written in any language or framework run inside a Cell while the Drone keeps ownership of identity, memory, eventing and checkpoints:
- The Drone starts the user entrypoint as a child process and exposes a local endpoint (Unix socket, gRPC) with: `Model.Chat` (proxied to gateway with Run identity), `Tools.List/Call` (proxied to broker), `Memory.*` (proxied to mindD with the Run's capability token), `Checkpoint.Save/Load`, `Events.Emit`, `Control.Watch` (pause/cancel/approval decisions).
- The Python SDK wraps this so a user can write a plain function or use their framework and still get everything. Frameworks that insist on calling providers directly are pointed at the local `Model.Chat` endpoint via the standard OpenAI-compatible base URL env var.
- Direct egress from the Cell to model providers or the internet is off by default; the Executor's egress policy enforces that.
## 9. Integration contracts (write these as ADRs before coding)
- **ADR-0001 hiveD scope and principles** (this document, condensed).
- **ADR-0002 Identity and capability tokens**: PASETO v4 public tokens; claims: hive, colony, agent, agentVersion, run, session, parentRun, exp; the Keeper is the issuer for both Run identity and mindD capabilities; mapping from Agent memory declarations to mindD capability scopes; key rotation.
- **ADR-0003 vibeD executor contract**: artifact layout (`/drone`, `/app`, `/run.json`), how vibeD receives the lane hint, env and secret injection, log and status retrieval, egress policy, timeout semantics, warm pool expectations, what happens when vibeD wipes a namespace.
- **ADR-0004 mindD usage**: which mindD blocks the Drone uses for what (kv: working state and checkpoints; episodic: transcript and tool call log; semantic: retrieval scopes; artifact: files produced; lease: coordination), path scheme `colony/<c>/agent/<a>/run/<r>` and `colony/<c>/session/<s>`, and the shared-service deployment mode with optional local proxy.
- **ADR-0005 Event schema and OTel mapping**.
- **ADR-0006 Policy language v1** (typed YAML) and evaluation points.
- **ADR-0007 API style**: protobuf as source of truth, REST generated, resource versioning, watch semantics, pagination, idempotency keys.
## 10. Non-goals (v0.x)
- No orchestration DSL, no graph builder, no visual agent builder.
- No own vector database, no own sandbox technology, no own model serving. mindD, vibeD and existing inference serve those.
- No Kubernetes CRDs or operator in the core (a `hived-operator` may wrap the API later).
- No fine-tuning, no evals framework beyond storing traces and exposing them.
- No end-user chat product.
## 11. Technology decisions
- Language: Go 1.23+ for Keeper, Drone, CLI. Python 3.11+ for the first SDK.
- API: protobuf + buf, gRPC, connect-go (gives gRPC, gRPC-Web and REST/JSON in one), OpenAPI generated.
- Storage: Postgres 15+ (resources, events, credentials encrypted at rest with a hive master key). sqlc or pgx, migrations with goose/atlas.
- Auth for humans and API clients: OIDC (any IdP), API keys per colony. Authorization: colony RBAC (owner, operator, developer, approver, viewer).
- Tokens: PASETO v4.
- Observability: OpenTelemetry traces/metrics/logs, Prometheus endpoint, structured logs (slog).
- Packaging: single container image per binary, Helm chart, docker-compose for local dev (Keeper + Postgres + mindD + local-docker executor).
- Licensing: Apache 2.0. Governance and DCO like vibeD.
- Repo: `github.com/hived-project/hived` (monorepo for keeper, drone, cli, proto, sdk-python; docs site as in vibeD).
Suggested layout:
```
hived/
  cmd/keeper/  cmd/drone/  cmd/hived/
  internal/api/  internal/store/  internal/scheduler/  internal/identity/
  internal/gateway/  internal/broker/  internal/events/  internal/executors/{docker,vibed,process}/
  proto/hived/v1alpha1/
  sdk/python/
  docs/  (PROJECT.md, adr/, api/)
  deploy/{helm,compose}/
  examples/ (hello-agent, tool-using-agent, two-agents-shared-lease)
```
