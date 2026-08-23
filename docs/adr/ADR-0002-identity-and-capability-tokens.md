# ADR-0002: Identity and capability tokens

## Status
Draft. Wire format frozen here for M1's issuer and mindD client to agree
on; no issuance ships in M0 (`internal/identity` is a no-op stub).

## Context

Every Run needs two related but distinct things once the Scheduler exists
(M1): an **identity** the Drone can present to the Keeper and to the Model
Gateway / Tool Broker, and a **capability** it can present to mindD to
scope what memory it may read or write. Both need to be short-lived,
verifiable without a round trip to the issuer wherever possible, and safe
to hand to code running inside an untrusted Cell. This decision needs to
be made before the proto is finalized, because the claim names show up in
`Run.status.identity` and constrain what the Scheduler and mindD client
build in M1.

## Decision

- **Token format: PASETO v4.public** for both Run identity tokens and
  mindD capability tokens. `v4.public` (asymmetric, Ed25519) rather than
  `v4.local` (symmetric) so that every verifier (the Drone, mindD, the
  Tool Broker) only needs the Keeper's **public** key, never a shared
  secret. This is also format-compatible with mindD's existing token
  story: mindD already verifies PASETO v4 tokens against a configured
  public key (see ADR-0004), so no new verification machinery is needed
  on that side.
- **The Keeper is the sole issuer** of both token kinds. Neither the
  Drone nor any Executor mints tokens.
- **Claim set** (beyond the standard `iss`, `iat`, `jti`, `exp`):
  `hive`, `colony`, `agent`, `agentVersion`, `run`, `session`,
  `parentRun`. `iss` is always `"hived-keeper"`. `exp` is mandatory and
  short (target: single-digit minutes to low hours depending on Run
  wall-clock limits, tuned in M1; not decided here).
- **Key rotation**: each signing key is identified by a `kid` carried in
  the PASETO footer. The Keeper publishes the current key set (a
  JWKS-equivalent list of active public keys by `kid`) so verifiers can
  fetch it without a Keeper code change. Rotation keeps the old key
  active for at least the maximum token TTL after a new key becomes
  primary, so no in-flight token is invalidated mid-Run. Rotation is a
  Keeper-side operation; verifiers never need to restart to pick up a new
  key, only to refresh their cached key set.
- **M0 boundary**: `internal/identity/` ships only an interface and a
  `NotImplemented` stub. No signing key is generated, no token is
  issued, and the API's stub auth interceptor (see `internal/api`)
  accepts any bearer value in M0. This ADR fixes the shape so that when
  M1 implements real issuance, nothing downstream (Drone, mindD client)
  needs to change its expectations.

## Consequences

- The M0 proto's `Run.status.identity` field exists and is typed as an
  opaque string (the eventual token), even though nothing populates it
  meaningfully yet. This avoids a breaking proto change when M1 wires up
  real issuance.
- Because verification only needs a public key, the Tool Broker and
  future mindD client can verify tokens locally without calling back into
  the Keeper on every request, which is important for latency once the
  general lane is in play.
- An M0 Keeper is **not safe to expose on an untrusted network**: the
  stub auth interceptor accepts anything. This is called out in
  `SECURITY.md` and must not be missed when M1 lands real issuance.

## Alternatives considered

- **JWT.** Rejected: JWT's algorithm-confusion class of vulnerabilities
  (e.g. `alg: none`, RS256/HS256 confusion) is a well-known footgun that
  PASETO's fixed-purpose versions avoid by construction. mindD also
  already speaks PASETO, so JWT would mean supporting two token formats
  for no benefit.
- **Macaroons.** Rejected for now: attractive for their delegation model
  (attenuation without contacting the issuer), but there is no mature,
  well-maintained Go and Python ecosystem support at the level PASETO and
  JWT have, and hiveD doesn't yet have a concrete use case that needs
  macaroon-style attenuation over simple short-lived, narrowly-scoped
  tokens.
- **v4.local (symmetric) PASETO.** Rejected: would require distributing a
  shared secret to every verifier (Drone, mindD, Tool Broker, any future
  Executor), which is a materially worse secret-management story than
  publishing a public key.
