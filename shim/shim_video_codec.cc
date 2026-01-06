/*
 * shim_video_codec.cc - Video encoder and decoder implementation
 *
 * Provides H.264, VP8, VP9, and AV1 video encoding and decoding
 * using libwebrtc's built-in codec factories.
 */

#include "shim_common.h"

#include <cstring>
#include <vector>
#include <atomic>

#include "api/video_codecs/builtin_video_encoder_factory.h"
#include "api/video_codecs/builtin_video_decoder_factory.h"
#include "api/video_codecs/scalability_mode.h"
#include "api/video/i420_buffer.h"
#include "api/video/video_frame.h"
#include "api/video/encoded_image.h"
#include "modules/video_coding/include/video_error_codes.h"
// InternalEncoderFactory provides all codecs when libwebrtc is built with rtc_use_h264=true
#include "media/engine/internal_encoder_factory.h"
#include "media/engine/internal_decoder_factory.h"

/* ============================================================================
 * Video Encoder Implementation
 * ========================================================================== */

struct ShimVideoEncoder {
    std::unique_ptr<webrtc::VideoEncoder> encoder;
    webrtc::VideoCodec codec_settings;
    ShimCodecType codec_type;
    std::mutex encode_mutex;       // Protects encode calls
    std::mutex output_mutex;       // Protects output data
    std::atomic<bool> force_keyframe{false};

    // Encoded frame callback storage (protected by output_mutex)
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

        std::lock_guard<std::mutex> lock(encoder_->output_mutex);

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

    // Use CreateBuiltinVideoEncoderFactory for full codec support including:
    // - VP8, VP9 (libvpx)
    // - H264 (VideoToolbox on macOS, OpenH264 on other platforms)
    // - AV1 (libaom, if enabled)
    auto factory = webrtc::CreateBuiltinVideoEncoderFactory();
    if (!factory) {
        return nullptr;
    }

    auto format = shim::CreateSdpVideoFormat(codec, config->h264_profile);
    auto encoder = factory->Create(shim::GetEnvironment(), format);
    if (!encoder) {
        return nullptr;
    }

    auto shim_encoder = std::make_unique<ShimVideoEncoder>();
    shim_encoder->encoder = std::move(encoder);
    shim_encoder->codec_type = codec;

    // Configure codec settings
    webrtc::VideoCodec& settings = shim_encoder->codec_settings;
    memset(&settings, 0, sizeof(settings));

    settings.codecType = shim::ToWebRTCCodecType(codec);
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
    } else if (codec == SHIM_CODEC_AV1) {
        settings.AV1()->automatic_resize_on = false;
        // AV1 requires scalability mode and qpMax to be set
        settings.SetScalabilityMode(webrtc::ScalabilityMode::kL1T1);
        settings.qpMax = 63;
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

    // Set initial rates - required for VP8 and other encoders before they produce output
    webrtc::VideoBitrateAllocation allocation;
    allocation.SetBitrate(0, 0, config->bitrate_bps);

    shim_encoder->encoder->SetRates(webrtc::VideoEncoder::RateControlParameters(
        allocation,
        static_cast<double>(config->framerate)
    ));

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

    // Use encode_mutex to serialize encode calls (but not output access)
    std::lock_guard<std::mutex> encode_lock(encoder->encode_mutex);

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

    // Reset output state (need output_mutex for this)
    {
        std::lock_guard<std::mutex> output_lock(encoder->output_mutex);
        encoder->has_output = false;
        encoder->encoded_data.clear();
    }

    // Encode - callback will be called synchronously and will acquire output_mutex
    int result = encoder->encoder->Encode(frame, &frame_types);
    if (result != WEBRTC_VIDEO_CODEC_OK) {
        return SHIM_ERROR_ENCODE_FAILED;
    }

    // Read output (need output_mutex)
    std::lock_guard<std::mutex> output_lock(encoder->output_mutex);

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

    std::lock_guard<std::mutex> lock(encoder->encode_mutex);

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

    std::lock_guard<std::mutex> lock(encoder->encode_mutex);

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
    std::mutex decode_mutex;   // Protects decode calls
    std::mutex output_mutex;   // Protects output access (separate to avoid deadlock)

    // Decoded frame storage
    webrtc::scoped_refptr<webrtc::I420BufferInterface> decoded_buffer;
    bool has_output = false;
};

class DecoderCallback : public webrtc::DecodedImageCallback {
public:
    explicit DecoderCallback(ShimVideoDecoder* dec) : decoder_(dec) {}

