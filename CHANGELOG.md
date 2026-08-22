# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
hiveD is pre-1.0; no version has been tagged yet.

## [Unreleased]

### Added

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
  mindD (MemorySidecar) placeholder service gated behind the `mind` compose
  profile, not required for a clean `up`.
- CI (`.github/workflows/ci.yaml`): build, race, import-boundary, proto
  drift, lint, store-integration, and (main-only) container jobs.

### Deferred to M1+

Scheduler/reconciliation, the Executor interface and implementations, the
`hived-drone` binary and IPC contract, the Model Gateway, the Tool Broker,
real PASETO token issuance, real mindD memory integration, the Python SDK,
the Helm chart, and OpenAPI publication. See `CLAUDE.md` for the full
scope and out-of-scope breakdown of the current milestone.
