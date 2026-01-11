/*
 * shim_packetizer.cc - RTP packetizer and depacketizer implementation
 *
 * Provides simple RTP packetization/depacketization for testing.
 * Note: Real packetization is done in the Go layer using pkg/packetizer.
 */

#include "shim_common.h"

#include <algorithm>
#include <cstring>
#include <vector>

/* ============================================================================
 * RTP Packetizer Implementation
 * ========================================================================== */

struct ShimPacketizer {
    ShimCodecType codec;
    uint32_t ssrc;
    uint8_t payload_type;
    uint16_t mtu;
    uint32_t clock_rate;
    uint16_t sequence_number;
    std::mutex mutex;
};

extern "C" {

SHIM_EXPORT ShimPacketizer* shim_packetizer_create(const ShimPacketizerConfig* config) {
    if (!config) {
        return nullptr;
    }

    auto packetizer = std::make_unique<ShimPacketizer>();
    packetizer->codec = static_cast<ShimCodecType>(config->codec);
    packetizer->ssrc = config->ssrc;
    packetizer->payload_type = config->payload_type;
    packetizer->mtu = config->mtu > 0 ? config->mtu : 1200;
    packetizer->clock_rate = config->clock_rate > 0 ? config->clock_rate : 90000;
    packetizer->sequence_number = 0;

    return packetizer.release();
}

SHIM_EXPORT int shim_packetizer_packetize(ShimPacketizerPacketizeParams* params) {
    if (!params) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    params->out_count = 0;
    if (!params->packetizer || !params->data || params->size <= 0 || !params->dst_buffer) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    std::lock_guard<std::mutex> lock(params->packetizer->mutex);

    // Simple packetization: split into MTU-sized chunks
    int payload_size = params->packetizer->mtu - 12;  // RTP header is 12 bytes
    int offset = 0;
    int packet_count = 0;
    int buffer_offset = 0;

    while (offset < params->size && packet_count < params->max_packets) {
        int chunk_size = std::min(payload_size, params->size - offset);
        bool is_last = (offset + chunk_size >= params->size);

        // Build RTP header
        uint8_t* packet = params->dst_buffer + buffer_offset;

        // Version (2), padding (0), extension (0), CSRC count (0)
        packet[0] = 0x80;
        // Marker bit, payload type
        packet[1] = (is_last ? 0x80 : 0x00) | params->packetizer->payload_type;
        // Sequence number
        packet[2] = (params->packetizer->sequence_number >> 8) & 0xFF;
        packet[3] = params->packetizer->sequence_number & 0xFF;
        // Timestamp
        packet[4] = (params->timestamp >> 24) & 0xFF;
        packet[5] = (params->timestamp >> 16) & 0xFF;
        packet[6] = (params->timestamp >> 8) & 0xFF;
        packet[7] = params->timestamp & 0xFF;
        // SSRC
        packet[8] = (params->packetizer->ssrc >> 24) & 0xFF;
        packet[9] = (params->packetizer->ssrc >> 16) & 0xFF;
        packet[10] = (params->packetizer->ssrc >> 8) & 0xFF;
        packet[11] = params->packetizer->ssrc & 0xFF;

        // Copy payload
        memcpy(packet + 12, params->data + offset, chunk_size);

        int packet_size = 12 + chunk_size;

        if (params->dst_offsets) params->dst_offsets[packet_count] = buffer_offset;
        if (params->dst_sizes) params->dst_sizes[packet_count] = packet_size;

        buffer_offset += packet_size;
        offset += chunk_size;
        packet_count++;
        params->packetizer->sequence_number++;
    }

    params->out_count = packet_count;
    return SHIM_OK;
}

SHIM_EXPORT uint16_t shim_packetizer_sequence_number(ShimPacketizer* packetizer) {
    if (!packetizer) return 0;
    return packetizer->sequence_number;
}

SHIM_EXPORT void shim_packetizer_destroy(ShimPacketizer* packetizer) {
    delete packetizer;
}

/* ============================================================================
 * RTP Depacketizer Implementation
 * ========================================================================== */

struct ShimDepacketizer {
    ShimCodecType codec;
    std::vector<uint8_t> frame_buffer;
    uint32_t current_timestamp;
    bool has_frame;
    bool is_keyframe;
    std::mutex mutex;
};

SHIM_EXPORT ShimDepacketizer* shim_depacketizer_create(ShimCodecType codec) {
    auto depacketizer = std::make_unique<ShimDepacketizer>();
    depacketizer->codec = codec;
    depacketizer->current_timestamp = 0;
    depacketizer->has_frame = false;
    depacketizer->is_keyframe = false;
    return depacketizer.release();
}

SHIM_EXPORT int shim_depacketizer_push(ShimDepacketizerPushParams* params) {
    if (!params) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    if (!params->depacketizer || !params->data || params->size < 12) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    std::lock_guard<std::mutex> lock(params->depacketizer->mutex);

    // Parse RTP header
    uint8_t marker = (params->data[1] >> 7) & 0x01;
    uint32_t timestamp =
        (static_cast<uint32_t>(params->data[4]) << 24) |
        (static_cast<uint32_t>(params->data[5]) << 16) |
        (static_cast<uint32_t>(params->data[6]) << 8) |
        static_cast<uint32_t>(params->data[7]);

    // Check if new frame
    if (timestamp != params->depacketizer->current_timestamp) {
        params->depacketizer->frame_buffer.clear();
        params->depacketizer->current_timestamp = timestamp;
        params->depacketizer->is_keyframe = false;
    }

    // Append payload (skip RTP header)
    params->depacketizer->frame_buffer.insert(
        params->depacketizer->frame_buffer.end(),
        params->data + 12,
        params->data + params->size
    );

    // Check marker bit for end of frame
    if (marker) {
        params->depacketizer->has_frame = true;

        // Simple keyframe detection (check NAL type for H264)
        if (params->depacketizer->codec == SHIM_CODEC_H264 && !params->depacketizer->frame_buffer.empty()) {
            uint8_t nal_type = params->depacketizer->frame_buffer[0] & 0x1F;
            params->depacketizer->is_keyframe = (nal_type == 5 || nal_type == 7);
        }
    }

    return SHIM_OK;
}

SHIM_EXPORT int shim_depacketizer_pop(ShimDepacketizerPopParams* params) {
    if (!params) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    params->out_size = 0;
    params->out_timestamp = 0;
    params->out_is_keyframe = 0;
    if (!params->depacketizer || !params->dst_buffer) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    std::lock_guard<std::mutex> lock(params->depacketizer->mutex);

    if (!params->depacketizer->has_frame) {
        return SHIM_ERROR_NEED_MORE_DATA;
    }

    size_t frame_size = params->depacketizer->frame_buffer.size();
    memcpy(params->dst_buffer, params->depacketizer->frame_buffer.data(), frame_size);

    params->out_size = static_cast<int>(frame_size);
    params->out_timestamp = params->depacketizer->current_timestamp;
    params->out_is_keyframe = params->depacketizer->is_keyframe ? 1 : 0;

    // Reset for next frame
    params->depacketizer->frame_buffer.clear();
    params->depacketizer->has_frame = false;

    return SHIM_OK;
}

SHIM_EXPORT void shim_depacketizer_destroy(ShimDepacketizer* depacketizer) {
    delete depacketizer;
}

}  // extern "C"
