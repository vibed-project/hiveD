---
sidebar_position: 5
title: CLI reference
---

# `hived` CLI reference

`hived` drives a hiveD Keeper: apply resources, inspect Runs, watch events.
This page documents every command and flag that exists in v0.1.0. Commands
described in the design brief but not implemented are listed at the bottom.

```
hived [command] [flags]
```

## Persistent flags

These are accepted by every subcommand.

| Flag | Type | Default | Meaning |
|---|---|---|---|
| `--server` | string | `http://localhost:8080` | hiveD Keeper base URL. |
| `--colony` | string | `""` | Colony to scope colony-scoped commands to. |
| `--token` | string | `""` | Bearer token, sent as `Authorization: Bearer <token>` on unary **and** streaming requests. A v0.1.0 Keeper's stub verifier ignores it. |
| `-o`, `--output` | string | `table` | Output format: `table`, `json` or `yaml`. Validated before the command runs, so a bogus value fails rather than printing nothing. |
| `--timeout` | duration | `30s` | Per-request timeout. `0` disables it. |

### How `--timeout` applies

- Unary commands get it as a context deadline, which bounds the whole request.
- `apply` applies it per document, so a 100-document manifest does not fail
  because the whole apply took longer than one request's timeout.
- Streaming commands (`watch`) are not bounded by it. It still applies to dial
  and response headers on the transport, so a black-hole server cannot hang
  the CLI, but an established stream stays open.

Ctrl-C cancels in-flight requests and streams.

## Commands

### `hived apply`

```
hived apply -f FILE|DIR|-
```

Create or update resources from a manifest.

| Flag | Type | Default | Meaning |
|---|---|---|---|
| `-f`, `--filename` | string | `""` | Manifest file, directory, or `-` for stdin. Required. |

Behaviour:

- Manifests use the envelope `apiVersion` / `kind` / `metadata` / `spec`. The
  only accepted `apiVersion` is `hived/v1alpha1`.
- Accepted kinds: `Colony`, `Agent`, `AgentVersion`, `Run`, `Tool`.
- Multi-document YAML is supported, including `--- # comment` separators and
  the `...` end-of-document marker. Empty and comment-only documents are
  skipped.
- A directory applies `*.yaml`, `*.yml` and `*.json`, sorted by filename.
- Unknown fields are an error. A typo such as `dispalyName` fails the apply
  instead of creating a resource with an empty spec.
- Apply is not atomic. On failure partway through, the CLI reports how many
  documents landed and how many were never attempted.
- Error messages name the kind, colony, name and document position. They never
  include the document body, which carries instructions and tool config and
  would otherwise end up in CI logs.
- The applied object is printed per `--output`.

The Colony a resource belongs to comes from `metadata.colony` in the manifest,
not from `--colony`.

### `hived get`

```
hived get <kind> [name]
```

With a name, fetch one object. Without, list every object of that kind.

No local flags. Uses `--colony` and `--output`.

A single object always prints as YAML unless you pass `-o json`. Lists in
`table` format print these columns:

| Kind | Columns |
|---|---|
| Colony | `NAME`, `DISPLAY NAME`, `GENERATION` |
| Agent | `COLONY`, `NAME`, `CURRENT VERSION` |
| AgentVersion | `COLONY`, `NAME`, `AGENT`, `VERSION` |
| Run | `COLONY`, `NAME`, `AGENT`, `PHASE` |
| Tool | `COLONY`, `NAME`, `TYPE`, `RISK` |

Lists in `json` print a JSON array; lists in `yaml` print `---`-separated
documents.

`get colony <name>` ignores `--colony`, since Colony is hive-scoped. When
`--colony` is empty, listing a colony-scoped kind returns every Colony's
resources; see [Roadmap](./roadmap.md#known-limitations-in-v010).

```bash
hived get colonies
hived --colony acme get agents
hived --colony acme get agent greeter
hived --colony acme get runs -o json
```

### `hived watch`

```
hived watch <kind>
```

Stream `ADDED`, `MODIFIED`, `DELETED` and `BOOKMARK` events for a resource
kind until interrupted.

| Flag | Type | Default | Meaning |
|---|---|---|---|
| `--since-resource-version` | int64 | `0` | Only stream events after this `resourceVersion`. |

Output is one tab-separated line per event: event type, resource version, and
name. For Colony the name is bare; for every other kind it is `colony/name`.

`watch` ignores `--output` and does not reconnect after a Keeper restart.

```bash
hived --colony acme watch runs
hived --colony acme watch runs --since-resource-version 42
```

### `hived events`

```
hived events <run>
```

List events recorded for a Run. Requires `--colony`.

| Flag | Type | Default | Meaning |
|---|---|---|---|
| `--since-seq` | int64 | `0` | Only list events with `seq` greater than this. |

`table` output prints `SEQ`, `TYPE` and `TIME` (formatted `15:04:05.000`).
`json` and `yaml` print each event in full.

Nothing emits events in v0.1.0, so this is normally empty.

### `hived run`

```
hived run <agent>
```

Create a Run for an Agent. Requires `--colony`.

| Flag | Type | Default | Meaning |
|---|---|---|---|
| `--name` | string | `""` | Name for the new Run. Required; Run names must be unique within a colony. |
| `--input` | string | `""` | JSON object passed as the Run's input. Must parse as JSON. |

The Run is created in `RUN_PHASE_PENDING` and stays there. The command prints
a warning to stderr saying so, then prints the created object per `--output`.

```bash
hived --colony acme run greeter --name run-1 --input '{"question":"hi"}'
```

### `hived logs`

```
hived logs <run>
```

| Flag | Type | Default | Meaning |
|---|---|---|---|
| `--follow` | bool | `false` | Reserved. The flag is parsed; the command fails regardless. |

**Always exits non-zero** with a "not implemented" message. Streaming a Cell's
output requires an Executor, and no Executor exists. The Keeper's
`RunService.Logs` RPC likewise returns `CodeUnimplemented`.

### `hived approve`

```
hived approve <run>
```

No flags. **Always exits non-zero** with a "not implemented" message.
Approving a pending tool call requires the Policy engine, an Approval resource
and the Tool Broker. None of them exists.

### `hived version`

```
hived version
```

Prints the CLI build information: version, commit and build date. Takes no
arguments. The format matches `hived-keeper version` so the two can be
compared at a glance when debugging a version skew.

## Kind aliases

`get` and `watch` accept any of these for `<kind>`.

| Kind | Aliases |
|---|---|
| `Colony` | `colony`, `colonies`, `co` |
| `Agent` | `agent`, `agents`, `ag` |
| `AgentVersion` | `agentversion`, `agentversions`, `av` |
| `Run` | `run`, `runs` |
| `Tool` | `tool`, `tools` |

Anything else is rejected with a message listing the valid kinds. Note that
`apply` routes on the manifest's `kind` field, which is the canonical
UpperCamelCase name, not an alias.

## Errors and exit codes

Errors print once, to stderr, and the CLI exits non-zero. Cobra's usage text
is suppressed on error, so a failure shows the failure and not a wall of help.

## Not implemented

`docs/PROJECT.md` sketches a wider verb set. These do **not** exist as
commands today:

`hived delete`, `hived cancel`, `hived colony create`, `hived tool register`,
`hived executor list`, `hived dev`.

There is also no delete, at any layer. The Keeper's services expose only
`Apply`, `Get`, `List` and `Watch` (plus `Logs` on Run), the store interface
has no delete method, and although the schema carries a `deletedAt` column and
`Watch` knows how to classify a `DELETED` event, nothing ever sets it.
`Run.spec.cancel` is settable but nothing acts on it.
