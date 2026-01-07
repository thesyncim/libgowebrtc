/*
 * shim_rtp_sender.cc - RTPSender and RTPReceiver implementation
 *
 * Provides RTP sender parameter management, bitrate control,
 * and layer manipulation.
 */

#include "shim_common.h"

#include <algorithm>
#include <cstring>

#include "api/rtp_sender_interface.h"
#include "api/rtp_receiver_interface.h"
#include "api/rtp_parameters.h"
#include "api/media_types.h"
#include "media/base/media_constants.h"

extern "C" {

/* ============================================================================
 * RTPSender Implementation
 * ========================================================================== */

SHIM_EXPORT int shim_rtp_sender_set_bitrate(ShimRTPSender* sender, uint32_t bitrate) {
    if (!sender) return SHIM_ERROR_INVALID_PARAM;

    auto webrtc_sender = reinterpret_cast<webrtc::RtpSenderInterface*>(sender);
    auto params = webrtc_sender->GetParameters();

    for (auto& encoding : params.encodings) {
        encoding.max_bitrate_bps = bitrate;
    }

    auto result = webrtc_sender->SetParameters(params);
    return result.ok() ? SHIM_OK : SHIM_ERROR_INIT_FAILED;
}

SHIM_EXPORT int shim_rtp_sender_replace_track(ShimRTPSender* sender, void* track) {
    if (!sender) return SHIM_ERROR_INVALID_PARAM;

    auto webrtc_sender = reinterpret_cast<webrtc::RtpSenderInterface*>(sender);
    auto media_track = static_cast<webrtc::MediaStreamTrackInterface*>(track);

    bool result = webrtc_sender->SetTrack(media_track);
    return result ? SHIM_OK : SHIM_ERROR_INIT_FAILED;
}

SHIM_EXPORT void shim_rtp_sender_destroy(ShimRTPSender* sender) {
    // Sender is owned by PeerConnection, don't delete
}

SHIM_EXPORT int shim_rtp_sender_get_parameters(
    ShimRTPSender* sender,
    ShimRTPSendParameters* out_params,
    ShimRTPEncodingParameters* encodings,
    int max_encodings
) {
    if (!sender || !out_params || !encodings) return SHIM_ERROR_INVALID_PARAM;

    auto webrtc_sender = reinterpret_cast<webrtc::RtpSenderInterface*>(sender);
    auto params = webrtc_sender->GetParameters();

    int count = std::min(static_cast<int>(params.encodings.size()), max_encodings);
    out_params->encoding_count = count;
    out_params->encodings = encodings;

    for (int i = 0; i < count; i++) {
        const auto& enc = params.encodings[i];
        auto& out = encodings[i];

        memset(&out, 0, sizeof(ShimRTPEncodingParameters));

        if (!enc.rid.empty()) {
            strncpy(out.rid, enc.rid.c_str(), sizeof(out.rid) - 1);
            out.rid[sizeof(out.rid) - 1] = '\0';
        }

        out.max_bitrate_bps = enc.max_bitrate_bps.value_or(0);
        out.min_bitrate_bps = enc.min_bitrate_bps.value_or(0);
        out.max_framerate = enc.max_framerate.value_or(0.0);
        out.scale_resolution_down_by = enc.scale_resolution_down_by.value_or(1.0);
        out.active = enc.active ? 1 : 0;

        if (enc.scalability_mode.has_value()) {
            strncpy(out.scalability_mode, enc.scalability_mode->c_str(), sizeof(out.scalability_mode) - 1);
            out.scalability_mode[sizeof(out.scalability_mode) - 1] = '\0';
        } else {
            out.scalability_mode[0] = '\0';
        }
    }

    strncpy(out_params->transaction_id, params.transaction_id.c_str(), sizeof(out_params->transaction_id) - 1);
    out_params->transaction_id[sizeof(out_params->transaction_id) - 1] = '\0';

    return SHIM_OK;
}

SHIM_EXPORT int shim_rtp_sender_set_parameters(
    ShimRTPSender* sender,
    const ShimRTPSendParameters* params
) {
    if (!sender || !params) return SHIM_ERROR_INVALID_PARAM;

    auto webrtc_sender = reinterpret_cast<webrtc::RtpSenderInterface*>(sender);
    auto rtp_params = webrtc_sender->GetParameters();

    // Update encodings
    for (int i = 0; i < params->encoding_count && i < static_cast<int>(rtp_params.encodings.size()); i++) {
        const auto& in = params->encodings[i];
        auto& enc = rtp_params.encodings[i];

        if (in.max_bitrate_bps > 0) enc.max_bitrate_bps = in.max_bitrate_bps;
        if (in.min_bitrate_bps > 0) enc.min_bitrate_bps = in.min_bitrate_bps;
        if (in.max_framerate > 0) enc.max_framerate = in.max_framerate;
        if (in.scale_resolution_down_by > 0) enc.scale_resolution_down_by = in.scale_resolution_down_by;
        enc.active = in.active != 0;

        if (in.scalability_mode[0] != '\0') {
            enc.scalability_mode = std::string(in.scalability_mode);
        }
    }

    auto result = webrtc_sender->SetParameters(rtp_params);
    return result.ok() ? SHIM_OK : SHIM_ERROR_INIT_FAILED;
}

SHIM_EXPORT void* shim_rtp_sender_get_track(ShimRTPSender* sender) {
    if (!sender) return nullptr;
    auto webrtc_sender = reinterpret_cast<webrtc::RtpSenderInterface*>(sender);
    return webrtc_sender->track().get();
}

SHIM_EXPORT int shim_rtp_sender_get_stats(ShimRTPSender* sender, ShimRTCStats* out_stats) {
    if (!sender || !out_stats) return SHIM_ERROR_INVALID_PARAM;
    memset(out_stats, 0, sizeof(ShimRTCStats));
    // TODO: Implement stats collection
    return SHIM_OK;
}

SHIM_EXPORT void shim_rtp_sender_set_on_rtcp_feedback(
    ShimRTPSender* sender,
    ShimOnRTCPFeedback callback,
    void* ctx
) {
    // TODO: Implement RTCP feedback notification
}

SHIM_EXPORT int shim_rtp_sender_set_layer_active(
    ShimRTPSender* sender,
    const char* rid,
    int active
) {
    if (!sender) return SHIM_ERROR_INVALID_PARAM;

    auto webrtc_sender = reinterpret_cast<webrtc::RtpSenderInterface*>(sender);
    auto params = webrtc_sender->GetParameters();

    bool found = false;
    for (auto& enc : params.encodings) {
        if (enc.rid == rid) {
            enc.active = active != 0;
            found = true;
            break;
        }
    }

    if (!found) return SHIM_ERROR_INVALID_PARAM;

    auto result = webrtc_sender->SetParameters(params);
    return result.ok() ? SHIM_OK : SHIM_ERROR_INIT_FAILED;
}

SHIM_EXPORT int shim_rtp_sender_set_layer_bitrate(
    ShimRTPSender* sender,
    const char* rid,
    uint32_t max_bitrate_bps
) {
    if (!sender) return SHIM_ERROR_INVALID_PARAM;

    auto webrtc_sender = reinterpret_cast<webrtc::RtpSenderInterface*>(sender);
    auto params = webrtc_sender->GetParameters();

    bool found = false;
    for (auto& enc : params.encodings) {
        if (enc.rid == rid) {
            enc.max_bitrate_bps = max_bitrate_bps;
            found = true;
            break;
        }
    }

    if (!found) return SHIM_ERROR_INVALID_PARAM;

    auto result = webrtc_sender->SetParameters(params);
    return result.ok() ? SHIM_OK : SHIM_ERROR_INIT_FAILED;
}

SHIM_EXPORT int shim_rtp_sender_get_active_layers(
    ShimRTPSender* sender,
    int* out_spatial,
    int* out_temporal
) {
    if (!sender || !out_spatial || !out_temporal) return SHIM_ERROR_INVALID_PARAM;

    auto webrtc_sender = reinterpret_cast<webrtc::RtpSenderInterface*>(sender);
    auto params = webrtc_sender->GetParameters();

    int active = 0;
    for (const auto& enc : params.encodings) {
        if (enc.active) active++;
    }

    *out_spatial = active;
    *out_temporal = 0; // Would need to parse scalability_mode to get temporal layers

    return SHIM_OK;
}

SHIM_EXPORT int shim_rtp_sender_set_scalability_mode(
    ShimRTPSender* sender,
    const char* scalability_mode
) {
    if (!sender || !scalability_mode) return SHIM_ERROR_INVALID_PARAM;

    auto webrtc_sender = reinterpret_cast<webrtc::RtpSenderInterface*>(sender);
    auto params = webrtc_sender->GetParameters();

    for (auto& enc : params.encodings) {
        enc.scalability_mode = std::string(scalability_mode);
    }

    auto result = webrtc_sender->SetParameters(params);
    return result.ok() ? SHIM_OK : SHIM_ERROR_INIT_FAILED;
}

SHIM_EXPORT int shim_rtp_sender_get_scalability_mode(
    ShimRTPSender* sender,
    char* mode_out,
    int mode_out_size
) {
    if (!sender || !mode_out || mode_out_size <= 0) return SHIM_ERROR_INVALID_PARAM;

    auto webrtc_sender = reinterpret_cast<webrtc::RtpSenderInterface*>(sender);
    auto params = webrtc_sender->GetParameters();

    if (!params.encodings.empty() && params.encodings[0].scalability_mode.has_value()) {
        strncpy(mode_out, params.encodings[0].scalability_mode->c_str(), mode_out_size - 1);
        mode_out[mode_out_size - 1] = '\0';
    } else {
        mode_out[0] = '\0';
    }

    return SHIM_OK;
}

SHIM_EXPORT int shim_rtp_sender_get_negotiated_codecs(
    ShimRTPSender* sender,
    ShimCodecCapability* codecs,
    int max_codecs,
    int* out_count
) {
    if (!sender || !codecs || !out_count) return SHIM_ERROR_INVALID_PARAM;

    auto webrtc_sender = reinterpret_cast<webrtc::RtpSenderInterface*>(sender);
    auto params = webrtc_sender->GetParameters();

    int count = 0;
    for (const auto& codec : params.codecs) {
        if (count >= max_codecs) break;

        memset(&codecs[count], 0, sizeof(ShimCodecCapability));

        // Build mime type from kind + name
        std::string kind_str = (webrtc_sender->media_type() == webrtc::MediaType::VIDEO) ? "video" : "audio";
        std::string mime = kind_str + "/" + codec.name;
        strncpy(codecs[count].mime_type, mime.c_str(), sizeof(codecs[count].mime_type) - 1);

        codecs[count].clock_rate = codec.clock_rate.value_or(0);
        codecs[count].channels = codec.num_channels.value_or(0);
        codecs[count].payload_type = codec.payload_type;

        // Build sdp_fmtp_line from parameters
        std::string fmtp;
        for (const auto& [key, value] : codec.parameters) {
            if (!fmtp.empty()) fmtp += ";";
            fmtp += key + "=" + value;
        }
        strncpy(codecs[count].sdp_fmtp_line, fmtp.c_str(), sizeof(codecs[count].sdp_fmtp_line) - 1);

        count++;
    }

    *out_count = count;
    return SHIM_OK;
}

SHIM_EXPORT int shim_rtp_sender_set_preferred_codec(
    ShimRTPSender* sender,
    const char* mime_type,
    int payload_type
) {
    if (!sender || !mime_type) return SHIM_ERROR_INVALID_PARAM;

    auto webrtc_sender = reinterpret_cast<webrtc::RtpSenderInterface*>(sender);
    auto params = webrtc_sender->GetParameters();

    if (params.codecs.empty()) {
        return SHIM_ERROR_NOT_FOUND;
    }

    // Parse mime_type to get kind and codec name
    std::string mime = mime_type;
    std::string codec_name;
    webrtc::MediaType kind = webrtc::MediaType::VIDEO;
    size_t slash = mime.find('/');
    if (slash != std::string::npos) {
        std::string kind_str = mime.substr(0, slash);
        codec_name = mime.substr(slash + 1);
        if (kind_str == "audio") {
            kind = webrtc::MediaType::AUDIO;
        }
    } else {
        codec_name = mime;
    }

    // Find the codec in the negotiated list
    const webrtc::RtpCodecParameters* found_codec = nullptr;
    for (const auto& codec : params.codecs) {
        bool name_match = (codec.name == codec_name);
        bool pt_match = (payload_type == 0 || codec.payload_type == payload_type);
        if (name_match && pt_match) {
            found_codec = &codec;
            break;
        }
    }

    if (!found_codec) {
        return SHIM_ERROR_NOT_FOUND;
    }

    // Set the codec on each encoding using the encoding.codec field
    // This tells the encoder which codec to use without reordering the codecs list
    for (auto& encoding : params.encodings) {
        webrtc::RtpCodec preferred;
        preferred.name = found_codec->name;
        preferred.kind = kind;
        preferred.clock_rate = found_codec->clock_rate;
        preferred.num_channels = found_codec->num_channels;
        preferred.parameters = found_codec->parameters;
        encoding.codec = preferred;
    }

    // Set the updated parameters
    auto result = webrtc_sender->SetParameters(params);
    if (!result.ok()) {
        // Log the error for debugging
        fprintf(stderr, "SHIM DEBUG: SetParameters failed: %s\n", result.message());
        return SHIM_ERROR_RENEGOTIATION_NEEDED;
    }

    return SHIM_OK;
}

}  // extern "C"
