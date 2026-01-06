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

// Note: shim_version and shim_libwebrtc_version are defined in shim_common.cc

}  // extern "C"
