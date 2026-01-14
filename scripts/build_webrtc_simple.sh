#!/bin/bash
#
# Simple libwebrtc builder - run this directly in your terminal
#
set -e

ARCH="${1:-arm64}"  # arm64 or x64
PLATFORM="linux_${ARCH/x64/amd64}"
BRANCH="branch-heads/7390"

echo "Building libwebrtc for $PLATFORM..."
echo "This will take 1-2 hours."

# Create output directory
mkdir -p ~/libwebrtc-builds

# Build in Docker (no -it for background runs)
# Use Ubuntu 24.04 for newer GCC with ARM v9 support
docker run --rm \
  -v ~/libwebrtc-builds:/output \
  ubuntu:24.04 bash -c "
set -e
export DEBIAN_FRONTEND=noninteractive

echo '==> Installing dependencies...'
apt-get update -qq
apt-get install -y -qq git python3 curl wget lsb-release sudo pkg-config ninja-build rsync xz-utils \
  libx11-dev libxcomposite-dev libxdamage-dev libxext-dev libxfixes-dev libxrandr-dev \
  libxrender-dev libxtst-dev libglib2.0-dev libgbm-dev libdrm-dev libpulse-dev \
  libasound2-dev libgtk-3-dev libcups2-dev libnss3-dev libpci-dev libdbus-1-dev

echo '==> Setting up depot_tools...'
git clone --depth 1 https://chromium.googlesource.com/chromium/tools/depot_tools.git /depot_tools
export PATH=/depot_tools:\$PATH

echo '==> Fetching WebRTC (this takes a while)...'
mkdir -p /build && cd /build
fetch --nohooks webrtc
cd src
git checkout $BRANCH || (git fetch origin $BRANCH && git checkout FETCH_HEAD)
gclient sync -D --force --reset

echo '==> Installing WebRTC build deps...'
./build/install-build-deps.sh --no-prompt || true

echo '==> Installing GCC...'
apt-get install -y -qq g++ gcc

echo '==> Configuring build with GCC (not clang) for compatibility...'
gn gen out/Release --args='
  target_os=\"linux\"
  target_cpu=\"$ARCH\"
  is_debug=false
  is_clang=false
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
  arm_use_neon=true
  libyuv_disable_sme=true
'

echo '==> Building (this is the slow part)...'
ninja -C out/Release webrtc

echo '==> Packaging...'
mkdir -p /output/libwebrtc-$PLATFORM/lib
mkdir -p /output/libwebrtc-$PLATFORM/include
cp out/Release/obj/libwebrtc.a /output/libwebrtc-$PLATFORM/lib/

for dir in api modules rtc_base call media pc p2p; do
  [ -d \"\$dir\" ] && rsync -a --include='*/' --include='*.h' --include='*.inc' --exclude='*' \"\$dir/\" \"/output/libwebrtc-$PLATFORM/include/\$dir/\"
done
mkdir -p /output/libwebrtc-$PLATFORM/include/third_party/abseil-cpp
rsync -a --include='*/' --include='*.h' --include='*.inc' --exclude='*' third_party/abseil-cpp/ /output/libwebrtc-$PLATFORM/include/third_party/abseil-cpp/

cd /output
tar -cJf libwebrtc-$PLATFORM.tar.xz libwebrtc-$PLATFORM
ls -lh libwebrtc-$PLATFORM.tar.xz

echo '==> Verifying no __Cr symbols...'
if nm libwebrtc-$PLATFORM/lib/libwebrtc.a 2>/dev/null | grep -q 'std::__Cr'; then
  echo 'WARNING: Still has __Cr symbols!'
else
  echo 'OK: No __Cr symbols - uses libstdc++'
fi

echo '==> DONE!'
"

echo ""
echo "Output: ~/libwebrtc-builds/libwebrtc-$PLATFORM.tar.xz"
ls -lh ~/libwebrtc-builds/libwebrtc-$PLATFORM.tar.xz
