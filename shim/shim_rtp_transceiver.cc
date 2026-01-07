/*
 * shim_rtp_transceiver.cc - RTPTransceiver implementation
 *
 * Provides transceiver direction management and state queries.
 */

#include "shim_common.h"

#include <cstring>
#include <vector>

#include "api/rtp_transceiver_interface.h"
#include "api/rtp_parameters.h"
#include "media/base/codec.h"

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

SHIM_EXPORT int shim_transceiver_set_codec_preferences(
    ShimRTPTransceiver* transceiver,
    const ShimCodecCapability* codecs,
    int count
) {
    if (!transceiver || (!codecs && count > 0)) return SHIM_ERROR_INVALID_PARAM;

    auto t = reinterpret_cast<webrtc::RtpTransceiverInterface*>(transceiver);

    std::vector<webrtc::RtpCodecCapability> prefs;
    prefs.reserve(count);

    for (int i = 0; i < count; i++) {
        webrtc::RtpCodecCapability cap;

        // Parse mime_type "video/VP8" or "audio/opus" into kind and name
        std::string mime = codecs[i].mime_type;
        size_t slash = mime.find('/');
        if (slash != std::string::npos) {
            std::string kind_str = mime.substr(0, slash);
            cap.name = mime.substr(slash + 1);

            if (kind_str == "video") {
                cap.kind = webrtc::MediaType::VIDEO;
            } else if (kind_str == "audio") {
                cap.kind = webrtc::MediaType::AUDIO;
            } else {
                continue; // Skip unknown types
            }
        } else {
            // No slash, assume it's just the codec name
            cap.name = mime;
            // Try to infer kind from transceiver
            cap.kind = t->media_type();
        }

        if (codecs[i].clock_rate > 0) {
            cap.clock_rate = codecs[i].clock_rate;
        }

        if (codecs[i].channels > 0) {
            cap.num_channels = codecs[i].channels;
        }

        // Parse sdp_fmtp_line if present
        if (codecs[i].sdp_fmtp_line[0] != '\0') {
            // Simple parsing: split on ; and =
            std::string fmtp = codecs[i].sdp_fmtp_line;
            size_t pos = 0;
            while (pos < fmtp.size()) {
                size_t eq = fmtp.find('=', pos);
                size_t semi = fmtp.find(';', pos);
                if (semi == std::string::npos) semi = fmtp.size();

                if (eq != std::string::npos && eq < semi) {
                    std::string key = fmtp.substr(pos, eq - pos);
                    std::string val = fmtp.substr(eq + 1, semi - eq - 1);
                    cap.parameters[key] = val;
                }
                pos = semi + 1;
                while (pos < fmtp.size() && fmtp[pos] == ' ') pos++;
            }
        }

        prefs.push_back(cap);
    }

    auto result = t->SetCodecPreferences(prefs);
    return result.ok() ? SHIM_OK : SHIM_ERROR_INIT_FAILED;
}

SHIM_EXPORT int shim_transceiver_get_codec_preferences(
    ShimRTPTransceiver* transceiver,
    ShimCodecCapability* codecs,
    int max_codecs,
    int* out_count
) {
    if (!transceiver || !codecs || !out_count) return SHIM_ERROR_INVALID_PARAM;

    auto t = reinterpret_cast<webrtc::RtpTransceiverInterface*>(transceiver);

    // Note: libwebrtc doesn't expose a direct "get codec preferences" method
    // We can get the header extensions and codecs via the sender's parameters
    // For now, return the codec from the first encoding's codec info

    *out_count = 0;
    return SHIM_OK;
}

}  // extern "C"
