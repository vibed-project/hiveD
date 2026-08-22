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

exec podman run --rm -t \
	-v "${REPO_ROOT}:/src:Z" \
	-v "${REPO_ROOT}/.gocache:/root/.cache/go-build:Z" \
	-v "${REPO_ROOT}/.gomodcache:/go/pkg/mod:Z" \
	-w /src \
	-e GOWORK=off \
	-e GOFLAGS=-mod=mod \
	-e "CGO_ENABLED=${CGO_ENABLED:-0}" \
	"${GO_IMAGE}" "$@"
