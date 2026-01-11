/*
 * shim_rtp_receiver.cc - RTPReceiver implementation
 *
 * RTP receiver functionality: track access, stats, and limited jitter buffer control.
 *
 * NOTE ON JITTER BUFFER:
 * libwebrtc only exposes SetJitterBufferMinimumDelay() via RtpReceiverInterface.
 * This sets a floor for the adaptive jitter buffer - the actual delay may be higher.
 * There is no API to:
 * - Set maximum delay
 * - Get jitter buffer statistics directly (use PeerConnection::GetStats() instead)
 * - Disable adaptive mode
 */

#include "shim_common.h"

#include <cstring>

#include "api/rtp_receiver_interface.h"

extern "C" {

/* ============================================================================
 * Jitter Buffer Control (Limited)
 * ========================================================================== */

SHIM_EXPORT int shim_rtp_receiver_set_jitter_buffer_min_delay(
    ShimRTPReceiver* receiver,
    int min_delay_ms
) {
    if (!receiver) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    auto webrtc_receiver = reinterpret_cast<webrtc::RtpReceiverInterface*>(receiver);

    if (min_delay_ms <= 0) {
        // Clear minimum delay - let libwebrtc's adaptive algorithm decide
        webrtc_receiver->SetJitterBufferMinimumDelay(std::nullopt);
    } else {
        // Set minimum delay floor (libwebrtc uses seconds as double)
        double delay_seconds = static_cast<double>(min_delay_ms) / 1000.0;
        webrtc_receiver->SetJitterBufferMinimumDelay(delay_seconds);
    }

    return SHIM_OK;
}

/* ============================================================================
 * RTPReceiver Track Access
 * ========================================================================== */

SHIM_EXPORT void* shim_rtp_receiver_get_track(ShimRTPReceiver* receiver) {
    if (!receiver) return nullptr;
    auto webrtc_receiver = reinterpret_cast<webrtc::RtpReceiverInterface*>(receiver);
    auto track = webrtc_receiver->track();
    return track.get();
}

/* ============================================================================
 * RTPReceiver Stats
 *
 * NOTE: For detailed receiver stats including jitter buffer info, use
 * PeerConnection::GetStats() which provides RTCInboundRtpStreamStats.
 * ========================================================================== */

SHIM_EXPORT int shim_rtp_receiver_get_stats(ShimRTPReceiver* receiver, ShimRTCStats* out_stats) {
    if (!receiver || !out_stats) return SHIM_ERROR_INVALID_PARAM;
    memset(out_stats, 0, sizeof(ShimRTCStats));
    // Stats come from PeerConnection::GetStats(), not directly from receiver
    // This function exists for API consistency but returns empty stats
    return SHIM_OK;
}

/* ============================================================================
 * RTCP Feedback
 * ========================================================================== */

SHIM_EXPORT int shim_rtp_receiver_request_keyframe(ShimRTPReceiver* receiver) {
    if (!receiver) return SHIM_ERROR_INVALID_PARAM;
    // libwebrtc does not expose a direct keyframe request on RtpReceiverInterface.
    return SHIM_ERROR_NOT_SUPPORTED;
}

}  // extern "C"
