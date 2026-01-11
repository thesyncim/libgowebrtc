/*
 * shim_video_codec.cc - Video encoder and decoder implementation
 *
 * Provides H.264, VP8, VP9, and AV1 video encoding and decoding.
 * - H.264: Uses OpenH264 directly on Linux, VideoToolbox via libwebrtc on macOS
 * - VP8/VP9/AV1: Uses libwebrtc's built-in codec factories
 */

#include "shim_common.h"
#include "openh264_codec.h"

#include <atomic>
#include <chrono>
#include <condition_variable>
#include <cstring>
#include <optional>
#include <vector>

// Windows compatibility: strcasecmp is _stricmp on MSVC
#ifdef _WIN32
#include <string.h>
#define strcasecmp _stricmp
#endif

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

namespace shim {

static std::string VideoCodecErrorString(int code) {
    switch (code) {
        case WEBRTC_VIDEO_CODEC_OK:
            return "ok";
        case WEBRTC_VIDEO_CODEC_OK_REQUEST_KEYFRAME:
            return "keyframe requested";
        case WEBRTC_VIDEO_CODEC_ERROR:
            return "generic video codec error";
        case WEBRTC_VIDEO_CODEC_MEMORY:
            return "out of memory";
        case WEBRTC_VIDEO_CODEC_ERR_PARAMETER:
            return "invalid parameter";
        case WEBRTC_VIDEO_CODEC_UNINITIALIZED:
            return "uninitialized";
        case WEBRTC_VIDEO_CODEC_FALLBACK_SOFTWARE:
            return "fallback to software";
        case WEBRTC_VIDEO_CODEC_TARGET_BITRATE_OVERSHOOT:
            return "target bitrate overshoot";
        case WEBRTC_VIDEO_CODEC_ERR_SIMULCAST_PARAMETERS_NOT_SUPPORTED:
            return "simulcast parameters not supported";
        case WEBRTC_VIDEO_CODEC_TIMEOUT:
            return "timeout";
        default:
            return "video codec error " + std::to_string(code);
    }
}

}  // namespace shim

/* ============================================================================
 * Video Encoder Implementation
 * ========================================================================== */

// Forward declaration for callback
class EncoderCallback;

struct ShimVideoEncoder {
    // libwebrtc encoder (for non-H264 codecs, or H264 on macOS with prefer_hw)
    std::unique_ptr<webrtc::VideoEncoder> encoder;
    std::unique_ptr<EncoderCallback> callback;  // Owns the callback
    webrtc::VideoCodec codec_settings;

    // OpenH264 direct encoder (for H264 on Linux, or macOS with prefer_hw=0)
    std::unique_ptr<shim::openh264::OpenH264Encoder> openh264_encoder;
    bool use_openh264 = false;

    ShimCodecType codec_type;
    std::mutex encode_mutex;       // Protects encode calls
    std::mutex output_mutex;       // Protects output data
    std::condition_variable output_cv;
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
        encoder_->output_cv.notify_one();

        return webrtc::EncodedImageCallback::Result(
            webrtc::EncodedImageCallback::Result::OK);
    }

private:
    ShimVideoEncoder* encoder_;
};

