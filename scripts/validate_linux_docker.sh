#!/bin/bash
#
# Build and validate the shim inside a clean Linux Docker image.
#
# This exercises the real Linux runtime path end-to-end:
#   1. build or fetch libwebrtc for the requested target
#   2. build the shim with Bazel
#   3. verify the Linux shim is portable (no host libstdc++/libgcc_s dependency)
#   4. run the zero-config H.264 smoke tests against the built shim
#
# Usage:
#   ./scripts/validate_linux_docker.sh
#   ./scripts/validate_linux_docker.sh --target linux_386
#   ./scripts/validate_linux_docker.sh --target linux_arm
#   ./scripts/validate_linux_docker.sh --target linux_arm64
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

TARGET_PLATFORM="${TARGET_PLATFORM:-linux_amd64}"
GO_VERSION="${GO_VERSION:-1.25}"
IMAGE_NAME="${IMAGE_NAME:-libgowebrtc-validate}"
DOCKER_PROGRESS="${DOCKER_PROGRESS:-plain}"
ARTIFACT_DIR="${ARTIFACT_DIR:-}"
CONTAINER_NAME=""

usage() {
    cat <<EOF
Validate Linux shim build in Docker.

Usage: ./scripts/validate_linux_docker.sh [OPTIONS]

Options:
  --target PLATFORM  linux_amd64 (default), linux_386, linux_arm64, or linux_arm
  --go VERSION       Go toolchain version for the validator image (default: $GO_VERSION)
  --artifact-dir DIR Copy the built shim and public headers to DIR after validation
  --help             Show this help
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --target)
            TARGET_PLATFORM="$2"
            shift 2
            ;;
        --go)
            GO_VERSION="$2"
            shift 2
            ;;
        --artifact-dir)
            ARTIFACT_DIR="$2"
            shift 2
            ;;
        --help)
            usage
            exit 0
            ;;
        *)
            echo "Unknown argument: $1" >&2
            usage >&2
            exit 1
            ;;
    esac
done

case "$TARGET_PLATFORM" in
    linux_amd64)
        DOCKER_PLATFORM="linux/amd64"
        TEST_GOARCH="amd64"
        EXTRA_SETUP=""
        ;;
    linux_386)
        DOCKER_PLATFORM="linux/amd64"
        TEST_GOARCH="386"
        EXTRA_SETUP=$'RUN if [ "TARGET_PLACEHOLDER" = "linux_386" ]; then \\\n        dpkg --add-architecture i386; \\\n        apt-get update; \\\n        apt-get install -y --no-install-recommends \\\n            gcc-multilib \\\n            g++-multilib \\\n            libc6-dev-i386 \\\n            lib32gcc-s1 \\\n            lib32stdc++6 \\\n            libdrm-dev:i386 \\\n            libgbm-dev:i386 \\\n            libglib2.0-dev:i386 \\\n            libx11-dev:i386 \\\n            libxcomposite-dev:i386 \\\n            libxdamage-dev:i386 \\\n            libxext-dev:i386 \\\n            libxfixes-dev:i386 \\\n            libxrandr-dev:i386 \\\n            libxrender-dev:i386 \\\n            libxtst-dev:i386; \\\n        rm -rf /var/lib/apt/lists/*; \\\n    fi'
        ;;
    linux_arm64)
        DOCKER_PLATFORM="linux/arm64/v8"
        TEST_GOARCH="arm64"
        EXTRA_SETUP=""
        ;;
    linux_arm)
        DOCKER_PLATFORM="linux/arm/v7"
        TEST_GOARCH="arm"
        EXTRA_SETUP=""
        ;;
    *)
        echo "Unsupported target: $TARGET_PLATFORM" >&2
        exit 1
        ;;
esac

echo "==> Validating $TARGET_PLATFORM in Docker"
echo "    Docker platform: $DOCKER_PLATFORM"
echo "    Go version:      $GO_VERSION"
echo ""

DOCKERIGNORE_PATH="$REPO_ROOT/.dockerignore.validate-linux"
DOCKERFILE_PATH="$REPO_ROOT/Dockerfile.validate-linux"

cat > "$DOCKERIGNORE_PATH" <<'EOF'
.git
lib/
bazel-*
*.tar.gz
.cache/
EOF

cat > "$DOCKERFILE_PATH" <<'EOF'
# syntax=docker/dockerfile:1.7
FROM golang:GO_VERSION_PLACEHOLDER-bookworm

