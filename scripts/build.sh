#!/bin/bash
#
# Build script for libgowebrtc shim
#
# Downloads pre-compiled libwebrtc from crow-misia/libwebrtc-bin
# and builds the C++ shim library using Bazel.
#
# Usage:
#   ./scripts/build.sh                        # Build for current platform
#   ./scripts/build.sh --target darwin_amd64  # Cross-compile for Intel Mac
#   ./scripts/build.sh --clean                # Clean and rebuild
#   ./scripts/build.sh --release              # Build release tarball
#   ./scripts/build.sh --help                 # Show help

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
        MINGW*|MSYS*|CYGWIN*) os="windows" ;;
        *)      os="unknown" ;;
    esac
    case "$(uname -m)" in
        arm64|aarch64) cpu="arm64" ;;
        x86_64|amd64|AMD64)  cpu="amd64" ;;
        *)             cpu="unknown" ;;
    esac
    echo "${os}_${cpu}"
}

# Get download platform name for a given target
get_download_platform_for_target() {
    local target="$1"
    case "$target" in
        darwin_arm64)  echo "macos-arm64" ;;
        darwin_amd64)  echo "macos-x64" ;;
        linux_arm64)   echo "linux-arm64" ;;
        linux_amd64)   echo "linux-x64" ;;
        windows_amd64) echo "win-x64" ;;
        *) echo "" ;;
    esac
}

HOST_PLATFORM=$(detect_platform)
TARGET_PLATFORM="${TARGET_PLATFORM:-$HOST_PLATFORM}"
TARGET_OS=$(echo "$TARGET_PLATFORM" | cut -d_ -f1)

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
  --target PLATFORM  Target platform (darwin_arm64, darwin_amd64, linux_amd64, linux_arm64)
                     Default: current platform ($HOST_PLATFORM)
  --clean            Clean build artifacts and rebuild
  --release          Create release tarball
  --help             Show this help

Environment:
  LIBWEBRTC_VERSION   Version to download (default: $LIBWEBRTC_VERSION)
  INSTALL_DIR         Where to install libwebrtc (default: ~/libwebrtc)
  TARGET_PLATFORM     Alternative way to set target platform

Examples:
  ./scripts/build.sh                        # Build for current platform
  ./scripts/build.sh --target darwin_amd64  # Cross-compile for Intel Mac
  ./scripts/build.sh --release              # Create release tarball

Cross-compilation:
  On Apple Silicon Mac, you can cross-compile for Intel Mac:
    ./scripts/build.sh --target darwin_amd64

  The script will download the correct libwebrtc and build with Bazel.
EOF
    exit 0
}

build_libcxx_cr() {
    # Build libc++ with __Cr namespace for Linux
    if [[ "$TARGET_OS" != "linux" ]]; then
        return 0
    fi

    local libcxx_dir="${LIBCXX_CR_DIR:-$PROJECT_ROOT/libcxx-cr}"

    # Check if already built
    if [[ -f "$libcxx_dir/lib/libc++.so.1" ]]; then
        log_info "libc++ with __Cr namespace already built at $libcxx_dir"
        export LIBCXX_CR_DIR="$libcxx_dir"
        return 0
    fi

    log_step "Building libc++ with __Cr namespace for Chromium ABI compatibility"

    if [[ -f "$SCRIPT_DIR/build-libcxx.sh" ]]; then
        LIBCXX_INSTALL_DIR="$libcxx_dir" "$SCRIPT_DIR/build-libcxx.sh"
        export LIBCXX_CR_DIR="$libcxx_dir"
    else
        log_error "build-libcxx.sh not found"
        exit 1
    fi
}