extern "C" {

SHIM_EXPORT ShimVideoEncoder* shim_video_encoder_create(
    ShimCodecType codec,
    const ShimVideoEncoderConfig* config,
    ShimErrorBuffer* error_out
) {
    if (!config || config->width <= 0 || config->height <= 0) {
        shim::SetErrorMessage(error_out, "invalid encoder config", SHIM_ERROR_INVALID_PARAM);
        return nullptr;
    }

    auto shim_encoder = std::make_unique<ShimVideoEncoder>();
    shim_encoder->codec_type = codec;

    // For H.264, try OpenH264 directly on Linux, or macOS with prefer_hw=0
    if (codec == SHIM_CODEC_H264) {
        bool use_openh264 = false;
#ifdef __linux__
        // On Linux, always use OpenH264 (no VideoToolbox available)
        use_openh264 = true;
#else
        // On macOS, use OpenH264 only if prefer_hw=0 or ShouldUseSoftwareCodecs()
        use_openh264 = (config->prefer_hw == 0) || shim::ShouldUseSoftwareCodecs();
#endif

        if (use_openh264 && shim::openh264::IsAvailable()) {
            auto openh264_enc = std::make_unique<shim::openh264::OpenH264Encoder>();
            int result = openh264_enc->Initialize(config, error_out);
            if (result == SHIM_OK) {
                shim_encoder->openh264_encoder = std::move(openh264_enc);
                shim_encoder->use_openh264 = true;
                // Store dimensions in codec_settings for reference
                memset(&shim_encoder->codec_settings, 0, sizeof(shim_encoder->codec_settings));
                shim_encoder->codec_settings.width = static_cast<uint16_t>(config->width);
                shim_encoder->codec_settings.height = static_cast<uint16_t>(config->height);
                shim_encoder->codec_settings.maxFramerate = static_cast<uint32_t>(config->framerate);
                return shim_encoder.release();
            }
            // OpenH264 init failed, fall through to try libwebrtc
        }
    }

    // Use libwebrtc for non-H264 codecs, or H264 on macOS with prefer_hw=1
    bool use_software = shim::ShouldUseSoftwareCodecs() || config->prefer_hw == 0;
    auto make_factory = [](bool software) -> std::unique_ptr<webrtc::VideoEncoderFactory> {
        return software
            ? std::make_unique<webrtc::InternalEncoderFactory>()
            : webrtc::CreateBuiltinVideoEncoderFactory();
    };

    auto format = shim::CreateSdpVideoFormat(codec, config->h264_profile);

    bool tried_fallback = false;
    auto factory = make_factory(use_software);
    auto encoder = factory ? factory->Create(shim::GetEnvironment(), format) : nullptr;

    if (!encoder && codec == SHIM_CODEC_H264) {
        tried_fallback = true;
        factory = make_factory(!use_software);
        encoder = factory ? factory->Create(shim::GetEnvironment(), format) : nullptr;
    }

    if (!encoder) {
        shim::SetErrorMessage(error_out, "encoder factory returned null (codec may not be supported)");
        return nullptr;
    }

    shim_encoder->encoder = std::move(encoder);

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

    shim_encoder->callback = std::make_unique<EncoderCallback>(shim_encoder.get());

    int init_result = shim_encoder->encoder->InitEncode(&settings, encoder_settings);
    if (init_result != WEBRTC_VIDEO_CODEC_OK && codec == SHIM_CODEC_H264 && !tried_fallback) {
        auto fallback_factory = make_factory(!use_software);
        auto fallback_encoder = fallback_factory
            ? fallback_factory->Create(shim::GetEnvironment(), format)
            : nullptr;
        if (fallback_encoder) {
            shim_encoder->encoder = std::move(fallback_encoder);
            init_result = shim_encoder->encoder->InitEncode(&settings, encoder_settings);
        }
    }
    if (init_result != WEBRTC_VIDEO_CODEC_OK) {
        shim::SetErrorMessage(error_out, shim::VideoCodecErrorString(init_result));
        return nullptr;
    }

    // Register callback (encoder doesn't own it, we do)
    shim_encoder->encoder->RegisterEncodeCompleteCallback(shim_encoder->callback.get());

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
    ShimVideoEncoderEncodeParams* params
) {
    if (!params) {
        return shim::SetErrorMessage(nullptr, "invalid parameter", SHIM_ERROR_INVALID_PARAM);
    }

    params->out_size = 0;
    params->out_is_keyframe = 0;

    if (!encoder || !params->y_plane || !params->u_plane || !params->v_plane ||
        !params->dst_buffer) {
        shim::SetErrorMessage(params->error_out, "invalid parameter", SHIM_ERROR_INVALID_PARAM);
        return SHIM_ERROR_INVALID_PARAM;
    }
    if (params->dst_buffer_size <= 0) {
        shim::SetErrorMessage(params->error_out, "invalid buffer size", SHIM_ERROR_INVALID_PARAM);
        return SHIM_ERROR_INVALID_PARAM;
    }

    // Use OpenH264 encoder if available for this encoder instance
    if (encoder->use_openh264 && encoder->openh264_encoder) {
        bool is_key = false;
        int result = encoder->openh264_encoder->Encode(
            params->y_plane, params->u_plane, params->v_plane,
            params->y_stride, params->u_stride, params->v_stride,
            params->timestamp, params->force_keyframe != 0,
            params->dst_buffer, params->dst_buffer_size,
            &params->out_size, &is_key,
            params->error_out
        );
        params->out_is_keyframe = is_key ? 1 : 0;
        return result;
    }

    // Use encode_mutex to serialize encode calls (but not output access)
    std::lock_guard<std::mutex> encode_lock(encoder->encode_mutex);

    int width = encoder->codec_settings.width;
    int height = encoder->codec_settings.height;

    // Create I420 buffer from input planes
    webrtc::scoped_refptr<webrtc::I420Buffer> buffer =
        webrtc::I420Buffer::Copy(
            width, height,
            params->y_plane, params->y_stride,
            params->u_plane, params->u_stride,
            params->v_plane, params->v_stride
        );

    if (!buffer) {
        return SHIM_ERROR_OUT_OF_MEMORY;
    }

    // Create video frame
    webrtc::VideoFrame frame = webrtc::VideoFrame::Builder()
        .set_video_frame_buffer(buffer)
        .set_timestamp_rtp(params->timestamp)
        .set_timestamp_ms(params->timestamp / 90)  // Convert from 90kHz to ms
        .build();

    // Determine frame types
    std::vector<webrtc::VideoFrameType> frame_types;
    if (params->force_keyframe || encoder->force_keyframe.exchange(false)) {
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
        shim::SetErrorMessage(params->error_out, shim::VideoCodecErrorString(result), SHIM_ERROR_ENCODE_FAILED);
        return SHIM_ERROR_ENCODE_FAILED;
    }

    // Read output (need output_mutex)
    std::unique_lock<std::mutex> output_lock(encoder->output_mutex);

    // Wait briefly for callback (hardware encoders can be async)
    if (!encoder->has_output) {
        constexpr auto kEncodeTimeout = std::chrono::milliseconds(200);
        encoder->output_cv.wait_for(output_lock, kEncodeTimeout, [encoder] {
            return encoder->has_output;
        });
    }

    if (!encoder->has_output || encoder->encoded_data.empty()) {
        params->out_size = 0;
        params->out_is_keyframe = 0;
        return shim::SetErrorMessage(params->error_out, "need more data", SHIM_ERROR_NEED_MORE_DATA);
    }

    // Copy encoded data to output buffer
    size_t encoded_size = encoder->encoded_data.size();
    if (static_cast<int>(encoded_size) > params->dst_buffer_size) {
        return SHIM_ERROR_BUFFER_TOO_SMALL;
    }
    memcpy(params->dst_buffer, encoder->encoded_data.data(), encoded_size);
    params->out_size = static_cast<int>(encoded_size);

    params->out_is_keyframe = encoder->is_keyframe ? 1 : 0;

    return SHIM_OK;
}

SHIM_EXPORT int shim_video_encoder_set_bitrate(
    ShimVideoEncoder* encoder,
    uint32_t bitrate_bps
) {
    if (!encoder) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    // Use OpenH264 if this encoder instance uses it
    if (encoder->use_openh264 && encoder->openh264_encoder) {
        return encoder->openh264_encoder->SetBitrate(bitrate_bps);
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

    // Use OpenH264 if this encoder instance uses it
    if (encoder->use_openh264 && encoder->openh264_encoder) {
        encoder->codec_settings.maxFramerate = static_cast<uint32_t>(framerate);
        return encoder->openh264_encoder->SetFramerate(framerate);
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

    // Use OpenH264 if this encoder instance uses it
    if (encoder->use_openh264 && encoder->openh264_encoder) {
        encoder->openh264_encoder->RequestKeyframe();
        return SHIM_OK;
    }

    encoder->force_keyframe = true;
    return SHIM_OK;
}

SHIM_EXPORT void shim_video_encoder_destroy(ShimVideoEncoder* encoder) {
    if (encoder) {
        // OpenH264 encoder is automatically destroyed via unique_ptr
        if (encoder->use_openh264) {
            // OpenH264 encoder cleanup is handled by destructor
        } else if (encoder->encoder) {
            // Unregister callback before releasing encoder to prevent use-after-free
            encoder->encoder->RegisterEncodeCompleteCallback(nullptr);
            encoder->encoder->Release();
        }
        delete encoder;  // This also destroys the callback and openh264_encoder
    }
}

/* ============================================================================
 * Video Decoder Implementation
 * ========================================================================== */

// Forward declaration for callback
class DecoderCallback;

struct ShimVideoDecoder {
    // libwebrtc decoder (for non-H264 codecs, or H264 on macOS with hardware)
    std::unique_ptr<webrtc::VideoDecoder> decoder;
    std::unique_ptr<DecoderCallback> callback;  // Owns the callback

    // OpenH264 direct decoder (for H264 on Linux, or macOS software decode)
    std::unique_ptr<shim::openh264::OpenH264Decoder> openh264_decoder;
    bool use_openh264 = false;

    ShimCodecType codec_type;
    std::mutex decode_mutex;   // Protects decode calls
    std::mutex output_mutex;   // Protects output access (separate to avoid deadlock)
    std::condition_variable output_cv;

    // Decoded frame storage
    webrtc::scoped_refptr<webrtc::I420BufferInterface> decoded_buffer;
    bool has_output = false;
};

class DecoderCallback : public webrtc::DecodedImageCallback {
public:
    explicit DecoderCallback(ShimVideoDecoder* dec) : decoder_(dec) {}

    int32_t Decoded(webrtc::VideoFrame& frame) override {
        DecodedInternal(frame);
        return WEBRTC_VIDEO_CODEC_OK;
    }

    void Decoded(webrtc::VideoFrame& frame,
                 std::optional<int32_t> decode_time_ms,
                 std::optional<uint8_t> qp) override {
        (void)decode_time_ms;
        (void)qp;
        DecodedInternal(frame);
    }

private:
    void DecodedInternal(webrtc::VideoFrame& frame) {
        // Use output_mutex, not decode_mutex to avoid deadlock
        std::lock_guard<std::mutex> lock(decoder_->output_mutex);

        auto buffer = frame.video_frame_buffer()->ToI420();
        decoder_->decoded_buffer = buffer;
        decoder_->has_output = true;
        decoder_->output_cv.notify_one();
    }

    ShimVideoDecoder* decoder_;
};

SHIM_EXPORT ShimVideoDecoder* shim_video_decoder_create(
    ShimCodecType codec,
    ShimErrorBuffer* error_out
) {
    auto shim_decoder = std::make_unique<ShimVideoDecoder>();
    shim_decoder->codec_type = codec;

    // For H.264, try OpenH264 directly on Linux
    if (codec == SHIM_CODEC_H264) {
        bool use_openh264 = false;
#ifdef __linux__
        // On Linux, always use OpenH264 (no VideoToolbox available)
        use_openh264 = true;
#else
        // On macOS, use OpenH264 only if ShouldUseSoftwareCodecs()
        use_openh264 = shim::ShouldUseSoftwareCodecs();
#endif

        if (use_openh264 && shim::openh264::IsAvailable()) {
            auto openh264_dec = std::make_unique<shim::openh264::OpenH264Decoder>();
            int result = openh264_dec->Initialize(error_out);
            if (result == SHIM_OK) {
                shim_decoder->openh264_decoder = std::move(openh264_dec);
                shim_decoder->use_openh264 = true;
                return shim_decoder.release();
            }
            // OpenH264 init failed, fall through to try libwebrtc
        }
    }

    // Use libwebrtc for non-H264 codecs, or H264 on macOS with hardware
    bool use_software = shim::ShouldUseSoftwareCodecs();
    auto make_factory = [](bool software) -> std::unique_ptr<webrtc::VideoDecoderFactory> {
        return software
            ? std::make_unique<webrtc::InternalDecoderFactory>()
            : webrtc::CreateBuiltinVideoDecoderFactory();
    };

    auto format = shim::CreateSdpVideoFormat(codec, nullptr);

    bool tried_fallback = false;
    auto factory = make_factory(use_software);
    auto decoder = factory ? factory->Create(shim::GetEnvironment(), format) : nullptr;
    if (!decoder && codec == SHIM_CODEC_H264) {
        tried_fallback = true;
        factory = make_factory(!use_software);
        decoder = factory ? factory->Create(shim::GetEnvironment(), format) : nullptr;
    }

    if (!decoder) {
        shim::SetErrorMessage(error_out, "decoder factory returned null (codec may not be supported)");
        return nullptr;
    }

    shim_decoder->decoder = std::move(decoder);

    // Configure decoder
    webrtc::VideoDecoder::Settings settings;
    settings.set_codec_type(shim::ToWebRTCCodecType(codec));
    settings.set_number_of_cores(1);
    settings.set_max_render_resolution({1920, 1080});

    shim_decoder->callback = std::make_unique<DecoderCallback>(shim_decoder.get());

    if (!shim_decoder->decoder->Configure(settings)) {
        if (codec == SHIM_CODEC_H264 && !tried_fallback) {
            auto fallback_factory = make_factory(!use_software);
            auto fallback_decoder = fallback_factory
                ? fallback_factory->Create(shim::GetEnvironment(), format)
                : nullptr;
            if (fallback_decoder) {
                shim_decoder->decoder = std::move(fallback_decoder);
                if (!shim_decoder->decoder->Configure(settings)) {
                    shim::SetErrorMessage(error_out, "decoder Configure() failed");
                    return nullptr;
                }
            } else {
                shim::SetErrorMessage(error_out, "decoder Configure() failed");
                return nullptr;
            }
        } else {
            shim::SetErrorMessage(error_out, "decoder Configure() failed");
            return nullptr;
        }
    }

    // Register callback (decoder doesn't own it, we do)
    shim_decoder->decoder->RegisterDecodeCompleteCallback(shim_decoder->callback.get());

    return shim_decoder.release();
}

SHIM_EXPORT int shim_video_decoder_decode(
    ShimVideoDecoder* decoder,
    ShimVideoDecoderDecodeParams* params
) {
    if (!params) {
        return shim::SetErrorMessage(nullptr, "invalid parameter", SHIM_ERROR_INVALID_PARAM);
    }

    params->out_width = 0;
    params->out_height = 0;
    params->out_y_stride = 0;
    params->out_u_stride = 0;
    params->out_v_stride = 0;

    if (!decoder || !params->data || params->size <= 0 ||
        !params->y_dst || !params->u_dst || !params->v_dst) {
        shim::SetErrorMessage(params->error_out, "invalid parameter", SHIM_ERROR_INVALID_PARAM);
        return SHIM_ERROR_INVALID_PARAM;
    }

    // Use OpenH264 decoder if available for this decoder instance
    if (decoder->use_openh264 && decoder->openh264_decoder) {
        return decoder->openh264_decoder->Decode(
            params->data, params->size,
            params->timestamp, params->is_keyframe != 0,
            params->y_dst, params->u_dst, params->v_dst,
            &params->out_width, &params->out_height,
            &params->out_y_stride, &params->out_u_stride, &params->out_v_stride,
            params->error_out
        );
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
        webrtc::EncodedImageBuffer::Create(params->data, params->size)
    );
    encoded.SetRtpTimestamp(params->timestamp);
    encoded._frameType = params->is_keyframe
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
            shim::SetErrorMessage(params->error_out, "keyframe requested", SHIM_ERROR_NEED_MORE_DATA);
            return SHIM_ERROR_NEED_MORE_DATA;
        }
        shim::SetErrorMessage(params->error_out, shim::VideoCodecErrorString(result), SHIM_ERROR_DECODE_FAILED);
        return SHIM_ERROR_DECODE_FAILED;
    }

    // Check and copy output under output_mutex
    std::unique_lock<std::mutex> lock(decoder->output_mutex);

    if (!decoder->has_output) {
        constexpr auto kDecodeTimeout = std::chrono::milliseconds(200);
        decoder->output_cv.wait_for(lock, kDecodeTimeout, [decoder] {
            return decoder->has_output;
        });
    }

    if (!decoder->has_output || !decoder->decoded_buffer) {
        return SHIM_ERROR_NEED_MORE_DATA;
    }

    // Copy decoded frame to output buffers
    auto& buffer = decoder->decoded_buffer;
    int width = buffer->width();
    int height = buffer->height();
    if (width == 0 || height == 0) {
        return SHIM_ERROR_NEED_MORE_DATA;
    }

    // Copy Y plane
    const uint8_t* src_y = buffer->DataY();
    int src_stride_y = buffer->StrideY();
    for (int row = 0; row < height; ++row) {
    memcpy(params->y_dst + row * width, src_y + row * src_stride_y, width);
    }

    // Copy U plane
    const uint8_t* src_u = buffer->DataU();
    int src_stride_u = buffer->StrideU();
    int uv_height = (height + 1) / 2;
    int uv_width = (width + 1) / 2;
    for (int row = 0; row < uv_height; ++row) {
    memcpy(params->u_dst + row * uv_width, src_u + row * src_stride_u, uv_width);
    }

    // Copy V plane
    const uint8_t* src_v = buffer->DataV();
    int src_stride_v = buffer->StrideV();
    for (int row = 0; row < uv_height; ++row) {
    memcpy(params->v_dst + row * uv_width, src_v + row * src_stride_v, uv_width);
    }

    params->out_width = width;
    params->out_height = height;
    params->out_y_stride = width;
    params->out_u_stride = uv_width;
    params->out_v_stride = uv_width;

    return SHIM_OK;
}

SHIM_EXPORT void shim_video_decoder_destroy(ShimVideoDecoder* decoder) {
    if (decoder) {
        // OpenH264 decoder is automatically destroyed via unique_ptr
        if (decoder->use_openh264) {
            // OpenH264 decoder cleanup is handled by destructor
        } else if (decoder->decoder) {
            // Unregister callback before releasing decoder to prevent use-after-free
            decoder->decoder->RegisterDecodeCompleteCallback(nullptr);
            decoder->decoder->Release();
        }
        delete decoder;  // This also destroys the callback and openh264_decoder
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
