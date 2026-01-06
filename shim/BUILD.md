# Building libwebrtc_shim

This document describes how to build the `libwebrtc_shim` library that wraps Google's libwebrtc with a C interface for use with Go via purego.

## Prerequisites

- CMake 3.16+
- Ninja (recommended) or Make
- C++17 compatible compiler (Clang 12+ or GCC 11+)
- Pre-built libwebrtc static library

### Installing Build Tools

**macOS:**
```bash
brew install cmake ninja
```

**Ubuntu/Debian:**
```bash
sudo apt-get install cmake ninja-build build-essential
```

## Quick Build

If you have libwebrtc already built:

```bash
cd shim
LIBWEBRTC_DIR=/path/to/libwebrtc ./build.sh
```

The built library will be placed in `lib/<platform>/libwebrtc_shim.dylib` (or `.so` on Linux).

## Getting libwebrtc

libwebrtc is Google's WebRTC implementation. There are several ways to obtain it:

### Option 1: Build from Source (Recommended)

Building from source gives you the most control and ensures compatibility.

#### 1. Install depot_tools

```bash
git clone https://chromium.googlesource.com/chromium/tools/depot_tools.git
export PATH="$PATH:$(pwd)/depot_tools"
```

#### 2. Fetch WebRTC Source

```bash
mkdir webrtc-checkout && cd webrtc-checkout
fetch --nohooks webrtc
gclient sync
```

This downloads ~20GB and takes significant time.

#### 3. Generate Build Files

```bash
cd src

# For macOS arm64 Release build
gn gen out/Release-arm64 --args='
  target_os="mac"
  target_cpu="arm64"
  is_debug=false
  is_component_build=false
  rtc_include_tests=false
  rtc_build_examples=false
  rtc_build_tools=false
  use_rtti=true
  treat_warnings_as_errors=false
'

# For macOS x86_64 Release build
gn gen out/Release-x64 --args='
  target_os="mac"
  target_cpu="x64"
  is_debug=false
  is_component_build=false
  rtc_include_tests=false
  rtc_build_examples=false
  rtc_build_tools=false
  use_rtti=true
  treat_warnings_as_errors=false
'

# For Linux x86_64
gn gen out/Release --args='
  target_os="linux"
  target_cpu="x64"
  is_debug=false
  is_component_build=false
  rtc_include_tests=false
  rtc_build_examples=false
  rtc_build_tools=false
  use_rtti=true
  treat_warnings_as_errors=false
'
```

#### 4. Build

```bash
ninja -C out/Release-arm64  # or appropriate directory
```

This produces `out/Release-arm64/obj/libwebrtc.a`.

#### 5. Create Install Directory

```bash
# Create installation directory structure
INSTALL_DIR=/opt/libwebrtc

sudo mkdir -p $INSTALL_DIR/{include,lib}

# Copy library
sudo cp out/Release-arm64/obj/libwebrtc.a $INSTALL_DIR/lib/

# Copy headers (preserving directory structure)
# This copies all .h files while maintaining the directory tree
sudo rsync -avm --include='*.h' --include='*/' --exclude='*' \
  ./ $INSTALL_DIR/include/

# Also copy third_party headers needed for abseil, boringssl, libyuv
sudo rsync -avm --include='*.h' --include='*/' --exclude='*' \
  third_party/abseil-cpp/ $INSTALL_DIR/include/third_party/abseil-cpp/
sudo rsync -avm --include='*.h' --include='*/' --exclude='*' \
  third_party/boringssl/src/include/ $INSTALL_DIR/include/third_party/boringssl/src/include/
sudo rsync -avm --include='*.h' --include='*/' --exclude='*' \
  third_party/libyuv/include/ $INSTALL_DIR/include/third_party/libyuv/include/
```

### Option 2: Community Pre-built Binaries

Some community projects maintain pre-built libwebrtc binaries:

