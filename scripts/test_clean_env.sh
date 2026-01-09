#!/bin/bash
#
# Clean-environment smoke test to validate shim auto-download.
#
# Usage:
#   ./scripts/test_clean_env.sh
#
# Environment variables:
#   TEST_PACKAGES - Go test target (default: ./internal/ffi)
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

WORKDIR="$(mktemp -d)"
SHIM_CACHE="$(mktemp -d)"
GOCACHE_DIR="$(mktemp -d)"
GOMODCACHE_DIR="$(mktemp -d)"

cleanup() {
    chmod -R u+w "$WORKDIR" "$SHIM_CACHE" "$GOCACHE_DIR" "$GOMODCACHE_DIR" 2>/dev/null || true
    rm -rf "$WORKDIR" "$SHIM_CACHE" "$GOCACHE_DIR" "$GOMODCACHE_DIR"
}
trap cleanup EXIT

rsync -a \
    --exclude '.git' \
    --exclude 'lib/' \
    --exclude 'shim/build/' \
    --exclude '*.tar.gz' \
    "$REPO_ROOT/" "$WORKDIR/"

export HOME="$WORKDIR/home"
mkdir -p "$HOME"

export LIBWEBRTC_SHIM_CACHE_DIR="$SHIM_CACHE"
unset LIBWEBRTC_SHIM_PATH LIBWEBRTC_DIR

export GOCACHE="$GOCACHE_DIR"
export GOMODCACHE="$GOMODCACHE_DIR"
export CGO_ENABLED=1

cd "$WORKDIR"
go test "${TEST_PACKAGES:-./internal/ffi}" -count=1
