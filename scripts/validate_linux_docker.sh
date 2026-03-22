#!/bin/bash
#
# Build and validate the shim inside a clean Linux Docker image.
#
# This exercises the real Linux runtime path end-to-end:
#   1. build or fetch libwebrtc for the requested target
#   2. build the shim with Bazel
#   3. verify the Linux shim is portable (no host libstdc++/libgcc_s dependency)
#   4. package a release-shaped shim tarball
#   5. run both direct-path and zero-config smoke tests against the built shim
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
SHIM_RELEASE_TAG="${SHIM_RELEASE_TAG:-$(cd "$REPO_ROOT" && python3 - <<'PY'
import json
import pathlib

manifest = json.loads(pathlib.Path("internal/ffi/shim_manifest.json").read_text())
print(manifest["flavors"]["basic"]["release_tag"])
PY
)}"

docker_go_version() {
    local version="$1"
    if [[ "$version" == *.x ]]; then
        echo "${version%.x}"
        return
    fi
    echo "$version"
}

DOCKER_GO_VERSION="$(docker_go_version "$GO_VERSION")"

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
        GO_TEST_ENV=""
        GO_TEST_EXPORTS=":;"
        ;;
    linux_386)
        DOCKER_PLATFORM="linux/amd64"
        TEST_GOARCH="386"
        EXTRA_SETUP=$'RUN set -eux; \\\n        dpkg --add-architecture i386; \\\n        apt-get update; \\\n        libstdcxx_dev_pkg="libstdc++-$(g++ -dumpversion | cut -d. -f1)-dev:i386"; \\\n        apt-get install -y --no-install-recommends \\\n            gcc-multilib \\\n            g++-multilib \\\n            libc6-dev-i386 \\\n            lib32gcc-s1 \\\n            lib32stdc++6 \\\n            "$libstdcxx_dev_pkg" \\\n            libasound2-dev:i386 \\\n            libdrm-dev:i386 \\\n            libgbm-dev:i386 \\\n            libglib2.0-dev:i386 \\\n            libpulse-dev:i386 \\\n            libx11-dev:i386 \\\n            libxcomposite-dev:i386 \\\n            libxdamage-dev:i386 \\\n            libxext-dev:i386 \\\n            libxfixes-dev:i386 \\\n            libxrandr-dev:i386 \\\n            libxrender-dev:i386 \\\n            libxtst-dev:i386; \\\n        rm -rf /var/lib/apt/lists/*'
        GO_TEST_ENV='CGO_ENABLED=1 CC="gcc -m32" CXX="g++ -m32"'
        GO_TEST_EXPORTS=$'export CGO_ENABLED=1; \\\n    export CC="gcc -m32"; \\\n    export CXX="g++ -m32";'
        ;;
    linux_arm64)
        DOCKER_PLATFORM="linux/arm64/v8"
        TEST_GOARCH="arm64"
        EXTRA_SETUP=""
        GO_TEST_ENV=""
        GO_TEST_EXPORTS=":;"
        ;;
    linux_arm)
        DOCKER_PLATFORM="linux/arm/v7"
        TEST_GOARCH="arm"
        EXTRA_SETUP=""
        GO_TEST_ENV=""
        GO_TEST_EXPORTS=":;"
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
FROM golang:DOCKER_IMAGE_GO_VERSION_PLACEHOLDER-bookworm

RUN apt-get update && apt-get install -y --no-install-recommends \
        bash \
        build-essential \
        ca-certificates \
        curl \
        file \
        gawk \
        git \
        libasound2-dev \
        libdrm-dev \
        libgbm-dev \
        libglib2.0-dev \
        lsb-release \
        ninja-build \
        pkg-config \
        libpulse-dev \
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
ENV WEBRTC_INSTALL_BUILD_DEPS=0
ENV WEBRTC_GCLIENT_JOBS=1

RUN --mount=type=cache,target=/root/.cache/libgowebrtc \
    --mount=type=cache,target=/root/.cache/bazel \
    TARGET_PLATFORM=TARGET_PLACEHOLDER INSTALL_DIR=/tmp/libwebrtc ./scripts/build.sh
RUN ldd lib/TARGET_PLACEHOLDER/libwebrtc_shim.so
RUN if ldd lib/TARGET_PLACEHOLDER/libwebrtc_shim.so | grep -E 'libstdc\+\+|libgcc_s'; then \
        echo "Linux shim should not depend on host libstdc++ or libgcc_s"; \
        exit 1; \
    fi

