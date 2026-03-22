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

GOCACHE_DIR="${TEST_CLEAN_ENV_GOCACHE_DIR:-$(mktemp -d)}"
GOMODCACHE_DIR="${TEST_CLEAN_ENV_GOMODCACHE_DIR:-$(mktemp -d)}"
HOME_DIR="${TEST_CLEAN_ENV_HOME_DIR:-$(mktemp -d)}"
HIDDEN_LIB_DIR=""
KEEP_DIRS="${TEST_CLEAN_ENV_KEEP_DIRS:-0}"

cleanup() {
    if [[ -n "$HIDDEN_LIB_DIR" && -d "$HIDDEN_LIB_DIR" ]]; then
        rm -rf "$REPO_ROOT/lib"
        mv "$HIDDEN_LIB_DIR" "$REPO_ROOT/lib"
    fi
    if [[ "$KEEP_DIRS" == "1" ]]; then
        printf 'test_clean_env kept dirs: HOME=%s GOCACHE=%s GOMODCACHE=%s\n' "$HOME_DIR" "$GOCACHE_DIR" "$GOMODCACHE_DIR" >&2
        return
    fi
    chmod -R u+w "$GOCACHE_DIR" "$GOMODCACHE_DIR" "$HOME_DIR" 2>/dev/null || true
    rm -rf "$GOCACHE_DIR" "$GOMODCACHE_DIR" "$HOME_DIR"
}
trap cleanup EXIT

if [[ -d "$REPO_ROOT/lib" ]]; then
    HIDDEN_LIB_DIR="$(mktemp -d "${REPO_ROOT}/.lib-hidden.XXXXXX")"
    rm -rf "$HIDDEN_LIB_DIR"
    mv "$REPO_ROOT/lib" "$HIDDEN_LIB_DIR"
fi

export HOME="$HOME_DIR"

unset LIBWEBRTC_SHIM_CACHE_DIR LIBWEBRTC_SHIM_PATH LIBWEBRTC_DIR

export GOCACHE="$GOCACHE_DIR"
export GOMODCACHE="$GOMODCACHE_DIR"
export CGO_ENABLED=1
export GOWORK=off

if [[ "${GOOS:-$(go env GOOS)}" == "linux" && "${GOARCH:-$(go env GOARCH)}" == "386" ]]; then
    export CC="${CC:-gcc -m32}"
    export CXX="${CXX:-g++ -m32}"
fi

cd "$REPO_ROOT"

declare -a test_packages=()
declare -a extra_args=()

if [[ -n "${TEST_PACKAGES:-}" ]]; then
    # shellcheck disable=SC2206
    test_packages=(${TEST_PACKAGES})
else
    test_packages=(./internal/ffi)
fi

if [[ -n "${GO_TEST_ARGS:-}" ]]; then
    # shellcheck disable=SC2206
    extra_args=(${GO_TEST_ARGS})
fi

go test "${test_packages[@]}" "${extra_args[@]}" -count=1
