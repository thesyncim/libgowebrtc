#!/bin/bash
#
# Build script for libgowebrtc shim
#
# Downloads pre-compiled libwebrtc from crow-misia/libwebrtc-bin
# and builds the C++ shim library using Bazel.
#
# Usage:
#   ./scripts/build.sh              # Build for current platform
#   ./scripts/build.sh --clean      # Clean and rebuild
#   ./scripts/build.sh --release    # Build release tarball
#   ./scripts/build.sh --help       # Show help

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Version - MUST match shim/shim_common.cc
LIBWEBRTC_VERSION="${LIBWEBRTC_VERSION:-141.7390.2.0}"

# Download configuration
LIBWEBRTC_BIN_REPO="${LIBWEBRTC_BIN_REPO:-crow-misia/libwebrtc-bin}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/libwebrtc}"

# Platform detection
detect_platform() {
    local os cpu
    case "$(uname -s)" in
        Darwin) os="darwin" ;;
        Linux)  os="linux" ;;
        *)      os="unknown" ;;
    esac
    case "$(uname -m)" in
        arm64|aarch64) cpu="arm64" ;;
        x86_64|amd64)  cpu="amd64" ;;
        *)             cpu="unknown" ;;
    esac
    echo "${os}_${cpu}"
}

get_download_platform() {
    case "$(uname -s)" in
        Darwin)
            case "$(uname -m)" in
                arm64) echo "macos-arm64" ;;
                x86_64) echo "macos-x64" ;;
            esac ;;
        Linux)
            case "$(uname -m)" in
                aarch64) echo "linux-arm64" ;;
                x86_64) echo "linux-x64" ;;
            esac ;;
    esac
}

PLATFORM=$(detect_platform)
TARGET_OS=$(echo "$PLATFORM" | cut -d_ -f1)

# Colors
log_info()    { echo -e "\033[0;34m[INFO]\033[0m $1"; }
log_success() { echo -e "\033[0;32m[SUCCESS]\033[0m $1"; }
log_error()   { echo -e "\033[0;31m[ERROR]\033[0m $1"; }
log_step()    { echo -e "\n\033[0;32m==>\033[0m \033[0;34m$1\033[0m"; }

show_help() {
    cat << EOF
libgowebrtc Build Script
========================

Downloads pre-compiled libwebrtc and builds the C++ shim.

Usage: ./scripts/build.sh [OPTIONS]

Options:
  --clean      Clean build artifacts and rebuild
  --release    Create release tarball
  --help       Show this help

Environment:
  LIBWEBRTC_VERSION   Version to download (default: $LIBWEBRTC_VERSION)
  INSTALL_DIR         Where to install libwebrtc (default: ~/libwebrtc)

Examples:
  ./scripts/build.sh              # Build shim
  ./scripts/build.sh --release    # Create release tarball
EOF
    exit 0
}

download_libwebrtc() {
    if [[ -f "$INSTALL_DIR/lib/libwebrtc.a" ]]; then
        log_info "libwebrtc already installed at $INSTALL_DIR"
        return 0
    fi

    log_step "Downloading pre-compiled libwebrtc v${LIBWEBRTC_VERSION}"

    local platform=$(get_download_platform)
    if [[ -z "$platform" ]]; then
        log_error "Unsupported platform: $(uname -s) $(uname -m)"
        exit 1
    fi

    local url="https://github.com/${LIBWEBRTC_BIN_REPO}/releases/download/${LIBWEBRTC_VERSION}/libwebrtc-${platform}.tar.xz"
    local tmpdir=$(mktemp -d)

    log_info "Downloading from: $url"
    if ! curl -fSL --progress-bar -o "$tmpdir/libwebrtc.tar.xz" "$url"; then
        log_error "Download failed"
        rm -rf "$tmpdir"
        exit 1
    fi

    log_info "Extracting to $INSTALL_DIR..."
    mkdir -p "$INSTALL_DIR"
    tar -xf "$tmpdir/libwebrtc.tar.xz" -C "$INSTALL_DIR"
    rm -rf "$tmpdir"

    if [[ -f "$INSTALL_DIR/lib/libwebrtc.a" ]]; then
        log_success "Pre-compiled libwebrtc installed to $INSTALL_DIR"
    else
        log_error "libwebrtc.a not found after extraction"
        exit 1
    fi
}

build_shim() {
    log_step "Building shim with Bazel"
    cd "$PROJECT_ROOT"

    export LIBWEBRTC_DIR="$INSTALL_DIR"
    log_info "Building for platform: $PLATFORM"

    bazel build //shim:webrtc_shim --config="$PLATFORM"

    local ext="so"
    [[ "$TARGET_OS" == "darwin" ]] && ext="dylib"

    local lib_dir="$PROJECT_ROOT/lib/$PLATFORM"
    mkdir -p "$lib_dir"
    cp "bazel-bin/shim/libwebrtc_shim.$ext" "$lib_dir/"

    log_success "Shim built: $lib_dir/libwebrtc_shim.$ext"
}

create_release() {
    log_step "Creating release tarball"
    cd "$PROJECT_ROOT"

    local ext="so"
    [[ "$TARGET_OS" == "darwin" ]] && ext="dylib"

    local tarball="libwebrtc_shim_${PLATFORM}.tar.gz"
    local dist_dir="$PROJECT_ROOT/dist"

    rm -rf "$dist_dir"
    mkdir -p "$dist_dir"

    cp "bazel-bin/shim/libwebrtc_shim.$ext" "$dist_dir/"
    cp "shim/shim.h" "$dist_dir/"
    [[ -f "LICENSE" ]] && cp "LICENSE" "$dist_dir/"

    cd "$dist_dir"
    tar -czf "../$tarball" *
    cd "$PROJECT_ROOT"

    shasum -a 256 "$tarball" > "${tarball}.sha256" 2>/dev/null || sha256sum "$tarball" > "${tarball}.sha256" 2>/dev/null || true

    log_success "Release: $tarball"
    ls -la "$tarball"*
}

clean_all() {
    log_step "Cleaning build artifacts"
    cd "$PROJECT_ROOT"
    bazel clean --expunge 2>/dev/null || true
    rm -rf "$PROJECT_ROOT/lib" "$PROJECT_ROOT/dist" "$PROJECT_ROOT"/*.tar.gz*
    log_success "Cleaned"
}

main() {
    local do_clean=false
    local do_release=false

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --clean)   do_clean=true; shift ;;
            --release) do_release=true; shift ;;
            --help)    show_help ;;
            *) log_error "Unknown option: $1"; exit 1 ;;
        esac
    done

    log_info "Platform: $PLATFORM"
    log_info "libwebrtc version: $LIBWEBRTC_VERSION"
    log_info "Install dir: $INSTALL_DIR"

    $do_clean && clean_all

    download_libwebrtc
    build_shim

    $do_release && create_release

    log_success "Build complete!"
    echo ""
    echo "To test:"
    echo "  LIBWEBRTC_SHIM_PATH=$PROJECT_ROOT/lib/$PLATFORM/libwebrtc_shim.${ext:-dylib} go test ./..."
}

main "$@"