RUN python3 - <<'PY'
import hashlib
import json
import pathlib
import tarfile

workspace = pathlib.Path("/workspace")
manifest = json.loads((workspace / "internal/ffi/shim_manifest.json").read_text())
flavor = manifest["flavors"]["basic"]
release_tag = flavor["release_tag"]
asset_name = flavor["assets"]["TARGET_PLACEHOLDER"]["file"]
release_dir = workspace / "release" / release_tag
dist_dir = workspace / "dist-release"
release_dir.mkdir(parents=True, exist_ok=True)
dist_dir.mkdir(parents=True, exist_ok=True)

files = [
    workspace / "lib/TARGET_PLACEHOLDER/libwebrtc_shim.so",
    workspace / "shim/shim.h",
]
license_file = workspace / "LICENSE"
if license_file.exists():
    files.append(license_file)

for src in files:
    (dist_dir / src.name).write_bytes(src.read_bytes())

tar_path = release_dir / asset_name
with tarfile.open(tar_path, "w:gz") as tar:
    for path in sorted(dist_dir.iterdir()):
        tar.add(path, arcname=path.name)

digest = hashlib.sha256(tar_path.read_bytes()).hexdigest()
(release_dir / f"{asset_name}.sha256").write_text(f"{digest}  {asset_name}\n")
PY

ENV LIBWEBRTC_SHIM_PATH=/workspace/lib/TARGET_PLACEHOLDER/libwebrtc_shim.so

