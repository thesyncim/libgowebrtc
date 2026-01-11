#!/bin/bash
#
# Build and release shim for all platforms
#
# Usage:
#   ./scripts/release.sh                    # Build all platforms
#   ./scripts/release.sh --upload v0.3.0    # Build and upload to release
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

LIBWEBRTC_VERSION="${LIBWEBRTC_VERSION:-141.7390.2.0}"
LIBWEBRTC_BIN_REPO="${LIBWEBRTC_BIN_REPO:-crow-misia/libwebrtc-bin}"

log_info()    { echo -e "\033[0;34m[INFO]\033[0m $1"; }
log_success() { echo -e "\033[0;32m[SUCCESS]\033[0m $1"; }
log_error()   { echo -e "\033[0;31m[ERROR]\033[0m $1"; }
log_step()    { echo -e "\n\033[0;32m==>\033[0m \033[0;34m$1\033[0m"; }

build_darwin_arm64() {
    log_step "Building darwin_arm64 (native)"

    if [[ "$(uname -s)" != "Darwin" ]] || [[ "$(uname -m)" != "arm64" ]]; then
        log_error "darwin_arm64 must be built on macOS arm64"
        return 1
    fi

    ./scripts/build.sh

    mkdir -p "$PROJECT_ROOT/lib/darwin_arm64"
    cp bazel-bin/shim/libwebrtc_shim.dylib "$PROJECT_ROOT/lib/darwin_arm64/"
    log_success "darwin_arm64 built"
}

build_linux_docker() {
    local platform=$1
    local arch=$2
    local bazelisk_arch=$3
    local libwebrtc_arch=$4

    log_step "Building $platform (Docker)"

    if ! command -v docker &> /dev/null; then
        log_error "Docker is required for Linux builds"
        return 1
    fi

    docker run --rm --platform="linux/$arch" \
        -v "$PROJECT_ROOT":/workspace -w /workspace \
        ubuntu:22.04 bash -c "
            set -e
            apt-get update -qq && apt-get install -y -qq curl xz-utils g++ \
                libx11-dev libxcomposite-dev libxdamage-dev libxext-dev \
                libxfixes-dev libxrandr-dev libxrender-dev libxtst-dev \
                libglib2.0-dev libgbm-dev libdrm-dev >/dev/null

            curl -fsSL -o /usr/local/bin/bazel \
                https://github.com/bazelbuild/bazelisk/releases/download/v1.25.0/bazelisk-linux-$bazelisk_arch
            chmod +x /usr/local/bin/bazel

            mkdir -p /tmp/libwebrtc
            curl -fsSL https://github.com/$LIBWEBRTC_BIN_REPO/releases/download/$LIBWEBRTC_VERSION/libwebrtc-$libwebrtc_arch.tar.xz \
                | tar -xJ -C /tmp/libwebrtc

            export LIBWEBRTC_DIR=/tmp/libwebrtc
            bazel build //shim:webrtc_shim --config=$platform

            mkdir -p lib/$platform
            cp bazel-bin/shim/libwebrtc_shim.so lib/$platform/
        "

    log_success "$platform built"
}

create_tarball() {
    local platform=$1
    local ext=$2

    log_info "Packaging $platform..."

    rm -rf "$PROJECT_ROOT/dist"
    mkdir -p "$PROJECT_ROOT/dist"

    cp "$PROJECT_ROOT/lib/$platform/libwebrtc_shim.$ext" "$PROJECT_ROOT/dist/"
    cp "$PROJECT_ROOT/shim/shim.h" "$PROJECT_ROOT/dist/"
    cp "$PROJECT_ROOT/LICENSE" "$PROJECT_ROOT/dist/" 2>/dev/null || true

    local tarball="libwebrtc_shim_${platform}.tar.gz"
    (cd "$PROJECT_ROOT/dist" && tar -czf "$PROJECT_ROOT/$tarball" *)

    if command -v sha256sum &> /dev/null; then
        sha256sum "$PROJECT_ROOT/$tarball" > "$PROJECT_ROOT/${tarball}.sha256"
    else
        shasum -a 256 "$PROJECT_ROOT/$tarball" > "$PROJECT_ROOT/${tarball}.sha256"
    fi

    log_success "$tarball created"
}

upload_release() {
    local tag=$1

    log_step "Uploading to release $tag"

    if ! command -v gh &> /dev/null; then
        log_error "GitHub CLI (gh) is required for uploads"
        return 1
    fi

    gh release upload "$tag" \
        libwebrtc_shim_darwin_arm64.tar.gz \
        libwebrtc_shim_darwin_arm64.tar.gz.sha256 \
        libwebrtc_shim_linux_amd64.tar.gz \
        libwebrtc_shim_linux_amd64.tar.gz.sha256 \
        libwebrtc_shim_linux_arm64.tar.gz \
        libwebrtc_shim_linux_arm64.tar.gz.sha256 \
        --clobber

    log_success "Uploaded to $tag"
    gh release view "$tag"
}

main() {
    local upload_tag=""

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --upload)
                upload_tag="$2"
                shift 2
                ;;
            --help)
                echo "Usage: ./scripts/release.sh [--upload TAG]"
                echo ""
                echo "Builds shim for all platforms using Docker for Linux."
                echo ""
                echo "Options:"
                echo "  --upload TAG    Upload tarballs to GitHub release TAG"
                echo ""
                echo "Platforms built:"
                echo "  - darwin_arm64 (native, requires macOS arm64)"
                echo "  - linux_amd64  (Docker)"
                echo "  - linux_arm64  (Docker)"
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                exit 1
                ;;
        esac
    done

    cd "$PROJECT_ROOT"

    # Build all platforms
    build_darwin_arm64
    build_linux_docker "linux_amd64" "amd64" "amd64" "linux-x64"
    build_linux_docker "linux_arm64" "arm64" "arm64" "linux-arm64"

    # Create tarballs
    log_step "Creating tarballs"
    create_tarball "darwin_arm64" "dylib"
    create_tarball "linux_amd64" "so"
    create_tarball "linux_arm64" "so"

    # Show results
    log_step "Build complete"
    ls -la libwebrtc_shim_*.tar.gz*

    # Upload if requested
    if [[ -n "$upload_tag" ]]; then
        upload_release "$upload_tag"
    fi
}

main "$@"
