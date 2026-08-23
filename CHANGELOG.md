# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
hiveD is pre-1.0: the API surface under `proto/hived/v1alpha1` may still change
between minor versions.

## [Unreleased]

## [0.1.0] - 2026-08-23

First tagged release. hiveD is a control plane that defines, schedules,
governs and observes AI agents; at this tag the Keeper's resource API, the
`hived` CLI and the Postgres-backed store are usable, and nothing executes an
agent yet (see Known limitations).

### Known limitations

- **`List` is not tenant-scoped when `options.colony` is empty.** It returns
  every Colony's resources. Authentication is a deliberate no-op stub in this
  milestone (`internal/identity`), so there is no privilege boundary to cross
  today, but `Principal.Colony` is not read anywhere: enabling real auth will
  not close this by itself. Do not expose a v0.1.0 Keeper to untrusted
  callers. See `SECURITY.md`.
- No Scheduler or Executor exists, so a `Run` is created and stays `PENDING`.
  `hived logs` and `hived approve` exit non-zero by design.
- Writes do not check that the referenced Colony or Agent exists, so a typo
  creates an orphaned resource.
- `Watch` ignores `label_selector` (`List` honours it), holds a pool
  connection while delivering, and swallows query errors while continuing to
  emit bookmarks — a watcher cannot distinguish "idle" from "database down".
- `hived watch` ignores `--output` and does not reconnect after a Keeper
  restart.
- All five ADRs are Draft; the contracts they describe may still change.

### Fixed

- `hived apply -f` silently discarded manifests. Documents were split with a
  `^---\s*$` regexp, so the ubiquitous `--- # comment` separator did not match
  and everything after the first document was dropped while apply still exited
  0. A three-Colony manifest created one Colony. Splitting now uses a real YAML
  decoder, which also handles the `...` end-of-document marker and skips empty
  or comment-only documents instead of failing the whole apply.
- `hived get -o yaml` on a list emitted documents with no `---` separator, so a
  YAML parser saw duplicate top-level keys and kept only the last item.
  `-o json` emitted a bare sequence of objects, which is not valid JSON
  (`Extra data`). Both now emit every item: a JSON array, and `---`-separated
  YAML documents.
- Every CLI command hung indefinitely against a server that accepted the
  connection and never responded: all clients used `http.DefaultClient`
  (no timeout) and commands ran on `context.Background()`. There is now a
  `--timeout` flag (default 30s, 0 disables) applied as a context deadline to
  unary commands and as dial/response-header bounds on the transport, so
  `watch` streams stay open but are still protected from a black-hole server.
- `--token` was never sent on streaming RPCs. `connect.UnaryInterceptorFunc`
  has a no-op `WrapStreamingClient`, so `hived watch` sent no `Authorization`
  header at all and would have failed auth on day one of M1.
- Ctrl-C now cancels in-flight requests and streams: the CLI ran on a
  background context with no signal handling, so `ExecuteContext` had nothing
  to cancel.
- Unknown manifest fields are rejected. A typo such as `dispalyName` was
  silently discarded and apply created a resource with an empty spec, exiting
  0.
- `apiVersion` is validated. It was parsed and then never read anywhere, so a
  future `v1beta1` manifest would have been silently decoded as `v1alpha1`.
- apply errors no longer embed the whole manifest body, which put agent
  instructions and tool config into CI logs; they name the kind, colony/name
  and document position instead. A partial apply now reports how many
  documents landed before the failure and how many were never attempted.
- Directory applies are ordered by filename. `filepath.Glob` ran once per
  extension, so every `.json` manifest applied after every `.yaml` one.
- An invalid `--output` is rejected up front. The check lived in a code path
  an empty list never reached, so `--output bogus` printed nothing and exited
  0.
- Errors printed twice (Cobra and `main` both wrote them).
- `ColonyService.Apply` double-wrapped an already-typed connect error, so
  validation failures surfaced as `invalid_argument: invalid_argument: ...`
  and a marshalling failure was misreported as the caller's fault.

### Added

- Tests for the `hived` CLI, which previously had none: multi-document
  parsing across every separator form, apiVersion/kind validation, error
  redaction, directory ordering, and the JSON/YAML list encodings. The `race`
  job and `make test-race` now cover `./...` rather than `./internal/...`.

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

[Unreleased]: https://github.com/vibed-project/hiveD/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/vibed-project/hiveD/releases/tag/v0.1.0
