#!/bin/bash
#
# Build script for libwebrtc_shim
#
# Usage:
#   ./build.sh                    # Build using LIBWEBRTC_DIR from environment
#   ./build.sh /path/to/libwebrtc # Build using specified libwebrtc directory
#   ./build.sh --fetch            # Fetch pre-built libwebrtc and build
#   ./build.sh --help             # Show this help
#
# Environment variables:
#   LIBWEBRTC_DIR   - Path to libwebrtc installation (include/ and lib/)
#   BUILD_TYPE      - Release (default) or Debug
#   CMAKE_GENERATOR - CMake generator (default: Ninja if available, else Unix Makefiles)
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUILD_TYPE="${BUILD_TYPE:-Release}"
BUILD_DIR="${SCRIPT_DIR}/build"
OUTPUT_DIR="${SCRIPT_DIR}/../lib"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

show_help() {
    cat << 'EOF'
libwebrtc_shim Build Script
===========================

Usage:
  ./build.sh                         Build using LIBWEBRTC_DIR from environment
  ./build.sh /path/to/libwebrtc      Build using specified libwebrtc directory
  ./build.sh --fetch                 Fetch pre-built libwebrtc and build (if available)
  ./build.sh --help                  Show this help

Environment Variables:
  LIBWEBRTC_DIR    Path to libwebrtc installation
  BUILD_TYPE       Release (default) or Debug
  CMAKE_GENERATOR  CMake generator to use

Directory Structure Expected:
  $LIBWEBRTC_DIR/
  ├── include/          # WebRTC headers
  │   ├── api/
  │   ├── rtc_base/
  │   ├── modules/
  │   └── third_party/
  └── lib/
      └── libwebrtc.a   # Static library

Building libwebrtc from source:
  See https://webrtc.googlesource.com/src/+/main/docs/native-code/development/

  Quick steps:
  1. Install depot_tools: https://commondatastorage.googleapis.com/chrome-infra-docs/flat/depot_tools/docs/html/depot_tools_tutorial.html
  2. Fetch WebRTC:
     $ mkdir webrtc && cd webrtc
     $ fetch --nohooks webrtc
     $ gclient sync
  3. Generate build files:
     $ cd src
     $ gn gen out/Release --args='is_debug=false is_component_build=false rtc_include_tests=false'
  4. Build:
     $ ninja -C out/Release
  5. Copy to install directory:
     $ mkdir -p /opt/libwebrtc/{include,lib}
     $ cp out/Release/obj/libwebrtc.a /opt/libwebrtc/lib/
     $ rsync -av --include='*.h' --include='*/' --exclude='*' ./ /opt/libwebrtc/include/

EOF
    exit 0
}

detect_platform() {
    local os=$(uname -s)
    local arch=$(uname -m)

    case "$os" in
        Darwin)
            case "$arch" in
                arm64) echo "darwin_arm64" ;;
                x86_64) echo "darwin_amd64" ;;
                *) log_error "Unsupported architecture: $arch"; exit 1 ;;
            esac
            ;;
        Linux)
            case "$arch" in
                aarch64) echo "linux_arm64" ;;
                x86_64) echo "linux_amd64" ;;
                *) log_error "Unsupported architecture: $arch"; exit 1 ;;
            esac
            ;;
        *)
            log_error "Unsupported OS: $os"
            exit 1
            ;;
    esac
}