RUN apt-get update && apt-get install -y --no-install-recommends \
        bash \
        build-essential \
        ca-certificates \
        curl \
        file \
        gawk \
        git \
        libdrm-dev \
        libgbm-dev \
        libglib2.0-dev \
        lsb-release \
        ninja-build \
        pkg-config \
        rsync \
        libx11-dev \
        libxcomposite-dev \
        libxdamage-dev \
        libxext-dev \
        libxfixes-dev \
        libxrandr-dev \
        libxrender-dev \
        libxtst-dev \
        openjdk-17-jdk-headless \
        python3 \
        sudo \
        unzip \
        xz-utils \
        zip \
    && rm -rf /var/lib/apt/lists/*

EXTRA_SETUP_PLACEHOLDER

RUN go install github.com/bazelbuild/bazelisk@latest \
    && ln -sf /go/bin/bazelisk /usr/local/bin/bazel

WORKDIR /workspace
COPY . .

ENV PATH=/usr/local/go/bin:/go/bin:/usr/local/bin:/usr/bin:/bin
ENV GOWORK=off
ENV WEBRTC_INSTALL_BUILD_DEPS=1
ENV WEBRTC_GCLIENT_JOBS=1

RUN --mount=type=cache,target=/root/.cache/libgowebrtc \
    --mount=type=cache,target=/root/.cache/bazel \
    TARGET_PLATFORM=TARGET_PLACEHOLDER INSTALL_DIR=/tmp/libwebrtc ./scripts/build.sh
RUN ldd lib/TARGET_PLACEHOLDER/libwebrtc_shim.so
RUN if ldd lib/TARGET_PLACEHOLDER/libwebrtc_shim.so | grep -E 'libstdc\+\+|libgcc_s'; then \
        echo "Linux shim should not depend on host libstdc++ or libgcc_s"; \
        exit 1; \
    fi

ENV LIBWEBRTC_SHIM_PATH=/workspace/lib/TARGET_PLACEHOLDER/libwebrtc_shim.so

RUN GOARCH=GOARCH_PLACEHOLDER go test -count=1 -run 'TestCreateVideoDecoderH264|TestCreateVideoEncoderH264|TestGetSupportedVideoCodecsFFI' ./internal/ffi
RUN GOARCH=GOARCH_PLACEHOLDER go test -count=1 -run TestH264EncoderEncode ./pkg/encoder
RUN GOARCH=GOARCH_PLACEHOLDER go test -count=1 -run TestH264EncodeDecode ./pkg/decoder
RUN GOARCH=GOARCH_PLACEHOLDER go test -count=1 -run TestGetSupportedVideoCodecs ./pkg/pc

CMD ["/bin/true"]
EOF

sed -i.bak \
    -e "s/GO_VERSION_PLACEHOLDER/$GO_VERSION/g" \
    -e "s/GOARCH_PLACEHOLDER/$TEST_GOARCH/g" \
    -e "s/TARGET_PLACEHOLDER/$TARGET_PLATFORM/g" \
    "$DOCKERFILE_PATH"
rm -f "$DOCKERFILE_PATH.bak"

python3 - "$DOCKERFILE_PATH" "$EXTRA_SETUP" <<'PY'
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
extra = sys.argv[2]
text = path.read_text()
text = text.replace("EXTRA_SETUP_PLACEHOLDER", extra)
path.write_text(text)
PY

cleanup() {
    rm -f "$DOCKERIGNORE_PATH" "$DOCKERFILE_PATH"
    if [[ -n "$CONTAINER_NAME" ]]; then
        docker rm -f "$CONTAINER_NAME" >/dev/null 2>&1 || true
    fi
}
trap cleanup EXIT

docker build \
    --progress "$DOCKER_PROGRESS" \
    --platform "$DOCKER_PLATFORM" \
    -f "$DOCKERFILE_PATH" \
    -t "${IMAGE_NAME}:${TARGET_PLATFORM}" \
    "$REPO_ROOT"

if [[ -n "$ARTIFACT_DIR" ]]; then
    CONTAINER_NAME="${IMAGE_NAME}-${TARGET_PLATFORM}-extract-$$"
    rm -rf "$ARTIFACT_DIR"
    mkdir -p "$ARTIFACT_DIR"
    docker create --platform "$DOCKER_PLATFORM" --name "$CONTAINER_NAME" "${IMAGE_NAME}:${TARGET_PLATFORM}" >/dev/null
    docker cp "$CONTAINER_NAME:/workspace/lib/$TARGET_PLATFORM/libwebrtc_shim.so" "$ARTIFACT_DIR/libwebrtc_shim.so"
    docker cp "$CONTAINER_NAME:/workspace/shim/shim.h" "$ARTIFACT_DIR/shim.h"
    docker cp "$CONTAINER_NAME:/workspace/LICENSE" "$ARTIFACT_DIR/LICENSE" >/dev/null 2>&1 || true
    docker rm -f "$CONTAINER_NAME" >/dev/null
    CONTAINER_NAME=""
fi

echo ""
echo "==> Docker validation passed for $TARGET_PLATFORM"
