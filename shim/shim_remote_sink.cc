/*
 * shim_remote_sink.cc - Remote track sink implementation
 *
 * Provides video and audio sinks for receiving frames from remote tracks.
 */

#include "shim_common.h"

#include <map>
#include <unordered_map>
#include <cstring>

#include "api/video/i420_buffer.h"
#include "api/video/video_frame.h"
#include "api/media_stream_interface.h"

/* ============================================================================
 * Forward Declarations for Callback Types
 * ========================================================================== */

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

/* ============================================================================
 * Video Sink Implementation
 * ========================================================================== */

class GoVideoSink : public webrtc::VideoSinkInterface<webrtc::VideoFrame> {
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

/* ============================================================================
 * Audio Sink Implementation
 * ========================================================================== */

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

/* ============================================================================
 * Global Sink Registry
 * ========================================================================== */

static std::mutex g_sink_mutex;
static std::unordered_map<void*, std::unique_ptr<GoVideoSink>> g_video_sinks;
static std::unordered_map<void*, std::unique_ptr<GoAudioSink>> g_audio_sinks;

/* ============================================================================
 * C API Implementation
 * ========================================================================== */

extern "C" {

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
    video_track->AddOrUpdateSink(sink.get(), webrtc::VideoSinkWants());
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

}  // extern "C"
