# ADR-0004: mindD memory usage

## Status
Draft. Decision (a) below applies starting M1 regardless of whether the
proposal in Decision (b) is accepted. No mindD Go client code and no real
mindD integration ships in M0 — `deploy/compose` includes mindD only as a
profile-gated placeholder service, not a live integration.

## Context

`docs/PROJECT.md` §9 assumes hiveD can scope a Run's mindD capability
token to a hierarchical path like `colony/<c>/agent/<a>/run/<r>` and
`colony/<c>/session/<s>`. Checking that assumption against mindD
(MemorySidecar) as it actually exists today surfaced a real gap:

- mindD's PASETO v4 capability scope (`internal/auth`, `IssuePASETO`) is
  `{tenant string (required), agent string (optional), namespaces
  []string (glob patterns), ops []string}`. There is **no client-driven,
  hierarchical path concept** — a token scopes to a flat `tenant` string
  plus glob-matched namespace patterns (e.g. `kv/scratchpad`,
  `kv/tool-*`), not to an arbitrary nested path a caller supplies.
- Namespaces themselves are **admin-predeclared** in mindD's server
  config (`NamespaceConfig{Block, Name, Backend, ...}`), not created
  dynamically by a client. mindD has no native concept of a "run" at all.
- mindD exposes **no importable Go SDK package**. The only Go client code
  in that repo is `cmd/memctl/dataplane.go`, which lives inside mindD's
  own module and imports its generated protobuf/connect stubs directly
  (`memsidecar/gen/memsidecar/{kv,episodic,semantic,artifact,lease}/v1`).
  A hiveD Go client would need to either import those generated packages
  directly (which the import-boundary check in `scripts/check-import-boundary.sh`
  currently forbids by design — see ADR-0001) or hand-write its own thin
  gRPC/connect client against mindD's public proto.
- FYI, not a discrepancy to resolve: mindD has a sixth block, `graph`,
  that `docs/PROJECT.md` §9 doesn't mention. hiveD ignores `graph` for
  now; it's available but unused, not a gap.

## Decision

### (a) Interim mapping hiveD adopts, independent of (b)

Until (if ever) mindD grows hierarchical/dynamic namespace scoping, hiveD
maps its model onto mindD's existing flat one as follows:

- **Colony → mindD's `tenant` claim.** `tenant = "<colony>"`. This is the
  isolation boundary mindD actually enforces today (`QualifyNamespace`/
  `StorageTenant` prefix storage by `tenant`), so it is the right anchor
  for hiveD's coarsest isolation unit too.
- **Agent → mindD's optional `agent` claim**, when per-agent isolation
  within a colony is needed, rather than folding it into `tenant` as a
  compound string. This uses the field mindD already defines for this
  purpose instead of inventing a naming convention on top of `tenant`.
- **Block-level scoping → mindD's namespace glob patterns**, narrowed to
  whichever predeclared namespaces the deployment has configured for that
  colony (`kv/*`, `episodic/*`, `semantic/*`, `artifact/*`, `lease/*`).
- **Run → an application-level key/session-prefix convention, not a
  mindD auth scope.** hiveD prefixes keys and episodic session IDs with
  `run/<runID>/...` inside the namespaces a Run's token already grants
  access to. This is a **namespacing convention for organizing data, not
  an isolation boundary.**

  **Security consequence to record honestly:** under this interim
  mapping, a compromised or leaked Run identity/capability token can read
  and write sibling Runs' memory within the same colony (and agent, if
  the `agent` claim is used), because mindD has no way to scope a token
  to a single run's keys. This is a real, accepted gap until (b) lands or
  an equivalent mechanism is built. It must stay visible — do not let a
  later implementation quietly treat the `run/<runID>/` prefix as if it
  were a security boundary.

- **Block-to-purpose mapping** (per `docs/PROJECT.md` §9, unchanged by
  this gap): `kv` → Drone working state and checkpoints; `episodic` →
  Run step/event transcript; `semantic` → retrievable agent knowledge;
  `artifact` → large Run outputs/blobs; `lease` → single-writer
  coordination between sibling Runs sharing a scope.

### (b) Cross-repository feature request

hiveD will file a feature proposal against mindD requesting
client-driven, hierarchical/dynamic namespace-pattern support — the
capability that would let a token be scoped to something like a specific
run's keys rather than only to predeclared, admin-configured namespace
globs.

**The proposal must be worded generically** — e.g. "support for
hierarchical capability scoping for multi-tenant, multi-agent-orchestration
use cases" — **and must not name hiveD** if mindD's issue tracker is
public. This mirrors the discipline already established elsewhere in this
workspace, where the public vibeD repository never references its private
sibling projects by name; the same boundary applies here in reverse (a
private/internal-facing project referencing mindD in a public-facing
mindD artifact).

Filing the issue is a **manual action, not a code artifact** — it is not
part of any M0 deliverable. Once filed, record the issue link here and
update this ADR's Status when mindD's maintainers respond.

## Consequences

- No mindD Go client code and no real mindD integration ships in M0. The
  M0 `deploy/compose` includes a mindD service definition gated behind
  the `mind` compose profile, documented but not started by default (see
  the docker-compose plan) — it exists as a placeholder for M1, not a
  working integration.
- M1's mindD client will be hand-written against mindD's public
  connect-go stubs (imported as a separate dependency, not via
  `memsidecar/gen/...` inside mindD's own module tree) or generated from
  mindD's published proto, whichever is cleaner once M1 scopes that work
  — this ADR does not decide that mechanism, only the claim/path mapping.
- The `run/<runID>/` key-prefix convention is designed so the **key
  layout survives unchanged** if/when mindD adds real hierarchical
  scoping — only the auth boundary tightens, from "colony+agent" to
  "colony+agent+run". No data migration should be needed at that point.
- If mindD's maintainers accept the proposal and hierarchical scoping
  ships, this ADR is superseded by a new ADR documenting the tightened
  mapping; this document's Status line is updated to point at it.

## Alternatives considered

- **Block on mindD shipping hierarchical scoping before starting M1's
  memory integration.** Rejected: this would make hiveD's M1 milestone
  dependent on a cross-repo feature with no committed timeline. The
  interim mapping in (a) lets M1 proceed now and tightens later without a
  data-layout change.
- **Encode `run` into the `tenant` claim instead of a key prefix** (e.g.
  `tenant = "<colony>.<agent>.<run>"`). Rejected: this would create a new
  predeclared-namespace-and-tenant combination per Run, which doesn't fit
  mindD's admin-predeclared namespace model at all — namespaces aren't
  meant to be created dynamically per Run, and tenant churn at Run
  granularity would be operationally worse than the accepted interim
  isolation gap.
- **Give up on hierarchical scoping and treat colony-level isolation as
  sufficient long-term.** Rejected: run-level isolation is directly
  relevant to hiveD's threat model (a compromised agent's tool-calling
  code should not be able to read another Run's memory by default), so
  this is worth pursuing via (b) rather than accepting permanently.
