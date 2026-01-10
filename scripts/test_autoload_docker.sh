#!/bin/bash
#
# Test shim auto-download in a clean Docker environment.
#
# This script verifies that the library correctly auto-downloads
# its native dependencies from GitHub Releases when running in
# a fresh environment with no pre-installed shim.
#
# Usage:
#   ./scripts/test_autoload_docker.sh [test-package]
#
# Examples:
#   ./scripts/test_autoload_docker.sh                    # Run FFI tests
#   ./scripts/test_autoload_docker.sh ./pkg/encoder      # Run encoder tests
#   ./scripts/test_autoload_docker.sh ./...              # Run all tests
#
# Environment variables:
#   SHIM_FLAVOR     - basic (default) or custom
#   GO_VERSION      - Go version to use (default: 1.23)
#   VERBOSE         - Set to 1 for verbose output
#   TEST_OPENH264   - Set to 1 to run OpenH264 auto-download test
#   REQUIRE_SHIM    - Set to 1 to require shim load (default: 1)
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

TEST_PACKAGE="${1:-./internal/ffi}"
SHIM_FLAVOR="${SHIM_FLAVOR:-basic}"
GO_VERSION="${GO_VERSION:-1.25}"
VERBOSE="${VERBOSE:-0}"
TEST_OPENH264="${TEST_OPENH264:-0}"
REQUIRE_SHIM="${REQUIRE_SHIM:-1}"

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
    x86_64)  GOARCH="amd64" ;;
    aarch64) GOARCH="arm64" ;;
    arm64)   GOARCH="arm64" ;;
    *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

IMAGE_NAME="libgowebrtc-autoload-test"
CONTAINER_NAME="libgowebrtc-autoload-test-$$"

echo "==> Testing shim auto-download in clean Docker environment"
echo "    Package:  $TEST_PACKAGE"
echo "    Flavor:   $SHIM_FLAVOR"
echo "    Go:       $GO_VERSION"
echo "    Arch:     $GOARCH"
echo "    OpenH264: $TEST_OPENH264"
echo "    Require:  $REQUIRE_SHIM"
echo ""

# Build test image
cat > "$REPO_ROOT/.dockerignore.autoload" << 'EOF'
.git
lib/
shim/build/
*.tar.gz
.cache/
EOF

