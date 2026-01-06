/*
 * libwebrtc_shim STUB implementation
 *
 * This is a stub implementation that compiles without libwebrtc.
 * It returns placeholder values and can be used to verify:
 * - Build system works
 * - FFI bindings work
 * - Go code can load the library
 *
 * For actual functionality, build with real libwebrtc (see BUILD.md)
 */

#include "shim.h"

#include <cstring>
#include <cstdlib>
#include <vector>
#include <string>
#include <mutex>
#include <atomic>

namespace {
    const char* kShimVersion = "1.0.0-stub";
    const char* kLibWebRTCVersion = "stub";

    // Thread-safe ID generator
    std::atomic<uintptr_t> g_next_handle{1};

    uintptr_t next_handle() {
        return g_next_handle.fetch_add(1);
    }
}

// Opaque handle structures
struct ShimVideoEncoder {
    int width;
    int height;
    uint32_t bitrate;
    ShimCodecType codec;
    std::vector<uint8_t> fake_encoded;
    bool keyframe_pending;
};

struct ShimVideoDecoder {
    ShimCodecType codec;
    int width;
    int height;
};

struct ShimAudioEncoder {
    int sample_rate;
    int channels;
    uint32_t bitrate;
};

struct ShimAudioDecoder {
    int sample_rate;
    int channels;
};

struct ShimPacketizer {
    ShimCodecType codec;
    uint32_t ssrc;
    uint16_t sequence;
    uint16_t mtu;
};

struct ShimDepacketizer {
    ShimCodecType codec;
    std::vector<uint8_t> buffer;
};

struct ShimPeerConnection {
    int signaling_state;
    int ice_connection_state;
    int ice_gathering_state;
    int connection_state;
    std::string local_sdp;
    std::string remote_sdp;

    // Callbacks
    void* ice_candidate_ctx;
    ShimOnICECandidate on_ice_candidate;
    void* connection_state_ctx;
    ShimOnConnectionStateChange on_connection_state;
    void* track_ctx;
    ShimOnTrack on_track;
    void* data_channel_ctx;
    ShimOnDataChannel on_data_channel;
};

struct ShimRTPSender {
    uintptr_t handle;
    ShimCodecType codec;
    std::string track_id;
};

struct ShimRTPReceiver {
    uintptr_t handle;
};

struct ShimRTPTransceiver {
    uintptr_t handle;
    int direction;
    ShimRTPSender* sender;
    ShimRTPReceiver* receiver;
    std::string mid;
};

struct ShimDataChannel {
    std::string label;
    int ready_state;
    void* message_ctx;
    ShimOnDataChannelMessage on_message;
};

struct ShimVideoTrackSource {
    int width;
    int height;
};

struct ShimAudioTrackSource {
    int sample_rate;
    int channels;
};

