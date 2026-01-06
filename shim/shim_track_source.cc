/*
 * shim_track_source.cc - Pushable video and audio track sources
 *
 * Provides custom track sources that can receive frames pushed from Go.
 */

#include "shim_common.h"

#include <algorithm>
#include <cstring>
#include <optional>
#include <vector>

#include "api/audio_options.h"
#include "api/video/i420_buffer.h"
#include "api/video/video_frame.h"
#include "api/media_stream_interface.h"
#include "api/video/recordable_encoded_frame.h"
#include "api/peer_connection_interface.h"

/* ============================================================================
 * Pushable Video Track Source Implementation
 * ========================================================================== */

// Custom video track source that accepts pushed frames
class PushableVideoTrackSource : public webrtc::VideoTrackSourceInterface {
public:
    PushableVideoTrackSource(int width, int height)
        : width_(width), height_(height), state_(webrtc::MediaSourceInterface::kLive) {}

    // VideoTrackSourceInterface
    bool is_screencast() const override { return false; }
    std::optional<bool> needs_denoising() const override { return std::nullopt; }

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

    // webrtc::VideoSourceInterface<VideoFrame>
    void AddOrUpdateSink(webrtc::VideoSinkInterface<webrtc::VideoFrame>* sink,
                         const webrtc::VideoSinkWants& wants) override {
        std::lock_guard<std::mutex> lock(mutex_);
        sinks_.push_back(sink);
    }

    void RemoveSink(webrtc::VideoSinkInterface<webrtc::VideoFrame>* sink) override {
        std::lock_guard<std::mutex> lock(mutex_);
        sinks_.erase(
            std::remove(sinks_.begin(), sinks_.end(), sink),
            sinks_.end()
        );
    }

    bool SupportsEncodedOutput() const override { return false; }
    void GenerateKeyFrame() override {}
    void AddEncodedSink(webrtc::VideoSinkInterface<webrtc::RecordableEncodedFrame>*) override {}
    void RemoveEncodedSink(webrtc::VideoSinkInterface<webrtc::RecordableEncodedFrame>*) override {}

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
    std::vector<webrtc::VideoSinkInterface<webrtc::VideoFrame>*> sinks_;
};

struct ShimVideoTrackSource {
    webrtc::scoped_refptr<PushableVideoTrackSource> source;
    webrtc::scoped_refptr<webrtc::VideoTrackInterface> track;
    webrtc::scoped_refptr<webrtc::PeerConnectionFactoryInterface> factory;
    int width;
    int height;
};

/* ============================================================================
 * Pushable Audio Track Source Implementation
 * ========================================================================== */

// Custom audio track source that accepts pushed audio frames
class PushableAudioSource : public webrtc::AudioSourceInterface {
public:
    PushableAudioSource(int sample_rate, int channels)
        : sample_rate_(sample_rate), channels_(channels), state_(kLive) {}

    // AudioSourceInterface
    void SetVolume(double volume) override { volume_ = volume; }
    void RegisterAudioObserver(AudioObserver* observer) override {
        std::lock_guard<std::mutex> lock(mutex_);
        audio_observers_.push_back(observer);
    }
    void UnregisterAudioObserver(AudioObserver* observer) override {
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
    const webrtc::AudioOptions options() const override { return webrtc::AudioOptions(); }

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
    std::mutex mutex_;
    std::vector<webrtc::ObserverInterface*> observers_;
    std::vector<AudioObserver*> audio_observers_;
    std::vector<webrtc::AudioTrackSinkInterface*> sinks_;
};

struct ShimAudioTrackSource {
    webrtc::scoped_refptr<PushableAudioSource> source;
    webrtc::scoped_refptr<webrtc::AudioTrackInterface> track;
    webrtc::scoped_refptr<webrtc::PeerConnectionFactoryInterface> factory;
    int sample_rate;
    int channels;
};

/* ============================================================================
 * C API Implementation
 * ========================================================================== */

extern "C" {

SHIM_EXPORT ShimVideoTrackSource* shim_video_track_source_create(
    ShimPeerConnection* pc,
    int width,
    int height
) {
    if (!pc || width <= 0 || height <= 0) {
        return nullptr;
    }

    // Get factory from peer connection (cast to internal struct)
    auto* pc_internal = reinterpret_cast<struct ShimPeerConnectionInternal*>(pc);
    if (!pc_internal) {
        return nullptr;
    }

    auto shim_source = std::make_unique<ShimVideoTrackSource>();
    shim_source->source = webrtc::make_ref_counted<PushableVideoTrackSource>(width, height);
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

SHIM_EXPORT void shim_video_track_source_destroy(ShimVideoTrackSource* source) {
    if (source) {
        source->track = nullptr;
        source->source = nullptr;
        delete source;
    }
}

SHIM_EXPORT ShimAudioTrackSource* shim_audio_track_source_create(
    ShimPeerConnection* pc,
    int sample_rate,
    int channels
) {
    if (!pc || sample_rate <= 0 || channels <= 0 || channels > 2) {
        return nullptr;
    }

    auto shim_source = std::make_unique<ShimAudioTrackSource>();
    shim_source->source = webrtc::make_ref_counted<PushableAudioSource>(sample_rate, channels);
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

SHIM_EXPORT void shim_audio_track_source_destroy(ShimAudioTrackSource* source) {
    if (source) {
        source->track = nullptr;
        source->source = nullptr;
        delete source;
    }
}

}  // extern "C"
