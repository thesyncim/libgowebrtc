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

SHIM_EXPORT int shim_rtp_sender_set_bitrate(ShimRTPSenderSetBitrateParams* params) {
    if (!params || !params->sender) {
        shim::SetErrorMessage(params ? params->error_out : nullptr, "invalid parameter", SHIM_ERROR_INVALID_PARAM);
        return SHIM_ERROR_INVALID_PARAM;
    }

    auto webrtc_sender = reinterpret_cast<webrtc::RtpSenderInterface*>(params->sender);
    auto rtp_params = webrtc_sender->GetParameters();

    for (auto& encoding : rtp_params.encodings) {
        encoding.max_bitrate_bps = params->bitrate;
    }

    auto result = webrtc_sender->SetParameters(rtp_params);
    if (!result.ok()) {
        shim::SetErrorFromRTCError(params->error_out, result);
        return SHIM_ERROR_INIT_FAILED;
    }
    return SHIM_OK;
}

SHIM_EXPORT int shim_rtp_sender_replace_track(ShimRTPSenderReplaceTrackParams* params) {
    if (!params || !params->sender) return SHIM_ERROR_INVALID_PARAM;

    auto webrtc_sender = reinterpret_cast<webrtc::RtpSenderInterface*>(params->sender);
    auto media_track = static_cast<webrtc::MediaStreamTrackInterface*>(params->track);

    bool result = webrtc_sender->SetTrack(media_track);
    return result ? SHIM_OK : SHIM_ERROR_INIT_FAILED;
}

SHIM_EXPORT void shim_rtp_sender_destroy(ShimRTPSender* sender) {
    // Sender is owned by PeerConnection, don't delete
}

