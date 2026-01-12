#!/bin/bash
#
# Build libwebrtc from source for Linux with libstdc++ compatibility
#
# This builds libwebrtc with use_custom_libcxx=false so it uses the system
# libstdc++ instead of Chrome's bundled libc++, avoiding the __Cr namespace
# ABI incompatibility.
#
# Requirements:
#   - ~30GB disk space
#   - 1-2 hours build time
#   - Internet connection for depot_tools and source fetch
#
# Usage:
#   ./scripts/build_libwebrtc_linux.sh [--branch branch-heads/7390] [--arch x64|arm64]
#

set -e

# Configuration
WEBRTC_BRANCH="${WEBRTC_BRANCH:-branch-heads/7390}"  # M141
BUILD_DIR="${BUILD_DIR:-$HOME/webrtc_build}"
OUTPUT_DIR="${OUTPUT_DIR:-$HOME/libwebrtc}"

# Detect architecture
detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)  echo "x64" ;;
        aarch64|arm64) echo "arm64" ;;
        *)             echo "x64" ;;
    esac
}

TARGET_ARCH="${TARGET_ARCH:-$(detect_arch)}"

# Colors
log_info()    { echo -e "\033[0;34m[INFO]\033[0m $1"; }
log_success() { echo -e "\033[0;32m[SUCCESS]\033[0m $1"; }
log_error()   { echo -e "\033[0;31m[ERROR]\033[0m $1" >&2; }
log_step()    { echo -e "\n\033[0;32m==>\033[0m \033[0;34m$1\033[0m"; }

show_help() {
    cat << EOF
Build libwebrtc from source for Linux

Usage: $0 [OPTIONS]

Options:
  --branch BRANCH   WebRTC branch (default: $WEBRTC_BRANCH)
  --arch ARCH       Target architecture: x64 or arm64 (default: auto-detect)
  --build-dir DIR   Build directory (default: $BUILD_DIR)
  --output-dir DIR  Output directory (default: $OUTPUT_DIR)
  --skip-fetch      Skip fetching source (use existing)
  --skip-deps       Skip installing build dependencies
  --help            Show this help

Environment variables:
  WEBRTC_BRANCH     Same as --branch
  TARGET_ARCH       Same as --arch
  BUILD_DIR         Same as --build-dir
  OUTPUT_DIR        Same as --output-dir

Examples:
  $0                          # Build for current arch
  $0 --arch arm64             # Build for ARM64
  $0 --branch branch-heads/7390  # Build specific version (M141)

Note: This requires ~30GB disk space and takes 1-2 hours.
EOF
    exit 0
}

check_requirements() {
    log_step "Checking requirements"

    if ! command -v git &> /dev/null; then
        log_error "git is required"
        exit 1
    fi

    if ! command -v python3 &> /dev/null; then
        log_error "python3 is required"
        exit 1
    fi

    # Check disk space (need at least 30GB)
    local available_gb=$(df -BG "$HOME" | tail -1 | awk '{print $4}' | tr -d 'G')
    if [[ "$available_gb" -lt 30 ]]; then
        log_error "Insufficient disk space. Need at least 30GB, have ${available_gb}GB"
        exit 1
    fi

    log_success "Requirements met (${available_gb}GB available)"
}

setup_depot_tools() {
    log_step "Setting up depot_tools"

    if [[ -d "$HOME/depot_tools" ]]; then
        log_info "depot_tools already exists, updating..."
        cd "$HOME/depot_tools"
        git pull --quiet
    else
        log_info "Cloning depot_tools..."
        git clone --depth 1 https://chromium.googlesource.com/chromium/tools/depot_tools.git "$HOME/depot_tools"
    fi

    export PATH="$HOME/depot_tools:$PATH"
    log_success "depot_tools ready"
}

install_build_deps() {
    log_step "Installing build dependencies"

    if [[ "$SKIP_DEPS" == "true" ]]; then
        log_info "Skipping dependency installation"
        return
    fi

    if command -v apt-get &> /dev/null; then
        sudo apt-get update
        sudo apt-get install -y \
            git python3 python3-pip curl wget \
            lsb-release sudo \
            libx11-dev libxcomposite-dev libxdamage-dev libxext-dev \
            libxfixes-dev libxrandr-dev libxrender-dev libxtst-dev \
            libglib2.0-dev libgbm-dev libdrm-dev \
            libpulse-dev libasound2-dev \
            libgtk-3-dev libcups2-dev libnss3-dev \
            libpci-dev libdbus-1-dev \
            ninja-build pkg-config
    else
        log_error "Only Debian/Ubuntu supported for automatic dependency installation"
        log_info "Please install build dependencies manually"
    fi

    log_success "Dependencies installed"
}

