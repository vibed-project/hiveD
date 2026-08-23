#!/usr/bin/env bash
#
# goreleaser-in-podman.sh — run goreleaser inside a pinned golang container,
# since this development environment has no host Go toolchain.
#
# Usage: scripts/goreleaser-in-podman.sh check
#        scripts/goreleaser-in-podman.sh build --snapshot --clean
#
# Keep GORELEASER_VERSION in sync with the goreleaser-action version pinned
# in .github/workflows/ci.yaml.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GO_IMAGE="${HIVED_GO_IMAGE:-docker.io/library/golang:1.26}"
GORELEASER_VERSION="${HIVED_GORELEASER_VERSION:-v2.12.7}"

mkdir -p "${REPO_ROOT}/.gocache" "${REPO_ROOT}/.gomodcache" "${REPO_ROOT}/.gotools"

exec podman run --rm -t \
	-v "${REPO_ROOT}:/src:Z" \
	-v "${REPO_ROOT}/.gocache:/root/.cache/go-build:Z" \
	-v "${REPO_ROOT}/.gomodcache:/go/pkg/mod:Z" \
	-v "${REPO_ROOT}/.gotools:/go/bin:Z" \
	-w /src \
	-e GOWORK=off \
	"${GO_IMAGE}" \
	sh -c "
		go install github.com/goreleaser/goreleaser/v2@${GORELEASER_VERSION} &&
		goreleaser $*
	"