extern "C" {

/* ============================================================================
 * Version
 * ========================================================================== */

SHIM_EXPORT const char* shim_version(void) {
    return kShimVersion;
}

SHIM_EXPORT const char* shim_libwebrtc_version(void) {
    return kLibWebRTCVersion;
}

/* ============================================================================
 * Video Encoder
 * ========================================================================== */

SHIM_EXPORT ShimVideoEncoder* shim_video_encoder_create(
    ShimCodecType codec,
    const ShimVideoEncoderConfig* config
) {
    if (!config || config->width <= 0 || config->height <= 0) {
        return nullptr;
    }

    auto enc = new ShimVideoEncoder();
    enc->width = config->width;
    enc->height = config->height;
    enc->bitrate = config->bitrate_bps;
    enc->codec = codec;
    enc->keyframe_pending = true;

    // Pre-allocate fake encoded data (just a header for testing)
    enc->fake_encoded.resize(64);
    return enc;
}

SHIM_EXPORT int shim_video_encoder_encode(
    ShimVideoEncoder* encoder,
    const uint8_t* y_plane,
    const uint8_t* u_plane,
    const uint8_t* v_plane,
    int y_stride,
    int u_stride,
    int v_stride,
    uint32_t timestamp,
    int force_keyframe,
    uint8_t* dst_buffer,
    int* out_size,
    int* out_is_keyframe
) {
    if (!encoder || !dst_buffer || !out_size || !out_is_keyframe) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    // Generate fake encoded data
    bool is_keyframe = force_keyframe || encoder->keyframe_pending;
    encoder->keyframe_pending = false;

    // Fake NAL unit for H.264 or frame header for VP8/VP9
    int size = 32;  // Small fake frame
    if (is_keyframe) {
        size = 128;  // Larger for keyframe
    }

    // Write fake data
    memset(dst_buffer, 0, size);
    dst_buffer[0] = is_keyframe ? 0x65 : 0x41;  // NAL type indicator
    dst_buffer[1] = (timestamp >> 24) & 0xFF;
    dst_buffer[2] = (timestamp >> 16) & 0xFF;
    dst_buffer[3] = (timestamp >> 8) & 0xFF;
    dst_buffer[4] = timestamp & 0xFF;

    *out_size = size;
    *out_is_keyframe = is_keyframe ? 1 : 0;

    return SHIM_OK;
}

SHIM_EXPORT int shim_video_encoder_set_bitrate(ShimVideoEncoder* encoder, uint32_t bitrate_bps) {
    if (!encoder) return SHIM_ERROR_INVALID_PARAM;
    encoder->bitrate = bitrate_bps;
    return SHIM_OK;
}

SHIM_EXPORT int shim_video_encoder_set_framerate(ShimVideoEncoder* encoder, float framerate) {
    if (!encoder) return SHIM_ERROR_INVALID_PARAM;
    return SHIM_OK;
}

SHIM_EXPORT int shim_video_encoder_request_keyframe(ShimVideoEncoder* encoder) {
    if (!encoder) return SHIM_ERROR_INVALID_PARAM;
    encoder->keyframe_pending = true;
    return SHIM_OK;
}

SHIM_EXPORT void shim_video_encoder_destroy(ShimVideoEncoder* encoder) {
    delete encoder;
}

/* ============================================================================
 * Video Decoder
 * ========================================================================== */

SHIM_EXPORT ShimVideoDecoder* shim_video_decoder_create(ShimCodecType codec) {
    auto dec = new ShimVideoDecoder();
    dec->codec = codec;
    dec->width = 0;
    dec->height = 0;
    return dec;
}

SHIM_EXPORT int shim_video_decoder_decode(
    ShimVideoDecoder* decoder,
    const uint8_t* data,
    int size,
    uint32_t timestamp,
    int is_keyframe,
    uint8_t* y_dst,
    uint8_t* u_dst,
    uint8_t* v_dst,
    int* out_width,
    int* out_height,
    int* out_y_stride,
    int* out_u_stride,
    int* out_v_stride
) {
    if (!decoder || !data || size <= 0) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    // Fake decode - extract dimensions from first keyframe
    if (is_keyframe && decoder->width == 0) {
        decoder->width = 1280;
        decoder->height = 720;
    }

    if (decoder->width == 0) {
        return SHIM_ERROR_NEED_MORE_DATA;
    }

    *out_width = decoder->width;
    *out_height = decoder->height;
    *out_y_stride = decoder->width;
    *out_u_stride = decoder->width / 2;
    *out_v_stride = decoder->width / 2;

    // Fill with gray (Y=128, U=128, V=128)
    if (y_dst) memset(y_dst, 128, decoder->width * decoder->height);
    if (u_dst) memset(u_dst, 128, (decoder->width/2) * (decoder->height/2));
    if (v_dst) memset(v_dst, 128, (decoder->width/2) * (decoder->height/2));

    return SHIM_OK;
}

SHIM_EXPORT void shim_video_decoder_destroy(ShimVideoDecoder* decoder) {
    delete decoder;
}

/* ============================================================================
 * Audio Encoder
 * ========================================================================== */

SHIM_EXPORT ShimAudioEncoder* shim_audio_encoder_create(const ShimAudioEncoderConfig* config) {
    if (!config) return nullptr;

    auto enc = new ShimAudioEncoder();
    enc->sample_rate = config->sample_rate;
    enc->channels = config->channels;
    enc->bitrate = config->bitrate_bps;
    return enc;
}

SHIM_EXPORT int shim_audio_encoder_encode(
    ShimAudioEncoder* encoder,
    const uint8_t* samples,
    int num_samples,
    uint8_t* dst_buffer,
    int* out_size
) {
    if (!encoder || !samples || !dst_buffer || !out_size) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    // Fake Opus frame (just copy some bytes)
    int out_bytes = num_samples / 4;  // Rough compression ratio
    if (out_bytes < 3) out_bytes = 3;
    if (out_bytes > 500) out_bytes = 500;

    memset(dst_buffer, 0, out_bytes);
    *out_size = out_bytes;

    return SHIM_OK;
}

SHIM_EXPORT int shim_audio_encoder_set_bitrate(ShimAudioEncoder* encoder, uint32_t bitrate_bps) {
    if (!encoder) return SHIM_ERROR_INVALID_PARAM;
    encoder->bitrate = bitrate_bps;
    return SHIM_OK;
}

SHIM_EXPORT void shim_audio_encoder_destroy(ShimAudioEncoder* encoder) {
    delete encoder;
}

/* ============================================================================
 * Audio Decoder
 * ========================================================================== */

SHIM_EXPORT ShimAudioDecoder* shim_audio_decoder_create(int sample_rate, int channels) {
    auto dec = new ShimAudioDecoder();
    dec->sample_rate = sample_rate;
    dec->channels = channels;
    return dec;
}

SHIM_EXPORT int shim_audio_decoder_decode(
    ShimAudioDecoder* decoder,
    const uint8_t* data,
    int size,
    uint8_t* dst_samples,
    int* out_num_samples
) {
    if (!decoder || !data || !dst_samples || !out_num_samples) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    // Fake decode - produce silence
    int samples = 960;  // 20ms at 48kHz
    memset(dst_samples, 0, samples * decoder->channels * 2);
    *out_num_samples = samples * decoder->channels;

    return SHIM_OK;
}

SHIM_EXPORT void shim_audio_decoder_destroy(ShimAudioDecoder* decoder) {
    delete decoder;
}

/* ============================================================================
 * Packetizer
 * ========================================================================== */

SHIM_EXPORT ShimPacketizer* shim_packetizer_create(const ShimPacketizerConfig* config) {
    if (!config) return nullptr;

    auto pkt = new ShimPacketizer();
    pkt->codec = config->codec;
    pkt->ssrc = config->ssrc;
    pkt->sequence = 0;
    pkt->mtu = config->mtu;
    return pkt;
}

SHIM_EXPORT int shim_packetizer_packetize(
    ShimPacketizer* packetizer,
    const uint8_t* data,
    int size,
    uint32_t timestamp,
    int is_keyframe,
    uint8_t* dst_buffer,
    int* dst_offsets,
    int* dst_sizes,
    int max_packets,
    int* out_count
) {
    if (!packetizer || !data || !dst_buffer || !dst_offsets || !dst_sizes || !out_count) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    // Simple packetization - one packet per MTU
    int mtu = packetizer->mtu;
    int offset = 0;
    int packet_count = 0;

    while (offset < size && packet_count < max_packets) {
        int payload_size = (size - offset) > (mtu - 12) ? (mtu - 12) : (size - offset);

        // Write RTP header (12 bytes)
        uint8_t* pkt = dst_buffer + offset;
        pkt[0] = 0x80;  // Version 2
        pkt[1] = 96 | ((offset + payload_size >= size) ? 0x80 : 0);  // Marker on last
        pkt[2] = (packetizer->sequence >> 8) & 0xFF;
        pkt[3] = packetizer->sequence & 0xFF;
        pkt[4] = (timestamp >> 24) & 0xFF;
        pkt[5] = (timestamp >> 16) & 0xFF;
        pkt[6] = (timestamp >> 8) & 0xFF;
        pkt[7] = timestamp & 0xFF;
        pkt[8] = (packetizer->ssrc >> 24) & 0xFF;
        pkt[9] = (packetizer->ssrc >> 16) & 0xFF;
        pkt[10] = (packetizer->ssrc >> 8) & 0xFF;
        pkt[11] = packetizer->ssrc & 0xFF;

        // Copy payload
        memcpy(pkt + 12, data + offset, payload_size);

        dst_offsets[packet_count] = (pkt - dst_buffer);
        dst_sizes[packet_count] = 12 + payload_size;

        packetizer->sequence++;
        offset += payload_size;
        packet_count++;
    }

    *out_count = packet_count;
    return SHIM_OK;
}

SHIM_EXPORT uint16_t shim_packetizer_sequence_number(ShimPacketizer* packetizer) {
    return packetizer ? packetizer->sequence : 0;
}

SHIM_EXPORT void shim_packetizer_destroy(ShimPacketizer* packetizer) {
    delete packetizer;
}

/* ============================================================================
 * Depacketizer
 * ========================================================================== */

SHIM_EXPORT ShimDepacketizer* shim_depacketizer_create(ShimCodecType codec) {
    auto dpkt = new ShimDepacketizer();
    dpkt->codec = codec;
    return dpkt;
}

SHIM_EXPORT int shim_depacketizer_push(
    ShimDepacketizer* depacketizer,
    const uint8_t* data,
    int size
) {
    if (!depacketizer || !data || size < 12) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    // Skip RTP header, accumulate payload
    depacketizer->buffer.insert(
        depacketizer->buffer.end(),
        data + 12,
        data + size
    );

    return SHIM_OK;
}

SHIM_EXPORT int shim_depacketizer_pop(
    ShimDepacketizer* depacketizer,
    uint8_t* dst_buffer,
    int* out_size,
    uint32_t* out_timestamp,
    int* out_is_keyframe
) {
    if (!depacketizer || depacketizer->buffer.empty()) {
        return SHIM_ERROR_NEED_MORE_DATA;
    }

    if (dst_buffer) {
        memcpy(dst_buffer, depacketizer->buffer.data(), depacketizer->buffer.size());
    }
    *out_size = depacketizer->buffer.size();
    *out_timestamp = 0;
    *out_is_keyframe = 1;

    depacketizer->buffer.clear();
    return SHIM_OK;
}

SHIM_EXPORT void shim_depacketizer_destroy(ShimDepacketizer* depacketizer) {
    delete depacketizer;
}

/* ============================================================================
 * PeerConnection
 * ========================================================================== */

SHIM_EXPORT ShimPeerConnection* shim_peer_connection_create(
    const ShimPeerConnectionConfig* config
) {
    auto pc = new ShimPeerConnection();
    pc->signaling_state = 0;  // stable
    pc->ice_connection_state = 0;  // new
    pc->ice_gathering_state = 0;  // new
    pc->connection_state = 0;  // new
    return pc;
}

SHIM_EXPORT void shim_peer_connection_destroy(ShimPeerConnection* pc) {
    delete pc;
}

SHIM_EXPORT void shim_peer_connection_set_on_ice_candidate(
    ShimPeerConnection* pc,
    ShimOnICECandidate callback,
    void* ctx
) {
    if (pc) {
        pc->on_ice_candidate = callback;
        pc->ice_candidate_ctx = ctx;
    }
}

SHIM_EXPORT void shim_peer_connection_set_on_connection_state_change(
    ShimPeerConnection* pc,
    ShimOnConnectionStateChange callback,
    void* ctx
) {
    if (pc) {
        pc->on_connection_state = callback;
        pc->connection_state_ctx = ctx;
    }
}

SHIM_EXPORT void shim_peer_connection_set_on_track(
    ShimPeerConnection* pc,
    ShimOnTrack callback,
    void* ctx
) {
    if (pc) {
        pc->on_track = callback;
        pc->track_ctx = ctx;
    }
}

SHIM_EXPORT void shim_peer_connection_set_on_data_channel(
    ShimPeerConnection* pc,
    ShimOnDataChannel callback,
    void* ctx
) {
    if (pc) {
        pc->on_data_channel = callback;
        pc->data_channel_ctx = ctx;
    }
}

SHIM_EXPORT void shim_peer_connection_set_on_signaling_state_change(
    ShimPeerConnection* pc,
    ShimOnSignalingStateChange callback,
    void* ctx
) {
    // Stub
}

SHIM_EXPORT void shim_peer_connection_set_on_ice_connection_state_change(
    ShimPeerConnection* pc,
    ShimOnICEConnectionStateChange callback,
    void* ctx
) {
    // Stub
}

SHIM_EXPORT void shim_peer_connection_set_on_ice_gathering_state_change(
    ShimPeerConnection* pc,
    ShimOnICEGatheringStateChange callback,
    void* ctx
) {
    // Stub
}

SHIM_EXPORT void shim_peer_connection_set_on_negotiation_needed(
    ShimPeerConnection* pc,
    ShimOnNegotiationNeeded callback,
    void* ctx
) {
    // Stub
}

SHIM_EXPORT int shim_peer_connection_create_offer(
    ShimPeerConnection* pc,
    char* sdp_out,
    int sdp_out_size,
    int* out_sdp_len
) {
    if (!pc || !sdp_out || !out_sdp_len) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    const char* fake_sdp =
        "v=0\r\n"
        "o=- 1234567890 1 IN IP4 127.0.0.1\r\n"
        "s=-\r\n"
        "t=0 0\r\n"
        "a=group:BUNDLE 0\r\n"
        "m=video 9 UDP/TLS/RTP/SAVPF 96\r\n"
        "c=IN IP4 0.0.0.0\r\n"
        "a=rtcp:9 IN IP4 0.0.0.0\r\n"
        "a=ice-ufrag:stub\r\n"
        "a=ice-pwd:stubstubstubstubstubstub\r\n"
        "a=fingerprint:sha-256 00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00\r\n"
        "a=setup:actpass\r\n"
        "a=mid:0\r\n"
        "a=sendrecv\r\n"
        "a=rtpmap:96 VP8/90000\r\n";

    int len = strlen(fake_sdp);
    if (len >= sdp_out_size) {
        return SHIM_ERROR_BUFFER_TOO_SMALL;
    }

    memcpy(sdp_out, fake_sdp, len);
    sdp_out[len] = '\0';
    *out_sdp_len = len;

    pc->local_sdp = fake_sdp;
    pc->signaling_state = 1;  // have-local-offer

    return SHIM_OK;
}

SHIM_EXPORT int shim_peer_connection_create_answer(
    ShimPeerConnection* pc,
    char* sdp_out,
    int sdp_out_size,
    int* out_sdp_len
) {
    if (!pc || !sdp_out || !out_sdp_len) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    const char* fake_sdp =
        "v=0\r\n"
        "o=- 1234567890 1 IN IP4 127.0.0.1\r\n"
        "s=-\r\n"
        "t=0 0\r\n"
        "a=group:BUNDLE 0\r\n"
        "m=video 9 UDP/TLS/RTP/SAVPF 96\r\n"
        "c=IN IP4 0.0.0.0\r\n"
        "a=rtcp:9 IN IP4 0.0.0.0\r\n"
        "a=ice-ufrag:stub\r\n"
        "a=ice-pwd:stubstubstubstubstubstub\r\n"
        "a=fingerprint:sha-256 00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00\r\n"
        "a=setup:active\r\n"
        "a=mid:0\r\n"
        "a=sendrecv\r\n"
        "a=rtpmap:96 VP8/90000\r\n";

    int len = strlen(fake_sdp);
    if (len >= sdp_out_size) {
        return SHIM_ERROR_BUFFER_TOO_SMALL;
    }

    memcpy(sdp_out, fake_sdp, len);
    sdp_out[len] = '\0';
    *out_sdp_len = len;

    pc->local_sdp = fake_sdp;

    return SHIM_OK;
}

SHIM_EXPORT int shim_peer_connection_set_local_description(
    ShimPeerConnection* pc,
    int type,
    const char* sdp
) {
    if (!pc || !sdp) return SHIM_ERROR_INVALID_PARAM;
    pc->local_sdp = sdp;
    pc->signaling_state = 0;  // stable after answer
    return SHIM_OK;
}

SHIM_EXPORT int shim_peer_connection_set_remote_description(
    ShimPeerConnection* pc,
    int type,
    const char* sdp
) {
    if (!pc || !sdp) return SHIM_ERROR_INVALID_PARAM;
    pc->remote_sdp = sdp;
    return SHIM_OK;
}

SHIM_EXPORT int shim_peer_connection_add_ice_candidate(
    ShimPeerConnection* pc,
    const char* candidate,
    const char* sdp_mid,
    int sdp_mline_index
) {
    if (!pc) return SHIM_ERROR_INVALID_PARAM;
    return SHIM_OK;
}

SHIM_EXPORT int shim_peer_connection_signaling_state(ShimPeerConnection* pc) {
    return pc ? pc->signaling_state : 0;
}

SHIM_EXPORT int shim_peer_connection_ice_connection_state(ShimPeerConnection* pc) {
    return pc ? pc->ice_connection_state : 0;
}

SHIM_EXPORT int shim_peer_connection_ice_gathering_state(ShimPeerConnection* pc) {
    return pc ? pc->ice_gathering_state : 0;
}

SHIM_EXPORT int shim_peer_connection_connection_state(ShimPeerConnection* pc) {
    return pc ? pc->connection_state : 0;
}

SHIM_EXPORT ShimRTPSender* shim_peer_connection_add_track(
    ShimPeerConnection* pc,
    ShimCodecType codec,
    const char* track_id,
    const char* stream_id
) {
    if (!pc || !track_id) return nullptr;

    auto sender = new ShimRTPSender();
    sender->handle = next_handle();
    sender->codec = codec;
    sender->track_id = track_id;
    return sender;
}

SHIM_EXPORT int shim_peer_connection_remove_track(
    ShimPeerConnection* pc,
    ShimRTPSender* sender
) {
    return SHIM_OK;
}

SHIM_EXPORT ShimDataChannel* shim_peer_connection_create_data_channel(
    ShimPeerConnection* pc,
    const char* label,
    int ordered,
    int max_retransmits,
    const char* protocol
) {
    if (!pc || !label) return nullptr;

    auto dc = new ShimDataChannel();
    dc->label = label;
    dc->ready_state = 1;  // open
    return dc;
}

SHIM_EXPORT void shim_peer_connection_close(ShimPeerConnection* pc) {
    if (pc) {
        pc->connection_state = 5;  // closed
        pc->signaling_state = 5;  // closed
    }
}

SHIM_EXPORT int shim_peer_connection_get_stats(
    ShimPeerConnection* pc,
    ShimRTCStats* out_stats
) {
    if (!pc || !out_stats) return SHIM_ERROR_INVALID_PARAM;

    memset(out_stats, 0, sizeof(ShimRTCStats));
    out_stats->timestamp_us = 1000000;
    out_stats->bytes_sent = 1024;
    out_stats->bytes_received = 2048;
    out_stats->packets_sent = 10;
    out_stats->packets_received = 20;
    out_stats->round_trip_time_ms = 50.0;

    return SHIM_OK;
}

SHIM_EXPORT int shim_peer_connection_restart_ice(ShimPeerConnection* pc) {
    return SHIM_OK;
}

/* ============================================================================
 * RTPSender
 * ========================================================================== */

SHIM_EXPORT int shim_rtp_sender_set_bitrate(ShimRTPSender* sender, uint32_t bitrate) {
    return SHIM_OK;
}

SHIM_EXPORT int shim_rtp_sender_replace_track(ShimRTPSender* sender, void* track) {
    return SHIM_OK;
}

SHIM_EXPORT void shim_rtp_sender_destroy(ShimRTPSender* sender) {
    delete sender;
}

SHIM_EXPORT int shim_rtp_sender_get_parameters(
    ShimRTPSender* sender,
    ShimRTPSendParameters* out_params,
    ShimRTPEncodingParameters* encodings,
    int max_encodings
) {
    if (!sender || !out_params) return SHIM_ERROR_INVALID_PARAM;
    out_params->encoding_count = 0;
    return SHIM_OK;
}

SHIM_EXPORT int shim_rtp_sender_set_parameters(
    ShimRTPSender* sender,
    const ShimRTPSendParameters* params
) {
    return SHIM_OK;
}

SHIM_EXPORT void* shim_rtp_sender_get_track(ShimRTPSender* sender) {
    return nullptr;
}

SHIM_EXPORT int shim_rtp_sender_get_stats(ShimRTPSender* sender, ShimRTCStats* out_stats) {
    if (!out_stats) return SHIM_ERROR_INVALID_PARAM;
    memset(out_stats, 0, sizeof(ShimRTCStats));
    return SHIM_OK;
}

SHIM_EXPORT int shim_rtp_sender_set_layer_active(ShimRTPSender* sender, const char* rid, int active) {
    return SHIM_OK;
}

SHIM_EXPORT int shim_rtp_sender_set_layer_bitrate(ShimRTPSender* sender, const char* rid, uint32_t max_bitrate_bps) {
    return SHIM_OK;
}

SHIM_EXPORT int shim_rtp_sender_get_active_layers(ShimRTPSender* sender, int* out_spatial, int* out_temporal) {
    if (out_spatial) *out_spatial = 1;
    if (out_temporal) *out_temporal = 1;
    return SHIM_OK;
}

SHIM_EXPORT void shim_rtp_sender_set_on_rtcp_feedback(ShimRTPSender* sender, ShimOnRTCPFeedback callback, void* ctx) {
    // Stub
}

SHIM_EXPORT int shim_rtp_sender_set_scalability_mode(ShimRTPSender* sender, const char* mode) {
    return SHIM_OK;
}

SHIM_EXPORT int shim_rtp_sender_get_scalability_mode(ShimRTPSender* sender, char* mode_out, int mode_out_size) {
    if (mode_out && mode_out_size > 0) {
        strncpy(mode_out, "L1T1", mode_out_size - 1);
        mode_out[mode_out_size - 1] = '\0';
    }
    return SHIM_OK;
}

/* ============================================================================
 * RTPReceiver
 * ========================================================================== */

SHIM_EXPORT void* shim_rtp_receiver_get_track(ShimRTPReceiver* receiver) {
    return nullptr;
}

SHIM_EXPORT int shim_rtp_receiver_get_stats(ShimRTPReceiver* receiver, ShimRTCStats* out_stats) {
    if (!out_stats) return SHIM_ERROR_INVALID_PARAM;
    memset(out_stats, 0, sizeof(ShimRTCStats));
    return SHIM_OK;
}

SHIM_EXPORT int shim_rtp_receiver_request_keyframe(ShimRTPReceiver* receiver) {
    return SHIM_OK;
}

/* ============================================================================
 * RTPTransceiver
 * ========================================================================== */

SHIM_EXPORT int shim_transceiver_get_direction(ShimRTPTransceiver* transceiver) {
    return transceiver ? transceiver->direction : 0;
}

SHIM_EXPORT int shim_transceiver_set_direction(ShimRTPTransceiver* transceiver, int direction) {
    if (transceiver) transceiver->direction = direction;
    return SHIM_OK;
}

SHIM_EXPORT int shim_transceiver_get_current_direction(ShimRTPTransceiver* transceiver) {
    return transceiver ? transceiver->direction : 0;
}

SHIM_EXPORT int shim_transceiver_stop(ShimRTPTransceiver* transceiver) {
    return SHIM_OK;
}

SHIM_EXPORT const char* shim_transceiver_mid(ShimRTPTransceiver* transceiver) {
    return transceiver ? transceiver->mid.c_str() : "";
}

SHIM_EXPORT ShimRTPSender* shim_transceiver_get_sender(ShimRTPTransceiver* transceiver) {
    return transceiver ? transceiver->sender : nullptr;
}

SHIM_EXPORT ShimRTPReceiver* shim_transceiver_get_receiver(ShimRTPTransceiver* transceiver) {
    return transceiver ? transceiver->receiver : nullptr;
}

SHIM_EXPORT ShimRTPTransceiver* shim_peer_connection_add_transceiver(
    ShimPeerConnection* pc,
    int kind,
    int direction
) {
    auto t = new ShimRTPTransceiver();
    t->handle = next_handle();
    t->direction = direction;
    t->mid = "0";
    t->sender = new ShimRTPSender();
    t->receiver = new ShimRTPReceiver();
    return t;
}

SHIM_EXPORT int shim_peer_connection_get_senders(
    ShimPeerConnection* pc,
    ShimRTPSender** senders,
    int max_senders,
    int* out_count
) {
    if (out_count) *out_count = 0;
    return SHIM_OK;
}

SHIM_EXPORT int shim_peer_connection_get_receivers(
    ShimPeerConnection* pc,
    ShimRTPReceiver** receivers,
    int max_receivers,
    int* out_count
) {
    if (out_count) *out_count = 0;
    return SHIM_OK;
}

SHIM_EXPORT int shim_peer_connection_get_transceivers(
    ShimPeerConnection* pc,
    ShimRTPTransceiver** transceivers,
    int max_transceivers,
    int* out_count
) {
    if (out_count) *out_count = 0;
    return SHIM_OK;
}

/* ============================================================================
 * VideoTrackSource
 * ========================================================================== */

SHIM_EXPORT ShimVideoTrackSource* shim_video_track_source_create(
    ShimPeerConnection* pc,
    int width,
    int height
) {
    auto src = new ShimVideoTrackSource();
    src->width = width;
    src->height = height;
    return src;
}

SHIM_EXPORT int shim_video_track_source_push_frame(
    ShimVideoTrackSource* source,
    const uint8_t* y_plane,
    const uint8_t* u_plane,
    const uint8_t* v_plane,
    int y_stride,
    int u_stride,
    int v_stride,
    int64_t timestamp_us
) {
    return SHIM_OK;
}

SHIM_EXPORT ShimRTPSender* shim_peer_connection_add_video_track_from_source(
    ShimPeerConnection* pc,
    ShimVideoTrackSource* source,
    const char* track_id,
    const char* stream_id
) {
    auto sender = new ShimRTPSender();
    sender->handle = next_handle();
    sender->track_id = track_id ? track_id : "";
    return sender;
}

SHIM_EXPORT void shim_video_track_source_destroy(ShimVideoTrackSource* source) {
    delete source;
}

/* ============================================================================
 * AudioTrackSource
 * ========================================================================== */

SHIM_EXPORT ShimAudioTrackSource* shim_audio_track_source_create(
    ShimPeerConnection* pc,
    int sample_rate,
    int channels
) {
    auto src = new ShimAudioTrackSource();
    src->sample_rate = sample_rate;
    src->channels = channels;
    return src;
}

SHIM_EXPORT int shim_audio_track_source_push_frame(
    ShimAudioTrackSource* source,
    const int16_t* samples,
    int num_samples,
    int64_t timestamp_us
) {
    return SHIM_OK;
}

SHIM_EXPORT ShimRTPSender* shim_peer_connection_add_audio_track_from_source(
    ShimPeerConnection* pc,
    ShimAudioTrackSource* source,
    const char* track_id,
    const char* stream_id
) {
    auto sender = new ShimRTPSender();
    sender->handle = next_handle();
    sender->track_id = track_id ? track_id : "";
    return sender;
}

SHIM_EXPORT void shim_audio_track_source_destroy(ShimAudioTrackSource* source) {
    delete source;
}

/* ============================================================================
 * Track Sinks
 * ========================================================================== */

SHIM_EXPORT int shim_track_set_video_sink(void* track, ShimOnVideoFrame callback, void* ctx) {
    return SHIM_OK;
}

SHIM_EXPORT int shim_track_set_audio_sink(void* track, ShimOnAudioFrame callback, void* ctx) {
    return SHIM_OK;
}

SHIM_EXPORT void shim_track_remove_video_sink(void* track) {}
SHIM_EXPORT void shim_track_remove_audio_sink(void* track) {}

SHIM_EXPORT const char* shim_track_kind(void* track) {
    return "video";
}

SHIM_EXPORT const char* shim_track_id(void* track) {
    return "track0";
}

/* ============================================================================
 * DataChannel
 * ========================================================================== */

SHIM_EXPORT void shim_data_channel_set_on_message(
    ShimDataChannel* dc,
    ShimOnDataChannelMessage callback,
    void* ctx
) {
    if (dc) {
        dc->on_message = callback;
        dc->message_ctx = ctx;
    }
}

SHIM_EXPORT void shim_data_channel_set_on_open(ShimDataChannel* dc, ShimOnDataChannelOpen callback, void* ctx) {}
SHIM_EXPORT void shim_data_channel_set_on_close(ShimDataChannel* dc, ShimOnDataChannelClose callback, void* ctx) {}

SHIM_EXPORT int shim_data_channel_send(ShimDataChannel* dc, const uint8_t* data, int size, int is_binary) {
    return SHIM_OK;
}

SHIM_EXPORT const char* shim_data_channel_label(ShimDataChannel* dc) {
    return dc ? dc->label.c_str() : "";
}

SHIM_EXPORT int shim_data_channel_ready_state(ShimDataChannel* dc) {
    return dc ? dc->ready_state : 0;
}

SHIM_EXPORT void shim_data_channel_close(ShimDataChannel* dc) {
    if (dc) dc->ready_state = 3;  // closed
}

SHIM_EXPORT void shim_data_channel_destroy(ShimDataChannel* dc) {
    delete dc;
}

/* ============================================================================
 * Device Enumeration
 * ========================================================================== */

SHIM_EXPORT int shim_enumerate_devices(ShimDeviceInfo* devices, int max_devices, int* out_count) {
    if (!out_count) return SHIM_ERROR_INVALID_PARAM;

    if (devices && max_devices > 0) {
        strncpy(devices[0].device_id, "default", sizeof(devices[0].device_id) - 1);
        strncpy(devices[0].label, "Default Camera", sizeof(devices[0].label) - 1);
        devices[0].kind = 0;  // videoinput
        *out_count = 1;
    } else {
        *out_count = 0;
    }

    return SHIM_OK;
}

SHIM_EXPORT int shim_enumerate_screens(ShimScreenInfo* screens, int max_screens, int* out_count) {
    if (!out_count) return SHIM_ERROR_INVALID_PARAM;

    if (screens && max_screens > 0) {
        screens[0].id = 0;
        strncpy(screens[0].title, "Main Display", sizeof(screens[0].title) - 1);
        screens[0].is_window = 0;
        *out_count = 1;
    } else {
        *out_count = 0;
    }

    return SHIM_OK;
}

/* ============================================================================
 * Video Capture (stub)
 * ========================================================================== */

struct ShimVideoCapture { int dummy; };

SHIM_EXPORT ShimVideoCapture* shim_video_capture_create(const char* device_id, int width, int height, int fps) {
    return new ShimVideoCapture();
}

SHIM_EXPORT int shim_video_capture_start(ShimVideoCapture* cap, ShimVideoCaptureCallback callback, void* ctx) {
    return SHIM_OK;
}

SHIM_EXPORT void shim_video_capture_stop(ShimVideoCapture* cap) {}
SHIM_EXPORT void shim_video_capture_destroy(ShimVideoCapture* cap) { delete cap; }

/* ============================================================================
 * Audio Capture (stub)
 * ========================================================================== */

struct ShimAudioCapture { int dummy; };

SHIM_EXPORT ShimAudioCapture* shim_audio_capture_create(const char* device_id, int sample_rate, int channels) {
    return new ShimAudioCapture();
}

SHIM_EXPORT int shim_audio_capture_start(ShimAudioCapture* cap, ShimAudioCaptureCallback callback, void* ctx) {
    return SHIM_OK;
}

SHIM_EXPORT void shim_audio_capture_stop(ShimAudioCapture* cap) {}
SHIM_EXPORT void shim_audio_capture_destroy(ShimAudioCapture* cap) { delete cap; }

/* ============================================================================
 * Screen Capture (stub)
 * ========================================================================== */

struct ShimScreenCapture { int dummy; };

SHIM_EXPORT ShimScreenCapture* shim_screen_capture_create(int64_t screen_id, int is_window, int fps) {
    return new ShimScreenCapture();
}

SHIM_EXPORT int shim_screen_capture_start(ShimScreenCapture* cap, ShimVideoCaptureCallback callback, void* ctx) {
    return SHIM_OK;
}

SHIM_EXPORT void shim_screen_capture_stop(ShimScreenCapture* cap) {}
SHIM_EXPORT void shim_screen_capture_destroy(ShimScreenCapture* cap) { delete cap; }

/* ============================================================================
 * Codec Capabilities
 * ========================================================================== */

SHIM_EXPORT int shim_get_supported_video_codecs(ShimCodecCapability* codecs, int max_codecs, int* out_count) {
    if (!out_count) return SHIM_ERROR_INVALID_PARAM;

    int count = 0;
    if (codecs && max_codecs > count) {
        strncpy(codecs[count].mime_type, "video/VP8", sizeof(codecs[count].mime_type));
        codecs[count].clock_rate = 90000;
        codecs[count].channels = 0;
        codecs[count].payload_type = 96;
        count++;
    }
    if (codecs && max_codecs > count) {
        strncpy(codecs[count].mime_type, "video/VP9", sizeof(codecs[count].mime_type));
        codecs[count].clock_rate = 90000;
        codecs[count].channels = 0;
        codecs[count].payload_type = 98;
        count++;
    }
    if (codecs && max_codecs > count) {
        strncpy(codecs[count].mime_type, "video/H264", sizeof(codecs[count].mime_type));
        codecs[count].clock_rate = 90000;
        codecs[count].channels = 0;
        codecs[count].payload_type = 102;
        count++;
    }

    *out_count = count;
    return SHIM_OK;
}

SHIM_EXPORT int shim_get_supported_audio_codecs(ShimCodecCapability* codecs, int max_codecs, int* out_count) {
    if (!out_count) return SHIM_ERROR_INVALID_PARAM;

    int count = 0;
    if (codecs && max_codecs > count) {
        strncpy(codecs[count].mime_type, "audio/opus", sizeof(codecs[count].mime_type));
        codecs[count].clock_rate = 48000;
        codecs[count].channels = 2;
        codecs[count].payload_type = 111;
        count++;
    }

    *out_count = count;
    return SHIM_OK;
}

SHIM_EXPORT int shim_is_codec_supported(const char* mime_type) {
    if (!mime_type) return 0;
    if (strstr(mime_type, "VP8") || strstr(mime_type, "vp8")) return 1;
    if (strstr(mime_type, "VP9") || strstr(mime_type, "vp9")) return 1;
    if (strstr(mime_type, "H264") || strstr(mime_type, "h264")) return 1;
    if (strstr(mime_type, "opus") || strstr(mime_type, "OPUS")) return 1;
    return 0;
}

/* ============================================================================
 * Bandwidth Estimation
 * ========================================================================== */

SHIM_EXPORT void shim_peer_connection_set_on_bandwidth_estimate(
    ShimPeerConnection* pc,
    ShimOnBandwidthEstimate callback,
    void* ctx
) {
    // Stub
}

SHIM_EXPORT int shim_peer_connection_get_bandwidth_estimate(
    ShimPeerConnection* pc,
    ShimBandwidthEstimate* out_estimate
) {
    if (!out_estimate) return SHIM_ERROR_INVALID_PARAM;

    memset(out_estimate, 0, sizeof(ShimBandwidthEstimate));
    out_estimate->target_bitrate_bps = 2000000;
    out_estimate->available_send_bps = 3000000;
    out_estimate->available_recv_bps = 3000000;

    return SHIM_OK;
}

/* ============================================================================
 * Memory helpers
 * ========================================================================== */

SHIM_EXPORT void shim_free_buffer(void* buffer) {
    free(buffer);
}

SHIM_EXPORT void shim_free_packets(void* packets, void* sizes, int count) {
    // Stub
}

}  // extern "C"
