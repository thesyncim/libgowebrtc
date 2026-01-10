#!/bin/bash
#
# Build Linux shims for release using Docker.
#
# This script builds libwebrtc_shim for Linux (amd64 and/or arm64) using Docker.
# It can either use pre-built libwebrtc or build it from source.
#
# Usage:
#   # Build everything from source (reproducible, but slow ~2-3 hours per arch):
#   ./scripts/build_linux_shims.sh
#
#   # With pre-built libwebrtc (faster):
#   LIBWEBRTC_LINUX_AMD64=/path/to/linux_amd64/libwebrtc ./scripts/build_linux_shims.sh
#   LIBWEBRTC_LINUX_ARM64=/path/to/linux_arm64/libwebrtc ./scripts/build_linux_shims.sh
#
#   # Build only specific architecture:
#   TARGET_ARCHS="amd64" ./scripts/build_linux_shims.sh
#
# Environment variables:
#   LIBWEBRTC_LINUX_AMD64 - Path to pre-built libwebrtc for linux/amd64
#   LIBWEBRTC_LINUX_ARM64 - Path to pre-built libwebrtc for linux/arm64
#   RELEASE_TAG           - GitHub release tag (default: shim-v0.2.0)
#   TARGET_ARCHS          - Space-separated list of archs (default: "amd64 arm64")
#   WEBRTC_BRANCH         - WebRTC branch to build (default: branch-heads/7390)
#   SKIP_UPLOAD           - Set to 1 to skip uploading to GitHub
#   PARALLEL              - Set to 1 to build architectures in parallel
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

RELEASE_TAG="${RELEASE_TAG:-shim-v0.2.0}"
TARGET_ARCHS="${TARGET_ARCHS:-amd64 arm64}"
WEBRTC_BRANCH="${WEBRTC_BRANCH:-branch-heads/7390}"
PARALLEL="${PARALLEL:-0}"

IMAGE_BASE="libgowebrtc-shim-builder"

echo "============================================================"
echo " Linux Shim Builder"
echo "============================================================"
echo " Release:      ${RELEASE_TAG}"
echo " Architectures: ${TARGET_ARCHS}"
echo " WebRTC Branch: ${WEBRTC_BRANCH}"
echo "============================================================"
echo ""

# Function to build a single architecture
build_arch() {
    local arch="$1"
    local image_name="${IMAGE_BASE}-${arch}"
    local platform="linux_${arch}"
    local logfile="${REPO_ROOT}/build_${arch}.log"

    echo "[${arch}] Starting build..."

    # Determine libwebrtc source
    local skip_libwebrtc_build="0"
    local libwebrtc_path=""

    if [ "$arch" = "amd64" ] && [ -n "${LIBWEBRTC_LINUX_AMD64:-}" ]; then
        libwebrtc_path="${LIBWEBRTC_LINUX_AMD64}"
        skip_libwebrtc_build="1"
        echo "[${arch}] Using pre-built libwebrtc from ${libwebrtc_path}"
    elif [ "$arch" = "arm64" ] && [ -n "${LIBWEBRTC_LINUX_ARM64:-}" ]; then
        libwebrtc_path="${LIBWEBRTC_LINUX_ARM64}"
        skip_libwebrtc_build="1"
        echo "[${arch}] Using pre-built libwebrtc from ${libwebrtc_path}"
    else
        echo "[${arch}] Building libwebrtc from source (this will take 2-3 hours)..."
    fi

    # Build Docker image
    local build_args=(
        --platform "linux/${arch}"
        --build-arg "TARGETARCH=${arch}"
        --build-arg "WEBRTC_BRANCH=${WEBRTC_BRANCH}"
    )

    if [ "$skip_libwebrtc_build" = "1" ]; then
        build_args+=(--build-arg "SKIP_LIBWEBRTC_BUILD=1")
    else
        # Set target CPU for libwebrtc build
        if [ "$arch" = "arm64" ]; then
            build_args+=(--build-arg "TARGET_CPU=arm64")
        else
            build_args+=(--build-arg "TARGET_CPU=x64")
        fi
    fi

    echo "[${arch}] Building Docker image (logging to ${logfile})..."
    if ! docker build "${build_args[@]}" -t "$image_name" -f "$REPO_ROOT/Dockerfile" "$REPO_ROOT" > "$logfile" 2>&1; then
        echo "[${arch}] ERROR: Docker build failed. Check ${logfile}"
        return 1
    fi

    # If using pre-built libwebrtc, run container with mount and rebuild shim
    if [ "$skip_libwebrtc_build" = "1" ]; then
        echo "[${arch}] Building shim with mounted libwebrtc..."

        if ! docker run --rm \
            --platform "linux/${arch}" \
            -v "${libwebrtc_path}:/opt/libwebrtc:ro" \
            -v "${REPO_ROOT}/shim:/workspace/shim:ro" \
            -v "${REPO_ROOT}/lib:/workspace/lib" \
            "$image_name" \
            /bin/bash -c "
                mkdir -p /tmp/shim-build && cd /tmp/shim-build && \
                cmake /workspace/shim \
                    -DCMAKE_BUILD_TYPE=Release \
                    -DLIBWEBRTC_DIR=/opt/libwebrtc \
                    -DBUILD_SHARED_LIBS=ON \
                    -DCMAKE_C_COMPILER=clang \
                    -DCMAKE_CXX_COMPILER=clang++ \
                    -DCMAKE_SHARED_LINKER_FLAGS='-fuse-ld=lld' && \
                cmake --build . --config Release -j\$(nproc) && \
                mkdir -p /workspace/lib/linux_${arch} && \
                cp libwebrtc_shim.so /workspace/lib/linux_${arch}/
            " >> "$logfile" 2>&1; then
            echo "[${arch}] ERROR: Shim build failed. Check ${logfile}"
            return 1
        fi
    else
        # Extract built shim from image
        echo "[${arch}] Extracting shim from Docker image..."
        mkdir -p "$REPO_ROOT/lib/${platform}"

        local container_id
        container_id=$(docker create --platform "linux/${arch}" "$image_name" /bin/true)
        if ! docker cp "${container_id}:/workspace/lib/${platform}/libwebrtc_shim.so" "$REPO_ROOT/lib/${platform}/" 2>> "$logfile"; then
            echo "[${arch}] ERROR: Failed to extract shim. Check ${logfile}"
            docker rm "$container_id" >/dev/null 2>&1 || true
            return 1
        fi
        docker rm "$container_id" >/dev/null 2>&1 || true
    fi

    # Package for release
    echo "[${arch}] Packaging..."
    TARGET_PLATFORM="${platform}" SKIP_BUILD=1 RELEASE_TAG="${RELEASE_TAG}" \
        "$REPO_ROOT/scripts/release_shim.sh" 2>&1 | tee -a "$logfile"

    echo "[${arch}] Build complete!"
    return 0
}