check_dependencies() {
    log_info "Checking build dependencies..."

    local missing=()

    if ! command -v cmake &> /dev/null; then
        missing+=("cmake")
    fi

    if ! command -v ninja &> /dev/null && ! command -v make &> /dev/null; then
        missing+=("ninja or make")
    fi

    if [ ${#missing[@]} -ne 0 ]; then
        log_error "Missing dependencies: ${missing[*]}"
        log_info "Install with:"
        echo "  macOS: brew install cmake ninja"
        echo "  Linux: apt-get install cmake ninja-build"
        exit 1
    fi

    log_success "All dependencies found"
}

verify_libwebrtc() {
    local dir="$1"

    if [ ! -d "$dir" ]; then
        log_error "Directory does not exist: $dir"
        return 1
    fi

    if [ ! -d "$dir/include" ]; then
        log_error "Missing include directory: $dir/include"
        return 1
    fi

    if [ ! -d "$dir/lib" ]; then
        log_error "Missing lib directory: $dir/lib"
        return 1
    fi

    local lib_file=""
    if [ -f "$dir/lib/libwebrtc.a" ]; then
        lib_file="$dir/lib/libwebrtc.a"
    elif [ -f "$dir/lib/webrtc.lib" ]; then
        lib_file="$dir/lib/webrtc.lib"
    else
        log_error "Missing libwebrtc library in: $dir/lib/"
        return 1
    fi

    log_success "Found libwebrtc at: $lib_file"
    return 0
}

fetch_libwebrtc() {
    local platform=$(detect_platform)
    log_info "Attempting to fetch pre-built libwebrtc for $platform..."

    # Pre-built binaries URLs (these would need to be hosted somewhere)
    # For now, we provide instructions since libwebrtc doesn't have official pre-built releases

    log_warn "Pre-built libwebrtc binaries are not currently available for automated download."
    log_info ""
    log_info "Options:"
    log_info "1. Build libwebrtc from source (see --help for instructions)"
    log_info "2. Use a community-maintained pre-built package:"
    log_info "   - https://github.com/aspect-build/aspect-cli-plugin-webrtc (has pre-built binaries)"
    log_info "   - https://github.com/aspect-build/aspect-cli-plugin-webrtc/releases"
    log_info ""
    log_info "Once you have libwebrtc, run:"
    log_info "  LIBWEBRTC_DIR=/path/to/libwebrtc ./build.sh"
    exit 1
}

build_shim() {
    local libwebrtc_dir="$1"
    local platform=$(detect_platform)

    log_info "Building libwebrtc_shim for $platform..."
    log_info "Using libwebrtc from: $libwebrtc_dir"
    log_info "Build type: $BUILD_TYPE"

    # Create build directory
    mkdir -p "$BUILD_DIR"
    cd "$BUILD_DIR"

    # Determine generator
    local generator="${CMAKE_GENERATOR:-}"
    if [ -z "$generator" ]; then
        if command -v ninja &> /dev/null; then
            generator="Ninja"
        else
            generator="Unix Makefiles"
        fi
    fi

    log_info "Using CMake generator: $generator"

    # Configure
    cmake -G "$generator" \
        -DCMAKE_BUILD_TYPE="$BUILD_TYPE" \
        -DLIBWEBRTC_DIR="$libwebrtc_dir" \
        -DBUILD_SHARED_LIBS=ON \
        -DENABLE_DEVICE_CAPTURE=ON \
        "$SCRIPT_DIR"

    # Build
    cmake --build . --config "$BUILD_TYPE" -j$(nproc 2>/dev/null || sysctl -n hw.ncpu)

    # Install to output directory
    mkdir -p "$OUTPUT_DIR/$platform"

    local lib_name=""
    case "$platform" in
        darwin_*) lib_name="libwebrtc_shim.dylib" ;;
        linux_*) lib_name="libwebrtc_shim.so" ;;
    esac

    if [ -f "$lib_name" ]; then
        cp "$lib_name" "$OUTPUT_DIR/$platform/"
        log_success "Built: $OUTPUT_DIR/$platform/$lib_name"
    elif [ -f "lib$lib_name" ]; then
        cp "lib$lib_name" "$OUTPUT_DIR/$platform/"
        log_success "Built: $OUTPUT_DIR/$platform/lib$lib_name"
    else
        # Try to find the library
        local found_lib=$(find . -name "*.dylib" -o -name "*.so" 2>/dev/null | head -1)
        if [ -n "$found_lib" ]; then
            cp "$found_lib" "$OUTPUT_DIR/$platform/$lib_name"
            log_success "Built: $OUTPUT_DIR/$platform/$lib_name"
        else
            log_error "Could not find built library"
            exit 1
        fi
    fi

    # Print usage instructions
    echo ""
    log_success "Build complete!"
    echo ""
    log_info "To use the shim:"
    echo "  export LIBWEBRTC_SHIM_PATH=$OUTPUT_DIR/$platform/$lib_name"
    echo "  go test ./..."
}

# Main
main() {
    local libwebrtc_dir=""

    # Parse arguments
    case "${1:-}" in
        --help|-h)
            show_help
            ;;
        --fetch)
            fetch_libwebrtc
            ;;
        "")
            if [ -z "${LIBWEBRTC_DIR:-}" ]; then
                log_error "LIBWEBRTC_DIR not set"
                log_info "Set LIBWEBRTC_DIR environment variable or pass path as argument"
                log_info "Run '$0 --help' for more information"
                exit 1
            fi
            libwebrtc_dir="$LIBWEBRTC_DIR"
            ;;
        *)
            libwebrtc_dir="$1"
            ;;
    esac

    check_dependencies

    if ! verify_libwebrtc "$libwebrtc_dir"; then
        exit 1
    fi

    build_shim "$libwebrtc_dir"
}

main "$@"
