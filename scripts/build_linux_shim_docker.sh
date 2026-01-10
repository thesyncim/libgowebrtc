#!/bin/bash
#
# Build Linux shim binaries via Docker buildx and optionally package them.
#
# Usage:
#   RELEASE_TAG=shim-v0.1.0 SHIM_FLAVOR=basic ./scripts/build_linux_shim_docker.sh [amd64|arm64|all]
#
# Environment variables:
#   SHIM_FLAVOR           - basic (default) or h264
#   ENABLE_H264           - Override H264 enablement (0/1). Defaults from SHIM_FLAVOR.
#   RELEASE_TAG           - If set, package and print manifest entries.
#   SKIP_LIBWEBRTC_BUILD  - Pass through to Dockerfile (default: 0)
#   WEBRTC_BRANCH         - Optional WebRTC branch for Docker build
#   GO_VERSION            - Optional Go version for Docker build
#   DOCKER_BUILDX_BUILDER - Optional buildx builder name
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

SHIM_FLAVOR="${SHIM_FLAVOR:-basic}"
SKIP_LIBWEBRTC_BUILD="${SKIP_LIBWEBRTC_BUILD:-0}"
RELEASE_TAG="${RELEASE_TAG:-}"

if [ -z "${ENABLE_H264:-}" ]; then
    ENABLE_H264=1
fi

if ! command -v docker >/dev/null 2>&1; then
    echo "docker is required"
    exit 1
fi

if ! docker buildx version >/dev/null 2>&1; then
    echo "docker buildx is required"
    exit 1
fi

archs=("$@")
if [ ${#archs[@]} -eq 0 ] || [ "${archs[0]}" = "all" ]; then
    archs=(amd64 arm64)
fi

arch_to_target_cpu() {
    case "$1" in
        amd64) echo "x64" ;;
        arm64) echo "arm64" ;;
        *)
            echo "unsupported arch: $1" >&2
            exit 1
            ;;
    esac
}

build_arch() {
    local arch="$1"
    local platform="linux/${arch}"
    local target_cpu
    local image
    local cid
    local dest_dir
    local lib_path
    local build_args
    local build_cmd

    target_cpu="$(arch_to_target_cpu "$arch")"
    image="libgowebrtc-shim-linux-${arch}-${SHIM_FLAVOR}"
    dest_dir="$REPO_ROOT/lib/linux_${arch}"
    lib_path="/workspace/lib/linux_${arch}/libwebrtc_shim.so"

    echo "==> Building ${platform} (TARGET_CPU=${target_cpu}, H264=${ENABLE_H264})"

    build_args=(
        --build-arg "SKIP_LIBWEBRTC_BUILD=${SKIP_LIBWEBRTC_BUILD}"
        --build-arg "TARGET_CPU=${target_cpu}"
        --build-arg "ENABLE_H264=${ENABLE_H264}"
        --build-arg "TARGETARCH=${arch}"
    )
    if [ -n "${WEBRTC_BRANCH:-}" ]; then
        build_args+=(--build-arg "WEBRTC_BRANCH=${WEBRTC_BRANCH}")
    fi
    if [ -n "${GO_VERSION:-}" ]; then
        build_args+=(--build-arg "GO_VERSION=${GO_VERSION}")
    fi

    build_cmd=(
        docker buildx build
        --platform "${platform}"
        --load
        -t "${image}"
        "${build_args[@]}"
    )
    if [ -n "${DOCKER_BUILDX_BUILDER:-}" ]; then
        build_cmd+=(--builder "${DOCKER_BUILDX_BUILDER}")
    fi
    build_cmd+=("${REPO_ROOT}")

    "${build_cmd[@]}"

    cid="$(docker create "${image}")"
    mkdir -p "${dest_dir}"
    docker cp "${cid}:${lib_path}" "${dest_dir}/libwebrtc_shim.so"
    docker rm "${cid}" >/dev/null

    echo "==> Copied ${dest_dir}/libwebrtc_shim.so"

    if [ -n "${RELEASE_TAG}" ]; then
        TARGET_PLATFORM="linux_${arch}" \
        SHIM_FLAVOR="${SHIM_FLAVOR}" \
        SKIP_BUILD=1 \
        RELEASE_TAG="${RELEASE_TAG}" \
        "${REPO_ROOT}/scripts/release_shim.sh"
    fi
}

for arch in "${archs[@]}"; do
    build_arch "$arch"
done
