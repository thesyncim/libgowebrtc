/*
 * shim_stats.cc - Statistics and memory helpers
 *
 * Provides RTCStats collection and memory management functions.
 */

#include "shim_common.h"

#include <cstdlib>
#include <cstring>

extern "C" {

/* ============================================================================
 * Memory Helpers
 * ========================================================================== */

SHIM_EXPORT void shim_free_buffer(void* buffer) {
    free(buffer);
}

SHIM_EXPORT void shim_free_packets(void* packets, void* sizes, int count) {
    free(packets);
    free(sizes);
}

/* ============================================================================
 * Version API
 * ========================================================================== */

SHIM_EXPORT const char* shim_version(void) {
    return shim::kShimVersion;
}

SHIM_EXPORT const char* shim_libwebrtc_version(void) {
    return shim::kLibWebRTCVersion;
}

}  // extern "C"
