"""Module extension for libwebrtc."""

load("//bazel/webrtc:repository.bzl", "libwebrtc_repository")

def _libwebrtc_impl(mctx):
    libwebrtc_repository(name = "libwebrtc")

libwebrtc = module_extension(
    implementation = _libwebrtc_impl,
)