fetch_webrtc() {
    log_step "Fetching WebRTC source ($WEBRTC_BRANCH)"

    if [[ "$SKIP_FETCH" == "true" && -d "$BUILD_DIR/src" ]]; then
        log_info "Skipping fetch, using existing source"
        return
    fi

    mkdir -p "$BUILD_DIR"
    cd "$BUILD_DIR"

    if [[ ! -d "src" ]]; then
        log_info "Initial fetch (this takes a while)..."
        fetch --nohooks webrtc
    fi

    cd src
    log_info "Checking out $WEBRTC_BRANCH..."
    git checkout "$WEBRTC_BRANCH" || git fetch origin "$WEBRTC_BRANCH" && git checkout FETCH_HEAD

    log_info "Syncing dependencies..."
    gclient sync -D --force --reset

    # Install WebRTC's own build dependencies
    if [[ -f build/install-build-deps.sh ]]; then
        log_info "Installing WebRTC build dependencies..."
        ./build/install-build-deps.sh --no-prompt || true
    fi

    log_success "WebRTC source ready"
}

build_webrtc() {
    log_step "Building libwebrtc for $TARGET_ARCH"

    cd "$BUILD_DIR/src"

    # Generate build files with libstdc++ compatibility
    log_info "Generating build files..."
    gn gen out/Release --args="
        target_os=\"linux\"
        target_cpu=\"$TARGET_ARCH\"
        is_debug=false
        is_component_build=false
        rtc_include_tests=false
        rtc_use_h264=true
        ffmpeg_branding=\"Chrome\"
        use_rtti=true
        use_custom_libcxx=false
        use_glib=false
        rtc_enable_protobuf=false
        treat_warnings_as_errors=false
        symbol_level=0
    "

    # Build
    log_info "Building (this takes 30-60 minutes)..."
    ninja -C out/Release webrtc

    log_success "Build complete"
}

package_output() {
    log_step "Packaging output to $OUTPUT_DIR"

    cd "$BUILD_DIR/src"

    rm -rf "$OUTPUT_DIR"
    mkdir -p "$OUTPUT_DIR/lib"
    mkdir -p "$OUTPUT_DIR/include"

    # Copy library
    if [[ -f out/Release/obj/libwebrtc.a ]]; then
        cp out/Release/obj/libwebrtc.a "$OUTPUT_DIR/lib/"
    else
        log_error "libwebrtc.a not found!"
        exit 1
    fi

    # Copy headers - selective copy of key directories
    for dir in api modules rtc_base call media pc p2p; do
        if [[ -d "$dir" ]]; then
            rsync -av --include='*/' --include='*.h' --include='*.inc' --exclude='*' \
                "$dir/" "$OUTPUT_DIR/include/$dir/"
        fi
    done

    # Copy third_party headers we need
    mkdir -p "$OUTPUT_DIR/include/third_party/abseil-cpp"
    rsync -av --include='*/' --include='*.h' --include='*.inc' --exclude='*' \
        third_party/abseil-cpp/ "$OUTPUT_DIR/include/third_party/abseil-cpp/"

    # Copy libyuv headers
    if [[ -d third_party/libyuv/include ]]; then
        mkdir -p "$OUTPUT_DIR/include/third_party/libyuv/include"
        rsync -av --include='*/' --include='*.h' --exclude='*' \
            third_party/libyuv/include/ "$OUTPUT_DIR/include/third_party/libyuv/include/"
    fi

    # Copy boringssl headers
    if [[ -d third_party/boringssl/src/include ]]; then
        mkdir -p "$OUTPUT_DIR/include/third_party/boringssl/src/include"
        rsync -av --include='*/' --include='*.h' --exclude='*' \
            third_party/boringssl/src/include/ "$OUTPUT_DIR/include/third_party/boringssl/src/include/"
    fi

    local lib_size=$(du -h "$OUTPUT_DIR/lib/libwebrtc.a" | cut -f1)
    log_success "Package complete:"
    log_info "  Library: $OUTPUT_DIR/lib/libwebrtc.a ($lib_size)"
    log_info "  Headers: $OUTPUT_DIR/include/"

    # Verify the library has correct symbols
    log_info "Verifying library symbols..."
    if nm "$OUTPUT_DIR/lib/libwebrtc.a" 2>/dev/null | grep -q "std::__Cr"; then
        log_error "WARNING: Library still contains std::__Cr symbols!"
    else
        log_success "Library uses libstdc++ (no __Cr namespace)"
    fi
}

main() {
    local skip_fetch=false
    local skip_deps=false

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --branch)     WEBRTC_BRANCH="$2"; shift 2 ;;
            --arch)       TARGET_ARCH="$2"; shift 2 ;;
            --build-dir)  BUILD_DIR="$2"; shift 2 ;;
            --output-dir) OUTPUT_DIR="$2"; shift 2 ;;
            --skip-fetch) SKIP_FETCH="true"; shift ;;
            --skip-deps)  SKIP_DEPS="true"; shift ;;
            --help)       show_help ;;
            *)            log_error "Unknown option: $1"; exit 1 ;;
        esac
    done

    log_info "Configuration:"
    log_info "  Branch: $WEBRTC_BRANCH"
    log_info "  Architecture: $TARGET_ARCH"
    log_info "  Build directory: $BUILD_DIR"
    log_info "  Output directory: $OUTPUT_DIR"

    check_requirements
    setup_depot_tools
    install_build_deps
    fetch_webrtc
    build_webrtc
    package_output

    echo ""
    log_success "libwebrtc built successfully!"
    echo ""
    echo "To use with libgowebrtc:"
    echo "  export LIBWEBRTC_DIR=$OUTPUT_DIR"
    echo "  ./scripts/build.sh"
}

main "$@"
