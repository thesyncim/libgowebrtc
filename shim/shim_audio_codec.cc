/*
 * shim_audio_codec.cc - Audio encoder and decoder implementation
 *
 * Provides Opus audio encoding and decoding using libwebrtc's
 * built-in audio codec factories.
 */

#include "shim_common.h"

#include <cstring>

#include "api/audio_codecs/opus/audio_encoder_opus.h"
#include "api/audio_codecs/opus/audio_decoder_opus.h"
#include "rtc_base/buffer.h"

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

extern "C" {

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

    // M141: MakeAudioEncoder now requires Environment and Options
    webrtc::AudioEncoderFactory::Options options;
    options.payload_type = 96;
    auto encoder = webrtc::AudioEncoderOpus::MakeAudioEncoder(
        shim::GetEnvironment(),
        opus_config,
        options
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
    webrtc::Buffer encoded_buffer;

    webrtc::AudioEncoder::EncodedInfo info = encoder->encoder->Encode(
        0,  // timestamp
        webrtc::ArrayView<const int16_t>(pcm, samples_per_channel * encoder->channels),
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

    // M141: MakeAudioDecoder now requires Environment
    auto decoder = webrtc::AudioDecoderOpus::MakeAudioDecoder(
        shim::GetEnvironment(),
        config
    );
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
 * Codec Capability API
 * ========================================================================== */

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

}  // extern "C"