# Track results using simple variables (bash 3.2 compatible)
build_result_amd64="skipped"
build_result_arm64="skipped"

set_result() {
    local arch="$1"
    local result="$2"
    case "$arch" in
        amd64) build_result_amd64="$result" ;;
        arm64) build_result_arm64="$result" ;;
    esac
}

get_result() {
    local arch="$1"
    case "$arch" in
        amd64) echo "$build_result_amd64" ;;
        arm64) echo "$build_result_arm64" ;;
        *) echo "skipped" ;;
    esac
}

# Build each architecture
if [ "$PARALLEL" = "1" ]; then
    echo "==> Building architectures in parallel..."
    pids=""
    archs_list=""
    for arch in $TARGET_ARCHS; do
        build_arch "$arch" &
        pids="$pids $!"
        archs_list="$archs_list $arch"
    done

    # Wait for all builds
    i=1
    for pid in $pids; do
        arch=$(echo $archs_list | cut -d' ' -f$i)
        if wait "$pid"; then
            set_result "$arch" "success"
        else
            set_result "$arch" "failed"
        fi
        i=$((i+1))
    done
else
    for arch in $TARGET_ARCHS; do
        if build_arch "$arch"; then
            set_result "$arch" "success"
        else
            set_result "$arch" "failed"
        fi
    done
fi

echo ""
echo "============================================================"
echo " Build Summary"
echo "============================================================"
for arch in $TARGET_ARCHS; do
    status=$(get_result "$arch")
    if [ "$status" = "success" ]; then
        echo " [OK] linux_${arch}"
    else
        echo " [FAIL] linux_${arch} - ${status}"
    fi
done
echo "============================================================"
echo ""

echo "==> Build complete!"
echo ""

# Upload to GitHub release
SKIP_UPLOAD="${SKIP_UPLOAD:-0}"
if [ "$SKIP_UPLOAD" != "1" ]; then
    echo "==> Uploading to GitHub release ${RELEASE_TAG}..."
    for arch in $TARGET_ARCHS; do
        platform="linux_${arch}"
        asset="${REPO_ROOT}/libwebrtc_shim_${platform}_basic.tar.gz"
        if [ -f "$asset" ]; then
            echo "    Uploading ${platform}..."
            gh release upload "${RELEASE_TAG}" "$asset" --clobber || {
                echo "    ERROR: Failed to upload ${platform}"
                echo "    Run manually: gh release upload ${RELEASE_TAG} \"$asset\""
            }
        fi
    done
    echo ""
    echo "==> Published to GitHub!"
else
    echo "SKIP_UPLOAD=1 set, skipping GitHub upload."
    echo "Upload manually:"
    for arch in $TARGET_ARCHS; do
        platform="linux_${arch}"
        asset="${REPO_ROOT}/libwebrtc_shim_${platform}_basic.tar.gz"
        if [ -f "$asset" ]; then
            echo "   gh release upload ${RELEASE_TAG} \"$asset\""
        fi
    done
fi

echo ""
echo "==> Don't forget to update internal/ffi/shim_manifest.json with the SHA256 hashes printed above!"
