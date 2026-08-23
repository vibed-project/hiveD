# hiveD

**Status: pre-alpha (M0).** hiveD is a standalone, open-source control
plane that defines, schedules, governs, and observes AI agents — running
them inside isolated sandboxes (vibeD by default) and giving them durable
memory (mindD by default).

> vibeD builds the cells, mindD keeps the memory, hiveD runs the colony.

See [docs/PROJECT.md](docs/PROJECT.md) for the full design brief and
[docs/adr/](docs/adr/) for the architecture decisions behind it.

## Vocabulary

| Term | Meaning |
|---|---|
| Hive | A hiveD installation (control plane instance). Multi-tenant. |
| Colony | Tenant / isolation boundary. Owns agents, policies, quotas, tool registrations, memory scope root. |
| Agent | A versioned, immutable definition of an agent: instructions, model requirements, tools, memory scopes, policies, resource hints. |
| Worker | One running instance of an Agent, i.e. a Run. "Worker" in prose, `Run` in the API. |
| Cell | The isolated sandbox a Worker runs in. Provisioned by an Executor. |
| Session | Optional grouping of Runs that share conversational context and an episodic memory scope. |
| Tool | A registered capability a Worker may call: an MCP server, a built-in, or a child-agent spawn. |
| Policy | Rules attached to a Colony or Agent: allowlists, budgets, egress, approval gates, data classes. |
| Keeper | The hiveD control plane process itself (API server + controllers). Binary `hived-keeper`. |
| Drone | The runtime binary injected into every Cell that executes the agent loop and talks to the Keeper. Binary `hived-drone`. Not built yet — see M1. |

## What's in M0

This milestone ships the foundations: repo scaffold, ADRs, the resource
proto (Colony/Agent/AgentVersion/Run/Event), a Postgres-backed resource
store, the Keeper's `apply/get/list/watch` API, a `hived` CLI skeleton, and
a docker-compose dev loop. **There is no Scheduler yet** — a `Run` you
apply stays `PENDING` forever; nothing executes it. See
[CLAUDE.md](CLAUDE.md) for the full scope/out-of-scope list and
[docs/PROJECT.md](docs/PROJECT.md) for the design brief behind it.

## Architecture

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

The Keeper never executes agent logic. The Drone never makes policy
decisions. Only the Keeper (control plane) and its Postgres-backed
resource API exist in M0 — the Cell, Drone, Executors, and Model Gateway
are M1+.

## Repository layout

```
cmd/keeper/        hived-keeper binary (control plane API server)
cmd/hived/          hived CLI
internal/store/     Postgres-backed resource store
internal/api/       Keeper API handlers (connect-go)
internal/identity/  token issuance (stub in M0)
proto/hived/v1alpha1/  resource + service definitions (source of truth)
gen/                generated Go code from proto/ — do not hand-edit
deploy/compose/     local dev docker-compose stack
docs/adr/           architecture decision records
```

## Install

Pre-built binaries for linux, macOS and Windows (amd64 and arm64) are attached
to each [release](https://github.com/vibed-project/hiveD/releases), with
`checksums.txt`. With a Go toolchain:

```bash
go install github.com/vibed-project/hiveD/cmd/hived@latest
```

The Keeper also ships as a container image:

```bash
docker pull ghcr.io/vibed-project/hived-keeper:v0.1.0
```

## Quickstart

```bash
make compose-up      # Keeper + Postgres
make build-cli       # bin/hived
```

Manifests follow a Kubernetes-style envelope (`apiVersion`/`kind`/`metadata`/`spec`):

```bash
cat > colony.yaml <<'EOF'
apiVersion: hived/v1alpha1
kind: Colony
metadata:
  name: acme
spec:
  displayName: Acme Corp
EOF

./bin/hived apply -f colony.yaml
./bin/hived get colonies
```

```bash
cat > agent.yaml <<'EOF'
apiVersion: hived/v1alpha1
kind: Agent
metadata:
  name: greeter
  colony: acme
spec:
  description: says hello
EOF

cat > agentversion.yaml <<'EOF'
apiVersion: hived/v1alpha1
kind: AgentVersion
metadata:
  name: greeter-v1
  colony: acme
spec:
  agent: greeter
  version: v1
  instructions: "Say hello politely."
EOF

./bin/hived apply -f agent.yaml
./bin/hived apply -f agentversion.yaml
./bin/hived --colony acme get agents
./bin/hived --colony acme get agentversions
```

```bash
./bin/hived --colony acme run greeter --name run-1 --input '{"question":"hi"}'
./bin/hived --colony acme get runs
# PHASE is RUN_PHASE_PENDING and stays that way — there is no Scheduler
# until M1, so nothing ever picks the Run up.
```

`./bin/hived logs run-1` and `./bin/hived approve run-1` exit non-zero with
a "not implemented until M1" message — that's expected in M0.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). All commits require DCO sign-off
(`git commit -s`).

## License

Apache 2.0 — see [LICENSE](LICENSE) and [NOTICE](NOTICE).
