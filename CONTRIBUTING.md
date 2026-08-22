# Contributing to hiveD

Thanks for your interest in hiveD! Contributions of all kinds are welcome —
bug reports, documentation fixes, tests, and code. This guide covers how to
get a local development environment running and what we expect from a pull
request.

hiveD is an Apache-2.0 project. By contributing you agree that your work is
licensed under the same terms — see [LICENSE](LICENSE).

## Prerequisites

- **Go 1.26** — but you do not need it installed on your host. Every build
  and test target runs inside a `golang:1.26` container via
  [`scripts/go-in-podman.sh`](scripts/go-in-podman.sh), because this project
  is developed without a host Go toolchain.
- **Podman** (or Docker) — the container runtime used for the Go toolchain
  shim, `docker-compose`/`podman compose`, and image builds.
- **[buf](https://buf.build)** — only required if you're changing `proto/`.
  If you don't have `buf` installed locally, run it through the same podman
  pattern (see `make proto`).

If you do have a host Go toolchain and prefer to use it directly, that's
fine too — `go build ./... ` and `go test ./...` work unmodified, just set
`GOWORK=off` first (see [Local Go workspace note](#local-go-workspace-note)).

## Building

```bash
make build       # bin/hived-keeper
make build-cli    # bin/hived
make build-all    # both
```

## Testing

```bash
make test              # unit tests (go test ./... -short)
make test-race         # -race, ./internal/...
make test-integration  # internal/store integration tests against Postgres (needs `make compose-up`)
```

Please add or update tests alongside any behavior change.

## Code style

```bash
make lint      # boundary check + golangci-lint
make boundary  # ./scripts/check-import-boundary.sh only
```

`make lint` runs `make boundary`, which asserts this module stays
self-contained: it must never import a sibling `github.com/vibed-project/*`
module directly — mindD included, since it is now
`github.com/vibed-project/mindD`. Those integrations go
through interfaces and generated clients, not cross-module Go imports — see
[docs/adr/ADR-0004-mind-memory-usage.md](docs/adr/ADR-0004-mind-memory-usage.md)
for why.

## Protobuf changes

`proto/` is the source of truth; `gen/` is generated and checked in — never
hand-edit it.

```bash
make proto-lint   # buf lint + buf format --diff --exit-code
make proto        # buf generate; commit the resulting gen/ diff
```

CI fails if `gen/` drifts from what `buf generate` produces, so always run
`make proto` after editing a `.proto` file and commit the result.

## Local Go workspace note

If you check this repository out next to sibling modules under a shared
`go.work`, note that hiveD is deliberately **not** a member of it — see
ADR-0004 for why. If you run `go` commands from inside `hiveD/` and a parent
`go.work` is picked up unexpectedly, set `GOWORK=off` explicitly. The
Makefile and the podman shim already do this for you.

## Local development loop

```bash
make compose-up     # Keeper + Postgres (default profile)
make compose-logs
make compose-down
```

Once Keeper is healthy (`curl localhost:8080/healthz`), use the CLI against
it: `bin/hived apply -f colony.yaml`, `bin/hived get colonies`, etc.

## Documentation

Architecture and design context live in [`docs/PROJECT.md`](docs/PROJECT.md)
and [`docs/adr/`](docs/adr/). If your change affects a cross-cutting
decision recorded there (identity/token shape, resource model, executor
contract, memory usage), update or add an ADR alongside the code change.

## Submitting changes

1. Fork the repository and create a topic branch off `main`.
2. Make your change, add tests, and run `make test` and `make lint` locally.
3. Keep commits focused and write clear commit messages.
4. Open a pull request describing what changed and why.

### Developer Certificate of Origin

All commits must be signed off under the
[Developer Certificate of Origin](DCO). This is a lightweight statement that
you wrote the patch or otherwise have the right to submit it under the
project's open-source license. Add the sign-off automatically with the `-s`
flag:

```bash
git commit -s -m "Your commit message"
```

This appends a `Signed-off-by: Your Name <your@email>` trailer to the commit
message. Every commit in a pull request needs one; if you forgot, amend with
`git commit --amend -s` (or rebase to sign off a series). The name and email
in the trailer must match the commit author.

## Getting help

If you are unsure where to start, open an issue describing what you would
like to work on. We are happy to point you in the right direction.