SHIM_EXPORT int shim_rtp_sender_get_parameters(ShimRTPSenderGetParametersParams* params) {
    if (!params || !params->sender || !params->encodings || params->max_encodings <= 0) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    auto webrtc_sender = reinterpret_cast<webrtc::RtpSenderInterface*>(params->sender);
    auto rtp_params = webrtc_sender->GetParameters();

    int count = std::min(static_cast<int>(rtp_params.encodings.size()), params->max_encodings);
    params->out_params.encoding_count = count;
    params->out_params.encodings = params->encodings;

    for (int i = 0; i < count; i++) {
        const auto& enc = rtp_params.encodings[i];
        auto& out = params->encodings[i];

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

    strncpy(params->out_params.transaction_id, rtp_params.transaction_id.c_str(), sizeof(params->out_params.transaction_id) - 1);
    params->out_params.transaction_id[sizeof(params->out_params.transaction_id) - 1] = '\0';

    return SHIM_OK;
}

SHIM_EXPORT int shim_rtp_sender_set_parameters(ShimRTPSenderSetParametersParams* params) {
    if (!params || !params->sender || !params->params) {
        shim::SetErrorMessage(params ? params->error_out : nullptr, "invalid parameter", SHIM_ERROR_INVALID_PARAM);
        return SHIM_ERROR_INVALID_PARAM;
    }

    auto webrtc_sender = reinterpret_cast<webrtc::RtpSenderInterface*>(params->sender);
    auto rtp_params = webrtc_sender->GetParameters();

    // Update encodings
    for (int i = 0; i < params->params->encoding_count && i < static_cast<int>(rtp_params.encodings.size()); i++) {
        const auto& in = params->params->encodings[i];
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
    if (!result.ok()) {
        shim::SetErrorFromRTCError(params->error_out, result);
        return SHIM_ERROR_INIT_FAILED;
    }
    return SHIM_OK;
}

SHIM_EXPORT void* shim_rtp_sender_get_track(ShimRTPSender* sender) {
    if (!sender) return nullptr;
    auto webrtc_sender = reinterpret_cast<webrtc::RtpSenderInterface*>(sender);
    return webrtc_sender->track().get();
}

SHIM_EXPORT int shim_rtp_sender_get_stats(ShimRTPSenderGetStatsParams* params) {
    if (!params) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    if (!params->sender) {
        memset(&params->out_stats, 0, sizeof(ShimRTCStats));
        return SHIM_ERROR_INVALID_PARAM;
    }

    memset(&params->out_stats, 0, sizeof(ShimRTCStats));
    // TODO: Implement stats collection
    return SHIM_OK;
}

SHIM_EXPORT void shim_rtp_sender_set_on_rtcp_feedback(ShimRTPSenderSetOnRTCPFeedbackParams* params) {
    // TODO: Implement RTCP feedback notification
    (void)params;
}

SHIM_EXPORT int shim_rtp_sender_set_layer_active(ShimRTPSenderSetLayerActiveParams* params) {
    if (!params || !params->sender) {
        shim::SetErrorMessage(params ? params->error_out : nullptr, "invalid parameter", SHIM_ERROR_INVALID_PARAM);
        return SHIM_ERROR_INVALID_PARAM;
    }

    auto webrtc_sender = reinterpret_cast<webrtc::RtpSenderInterface*>(params->sender);
    auto rtp_params = webrtc_sender->GetParameters();

    bool found = false;
    for (auto& enc : rtp_params.encodings) {
        if (enc.rid == params->rid) {
            enc.active = params->active != 0;
            found = true;
            break;
        }
    }

    if (!found) {
        std::string msg = "RID not found: ";
        msg += params->rid ? params->rid : "(null)";
        shim::SetErrorMessage(params->error_out, msg, SHIM_ERROR_INVALID_PARAM);
        return SHIM_ERROR_INVALID_PARAM;
    }

    auto result = webrtc_sender->SetParameters(rtp_params);
    if (!result.ok()) {
        shim::SetErrorFromRTCError(params->error_out, result);
        return SHIM_ERROR_INIT_FAILED;
    }
    return SHIM_OK;
}

SHIM_EXPORT int shim_rtp_sender_set_layer_bitrate(ShimRTPSenderSetLayerBitrateParams* params) {
    if (!params || !params->sender) {
        shim::SetErrorMessage(params ? params->error_out : nullptr, "invalid parameter", SHIM_ERROR_INVALID_PARAM);
        return SHIM_ERROR_INVALID_PARAM;
    }

    auto webrtc_sender = reinterpret_cast<webrtc::RtpSenderInterface*>(params->sender);
    auto rtp_params = webrtc_sender->GetParameters();

    bool found = false;
    for (auto& enc : rtp_params.encodings) {
        if (enc.rid == params->rid) {
            enc.max_bitrate_bps = params->max_bitrate_bps;
            found = true;
            break;
        }
    }

    if (!found) {
        std::string msg = "RID not found: ";
        msg += params->rid ? params->rid : "(null)";
        shim::SetErrorMessage(params->error_out, msg, SHIM_ERROR_INVALID_PARAM);
        return SHIM_ERROR_INVALID_PARAM;
    }

    auto result = webrtc_sender->SetParameters(rtp_params);
    if (!result.ok()) {
        shim::SetErrorFromRTCError(params->error_out, result);
        return SHIM_ERROR_INIT_FAILED;
    }
    return SHIM_OK;
}

SHIM_EXPORT int shim_rtp_sender_get_active_layers(ShimRTPSenderGetActiveLayersParams* params) {
    if (!params || !params->sender) return SHIM_ERROR_INVALID_PARAM;
    params->out_spatial = 0;
    params->out_temporal = 0;

    auto webrtc_sender = reinterpret_cast<webrtc::RtpSenderInterface*>(params->sender);
    auto rtp_params = webrtc_sender->GetParameters();

    int active = 0;
    for (const auto& enc : rtp_params.encodings) {
        if (enc.active) active++;
    }

    params->out_spatial = active;
    params->out_temporal = 0; // Would need to parse scalability_mode to get temporal layers

    return SHIM_OK;
}

SHIM_EXPORT int shim_rtp_sender_set_scalability_mode(ShimRTPSenderSetScalabilityModeParams* params) {
    if (!params || !params->sender || !params->scalability_mode) {
        shim::SetErrorMessage(params ? params->error_out : nullptr, "invalid parameter", SHIM_ERROR_INVALID_PARAM);
        return SHIM_ERROR_INVALID_PARAM;
    }

    auto webrtc_sender = reinterpret_cast<webrtc::RtpSenderInterface*>(params->sender);
    auto rtp_params = webrtc_sender->GetParameters();

    for (auto& enc : rtp_params.encodings) {
        enc.scalability_mode = std::string(params->scalability_mode);
    }

    auto result = webrtc_sender->SetParameters(rtp_params);
    if (!result.ok()) {
        shim::SetErrorFromRTCError(params->error_out, result);
        return SHIM_ERROR_INIT_FAILED;
    }
    return SHIM_OK;
}

SHIM_EXPORT int shim_rtp_sender_get_scalability_mode(ShimRTPSenderGetScalabilityModeParams* params) {
    if (!params || !params->sender || !params->mode_out || params->mode_out_size <= 0) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    auto webrtc_sender = reinterpret_cast<webrtc::RtpSenderInterface*>(params->sender);
    auto rtp_params = webrtc_sender->GetParameters();

    if (!rtp_params.encodings.empty() && rtp_params.encodings[0].scalability_mode.has_value()) {
        strncpy(params->mode_out, rtp_params.encodings[0].scalability_mode->c_str(), params->mode_out_size - 1);
        params->mode_out[params->mode_out_size - 1] = '\0';
    } else {
        params->mode_out[0] = '\0';
    }

    return SHIM_OK;
}

SHIM_EXPORT int shim_rtp_sender_get_negotiated_codecs(ShimRTPSenderGetNegotiatedCodecsParams* params) {
    if (!params) {
        return SHIM_ERROR_INVALID_PARAM;
    }
    if (!params->sender || !params->codecs || params->max_codecs <= 0) {
        params->out_count = 0;
        return SHIM_ERROR_INVALID_PARAM;
    }

    auto webrtc_sender = reinterpret_cast<webrtc::RtpSenderInterface*>(params->sender);
    auto rtp_params = webrtc_sender->GetParameters();

    int count = 0;
    for (const auto& codec : rtp_params.codecs) {
        if (count >= params->max_codecs) break;

        memset(&params->codecs[count], 0, sizeof(ShimCodecCapability));

        // Build mime type from kind + name
        std::string kind_str = (webrtc_sender->media_type() == webrtc::MediaType::VIDEO) ? "video" : "audio";
        std::string mime = kind_str + "/" + codec.name;
        strncpy(params->codecs[count].mime_type, mime.c_str(), sizeof(params->codecs[count].mime_type) - 1);

        params->codecs[count].clock_rate = codec.clock_rate.value_or(0);
        params->codecs[count].channels = codec.num_channels.value_or(0);
        params->codecs[count].payload_type = codec.payload_type;

        // Build sdp_fmtp_line from parameters
        std::string fmtp;
        for (const auto& [key, value] : codec.parameters) {
            if (!fmtp.empty()) fmtp += ";";
            fmtp += key + "=" + value;
        }
        strncpy(params->codecs[count].sdp_fmtp_line, fmtp.c_str(), sizeof(params->codecs[count].sdp_fmtp_line) - 1);

        count++;
    }

    params->out_count = count;
    return SHIM_OK;
}

SHIM_EXPORT int shim_rtp_sender_set_preferred_codec(ShimRTPSenderSetPreferredCodecParams* params) {
    if (!params || !params->sender || !params->mime_type) {
        shim::SetErrorMessage(params ? params->error_out : nullptr, "invalid parameter", SHIM_ERROR_INVALID_PARAM);
        return SHIM_ERROR_INVALID_PARAM;
    }

    auto webrtc_sender = reinterpret_cast<webrtc::RtpSenderInterface*>(params->sender);
    auto rtp_params = webrtc_sender->GetParameters();

    if (rtp_params.codecs.empty()) {
        shim::SetErrorMessage(params->error_out, "no codecs negotiated", SHIM_ERROR_NOT_FOUND);
        return SHIM_ERROR_NOT_FOUND;
    }

    // Parse mime_type to get kind and codec name
    std::string mime = params->mime_type;
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
    for (const auto& codec : rtp_params.codecs) {
        bool name_match = (codec.name == codec_name);
        bool pt_match = (params->payload_type == 0 || codec.payload_type == params->payload_type);
        if (name_match && pt_match) {
            found_codec = &codec;
            break;
        }
    }

    if (!found_codec) {
        std::string msg = "codec not found: " + mime;
        shim::SetErrorMessage(params->error_out, msg, SHIM_ERROR_NOT_FOUND);
        return SHIM_ERROR_NOT_FOUND;
    }

    // Set the codec on each encoding using the encoding.codec field
    for (auto& encoding : rtp_params.encodings) {
        webrtc::RtpCodec preferred;
        preferred.name = found_codec->name;
        preferred.kind = kind;
        preferred.clock_rate = found_codec->clock_rate;
        preferred.num_channels = found_codec->num_channels;
        preferred.parameters = found_codec->parameters;
        encoding.codec = preferred;
    }

    // Set the updated parameters
    auto result = webrtc_sender->SetParameters(rtp_params);
    if (!result.ok()) {
        shim::SetErrorFromRTCError(params->error_out, result, SHIM_ERROR_RENEGOTIATION_NEEDED);
        return SHIM_ERROR_RENEGOTIATION_NEEDED;
    }

    return SHIM_OK;
}

}  // extern "C"
