#!/usr/bin/env bash
#
# buf-in-podman.sh — run buf (plus the protoc-gen-go / protoc-gen-connect-go
# plugins it shells out to) inside a pinned golang container, since this
# development environment has no host Go toolchain and no local buf install.
#
# Usage: scripts/buf-in-podman.sh generate
#        scripts/buf-in-podman.sh lint
#        scripts/buf-in-podman.sh format --diff --exit-code
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
GO_IMAGE="${HIVED_GO_IMAGE:-docker.io/library/golang:1.26}"

BUF_VERSION="${HIVED_BUF_VERSION:-v1.47.2}"
# Keep these in sync with the google.golang.org/protobuf and
# connectrpc.com/connect versions pinned in go.mod.
PROTOC_GEN_GO_VERSION="${HIVED_PROTOC_GEN_GO_VERSION:-v1.36.12}"
PROTOC_GEN_CONNECT_GO_VERSION="${HIVED_PROTOC_GEN_CONNECT_GO_VERSION:-v1.20.0}"

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
		go install github.com/bufbuild/buf/cmd/buf@${BUF_VERSION} &&
		go install google.golang.org/protobuf/cmd/protoc-gen-go@${PROTOC_GEN_GO_VERSION} &&
		go install connectrpc.com/connect/cmd/protoc-gen-connect-go@${PROTOC_GEN_CONNECT_GO_VERSION} &&
		buf $*
	"
