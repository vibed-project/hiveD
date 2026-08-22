# ADR-0003: vibeD executor contract

## Status
Draft — contract only. No code ships against this ADR in M0; the vibeD
Executor implementation is targeted for M2. This ADR exists now so the M0
proto (`Run.spec.executorHint`) doesn't need a breaking change later.

## Context

hiveD stays independent of vibeD through the `Executor` interface (see
`docs/PROJECT.md` §7), but vibeD is the default, best-supported Executor —
the one that provisions a Cell as a Kata + Firecracker microVM (general
lane) or a fast-start sandbox (fast lane). Pinning down what the vibeD
Executor actually hands to a Cell, and what it expects back, needs to
happen before M2's implementation so the Drone (M1) and the vibeD
Executor (M2) are built against the same assumptions from the start.

## Decision

- **Artifact layout delivered into the Cell:**
  - `/drone` — the `hived-drone` binary.
  - `/app` — the agent bundle: rendered instructions, tool manifests, and
    (for `runtime.loop: custom`) the user's entrypoint code/package.
  - `/run.json` — the frozen Run spec plus the resolved AgentVersion
    snapshot plus a *reference* to the identity token (never the raw
    signing material — see below).
- **Lane hint, not a lane guarantee.** `Run.spec.executorHint` (and the
  AgentVersion's `runtime.lane` hint) map to vibeD's classifier hint
  (`fast` → workerd/static lane, `general` → Kata + Firecracker), but the
  Keeper may override the hint based on policy, and the Executor may
  reject a hint it cannot satisfy. Callers must not assume the hint is
  binding.
- **Env / secret injection.** Secrets are injected **by reference**
  (a `Credential` name the Executor resolves at Cell start against its
  own secret store), never written into `/run.json` in the clear.
  Environment variable naming: `HIVED_RUN_ID`, `HIVED_KEEPER_ADDR`,
  `HIVED_IDENTITY_TOKEN_REF`, `HIVED_MODEL_GATEWAY_ADDR`,
  `HIVED_TOOL_BROKER_ADDR`, `HIVED_MIND_ADDR` — the Drone reads these to
  bootstrap; user code never sees them directly (it talks to the Drone's
  local IPC endpoint instead, per `docs/PROJECT.md` §8).
- **Log and status retrieval.** The Executor exposes `Logs` (streaming)
  and `Status` (poll) per the `Executor` interface in §7. The Keeper polls
  `Status` for terminal-state detection; it does not require the
  Executor to push status changes in M2's first cut, though push (e.g.
  webhook) is not precluded later.
- **Egress policy.** The Cell is **deny-by-default**. The allowlist the
  Executor enforces is derived from the effective Colony + AgentVersion
  `spec.tools` policy, resolved by the Keeper and passed to `Provision`.
  Enforcement is entirely the Executor's/vibeD's responsibility — **never
  the Drone's** (restates the Keeper/Drone split from ADR-0001; the Drone
  has no egress-enforcement code path, by construction).
- **Timeout semantics.** Three distinct timers: Run-level wall clock
  (`AgentVersion.spec.limits`), per-step timeout (Drone-local), and Cell
  idle timeout (Executor-local, protects against a wedged Drone). Any of
  the three firing kills the Cell and marks `Run.status.phase = Failed`
  or `TimedOut` with a message identifying which timer fired.
- **Warm pool expectations.** The Executor may reuse a warm Cell shell
  (network setup, base filesystem) across Runs for latency, but the
  identity token, `/run.json`, and `/app` must always be freshly injected
  per Run — nothing Run-specific may be warm-pooled.

## Consequences

- `Run.spec.executor_hint` must exist as a proto field in M0 even though
  no Executor consumes it yet, so M2 doesn't require a `Run` schema
  break.
- Because egress enforcement lives entirely in the Executor, a
  `local-docker` or `process` Executor (M1) that doesn't implement egress
  control is explicitly **not** a security boundary — this must be
  documented wherever those Executors are used for anything beyond local
  dev.
- The env-var contract above becomes the first thing M1's Drone
  bootstrap code and M2's vibeD Executor `Provision` implementation both
  have to agree on; changing it later is a coordinated cross-repo change.

## Alternatives considered

- **Push-based status (Executor calls back into the Keeper on state
  change) instead of poll.** Deferred, not rejected: push is lower
  latency but adds a callback endpoint the Keeper must expose and secure
  before any Executor is even implemented. Polling is simpler to get
  right first; push can be added as an optimization without changing the
  `Executor` interface's `Status` semantics.
- **Bundling secrets directly into `/run.json`.** Rejected: a `run.json`
  is exactly the kind of artifact likely to end up in logs, warm-pool
  caches, or debugging output; by-reference injection keeps raw secret
  material out of anything that isn't the Executor's own secret store.
