/*
 * libwebrtc_shim - C API wrapper for libwebrtc
 *
 * This implementation wraps libwebrtc's C++ API with a C interface
 * suitable for FFI bindings (Go via purego).
 */

#include "shim.h"

#include <cstring>
#include <memory>
#include <mutex>
#include <vector>
#include <string>
#include <atomic>

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
#include "modules/rtp_rtcp/source/rtp_packetizer_h264.h"
#include "modules/rtp_rtcp/source/rtp_packetizer_vp8.h"
#include "modules/rtp_rtcp/source/rtp_packetizer_vp9.h"

namespace {

// Version strings
const char* kShimVersion = "1.0.0";
const char* kLibWebRTCVersion = "M120";  // Update based on libwebrtc version

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
    rtc::scoped_refptr<webrtc::I420Buffer> buffer =
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
    rtc::scoped_refptr<webrtc::I420BufferInterface> decoded_buffer;
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
    rtc::scoped_refptr<webrtc::PeerConnectionFactoryInterface> factory;
    rtc::scoped_refptr<webrtc::PeerConnectionInterface> peer_connection;
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

    // Track senders
    std::vector<rtc::scoped_refptr<webrtc::RTPSenderInterface>> senders;
};

class PeerConnectionObserver : public webrtc::PeerConnectionObserver {
public:
    explicit PeerConnectionObserver(ShimPeerConnection* pc) : pc_(pc) {}

    void OnSignalingChange(webrtc::PeerConnectionInterface::SignalingState state) override {}
    void OnDataChannel(rtc::scoped_refptr<webrtc::DataChannelInterface> channel) override {}
    void OnRenegotiationNeeded() override {}
    void OnIceConnectionChange(webrtc::PeerConnectionInterface::IceConnectionState state) override {}
    void OnIceGatheringChange(webrtc::PeerConnectionInterface::IceGatheringState state) override {}

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

    void OnTrack(rtc::scoped_refptr<webrtc::RtpTransceiverInterface> transceiver) override {
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

    auto observer = rtc::make_ref_counted<CreateSessionDescriptionObserver>();

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

    auto observer = rtc::make_ref_counted<CreateSessionDescriptionObserver>();

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

    auto observer = rtc::make_ref_counted<SetSessionDescriptionObserver>();
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

    auto observer = rtc::make_ref_counted<SetSessionDescriptionObserver>();
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
        rtc::scoped_refptr<webrtc::RtpSenderInterface>(webrtc_sender)
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

/* ============================================================================
 * DataChannel Implementation
 * ========================================================================== */

SHIM_EXPORT void shim_data_channel_set_on_message(
    ShimDataChannel* dc,
    ShimOnDataChannelMessage callback,
    void* ctx
) {
    // TODO: Implement DataChannelObserver
}

SHIM_EXPORT void shim_data_channel_set_on_open(
    ShimDataChannel* dc,
    ShimOnDataChannelOpen callback,
    void* ctx
) {
    // TODO: Implement DataChannelObserver
}

SHIM_EXPORT void shim_data_channel_set_on_close(
    ShimDataChannel* dc,
    ShimOnDataChannelClose callback,
    void* ctx
) {
    // TODO: Implement DataChannelObserver
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
 * Version Information
 * ========================================================================== */

SHIM_EXPORT const char* shim_libwebrtc_version(void) {
    return kLibWebRTCVersion;
}

SHIM_EXPORT const char* shim_version(void) {
    return kShimVersion;
}

}  // extern "C"
