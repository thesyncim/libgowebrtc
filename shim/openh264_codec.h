/*
 * openh264_codec.h - OpenH264 encoder/decoder wrappers
 *
 * Provides direct OpenH264 integration for H.264 encoding/decoding,
 * bypassing libwebrtc's codec factories. OpenH264 is loaded dynamically
 * at runtime using dlsym() since Go loads it with RTLD_GLOBAL.
 */

#ifndef OPENH264_CODEC_H_
#define OPENH264_CODEC_H_

#include "shim.h"
#include "openh264_types.h"

#include <atomic>
#include <cstdint>
#include <mutex>
#include <vector>

namespace shim {
namespace openh264 {

// ============================================================================
// Dynamic Loading
// ============================================================================

// Check if OpenH264 symbols are available (already loaded by Go with RTLD_GLOBAL)
bool IsAvailable();

// Load OpenH264 symbols. Returns true if all required symbols found.
// Thread-safe, only loads once.
bool Load();

// ============================================================================
// OpenH264Encoder
// ============================================================================

class OpenH264Encoder {
public:
    OpenH264Encoder();
    ~OpenH264Encoder();

    // Non-copyable
    OpenH264Encoder(const OpenH264Encoder&) = delete;
    OpenH264Encoder& operator=(const OpenH264Encoder&) = delete;

    // Initialize encoder with configuration matching libwebrtc settings
    // Returns 0 on success, negative error code on failure
    int Initialize(const ShimVideoEncoderConfig* config, ShimErrorBuffer* error_out);

    // Encode a frame
    // Returns 0 on success, negative error code on failure
    // On success, encoded data is in output vector, is_keyframe indicates frame type
    int Encode(
        const uint8_t* y_plane, const uint8_t* u_plane, const uint8_t* v_plane,
        int y_stride, int u_stride, int v_stride,
        uint32_t timestamp, bool force_keyframe,
        uint8_t* dst_buffer, int dst_buffer_size,
        int* out_size, bool* is_keyframe,
        ShimErrorBuffer* error_out
    );

    // Set target bitrate (bps)
    int SetBitrate(uint32_t bitrate_bps);

    // Set target framerate
    int SetFramerate(float framerate);

    // Request keyframe on next encode
    void RequestKeyframe();

    // Get encoder width
    int Width() const { return width_; }

    // Get encoder height
    int Height() const { return height_; }

private:
    void Release();

    void* encoder_ = nullptr;  // ISVCEncoder*
    int width_ = 0;
    int height_ = 0;
    float framerate_ = 30.0f;
    std::atomic<bool> force_keyframe_{false};
    std::mutex encode_mutex_;
};

// ============================================================================
// OpenH264Decoder
// ============================================================================

class OpenH264Decoder {
public:
    OpenH264Decoder();
    ~OpenH264Decoder();

    // Non-copyable
    OpenH264Decoder(const OpenH264Decoder&) = delete;
    OpenH264Decoder& operator=(const OpenH264Decoder&) = delete;

    // Initialize decoder
    // Returns 0 on success, negative error code on failure
    int Initialize(ShimErrorBuffer* error_out);

    // Decode a frame
    // Returns 0 on success, negative error code on failure
    // Output is written to y_dst, u_dst, v_dst with dimensions in out_* params
    int Decode(
        const uint8_t* data, int size,
        uint32_t timestamp, bool is_keyframe,
        uint8_t* y_dst, uint8_t* u_dst, uint8_t* v_dst,
        int* out_width, int* out_height,
        int* out_y_stride, int* out_u_stride, int* out_v_stride,
        ShimErrorBuffer* error_out
    );

private:
    void Release();

    void* decoder_ = nullptr;  // ISVCDecoder*
    std::mutex decode_mutex_;
};

}  // namespace openh264
}  // namespace shim

#endif  // OPENH264_CODEC_H_
