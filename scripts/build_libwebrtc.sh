#!/bin/bash
#
# Automated libwebrtc build script for libgowebrtc
#
# This script builds libwebrtc from source with H264 support enabled.
# It handles the complete build process including:
# - depot_tools setup
# - WebRTC source checkout (specific milestone branch)
# - Building with proper configuration
# - Installing headers and library
#
# Usage:
#   ./build_libwebrtc.sh                    # Build for current platform
#   ./build_libwebrtc.sh --clean            # Clean and rebuild
#   ./build_libwebrtc.sh --install-only     # Just install (skip build)
#   ./build_libwebrtc.sh --help             # Show help
#
# Environment variables:
#   WEBRTC_BRANCH     - WebRTC branch to build (default: branch-heads/7390 = M141)
#   BUILD_DIR         - Where to build (default: ~/webrtc_build)
#   INSTALL_DIR       - Where to install (default: ~/libwebrtc)
#   JOBS              - Number of parallel jobs (default: auto-detect)
#   TARGET_OS         - Target OS: mac, linux, win (default: auto-detect)
#   TARGET_CPU        - Target CPU: arm64, x64 (default: auto-detect)
#

set -e

# Configuration
WEBRTC_BRANCH="${WEBRTC_BRANCH:-branch-heads/7390}"  # M141.7390
BUILD_DIR="${BUILD_DIR:-$HOME/webrtc_build}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/libwebrtc}"
DEPOT_TOOLS_DIR="${DEPOT_TOOLS_DIR:-$HOME/depot_tools}"

# Auto-detect platform
detect_os() {
    case "$(uname -s)" in
        Darwin) echo "mac" ;;
        Linux)  echo "linux" ;;
        MINGW*|MSYS*|CYGWIN*) echo "win" ;;
        *) echo "unknown" ;;
    esac
}

detect_cpu() {
    case "$(uname -m)" in
        arm64|aarch64) echo "arm64" ;;
        x86_64|amd64)  echo "x64" ;;
        *) echo "unknown" ;;
    esac
}

TARGET_OS="${TARGET_OS:-$(detect_os)}"
TARGET_CPU="${TARGET_CPU:-$(detect_cpu)}"
JOBS="${JOBS:-$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4)}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info()    { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warn()    { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error()   { echo -e "${RED}[ERROR]${NC} $1"; }
log_step()    { echo -e "\n${GREEN}==>${NC} ${BLUE}$1${NC}"; }

show_help() {
    cat << 'EOF'
libwebrtc Build Script
======================

Builds libwebrtc from source with H264 support for use with libgowebrtc.

Usage:
  ./build_libwebrtc.sh [OPTIONS]

Options:
  --clean           Clean build directory and rebuild from scratch
  --install-only    Skip build, just install headers and library
  --skip-fetch      Skip depot_tools and source fetch (for rebuilds)
  --help            Show this help

Environment Variables:
  WEBRTC_BRANCH     WebRTC branch (default: branch-heads/7390 = M141)
  BUILD_DIR         Build directory (default: ~/webrtc_build)
  INSTALL_DIR       Install directory (default: ~/libwebrtc)
  DEPOT_TOOLS_DIR   depot_tools location (default: ~/depot_tools)
  TARGET_OS         Target OS: mac, linux, win (default: auto)
  TARGET_CPU        Target CPU: arm64, x64 (default: auto)
  JOBS              Parallel jobs (default: auto)

Examples:
  # Full build for current platform
  ./build_libwebrtc.sh

  # Rebuild after code changes
  ./build_libwebrtc.sh --skip-fetch

  # Build for specific platform
  TARGET_OS=linux TARGET_CPU=x64 ./build_libwebrtc.sh

  # Use different milestone
  WEBRTC_BRANCH=branch-heads/6834 ./build_libwebrtc.sh

Build Configuration:
  The script configures WebRTC with:
  - is_debug = false
  - is_component_build = false
  - rtc_include_tests = false
  - rtc_use_h264 = true (enables H264 codec)
  - use_rtti = true (required for Go bindings)
  - treat_warnings_as_errors = false

EOF
    exit 0
}

