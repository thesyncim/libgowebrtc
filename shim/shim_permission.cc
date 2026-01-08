/*
 * shim_permission.cc - Non-macOS permission stub implementation
 *
 * On Linux/Windows, permissions are typically handled at the system level
 * or not required for device access.
 */

#include "shim_common.h"

#if !defined(__APPLE__)

extern "C" {

SHIM_EXPORT int shim_check_camera_permission(void) {
    // On non-macOS platforms, assume authorized
    return 1;
}

SHIM_EXPORT int shim_check_microphone_permission(void) {
    return 1;
}

SHIM_EXPORT int shim_request_camera_permission(void) {
    return 1;
}

SHIM_EXPORT int shim_request_microphone_permission(void) {
    return 1;
}

}  // extern "C"

#endif  // !__APPLE__