    int32_t Decoded(webrtc::VideoFrame& frame) override {
        // Use output_mutex, not decode_mutex to avoid deadlock
        std::lock_guard<std::mutex> lock(decoder_->output_mutex);

        auto buffer = frame.video_frame_buffer()->ToI420();
        decoder_->decoded_buffer = buffer;
        decoder_->has_output = true;

        return WEBRTC_VIDEO_CODEC_OK;
    }

private:
    ShimVideoDecoder* decoder_;
};

SHIM_EXPORT ShimVideoDecoder* shim_video_decoder_create(ShimCodecType codec) {
    // Use CreateBuiltinVideoDecoderFactory for full codec support including H264/VideoToolbox
    auto factory = webrtc::CreateBuiltinVideoDecoderFactory();

    auto format = shim::CreateSdpVideoFormat(codec, nullptr);
    auto decoder = factory->Create(shim::GetEnvironment(), format);
    if (!decoder) {
        return nullptr;
    }

    auto shim_decoder = std::make_unique<ShimVideoDecoder>();
    shim_decoder->decoder = std::move(decoder);
    shim_decoder->codec_type = codec;

    // Configure decoder
    webrtc::VideoDecoder::Settings settings;
    settings.set_codec_type(shim::ToWebRTCCodecType(codec));
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

    // Reset output state under output_mutex
    {
        std::lock_guard<std::mutex> lock(decoder->output_mutex);
        decoder->has_output = false;
        decoder->decoded_buffer = nullptr;
    }

    // Create encoded image
    webrtc::EncodedImage encoded;
    encoded.SetEncodedData(
        webrtc::EncodedImageBuffer::Create(data, size)
    );
    encoded.SetRtpTimestamp(timestamp);
    encoded._frameType = is_keyframe
        ? webrtc::VideoFrameType::kVideoFrameKey
        : webrtc::VideoFrameType::kVideoFrameDelta;

    // Decode under decode_mutex (callback uses output_mutex, so no deadlock)
    int result;
    {
        std::lock_guard<std::mutex> lock(decoder->decode_mutex);
        result = decoder->decoder->Decode(encoded, false, 0);
    }

    if (result != WEBRTC_VIDEO_CODEC_OK) {
        if (result == WEBRTC_VIDEO_CODEC_OK_REQUEST_KEYFRAME) {
            return SHIM_ERROR_NEED_MORE_DATA;
        }
        return SHIM_ERROR_DECODE_FAILED;
    }

    // Check and copy output under output_mutex
    std::lock_guard<std::mutex> lock(decoder->output_mutex);

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

    // Use CreateBuiltinVideoEncoderFactory for full codec support
    auto factory = webrtc::CreateBuiltinVideoEncoderFactory();
    if (!factory) {
        *out_count = 0;
        return SHIM_ERROR_INVALID_PARAM;
    }
    auto formats = factory->GetSupportedFormats();
    int count = 0;
    int payload_type = 96;

    for (const auto& format : formats) {
        if (count >= max_codecs) break;

        std::string mime = "video/" + format.name;
        strncpy(codecs[count].mime_type, mime.c_str(), sizeof(codecs[count].mime_type) - 1);
        codecs[count].mime_type[sizeof(codecs[count].mime_type) - 1] = '\0';
        codecs[count].clock_rate = 90000;
        codecs[count].channels = 0;

        // Build fmtp line from parameters
        std::string fmtp;
        for (const auto& param : format.parameters) {
            if (!fmtp.empty()) fmtp += ";";
            fmtp += param.first + "=" + param.second;
        }
        strncpy(codecs[count].sdp_fmtp_line, fmtp.c_str(), sizeof(codecs[count].sdp_fmtp_line) - 1);
        codecs[count].sdp_fmtp_line[sizeof(codecs[count].sdp_fmtp_line) - 1] = '\0';

        codecs[count].payload_type = payload_type++;
        count++;
    }

    *out_count = count;
    return SHIM_OK;
}

SHIM_EXPORT int shim_is_codec_supported(const char* mime_type) {
    if (!mime_type) return 0;

    // Audio codecs are always supported
    const char* audio_codecs[] = {"audio/opus", "audio/PCMU", "audio/PCMA"};
    for (size_t i = 0; i < sizeof(audio_codecs) / sizeof(audio_codecs[0]); i++) {
        if (strcasecmp(mime_type, audio_codecs[i]) == 0) {
            return 1;
        }
    }

    // Check video codecs against builtin factory
    auto factory = webrtc::CreateBuiltinVideoEncoderFactory();
    auto formats = factory->GetSupportedFormats();
    for (const auto& format : formats) {
        std::string mime = "video/" + format.name;
        if (strcasecmp(mime_type, mime.c_str()) == 0) {
            return 1;
        }
    }
    return 0;
}

}  // extern "C"