DOCKERFILE=$(cat << 'DOCKERFILE_END'
FROM golang:GO_VERSION_PLACEHOLDER-bookworm

# Refresh Debian archive keyring in case the base image is stale.
ADD https://deb.debian.org/debian/pool/main/d/debian-archive-keyring/debian-archive-keyring_2023.3+deb12u2_all.deb /tmp/debian-archive-keyring.deb
RUN dpkg -i /tmp/debian-archive-keyring.deb && rm -f /tmp/debian-archive-keyring.deb
RUN if [ -f /etc/apt/sources.list ]; then \
        sed -i 's|http://deb.debian.org|https://deb.debian.org|g' /etc/apt/sources.list; \
    fi && \
    if [ -d /etc/apt/sources.list.d ]; then \
        sed -i 's|http://deb.debian.org|https://deb.debian.org|g' /etc/apt/sources.list.d/*.list /etc/apt/sources.list.d/*.sources 2>/dev/null || true; \
    fi

# Install minimal runtime deps for the shim (fallback to insecure if apt metadata is broken).
RUN set -e; \
    APT_CACHE_DIR=/var/tmp/apt-cache; \
    mkdir -p "$APT_CACHE_DIR/partial"; \
    APT_OPTS="-o Dir::Cache::archives=$APT_CACHE_DIR -o Dir::Cache::archives/partial=$APT_CACHE_DIR/partial"; \
    if ! apt-get $APT_OPTS update; then \
        apt-get $APT_OPTS -o Acquire::AllowInsecureRepositories=true \
            -o Acquire::AllowDowngradeToInsecureRepositories=true update; \
    fi; \
    if ! apt-get $APT_OPTS install -y --no-install-recommends \
        ca-certificates \
        libx11-6 \
        libxcomposite1 \
        libxdamage1 \
        libxfixes3 \
        libxext6 \
        libxrandr2 \
        libxrender1 \
        libxtst6 \
        libglib2.0-0 \
        ; then \
        apt-get $APT_OPTS install -y --no-install-recommends --allow-unauthenticated \
            ca-certificates \
            libx11-6 \
            libxcomposite1 \
            libxdamage1 \
            libxfixes3 \
            libxext6 \
            libxrandr2 \
            libxrender1 \
            libxtst6 \
            libglib2.0-0 \
            ; \
    fi; \
    rm -rf /var/lib/apt/lists/* "$APT_CACHE_DIR"

WORKDIR /workspace

# Copy go.mod first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source (excluding lib/, shim/build/, *.tar.gz via .dockerignore)
COPY . .

# Ensure no pre-built shim exists
RUN rm -rf lib/ shim/build/ *.tar.gz ~/.libgowebrtc

# Environment for auto-download (CGO not required - using purego)
ENV CGO_ENABLED=0
ENV LIBWEBRTC_SHIM_FLAVOR=SHIM_FLAVOR_PLACEHOLDER
REQUIRE_SHIM_ENV_PLACEHOLDER
OPENH264_ENV_PLACEHOLDER

# Default: run tests (triggers auto-download)
CMD ["go", "test", "-v", "-count=1", "TEST_PACKAGE_PLACEHOLDER"]
DOCKERFILE_END
)

OPENH264_ENV=""
if [ "$TEST_OPENH264" = "1" ]; then
    OPENH264_ENV=$'ENV LIBWEBRTC_TEST_OPENH264_DOWNLOAD=1\nENV LIBWEBRTC_PREFER_SOFTWARE_CODECS=1'
fi

REQUIRE_SHIM_ENV=""
if [ "$REQUIRE_SHIM" = "1" ]; then
    REQUIRE_SHIM_ENV=$'ENV LIBWEBRTC_TEST_REQUIRE_SHIM=1'
fi

# Replace placeholders
DOCKERFILE="${DOCKERFILE//GO_VERSION_PLACEHOLDER/$GO_VERSION}"
DOCKERFILE="${DOCKERFILE//SHIM_FLAVOR_PLACEHOLDER/$SHIM_FLAVOR}"
DOCKERFILE="${DOCKERFILE//TEST_PACKAGE_PLACEHOLDER/$TEST_PACKAGE}"
DOCKERFILE="${DOCKERFILE//OPENH264_ENV_PLACEHOLDER/$OPENH264_ENV}"
DOCKERFILE="${DOCKERFILE//REQUIRE_SHIM_ENV_PLACEHOLDER/$REQUIRE_SHIM_ENV}"

echo "$DOCKERFILE" > "$REPO_ROOT/Dockerfile.autoload"

cleanup() {
    rm -f "$REPO_ROOT/Dockerfile.autoload" "$REPO_ROOT/.dockerignore.autoload"
    docker rm -f "$CONTAINER_NAME" 2>/dev/null || true
}
trap cleanup EXIT

echo "==> Building test image..."
docker build \
    --platform "linux/$GOARCH" \
    -f "$REPO_ROOT/Dockerfile.autoload" \
    -t "$IMAGE_NAME" \
    "$REPO_ROOT"

echo ""
echo "==> Running tests (shim will auto-download)..."
echo "----------------------------------------"

DOCKER_RUN_ARGS=(
    --rm
    --name "$CONTAINER_NAME"
    --platform "linux/$GOARCH"
)

if [ "$VERBOSE" = "1" ]; then
    DOCKER_RUN_ARGS+=(-e "VERBOSE=1")
fi

if docker run "${DOCKER_RUN_ARGS[@]}" "$IMAGE_NAME"; then
    echo "----------------------------------------"
    echo ""
    echo "==> SUCCESS: Auto-download and tests passed!"
    exit 0
else
    echo "----------------------------------------"
    echo ""
    echo "==> FAILED: Tests failed"
    exit 1
fi