download_libwebrtc() {
    # Check for existing installation
    local lib_file="libwebrtc.a"
    [[ "$TARGET_OS" == "windows" ]] && lib_file="webrtc.lib"

    if [[ -f "$INSTALL_DIR/lib/$lib_file" ]]; then
        log_info "libwebrtc already installed at $INSTALL_DIR"
        return 0
    fi

    log_step "Downloading pre-compiled libwebrtc v${LIBWEBRTC_VERSION} for ${TARGET_PLATFORM}"

    local download_platform=$(get_download_platform_for_target "$TARGET_PLATFORM")
    if [[ -z "$download_platform" ]]; then
        log_error "Unsupported target platform: $TARGET_PLATFORM"
        exit 1
    fi

    local tmpdir=$(mktemp -d)
    local archive_ext="tar.xz"
    [[ "$TARGET_OS" == "windows" ]] && archive_ext="7z"

    local url="https://github.com/${LIBWEBRTC_BIN_REPO}/releases/download/${LIBWEBRTC_VERSION}/libwebrtc-${download_platform}.${archive_ext}"

    log_info "Downloading from: $url"
    if ! curl -fSL --progress-bar -o "$tmpdir/libwebrtc.${archive_ext}" "$url"; then
        log_error "Download failed"
        rm -rf "$tmpdir"
        exit 1
    fi

    log_info "Extracting to $INSTALL_DIR..."
    mkdir -p "$INSTALL_DIR"

    if [[ "$TARGET_OS" == "windows" ]]; then
        # Windows uses 7z format
        log_info "Listing archive contents..."
        local sevenzip_cmd=""
        if command -v 7z &> /dev/null; then
            sevenzip_cmd="7z"
        elif command -v 7za &> /dev/null; then
            sevenzip_cmd="7za"
        else
            log_error "7z or 7za not found. Please install p7zip."
            rm -rf "$tmpdir"
            exit 1
        fi

        # List archive contents to see what we're working with
        $sevenzip_cmd l "$tmpdir/libwebrtc.7z" | grep -E "(webrtc\.lib|debug/|release/)" | head -20 || true

        log_info "Extracting archive (this may take a while for large files)..."
        # Extract directly to INSTALL_DIR to avoid copy issues
        $sevenzip_cmd x -o"$INSTALL_DIR" "$tmpdir/libwebrtc.7z" -y -bb1

        log_info "Extraction complete. Contents of $INSTALL_DIR:"
        ls -la "$INSTALL_DIR/" 2>/dev/null || dir "$INSTALL_DIR/" 2>/dev/null || true

        # Check specifically for debug folder
        if [[ -d "$INSTALL_DIR/debug" ]]; then
            log_info "Contents of debug/:"
            ls -la "$INSTALL_DIR/debug/" 2>/dev/null || dir "$INSTALL_DIR/debug/" 2>/dev/null || true
        else
            log_info "No debug/ folder found, checking all subdirs:"
            find "$INSTALL_DIR" -type d 2>/dev/null | head -20 || true
        fi
    else
        tar -xf "$tmpdir/libwebrtc.tar.xz" -C "$INSTALL_DIR"
    fi
    rm -rf "$tmpdir"

    # Debug: show what we got
    log_info "Contents of $INSTALL_DIR:"
    ls -la "$INSTALL_DIR/" 2>/dev/null || true
    ls -la "$INSTALL_DIR/lib/" 2>/dev/null || true

    # Find the lib file - crow-misia may use different names/locations
    local found_lib=""
    if [[ -f "$INSTALL_DIR/lib/$lib_file" ]]; then
        found_lib="$INSTALL_DIR/lib/$lib_file"
    elif [[ "$TARGET_OS" == "windows" ]]; then
        # Try alternative Windows locations/names
        # IMPORTANT: Prefer release/ over debug/ to avoid CRT mismatch
        # (debug is built with /MTd, we need /MD compatible)
        for try_path in \
            "$INSTALL_DIR/lib/webrtc.lib" \
            "$INSTALL_DIR/release/webrtc.lib" \
            "$INSTALL_DIR/webrtc.lib" \
            "$INSTALL_DIR/lib/libwebrtc.lib" \
            "$INSTALL_DIR/libwebrtc.lib" \
            "$INSTALL_DIR/out/Release/obj/webrtc.lib" \
            "$INSTALL_DIR/obj/webrtc.lib" \
            "$INSTALL_DIR/debug/webrtc.lib"; do
            if [[ -f "$try_path" ]]; then
                found_lib="$try_path"
                break
            fi
        done
        # If still not found, search for it
        if [[ -z "$found_lib" ]]; then
            found_lib=$(find "$INSTALL_DIR" -name "webrtc.lib" -type f 2>/dev/null | head -1)
        fi
        if [[ -z "$found_lib" ]]; then
            found_lib=$(find "$INSTALL_DIR" -name "*.lib" -type f 2>/dev/null | grep -i webrtc | head -1)
        fi
    fi

    if [[ -n "$found_lib" ]]; then
        log_success "Pre-compiled libwebrtc found at: $found_lib"
        # Make sure it's in the expected location for bazel
        if [[ "$found_lib" != "$INSTALL_DIR/lib/$lib_file" ]]; then
            mkdir -p "$INSTALL_DIR/lib"
            cp "$found_lib" "$INSTALL_DIR/lib/$lib_file"
            log_info "Copied to $INSTALL_DIR/lib/$lib_file"
        fi
    else
        log_error "$lib_file not found after extraction"
        log_error "Directory structure:"
        find "$INSTALL_DIR" -type f -name "*.lib" 2>/dev/null | head -30 || true
        find "$INSTALL_DIR" -type f -name "*.a" 2>/dev/null | head -30 || true
        log_error "All directories:"
        find "$INSTALL_DIR" -type d 2>/dev/null | head -50 || true
        exit 1
    fi
}

