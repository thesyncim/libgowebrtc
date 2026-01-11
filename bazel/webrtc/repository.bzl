"""Repository rule for libwebrtc.

Uses pre-built libwebrtc if LIBWEBRTC_DIR is set, otherwise builds from source.
"""

_BUILD_FILE_CONTENT = """
package(default_visibility = ["//visibility:public"])

cc_library(
    name = "libwebrtc",
    hdrs = glob(["include/**/*.h", "include/**/*.inc"]),
    srcs = glob(["lib/*.a"]),  # All static libs (libwebrtc.a + any codec libs if present)
    includes = [
        "include",
        "include/third_party/abseil-cpp",
        "include/third_party/boringssl/src/include",
        "include/third_party/libyuv/include",
    ],
    defines = [
        "WEBRTC_POSIX",
    ] + select({
        "@platforms//os:macos": ["WEBRTC_MAC"],
        "@platforms//os:linux": ["WEBRTC_LINUX"],
        "//conditions:default": [],
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
)
"""

def _libwebrtc_repository_impl(rctx):
    """Uses pre-built libwebrtc from LIBWEBRTC_DIR or ~/libwebrtc."""

    # Check for pre-built libwebrtc
    libwebrtc_dir = rctx.os.environ.get("LIBWEBRTC_DIR", "")

    if not libwebrtc_dir:
        # Default locations to check
        home = rctx.os.environ.get("HOME", "")
        default_paths = [
            home + "/libwebrtc",
            "/usr/local/libwebrtc",
        ]
        for path in default_paths:
            result = rctx.execute(["test", "-f", path + "/lib/libwebrtc.a"])
            if result.return_code == 0:
                libwebrtc_dir = path
                break

    if not libwebrtc_dir:
        fail("""
libwebrtc not found. Please either:
1. Set LIBWEBRTC_DIR environment variable to your libwebrtc installation
2. Build libwebrtc and install it to ~/libwebrtc

To build libwebrtc, you can use the old script (if available) or:
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
