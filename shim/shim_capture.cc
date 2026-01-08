/*
 * shim_capture.cc - Video, audio, and screen capture implementation
 *
 * Provides device enumeration and capture functionality.
 * Uses conditional compilation for device capture features.
 */

#include "shim_common.h"

#include <algorithm>
#include <chrono>
#include <cstring>
#include <thread>
#include <vector>
#include <atomic>

#include "api/video/i420_buffer.h"

// Device capture headers (conditionally included based on build config)
#if defined(SHIM_ENABLE_DEVICE_CAPTURE)
#include "modules/video_capture/video_capture_factory.h"
#include "modules/audio_device/include/audio_device.h"
#include "api/audio/create_audio_device_module.h"
#include "modules/desktop_capture/desktop_capturer.h"
#include "modules/desktop_capture/desktop_capture_options.h"
#include "api/task_queue/default_task_queue_factory.h"
#endif

/* ============================================================================
 * Video Capture Implementation
 * ========================================================================== */

#if defined(SHIM_ENABLE_DEVICE_CAPTURE)
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
class VideoCaptureDataCallback : public webrtc::VideoSinkInterface<webrtc::VideoFrame> {
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
#endif

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
#endif

/* ============================================================================
 * Screen Capture Implementation
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
#endif

/* ============================================================================
 * C API Implementation
 * ========================================================================== */

extern "C" {

SHIM_EXPORT int shim_enumerate_devices(
    ShimDeviceInfo* devices,
    int max_devices,
    int* out_count,
    ShimErrorBuffer* error_out
) {
    if (!devices || max_devices <= 0 || !out_count) {
        shim::SetErrorMessage(error_out, "invalid parameter", SHIM_ERROR_INVALID_PARAM);
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
        fprintf(stderr, "SHIM DEBUG: Found %d video capture devices\n", num_video);
        for (int i = 0; i < num_video && count < max_devices; i++) {
            char device_name[256] = {0};
            char unique_id[256] = {0};
            if (video_info->GetDeviceName(i, device_name, sizeof(device_name),
                                          unique_id, sizeof(unique_id)) == 0) {
                fprintf(stderr, "SHIM DEBUG: Video device %d: %s (%s)\n", i, device_name, unique_id);
                strncpy(devices[count].device_id, unique_id, 255);
                devices[count].device_id[255] = '\0';
                strncpy(devices[count].label, device_name, 255);
                devices[count].label[255] = '\0';
                devices[count].kind = 0;  // videoinput
                count++;
            }
        }
    } else {
        fprintf(stderr, "SHIM DEBUG: VideoCaptureFactory::CreateDeviceInfo() returned nullptr - camera access may be denied\n");
    }

    // Enumerate audio devices using AudioDeviceModule
    webrtc::scoped_refptr<webrtc::AudioDeviceModule> adm =
        webrtc::CreateAudioDeviceModule(
            shim::GetEnvironment(),
            webrtc::AudioDeviceModule::kPlatformDefaultAudio
        );
    if (adm && adm->Init() == 0) {
        // Audio input devices
        int16_t num_recording = adm->RecordingDevices();
        for (int16_t i = 0; i < num_recording && count < max_devices; i++) {
            char device_name[webrtc::kAdmMaxDeviceNameSize] = {0};
            char guid[webrtc::kAdmMaxGuidSize] = {0};
            if (adm->RecordingDeviceName(i, device_name, guid) == 0) {
                // Use GUID if available, otherwise use "audioinput:<index>" as device ID
                if (guid[0] != '\0') {
                    strncpy(devices[count].device_id, guid, 255);
                } else {
                    snprintf(devices[count].device_id, 256, "audioinput:%d", i);
                }
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
                // Use GUID if available, otherwise use "audiooutput:<index>" as device ID
                if (guid[0] != '\0') {
                    strncpy(devices[count].device_id, guid, 255);
                } else {
                    snprintf(devices[count].device_id, 256, "audiooutput:%d", i);
                }
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

SHIM_EXPORT ShimVideoCapture* shim_video_capture_create(
    const char* device_id,
    int width,
    int height,
    int fps,
    ShimErrorBuffer* error_out
) {
    if (width <= 0 || height <= 0 || fps <= 0) {
        shim::SetErrorMessage(error_out, "invalid capture dimensions or fps", SHIM_ERROR_INVALID_PARAM);
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
            shim::SetErrorMessage(error_out, "video capture module creation failed");
            return nullptr;
        }
    }
#endif

    return capture.release();
}

SHIM_EXPORT int shim_video_capture_start(
    ShimVideoCapture* cap,
    ShimVideoCaptureCallback callback,
    void* ctx,
    ShimErrorBuffer* error_out
) {
    if (!cap || !callback) {
        shim::SetErrorMessage(error_out, "invalid parameter", SHIM_ERROR_INVALID_PARAM);
        return SHIM_ERROR_INVALID_PARAM;
    }

    std::lock_guard<std::mutex> lock(cap->mutex);

    if (cap->running) {
        shim::SetErrorMessage(error_out, "capture already running", SHIM_ERROR_INIT_FAILED);
        return SHIM_ERROR_INIT_FAILED;
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

        int result = cap->capture_module->StartCapture(capability);
        if (result != 0) {
            cap->running = false;
            cap->callback = nullptr;
            cap->callback_ctx = nullptr;
            char msg[64];
            snprintf(msg, sizeof(msg), "StartCapture failed with code %d", result);
            shim::SetErrorMessage(error_out, msg, SHIM_ERROR_INIT_FAILED);
            return SHIM_ERROR_INIT_FAILED;
        }
    }
#endif

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
#endif

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

SHIM_EXPORT ShimAudioCapture* shim_audio_capture_create(
    const char* device_id,
    int sample_rate,
    int channels,
    ShimErrorBuffer* error_out
) {
    if (sample_rate <= 0 || channels <= 0 || channels > 2) {
        shim::SetErrorMessage(error_out, "invalid sample rate or channels", SHIM_ERROR_INVALID_PARAM);
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
    capture->adm = webrtc::CreateAudioDeviceModule(
        shim::GetEnvironment(),
        webrtc::AudioDeviceModule::kPlatformDefaultAudio
    );

    if (!capture->adm) {
        shim::SetErrorMessage(error_out, "failed to create audio device module");
        return nullptr;
    }

    if (capture->adm->Init() != 0) {
        shim::SetErrorMessage(error_out, "audio device module initialization failed");
        return nullptr;
    }

    if (!capture->device_id.empty()) {
        // Check if device_id is in format "audioinput:<index>"
        if (capture->device_id.rfind("audioinput:", 0) == 0) {
            // Parse index from device ID
            capture->device_index = std::atoi(capture->device_id.c_str() + 11);
        } else {
            // Try to match by GUID
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
    }
#endif

    return capture.release();
}

SHIM_EXPORT int shim_audio_capture_start(
    ShimAudioCapture* cap,
    ShimAudioCaptureCallback callback,
    void* ctx,
    ShimErrorBuffer* error_out
) {
    if (!cap || !callback) {
        shim::SetErrorMessage(error_out, "invalid parameter", SHIM_ERROR_INVALID_PARAM);
        return SHIM_ERROR_INVALID_PARAM;
    }

    std::lock_guard<std::mutex> lock(cap->mutex);

    if (cap->running) {
        shim::SetErrorMessage(error_out, "audio capture already running", SHIM_ERROR_INIT_FAILED);
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
            shim::SetErrorMessage(error_out, "SetRecordingDevice failed");
            return SHIM_ERROR_INIT_FAILED;
        }

        if (cap->adm->InitRecording() != 0) {
            cap->running = false;
            shim::SetErrorMessage(error_out, "InitRecording failed");
            return SHIM_ERROR_INIT_FAILED;
        }

        if (cap->adm->StartRecording() != 0) {
            cap->running = false;
            shim::SetErrorMessage(error_out, "StartRecording failed");
            return SHIM_ERROR_INIT_FAILED;
        }
    }
#endif

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
#endif

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

SHIM_EXPORT int shim_enumerate_screens(
    ShimScreenInfo* screens,
    int max_screens,
    int* out_count,
    ShimErrorBuffer* error_out
) {
    if (!screens || max_screens <= 0 || !out_count) {
        shim::SetErrorMessage(error_out, "invalid parameter", SHIM_ERROR_INVALID_PARAM);
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
#endif

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
#endif

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
#endif

    return SHIM_OK;
}

SHIM_EXPORT void shim_screen_capture_stop(ShimScreenCapture* cap) {
    if (!cap) return;

    {
        std::lock_guard<std::mutex> lock(cap->mutex);
        if (!cap->running) return;
        cap->running = false;
    }

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

}  // extern "C"
