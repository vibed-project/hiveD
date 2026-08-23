# Security Policy

hiveD is a control plane for running AI agents with an explicit identity,
policy, and audit model, see [docs/PROJECT.md](docs/PROJECT.md) sections 1 to 3.
This document covers how to report a vulnerability and the security model
the project is being built around. **hiveD is pre-alpha**: v0.1.0 is tagged
and M1 is in progress, but most of the security-relevant machinery described
below (real token issuance, policy enforcement, executor egress control) is
not implemented yet. See [docs/adr/](docs/adr/) for what is drafted versus
built, and treat this document as a statement of intent as much as current
behavior.

## Reporting a Vulnerability

**Please do not open a public GitHub issue for security vulnerabilities.**

Report suspected vulnerabilities privately to the maintainers via
[GitHub's private security advisory](https://github.com/vibed-project/hiveD/security/advisories/new)
("Report a vulnerability"). That is the only private channel we monitor.

If you cannot use GitHub Security Advisories, open a public issue that says
only that you have a security report and asks for a private contact — no
details, no reproduction steps — and a maintainer will follow up privately.

Please include, where you can:

- A description of the issue and the impact you believe it has
- The affected component(s) — e.g. `hived-keeper`, `hived-drone`, `hived` CLI
- Steps to reproduce, or a proof of concept
- The version / commit you tested against and relevant configuration

We aim to acknowledge a report within a few business days and will keep you
updated as we investigate. Please give us a reasonable window to release a
fix before any public disclosure. We are happy to credit reporters in the
advisory unless you prefer to remain anonymous.

## Supported Versions

hiveD is experimental and pre-1.0. Security fixes land on `main` and ship in
the next tagged release; there are no backports to earlier tags.

| Version | Supported |
|---------|-----------|
| `main` | ✅ |
| `v0.1.x` (latest tag) | ✅ |
| Older tags | ❌ |

### Known gap in v0.1.0

`List` is not tenant-scoped when `options.colony` is empty: it returns every
Colony's resources. Authentication is a deliberate no-op stub in this
milestone, so there is no privilege boundary being bypassed today, but
`Principal.Colony` is not consulted anywhere and enabling real auth will not
close this on its own. Treat a v0.1.0 Keeper as single-tenant and do not
expose it to untrusted callers. Tracked for the milestone that turns on
identity; reports of *other* isolation gaps are still very welcome.

## Security Model

### Identity and capability tokens

hiveD's design (ADR-0002) is built around PASETO v4.public tokens, issued
solely by the Keeper, scoped to a `hive`/`colony`/`agent`/`agentVersion`/
`run`/`session`/`parentRun` claim set. **In M0 this is drafted but not
issued** — `internal/identity` ships as a no-op stub; every request is
accepted with a fixed dev principal. Do not expose an M0 Keeper on an
untrusted network.

### Keeper / Drone split

The Keeper (control plane) never executes agent logic; the Drone (per-Cell
runtime) never makes policy decisions — this invariant is structural,
enforced by which binary the code lives in, not by a runtime check. See
ADR-0001. **The Drone does not exist yet in M0.**

### Executor egress and isolation

Cell isolation and egress control are the responsibility of the pluggable
Executor (vibeD, local-docker, ...), never the Drone — see ADR-0003 for the
vibeD executor contract (drafted, not yet implemented). No Executor ships in
M0.

### Memory scoping

hiveD delegates durable memory to mindD. ADR-0004 records a
known gap: mindD's capability tokens today scope by a flat tenant string
plus admin-predeclared namespace globs, not by hiveD's `run`-level path
scheme — until that is resolved, a compromised Run token can reach sibling
Runs' memory within the same colony. No mindD integration ships in M0.

### Cross-repository boundary

This module's `scripts/check-import-boundary.sh` (run in CI) asserts it
never imports `github.com/vibed-project/*` directly — which now covers mindD
(`github.com/vibed-project/mindD`) — so its
security posture does not implicitly depend on a sibling repository's
internals.

## Scope and Assumptions

- hiveD does not sandbox anything itself; sandboxing is the Executor's job.
  Treat hiveD's own configuration, secrets, and database credentials as
  trusted inputs.
- Reports about missing hardening in explicitly-stubbed M0 paths (no-op
  identity, unauthenticated dev Keeper) are useful for tracking, but note
  these are intended to be replaced before any production exposure.

Thank you for helping keep hiveD and its users safe.
