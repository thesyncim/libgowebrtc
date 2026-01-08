#!/bin/bash
#
# Build and package libwebrtc_shim for GitHub Releases.
#
# Usage:
#   LIBWEBRTC_DIR=/path/to/libwebrtc RELEASE_TAG=shim-v0.1.0 ./scripts/release_shim.sh
#
# Environment variables:
#   LIBWEBRTC_DIR   - Path to installed libwebrtc (include/ + lib/)
#   RELEASE_TAG     - GitHub release tag to publish to (required)
#   SHIM_FLAVOR     - basic (default) or h264
#   SKIP_BUILD      - If set to 1, skip shim build and just package
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

LIBWEBRTC_DIR="${LIBWEBRTC_DIR:-}"
RELEASE_TAG="${RELEASE_TAG:-}"
SHIM_FLAVOR="${SHIM_FLAVOR:-basic}"
SKIP_BUILD="${SKIP_BUILD:-0}"

detect_platform() {
    local os
    local arch
    os="$(uname -s)"
    arch="$(uname -m)"

    case "$os" in
        Darwin)
            case "$arch" in
                arm64) echo "darwin_arm64" ;;
                x86_64) echo "darwin_amd64" ;;
                *) echo "unsupported" ;;
            esac
            ;;
        Linux)
            case "$arch" in
                aarch64) echo "linux_arm64" ;;
                x86_64) echo "linux_amd64" ;;
                *) echo "unsupported" ;;
            esac
            ;;
        *)
            echo "unsupported"
            ;;
    esac
}

if [ -z "$RELEASE_TAG" ]; then
    echo "RELEASE_TAG is required"
    exit 1
fi

if [ "$SKIP_BUILD" != "1" ]; then
    if [ -z "$LIBWEBRTC_DIR" ]; then
        echo "LIBWEBRTC_DIR is required to build the shim"
        exit 1
    fi
    echo "Building shim using LIBWEBRTC_DIR=$LIBWEBRTC_DIR"
    LIBWEBRTC_DIR="$LIBWEBRTC_DIR" "$REPO_ROOT/shim/build.sh"
else
    echo "SKIP_BUILD=1 set; packaging only"
fi

platform="$(detect_platform)"
if [ "$platform" = "unsupported" ]; then
    echo "Unsupported platform for release packaging"
    exit 1
fi

case "$platform" in
    darwin_*) lib_name="libwebrtc_shim.dylib" ;;
    linux_*) lib_name="libwebrtc_shim.so" ;;
    *) echo "Unsupported platform $platform"; exit 1 ;;
esac

lib_path="$REPO_ROOT/lib/$platform/$lib_name"
if [ ! -f "$lib_path" ]; then
    echo "Shim library not found at $lib_path"
    exit 1
fi

asset_name="libwebrtc_shim_${platform}_${SHIM_FLAVOR}.tar.gz"
asset_path="$REPO_ROOT/$asset_name"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

cp "$lib_path" "$tmp_dir/"
if [ -f "$REPO_ROOT/LICENSE" ]; then
    cp "$REPO_ROOT/LICENSE" "$tmp_dir/"
fi
if [ -f "$REPO_ROOT/NOTICE" ]; then
    cp "$REPO_ROOT/NOTICE" "$tmp_dir/"
fi

tar -czf "$asset_path" -C "$tmp_dir" .

if command -v sha256sum >/dev/null 2>&1; then
    sha256="$(sha256sum "$asset_path" | awk '{print $1}')"
else
    sha256="$(shasum -a 256 "$asset_path" | awk '{print $1}')"
fi

echo ""
echo "Release asset: $asset_path"
echo "SHA256: $sha256"
echo ""
echo "Manifest entry:"
echo "  \"$platform\": { \"file\": \"$asset_name\", \"sha256\": \"$sha256\" }"
echo ""
echo "Upload command:"
echo "  gh release upload \"$RELEASE_TAG\" \"$asset_path\""
