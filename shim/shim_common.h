/*
 * shim_common.h - Shared declarations for libwebrtc_shim
 *
 * Contains global initialization, thread management, Environment,
 * and utility functions used across all shim modules.
 */

#ifndef SHIM_COMMON_H_
#define SHIM_COMMON_H_

#include "shim.h"

#include <memory>
#include <mutex>
#include <string>

// libwebrtc includes
#include "rtc_base/thread.h"
#include "api/environment/environment.h"
#include "api/environment/environment_factory.h"
#include "api/video_codecs/video_encoder.h"
#include "api/video_codecs/video_decoder.h"
#include "api/video_codecs/sdp_video_format.h"

namespace shim {

// Version strings
extern const char* kShimVersion;
extern const char* kLibWebRTCVersion;

// Initialize global resources (threads, etc.)
// Thread-safe, can be called multiple times.
void InitializeGlobals();

// Get the global Environment for M141+ API
const webrtc::Environment& GetEnvironment();

// Thread accessors (M141: Thread is now in webrtc namespace)
webrtc::Thread* GetSignalingThread();
webrtc::Thread* GetWorkerThread();
webrtc::Thread* GetNetworkThread();

// Codec type conversions
webrtc::VideoCodecType ToWebRTCCodecType(ShimCodecType codec);
std::string CodecTypeToString(ShimCodecType codec);

// SDP format helper
webrtc::SdpVideoFormat CreateSdpVideoFormat(ShimCodecType codec, const char* h264_profile = nullptr);

}  // namespace shim

#endif  // SHIM_COMMON_H_
