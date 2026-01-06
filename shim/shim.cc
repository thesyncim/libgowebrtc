/*
 * libwebrtc_shim - C API wrapper for libwebrtc
 *
 * This implementation wraps libwebrtc's C++ API with a C interface
 * suitable for FFI bindings (Go via purego).
 */

#include "shim.h"

#include <algorithm>
#include <cstring>
#include <chrono>
#include <map>
#include <memory>
#include <mutex>
#include <vector>
#include <string>
#include <atomic>
#include <thread>

// libwebrtc includes
#include "api/video_codecs/video_encoder.h"
#include "api/video_codecs/video_decoder.h"
#include "api/video_codecs/builtin_video_encoder_factory.h"
#include "api/video_codecs/builtin_video_decoder_factory.h"
#include "api/audio_codecs/audio_encoder.h"
#include "api/audio_codecs/audio_decoder.h"
#include "api/audio_codecs/opus/audio_encoder_opus.h"
#include "api/audio_codecs/opus/audio_decoder_opus.h"
#include "api/video/i420_buffer.h"
#include "api/video/video_frame.h"
#include "api/peer_connection_interface.h"
#include "api/create_peerconnection_factory.h"
#include "api/task_queue/default_task_queue_factory.h"
#include "api/audio_codecs/builtin_audio_encoder_factory.h"
#include "api/audio_codecs/builtin_audio_decoder_factory.h"
#include "rtc_base/thread.h"
// Note: Packetization is done in Go layer using pkg/packetizer
// The shim only provides simple RTP framing for testing

namespace {

// Version strings
const char* kShimVersion = "1.0.0";
const char* kLibWebRTCVersion = "M141";  // crow-misia/libwebrtc-bin 141.7390.2.0

// Global initialization state
std::once_flag g_init_flag;
std::unique_ptr<rtc::Thread> g_signaling_thread;
std::unique_ptr<rtc::Thread> g_worker_thread;
std::unique_ptr<rtc::Thread> g_network_thread;

void InitializeGlobals() {
    std::call_once(g_init_flag, []() {
        g_signaling_thread = rtc::Thread::Create();
        g_signaling_thread->SetName("signaling_thread", nullptr);
        g_signaling_thread->Start();

        g_worker_thread = rtc::Thread::Create();
        g_worker_thread->SetName("worker_thread", nullptr);
        g_worker_thread->Start();

        g_network_thread = rtc::Thread::CreateWithSocketServer();
        g_network_thread->SetName("network_thread", nullptr);
        g_network_thread->Start();
    });
}

// Convert codec type
webrtc::VideoCodecType ToWebRTCCodecType(ShimCodecType codec) {
    switch (codec) {
        case SHIM_CODEC_H264: return webrtc::kVideoCodecH264;
        case SHIM_CODEC_VP8:  return webrtc::kVideoCodecVP8;
        case SHIM_CODEC_VP9:  return webrtc::kVideoCodecVP9;
        case SHIM_CODEC_AV1:  return webrtc::kVideoCodecAV1;
        default: return webrtc::kVideoCodecGeneric;
    }
}

std::string CodecTypeToString(ShimCodecType codec) {
    switch (codec) {
        case SHIM_CODEC_H264: return "H264";
        case SHIM_CODEC_VP8:  return "VP8";
        case SHIM_CODEC_VP9:  return "VP9";
        case SHIM_CODEC_AV1:  return "AV1";
        default: return "unknown";
    }
}

}  // namespace

/* ============================================================================
 * Video Encoder Implementation
 * ========================================================================== */

struct ShimVideoEncoder {
    std::unique_ptr<webrtc::VideoEncoder> encoder;
    webrtc::VideoCodec codec_settings;
    ShimCodecType codec_type;
    std::mutex mutex;
    std::atomic<bool> force_keyframe{false};

    // Encoded frame callback storage
    std::vector<uint8_t> encoded_data;
    bool is_keyframe = false;
    bool has_output = false;
};

// Video encoder callback adapter
class EncoderCallback : public webrtc::EncodedImageCallback {
public:
    explicit EncoderCallback(ShimVideoEncoder* enc) : encoder_(enc) {}

    webrtc::EncodedImageCallback::Result OnEncodedImage(
        const webrtc::EncodedImage& encoded_image,
        const webrtc::CodecSpecificInfo* codec_specific_info) override {

        std::lock_guard<std::mutex> lock(encoder_->mutex);

        encoder_->encoded_data.assign(
            encoded_image.data(),
            encoded_image.data() + encoded_image.size()
        );
        encoder_->is_keyframe = (encoded_image._frameType == webrtc::VideoFrameType::kVideoFrameKey);
        encoder_->has_output = true;

        return webrtc::EncodedImageCallback::Result(
            webrtc::EncodedImageCallback::Result::OK);
    }

private:
    ShimVideoEncoder* encoder_;
};

