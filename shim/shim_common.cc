/*
 * shim_common.cc - Global initialization and utilities
 *
 * Provides thread management, Environment creation for M141+ API,
 * and codec conversion utilities.
 */

#include "shim_common.h"

#include <algorithm>
#include <cctype>
#include <cstdlib>
#include <mutex>

namespace shim {

// Version strings
const char* kShimVersion = "0.2.2";  // Force rebuild with --whole-archive for Linux
const char* kLibWebRTCVersion = "M141";  // crow-misia/libwebrtc-bin 141.7390.2.0

namespace {

// Global initialization state
std::once_flag g_init_flag;
std::unique_ptr<webrtc::Thread> g_signaling_thread;
std::unique_ptr<webrtc::Thread> g_worker_thread;
std::unique_ptr<webrtc::Thread> g_network_thread;

// Global Environment for M141+ API
std::once_flag g_env_flag;
std::unique_ptr<webrtc::Environment> g_environment;

bool IsTruthyEnv(const char* value) {
    if (!value) {
        return false;
    }
    std::string lowered(value);
    if (lowered.empty()) {
        return false;
    }
    std::transform(lowered.begin(), lowered.end(), lowered.begin(),
                   [](unsigned char c) { return static_cast<char>(std::tolower(c)); });
    return lowered != "0" && lowered != "false";
}

}  // namespace

void InitializeGlobals() {
    std::call_once(g_init_flag, []() {
        g_signaling_thread = webrtc::Thread::Create();
        g_signaling_thread->SetName("signaling_thread", nullptr);
        g_signaling_thread->Start();

        g_worker_thread = webrtc::Thread::Create();
        g_worker_thread->SetName("worker_thread", nullptr);
        g_worker_thread->Start();

        g_network_thread = webrtc::Thread::CreateWithSocketServer();
        g_network_thread->SetName("network_thread", nullptr);
        g_network_thread->Start();
    });
}

const webrtc::Environment& GetEnvironment() {
    std::call_once(g_env_flag, []() {
        g_environment = std::make_unique<webrtc::Environment>(
            webrtc::EnvironmentFactory().Create()
        );
    });
    return *g_environment;
}

webrtc::Thread* GetSignalingThread() {
    InitializeGlobals();
    return g_signaling_thread.get();
}

webrtc::Thread* GetWorkerThread() {
    InitializeGlobals();
    return g_worker_thread.get();
}

webrtc::Thread* GetNetworkThread() {
    InitializeGlobals();
    return g_network_thread.get();
}

bool ShouldUseSoftwareCodecs() {
    return IsTruthyEnv(std::getenv("LIBWEBRTC_PREFER_SOFTWARE_CODECS"));
}

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

webrtc::SdpVideoFormat CreateSdpVideoFormat(ShimCodecType codec, const char* h264_profile) {
    if (codec == SHIM_CODEC_AV1) {
        // Use the standard AV1 Profile 0 format (8/10-bit 4:2:0)
        return webrtc::SdpVideoFormat::AV1Profile0();
    }

    webrtc::SdpVideoFormat format(CodecTypeToString(codec));

    if (codec == SHIM_CODEC_H264) {
        // H264 requires profile-level-id and packetization-mode
        if (h264_profile) {
            format.parameters["profile-level-id"] = h264_profile;
        } else {
            // Default to Constrained Baseline Level 3.1
            format.parameters["profile-level-id"] = "42e01f";
        }
        format.parameters["packetization-mode"] = "1";
    }

    return format;
}

}  // namespace shim

// C API - Version functions
extern "C" {

SHIM_EXPORT const char* shim_version(void) {
    return shim::kShimVersion;
}

SHIM_EXPORT const char* shim_libwebrtc_version(void) {
    return shim::kLibWebRTCVersion;
}

}  // extern "C"
