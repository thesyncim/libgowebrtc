/*
 * shim_track_source.cc - Pushable video and audio track sources
 *
 * Provides custom track sources that can receive frames pushed from Go.
 */

#include "shim_common.h"
#include "shim_internal.h"

#include <algorithm>
#include <cstring>
#include <optional>
#include <vector>

#include "api/audio_options.h"
#include "rtc_base/time_utils.h"
#include "api/video/i420_buffer.h"
#include "api/video/video_frame.h"
#include "api/media_stream_interface.h"
#include "api/video/recordable_encoded_frame.h"
#include "api/peer_connection_interface.h"
#include "api/rtp_sender_interface.h"

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

    // Returns stats about the video source
    bool GetStats(Stats* stats) override {
        if (!stats) return false;
        stats->input_width = width_;
        stats->input_height = height_;
        return true;
    }

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
        // Check if sink already exists (avoid duplicates)
        auto it = std::find(sinks_.begin(), sinks_.end(), sink);
        if (it == sinks_.end()) {
            sinks_.push_back(sink);
            fprintf(stderr, "SHIM DEBUG: AddOrUpdateSink NEW sink=%p, total sinks=%zu\n",
                    (void*)sink, sinks_.size());
        } else {
            fprintf(stderr, "SHIM DEBUG: AddOrUpdateSink UPDATE sink=%p (already registered)\n",
                    (void*)sink);
        }
        fflush(stderr);
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
    void PushFrame(webrtc::scoped_refptr<webrtc::I420Buffer> buffer, int64_t timestamp_us, uint32_t rtp_timestamp) {
        std::lock_guard<std::mutex> lock(mutex_);

        // Use real wall-clock time for timestamp_us - this is what WebRTC expects
        // The passed-in timestamp_us is used to derive RTP timestamp if not explicitly provided
        int64_t capture_time_us = webrtc::TimeMicros();

        // DEBUG: Log sink count periodically
        static int frame_count = 0;
        frame_count++;
        if (frame_count % 100 == 0) {
            fprintf(stderr, "SHIM DEBUG: PushFrame frame=%d sinks=%zu capture_us=%lld rtp_ts=%u\n",
                    frame_count, sinks_.size(), (long long)capture_time_us, rtp_timestamp);
            fflush(stderr);
        }

        webrtc::VideoFrame frame = webrtc::VideoFrame::Builder()
            .set_video_frame_buffer(buffer)
            .set_timestamp_us(capture_time_us)
            .set_timestamp_rtp(rtp_timestamp)
            .set_rotation(webrtc::kVideoRotation_0)
            .build();

        // DEBUG: Log frame delivery to sinks
        static int deliver_count = 0;
        deliver_count++;
        if (deliver_count % 100 == 0) {
            fprintf(stderr, "SHIM DEBUG: Delivering frame %d to %zu sinks, size=%dx%d rtp=%u\n",
                    deliver_count, sinks_.size(), buffer->width(), buffer->height(), rtp_timestamp);
            fflush(stderr);
        }

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
    webrtc::scoped_refptr<webrtc::PeerConnectionFactoryInterface> factory;  // Keep reference to factory for track creation
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
    webrtc::scoped_refptr<webrtc::PeerConnectionFactoryInterface> factory;  // Keep reference to factory for track creation
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

    auto shim_source = std::make_unique<ShimVideoTrackSource>();
    shim_source->source = webrtc::make_ref_counted<PushableVideoTrackSource>(width, height);
    shim_source->factory = pc->factory;  // Keep reference to factory
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

    // Convert timestamp_us back to RTP timestamp (90kHz)
    // The Go side passes timestamp_us as PTS * 1000000 / 90000
    // So RTP = timestamp_us * 90000 / 1000000 = timestamp_us * 9 / 100
    uint32_t rtp_timestamp = static_cast<uint32_t>(timestamp_us * 9 / 100);

    source->source->PushFrame(buffer, timestamp_us, rtp_timestamp);
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
    shim_source->factory = pc->factory;  // Keep reference to factory
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

SHIM_EXPORT ShimRTPSender* shim_peer_connection_add_video_track_from_source(
    ShimPeerConnection* pc,
    ShimVideoTrackSource* source,
    const char* track_id,
    const char* stream_id,
    ShimErrorBuffer* error_out
) {
    if (!pc || !pc->peer_connection || !pc->factory || !source || !source->source || !track_id) {
        shim::SetErrorMessage(error_out, "invalid parameter");
        return nullptr;
    }

    // Create video track from source
    source->track = pc->factory->CreateVideoTrack(
        source->source,
        track_id
    );

    if (!source->track) {
        shim::SetErrorMessage(error_out, "CreateVideoTrack failed");
        return nullptr;
    }

    // Ensure track is enabled
    source->track->set_enabled(true);

    // Add track to peer connection
    std::vector<std::string> stream_ids;
    if (stream_id) {
        stream_ids.push_back(stream_id);
    }

    auto result = pc->peer_connection->AddTrack(source->track, stream_ids);
    if (!result.ok()) {
        shim::SetErrorFromRTCError(error_out, result.error());
        return nullptr;
    }

    auto sender = result.value();
    pc->senders.push_back(sender);

    return reinterpret_cast<ShimRTPSender*>(sender.get());
}

SHIM_EXPORT ShimRTPSender* shim_peer_connection_add_audio_track_from_source(
    ShimPeerConnection* pc,
    ShimAudioTrackSource* source,
    const char* track_id,
    const char* stream_id,
    ShimErrorBuffer* error_out
) {
    if (!pc || !pc->peer_connection || !pc->factory || !source || !source->source || !track_id) {
        shim::SetErrorMessage(error_out, "invalid parameter");
        return nullptr;
    }

    // Create audio track from source
    source->track = pc->factory->CreateAudioTrack(
        track_id,
        source->source.get()
    );

    if (!source->track) {
        shim::SetErrorMessage(error_out, "CreateAudioTrack failed");
        return nullptr;
    }

    // Add track to peer connection
    std::vector<std::string> stream_ids;
    if (stream_id) {
        stream_ids.push_back(stream_id);
    }

    auto result = pc->peer_connection->AddTrack(source->track, stream_ids);
    if (!result.ok()) {
        shim::SetErrorFromRTCError(error_out, result.error());
        return nullptr;
    }

    auto sender = result.value();
    pc->senders.push_back(sender);

    return reinterpret_cast<ShimRTPSender*>(sender.get());
}

}  // extern "C"