extern "C" {

SHIM_EXPORT ShimVideoEncoder* shim_video_encoder_create(
    ShimCodecType codec,
    const ShimVideoEncoderConfig* config
) {
    if (!config || config->width <= 0 || config->height <= 0) {
        return nullptr;
    }

    auto factory = webrtc::CreateBuiltinVideoEncoderFactory();
    if (!factory) {
        return nullptr;
    }

    webrtc::SdpVideoFormat format(CodecTypeToString(codec));

    // Add H.264 profile if specified
    if (codec == SHIM_CODEC_H264 && config->h264_profile) {
        format.parameters["profile-level-id"] = config->h264_profile;
    }

    auto encoder = factory->CreateVideoEncoder(format);
    if (!encoder) {
        return nullptr;
    }

    auto shim_encoder = std::make_unique<ShimVideoEncoder>();
    shim_encoder->encoder = std::move(encoder);
    shim_encoder->codec_type = codec;

    // Configure codec settings
    webrtc::VideoCodec& settings = shim_encoder->codec_settings;
    memset(&settings, 0, sizeof(settings));

    settings.codecType = ToWebRTCCodecType(codec);
    settings.width = static_cast<uint16_t>(config->width);
    settings.height = static_cast<uint16_t>(config->height);
    settings.startBitrate = config->bitrate_bps / 1000;
    settings.maxBitrate = config->bitrate_bps / 1000;
    settings.minBitrate = 100;
    settings.maxFramerate = static_cast<uint32_t>(config->framerate);

    if (codec == SHIM_CODEC_H264) {
        settings.H264()->numberOfTemporalLayers = 1;
    } else if (codec == SHIM_CODEC_VP8) {
        settings.VP8()->numberOfTemporalLayers = 1;
    } else if (codec == SHIM_CODEC_VP9) {
        settings.VP9()->numberOfTemporalLayers = 1;
        settings.VP9()->numberOfSpatialLayers = 1;
    }

    // Initialize encoder
    webrtc::VideoEncoder::Settings encoder_settings(
        webrtc::VideoEncoder::Capabilities(false),  // loss_notification
        1,  // number_of_cores
        1000  // max_payload_size
    );

    auto callback = std::make_unique<EncoderCallback>(shim_encoder.get());

    if (shim_encoder->encoder->InitEncode(&settings, encoder_settings) != WEBRTC_VIDEO_CODEC_OK) {
        return nullptr;
    }

    shim_encoder->encoder->RegisterEncodeCompleteCallback(callback.release());

    return shim_encoder.release();
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
    if (!encoder || !y_plane || !u_plane || !v_plane || !dst_buffer || !out_size) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    std::lock_guard<std::mutex> lock(encoder->mutex);

    int width = encoder->codec_settings.width;
    int height = encoder->codec_settings.height;

    // Create I420 buffer from input planes
    webrtc::scoped_refptr<webrtc::I420Buffer> buffer =
        webrtc::I420Buffer::Copy(
            width, height,
            y_plane, y_stride,
            u_plane, u_stride,
            v_plane, v_stride
        );

    if (!buffer) {
        return SHIM_ERROR_OUT_OF_MEMORY;
    }

    // Create video frame
    webrtc::VideoFrame frame = webrtc::VideoFrame::Builder()
        .set_video_frame_buffer(buffer)
        .set_timestamp_rtp(timestamp)
        .set_timestamp_ms(timestamp / 90)  // Convert from 90kHz to ms
        .build();

    // Determine frame types
    std::vector<webrtc::VideoFrameType> frame_types;
    if (force_keyframe || encoder->force_keyframe.exchange(false)) {
        frame_types.push_back(webrtc::VideoFrameType::kVideoFrameKey);
    } else {
        frame_types.push_back(webrtc::VideoFrameType::kVideoFrameDelta);
    }

    // Reset output state
    encoder->has_output = false;
    encoder->encoded_data.clear();

    // Encode
    int result = encoder->encoder->Encode(frame, &frame_types);
    if (result != WEBRTC_VIDEO_CODEC_OK) {
        return SHIM_ERROR_ENCODE_FAILED;
    }

    // Wait for callback (synchronous in most implementations)
    if (!encoder->has_output) {
        *out_size = 0;
        return SHIM_OK;
    }

    // Copy encoded data to output buffer
    size_t encoded_size = encoder->encoded_data.size();
    memcpy(dst_buffer, encoder->encoded_data.data(), encoded_size);
    *out_size = static_cast<int>(encoded_size);

    if (out_is_keyframe) {
        *out_is_keyframe = encoder->is_keyframe ? 1 : 0;
    }

    return SHIM_OK;
}

SHIM_EXPORT int shim_video_encoder_set_bitrate(
    ShimVideoEncoder* encoder,
    uint32_t bitrate_bps
) {
    if (!encoder) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    std::lock_guard<std::mutex> lock(encoder->mutex);

    webrtc::VideoBitrateAllocation allocation;
    allocation.SetBitrate(0, 0, bitrate_bps);

    encoder->encoder->SetRates(webrtc::VideoEncoder::RateControlParameters(
        allocation,
        encoder->codec_settings.maxFramerate
    ));

    return SHIM_OK;
}

SHIM_EXPORT int shim_video_encoder_set_framerate(
    ShimVideoEncoder* encoder,
    float framerate
) {
    if (!encoder || framerate <= 0) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    std::lock_guard<std::mutex> lock(encoder->mutex);

    encoder->codec_settings.maxFramerate = static_cast<uint32_t>(framerate);

    webrtc::VideoBitrateAllocation allocation;
    allocation.SetBitrate(0, 0, encoder->codec_settings.maxBitrate * 1000);

    encoder->encoder->SetRates(webrtc::VideoEncoder::RateControlParameters(
        allocation,
        static_cast<double>(framerate)
    ));

    return SHIM_OK;
}

SHIM_EXPORT int shim_video_encoder_request_keyframe(ShimVideoEncoder* encoder) {
    if (!encoder) {
        return SHIM_ERROR_INVALID_PARAM;
    }
    encoder->force_keyframe = true;
    return SHIM_OK;
}

SHIM_EXPORT void shim_video_encoder_destroy(ShimVideoEncoder* encoder) {
    if (encoder) {
        encoder->encoder->Release();
        delete encoder;
    }
}

/* ============================================================================
 * Video Decoder Implementation
 * ========================================================================== */

struct ShimVideoDecoder {
    std::unique_ptr<webrtc::VideoDecoder> decoder;
    ShimCodecType codec_type;
    std::mutex mutex;

    // Decoded frame storage
    webrtc::scoped_refptr<webrtc::I420BufferInterface> decoded_buffer;
    bool has_output = false;
};

class DecoderCallback : public webrtc::DecodedImageCallback {
public:
    explicit DecoderCallback(ShimVideoDecoder* dec) : decoder_(dec) {}

    int32_t Decoded(webrtc::VideoFrame& frame) override {
        std::lock_guard<std::mutex> lock(decoder_->mutex);

        auto buffer = frame.video_frame_buffer()->ToI420();
        decoder_->decoded_buffer = buffer;
        decoder_->has_output = true;

        return WEBRTC_VIDEO_CODEC_OK;
    }

private:
    ShimVideoDecoder* decoder_;
};

SHIM_EXPORT ShimVideoDecoder* shim_video_decoder_create(ShimCodecType codec) {
    auto factory = webrtc::CreateBuiltinVideoDecoderFactory();
    if (!factory) {
        return nullptr;
    }

    webrtc::SdpVideoFormat format(CodecTypeToString(codec));
    auto decoder = factory->CreateVideoDecoder(format);
    if (!decoder) {
        return nullptr;
    }

    auto shim_decoder = std::make_unique<ShimVideoDecoder>();
    shim_decoder->decoder = std::move(decoder);
    shim_decoder->codec_type = codec;

    // Configure decoder
    webrtc::VideoDecoder::Settings settings;
    settings.set_codec_type(ToWebRTCCodecType(codec));
    settings.set_number_of_cores(1);
    settings.set_max_render_resolution({1920, 1080});

    auto callback = std::make_unique<DecoderCallback>(shim_decoder.get());

    if (!shim_decoder->decoder->Configure(settings)) {
        return nullptr;
    }

    shim_decoder->decoder->RegisterDecodeCompleteCallback(callback.release());

    return shim_decoder.release();
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
    if (!decoder || !data || size <= 0 || !y_dst || !u_dst || !v_dst) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    std::lock_guard<std::mutex> lock(decoder->mutex);

    // Create encoded image
    webrtc::EncodedImage encoded;
    encoded.SetEncodedData(
        webrtc::EncodedImageBuffer::Create(data, size)
    );
    encoded.SetRtpTimestamp(timestamp);
    encoded._frameType = is_keyframe
        ? webrtc::VideoFrameType::kVideoFrameKey
        : webrtc::VideoFrameType::kVideoFrameDelta;

    // Reset output state
    decoder->has_output = false;
    decoder->decoded_buffer = nullptr;

    // Decode
    int result = decoder->decoder->Decode(encoded, false, 0);
    if (result != WEBRTC_VIDEO_CODEC_OK) {
        if (result == WEBRTC_VIDEO_CODEC_OK_REQUEST_KEYFRAME) {
            return SHIM_ERROR_NEED_MORE_DATA;
        }
        return SHIM_ERROR_DECODE_FAILED;
    }

    if (!decoder->has_output || !decoder->decoded_buffer) {
        return SHIM_ERROR_NEED_MORE_DATA;
    }

    // Copy decoded frame to output buffers
    auto& buffer = decoder->decoded_buffer;
    int width = buffer->width();
    int height = buffer->height();

    // Copy Y plane
    const uint8_t* src_y = buffer->DataY();
    int src_stride_y = buffer->StrideY();
    for (int row = 0; row < height; ++row) {
        memcpy(y_dst + row * width, src_y + row * src_stride_y, width);
    }

    // Copy U plane
    const uint8_t* src_u = buffer->DataU();
    int src_stride_u = buffer->StrideU();
    int uv_height = (height + 1) / 2;
    int uv_width = (width + 1) / 2;
    for (int row = 0; row < uv_height; ++row) {
        memcpy(u_dst + row * uv_width, src_u + row * src_stride_u, uv_width);
    }

    // Copy V plane
    const uint8_t* src_v = buffer->DataV();
    int src_stride_v = buffer->StrideV();
    for (int row = 0; row < uv_height; ++row) {
        memcpy(v_dst + row * uv_width, src_v + row * src_stride_v, uv_width);
    }

    *out_width = width;
    *out_height = height;
    *out_y_stride = width;
    *out_u_stride = uv_width;
    *out_v_stride = uv_width;

    return SHIM_OK;
}

SHIM_EXPORT void shim_video_decoder_destroy(ShimVideoDecoder* decoder) {
    if (decoder) {
        decoder->decoder->Release();
        delete decoder;
    }
}

/* ============================================================================
 * Audio Encoder Implementation
 * ========================================================================== */

struct ShimAudioEncoder {
    std::unique_ptr<webrtc::AudioEncoder> encoder;
    int sample_rate;
    int channels;
    int frame_size;
    std::mutex mutex;
};

SHIM_EXPORT ShimAudioEncoder* shim_audio_encoder_create(
    const ShimAudioEncoderConfig* config
) {
    if (!config || config->sample_rate <= 0 || config->channels <= 0) {
        return nullptr;
    }

    webrtc::AudioEncoderOpusConfig opus_config;
    opus_config.frame_size_ms = 20;
    opus_config.sample_rate_hz = config->sample_rate;
    opus_config.num_channels = config->channels;
    opus_config.bitrate_bps = config->bitrate_bps > 0 ? config->bitrate_bps : 64000;
    opus_config.application = webrtc::AudioEncoderOpusConfig::ApplicationMode::kVoip;

    auto encoder = webrtc::AudioEncoderOpus::MakeAudioEncoder(
        opus_config,
        96  // payload type
    );

    if (!encoder) {
        return nullptr;
    }

    auto shim_encoder = std::make_unique<ShimAudioEncoder>();
    shim_encoder->encoder = std::move(encoder);
    shim_encoder->sample_rate = config->sample_rate;
    shim_encoder->channels = config->channels;
    shim_encoder->frame_size = (config->sample_rate * 20) / 1000;  // 20ms

    return shim_encoder.release();
}

SHIM_EXPORT int shim_audio_encoder_encode(
    ShimAudioEncoder* encoder,
    const uint8_t* samples,
    int num_samples,
    uint8_t* dst_buffer,
    int* out_size
) {
    if (!encoder || !samples || num_samples <= 0 || !dst_buffer || !out_size) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    std::lock_guard<std::mutex> lock(encoder->mutex);

    // Convert bytes to int16 samples
    const int16_t* pcm = reinterpret_cast<const int16_t*>(samples);
    int samples_per_channel = num_samples;

    // Create encoded buffer
    rtc::Buffer encoded_buffer;

    webrtc::AudioEncoder::EncodedInfo info = encoder->encoder->Encode(
        0,  // timestamp
        rtc::ArrayView<const int16_t>(pcm, samples_per_channel * encoder->channels),
        &encoded_buffer
    );

    if (encoded_buffer.empty()) {
        *out_size = 0;
        return SHIM_OK;
    }

    memcpy(dst_buffer, encoded_buffer.data(), encoded_buffer.size());
    *out_size = static_cast<int>(encoded_buffer.size());

    return SHIM_OK;
}

SHIM_EXPORT int shim_audio_encoder_set_bitrate(
    ShimAudioEncoder* encoder,
    uint32_t bitrate_bps
) {
    if (!encoder) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    std::lock_guard<std::mutex> lock(encoder->mutex);
    encoder->encoder->OnReceivedTargetAudioBitrate(bitrate_bps);
    return SHIM_OK;
}

SHIM_EXPORT void shim_audio_encoder_destroy(ShimAudioEncoder* encoder) {
    delete encoder;
}

/* ============================================================================
 * Audio Decoder Implementation
 * ========================================================================== */

struct ShimAudioDecoder {
    std::unique_ptr<webrtc::AudioDecoder> decoder;
    int sample_rate;
    int channels;
    std::mutex mutex;
};

SHIM_EXPORT ShimAudioDecoder* shim_audio_decoder_create(int sample_rate, int channels) {
    if (sample_rate <= 0 || channels <= 0) {
        return nullptr;
    }

    webrtc::AudioDecoderOpus::Config config;
    config.sample_rate_hz = sample_rate;
    config.num_channels = channels;

    auto decoder = webrtc::AudioDecoderOpus::MakeAudioDecoder(config);
    if (!decoder) {
        return nullptr;
    }

    auto shim_decoder = std::make_unique<ShimAudioDecoder>();
    shim_decoder->decoder = std::move(decoder);
    shim_decoder->sample_rate = sample_rate;
    shim_decoder->channels = channels;

    return shim_decoder.release();
}

SHIM_EXPORT int shim_audio_decoder_decode(
    ShimAudioDecoder* decoder,
    const uint8_t* data,
    int size,
    uint8_t* dst_samples,
    int* out_num_samples
) {
    if (!decoder || !data || size <= 0 || !dst_samples || !out_num_samples) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    std::lock_guard<std::mutex> lock(decoder->mutex);

    // Maximum samples for 120ms at 48kHz stereo
    constexpr int kMaxSamples = 48000 * 120 / 1000 * 2;
    int16_t pcm_buffer[kMaxSamples];

    webrtc::AudioDecoder::SpeechType speech_type;
    int decoded_samples = decoder->decoder->Decode(
        data,
        size,
        decoder->sample_rate,
        sizeof(pcm_buffer),
        pcm_buffer,
        &speech_type
    );

    if (decoded_samples < 0) {
        return SHIM_ERROR_DECODE_FAILED;
    }

    // Copy decoded samples as bytes
    int total_samples = decoded_samples * decoder->channels;
    memcpy(dst_samples, pcm_buffer, total_samples * sizeof(int16_t));
    *out_num_samples = total_samples;

    return SHIM_OK;
}

SHIM_EXPORT void shim_audio_decoder_destroy(ShimAudioDecoder* decoder) {
    delete decoder;
}

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
    // Real implementation would use codec-specific packetizers

    int payload_size = packetizer->mtu - 12;  // RTP header is 12 bytes
    int offset = 0;
    int packet_count = 0;
    int buffer_offset = 0;

    while (offset < size && packet_count < max_packets) {
        int chunk_size = std::min(payload_size, size - offset);
        bool is_first = (offset == 0);
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

/* ============================================================================
 * PeerConnection Implementation
 * ========================================================================== */

struct ShimPeerConnection {
    webrtc::scoped_refptr<webrtc::PeerConnectionFactoryInterface> factory;
    webrtc::scoped_refptr<webrtc::PeerConnectionInterface> peer_connection;
    std::mutex mutex;

    // Callbacks
    ShimOnICECandidate on_ice_candidate = nullptr;
    void* on_ice_candidate_ctx = nullptr;
    ShimOnConnectionStateChange on_connection_state_change = nullptr;
    void* on_connection_state_change_ctx = nullptr;
    ShimOnTrack on_track = nullptr;
    void* on_track_ctx = nullptr;
    ShimOnDataChannel on_data_channel = nullptr;
    void* on_data_channel_ctx = nullptr;
    ShimOnSignalingStateChange on_signaling_state_change = nullptr;
    void* on_signaling_state_change_ctx = nullptr;
    ShimOnICEConnectionStateChange on_ice_connection_state_change = nullptr;
    void* on_ice_connection_state_change_ctx = nullptr;
    ShimOnICEGatheringStateChange on_ice_gathering_state_change = nullptr;
    void* on_ice_gathering_state_change_ctx = nullptr;
    ShimOnNegotiationNeeded on_negotiation_needed = nullptr;
    void* on_negotiation_needed_ctx = nullptr;

    // Track senders
    std::vector<webrtc::scoped_refptr<webrtc::RtpSenderInterface>> senders;
};

class PeerConnectionObserver : public webrtc::PeerConnectionObserver {
public:
    explicit PeerConnectionObserver(ShimPeerConnection* pc) : pc_(pc) {}

    void OnSignalingChange(webrtc::PeerConnectionInterface::SignalingState state) override {
        if (pc_->on_signaling_state_change) {
            pc_->on_signaling_state_change(pc_->on_signaling_state_change_ctx, static_cast<int>(state));
        }
    }

    void OnDataChannel(webrtc::scoped_refptr<webrtc::DataChannelInterface> channel) override {
        if (pc_->on_data_channel) {
            pc_->on_data_channel(pc_->on_data_channel_ctx, channel.release());
        }
    }

    void OnRenegotiationNeeded() override {
        if (pc_->on_negotiation_needed) {
            pc_->on_negotiation_needed(pc_->on_negotiation_needed_ctx);
        }
    }

    void OnIceConnectionChange(webrtc::PeerConnectionInterface::IceConnectionState state) override {
        if (pc_->on_ice_connection_state_change) {
            pc_->on_ice_connection_state_change(pc_->on_ice_connection_state_change_ctx, static_cast<int>(state));
        }
    }

    void OnIceGatheringChange(webrtc::PeerConnectionInterface::IceGatheringState state) override {
        if (pc_->on_ice_gathering_state_change) {
            pc_->on_ice_gathering_state_change(pc_->on_ice_gathering_state_change_ctx, static_cast<int>(state));
        }
    }

    void OnIceCandidate(const webrtc::IceCandidateInterface* candidate) override {
        if (pc_->on_ice_candidate) {
            std::string sdp;
            candidate->ToString(&sdp);

            ShimICECandidate shim_candidate;
            shim_candidate.candidate = sdp.c_str();
            shim_candidate.sdp_mid = candidate->sdp_mid().c_str();
            shim_candidate.sdp_mline_index = candidate->sdp_mline_index();

            pc_->on_ice_candidate(pc_->on_ice_candidate_ctx, &shim_candidate);
        }
    }

    void OnConnectionChange(webrtc::PeerConnectionInterface::PeerConnectionState state) override {
        if (pc_->on_connection_state_change) {
            pc_->on_connection_state_change(pc_->on_connection_state_change_ctx, static_cast<int>(state));
        }
    }

    void OnTrack(webrtc::scoped_refptr<webrtc::RtpTransceiverInterface> transceiver) override {
        if (pc_->on_track) {
            auto receiver = transceiver->receiver();
            auto track = receiver->track();
            pc_->on_track(pc_->on_track_ctx, track.get(), receiver.get(), "");
        }
    }

private:
    ShimPeerConnection* pc_;
};

SHIM_EXPORT ShimPeerConnection* shim_peer_connection_create(
    const ShimPeerConnectionConfig* config
) {
    InitializeGlobals();

    auto pc = std::make_unique<ShimPeerConnection>();

    // Create PeerConnectionFactory
    pc->factory = webrtc::CreatePeerConnectionFactory(
        g_network_thread.get(),
        g_worker_thread.get(),
        g_signaling_thread.get(),
        nullptr,  // default_adm
        webrtc::CreateBuiltinAudioEncoderFactory(),
        webrtc::CreateBuiltinAudioDecoderFactory(),
        webrtc::CreateBuiltinVideoEncoderFactory(),
        webrtc::CreateBuiltinVideoDecoderFactory(),
        nullptr,  // audio_mixer
        nullptr   // audio_processing
    );

    if (!pc->factory) {
        return nullptr;
    }

    // Configure ICE servers
    webrtc::PeerConnectionInterface::RTCConfiguration rtc_config;
    rtc_config.sdp_semantics = webrtc::SdpSemantics::kUnifiedPlan;

    if (config) {
        for (int i = 0; i < config->ice_server_count; i++) {
            webrtc::PeerConnectionInterface::IceServer server;
            for (int j = 0; j < config->ice_servers[i].url_count; j++) {
                server.urls.push_back(config->ice_servers[i].urls[j]);
            }
            if (config->ice_servers[i].username) {
                server.username = config->ice_servers[i].username;
            }
            if (config->ice_servers[i].credential) {
                server.password = config->ice_servers[i].credential;
            }
            rtc_config.servers.push_back(server);
        }
    }

    // Create PeerConnection
    auto observer = std::make_unique<PeerConnectionObserver>(pc.get());

    webrtc::PeerConnectionDependencies deps(observer.release());

    auto result = pc->factory->CreatePeerConnectionOrError(rtc_config, std::move(deps));
    if (!result.ok()) {
        return nullptr;
    }

    pc->peer_connection = result.MoveValue();

    return pc.release();
}

SHIM_EXPORT void shim_peer_connection_destroy(ShimPeerConnection* pc) {
    if (pc) {
        if (pc->peer_connection) {
            pc->peer_connection->Close();
        }
        delete pc;
    }
}

SHIM_EXPORT void shim_peer_connection_set_on_ice_candidate(
    ShimPeerConnection* pc,
    ShimOnICECandidate callback,
    void* ctx
) {
    if (pc) {
        pc->on_ice_candidate = callback;
        pc->on_ice_candidate_ctx = ctx;
    }
}

SHIM_EXPORT void shim_peer_connection_set_on_connection_state_change(
    ShimPeerConnection* pc,
    ShimOnConnectionStateChange callback,
    void* ctx
) {
    if (pc) {
        pc->on_connection_state_change = callback;
        pc->on_connection_state_change_ctx = ctx;
    }
}

SHIM_EXPORT void shim_peer_connection_set_on_track(
    ShimPeerConnection* pc,
    ShimOnTrack callback,
    void* ctx
) {
    if (pc) {
        pc->on_track = callback;
        pc->on_track_ctx = ctx;
    }
}

SHIM_EXPORT void shim_peer_connection_set_on_data_channel(
    ShimPeerConnection* pc,
    ShimOnDataChannel callback,
    void* ctx
) {
    if (pc) {
        pc->on_data_channel = callback;
        pc->on_data_channel_ctx = ctx;
    }
}

SHIM_EXPORT void shim_peer_connection_set_on_signaling_state_change(
    ShimPeerConnection* pc,
    ShimOnSignalingStateChange callback,
    void* ctx
) {
    if (pc) {
        pc->on_signaling_state_change = callback;
        pc->on_signaling_state_change_ctx = ctx;
    }
}

SHIM_EXPORT void shim_peer_connection_set_on_ice_connection_state_change(
    ShimPeerConnection* pc,
    ShimOnICEConnectionStateChange callback,
    void* ctx
) {
    if (pc) {
        pc->on_ice_connection_state_change = callback;
        pc->on_ice_connection_state_change_ctx = ctx;
    }
}

SHIM_EXPORT void shim_peer_connection_set_on_ice_gathering_state_change(
    ShimPeerConnection* pc,
    ShimOnICEGatheringStateChange callback,
    void* ctx
) {
    if (pc) {
        pc->on_ice_gathering_state_change = callback;
        pc->on_ice_gathering_state_change_ctx = ctx;
    }
}

SHIM_EXPORT void shim_peer_connection_set_on_negotiation_needed(
    ShimPeerConnection* pc,
    ShimOnNegotiationNeeded callback,
    void* ctx
) {
    if (pc) {
        pc->on_negotiation_needed = callback;
        pc->on_negotiation_needed_ctx = ctx;
    }
}

SHIM_EXPORT int shim_peer_connection_create_offer(
    ShimPeerConnection* pc,
    char* sdp_out,
    int sdp_out_size,
    int* out_sdp_len
) {
    if (!pc || !pc->peer_connection || !sdp_out || !out_sdp_len) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    class CreateSessionDescriptionObserver
        : public webrtc::CreateSessionDescriptionObserver {
    public:
        std::string sdp;
        bool success = false;
        std::mutex mutex;
        std::condition_variable cv;
        bool done = false;

        void OnSuccess(webrtc::SessionDescriptionInterface* desc) override {
            desc->ToString(&sdp);
            std::lock_guard<std::mutex> lock(mutex);
            success = true;
            done = true;
            cv.notify_one();
        }

        void OnFailure(webrtc::RTCError error) override {
            std::lock_guard<std::mutex> lock(mutex);
            success = false;
            done = true;
            cv.notify_one();
        }
    };

    auto observer = webrtc::make_ref_counted<CreateSessionDescriptionObserver>();

    webrtc::PeerConnectionInterface::RTCOfferAnswerOptions options;
    pc->peer_connection->CreateOffer(observer.get(), options);

    // Wait for completion
    {
        std::unique_lock<std::mutex> lock(observer->mutex);
        observer->cv.wait(lock, [&]() { return observer->done; });
    }

    if (!observer->success) {
        return SHIM_ERROR_INIT_FAILED;
    }

    int len = static_cast<int>(observer->sdp.size());
    if (len >= sdp_out_size) {
        return SHIM_ERROR_BUFFER_TOO_SMALL;
    }

    memcpy(sdp_out, observer->sdp.c_str(), len + 1);
    *out_sdp_len = len;

    return SHIM_OK;
}

SHIM_EXPORT int shim_peer_connection_create_answer(
    ShimPeerConnection* pc,
    char* sdp_out,
    int sdp_out_size,
    int* out_sdp_len
) {
    if (!pc || !pc->peer_connection || !sdp_out || !out_sdp_len) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    class CreateSessionDescriptionObserver
        : public webrtc::CreateSessionDescriptionObserver {
    public:
        std::string sdp;
        bool success = false;
        std::mutex mutex;
        std::condition_variable cv;
        bool done = false;

        void OnSuccess(webrtc::SessionDescriptionInterface* desc) override {
            desc->ToString(&sdp);
            std::lock_guard<std::mutex> lock(mutex);
            success = true;
            done = true;
            cv.notify_one();
        }

        void OnFailure(webrtc::RTCError error) override {
            std::lock_guard<std::mutex> lock(mutex);
            success = false;
            done = true;
            cv.notify_one();
        }
    };

    auto observer = webrtc::make_ref_counted<CreateSessionDescriptionObserver>();

    webrtc::PeerConnectionInterface::RTCOfferAnswerOptions options;
    pc->peer_connection->CreateAnswer(observer.get(), options);

    // Wait for completion
    {
        std::unique_lock<std::mutex> lock(observer->mutex);
        observer->cv.wait(lock, [&]() { return observer->done; });
    }

    if (!observer->success) {
        return SHIM_ERROR_INIT_FAILED;
    }

    int len = static_cast<int>(observer->sdp.size());
    if (len >= sdp_out_size) {
        return SHIM_ERROR_BUFFER_TOO_SMALL;
    }

    memcpy(sdp_out, observer->sdp.c_str(), len + 1);
    *out_sdp_len = len;

    return SHIM_OK;
}

SHIM_EXPORT int shim_peer_connection_set_local_description(
    ShimPeerConnection* pc,
    int type,
    const char* sdp
) {
    if (!pc || !pc->peer_connection || !sdp) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    webrtc::SdpType sdp_type;
    switch (type) {
        case 0: sdp_type = webrtc::SdpType::kOffer; break;
        case 1: sdp_type = webrtc::SdpType::kPrAnswer; break;
        case 2: sdp_type = webrtc::SdpType::kAnswer; break;
        default: return SHIM_ERROR_INVALID_PARAM;
    }

    webrtc::SdpParseError error;
    auto desc = webrtc::CreateSessionDescription(sdp_type, sdp, &error);
    if (!desc) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    class SetSessionDescriptionObserver
        : public webrtc::SetSessionDescriptionObserver {
    public:
        bool success = false;
        std::mutex mutex;
        std::condition_variable cv;
        bool done = false;

        void OnSuccess() override {
            std::lock_guard<std::mutex> lock(mutex);
            success = true;
            done = true;
            cv.notify_one();
        }

        void OnFailure(webrtc::RTCError error) override {
            std::lock_guard<std::mutex> lock(mutex);
            success = false;
            done = true;
            cv.notify_one();
        }
    };

    auto observer = webrtc::make_ref_counted<SetSessionDescriptionObserver>();
    pc->peer_connection->SetLocalDescription(observer.get(), desc.release());

    {
        std::unique_lock<std::mutex> lock(observer->mutex);
        observer->cv.wait(lock, [&]() { return observer->done; });
    }

    return observer->success ? SHIM_OK : SHIM_ERROR_INIT_FAILED;
}

SHIM_EXPORT int shim_peer_connection_set_remote_description(
    ShimPeerConnection* pc,
    int type,
    const char* sdp
) {
    if (!pc || !pc->peer_connection || !sdp) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    webrtc::SdpType sdp_type;
    switch (type) {
        case 0: sdp_type = webrtc::SdpType::kOffer; break;
        case 1: sdp_type = webrtc::SdpType::kPrAnswer; break;
        case 2: sdp_type = webrtc::SdpType::kAnswer; break;
        default: return SHIM_ERROR_INVALID_PARAM;
    }

    webrtc::SdpParseError error;
    auto desc = webrtc::CreateSessionDescription(sdp_type, sdp, &error);
    if (!desc) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    class SetSessionDescriptionObserver
        : public webrtc::SetSessionDescriptionObserver {
    public:
        bool success = false;
        std::mutex mutex;
        std::condition_variable cv;
        bool done = false;

        void OnSuccess() override {
            std::lock_guard<std::mutex> lock(mutex);
            success = true;
            done = true;
            cv.notify_one();
        }

        void OnFailure(webrtc::RTCError error) override {
            std::lock_guard<std::mutex> lock(mutex);
            success = false;
            done = true;
            cv.notify_one();
        }
    };

    auto observer = webrtc::make_ref_counted<SetSessionDescriptionObserver>();
    pc->peer_connection->SetRemoteDescription(observer.get(), desc.release());

    {
        std::unique_lock<std::mutex> lock(observer->mutex);
        observer->cv.wait(lock, [&]() { return observer->done; });
    }

    return observer->success ? SHIM_OK : SHIM_ERROR_INIT_FAILED;
}

SHIM_EXPORT int shim_peer_connection_add_ice_candidate(
    ShimPeerConnection* pc,
    const char* candidate,
    const char* sdp_mid,
    int sdp_mline_index
) {
    if (!pc || !pc->peer_connection || !candidate) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    webrtc::SdpParseError error;
    auto ice_candidate = webrtc::CreateIceCandidate(
        sdp_mid ? sdp_mid : "",
        sdp_mline_index,
        candidate,
        &error
    );

    if (!ice_candidate) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    if (!pc->peer_connection->AddIceCandidate(ice_candidate.get())) {
        return SHIM_ERROR_INIT_FAILED;
    }

    return SHIM_OK;
}

SHIM_EXPORT int shim_peer_connection_signaling_state(ShimPeerConnection* pc) {
    if (!pc || !pc->peer_connection) return -1;
    return static_cast<int>(pc->peer_connection->signaling_state());
}

SHIM_EXPORT int shim_peer_connection_ice_connection_state(ShimPeerConnection* pc) {
    if (!pc || !pc->peer_connection) return -1;
    return static_cast<int>(pc->peer_connection->ice_connection_state());
}

SHIM_EXPORT int shim_peer_connection_ice_gathering_state(ShimPeerConnection* pc) {
    if (!pc || !pc->peer_connection) return -1;
    return static_cast<int>(pc->peer_connection->ice_gathering_state());
}

SHIM_EXPORT int shim_peer_connection_connection_state(ShimPeerConnection* pc) {
    if (!pc || !pc->peer_connection) return -1;
    return static_cast<int>(pc->peer_connection->peer_connection_state());
}

SHIM_EXPORT ShimRTPSender* shim_peer_connection_add_track(
    ShimPeerConnection* pc,
    ShimCodecType codec,
    const char* track_id,
    const char* stream_id
) {
    if (!pc || !pc->peer_connection || !track_id) {
        return nullptr;
    }

    // Create dummy track source
    auto result = pc->peer_connection->AddTransceiver(
        codec == SHIM_CODEC_OPUS ? cricket::MEDIA_TYPE_AUDIO : cricket::MEDIA_TYPE_VIDEO
    );

    if (!result.ok()) {
        return nullptr;
    }

    auto sender = result.value()->sender();
    pc->senders.push_back(sender);

    return reinterpret_cast<ShimRTPSender*>(sender.get());
}

SHIM_EXPORT int shim_peer_connection_remove_track(
    ShimPeerConnection* pc,
    ShimRTPSender* sender
) {
    if (!pc || !pc->peer_connection || !sender) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    auto webrtc_sender = reinterpret_cast<webrtc::RtpSenderInterface*>(sender);

    auto result = pc->peer_connection->RemoveTrackOrError(
        webrtc::scoped_refptr<webrtc::RtpSenderInterface>(webrtc_sender)
    );

    return result.ok() ? SHIM_OK : SHIM_ERROR_INIT_FAILED;
}

SHIM_EXPORT ShimDataChannel* shim_peer_connection_create_data_channel(
    ShimPeerConnection* pc,
    const char* label,
    int ordered,
    int max_retransmits,
    const char* protocol
) {
    if (!pc || !pc->peer_connection || !label) {
        return nullptr;
    }

    webrtc::DataChannelInit config;
    config.ordered = ordered != 0;
    if (max_retransmits >= 0) {
        config.maxRetransmits = max_retransmits;
    }
    if (protocol) {
        config.protocol = protocol;
    }

    auto result = pc->peer_connection->CreateDataChannelOrError(label, &config);
    if (!result.ok()) {
        return nullptr;
    }

    return reinterpret_cast<ShimDataChannel*>(result.value().release());
}

SHIM_EXPORT void shim_peer_connection_close(ShimPeerConnection* pc) {
    if (pc && pc->peer_connection) {
        pc->peer_connection->Close();
    }
}

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
    // Simplified - real implementation would handle track replacement
    return SHIM_ERROR_NOT_SUPPORTED;
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

        strncpy(out.rid, enc.rid.c_str(), sizeof(out.rid) - 1);
        out.rid[sizeof(out.rid) - 1] = '\0';

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

/* ============================================================================
 * RTPReceiver Implementation
 * ========================================================================== */

SHIM_EXPORT void* shim_rtp_receiver_get_track(ShimRTPReceiver* receiver) {
    if (!receiver) return nullptr;
    auto webrtc_receiver = reinterpret_cast<webrtc::RtpReceiverInterface*>(receiver);
    return webrtc_receiver->track().get();
}

SHIM_EXPORT int shim_rtp_receiver_get_stats(ShimRTPReceiver* receiver, ShimRTCStats* out_stats) {
    if (!receiver || !out_stats) return SHIM_ERROR_INVALID_PARAM;
    memset(out_stats, 0, sizeof(ShimRTCStats));
    // TODO: Implement stats collection
    return SHIM_OK;
}

SHIM_EXPORT int shim_rtp_receiver_request_keyframe(ShimRTPReceiver* receiver) {
    if (!receiver) return SHIM_ERROR_INVALID_PARAM;
    // TODO: Send PLI via RTCP
    return SHIM_OK;
}

/* ============================================================================
 * RTPTransceiver Implementation
 * ========================================================================== */

SHIM_EXPORT int shim_transceiver_get_direction(ShimRTPTransceiver* transceiver) {
    if (!transceiver) return SHIM_TRANSCEIVER_DIRECTION_INACTIVE;
    auto t = reinterpret_cast<webrtc::RtpTransceiverInterface*>(transceiver);
    return static_cast<int>(t->direction());
}

SHIM_EXPORT int shim_transceiver_set_direction(ShimRTPTransceiver* transceiver, int direction) {
    if (!transceiver) return SHIM_ERROR_INVALID_PARAM;
    auto t = reinterpret_cast<webrtc::RtpTransceiverInterface*>(transceiver);
    auto dir = static_cast<webrtc::RtpTransceiverDirection>(direction);
    auto result = t->SetDirectionWithError(dir);
    return result.ok() ? SHIM_OK : SHIM_ERROR_INVALID_PARAM;
}

SHIM_EXPORT int shim_transceiver_get_current_direction(ShimRTPTransceiver* transceiver) {
    if (!transceiver) return SHIM_TRANSCEIVER_DIRECTION_INACTIVE;
    auto t = reinterpret_cast<webrtc::RtpTransceiverInterface*>(transceiver);
    auto dir = t->current_direction();
    return dir.has_value() ? static_cast<int>(dir.value()) : SHIM_TRANSCEIVER_DIRECTION_INACTIVE;
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
    return mid.has_value() ? mid->c_str() : "";
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

/* ============================================================================
 * PeerConnection Extended Implementation
 * ========================================================================== */

SHIM_EXPORT ShimRTPTransceiver* shim_peer_connection_add_transceiver(
    ShimPeerConnection* pc,
    int kind,
    int direction
) {
    if (!pc || !pc->peer_connection) return nullptr;

    cricket::MediaType media_type = kind == 0 ? cricket::MEDIA_TYPE_AUDIO : cricket::MEDIA_TYPE_VIDEO;
    webrtc::RtpTransceiverInit init;
    init.direction = static_cast<webrtc::RtpTransceiverDirection>(direction);

    auto result = pc->peer_connection->AddTransceiver(media_type, init);
    if (!result.ok()) return nullptr;

    return reinterpret_cast<ShimRTPTransceiver*>(result.value().get());
}

SHIM_EXPORT int shim_peer_connection_get_senders(
    ShimPeerConnection* pc,
    ShimRTPSender** senders,
    int max_senders,
    int* out_count
) {
    if (!pc || !pc->peer_connection || !senders || !out_count) return SHIM_ERROR_INVALID_PARAM;

    auto all_senders = pc->peer_connection->GetSenders();
    int count = std::min(static_cast<int>(all_senders.size()), max_senders);

    for (int i = 0; i < count; i++) {
        senders[i] = reinterpret_cast<ShimRTPSender*>(all_senders[i].get());
    }
    *out_count = count;

    return SHIM_OK;
}

SHIM_EXPORT int shim_peer_connection_get_receivers(
    ShimPeerConnection* pc,
    ShimRTPReceiver** receivers,
    int max_receivers,
    int* out_count
) {
    if (!pc || !pc->peer_connection || !receivers || !out_count) return SHIM_ERROR_INVALID_PARAM;

    auto all_receivers = pc->peer_connection->GetReceivers();
    int count = std::min(static_cast<int>(all_receivers.size()), max_receivers);

    for (int i = 0; i < count; i++) {
        receivers[i] = reinterpret_cast<ShimRTPReceiver*>(all_receivers[i].get());
    }
    *out_count = count;

    return SHIM_OK;
}

SHIM_EXPORT int shim_peer_connection_get_transceivers(
    ShimPeerConnection* pc,
    ShimRTPTransceiver** transceivers,
    int max_transceivers,
    int* out_count
) {
    if (!pc || !pc->peer_connection || !transceivers || !out_count) return SHIM_ERROR_INVALID_PARAM;

    auto all_transceivers = pc->peer_connection->GetTransceivers();
    int count = std::min(static_cast<int>(all_transceivers.size()), max_transceivers);

    for (int i = 0; i < count; i++) {
        transceivers[i] = reinterpret_cast<ShimRTPTransceiver*>(all_transceivers[i].get());
    }
    *out_count = count;

    return SHIM_OK;
}

SHIM_EXPORT int shim_peer_connection_restart_ice(ShimPeerConnection* pc) {
    if (!pc || !pc->peer_connection) return SHIM_ERROR_INVALID_PARAM;
    pc->peer_connection->RestartIce();
    return SHIM_OK;
}

SHIM_EXPORT int shim_peer_connection_get_stats(ShimPeerConnection* pc, ShimRTCStats* out_stats) {
    if (!pc || !pc->peer_connection || !out_stats) return SHIM_ERROR_INVALID_PARAM;
    memset(out_stats, 0, sizeof(ShimRTCStats));
    // TODO: Implement proper stats collection via GetStats() callback
    return SHIM_OK;
}

/* ============================================================================
 * Video Track Source Implementation (Pushable Frame Source)
 * ========================================================================== */

// Custom video track source that accepts pushed frames
class PushableVideoTrackSource : public webrtc::VideoTrackSourceInterface {
public:
    PushableVideoTrackSource(int width, int height)
        : width_(width), height_(height), state_(webrtc::MediaSourceInterface::kLive) {}

    // VideoTrackSourceInterface
    bool is_screencast() const override { return false; }
    absl::optional<bool> needs_denoising() const override { return absl::nullopt; }

    // MediaSourceInterface
    SourceState state() const override { return state_; }
    bool remote() const override { return false; }

    // NotifierInterface (part of VideoTrackSourceInterface)
    void RegisterObserver(webrtc::ObserverInterface* observer) override {
        std::lock_guard<std::mutex> lock(mutex_);
        observers_.push_back(observer);
    }

    void UnregisterObserver(webrtc::ObserverInterface* observer) override {
        std::lock_guard<std::mutex> lock(mutex_);
        observers_.erase(
            std::remove(observers_.begin(), observers_.end(), observer),
            observers_.end()
        );
    }

    // rtc::VideoSourceInterface<VideoFrame>
    void AddOrUpdateSink(rtc::VideoSinkInterface<webrtc::VideoFrame>* sink,
                         const rtc::VideoSinkWants& wants) override {
        std::lock_guard<std::mutex> lock(mutex_);
        sinks_.push_back(sink);
    }

    void RemoveSink(rtc::VideoSinkInterface<webrtc::VideoFrame>* sink) override {
        std::lock_guard<std::mutex> lock(mutex_);
        sinks_.erase(
            std::remove(sinks_.begin(), sinks_.end(), sink),
            sinks_.end()
        );
    }

    bool SupportsEncodedOutput() const override { return false; }
    void GenerateKeyFrame() override {}
    void AddEncodedSink(rtc::VideoSinkInterface<webrtc::RecordableEncodedFrame>*) override {}
    void RemoveEncodedSink(rtc::VideoSinkInterface<webrtc::RecordableEncodedFrame>*) override {}

    // Push a frame to all registered sinks
    void PushFrame(webrtc::scoped_refptr<webrtc::I420Buffer> buffer, int64_t timestamp_us) {
        std::lock_guard<std::mutex> lock(mutex_);

        webrtc::VideoFrame frame = webrtc::VideoFrame::Builder()
            .set_video_frame_buffer(buffer)
            .set_timestamp_us(timestamp_us)
            .set_timestamp_rtp(static_cast<uint32_t>(timestamp_us * 90 / 1000))  // Convert to 90kHz
            .build();

        for (auto* sink : sinks_) {
            sink->OnFrame(frame);
        }
    }

    int width() const { return width_; }
    int height() const { return height_; }

private:
    int width_;
    int height_;
    SourceState state_;
    std::mutex mutex_;
    std::vector<webrtc::ObserverInterface*> observers_;
    std::vector<rtc::VideoSinkInterface<webrtc::VideoFrame>*> sinks_;
};

struct ShimVideoTrackSource {
    webrtc::scoped_refptr<PushableVideoTrackSource> source;
    webrtc::scoped_refptr<webrtc::VideoTrackInterface> track;
    ShimPeerConnection* pc;
    int width;
    int height;
};

SHIM_EXPORT ShimVideoTrackSource* shim_video_track_source_create(
    ShimPeerConnection* pc,
    int width,
    int height
) {
    if (!pc || !pc->factory || width <= 0 || height <= 0) {
        return nullptr;
    }

    auto shim_source = std::make_unique<ShimVideoTrackSource>();
    shim_source->source = webrtc::make_ref_counted<PushableVideoTrackSource>(width, height);
    shim_source->pc = pc;
    shim_source->width = width;
    shim_source->height = height;
    shim_source->track = nullptr;  // Created when added to PC

    return shim_source.release();
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
    if (!source || !source->source || !y_plane || !u_plane || !v_plane) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    // Create I420 buffer from input planes
    webrtc::scoped_refptr<webrtc::I420Buffer> buffer = webrtc::I420Buffer::Copy(
        source->width, source->height,
        y_plane, y_stride,
        u_plane, u_stride,
        v_plane, v_stride
    );

    if (!buffer) {
        return SHIM_ERROR_OUT_OF_MEMORY;
    }

    source->source->PushFrame(buffer, timestamp_us);
    return SHIM_OK;
}

SHIM_EXPORT ShimRTPSender* shim_peer_connection_add_video_track_from_source(
    ShimPeerConnection* pc,
    ShimVideoTrackSource* source,
    const char* track_id,
    const char* stream_id
) {
    if (!pc || !pc->peer_connection || !pc->factory || !source || !source->source || !track_id) {
        return nullptr;
    }

    // Create video track from source
    source->track = pc->factory->CreateVideoTrack(
        track_id,
        source->source.get()
    );

    if (!source->track) {
        return nullptr;
    }

    // Add track to peer connection
    std::vector<std::string> stream_ids;
    if (stream_id) {
        stream_ids.push_back(stream_id);
    }

    auto result = pc->peer_connection->AddTrack(source->track, stream_ids);
    if (!result.ok()) {
        return nullptr;
    }

    auto sender = result.value();
    pc->senders.push_back(sender);

    return reinterpret_cast<ShimRTPSender*>(sender.get());
}

SHIM_EXPORT void shim_video_track_source_destroy(ShimVideoTrackSource* source) {
    if (source) {
        source->track = nullptr;
        source->source = nullptr;
        delete source;
    }
}

/* ============================================================================
 * Audio Track Source Implementation (Pushable Frame Source)
 * ========================================================================== */

// Custom audio track source that accepts pushed audio frames
class PushableAudioSource : public webrtc::AudioSourceInterface {
public:
    PushableAudioSource(int sample_rate, int channels)
        : sample_rate_(sample_rate), channels_(channels), state_(kLive) {}

    // AudioSourceInterface
    void SetVolume(double volume) override { volume_ = volume; }
    void RegisterAudioObserver(webrtc::AudioObserver* observer) override {
        std::lock_guard<std::mutex> lock(mutex_);
        audio_observers_.push_back(observer);
    }
    void UnregisterAudioObserver(webrtc::AudioObserver* observer) override {
        std::lock_guard<std::mutex> lock(mutex_);
        audio_observers_.erase(
            std::remove(audio_observers_.begin(), audio_observers_.end(), observer),
            audio_observers_.end()
        );
    }
    void AddSink(webrtc::AudioTrackSinkInterface* sink) override {
        std::lock_guard<std::mutex> lock(mutex_);
        sinks_.push_back(sink);
    }
    void RemoveSink(webrtc::AudioTrackSinkInterface* sink) override {
        std::lock_guard<std::mutex> lock(mutex_);
        sinks_.erase(
            std::remove(sinks_.begin(), sinks_.end(), sink),
            sinks_.end()
        );
    }
    const cricket::AudioOptions options() const override { return options_; }

    // MediaSourceInterface
    SourceState state() const override { return state_; }
    bool remote() const override { return false; }

    // NotifierInterface
    void RegisterObserver(webrtc::ObserverInterface* observer) override {
        std::lock_guard<std::mutex> lock(mutex_);
        observers_.push_back(observer);
    }
    void UnregisterObserver(webrtc::ObserverInterface* observer) override {
        std::lock_guard<std::mutex> lock(mutex_);
        observers_.erase(
            std::remove(observers_.begin(), observers_.end(), observer),
            observers_.end()
        );
    }

    // Push audio data to all registered sinks
    void PushAudio(const int16_t* samples, int num_samples, int64_t timestamp_us) {
        std::lock_guard<std::mutex> lock(mutex_);

        for (auto* sink : sinks_) {
            sink->OnData(
                samples,
                16,  // bits per sample
                sample_rate_,
                channels_,
                num_samples
            );
        }
    }

    int sample_rate() const { return sample_rate_; }
    int channels() const { return channels_; }

private:
    int sample_rate_;
    int channels_;
    double volume_ = 1.0;
    SourceState state_;
    cricket::AudioOptions options_;
    std::mutex mutex_;
    std::vector<webrtc::ObserverInterface*> observers_;
    std::vector<webrtc::AudioObserver*> audio_observers_;
    std::vector<webrtc::AudioTrackSinkInterface*> sinks_;
};

struct ShimAudioTrackSource {
    webrtc::scoped_refptr<PushableAudioSource> source;
    webrtc::scoped_refptr<webrtc::AudioTrackInterface> track;
    ShimPeerConnection* pc;
    int sample_rate;
    int channels;
};

SHIM_EXPORT ShimAudioTrackSource* shim_audio_track_source_create(
    ShimPeerConnection* pc,
    int sample_rate,
    int channels
) {
    if (!pc || !pc->factory || sample_rate <= 0 || channels <= 0 || channels > 2) {
        return nullptr;
    }

    auto shim_source = std::make_unique<ShimAudioTrackSource>();
    shim_source->source = webrtc::make_ref_counted<PushableAudioSource>(sample_rate, channels);
    shim_source->pc = pc;
    shim_source->sample_rate = sample_rate;
    shim_source->channels = channels;
    shim_source->track = nullptr;

    return shim_source.release();
}

SHIM_EXPORT int shim_audio_track_source_push_frame(
    ShimAudioTrackSource* source,
    const int16_t* samples,
    int num_samples,
    int64_t timestamp_us
) {
    if (!source || !source->source || !samples || num_samples <= 0) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    source->source->PushAudio(samples, num_samples, timestamp_us);
    return SHIM_OK;
}

SHIM_EXPORT ShimRTPSender* shim_peer_connection_add_audio_track_from_source(
    ShimPeerConnection* pc,
    ShimAudioTrackSource* source,
    const char* track_id,
    const char* stream_id
) {
    if (!pc || !pc->peer_connection || !pc->factory || !source || !source->source || !track_id) {
        return nullptr;
    }

    // Create audio track from source
    source->track = pc->factory->CreateAudioTrack(
        track_id,
        source->source.get()
    );

    if (!source->track) {
        return nullptr;
    }

    // Add track to peer connection
    std::vector<std::string> stream_ids;
    if (stream_id) {
        stream_ids.push_back(stream_id);
    }

    auto result = pc->peer_connection->AddTrack(source->track, stream_ids);
    if (!result.ok()) {
        return nullptr;
    }

    auto sender = result.value();
    pc->senders.push_back(sender);

    return reinterpret_cast<ShimRTPSender*>(sender.get());
}

SHIM_EXPORT void shim_audio_track_source_destroy(ShimAudioTrackSource* source) {
    if (source) {
        source->track = nullptr;
        source->source = nullptr;
        delete source;
    }
}

/* ============================================================================
 * DataChannel Implementation
 * ========================================================================== */

// DataChannel wrapper with observer
struct ShimDataChannelWrapper {
    webrtc::scoped_refptr<webrtc::DataChannelInterface> channel;
    ShimOnDataChannelMessage on_message = nullptr;
    void* on_message_ctx = nullptr;
    ShimOnDataChannelOpen on_open = nullptr;
    void* on_open_ctx = nullptr;
    ShimOnDataChannelClose on_close = nullptr;
    void* on_close_ctx = nullptr;
};

class DataChannelObserverImpl : public webrtc::DataChannelObserver {
public:
    explicit DataChannelObserverImpl(ShimDataChannelWrapper* wrapper)
        : wrapper_(wrapper) {}

    void OnStateChange() override {
        if (!wrapper_) return;
        auto state = wrapper_->channel->state();
        if (state == webrtc::DataChannelInterface::kOpen && wrapper_->on_open) {
            wrapper_->on_open(wrapper_->on_open_ctx);
        } else if (state == webrtc::DataChannelInterface::kClosed && wrapper_->on_close) {
            wrapper_->on_close(wrapper_->on_close_ctx);
        }
    }

    void OnMessage(const webrtc::DataBuffer& buffer) override {
        if (wrapper_ && wrapper_->on_message) {
            wrapper_->on_message(
                wrapper_->on_message_ctx,
                buffer.data.data(),
                static_cast<int>(buffer.data.size()),
                buffer.binary ? 1 : 0
            );
        }
    }

    void OnBufferedAmountChange(uint64_t sent_data_size) override {}

private:
    ShimDataChannelWrapper* wrapper_;
};

// Global registry for data channel wrappers
namespace {
    std::mutex g_dc_registry_mutex;
    std::map<webrtc::DataChannelInterface*, std::unique_ptr<ShimDataChannelWrapper>> g_dc_registry;
    std::map<ShimDataChannelWrapper*, std::unique_ptr<DataChannelObserverImpl>> g_dc_observers;
}

static ShimDataChannelWrapper* GetOrCreateWrapper(ShimDataChannel* dc) {
    if (!dc) return nullptr;
    auto channel = reinterpret_cast<webrtc::DataChannelInterface*>(dc);

    std::lock_guard<std::mutex> lock(g_dc_registry_mutex);
    auto it = g_dc_registry.find(channel);
    if (it != g_dc_registry.end()) {
        return it->second.get();
    }

    auto wrapper = std::make_unique<ShimDataChannelWrapper>();
    wrapper->channel = webrtc::scoped_refptr<webrtc::DataChannelInterface>(channel);
    auto* raw_wrapper = wrapper.get();

    auto observer = std::make_unique<DataChannelObserverImpl>(raw_wrapper);
    channel->RegisterObserver(observer.get());

    g_dc_observers[raw_wrapper] = std::move(observer);
    g_dc_registry[channel] = std::move(wrapper);

    return raw_wrapper;
}

SHIM_EXPORT void shim_data_channel_set_on_message(
    ShimDataChannel* dc,
    ShimOnDataChannelMessage callback,
    void* ctx
) {
    auto* wrapper = GetOrCreateWrapper(dc);
    if (wrapper) {
        wrapper->on_message = callback;
        wrapper->on_message_ctx = ctx;
    }
}

SHIM_EXPORT void shim_data_channel_set_on_open(
    ShimDataChannel* dc,
    ShimOnDataChannelOpen callback,
    void* ctx
) {
    auto* wrapper = GetOrCreateWrapper(dc);
    if (wrapper) {
        wrapper->on_open = callback;
        wrapper->on_open_ctx = ctx;
    }
}

SHIM_EXPORT void shim_data_channel_set_on_close(
    ShimDataChannel* dc,
    ShimOnDataChannelClose callback,
    void* ctx
) {
    auto* wrapper = GetOrCreateWrapper(dc);
    if (wrapper) {
        wrapper->on_close = callback;
        wrapper->on_close_ctx = ctx;
    }
}

SHIM_EXPORT int shim_data_channel_send(
    ShimDataChannel* dc,
    const uint8_t* data,
    int size,
    int is_binary
) {
    if (!dc || !data) return SHIM_ERROR_INVALID_PARAM;

    auto channel = reinterpret_cast<webrtc::DataChannelInterface*>(dc);

    rtc::CopyOnWriteBuffer buffer(data, size);
    webrtc::DataBuffer db(buffer, is_binary != 0);

    return channel->Send(db) ? SHIM_OK : SHIM_ERROR_INIT_FAILED;
}

SHIM_EXPORT const char* shim_data_channel_label(ShimDataChannel* dc) {
    if (!dc) return nullptr;
    auto channel = reinterpret_cast<webrtc::DataChannelInterface*>(dc);
    return channel->label().c_str();
}

SHIM_EXPORT int shim_data_channel_ready_state(ShimDataChannel* dc) {
    if (!dc) return -1;
    auto channel = reinterpret_cast<webrtc::DataChannelInterface*>(dc);
    return static_cast<int>(channel->state());
}

SHIM_EXPORT void shim_data_channel_close(ShimDataChannel* dc) {
    if (dc) {
        auto channel = reinterpret_cast<webrtc::DataChannelInterface*>(dc);
        channel->Close();
    }
}

SHIM_EXPORT void shim_data_channel_destroy(ShimDataChannel* dc) {
    // DataChannel ref-counted, release will happen automatically
}

/* ============================================================================
 * Device Enumeration Implementation
 * ========================================================================== */

// Device capture headers (conditionally included based on build config)
#if defined(SHIM_ENABLE_DEVICE_CAPTURE)
#include "modules/video_capture/video_capture_factory.h"
#include "modules/audio_device/include/audio_device.h"
#include "modules/desktop_capture/desktop_capturer.h"
#include "modules/desktop_capture/desktop_capture_options.h"
#endif

SHIM_EXPORT int shim_enumerate_devices(
    ShimDeviceInfo* devices,
    int max_devices,
    int* out_count
) {
    if (!devices || max_devices <= 0 || !out_count) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    int count = 0;

#if defined(SHIM_ENABLE_DEVICE_CAPTURE)
    // Enumerate video input devices
    std::unique_ptr<webrtc::VideoCaptureModule::DeviceInfo> video_info(
        webrtc::VideoCaptureFactory::CreateDeviceInfo()
    );
    if (video_info) {
        int num_video = video_info->NumberOfDevices();
        for (int i = 0; i < num_video && count < max_devices; i++) {
            char device_name[256] = {0};
            char unique_id[256] = {0};
            if (video_info->GetDeviceName(i, device_name, sizeof(device_name),
                                          unique_id, sizeof(unique_id)) == 0) {
                strncpy(devices[count].device_id, unique_id, 255);
                devices[count].device_id[255] = '\0';
                strncpy(devices[count].label, device_name, 255);
                devices[count].label[255] = '\0';
                devices[count].kind = 0;  // videoinput
                count++;
            }
        }
    }

    // Enumerate audio devices using AudioDeviceModule
    webrtc::scoped_refptr<webrtc::AudioDeviceModule> adm =
        webrtc::AudioDeviceModule::Create(
            webrtc::AudioDeviceModule::kPlatformDefaultAudio,
            webrtc::CreateDefaultTaskQueueFactory().get()
        );
    if (adm && adm->Init() == 0) {
        // Audio input devices
        int16_t num_recording = adm->RecordingDevices();
        for (int16_t i = 0; i < num_recording && count < max_devices; i++) {
            char device_name[webrtc::kAdmMaxDeviceNameSize] = {0};
            char guid[webrtc::kAdmMaxGuidSize] = {0};
            if (adm->RecordingDeviceName(i, device_name, guid) == 0) {
                strncpy(devices[count].device_id, guid, 255);
                devices[count].device_id[255] = '\0';
                strncpy(devices[count].label, device_name, 255);
                devices[count].label[255] = '\0';
                devices[count].kind = 1;  // audioinput
                count++;
            }
        }

        // Audio output devices
        int16_t num_playout = adm->PlayoutDevices();
        for (int16_t i = 0; i < num_playout && count < max_devices; i++) {
            char device_name[webrtc::kAdmMaxDeviceNameSize] = {0};
            char guid[webrtc::kAdmMaxGuidSize] = {0};
            if (adm->PlayoutDeviceName(i, device_name, guid) == 0) {
                strncpy(devices[count].device_id, guid, 255);
                devices[count].device_id[255] = '\0';
                strncpy(devices[count].label, device_name, 255);
                devices[count].label[255] = '\0';
                devices[count].kind = 2;  // audiooutput
                count++;
            }
        }

        adm->Terminate();
    }
#endif  // SHIM_ENABLE_DEVICE_CAPTURE

    *out_count = count;
    return SHIM_OK;
}

/* ============================================================================
 * Video Capture Implementation
 * ========================================================================== */

#if defined(SHIM_ENABLE_DEVICE_CAPTURE)
// Forward declaration for callback adapter
class VideoCaptureDataCallback;
#endif

struct ShimVideoCapture {
    std::string device_id;
    int width;
    int height;
    int fps;
    ShimVideoCaptureCallback callback;
    void* callback_ctx;
    std::atomic<bool> running{false};
    std::mutex mutex;

#if defined(SHIM_ENABLE_DEVICE_CAPTURE)
    webrtc::scoped_refptr<webrtc::VideoCaptureModule> capture_module;
    std::unique_ptr<VideoCaptureDataCallback> data_callback;
#endif
};

#if defined(SHIM_ENABLE_DEVICE_CAPTURE)
class VideoCaptureDataCallback : public rtc::VideoSinkInterface<webrtc::VideoFrame> {
public:
    explicit VideoCaptureDataCallback(ShimVideoCapture* cap) : capture_(cap) {}

    void OnFrame(const webrtc::VideoFrame& frame) override {
        if (!capture_ || !capture_->running || !capture_->callback) return;

        webrtc::scoped_refptr<webrtc::I420BufferInterface> buffer =
            frame.video_frame_buffer()->ToI420();

        capture_->callback(
            capture_->callback_ctx,
            buffer->DataY(),
            buffer->DataU(),
            buffer->DataV(),
            buffer->width(),
            buffer->height(),
            buffer->StrideY(),
            buffer->StrideU(),
            buffer->StrideV(),
            frame.timestamp_us()
        );
    }

private:
    ShimVideoCapture* capture_;
};
#endif  // SHIM_ENABLE_DEVICE_CAPTURE

SHIM_EXPORT ShimVideoCapture* shim_video_capture_create(
    const char* device_id,
    int width,
    int height,
    int fps
) {
    if (width <= 0 || height <= 0 || fps <= 0) {
        return nullptr;
    }

    auto capture = std::make_unique<ShimVideoCapture>();
    capture->device_id = device_id ? device_id : "";
    capture->width = width;
    capture->height = height;
    capture->fps = fps;
    capture->callback = nullptr;
    capture->callback_ctx = nullptr;

#if defined(SHIM_ENABLE_DEVICE_CAPTURE)
    // Get device unique ID if not provided
    std::string unique_id = capture->device_id;
    if (unique_id.empty()) {
        std::unique_ptr<webrtc::VideoCaptureModule::DeviceInfo> info(
            webrtc::VideoCaptureFactory::CreateDeviceInfo()
        );
        if (info && info->NumberOfDevices() > 0) {
            char name[256], id[256];
            info->GetDeviceName(0, name, sizeof(name), id, sizeof(id));
            unique_id = id;
        }
    }

    if (!unique_id.empty()) {
        capture->capture_module = webrtc::VideoCaptureFactory::Create(unique_id.c_str());
        if (!capture->capture_module) {
            return nullptr;
        }
    }
#endif  // SHIM_ENABLE_DEVICE_CAPTURE

    return capture.release();
}

SHIM_EXPORT int shim_video_capture_start(
    ShimVideoCapture* cap,
    ShimVideoCaptureCallback callback,
    void* ctx
) {
    if (!cap || !callback) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    std::lock_guard<std::mutex> lock(cap->mutex);

    if (cap->running) {
        return SHIM_ERROR_INIT_FAILED;  // Already running
    }

    cap->callback = callback;
    cap->callback_ctx = ctx;
    cap->running = true;

#if defined(SHIM_ENABLE_DEVICE_CAPTURE)
    if (cap->capture_module) {
        webrtc::VideoCaptureCapability capability;
        capability.width = cap->width;
        capability.height = cap->height;
        capability.maxFPS = cap->fps;
        capability.videoType = webrtc::VideoType::kI420;

        cap->data_callback = std::make_unique<VideoCaptureDataCallback>(cap);
        cap->capture_module->RegisterCaptureDataCallback(cap->data_callback.get());

        if (cap->capture_module->StartCapture(capability) != 0) {
            cap->running = false;
            cap->callback = nullptr;
            cap->callback_ctx = nullptr;
            return SHIM_ERROR_INIT_FAILED;
        }
    }
#endif  // SHIM_ENABLE_DEVICE_CAPTURE

    return SHIM_OK;
}

SHIM_EXPORT void shim_video_capture_stop(ShimVideoCapture* cap) {
    if (!cap) return;

    std::lock_guard<std::mutex> lock(cap->mutex);

    if (!cap->running) return;

    cap->running = false;

#if defined(SHIM_ENABLE_DEVICE_CAPTURE)
    if (cap->capture_module) {
        cap->capture_module->StopCapture();
        cap->capture_module->DeRegisterCaptureDataCallback();
    }
    cap->data_callback.reset();
#endif  // SHIM_ENABLE_DEVICE_CAPTURE

    cap->callback = nullptr;
    cap->callback_ctx = nullptr;
}

SHIM_EXPORT void shim_video_capture_destroy(ShimVideoCapture* cap) {
    if (!cap) return;

    shim_video_capture_stop(cap);

#if defined(SHIM_ENABLE_DEVICE_CAPTURE)
    cap->capture_module = nullptr;
#endif

    delete cap;
}

/* ============================================================================
 * Audio Capture Implementation
 * ========================================================================== */

#if defined(SHIM_ENABLE_DEVICE_CAPTURE)
class AudioTransportCallback;
#endif

struct ShimAudioCapture {
    std::string device_id;
    int sample_rate;
    int channels;
    ShimAudioCaptureCallback callback;
    void* callback_ctx;
    std::atomic<bool> running{false};
    std::mutex mutex;
    int device_index;

#if defined(SHIM_ENABLE_DEVICE_CAPTURE)
    webrtc::scoped_refptr<webrtc::AudioDeviceModule> adm;
    std::unique_ptr<AudioTransportCallback> transport;
#endif
};

#if defined(SHIM_ENABLE_DEVICE_CAPTURE)
class AudioTransportCallback : public webrtc::AudioTransport {
public:
    explicit AudioTransportCallback(ShimAudioCapture* cap) : capture_(cap) {}

    int32_t RecordedDataIsAvailable(
        const void* audioSamples,
        size_t nSamples,
        size_t nBytesPerSample,
        size_t nChannels,
        uint32_t samplesPerSec,
        uint32_t totalDelayMS,
        int32_t clockDrift,
        uint32_t currentMicLevel,
        bool keyPressed,
        uint32_t& newMicLevel
    ) override {
        if (!capture_ || !capture_->running || !capture_->callback) {
            return 0;
        }

        int64_t timestamp_us = std::chrono::duration_cast<std::chrono::microseconds>(
            std::chrono::steady_clock::now().time_since_epoch()
        ).count();

        capture_->callback(
            capture_->callback_ctx,
            static_cast<const int16_t*>(audioSamples),
            static_cast<int>(nSamples),
            static_cast<int>(nChannels),
            static_cast<int>(samplesPerSec),
            timestamp_us
        );

        newMicLevel = currentMicLevel;
        return 0;
    }

    int32_t NeedMorePlayData(
        size_t nSamples,
        size_t nBytesPerSample,
        size_t nChannels,
        uint32_t samplesPerSec,
        void* audioSamples,
        size_t& nSamplesOut,
        int64_t* elapsed_time_ms,
        int64_t* ntp_time_ms
    ) override {
        nSamplesOut = 0;
        return 0;
    }

    void PullRenderData(
        int bits_per_sample,
        int sample_rate,
        size_t number_of_channels,
        size_t number_of_frames,
        void* audio_data,
        int64_t* elapsed_time_ms,
        int64_t* ntp_time_ms
    ) override {}

private:
    ShimAudioCapture* capture_;
};
#endif  // SHIM_ENABLE_DEVICE_CAPTURE

SHIM_EXPORT ShimAudioCapture* shim_audio_capture_create(
    const char* device_id,
    int sample_rate,
    int channels
) {
    if (sample_rate <= 0 || channels <= 0 || channels > 2) {
        return nullptr;
    }

    auto capture = std::make_unique<ShimAudioCapture>();
    capture->device_id = device_id ? device_id : "";
    capture->sample_rate = sample_rate;
    capture->channels = channels;
    capture->callback = nullptr;
    capture->callback_ctx = nullptr;
    capture->device_index = 0;

#if defined(SHIM_ENABLE_DEVICE_CAPTURE)
    capture->adm = webrtc::AudioDeviceModule::Create(
        webrtc::AudioDeviceModule::kPlatformDefaultAudio,
        webrtc::CreateDefaultTaskQueueFactory().get()
    );

    if (!capture->adm || capture->adm->Init() != 0) {
        return nullptr;
    }

    // Find device by ID if specified
    if (!capture->device_id.empty()) {
        int16_t num_devices = capture->adm->RecordingDevices();
        for (int16_t i = 0; i < num_devices; i++) {
            char name[webrtc::kAdmMaxDeviceNameSize] = {0};
            char guid[webrtc::kAdmMaxGuidSize] = {0};
            if (capture->adm->RecordingDeviceName(i, name, guid) == 0) {
                if (capture->device_id == guid) {
                    capture->device_index = i;
                    break;
                }
            }
        }
    }
#endif  // SHIM_ENABLE_DEVICE_CAPTURE

    return capture.release();
}

SHIM_EXPORT int shim_audio_capture_start(
    ShimAudioCapture* cap,
    ShimAudioCaptureCallback callback,
    void* ctx
) {
    if (!cap || !callback) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    std::lock_guard<std::mutex> lock(cap->mutex);

    if (cap->running) {
        return SHIM_ERROR_INIT_FAILED;
    }

    cap->callback = callback;
    cap->callback_ctx = ctx;
    cap->running = true;

#if defined(SHIM_ENABLE_DEVICE_CAPTURE)
    if (cap->adm) {
        cap->transport = std::make_unique<AudioTransportCallback>(cap);
        cap->adm->RegisterAudioCallback(cap->transport.get());

        if (cap->adm->SetRecordingDevice(cap->device_index) != 0) {
            cap->running = false;
            return SHIM_ERROR_INIT_FAILED;
        }

        if (cap->adm->InitRecording() != 0) {
            cap->running = false;
            return SHIM_ERROR_INIT_FAILED;
        }

        if (cap->adm->StartRecording() != 0) {
            cap->running = false;
            return SHIM_ERROR_INIT_FAILED;
        }
    }
#endif  // SHIM_ENABLE_DEVICE_CAPTURE

    return SHIM_OK;
}

SHIM_EXPORT void shim_audio_capture_stop(ShimAudioCapture* cap) {
    if (!cap) return;

    std::lock_guard<std::mutex> lock(cap->mutex);

    if (!cap->running) return;

    cap->running = false;

#if defined(SHIM_ENABLE_DEVICE_CAPTURE)
    if (cap->adm) {
        cap->adm->StopRecording();
        cap->adm->RegisterAudioCallback(nullptr);
    }
    cap->transport.reset();
#endif  // SHIM_ENABLE_DEVICE_CAPTURE

    cap->callback = nullptr;
    cap->callback_ctx = nullptr;
}

SHIM_EXPORT void shim_audio_capture_destroy(ShimAudioCapture* cap) {
    if (!cap) return;

    shim_audio_capture_stop(cap);

#if defined(SHIM_ENABLE_DEVICE_CAPTURE)
    if (cap->adm) {
        cap->adm->Terminate();
        cap->adm = nullptr;
    }
#endif

    delete cap;
}

/* ============================================================================
 * Screen/Window Capture Implementation
 * ========================================================================== */

#if defined(SHIM_ENABLE_DEVICE_CAPTURE)
class ScreenCaptureCallback;
#endif

struct ShimScreenCapture {
    int64_t source_id;
    bool is_window;
    int fps;
    ShimVideoCaptureCallback callback;
    void* callback_ctx;
    std::atomic<bool> running{false};
    std::mutex mutex;
    std::unique_ptr<std::thread> capture_thread;

#if defined(SHIM_ENABLE_DEVICE_CAPTURE)
    std::unique_ptr<webrtc::DesktopCapturer> capturer;
    std::unique_ptr<ScreenCaptureCallback> capture_callback;
#endif
};

#if defined(SHIM_ENABLE_DEVICE_CAPTURE)
class ScreenCaptureCallback : public webrtc::DesktopCapturer::Callback {
public:
    explicit ScreenCaptureCallback(ShimScreenCapture* cap) : capture_(cap) {}

    void OnCaptureResult(
        webrtc::DesktopCapturer::Result result,
        std::unique_ptr<webrtc::DesktopFrame> frame
    ) override {
        if (result != webrtc::DesktopCapturer::Result::SUCCESS ||
            !frame || !capture_ || !capture_->running || !capture_->callback) {
            return;
        }

        // Convert ARGB/BGRA to I420
        int width = frame->size().width();
        int height = frame->size().height();

        // Allocate I420 buffer
        int y_size = width * height;
        int uv_size = ((width + 1) / 2) * ((height + 1) / 2);
        std::vector<uint8_t> i420_buffer(y_size + uv_size * 2);

        uint8_t* y_plane = i420_buffer.data();
        uint8_t* u_plane = y_plane + y_size;
        uint8_t* v_plane = u_plane + uv_size;

        const uint8_t* argb = frame->data();
        int argb_stride = frame->stride();

        // Simple ARGB to I420 conversion
        for (int row = 0; row < height; ++row) {
            for (int col = 0; col < width; ++col) {
                int idx = row * argb_stride + col * 4;
                uint8_t b = argb[idx + 0];
                uint8_t g = argb[idx + 1];
                uint8_t r = argb[idx + 2];

                // Y
                int y = ((66 * r + 129 * g + 25 * b + 128) >> 8) + 16;
                y_plane[row * width + col] = static_cast<uint8_t>(std::clamp(y, 0, 255));

                // U/V (subsample 2x2)
                if ((row % 2 == 0) && (col % 2 == 0)) {
                    int uv_row = row / 2;
                    int uv_col = col / 2;
                    int uv_width = (width + 1) / 2;
                    int u = ((-38 * r - 74 * g + 112 * b + 128) >> 8) + 128;
                    int v = ((112 * r - 94 * g - 18 * b + 128) >> 8) + 128;
                    u_plane[uv_row * uv_width + uv_col] = static_cast<uint8_t>(std::clamp(u, 0, 255));
                    v_plane[uv_row * uv_width + uv_col] = static_cast<uint8_t>(std::clamp(v, 0, 255));
                }
            }
        }

        int64_t timestamp_us = std::chrono::duration_cast<std::chrono::microseconds>(
            std::chrono::steady_clock::now().time_since_epoch()
        ).count();

        capture_->callback(
            capture_->callback_ctx,
            y_plane,
            u_plane,
            v_plane,
            width,
            height,
            width,              // y_stride
            (width + 1) / 2,    // u_stride
            (width + 1) / 2,    // v_stride
            timestamp_us
        );
    }

private:
    ShimScreenCapture* capture_;
};
#endif  // SHIM_ENABLE_DEVICE_CAPTURE

SHIM_EXPORT int shim_enumerate_screens(
    ShimScreenInfo* screens,
    int max_screens,
    int* out_count
) {
    if (!screens || max_screens <= 0 || !out_count) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    int count = 0;

#if defined(SHIM_ENABLE_DEVICE_CAPTURE)
    webrtc::DesktopCaptureOptions options =
        webrtc::DesktopCaptureOptions::CreateDefault();

    // Enumerate screens
    auto screen_capturer = webrtc::DesktopCapturer::CreateScreenCapturer(options);
    if (screen_capturer) {
        webrtc::DesktopCapturer::SourceList sources;
        if (screen_capturer->GetSourceList(&sources)) {
            for (const auto& source : sources) {
                if (count >= max_screens) break;
                screens[count].id = source.id;
                strncpy(screens[count].title, source.title.c_str(), 255);
                screens[count].title[255] = '\0';
                screens[count].is_window = 0;
                count++;
            }
        }
    }

    // Enumerate windows
    auto window_capturer = webrtc::DesktopCapturer::CreateWindowCapturer(options);
    if (window_capturer) {
        webrtc::DesktopCapturer::SourceList sources;
        if (window_capturer->GetSourceList(&sources)) {
            for (const auto& source : sources) {
                if (count >= max_screens) break;
                screens[count].id = source.id;
                strncpy(screens[count].title, source.title.c_str(), 255);
                screens[count].title[255] = '\0';
                screens[count].is_window = 1;
                count++;
            }
        }
    }
#endif  // SHIM_ENABLE_DEVICE_CAPTURE

    *out_count = count;
    return SHIM_OK;
}

SHIM_EXPORT ShimScreenCapture* shim_screen_capture_create(
    int64_t screen_or_window_id,
    int is_window,
    int fps
) {
    if (fps <= 0) {
        return nullptr;
    }

    auto capture = std::make_unique<ShimScreenCapture>();
    capture->source_id = screen_or_window_id;
    capture->is_window = is_window != 0;
    capture->fps = fps;
    capture->callback = nullptr;
    capture->callback_ctx = nullptr;

#if defined(SHIM_ENABLE_DEVICE_CAPTURE)
    webrtc::DesktopCaptureOptions options =
        webrtc::DesktopCaptureOptions::CreateDefault();

    if (is_window) {
        capture->capturer = webrtc::DesktopCapturer::CreateWindowCapturer(options);
    } else {
        capture->capturer = webrtc::DesktopCapturer::CreateScreenCapturer(options);
    }

    if (!capture->capturer) {
        return nullptr;
    }

    if (!capture->capturer->SelectSource(screen_or_window_id)) {
        return nullptr;
    }
#endif  // SHIM_ENABLE_DEVICE_CAPTURE

    return capture.release();
}

SHIM_EXPORT int shim_screen_capture_start(
    ShimScreenCapture* cap,
    ShimVideoCaptureCallback callback,
    void* ctx
) {
    if (!cap || !callback) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    std::lock_guard<std::mutex> lock(cap->mutex);

    if (cap->running) {
        return SHIM_ERROR_INIT_FAILED;
    }

    cap->callback = callback;
    cap->callback_ctx = ctx;
    cap->running = true;

#if defined(SHIM_ENABLE_DEVICE_CAPTURE)
    if (cap->capturer) {
        cap->capture_callback = std::make_unique<ScreenCaptureCallback>(cap);
        cap->capturer->Start(cap->capture_callback.get());

        // Start capture loop in separate thread
        cap->capture_thread = std::make_unique<std::thread>([cap]() {
            auto frame_interval = std::chrono::milliseconds(1000 / cap->fps);
            while (cap->running) {
                auto start = std::chrono::steady_clock::now();
                cap->capturer->CaptureFrame();
                auto elapsed = std::chrono::steady_clock::now() - start;
                if (elapsed < frame_interval) {
                    std::this_thread::sleep_for(frame_interval - elapsed);
                }
            }
        });
    }
#endif  // SHIM_ENABLE_DEVICE_CAPTURE

    return SHIM_OK;
}

SHIM_EXPORT void shim_screen_capture_stop(ShimScreenCapture* cap) {
    if (!cap) return;

    {
        std::lock_guard<std::mutex> lock(cap->mutex);
        if (!cap->running) return;
        cap->running = false;
    }

    // Wait for capture thread to finish
    if (cap->capture_thread && cap->capture_thread->joinable()) {
        cap->capture_thread->join();
    }
    cap->capture_thread.reset();

#if defined(SHIM_ENABLE_DEVICE_CAPTURE)
    cap->capture_callback.reset();
#endif

    cap->callback = nullptr;
    cap->callback_ctx = nullptr;
}

SHIM_EXPORT void shim_screen_capture_destroy(ShimScreenCapture* cap) {
    if (!cap) return;

    shim_screen_capture_stop(cap);

#if defined(SHIM_ENABLE_DEVICE_CAPTURE)
    cap->capturer.reset();
#endif

    delete cap;
}

/* ============================================================================
 * Remote Track Sink (for receiving frames from remote tracks)
 * ========================================================================== */

// Forward declarations for callback types
typedef void (*ShimOnVideoFrame)(
    void* ctx,
    int width, int height,
    const uint8_t* y_plane, const uint8_t* u_plane, const uint8_t* v_plane,
    int y_stride, int u_stride, int v_stride,
    int64_t timestamp_us
);

typedef void (*ShimOnAudioFrame)(
    void* ctx,
    const int16_t* samples,
    int num_samples,
    int sample_rate,
    int channels,
    int64_t timestamp_us
);

// Video sink that calls back to Go
class GoVideoSink : public rtc::VideoSinkInterface<webrtc::VideoFrame> {
public:
    GoVideoSink(ShimOnVideoFrame callback, void* ctx)
        : callback_(callback), ctx_(ctx) {}

    void OnFrame(const webrtc::VideoFrame& frame) override {
        if (!callback_) return;

        webrtc::scoped_refptr<webrtc::I420BufferInterface> buffer =
            frame.video_frame_buffer()->ToI420();

        callback_(
            ctx_,
            buffer->width(),
            buffer->height(),
            buffer->DataY(),
            buffer->DataU(),
            buffer->DataV(),
            buffer->StrideY(),
            buffer->StrideU(),
            buffer->StrideV(),
            frame.timestamp_us()
        );
    }

private:
    ShimOnVideoFrame callback_;
    void* ctx_;
};

// Audio sink that calls back to Go
class GoAudioSink : public webrtc::AudioTrackSinkInterface {
public:
    GoAudioSink(ShimOnAudioFrame callback, void* ctx)
        : callback_(callback), ctx_(ctx) {}

    void OnData(const void* audio_data,
                int bits_per_sample,
                int sample_rate,
                size_t number_of_channels,
                size_t number_of_frames) override {
        if (!callback_) return;

        // Convert to int16
        if (bits_per_sample == 16) {
            callback_(
                ctx_,
                static_cast<const int16_t*>(audio_data),
                static_cast<int>(number_of_frames),
                sample_rate,
                static_cast<int>(number_of_channels),
                0  // timestamp not available in this callback
            );
        }
    }

private:
    ShimOnAudioFrame callback_;
    void* ctx_;
};

// Global map to track sinks (so we can remove them later)
static std::mutex g_sink_mutex;
static std::unordered_map<void*, std::unique_ptr<GoVideoSink>> g_video_sinks;
static std::unordered_map<void*, std::unique_ptr<GoAudioSink>> g_audio_sinks;

SHIM_EXPORT int shim_track_set_video_sink(
    void* track_ptr,
    ShimOnVideoFrame callback,
    void* ctx
) {
    if (!track_ptr || !callback) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    auto* track = static_cast<webrtc::MediaStreamTrackInterface*>(track_ptr);
    if (track->kind() != webrtc::MediaStreamTrackInterface::kVideoKind) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    auto* video_track = static_cast<webrtc::VideoTrackInterface*>(track);

    std::lock_guard<std::mutex> lock(g_sink_mutex);

    // Remove existing sink if any
    auto it = g_video_sinks.find(track_ptr);
    if (it != g_video_sinks.end()) {
        video_track->RemoveSink(it->second.get());
        g_video_sinks.erase(it);
    }

    // Create and add new sink
    auto sink = std::make_unique<GoVideoSink>(callback, ctx);
    video_track->AddOrUpdateSink(sink.get(), rtc::VideoSinkWants());
    g_video_sinks[track_ptr] = std::move(sink);

    return SHIM_OK;
}

SHIM_EXPORT int shim_track_set_audio_sink(
    void* track_ptr,
    ShimOnAudioFrame callback,
    void* ctx
) {
    if (!track_ptr || !callback) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    auto* track = static_cast<webrtc::MediaStreamTrackInterface*>(track_ptr);
    if (track->kind() != webrtc::MediaStreamTrackInterface::kAudioKind) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    auto* audio_track = static_cast<webrtc::AudioTrackInterface*>(track);

    std::lock_guard<std::mutex> lock(g_sink_mutex);

    // Remove existing sink if any
    auto it = g_audio_sinks.find(track_ptr);
    if (it != g_audio_sinks.end()) {
        audio_track->RemoveSink(it->second.get());
        g_audio_sinks.erase(it);
    }

    // Create and add new sink
    auto sink = std::make_unique<GoAudioSink>(callback, ctx);
    audio_track->AddSink(sink.get());
    g_audio_sinks[track_ptr] = std::move(sink);

    return SHIM_OK;
}

SHIM_EXPORT void shim_track_remove_video_sink(void* track_ptr) {
    if (!track_ptr) return;

    auto* track = static_cast<webrtc::MediaStreamTrackInterface*>(track_ptr);
    if (track->kind() != webrtc::MediaStreamTrackInterface::kVideoKind) {
        return;
    }

    auto* video_track = static_cast<webrtc::VideoTrackInterface*>(track);

    std::lock_guard<std::mutex> lock(g_sink_mutex);
    auto it = g_video_sinks.find(track_ptr);
    if (it != g_video_sinks.end()) {
        video_track->RemoveSink(it->second.get());
        g_video_sinks.erase(it);
    }
}

SHIM_EXPORT void shim_track_remove_audio_sink(void* track_ptr) {
    if (!track_ptr) return;

    auto* track = static_cast<webrtc::MediaStreamTrackInterface*>(track_ptr);
    if (track->kind() != webrtc::MediaStreamTrackInterface::kAudioKind) {
        return;
    }

    auto* audio_track = static_cast<webrtc::AudioTrackInterface*>(track);

    std::lock_guard<std::mutex> lock(g_sink_mutex);
    auto it = g_audio_sinks.find(track_ptr);
    if (it != g_audio_sinks.end()) {
        audio_track->RemoveSink(it->second.get());
        g_audio_sinks.erase(it);
    }
}

SHIM_EXPORT const char* shim_track_kind(void* track_ptr) {
    if (!track_ptr) return "";
    auto* track = static_cast<webrtc::MediaStreamTrackInterface*>(track_ptr);
    // Return static strings (safe to return)
    if (track->kind() == webrtc::MediaStreamTrackInterface::kAudioKind) {
        return "audio";
    } else if (track->kind() == webrtc::MediaStreamTrackInterface::kVideoKind) {
        return "video";
    }
    return "";
}

SHIM_EXPORT const char* shim_track_id(void* track_ptr) {
    if (!track_ptr) return "";
    auto* track = static_cast<webrtc::MediaStreamTrackInterface*>(track_ptr);
    // Note: This returns a pointer to internal string - caller must copy if needed
    static thread_local std::string id_buffer;
    id_buffer = track->id();
    return id_buffer.c_str();
}

/* ============================================================================
 * Memory helpers
 * ========================================================================== */

SHIM_EXPORT void shim_free_buffer(void* buffer) {
    free(buffer);
}

SHIM_EXPORT void shim_free_packets(void* packets, void* sizes, int count) {
    free(packets);
    free(sizes);
}

/* ============================================================================
 * RTPSender Parameters API
 * ========================================================================== */

SHIM_EXPORT int shim_rtp_sender_get_parameters(
    ShimRTPSender* sender,
    ShimRTPSendParameters* out_params,
    ShimRTPEncodingParameters* encodings,
    int max_encodings
) {
    if (!sender || !out_params || !encodings || max_encodings <= 0) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    auto* rtp_sender = static_cast<webrtc::RtpSenderInterface*>(sender);
    webrtc::RtpParameters params = rtp_sender->GetParameters();

    int count = std::min(static_cast<int>(params.encodings.size()), max_encodings);
    out_params->encoding_count = count;
    out_params->encodings = encodings;

    // Copy transaction_id
    strncpy(out_params->transaction_id, params.transaction_id.c_str(),
            sizeof(out_params->transaction_id) - 1);

    for (int i = 0; i < count; i++) {
        const auto& enc = params.encodings[i];
        memset(&encodings[i], 0, sizeof(ShimRTPEncodingParameters));

        if (enc.rid) {
            strncpy(encodings[i].rid, enc.rid->c_str(), sizeof(encodings[i].rid) - 1);
        }
        encodings[i].active = enc.active ? 1 : 0;
        if (enc.max_bitrate_bps) {
            encodings[i].max_bitrate_bps = *enc.max_bitrate_bps;
        }
        if (enc.min_bitrate_bps) {
            encodings[i].min_bitrate_bps = *enc.min_bitrate_bps;
        }
        if (enc.max_framerate) {
            encodings[i].max_framerate = *enc.max_framerate;
        }
        if (enc.scale_resolution_down_by) {
            encodings[i].scale_resolution_down_by = *enc.scale_resolution_down_by;
        }
        if (enc.scalability_mode) {
            strncpy(encodings[i].scalability_mode, enc.scalability_mode->c_str(),
                    sizeof(encodings[i].scalability_mode) - 1);
        }
    }

    return SHIM_OK;
}

SHIM_EXPORT int shim_rtp_sender_set_parameters(
    ShimRTPSender* sender,
    const ShimRTPSendParameters* params
) {
    if (!sender || !params) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    auto* rtp_sender = static_cast<webrtc::RtpSenderInterface*>(sender);
    webrtc::RtpParameters rtp_params = rtp_sender->GetParameters();

    // Update encodings
    int count = std::min(params->encoding_count, static_cast<int>(rtp_params.encodings.size()));
    for (int i = 0; i < count; i++) {
        auto& enc = rtp_params.encodings[i];
        const auto& src = params->encodings[i];

        enc.active = src.active != 0;
        if (src.max_bitrate_bps > 0) {
            enc.max_bitrate_bps = src.max_bitrate_bps;
        }
        if (src.min_bitrate_bps > 0) {
            enc.min_bitrate_bps = src.min_bitrate_bps;
        }
        if (src.max_framerate > 0) {
            enc.max_framerate = src.max_framerate;
        }
        if (src.scale_resolution_down_by > 0) {
            enc.scale_resolution_down_by = src.scale_resolution_down_by;
        }
    }

    auto result = rtp_sender->SetParameters(rtp_params);
    return result.ok() ? SHIM_OK : SHIM_ERROR_INVALID_PARAM;
}

SHIM_EXPORT void* shim_rtp_sender_get_track(ShimRTPSender* sender) {
    if (!sender) return nullptr;
    auto* rtp_sender = static_cast<webrtc::RtpSenderInterface*>(sender);
    return rtp_sender->track().get();
}

SHIM_EXPORT int shim_rtp_sender_get_stats(ShimRTPSender* sender, ShimRTCStats* out_stats) {
    if (!sender || !out_stats) {
        return SHIM_ERROR_INVALID_PARAM;
    }
    // TODO: Implement actual stats collection via RTCStatsReport
    memset(out_stats, 0, sizeof(ShimRTCStats));
    return SHIM_OK;
}

SHIM_EXPORT void shim_rtp_sender_set_on_rtcp_feedback(
    ShimRTPSender* sender,
    ShimOnRTCPFeedback callback,
    void* ctx
) {
    // TODO: Implement RTCP feedback callback
    // This requires intercepting RTCP packets in the pipeline
}

SHIM_EXPORT int shim_rtp_sender_set_layer_active(
    ShimRTPSender* sender,
    const char* rid,
    int active
) {
    if (!sender) return SHIM_ERROR_INVALID_PARAM;

    auto* rtp_sender = static_cast<webrtc::RtpSenderInterface*>(sender);
    webrtc::RtpParameters params = rtp_sender->GetParameters();

    for (auto& enc : params.encodings) {
        if (enc.rid && *enc.rid == rid) {
            enc.active = active != 0;
            auto result = rtp_sender->SetParameters(params);
            return result.ok() ? SHIM_OK : SHIM_ERROR_INVALID_PARAM;
        }
    }

    return SHIM_ERROR_INVALID_PARAM;
}

SHIM_EXPORT int shim_rtp_sender_set_layer_bitrate(
    ShimRTPSender* sender,
    const char* rid,
    uint32_t max_bitrate_bps
) {
    if (!sender) return SHIM_ERROR_INVALID_PARAM;

    auto* rtp_sender = static_cast<webrtc::RtpSenderInterface*>(sender);
    webrtc::RtpParameters params = rtp_sender->GetParameters();

    for (auto& enc : params.encodings) {
        if (enc.rid && *enc.rid == rid) {
            enc.max_bitrate_bps = max_bitrate_bps;
            auto result = rtp_sender->SetParameters(params);
            return result.ok() ? SHIM_OK : SHIM_ERROR_INVALID_PARAM;
        }
    }

    return SHIM_ERROR_INVALID_PARAM;
}

SHIM_EXPORT int shim_rtp_sender_get_active_layers(
    ShimRTPSender* sender,
    int* out_spatial,
    int* out_temporal
) {
    if (!sender || !out_spatial || !out_temporal) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    auto* rtp_sender = static_cast<webrtc::RtpSenderInterface*>(sender);
    webrtc::RtpParameters params = rtp_sender->GetParameters();

    *out_spatial = 0;
    *out_temporal = 0;

    for (const auto& enc : params.encodings) {
        if (enc.active) {
            (*out_spatial)++;
        }
    }

    // TODO: Parse scalability_mode for temporal layer count
    *out_temporal = 1;

    return SHIM_OK;
}

/* ============================================================================
 * RTPReceiver API
 * ========================================================================== */

SHIM_EXPORT void* shim_rtp_receiver_get_track(ShimRTPReceiver* receiver) {
    if (!receiver) return nullptr;
    auto* rtp_receiver = static_cast<webrtc::RtpReceiverInterface*>(receiver);
    return rtp_receiver->track().get();
}

SHIM_EXPORT int shim_rtp_receiver_get_stats(ShimRTPReceiver* receiver, ShimRTCStats* out_stats) {
    if (!receiver || !out_stats) {
        return SHIM_ERROR_INVALID_PARAM;
    }
    // TODO: Implement actual stats collection
    memset(out_stats, 0, sizeof(ShimRTCStats));
    return SHIM_OK;
}

SHIM_EXPORT int shim_rtp_receiver_request_keyframe(ShimRTPReceiver* receiver) {
    if (!receiver) return SHIM_ERROR_INVALID_PARAM;
    // TODO: Implement keyframe request via RTCP PLI
    return SHIM_OK;
}

/* ============================================================================
 * RTPTransceiver API
 * ========================================================================== */

SHIM_EXPORT int shim_transceiver_get_direction(ShimRTPTransceiver* transceiver) {
    if (!transceiver) return SHIM_TRANSCEIVER_DIRECTION_INACTIVE;
    auto* t = static_cast<webrtc::RtpTransceiverInterface*>(transceiver);

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
    auto* t = static_cast<webrtc::RtpTransceiverInterface*>(transceiver);

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
    auto* t = static_cast<webrtc::RtpTransceiverInterface*>(transceiver);

    auto current = t->current_direction();
    if (!current) return SHIM_TRANSCEIVER_DIRECTION_INACTIVE;

    switch (*current) {
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
    auto* t = static_cast<webrtc::RtpTransceiverInterface*>(transceiver);
    auto result = t->StopStandard();
    return result.ok() ? SHIM_OK : SHIM_ERROR_INVALID_PARAM;
}

SHIM_EXPORT const char* shim_transceiver_mid(ShimRTPTransceiver* transceiver) {
    if (!transceiver) return "";
    auto* t = static_cast<webrtc::RtpTransceiverInterface*>(transceiver);
    auto mid = t->mid();
    if (!mid) return "";
    static thread_local std::string mid_buffer;
    mid_buffer = *mid;
    return mid_buffer.c_str();
}

SHIM_EXPORT ShimRTPSender* shim_transceiver_get_sender(ShimRTPTransceiver* transceiver) {
    if (!transceiver) return nullptr;
    auto* t = static_cast<webrtc::RtpTransceiverInterface*>(transceiver);
    return static_cast<ShimRTPSender*>(t->sender().get());
}

SHIM_EXPORT ShimRTPReceiver* shim_transceiver_get_receiver(ShimRTPTransceiver* transceiver) {
    if (!transceiver) return nullptr;
    auto* t = static_cast<webrtc::RtpTransceiverInterface*>(transceiver);
    return static_cast<ShimRTPReceiver*>(t->receiver().get());
}

/* ============================================================================
 * PeerConnection Extended API
 * ========================================================================== */

SHIM_EXPORT ShimRTPTransceiver* shim_peer_connection_add_transceiver(
    ShimPeerConnection* pc,
    int kind,
    int direction
) {
    if (!pc) return nullptr;

    cricket::MediaType media_type = (kind == 0) ? cricket::MEDIA_TYPE_AUDIO : cricket::MEDIA_TYPE_VIDEO;

    webrtc::RtpTransceiverInit init;
    switch (direction) {
        case SHIM_TRANSCEIVER_DIRECTION_SENDRECV:
            init.direction = webrtc::RtpTransceiverDirection::kSendRecv; break;
        case SHIM_TRANSCEIVER_DIRECTION_SENDONLY:
            init.direction = webrtc::RtpTransceiverDirection::kSendOnly; break;
        case SHIM_TRANSCEIVER_DIRECTION_RECVONLY:
            init.direction = webrtc::RtpTransceiverDirection::kRecvOnly; break;
        case SHIM_TRANSCEIVER_DIRECTION_INACTIVE:
            init.direction = webrtc::RtpTransceiverDirection::kInactive; break;
        default:
            init.direction = webrtc::RtpTransceiverDirection::kSendRecv; break;
    }

    auto result = pc->AddTransceiver(media_type, init);
    if (!result.ok()) return nullptr;

    return static_cast<ShimRTPTransceiver*>(result.value().get());
}

SHIM_EXPORT int shim_peer_connection_get_senders(
    ShimPeerConnection* pc,
    ShimRTPSender** senders,
    int max_senders,
    int* out_count
) {
    if (!pc || !senders || !out_count || max_senders <= 0) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    auto sender_list = pc->GetSenders();
    int count = std::min(static_cast<int>(sender_list.size()), max_senders);

    for (int i = 0; i < count; i++) {
        senders[i] = static_cast<ShimRTPSender*>(sender_list[i].get());
    }

    *out_count = count;
    return SHIM_OK;
}

SHIM_EXPORT int shim_peer_connection_get_receivers(
    ShimPeerConnection* pc,
    ShimRTPReceiver** receivers,
    int max_receivers,
    int* out_count
) {
    if (!pc || !receivers || !out_count || max_receivers <= 0) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    auto receiver_list = pc->GetReceivers();
    int count = std::min(static_cast<int>(receiver_list.size()), max_receivers);

    for (int i = 0; i < count; i++) {
        receivers[i] = static_cast<ShimRTPReceiver*>(receiver_list[i].get());
    }

    *out_count = count;
    return SHIM_OK;
}

SHIM_EXPORT int shim_peer_connection_get_transceivers(
    ShimPeerConnection* pc,
    ShimRTPTransceiver** transceivers,
    int max_transceivers,
    int* out_count
) {
    if (!pc || !transceivers || !out_count || max_transceivers <= 0) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    auto transceiver_list = pc->GetTransceivers();
    int count = std::min(static_cast<int>(transceiver_list.size()), max_transceivers);

    for (int i = 0; i < count; i++) {
        transceivers[i] = static_cast<ShimRTPTransceiver*>(transceiver_list[i].get());
    }

    *out_count = count;
    return SHIM_OK;
}

SHIM_EXPORT int shim_peer_connection_restart_ice(ShimPeerConnection* pc) {
    if (!pc) return SHIM_ERROR_INVALID_PARAM;
    pc->RestartIce();
    return SHIM_OK;
}

SHIM_EXPORT int shim_peer_connection_get_stats(
    ShimPeerConnection* pc,
    ShimRTCStats* out_stats
) {
    if (!pc || !out_stats) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    // Initialize stats to zero
    memset(out_stats, 0, sizeof(ShimRTCStats));
    out_stats->timestamp_us = rtc::TimeMicros();

    // TODO: Implement full stats collection via GetStats callback
    // For now, return empty stats

    return SHIM_OK;
}

/* ============================================================================
 * Codec Capability API
 * ========================================================================== */

SHIM_EXPORT int shim_get_supported_video_codecs(
    ShimCodecCapability* codecs,
    int max_codecs,
    int* out_count
) {
    if (!codecs || !out_count || max_codecs <= 0) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    // List of supported video codecs
    const struct {
        const char* mime_type;
        int clock_rate;
        int payload_type;
    } video_codecs[] = {
        {"video/VP8", 90000, 96},
        {"video/VP9", 90000, 98},
        {"video/H264", 90000, 102},
        {"video/AV1", 90000, 41},
    };

    int count = 0;
    for (size_t i = 0; i < sizeof(video_codecs) / sizeof(video_codecs[0]) && count < max_codecs; i++) {
        strncpy(codecs[count].mime_type, video_codecs[i].mime_type, sizeof(codecs[count].mime_type) - 1);
        codecs[count].mime_type[sizeof(codecs[count].mime_type) - 1] = '\0';
        codecs[count].clock_rate = video_codecs[i].clock_rate;
        codecs[count].channels = 0;
        codecs[count].sdp_fmtp_line[0] = '\0';
        codecs[count].payload_type = video_codecs[i].payload_type;
        count++;
    }

    *out_count = count;
    return SHIM_OK;
}

SHIM_EXPORT int shim_get_supported_audio_codecs(
    ShimCodecCapability* codecs,
    int max_codecs,
    int* out_count
) {
    if (!codecs || !out_count || max_codecs <= 0) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    // List of supported audio codecs
    const struct {
        const char* mime_type;
        int clock_rate;
        int channels;
        int payload_type;
    } audio_codecs[] = {
        {"audio/opus", 48000, 2, 111},
        {"audio/PCMU", 8000, 1, 0},
        {"audio/PCMA", 8000, 1, 8},
    };

    int count = 0;
    for (size_t i = 0; i < sizeof(audio_codecs) / sizeof(audio_codecs[0]) && count < max_codecs; i++) {
        strncpy(codecs[count].mime_type, audio_codecs[i].mime_type, sizeof(codecs[count].mime_type) - 1);
        codecs[count].mime_type[sizeof(codecs[count].mime_type) - 1] = '\0';
        codecs[count].clock_rate = audio_codecs[i].clock_rate;
        codecs[count].channels = audio_codecs[i].channels;
        codecs[count].sdp_fmtp_line[0] = '\0';
        codecs[count].payload_type = audio_codecs[i].payload_type;
        count++;
    }

    *out_count = count;
    return SHIM_OK;
}

SHIM_EXPORT int shim_is_codec_supported(const char* mime_type) {
    if (!mime_type) return 0;

    const char* supported[] = {
        "video/VP8", "video/VP9", "video/H264", "video/AV1",
        "audio/opus", "audio/PCMU", "audio/PCMA"
    };

    for (size_t i = 0; i < sizeof(supported) / sizeof(supported[0]); i++) {
        if (strcasecmp(mime_type, supported[i]) == 0) {
            return 1;
        }
    }
    return 0;
}

/* ============================================================================
 * Bandwidth Estimation API
 * ========================================================================== */

SHIM_EXPORT void shim_peer_connection_set_on_bandwidth_estimate(
    ShimPeerConnection* pc,
    ShimOnBandwidthEstimate callback,
    void* ctx
) {
    if (!pc) return;
    // TODO: Wire up BWE callback from libwebrtc's BitrateController
    // This requires implementing a NetworkControllerObserver
}

SHIM_EXPORT int shim_peer_connection_get_bandwidth_estimate(
    ShimPeerConnection* pc,
    ShimBandwidthEstimate* out_estimate
) {
    if (!pc || !out_estimate) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    memset(out_estimate, 0, sizeof(ShimBandwidthEstimate));
    out_estimate->timestamp_us = rtc::TimeMicros();

    // TODO: Get actual BWE from libwebrtc's transport controller
    // For now, return placeholder values
    out_estimate->target_bitrate_bps = 0;
    out_estimate->available_send_bps = 0;
    out_estimate->available_recv_bps = 0;
    out_estimate->pacing_rate_bps = 0;
    out_estimate->congestion_window = 0;
    out_estimate->loss_rate = 0.0;

    return SHIM_OK;
}

/* ============================================================================
 * Version Information
 * ========================================================================== */

SHIM_EXPORT const char* shim_libwebrtc_version(void) {
    return kLibWebRTCVersion;
}

SHIM_EXPORT const char* shim_version(void) {
    return kShimVersion;
}

}  // extern "C"