- **aspect-build**: https://github.com/aspect-build/aspect-cli-plugin-webrtc/releases
- **aspect-build libwebrtc**: Pre-built for multiple platforms

Download and extract to a directory, then point `LIBWEBRTC_DIR` to it.

### Option 3: Use vcpkg (Windows/Linux)

```bash
vcpkg install libwebrtc
```

Note: vcpkg version may be outdated.

## Building the Shim

Once you have libwebrtc:

### Using the Build Script

```bash
cd shim

# Set the path to your libwebrtc installation
export LIBWEBRTC_DIR=/opt/libwebrtc

# Build
./build.sh

# Or pass the path directly
./build.sh /path/to/libwebrtc
```

### Manual CMake Build

```bash
cd shim
mkdir build && cd build

cmake .. \
  -DLIBWEBRTC_DIR=/opt/libwebrtc \
  -DCMAKE_BUILD_TYPE=Release \
  -DBUILD_SHARED_LIBS=ON \
  -G Ninja

ninja
```

### Build Options

| Option | Default | Description |
|--------|---------|-------------|
| `LIBWEBRTC_DIR` | (required) | Path to libwebrtc installation |
| `BUILD_SHARED_LIBS` | ON | Build as shared library |
| `BUILD_TYPE` | Release | CMake build type |
| `ENABLE_DEVICE_CAPTURE` | ON | Enable camera/mic/screen capture |

## Output

After building, the library is placed in:

- macOS arm64: `lib/darwin_arm64/libwebrtc_shim.dylib`
- macOS x86_64: `lib/darwin_amd64/libwebrtc_shim.dylib`
- Linux x86_64: `lib/linux_amd64/libwebrtc_shim.so`
- Linux arm64: `lib/linux_arm64/libwebrtc_shim.so`

## Using the Shim

Set the environment variable to point to the built library:

```bash
export LIBWEBRTC_SHIM_PATH=/path/to/libgowebrtc/lib/darwin_arm64/libwebrtc_shim.dylib

# Run tests
go test ./...

# Run browser example
go run ./examples/camera_to_browser
```

## Troubleshooting

### Missing Headers

If you get "header not found" errors, ensure the include directory structure is correct:

```
$LIBWEBRTC_DIR/
├── include/
│   ├── api/
│   │   ├── video_codecs/
│   │   ├── audio_codecs/
│   │   └── ...
│   ├── rtc_base/
│   ├── modules/
│   └── third_party/
│       ├── abseil-cpp/
│       ├── boringssl/
│       └── libyuv/
└── lib/
    └── libwebrtc.a
```

### Undefined Symbols

If you get undefined symbol errors at runtime, the libwebrtc may have been built with incompatible settings. Ensure:

- `use_rtti=true` is set in GN args
- `is_component_build=false` for static linking
- Same C++ standard library (libc++ on macOS)

### Platform-Specific Issues

**macOS:**
- Requires Xcode Command Line Tools
- May need to disable hardened runtime for unsigned libraries
- Rosetta 2 allows running x86_64 binaries on arm64

**Linux:**
- Requires `libpulse-dev` for audio capture
- Requires `libx11-dev` for screen capture
- May need `libasound2-dev` for ALSA

## CI/CD Integration

For automated builds, you can:

1. Cache the libwebrtc build (it's large but doesn't change often)
2. Use the build script with environment variables
3. Archive the built shim as a build artifact

Example GitHub Actions:

```yaml
- name: Build shim
  env:
    LIBWEBRTC_DIR: ${{ github.workspace }}/libwebrtc
  run: |
    cd shim
    ./build.sh

- name: Upload shim artifact
  uses: actions/upload-artifact@v3
  with:
    name: libwebrtc_shim-${{ matrix.os }}
    path: lib/*/libwebrtc_shim.*
```

## Updating libwebrtc

When updating to a new libwebrtc version:

1. Check the WebRTC release notes for API changes
2. Update `kLibWebRTCVersion` in `shim.cc`
3. Test all functionality
4. Update this document if build process changes
