#!/usr/bin/env bash
#
# go-in-podman.sh — run a Go toolchain command inside a pinned golang
# container, since this development environment has no host Go toolchain.
#
# Usage: scripts/go-in-podman.sh go build ./...
#        scripts/go-in-podman.sh go test ./... -short
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GO_IMAGE="${HIVED_GO_IMAGE:-docker.io/library/golang:1.26}"

mkdir -p "${REPO_ROOT}/.gocache" "${REPO_ROOT}/.gomodcache"

# allow_host_loopback lets the container reach a Postgres published on the
# host (host.containers.internal). Without it the integration tests resolved
# "localhost" to the container itself and silently skipped.
exec podman run --rm -t \
	--network "slirp4netns:allow_host_loopback=true" \
	-v "${REPO_ROOT}:/src:Z" \
	-v "${REPO_ROOT}/.gocache:/root/.cache/go-build:Z" \
	-v "${REPO_ROOT}/.gomodcache:/go/pkg/mod:Z" \
	-w /src \
	-e GOWORK=off \
	-e GOFLAGS=-mod=mod \
	-e "CGO_ENABLED=${CGO_ENABLED:-0}" \
	-e "HIVED_TEST_PG_DSN=${HIVED_TEST_PG_DSN:-}" \
	-e "HIVED_REQUIRE_PG=${HIVED_REQUIRE_PG:-}" \
	"${GO_IMAGE}" "$@"
