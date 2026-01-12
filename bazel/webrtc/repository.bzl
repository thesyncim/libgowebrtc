"""Repository rule for libwebrtc.

Uses pre-built libwebrtc if LIBWEBRTC_DIR is set, otherwise builds from source.
"""

_BUILD_FILE_CONTENT = """
package(default_visibility = ["//visibility:public"])

# Import the pre-built static library with alwayslink to ensure all symbols are included
cc_import(
    name = "libwebrtc_archive",
    static_library = select({
        "@platforms//os:windows": "lib/webrtc.lib",
        "//conditions:default": "lib/libwebrtc.a",
    }),
    alwayslink = True,  # Force --whole-archive behavior for this archive
)

cc_library(
    name = "libwebrtc",
    hdrs = glob(["include/**/*.h", "include/**/*.inc"]),
    includes = [
        "include",
        "include/third_party/abseil-cpp",
        "include/third_party/boringssl/src/include",
        "include/third_party/libyuv/include",
    ],
    defines = select({
        "@platforms//os:macos": ["WEBRTC_POSIX", "WEBRTC_MAC"],
        "@platforms//os:linux": ["WEBRTC_POSIX", "WEBRTC_LINUX"],
        "@platforms//os:windows": ["WEBRTC_WIN", "NOMINMAX", "WIN32_LEAN_AND_MEAN"],
        "//conditions:default": ["WEBRTC_POSIX"],
    }),
    linkopts = select({
        "@platforms//os:macos": [
            "-framework Foundation",
            "-framework AVFoundation",
            "-framework CoreAudio",
            "-framework CoreMedia",
            "-framework CoreVideo",
            "-framework VideoToolbox",
            "-framework AudioToolbox",
            "-framework CoreFoundation",
            "-framework CoreGraphics",
            "-framework IOSurface",
            "-framework Metal",
            "-framework AppKit",
            "-framework ScreenCaptureKit",
            "-framework ApplicationServices",
        ],
        "@platforms//os:linux": [
            "-lpthread",
            "-ldl",
            "-lrt",
        ],
        "//conditions:default": [],
    }),
    deps = [":libwebrtc_archive"],
)
"""

def _check_file_exists(rctx, path):
    """Check if a file exists, cross-platform."""
    # Try Python first (works on all platforms)
    result = rctx.execute(["python3", "-c", "import os; exit(0 if os.path.isfile('{}') else 1)".format(path)])
    if result.return_code == 0:
        return True
    # Fallback to test command (Unix)
    result = rctx.execute(["test", "-f", path])
    return result.return_code == 0

def _libwebrtc_repository_impl(rctx):
    """Uses pre-built libwebrtc from LIBWEBRTC_DIR or ~/libwebrtc."""

    # Check for pre-built libwebrtc
    libwebrtc_dir = rctx.os.environ.get("LIBWEBRTC_DIR", "")

    # Determine which lib file to look for based on OS
    is_windows = rctx.os.name.startswith("windows")
    lib_file = "webrtc.lib" if is_windows else "libwebrtc.a"

    if not libwebrtc_dir:
        # Default locations to check
        home = rctx.os.environ.get("HOME", "") or rctx.os.environ.get("USERPROFILE", "")
        default_paths = [
            home + "/libwebrtc",
            "/usr/local/libwebrtc",
        ]
        for path in default_paths:
            if _check_file_exists(rctx, path + "/lib/" + lib_file):
                libwebrtc_dir = path
                break

    if not libwebrtc_dir:
        fail("""
libwebrtc not found. Please either:
1. Set LIBWEBRTC_DIR environment variable to your libwebrtc installation
2. Run ./scripts/build.sh which will download pre-built binaries

For manual setup:
  git clone https://chromium.googlesource.com/chromium/tools/depot_tools.git ~/depot_tools
  export PATH=$HOME/depot_tools:$PATH
  mkdir -p ~/webrtc_build && cd ~/webrtc_build
  fetch webrtc
  cd src
  gn gen out/Release --args='is_debug=false rtc_include_tests=false use_rtti=true'
  ninja -C out/Release
""")

    print("Using pre-built libwebrtc from: " + libwebrtc_dir)

    # Create symlinks to the pre-built libwebrtc
    rctx.symlink(libwebrtc_dir + "/lib", "lib")
    rctx.symlink(libwebrtc_dir + "/include", "include")

    # Generate BUILD file
    rctx.file("BUILD.bazel", _BUILD_FILE_CONTENT)

libwebrtc_repository = repository_rule(
    implementation = _libwebrtc_repository_impl,
    environ = ["LIBWEBRTC_DIR", "HOME"],
    local = True,
)
