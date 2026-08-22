# Local dev compose stack

```bash
make compose-up      # Keeper + Postgres, default profile
make compose-logs
make compose-down
```

Uses `podman compose` (see the Makefile's `COMPOSE` variable) since this
project's dev workflow assumes podman, not Docker — see
[CONTRIBUTING.md](../../CONTRIBUTING.md). If you use Docker instead, the
same compose file works with `docker compose -f deploy/compose/docker-compose.yaml ...`.

Once `keeper` reports healthy:

```bash
curl localhost:8080/healthz
curl localhost:8080/readyz
curl localhost:9090/metrics
```

## The `mind` profile

There is no real mindD (MemorySidecar) integration in M0 — see
[docs/adr/ADR-0004](../../docs/adr/ADR-0004-mind-memory-usage.md). The
`mind` service in `docker-compose.yaml` is a placeholder documenting the
eventual integration point, gated behind a compose profile so it is never
required for a clean `make compose-up`:

```bash
podman compose -f deploy/compose/docker-compose.yaml --profile mind up -d
```

This builds mindD from a sibling checkout at `../../../MemorySidecar`
(mindD publishes no container image of its own), so it only works if you
have that repository cloned next to `hiveD/`. If you don't, that's fine —
you don't need the `mind` profile for anything in M0.

**Dev credentials only.** Any PASETO keys or tokens used against the
`mind` service in this compose file (once wired up in M1) are for local
development only — never reuse them anywhere real.
