GO      := ./scripts/go-in-podman.sh go
COMPOSE := podman compose -f deploy/compose/docker-compose.yaml

# buf and goreleaser run from the host when installed, otherwise through the
# pinned golang container (this environment has no host Go toolchain).
BUF        := $(shell command -v buf 2>/dev/null || echo ./scripts/buf-in-podman.sh)
GORELEASER := ./scripts/goreleaser-in-podman.sh

# Postgres for `make test-integration`; override to point elsewhere.
HIVED_TEST_PG_DSN ?= postgres://hived:hived@host.containers.internal:5432/hived?sslmode=disable

# Build metadata injected into internal/version. A plain `make build` should
# report something traceable, not the bare "dev" default.
MODULE     := github.com/vibed-project/hiveD
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT     ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    := -s -w \
	-X $(MODULE)/internal/version.Version=$(VERSION) \
	-X $(MODULE)/internal/version.Commit=$(COMMIT) \
	-X $(MODULE)/internal/version.BuildDate=$(BUILD_DATE)

.PHONY: build build-cli build-all
build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/hived-keeper ./cmd/keeper

build-cli:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/hived ./cmd/hived

build-all: build build-cli

.PHONY: test test-race test-integration
test:
	$(GO) test ./... -short -count=1

test-race:
	CGO_ENABLED=1 $(GO) test -race -short -count=1 ./...

# Requires a reachable Postgres (`make compose-up`). HIVED_REQUIRE_PG makes an
# unreachable one fail instead of skipping, so this target cannot report a
# false green. host.containers.internal is how the toolchain container reaches
# a Postgres published on the host.
test-integration:
	HIVED_TEST_PG_DSN="$(HIVED_TEST_PG_DSN)" HIVED_REQUIRE_PG=1 \
		$(GO) test -tags=integration -count=1 ./internal/store/...

.PHONY: lint boundary
lint: boundary
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run ./...

boundary:
	./scripts/check-import-boundary.sh

.PHONY: proto proto-lint proto-check
proto:
	$(BUF) generate

proto-lint:
	$(BUF) lint
	$(BUF) format --diff --exit-code

proto-check: proto
	git diff --exit-code gen/

.PHONY: tidy
tidy:
	$(GO) mod tidy

.PHONY: image
image:
	podman build -t localhost/hived:dev .

.PHONY: release-check release-snapshot
# Validate .goreleaser.yaml without publishing anything.
release-check:
	$(GORELEASER) check

# Build every release artifact locally, exactly as a tag build would, but
# without touching GitHub. Output lands in ./dist.
release-snapshot:
	$(GORELEASER) build --snapshot --clean

.PHONY: compose-up compose-down compose-logs
compose-up:
	$(COMPOSE) up -d --build

compose-down:
	$(COMPOSE) down -v

compose-logs:
	$(COMPOSE) logs -f

.PHONY: migrate-up migrate-status
migrate-up:
	./bin/hived-keeper migrate up

migrate-status:
	./bin/hived-keeper migrate status

.PHONY: clean
clean:
	rm -rf bin/