RUN GO_TEST_ENV_PLACEHOLDER GOARCH=GOARCH_PLACEHOLDER LIBWEBRTC_PREFER_SOFTWARE_CODECS=1 go test -count=1 -run 'TestCreateVideoDecoderH264|TestCreateVideoEncoderH264|TestGetSupportedVideoCodecsFFI' ./internal/ffi
RUN GO_TEST_ENV_PLACEHOLDER GOARCH=GOARCH_PLACEHOLDER LIBWEBRTC_PREFER_SOFTWARE_CODECS=1 go test -count=1 -run TestH264EncoderEncode ./pkg/encoder
RUN GO_TEST_ENV_PLACEHOLDER GOARCH=GOARCH_PLACEHOLDER LIBWEBRTC_PREFER_SOFTWARE_CODECS=1 go test -count=1 -run TestH264EncodeDecode ./pkg/decoder
RUN GO_TEST_ENV_PLACEHOLDER GOARCH=GOARCH_PLACEHOLDER LIBWEBRTC_PREFER_SOFTWARE_CODECS=1 go test -count=1 -run TestGetSupportedVideoCodecs ./pkg/pc
RUN mkdir -p /tmp/relocated-shim && cp lib/TARGET_PLACEHOLDER/libwebrtc_shim.so /tmp/relocated-shim/libwebrtc_shim.so
RUN GO_TEST_ENV_PLACEHOLDER GOARCH=GOARCH_PLACEHOLDER LIBWEBRTC_PREFER_SOFTWARE_CODECS=1 LIBWEBRTC_SHIM_PATH=/tmp/relocated-shim/libwebrtc_shim.so go test -count=1 -run 'TestCreateVideoDecoderH264|TestCreateVideoEncoderH264|TestGetSupportedVideoCodecsFFI' ./internal/ffi
RUN mkdir -p /tmp/release-extract && tar -xzf /workspace/release/*/libwebrtc_shim_TARGET_PLACEHOLDER.tar.gz -C /tmp/release-extract
RUN cmp /workspace/lib/TARGET_PLACEHOLDER/libwebrtc_shim.so /tmp/release-extract/libwebrtc_shim.so
RUN GO_TEST_ENV_PLACEHOLDER GOARCH=GOARCH_PLACEHOLDER LIBWEBRTC_PREFER_SOFTWARE_CODECS=1 LIBWEBRTC_SHIM_PATH=/tmp/release-extract/libwebrtc_shim.so go test -count=1 -run 'TestCreateVideoDecoderH264|TestCreateVideoEncoderH264|TestGetSupportedVideoCodecsFFI' ./internal/ffi
RUN mkdir -p /tmp/synthetic-cache/shim/basic/SHIM_RELEASE_TAG_PLACEHOLDER/TARGET_PLACEHOLDER && cp /workspace/lib/TARGET_PLACEHOLDER/libwebrtc_shim.so /tmp/synthetic-cache/shim/basic/SHIM_RELEASE_TAG_PLACEHOLDER/TARGET_PLACEHOLDER/libwebrtc_shim.so
RUN GO_TEST_ENV_PLACEHOLDER GOARCH=GOARCH_PLACEHOLDER LIBWEBRTC_PREFER_SOFTWARE_CODECS=1 LIBWEBRTC_SHIM_PATH=/tmp/synthetic-cache/shim/basic/SHIM_RELEASE_TAG_PLACEHOLDER/TARGET_PLACEHOLDER/libwebrtc_shim.so go test -count=1 -run 'TestCreateVideoDecoderH264|TestCreateVideoEncoderH264|TestGetSupportedVideoCodecsFFI' ./internal/ffi
RUN tmp_home=$(mktemp -d) && tmp_gocache=$(mktemp -d) && tmp_gomodcache=$(mktemp -d) && \
    HOME="$tmp_home" GOCACHE="$tmp_gocache" GOMODCACHE="$tmp_gomodcache" CGO_ENABLED=1 \
    GO_TEST_ENV_PLACEHOLDER GOARCH=GOARCH_PLACEHOLDER LIBWEBRTC_PREFER_SOFTWARE_CODECS=1 \
    LIBWEBRTC_SHIM_PATH=/tmp/synthetic-cache/shim/basic/SHIM_RELEASE_TAG_PLACEHOLDER/TARGET_PLACEHOLDER/libwebrtc_shim.so \
    go test -count=1 -run 'TestCreateVideoDecoderH264|TestCreateVideoEncoderH264|TestGetSupportedVideoCodecsFFI' ./internal/ffi

RUN set -eux; \
    python3 -m http.server 18080 --bind 127.0.0.1 --directory /workspace/release >/tmp/libgowebrtc-http.log 2>&1 & \
    server_pid=$!; \
    trap 'kill "$server_pid" 2>/dev/null || true' EXIT; \
    until curl -fsSL http://127.0.0.1:18080/ >/dev/null; do sleep 1; done; \
    unset LIBWEBRTC_SHIM_PATH; \
    export GOARCH=GOARCH_PLACEHOLDER; \
    GO_TEST_EXPORTS_PLACEHOLDER \
    export LIBWEBRTC_SHIM_BASE_URL=http://127.0.0.1:18080; \
    export LIBWEBRTC_PREFER_SOFTWARE_CODECS=1; \
    TEST_PACKAGES="./internal/ffi" GO_TEST_ARGS="-run TestCreateVideoDecoderH264|TestCreateVideoEncoderH264|TestGetSupportedVideoCodecsFFI" ./scripts/test_clean_env.sh; \
    TEST_PACKAGES="./pkg/encoder" GO_TEST_ARGS="-run TestH264EncoderEncode" ./scripts/test_clean_env.sh; \
    TEST_PACKAGES="./pkg/decoder" GO_TEST_ARGS="-run TestH264EncodeDecode" ./scripts/test_clean_env.sh; \
    TEST_PACKAGES="./pkg/pc" GO_TEST_ARGS="-run TestGetSupportedVideoCodecs" ./scripts/test_clean_env.sh; \
    kill "$server_pid"; \
    wait "$server_pid" || true

CMD ["/bin/true"]
EOF

sed -i.bak \
    -e "s/DOCKER_IMAGE_GO_VERSION_PLACEHOLDER/$DOCKER_GO_VERSION/g" \
    -e "s/GO_VERSION_PLACEHOLDER/$GO_VERSION/g" \
    -e "s/GOARCH_PLACEHOLDER/$TEST_GOARCH/g" \
    -e "s|GO_TEST_ENV_PLACEHOLDER|$GO_TEST_ENV|g" \
    -e "s/SHIM_RELEASE_TAG_PLACEHOLDER/$SHIM_RELEASE_TAG/g" \
    -e "s/TARGET_PLACEHOLDER/$TARGET_PLATFORM/g" \
    "$DOCKERFILE_PATH"
rm -f "$DOCKERFILE_PATH.bak"

python3 - "$DOCKERFILE_PATH" "$EXTRA_SETUP" "$GO_TEST_EXPORTS" <<'PY'
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
extra = sys.argv[2]
go_test_exports = sys.argv[3]
text = path.read_text()
text = text.replace("EXTRA_SETUP_PLACEHOLDER", extra)
text = text.replace("GO_TEST_EXPORTS_PLACEHOLDER", go_test_exports)
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
