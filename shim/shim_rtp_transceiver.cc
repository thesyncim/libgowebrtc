/*
 * shim_rtp_transceiver.cc - RTPTransceiver implementation
 *
 * Provides transceiver direction management and state queries.
 */

#include "shim_common.h"

#include <cstring>

#include "api/rtp_transceiver_interface.h"

extern "C" {

SHIM_EXPORT int shim_transceiver_get_direction(ShimRTPTransceiver* transceiver) {
    if (!transceiver) return SHIM_TRANSCEIVER_DIRECTION_INACTIVE;
    auto t = reinterpret_cast<webrtc::RtpTransceiverInterface*>(transceiver);

    switch (t->direction()) {
        case webrtc::RtpTransceiverDirection::kSendRecv: return SHIM_TRANSCEIVER_DIRECTION_SENDRECV;
        case webrtc::RtpTransceiverDirection::kSendOnly: return SHIM_TRANSCEIVER_DIRECTION_SENDONLY;
        case webrtc::RtpTransceiverDirection::kRecvOnly: return SHIM_TRANSCEIVER_DIRECTION_RECVONLY;
        case webrtc::RtpTransceiverDirection::kInactive: return SHIM_TRANSCEIVER_DIRECTION_INACTIVE;
        case webrtc::RtpTransceiverDirection::kStopped: return SHIM_TRANSCEIVER_DIRECTION_STOPPED;
        default: return SHIM_TRANSCEIVER_DIRECTION_INACTIVE;
    }
}

SHIM_EXPORT int shim_transceiver_set_direction(ShimRTPTransceiver* transceiver, int direction) {
    if (!transceiver) return SHIM_ERROR_INVALID_PARAM;
    auto t = reinterpret_cast<webrtc::RtpTransceiverInterface*>(transceiver);

    webrtc::RtpTransceiverDirection dir;
    switch (direction) {
        case SHIM_TRANSCEIVER_DIRECTION_SENDRECV: dir = webrtc::RtpTransceiverDirection::kSendRecv; break;
        case SHIM_TRANSCEIVER_DIRECTION_SENDONLY: dir = webrtc::RtpTransceiverDirection::kSendOnly; break;
        case SHIM_TRANSCEIVER_DIRECTION_RECVONLY: dir = webrtc::RtpTransceiverDirection::kRecvOnly; break;
        case SHIM_TRANSCEIVER_DIRECTION_INACTIVE: dir = webrtc::RtpTransceiverDirection::kInactive; break;
        default: return SHIM_ERROR_INVALID_PARAM;
    }

    auto result = t->SetDirectionWithError(dir);
    return result.ok() ? SHIM_OK : SHIM_ERROR_INVALID_PARAM;
}

SHIM_EXPORT int shim_transceiver_get_current_direction(ShimRTPTransceiver* transceiver) {
    if (!transceiver) return SHIM_TRANSCEIVER_DIRECTION_INACTIVE;
    auto t = reinterpret_cast<webrtc::RtpTransceiverInterface*>(transceiver);
    auto dir = t->current_direction();

    if (!dir.has_value()) return SHIM_TRANSCEIVER_DIRECTION_INACTIVE;

    switch (dir.value()) {
        case webrtc::RtpTransceiverDirection::kSendRecv: return SHIM_TRANSCEIVER_DIRECTION_SENDRECV;
        case webrtc::RtpTransceiverDirection::kSendOnly: return SHIM_TRANSCEIVER_DIRECTION_SENDONLY;
        case webrtc::RtpTransceiverDirection::kRecvOnly: return SHIM_TRANSCEIVER_DIRECTION_RECVONLY;
        case webrtc::RtpTransceiverDirection::kInactive: return SHIM_TRANSCEIVER_DIRECTION_INACTIVE;
        case webrtc::RtpTransceiverDirection::kStopped: return SHIM_TRANSCEIVER_DIRECTION_STOPPED;
        default: return SHIM_TRANSCEIVER_DIRECTION_INACTIVE;
    }
}

SHIM_EXPORT int shim_transceiver_stop(ShimRTPTransceiver* transceiver) {
    if (!transceiver) return SHIM_ERROR_INVALID_PARAM;
    auto t = reinterpret_cast<webrtc::RtpTransceiverInterface*>(transceiver);
    auto result = t->StopStandard();
    return result.ok() ? SHIM_OK : SHIM_ERROR_INIT_FAILED;
}

SHIM_EXPORT const char* shim_transceiver_mid(ShimRTPTransceiver* transceiver) {
    if (!transceiver) return "";
    auto t = reinterpret_cast<webrtc::RtpTransceiverInterface*>(transceiver);
    auto mid = t->mid();

    if (!mid.has_value()) return "";

    // Use thread-local storage for returned string
    static thread_local std::string mid_buffer;
    mid_buffer = mid.value();
    return mid_buffer.c_str();
}

SHIM_EXPORT ShimRTPSender* shim_transceiver_get_sender(ShimRTPTransceiver* transceiver) {
    if (!transceiver) return nullptr;
    auto t = reinterpret_cast<webrtc::RtpTransceiverInterface*>(transceiver);
    return reinterpret_cast<ShimRTPSender*>(t->sender().get());
}

SHIM_EXPORT ShimRTPReceiver* shim_transceiver_get_receiver(ShimRTPTransceiver* transceiver) {
    if (!transceiver) return nullptr;
    auto t = reinterpret_cast<webrtc::RtpTransceiverInterface*>(transceiver);
    return reinterpret_cast<ShimRTPReceiver*>(t->receiver().get());
}

}  // extern "C"
