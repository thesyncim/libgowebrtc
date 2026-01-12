#!/bin/bash
#
# Build libc++ with Chromium's __Cr namespace for Linux
#
# This builds LLVM's libc++ with _LIBCPP_ABI_NAMESPACE=__Cr to be
# ABI-compatible with Chromium's custom libc++ used in libwebrtc.
#
# Usage:
#   ./scripts/build-libcxx.sh [--install-dir DIR]
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# LLVM version to use (should be compatible with Chromium's version)
LLVM_VERSION="${LLVM_VERSION:-18.1.8}"
INSTALL_DIR="${LIBCXX_INSTALL_DIR:-$PROJECT_ROOT/libcxx-cr}"

# Colors
log_info()    { echo -e "\033[0;34m[INFO]\033[0m $1"; }
log_success() { echo -e "\033[0;32m[SUCCESS]\033[0m $1"; }
log_error()   { echo -e "\033[0;31m[ERROR]\033[0m $1"; }
log_step()    { echo -e "\n\033[0;32m==>\033[0m \033[0;34m$1\033[0m"; }

# Parse arguments
while [[ $# -gt 0 ]]; do
    case "$1" in
        --install-dir)
            INSTALL_DIR="$2"
            shift 2
            ;;
        --llvm-version)
            LLVM_VERSION="$2"
            shift 2
            ;;
        *)
            log_error "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Check if already built
if [[ -f "$INSTALL_DIR/lib/libc++.so.1" ]]; then
    log_info "libc++ with __Cr namespace already built at $INSTALL_DIR"
    exit 0
fi

log_step "Building libc++ with __Cr namespace (Chromium ABI)"
log_info "LLVM version: $LLVM_VERSION"
log_info "Install directory: $INSTALL_DIR"

# Create build directory
BUILD_DIR=$(mktemp -d)
trap "rm -rf $BUILD_DIR" EXIT

cd "$BUILD_DIR"

# Download LLVM source
log_step "Downloading LLVM $LLVM_VERSION source"
LLVM_URL="https://github.com/llvm/llvm-project/releases/download/llvmorg-${LLVM_VERSION}/llvm-project-${LLVM_VERSION}.src.tar.xz"
log_info "URL: $LLVM_URL"

if ! curl -fSL --progress-bar -o llvm.tar.xz "$LLVM_URL"; then
    log_error "Failed to download LLVM source"
    exit 1
fi

log_info "Extracting..."
tar -xf llvm.tar.xz
cd "llvm-project-${LLVM_VERSION}.src"

# Create build directory for libc++
mkdir -p build-libcxx
cd build-libcxx

log_step "Configuring libc++ build"

# Configure with CMake
# Key flags:
# - _LIBCPP_ABI_NAMESPACE=__Cr : Use Chromium's namespace
# - LIBCXX_ABI_VERSION=1 : Standard ABI version
# - BUILD_SHARED_LIBS=ON : Build shared library
cmake ../runtimes \
    -DCMAKE_BUILD_TYPE=Release \
    -DCMAKE_INSTALL_PREFIX="$INSTALL_DIR" \
    -DLLVM_ENABLE_RUNTIMES="libcxx;libcxxabi" \
    -DLIBCXX_CXX_ABI=libcxxabi \
    -DLIBCXX_ABI_VERSION=1 \
    -DLIBCXX_ABI_NAMESPACE=__Cr \
    -DLIBCXX_ENABLE_SHARED=ON \
    -DLIBCXX_ENABLE_STATIC=OFF \
    -DLIBCXXABI_ENABLE_SHARED=ON \
    -DLIBCXXABI_ENABLE_STATIC=OFF \
    -DLIBCXX_INCLUDE_TESTS=OFF \
    -DLIBCXX_INCLUDE_BENCHMARKS=OFF \
    -DLIBCXXABI_INCLUDE_TESTS=OFF \
    -DCMAKE_C_COMPILER=clang \
    -DCMAKE_CXX_COMPILER=clang++

log_step "Building libc++"
cmake --build . --parallel $(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4)

log_step "Installing libc++"
cmake --install .

# Verify installation
if [[ -f "$INSTALL_DIR/lib/libc++.so.1" ]] && [[ -f "$INSTALL_DIR/lib/libc++abi.so.1" ]]; then
    log_success "libc++ with __Cr namespace built successfully!"
    log_info "Libraries installed to: $INSTALL_DIR/lib"
    ls -la "$INSTALL_DIR/lib/"*.so* 2>/dev/null || true

    # Verify the namespace
    log_info "Verifying __Cr namespace in symbols:"
    nm -D "$INSTALL_DIR/lib/libc++.so.1" 2>/dev/null | grep "__Cr" | head -5 || true
else
    log_error "Build failed - libc++.so not found"
    exit 1
fi

log_info ""
log_info "To use with libwebrtc shim on Linux:"
log_info "  export LD_LIBRARY_PATH=$INSTALL_DIR/lib:\$LD_LIBRARY_PATH"
log_info "  # or set RPATH during shim build"
