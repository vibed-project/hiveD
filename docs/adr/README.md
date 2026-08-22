# Architecture Decision Records

hiveD records cross-cutting architectural decisions as ADRs — no tooling,
just plain markdown files, one decision per file.

## Convention

- Filename: `ADR-NNNN-kebab-title.md`, number zero-padded to 4 digits,
  assigned sequentially, never reused.
- Fixed heading skeleton:

  ```markdown
  # ADR-NNNN: Title

  ## Status
  Draft | Accepted | Superseded by ADR-XXXX

  ## Context
  ## Decision
  ## Consequences
  ## Alternatives considered
  ```

- A decision that reverses or replaces an earlier one gets a **new** ADR
  number; the old one's Status line is updated to point at it. ADRs are
  never edited to retroactively change what was decided — only their
  Status and, where useful, a short "Update" note at the bottom.
- Write a new ADR (or update an existing Draft one before it's built)
  whenever a change establishes or alters a contract another component
  depends on: identity/token shape, an executor contract, memory usage,
  event schema, policy language, API style. Don't write one for an
  ordinary implementation detail — see `CLAUDE.md` for the cutoff.

## Index

| # | Title | Status |
|---|---|---|
| [0001](ADR-0001-scope-and-principles.md) | Scope and principles | Draft |
| [0002](ADR-0002-identity-and-capability-tokens.md) | Identity and capability tokens | Draft |
| [0003](ADR-0003-vibed-executor-contract.md) | vibeD executor contract | Draft |
| [0004](ADR-0004-mind-memory-usage.md) | mindD memory usage | Draft |
| [0005](ADR-0005-event-schema.md) | Event schema | Draft |
