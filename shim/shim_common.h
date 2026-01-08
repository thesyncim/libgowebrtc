/*
 * shim_common.h - Shared declarations for libwebrtc_shim
 *
 * Contains global initialization, thread management, Environment,
 * and utility functions used across all shim modules.
 */

#ifndef SHIM_COMMON_H_
#define SHIM_COMMON_H_

#include "shim.h"

#include <memory>
#include <mutex>
#include <string>

// libwebrtc includes
#include "rtc_base/thread.h"
#include "api/environment/environment.h"
#include "api/environment/environment_factory.h"
#include "api/rtc_error.h"
#include "api/video_codecs/video_encoder.h"
#include "api/video_codecs/video_decoder.h"
#include "api/video_codecs/sdp_video_format.h"

namespace shim {

// Version strings
extern const char* kShimVersion;
extern const char* kLibWebRTCVersion;

// Initialize global resources (threads, etc.)
// Thread-safe, can be called multiple times.
void InitializeGlobals();

// Get the global Environment for M141+ API
const webrtc::Environment& GetEnvironment();

// Thread accessors (M141: Thread is now in webrtc namespace)
webrtc::Thread* GetSignalingThread();
webrtc::Thread* GetWorkerThread();
webrtc::Thread* GetNetworkThread();

// Codec type conversions
webrtc::VideoCodecType ToWebRTCCodecType(ShimCodecType codec);
std::string CodecTypeToString(ShimCodecType codec);

// SDP format helper
webrtc::SdpVideoFormat CreateSdpVideoFormat(ShimCodecType codec, const char* h264_profile = nullptr);

// ============================================================================
// Error Message Helpers
// ============================================================================

// Copy an error message to the ShimErrorBuffer.
// Returns the provided error_code for convenient chaining.
// If error_out is NULL, no message is written.
inline int SetErrorMessage(ShimErrorBuffer* error_out,
                           const std::string& message,
                           int error_code = SHIM_ERROR_INIT_FAILED) {
    if (error_out) {
        size_t copy_len = std::min(static_cast<size_t>(SHIM_MAX_ERROR_MSG_LEN - 1), message.size());
        memcpy(error_out->message, message.c_str(), copy_len);
        error_out->message[copy_len] = '\0';
    }
    return error_code;
}

// Copy an error message from webrtc::RTCError to the ShimErrorBuffer.
// If the RTCError has no message, falls back to the error type name.
// Returns the provided error_code for convenient chaining.
inline int SetErrorFromRTCError(ShimErrorBuffer* error_out,
                                 const webrtc::RTCError& error,
                                 int error_code = SHIM_ERROR_INIT_FAILED) {
    if (error_out) {
        std::string msg = error.message();
        if (msg.empty()) {
            // Fall back to error type name if no message
            msg = webrtc::ToString(error.type());
        }
        size_t copy_len = std::min(static_cast<size_t>(SHIM_MAX_ERROR_MSG_LEN - 1), msg.size());
        memcpy(error_out->message, msg.c_str(), copy_len);
        error_out->message[copy_len] = '\0';
    }
    return error_code;
}

// Clear the error buffer (set to empty string).
inline void ClearError(ShimErrorBuffer* error_out) {
    if (error_out) {
        error_out->message[0] = '\0';
    }
}

}  // namespace shim

#endif  // SHIM_COMMON_H_