build_shim() {
    log_step "Building shim with Bazel"
    cd "$PROJECT_ROOT"

    export LIBWEBRTC_DIR="$INSTALL_DIR"

    if [[ "$HOST_PLATFORM" != "$TARGET_PLATFORM" ]]; then
        log_info "Cross-compiling: $HOST_PLATFORM -> $TARGET_PLATFORM"
    else
        log_info "Building for platform: $TARGET_PLATFORM"
    fi

    # For Linux, add custom libc++ include and library paths
    local extra_opts=""
    if [[ "$TARGET_OS" == "linux" && -n "$LIBCXX_CR_DIR" ]]; then
        log_info "Using libc++ from: $LIBCXX_CR_DIR"
        # Add include path for libc++ headers and library path for linking
        # Use --strategy=CppCompile=local to disable sandbox for C++ compilation
        extra_opts="--copt=-isystem$LIBCXX_CR_DIR/include/c++/v1 --linkopt=-L$LIBCXX_CR_DIR/lib --strategy=CppCompile=local --strategy=CppLink=local"
    fi

    # On Windows Git Bash/MSYS, // gets converted to / - disable path conversion
    MSYS_NO_PATHCONV=1 MSYS2_ARG_CONV_EXCL="*" bazel build //shim:webrtc_shim --config="$TARGET_PLATFORM" $extra_opts

    local ext="so"
    case "$TARGET_OS" in
        darwin)  ext="dylib" ;;
        windows) ext="dll" ;;
    esac

    local lib_dir="$PROJECT_ROOT/lib/$TARGET_PLATFORM"
    mkdir -p "$lib_dir"

    # Bazel outputs: libwebrtc_shim.{so,dylib} on Unix, webrtc_shim.dll on Windows
    # We unify to libwebrtc_shim.* for consistency
    local src_name="libwebrtc_shim.$ext"
    [[ "$TARGET_OS" == "windows" ]] && src_name="webrtc_shim.$ext"

    cp "bazel-bin/shim/$src_name" "$lib_dir/libwebrtc_shim.$ext"

    # For Linux, copy libc++ libraries alongside the shim
    if [[ "$TARGET_OS" == "linux" && -n "$LIBCXX_CR_DIR" ]]; then
        log_info "Copying libc++ runtime libraries..."
        cp "$LIBCXX_CR_DIR/lib/libc++.so"* "$lib_dir/" 2>/dev/null || true
        cp "$LIBCXX_CR_DIR/lib/libc++abi.so"* "$lib_dir/" 2>/dev/null || true
        log_info "Contents of $lib_dir:"
        ls -la "$lib_dir/"
    fi

    log_success "Shim built: $lib_dir/libwebrtc_shim.$ext"
}

create_release() {
    log_step "Creating release tarball"
    cd "$PROJECT_ROOT"

    local ext="so"
    case "$TARGET_OS" in
        darwin)  ext="dylib" ;;
        windows) ext="dll" ;;
    esac

    local tarball="libwebrtc_shim_${TARGET_PLATFORM}.tar.gz"
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
            --target)
                TARGET_PLATFORM="$2"
                TARGET_OS=$(echo "$TARGET_PLATFORM" | cut -d_ -f1)
                shift 2
                ;;
            --clean)   do_clean=true; shift ;;
            --release) do_release=true; shift ;;
            --help)    show_help ;;
            *) log_error "Unknown option: $1"; exit 1 ;;
        esac
    done

    # Update INSTALL_DIR to be platform-specific for cross-compilation
    if [[ "$HOST_PLATFORM" != "$TARGET_PLATFORM" ]]; then
        INSTALL_DIR="${INSTALL_DIR:-$HOME/libwebrtc}_${TARGET_PLATFORM}"
    fi

    log_info "Host platform: $HOST_PLATFORM"
    log_info "Target platform: $TARGET_PLATFORM"
    log_info "libwebrtc version: $LIBWEBRTC_VERSION"
    log_info "Install dir: $INSTALL_DIR"

    $do_clean && clean_all

    # For Linux, build libc++ with __Cr namespace first
    build_libcxx_cr

    download_libwebrtc
    build_shim

    $do_release && create_release

    local ext="so"
    case "$TARGET_OS" in
        darwin)  ext="dylib" ;;
        windows) ext="dll" ;;
    esac

    log_success "Build complete!"
    echo ""
    echo "To test:"
    if [[ "$HOST_PLATFORM" != "$TARGET_PLATFORM" && "$TARGET_PLATFORM" == "darwin_amd64" ]]; then
        echo "  arch -x86_64 env LIBWEBRTC_SHIM_PATH=$PROJECT_ROOT/lib/$TARGET_PLATFORM/libwebrtc_shim.$ext go test ./..."
    else
        echo "  LIBWEBRTC_SHIM_PATH=$PROJECT_ROOT/lib/$TARGET_PLATFORM/libwebrtc_shim.$ext go test ./..."
    fi
}

main "$@"