check_dependencies() {
    log_step "Checking dependencies..."

    local missing=()

    # Git is required
    if ! command -v git &> /dev/null; then
        missing+=("git")
    fi

    # Python is required
    if ! command -v python3 &> /dev/null && ! command -v python &> /dev/null; then
        missing+=("python3")
    fi

    # Platform-specific checks
    if [ "$TARGET_OS" = "mac" ]; then
        if ! xcode-select -p &> /dev/null; then
            log_warn "Xcode command line tools may not be installed"
            log_info "Run: xcode-select --install"
        fi
    fi

    if [ ${#missing[@]} -ne 0 ]; then
        log_error "Missing dependencies: ${missing[*]}"
        exit 1
    fi

    log_success "All dependencies found"
}

setup_depot_tools() {
    log_step "Setting up depot_tools..."

    if [ -d "$DEPOT_TOOLS_DIR" ]; then
        log_info "depot_tools already exists at $DEPOT_TOOLS_DIR"
        cd "$DEPOT_TOOLS_DIR"
        git pull origin main 2>/dev/null || true
    else
        log_info "Cloning depot_tools..."
        git clone https://chromium.googlesource.com/chromium/tools/depot_tools.git "$DEPOT_TOOLS_DIR"
    fi

    export PATH="$DEPOT_TOOLS_DIR:$PATH"
    log_success "depot_tools ready"
}

fetch_webrtc() {
    log_step "Fetching WebRTC source (branch: $WEBRTC_BRANCH)..."

    mkdir -p "$BUILD_DIR"
    cd "$BUILD_DIR"

    # Create .gclient configuration
    cat > .gclient << EOF
solutions = [
  {
    "name": "src",
    "url": "https://webrtc.googlesource.com/src.git@$WEBRTC_BRANCH",
    "deps_file": "DEPS",
    "managed": False,
    "custom_deps": {},
  },
]
target_os = ["$TARGET_OS"]
EOF

    log_info "Running gclient sync (this may take a while)..."
    gclient sync --nohooks

    log_info "Running hooks..."
    cd src
    gclient runhooks

    log_success "WebRTC source ready"
}

generate_build_files() {
    log_step "Generating build files..."

    cd "$BUILD_DIR/src"

    local out_dir="out/Release"
    mkdir -p "$out_dir"

    # Write args.gn
    cat > "$out_dir/args.gn" << EOF
# libwebrtc build configuration for libgowebrtc
# Generated by build_libwebrtc.sh

is_debug = false
is_component_build = false
rtc_include_tests = false

# Enable H264 support
rtc_use_h264 = true

# Required for Go CGO bindings
use_rtti = true

# Avoid build failures on warnings
treat_warnings_as_errors = false

# Target architecture
target_cpu = "$TARGET_CPU"
EOF

    # Platform-specific args
    if [ "$TARGET_OS" = "mac" ]; then
        cat >> "$out_dir/args.gn" << EOF

# macOS specific
target_os = "mac"
mac_deployment_target = "11.0"
EOF
    elif [ "$TARGET_OS" = "linux" ]; then
        cat >> "$out_dir/args.gn" << EOF

# Linux specific
target_os = "linux"
is_clang = true
use_sysroot = false
EOF
    fi

    log_info "Build configuration:"
    cat "$out_dir/args.gn"
    echo ""

    # Generate ninja files
    gn gen "$out_dir"

    log_success "Build files generated"
}

build_libwebrtc() {
    log_step "Building libwebrtc (using $JOBS parallel jobs)..."

    cd "$BUILD_DIR/src"

    # Build the main library and required targets
    ninja -C out/Release -j"$JOBS" \
        :webrtc \
        api/video_codecs:builtin_video_decoder_factory \
        api/video_codecs:builtin_video_encoder_factory \
        api/audio_codecs:builtin_audio_decoder_factory \
        api/audio_codecs:builtin_audio_encoder_factory \
        modules/audio_device

    # Verify the library was built
    if [ ! -f "out/Release/obj/libwebrtc.a" ]; then
        log_error "libwebrtc.a was not built!"
        exit 1
    fi

    local lib_size=$(ls -lh out/Release/obj/libwebrtc.a | awk '{print $5}')
    log_success "libwebrtc.a built successfully ($lib_size)"
}

install_libwebrtc() {
    log_step "Installing to $INSTALL_DIR..."

    mkdir -p "$INSTALL_DIR"/{include,lib,Frameworks}

    # Copy library
    log_info "Copying library..."
    cp "$BUILD_DIR/src/out/Release/obj/libwebrtc.a" "$INSTALL_DIR/lib/"

    # Copy headers
    log_info "Copying headers..."
    cd "$BUILD_DIR/src"

    # List of header directories to copy
    local header_dirs=(
        "api"
        "call"
        "common_video"
        "media"
        "modules"
        "p2p"
        "pc"
        "rtc_base"
        "system_wrappers"
        "common_audio"
        "video"
        "audio"
        "logging"
        "stats"
        "sdk"
    )

    for dir in "${header_dirs[@]}"; do
        if [ -d "$dir" ]; then
            rsync -av --include='*.h' --include='*/' --exclude='*' \
                "$dir/" "$INSTALL_DIR/include/$dir/" 2>/dev/null || true
        fi
    done

    # Copy third_party headers (abseil, etc.)
    log_info "Copying third_party headers..."
    mkdir -p "$INSTALL_DIR/include/third_party"

    if [ -d "third_party/abseil-cpp/absl" ]; then
        rsync -av --include='*.h' --include='*.inc' --include='*/' --exclude='*' \
            "third_party/abseil-cpp/absl/" "$INSTALL_DIR/include/third_party/abseil-cpp/absl/" 2>/dev/null || true
    fi

    if [ -d "third_party/libyuv/include" ]; then
        rsync -av "third_party/libyuv/include/" "$INSTALL_DIR/include/third_party/libyuv/include/" 2>/dev/null || true
    fi

    # Write version file
    local branch_version=$(echo "$WEBRTC_BRANCH" | sed 's/branch-heads\//m/')
    echo "${branch_version}@{#2}" > "$INSTALL_DIR/VERSION"

    # Copy NOTICE file if exists
    if [ -f "LICENSE" ]; then
        cp "LICENSE" "$INSTALL_DIR/NOTICE"
    fi

    # Print summary
    echo ""
    log_success "Installation complete!"
    echo ""
    log_info "Install location: $INSTALL_DIR"
    log_info "Library:          $INSTALL_DIR/lib/libwebrtc.a"
    log_info "Headers:          $INSTALL_DIR/include/"
    log_info "Version:          $(cat $INSTALL_DIR/VERSION)"
    echo ""
    log_info "To build the shim:"
    echo "  cd $(dirname "$0")/.."
    echo "  LIBWEBRTC_DIR=$INSTALL_DIR make shim shim-install"
    echo ""
    log_info "To run tests:"
    echo "  LIBWEBRTC_SHIM_PATH=\$PWD/lib/darwin_arm64/libwebrtc_shim.dylib go test ./..."
}

clean_build() {
    log_step "Cleaning build directory..."

    if [ -d "$BUILD_DIR/src/out" ]; then
        rm -rf "$BUILD_DIR/src/out"
        log_success "Build output cleaned"
    else
        log_info "Nothing to clean"
    fi
}

# Main entry point
main() {
    local skip_fetch=false
    local install_only=false
    local clean=false

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --help|-h)
                show_help
                ;;
            --clean)
                clean=true
                shift
                ;;
            --skip-fetch)
                skip_fetch=true
                shift
                ;;
            --install-only)
                install_only=true
                shift
                ;;
            *)
                log_error "Unknown option: $1"
                exit 1
                ;;
        esac
    done

    echo ""
    echo "======================================"
    echo "  libwebrtc Build Script"
    echo "======================================"
    echo ""
    echo "Configuration:"
    echo "  Branch:      $WEBRTC_BRANCH"
    echo "  Target OS:   $TARGET_OS"
    echo "  Target CPU:  $TARGET_CPU"
    echo "  Build Dir:   $BUILD_DIR"
    echo "  Install Dir: $INSTALL_DIR"
    echo "  Jobs:        $JOBS"
    echo ""

    if [ "$install_only" = true ]; then
        install_libwebrtc
        exit 0
    fi

    check_dependencies

    if [ "$clean" = true ]; then
        clean_build
    fi

    if [ "$skip_fetch" = false ]; then
        setup_depot_tools
        fetch_webrtc
    fi

    generate_build_files
    build_libwebrtc
    install_libwebrtc

    log_success "All done!"
}

main "$@"
