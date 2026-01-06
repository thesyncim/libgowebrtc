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
    if (!packetizer || !data || size <= 0 || !dst_buffer || !out_count) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    std::lock_guard<std::mutex> lock(packetizer->mutex);

    // Simple packetization: split into MTU-sized chunks
    int payload_size = packetizer->mtu - 12;  // RTP header is 12 bytes
    int offset = 0;
    int packet_count = 0;
    int buffer_offset = 0;

    while (offset < size && packet_count < max_packets) {
        int chunk_size = std::min(payload_size, size - offset);
        bool is_last = (offset + chunk_size >= size);

        // Build RTP header
        uint8_t* packet = dst_buffer + buffer_offset;

        // Version (2), padding (0), extension (0), CSRC count (0)
        packet[0] = 0x80;
        // Marker bit, payload type
        packet[1] = (is_last ? 0x80 : 0x00) | packetizer->payload_type;
        // Sequence number
        packet[2] = (packetizer->sequence_number >> 8) & 0xFF;
        packet[3] = packetizer->sequence_number & 0xFF;
        // Timestamp
        packet[4] = (timestamp >> 24) & 0xFF;
        packet[5] = (timestamp >> 16) & 0xFF;
        packet[6] = (timestamp >> 8) & 0xFF;
        packet[7] = timestamp & 0xFF;
        // SSRC
        packet[8] = (packetizer->ssrc >> 24) & 0xFF;
        packet[9] = (packetizer->ssrc >> 16) & 0xFF;
        packet[10] = (packetizer->ssrc >> 8) & 0xFF;
        packet[11] = packetizer->ssrc & 0xFF;

        // Copy payload
        memcpy(packet + 12, data + offset, chunk_size);

        int packet_size = 12 + chunk_size;

        if (dst_offsets) dst_offsets[packet_count] = buffer_offset;
        if (dst_sizes) dst_sizes[packet_count] = packet_size;

        buffer_offset += packet_size;
        offset += chunk_size;
        packet_count++;
        packetizer->sequence_number++;
    }

    *out_count = packet_count;
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

SHIM_EXPORT int shim_depacketizer_push(
    ShimDepacketizer* depacketizer,
    const uint8_t* data,
    int size
) {
    if (!depacketizer || !data || size < 12) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    std::lock_guard<std::mutex> lock(depacketizer->mutex);

    // Parse RTP header
    uint8_t marker = (data[1] >> 7) & 0x01;
    uint32_t timestamp =
        (static_cast<uint32_t>(data[4]) << 24) |
        (static_cast<uint32_t>(data[5]) << 16) |
        (static_cast<uint32_t>(data[6]) << 8) |
        static_cast<uint32_t>(data[7]);

    // Check if new frame
    if (timestamp != depacketizer->current_timestamp) {
        depacketizer->frame_buffer.clear();
        depacketizer->current_timestamp = timestamp;
        depacketizer->is_keyframe = false;
    }

    // Append payload (skip RTP header)
    depacketizer->frame_buffer.insert(
        depacketizer->frame_buffer.end(),
        data + 12,
        data + size
    );

    // Check marker bit for end of frame
    if (marker) {
        depacketizer->has_frame = true;

        // Simple keyframe detection (check NAL type for H264)
        if (depacketizer->codec == SHIM_CODEC_H264 && !depacketizer->frame_buffer.empty()) {
            uint8_t nal_type = depacketizer->frame_buffer[0] & 0x1F;
            depacketizer->is_keyframe = (nal_type == 5 || nal_type == 7);
        }
    }

    return SHIM_OK;
}

SHIM_EXPORT int shim_depacketizer_pop(
    ShimDepacketizer* depacketizer,
    uint8_t* dst_buffer,
    int* out_size,
    uint32_t* out_timestamp,
    int* out_is_keyframe
) {
    if (!depacketizer || !dst_buffer || !out_size) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    std::lock_guard<std::mutex> lock(depacketizer->mutex);

    if (!depacketizer->has_frame) {
        return SHIM_ERROR_NEED_MORE_DATA;
    }

    size_t frame_size = depacketizer->frame_buffer.size();
    memcpy(dst_buffer, depacketizer->frame_buffer.data(), frame_size);

    *out_size = static_cast<int>(frame_size);
    if (out_timestamp) *out_timestamp = depacketizer->current_timestamp;
    if (out_is_keyframe) *out_is_keyframe = depacketizer->is_keyframe ? 1 : 0;

    // Reset for next frame
    depacketizer->frame_buffer.clear();
    depacketizer->has_frame = false;

    return SHIM_OK;
}

SHIM_EXPORT void shim_depacketizer_destroy(ShimDepacketizer* depacketizer) {
    delete depacketizer;
}

}  // extern "C"
