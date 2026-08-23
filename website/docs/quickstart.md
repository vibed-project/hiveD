---
sidebar_position: 2
title: Quickstart
---

# Quickstart

This page takes you from nothing to a running Keeper with a Colony, an Agent
and a Run. Everything here works against v0.1.0.

Read the honest ending first: the Run you create at the end will sit in
`RUN_PHASE_PENDING` and never execute, because v0.1.0 has no Scheduler and no
Executor. See [Roadmap](./roadmap.md).

## Prerequisites

- podman or docker, with compose support. The dev stack runs the Keeper and
  Postgres as containers.
- Optionally a Go toolchain, if you want to `go install` the CLI or build
  from source.

## Install the CLI

Three options.

**Release binaries.** Every [release](https://github.com/vibed-project/hiveD/releases)
attaches `hived` and `hived-keeper` binaries for linux, macOS and Windows on
amd64 and arm64, plus a `checksums.txt`.

**With a Go toolchain:**

```bash
go install github.com/vibed-project/hiveD/cmd/hived@latest
```

**From a checkout**, which builds through the pinned toolchain container:

```bash
make build-cli   # produces bin/hived
make build       # produces bin/hived-keeper
```

## The Keeper container image

The Keeper ships as a container image:

```bash
docker pull ghcr.io/vibed-project/hived-keeper:v0.1.0
```

The image is multi-arch (linux amd64 and arm64).

Pin a version tag. `:latest` is republished on every push to `main`, not only
on releases, so it does not track the newest release and can move under you.

## Start a Keeper

From a checkout of the repository:

```bash
make compose-up     # Keeper + Postgres
make compose-logs
make compose-down   # tears down, including volumes
```

`make compose-up` runs `podman compose -f deploy/compose/docker-compose.yaml
up -d --build`. If you use docker instead, the same file works directly:

```bash
docker compose -f deploy/compose/docker-compose.yaml up -d --build
```

Check that it came up:

```bash
curl localhost:8080/healthz    # {"status":"ok"}
curl localhost:8080/readyz     # {"status":"ready"} once Postgres is reachable
curl localhost:9090/metrics    # Prometheus metrics, separate port
```

The compose file also defines a mindD service behind the `mind` profile. It is
a placeholder that documents a future integration point, it builds from a
sibling checkout, and nothing in v0.1.0 uses it. You do not need it.

### Keeper configuration

The Keeper is configured entirely through environment variables.

| Variable | Default | Meaning |
|---|---|---|
| `HIVED_PG_DSN` | `postgres://hived:hived@localhost:5432/hived?sslmode=disable` | Postgres connection string |
| `HIVED_LISTEN_ADDR` | `:8080` | API listener (connect-go: gRPC, gRPC-Web, HTTP/JSON) |
| `HIVED_METRICS_ADDR` | `:9090` | Prometheus listener, deliberately a separate port |
| `HIVED_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `HIVED_LOG_FORMAT` | `json` | `json` or `text` |
| `HIVED_AUTO_MIGRATE` | `true` | Run schema migrations on startup |

`hived-keeper` has three subcommands: `serve`, `migrate <up|down|status>` and
`version`.

## Apply a Colony

Manifests use a Kubernetes-style envelope: `apiVersion`, `kind`, `metadata`,
`spec`. The only accepted `apiVersion` is `hived/v1alpha1`; anything else is
rejected rather than silently reinterpreted. Unknown fields are also rejected,
so a typo fails the apply instead of producing an empty spec.

```yaml
# colony.yaml
apiVersion: hived/v1alpha1
kind: Colony
metadata:
  name: acme
spec:
  displayName: Acme Corp
  quotas:
    maxConcurrentRuns: 4
    tokenBudget: 1000000
    costBudget: "25.00"
  memoryRoot: colony/acme
```

```bash
hived apply -f colony.yaml
hived get colonies
```

Colony is the only hive-scoped kind: it has no `metadata.colony` of its own.
Quotas and policies are stored but not enforced in v0.1.0.

## Apply an Agent and an AgentVersion

`Agent` is the mutable envelope. The real definition lives on `AgentVersion`,
whose spec is immutable after creation. Both carry `metadata.colony`, which is
required.

```yaml
# agent.yaml
apiVersion: hived/v1alpha1
kind: Agent
metadata:
  name: greeter
  colony: acme
spec:
  description: says hello
---
apiVersion: hived/v1alpha1
kind: AgentVersion
metadata:
  name: greeter-v1
  colony: acme
spec:
  agent: greeter
  version: v1
  instructions: "Say hello politely, then stop."
  model:
    provider: openai
    name: gpt-4o-mini
  limits:
    maxSteps: 8
    maxTokens: 20000
    timeout: 300s
```

```bash
hived apply -f agent.yaml
```

`apply -f` accepts a file, a directory (which applies `*.yaml`, `*.yml` and
`*.json` in filename order), or `-` for stdin. Multi-document YAML works, and
so do `---` separators with trailing comments and the `...` end-of-document
marker.

Applying a *different* AgentVersion spec to the same name is rejected as
immutable. Re-applying an identical one is a no-op success and does not even
advance `resourceVersion`.

## List and inspect

```bash
hived get colonies
hived --colony acme get agents
hived --colony acme get agentversions
hived --colony acme get agent greeter
hived --colony acme get agents -o json
hived --colony acme get agentversions -o yaml
```

`get <kind>` with no name lists; with a name it fetches one object. Lists in
`table` format print a per-kind column set; a single object always prints as
YAML unless you ask for `-o json`.

Aliases work as you would expect: `co`, `colonies`, `ag`, `agents`, `av`,
`agentversions`, `runs`, `tools`. The full list is in the
[CLI reference](./cli.md#kind-aliases).

Note that when `--colony` is empty, `List` is not colony-scoped: it returns
every Colony's resources. This is a known v0.1.0 limitation.

## Watch

`watch` streams `ADDED`, `MODIFIED`, `DELETED` and `BOOKMARK` events for a
kind, printing type, resource version and name as tab-separated lines. Press
Ctrl-C to stop.

```bash
hived --colony acme watch agents
```

To pick up exactly where a `List` left off, pass the list's
`listMeta.resourceVersion`:

```bash
hived --colony acme watch runs --since-resource-version 42
```

`hived watch` ignores `--output` and does not reconnect if the Keeper
restarts.

## Create a Run

```bash
hived --colony acme run greeter --name run-1 --input '{"question":"hi"}'
hived --colony acme get runs
```

The CLI prints a warning to stderr, and the Run's phase is
`RUN_PHASE_PENDING`:

```
COLONY  NAME   AGENT    PHASE
acme    run-1  greeter  RUN_PHASE_PENDING
```

It stays there. `RunService.Apply` always persists a Run as `PENDING` with
`attempt` 0, and the Scheduler that owns every later transition does not
exist yet.

`--name` and `--colony` are both required. `--input` must be a JSON object.

## Events

```bash
hived --colony acme events run-1
```

The event log is append-only and per Run. In v0.1.0 nothing emits into it: the
two emitters, the Scheduler and the Drone, are not built. The API accepts
`EventService.Append` from a client, but the CLI has no command to write one,
so this list is empty unless you appended events yourself. The vocabulary the
emitters will use is fixed in ADR-0005.

## What deliberately fails

```bash
hived logs run-1
hived approve run-1
```

Both exit non-zero with a "not implemented until M1" message. `logs` needs an
Executor to read a Cell's output; `approve` needs the Policy engine, an
Approval resource and the Tool Broker. None of those exist. The Keeper's
`RunService.Logs` RPC likewise returns `CodeUnimplemented`.

## Pointing the CLI elsewhere

```bash
hived --server https://keeper.example.internal --token "$HIVED_TOKEN" get colonies
```

`--token` is sent as a bearer token on every request, including streams. A
v0.1.0 Keeper's stub verifier accepts anything, including no token at all.

## Next

- [Resource reference](./resources.md) for every spec field you can put in a
  manifest.
- [CLI reference](./cli.md) for every command and flag.
- [Architecture](./architecture.md) for what the Keeper is doing underneath.
