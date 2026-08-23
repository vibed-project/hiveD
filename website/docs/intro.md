---
slug: /
sidebar_position: 1
title: Introduction
---

# hiveD

hiveD is a standalone, open source control plane that defines, schedules,
governs and observes AI agents, running them inside isolated sandboxes
(vibeD by default) and giving them durable memory (mindD by default).

> vibeD builds the cells, mindD keeps the memory, hiveD runs the colony.

## Why it exists

Agent frameworks (CrewAI, LangGraph, ADK, the OpenAI and Anthropic SDKs)
solve the in-process problem: how to write an agent loop. They do not solve
the operational problem: where the agent runs, under what identity, with
which tools and budget, how it is paused, resumed, audited and stopped, and
how many agents coordinate without stepping on each other. Hyperscalers
answer that with proprietary managed runtimes. hiveD is meant to be the
vendor-neutral, self-hostable answer.

Four things follow from that:

- **Framework-agnostic.** hiveD does not care which library an agent's logic
  uses. Third-party frameworks are user code inside the sandbox, not a hiveD
  dependency.
- **Composable, not monolithic.** hiveD, vibeD and mindD are three
  repositories with hard API contracts. Each is useful alone.
- **Governance is a core primitive.** Identity, policy, budgets, approval
  gates and audit are in the resource model from day one, not bolted on.
- **Boring, operable software.** Go binaries, Postgres, protobuf and
  connect-go. No exotic dependencies.

## Vocabulary

This vocabulary is normative. Use these words exactly, in code, docs and UI,
not synonyms.

| Term | Meaning |
|---|---|
| Hive | A hiveD installation (control plane instance). Multi-tenant. |
| Colony | Tenant / isolation boundary. |
| Agent | A versioned, immutable definition of an agent. |
| Run ("Worker" in prose) | One running instance of an Agent. |
| Cell | The isolated sandbox a Run executes in. |
| Session | Optional grouping of Runs sharing context and episodic memory. |
| Tool | A registered capability a Run may call. |
| Policy | Rules attached to a Colony or Agent. |
| Keeper | The control plane process. Binary `hived-keeper`. |
| Drone | The per-Cell runtime binary. Binary `hived-drone`. |

## Status today

**v0.1.0 is released.** It is the first tagged release and it is pre-alpha.
Be clear about what that means.

What works:

- The Keeper's resource API: `Apply`, `Get`, `List` and `Watch` for Colony,
  Agent, AgentVersion, Run and Tool, plus an append-only Event log.
- A Postgres-backed resource store with optimistic concurrency, pagination,
  and a List-then-Watch cursor handoff.
- The `hived` CLI: `apply`, `get`, `watch`, `events`, `run`, `version`.
- A docker-compose dev stack (Keeper plus Postgres) and released binaries
  and a container image.

What does not work yet:

- **There is no Scheduler and no Executor.** A Run you create is persisted in
  `RUN_PHASE_PENDING` and stays there forever. Nothing picks it up, nothing
  provisions a Cell, nothing calls a model.
- The `hived-drone` binary does not exist. Neither does the Model Gateway or
  the Tool Broker; their proto contracts are defined, the implementations are
  not.
- Authentication is a no-op stub. Every request is accepted with a fixed dev
  principal, and `List` returns every Colony's resources when no colony is
  given. Do not expose a v0.1.0 Keeper to untrusted callers.
- Policy is stored but never evaluated. `Tool.spec.riskClass` is recorded and
  not enforced.
- `hived logs` and `hived approve` exit non-zero on purpose.

The full list is in [Roadmap and known limitations](./roadmap.md).

## Next

- [Quickstart](./quickstart.md): install, start a Keeper, apply your first
  Colony and Agent.
- [Architecture](./architecture.md): the Keeper/Drone split, the invariants,
  and how the store works.
- [Resource reference](./resources.md): every kind and every spec field.
- [CLI reference](./cli.md): every command and flag.
