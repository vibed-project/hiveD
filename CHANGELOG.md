# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
hiveD is pre-1.0; no version has been tagged yet.

## [Unreleased]

### Fixed

- Concurrent `Append` to the same Run silently dropped most events. `seq` was
  assigned with an unserialized `MAX(seq)+1`, so concurrent appenders computed
  the same value and `UNIQUE (colony, run, seq)` rejected all but one, with no
  retry. Measured before the fix: 200 concurrent appends, 78 persisted. Appends
  now take a transaction-scoped advisory lock keyed on (colony, run); appends to
  different Runs never contend.
- `List` pagination silently skipped rows. The keyset cursor was `name` alone,
  but name is unique only per `(kind, colony)`, so same-named resources in later
  colonies were never returned: three Agents named `bot` in three colonies
  paginated down to one. The cursor is now `(colony, name)` and page tokens are
  opaque.
- `resource_version` is now allocated from a transactional counter rather than a
  sequence. `nextval()` is non-transactional, so version order did not have to
  match commit order and a watcher could advance its cursor past a row that had
  not committed yet, losing it permanently. Relatedly, on a fresh database the
  `List` watermark equalled the version the first resource would receive, so the
  documented List-then-Watch handoff never delivered the first resource in a
  hive.
- `PageSize` of `MaxInt32` overflowed to a negative `LIMIT` and returned a raw
  Postgres error; no upper bound existed at all, so one request could ask the
  Keeper to load every row into memory. Page size is now clamped to
  `MaxPageSize` (1000), and a malformed page token is reported as
  `InvalidArgument` rather than `Internal`.
- `Append` discarded a caller-supplied event timestamp: `ts` was omitted from
  the INSERT column list, so every event silently took ingestion time.
- Re-applying an identical manifest advanced `resource_version` (twice), waking
  every watcher. A controller that reconciles on its own watch was a
  self-sustaining hot loop. Only a real spec/labels/annotations change now
  advances it, in both stores.
- `make test-integration` reported `ok` while running nothing: the toolchain
  container had no host network, so it could not reach a Postgres published on
  the host and every Postgres test skipped. The container now allows host
  loopback, and `HIVED_REQUIRE_PG` (set by that target and by CI) turns an
  unreachable Postgres into a failure instead of a skip.
- `MemoryStore` ignored `PageSize`/`PageToken` entirely, so no handler-level or
  conformance test could exercise pagination — which is why the cursor bug
  shipped. It now paginates identically to `PostgresStore`.

- Module path is now `github.com/vibed-project/hiveD`, matching the repository.
  It previously declared `github.com/hived-project/hived`, an org that does not
  exist, so `go get` and `go install` failed for every user by both paths.
- The published multi-arch image shipped x86-64 binaries under the `linux/arm64`
  manifest. `Dockerfile` gave BuildKit's `TARGETOS`/`TARGETARCH` defaults, which
  shadow the per-platform values BuildKit injects; the defaults are gone.
- `SECURITY.md` pointed at a 404 advisory URL and a `security@hived-project.dev`
  address on a domain that does not resolve, so there was no working way to
  report a vulnerability privately.

### Added

- Tag-triggered releases. Pushing `v*` now runs the full CI gate set, publishes
  a semver-tagged multi-arch image alongside `:latest`, and attaches
  cross-platform `hived` / `hived-keeper` binaries plus `checksums.txt` to a
  GitHub Release (`.goreleaser.yaml`, `release` job). Previously a tag triggered
  no workflow at all and produced no artifacts.
- Container images carry `org.opencontainers.image.*` labels, which links the
  GHCR package to this repository.
- `hived version`, mirroring `hived-keeper version`. The CLI compiled its
  build info in but exposed no way to read it, which matters now that the
  CLI ships as a release binary.
- `make release-check` / `make release-snapshot` to validate and dry-run the
  release locally via `scripts/goreleaser-in-podman.sh`.

### Changed

- Image `VERSION` and the `internal/version` ldflags come from the git tag on a
  tag build (they were always the commit SHA), and `BUILD_DATE` is now passed —
  it previously always reported `unknown`.
- `make build` / `make build-cli` inject version ldflags, so a local build no
  longer reports a bare `dev`.
- `make proto` uses host `buf` when present and falls back to
  `scripts/buf-in-podman.sh`, matching what CONTRIBUTING.md already described.

### Added (M1)

- `proto/hived/v1alpha1`: `Tool` resource (`ToolService` with
  `Apply/Get/List/Watch`; types MCP, BUILTIN, AGENT); `DroneService`
  (`Bootstrap/Heartbeat/Finish`) and `ToolBrokerService`
  (`ListRunTools/CallTool`) as the Drone ↔ Keeper contract; `RunService.Logs`
  (returns `CodeUnimplemented` until the Executor exists);
  `RunPhase.TIMED_OUT`; `RunSpec.cancel`; `RunStatus.attempt`,
  `observed_generation`, `last_heartbeat_at`. All additive.
- ADR-0005 (Draft): the Event `type` vocabulary and the Keeper/Drone
  emitter split.
- `hived apply/get/watch` understand the `Tool` kind.

### Added (M0)

- Repository scaffold: governance docs (LICENSE, DCO, NOTICE, CONTRIBUTING,
  SECURITY, CLAUDE.md), `docs/PROJECT.md` design brief, ADR-0001 through
  ADR-0004 (all Draft).
- `proto/hived/v1alpha1`: Colony, Agent, AgentVersion, Run, Event resource
  definitions and typed `Apply/Get/List/Watch` services, generated via
  `buf` + `connect-go` (`gen/` checked in).
- `internal/store`: Postgres-backed resource store (shared
  `resources`/`events` tables, optimistic concurrency via
  `resource_version`, `generation` bumped only on spec change, AgentVersion
  immutability enforced), plus an in-memory implementation for tests.
  Watch is a polling implementation in M0.
- `hived-keeper`: Keeper API server (`apply/get/list/watch` for all five M0
  resource kinds) with `serve`, `migrate`, and `version` subcommands.
- `hived` CLI (Cobra): `apply -f`, `get`, `watch`, `events`, and `run`
  (creates a `Run`, but it stays `PENDING` — no Scheduler exists yet)
  wired to a live Keeper; `logs` and `approve` exit non-zero with an
  explicit "not implemented until M1" message.
- `deploy/compose/docker-compose.yaml`: Keeper + Postgres by default; a
  mindD placeholder service gated behind the `mind` compose
  profile, not required for a clean `up`.
- CI (`.github/workflows/ci.yaml`): build, race, import-boundary, proto
  drift, lint, store-integration, and (main-only) container jobs.

### Deferred to M1+

Scheduler/reconciliation, the Executor interface and implementations, the
`hived-drone` binary and IPC contract, the Model Gateway, the Tool Broker,
real PASETO token issuance, real mindD memory integration, the Python SDK,
the Helm chart, and OpenAPI publication. See `CLAUDE.md` for the full
scope and out-of-scope breakdown of the current milestone.
