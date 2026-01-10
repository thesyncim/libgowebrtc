/*
 * openh264_codec.cc - OpenH264 encoder/decoder implementation
 *
 * This file implements direct OpenH264 integration, bypassing libwebrtc's
 * codec factories. OpenH264 is loaded dynamically at runtime using dlsym()
 * since Go loads it with RTLD_GLOBAL.
 *
 * Configuration matches libwebrtc's H.264 encoder settings exactly:
 * - Constrained Baseline profile (Level 3.1)
 * - Temporal layers: 1
 * - VBR rate control
 * - Frame skipping enabled
 */

#include "openh264_codec.h"
#include "shim_common.h"

#include <cstring>
#include <mutex>

#ifndef _WIN32
#include <dlfcn.h>
#else
#include <windows.h>
#endif

namespace shim {
namespace openh264 {

// ============================================================================
// Function pointer types matching OpenH264's API
// ============================================================================

typedef int (*WelsCreateSVCEncoderFunc)(void** ppEncoder);
typedef void (*WelsDestroySVCEncoderFunc)(void* pEncoder);
typedef long (*WelsCreateDecoderFunc)(void** ppDecoder);
typedef void (*WelsDestroyDecoderFunc)(void* pDecoder);
typedef int (*WelsGetCodecVersionFunc)(void*);

// ============================================================================
// Global function pointers (lazy-loaded once)
// ============================================================================

static std::once_flag g_load_flag;
static bool g_available = false;
static WelsCreateSVCEncoderFunc g_create_encoder = nullptr;
static WelsDestroySVCEncoderFunc g_destroy_encoder = nullptr;
static WelsCreateDecoderFunc g_create_decoder = nullptr;
static WelsDestroyDecoderFunc g_destroy_decoder = nullptr;

// ============================================================================
// Dynamic loading implementation
// ============================================================================

static void* DlSym(const char* symbol) {
#ifndef _WIN32
    // RTLD_DEFAULT searches all loaded libraries including those loaded with RTLD_GLOBAL
    return dlsym(RTLD_DEFAULT, symbol);
#else
    // On Windows, we need to get the module handle first
    // Go should have loaded it already
    HMODULE hModule = GetModuleHandleA("openh264.dll");
    if (!hModule) {
        hModule = GetModuleHandleA("openh264-7.dll");  // Try versioned name
    }
    if (!hModule) {
        return nullptr;
    }
    return (void*)GetProcAddress(hModule, symbol);
#endif
}

static void LoadOnce() {
    g_create_encoder = (WelsCreateSVCEncoderFunc)DlSym("WelsCreateSVCEncoder");
    g_destroy_encoder = (WelsDestroySVCEncoderFunc)DlSym("WelsDestroySVCEncoder");
    g_create_decoder = (WelsCreateDecoderFunc)DlSym("WelsCreateDecoder");
    g_destroy_decoder = (WelsDestroyDecoderFunc)DlSym("WelsDestroyDecoder");

    g_available = (g_create_encoder != nullptr) &&
                  (g_destroy_encoder != nullptr) &&
                  (g_create_decoder != nullptr) &&
                  (g_destroy_decoder != nullptr);
}

bool Load() {
    std::call_once(g_load_flag, LoadOnce);
    return g_available;
}

bool IsAvailable() {
    Load();  // Ensure loading has been attempted
    return g_available;
}

// ============================================================================
// Vtable helpers for calling OpenH264 interface methods
// ============================================================================

// ISVCEncoder vtable layout (Itanium ABI - destructor at END since declared last):
// [0] = Initialize(const SEncParamBase*)
// [1] = InitializeExt(const SEncParamExt*)
// [2] = GetDefaultParams(SEncParamExt*)
// [3] = Uninitialize()
// [4] = EncodeFrame(const SSourcePicture*, SFrameBSInfo*)
// [5] = EncodeParameterSets(SFrameBSInfo*)
// [6] = ForceIntraFrame(bool, int)
// [7] = SetOption(ENCODER_OPTION, void*)
// [8] = GetOption(ENCODER_OPTION, void*)
// [9] = ~ISVCEncoder() (virtual destructor - deleting)
// [10] = ~ISVCEncoder() (virtual destructor - complete)
//
// Note: Destructor is declared at the END of ISVCEncoder class in OpenH264,
// so it appears at the end of the vtable, not the beginning.

// Encoder vtable indices
constexpr int kEncoderVtable_Initialize        = 0;
constexpr int kEncoderVtable_InitializeExt     = 1;
constexpr int kEncoderVtable_GetDefaultParams  = 2;
constexpr int kEncoderVtable_Uninitialize      = 3;
constexpr int kEncoderVtable_EncodeFrame       = 4;
constexpr int kEncoderVtable_EncodeParameterSets = 5;
constexpr int kEncoderVtable_ForceIntraFrame   = 6;
constexpr int kEncoderVtable_SetOption         = 7;
constexpr int kEncoderVtable_GetOption         = 8;

template<typename Ret, typename... Args>
static Ret CallEncoderMethod(void* encoder, int vtable_index, Args... args) {
    auto vtable = *reinterpret_cast<void***>(encoder);
    auto method = reinterpret_cast<Ret(*)(void*, Args...)>(vtable[vtable_index]);
    return method(encoder, args...);
}

// ISVCDecoder vtable layout (Itanium ABI - destructor at END since declared last):
// [0] = Initialize(const SDecodingParam*)
// [1] = Uninitialize()
// [2] = DecodeFrame2(const unsigned char*, int, unsigned char**, SBufferInfo*)
// [3] = DecodeFrameNoDelay(const unsigned char*, int, unsigned char**, SBufferInfo*)
// [4] = DecodeParser(const unsigned char*, int, SParserBsInfo*)
// [5] = GetOption(DECODER_OPTION, void*)
// [6] = SetOption(DECODER_OPTION, void*)
// [7] = ~ISVCDecoder() (virtual destructor - deleting)
// [8] = ~ISVCDecoder() (virtual destructor - complete)

// Decoder vtable indices
constexpr int kDecoderVtable_Initialize        = 0;
constexpr int kDecoderVtable_Uninitialize      = 1;
constexpr int kDecoderVtable_DecodeFrame2      = 2;
constexpr int kDecoderVtable_DecodeFrameNoDelay = 3;
constexpr int kDecoderVtable_DecodeParser      = 4;
constexpr int kDecoderVtable_GetOption         = 5;
constexpr int kDecoderVtable_SetOption         = 6;

template<typename Ret, typename... Args>
static Ret CallDecoderMethod(void* decoder, int vtable_index, Args... args) {
    auto vtable = *reinterpret_cast<void***>(decoder);
    auto method = reinterpret_cast<Ret(*)(void*, Args...)>(vtable[vtable_index]);
    return method(decoder, args...);
}

// ============================================================================
// OpenH264Encoder implementation
// ============================================================================

OpenH264Encoder::OpenH264Encoder() = default;

OpenH264Encoder::~OpenH264Encoder() {
    Release();
}

void OpenH264Encoder::Release() {
    if (encoder_ && g_destroy_encoder) {
        CallEncoderMethod<int>(encoder_, kEncoderVtable_Uninitialize);
        g_destroy_encoder(encoder_);
        encoder_ = nullptr;
    }
}

int OpenH264Encoder::Initialize(const ShimVideoEncoderConfig* config, ShimErrorBuffer* error_out) {
    if (!Load()) {
        return SetErrorMessage(error_out, "OpenH264 library not loaded", SHIM_ERROR_NOT_SUPPORTED);
    }

    if (!config) {
        return SetErrorMessage(error_out, "Invalid encoder config", SHIM_ERROR_INVALID_PARAM);
    }

    // Create encoder
    int ret = g_create_encoder(&encoder_);
    if (ret != 0 || !encoder_) {
        return SetErrorMessage(error_out, "WelsCreateSVCEncoder failed", SHIM_ERROR_INIT_FAILED);
    }

    // Get default params
    SEncParamExt param;
    memset(&param, 0, sizeof(param));
    ret = CallEncoderMethod<int, SEncParamExt*>(encoder_, kEncoderVtable_GetDefaultParams, &param);
    if (ret != 0) {
        Release();
        return SetErrorMessage(error_out, "GetDefaultParams failed", SHIM_ERROR_INIT_FAILED);
    }

    // Configure to match libwebrtc settings exactly
    param.iUsageType = CAMERA_VIDEO_REAL_TIME;
    param.iPicWidth = config->width;
    param.iPicHeight = config->height;
    param.iTargetBitrate = config->bitrate_bps;
    param.iMaxBitrate = config->bitrate_bps;
    param.iRCMode = RC_BITRATE_MODE;  // VBR like libwebrtc
    param.fMaxFrameRate = config->framerate > 0 ? config->framerate : 30.0f;

    // Temporal layers = 1 (matching libwebrtc)
    param.iTemporalLayerNum = 1;
    param.iSpatialLayerNum = 1;

    // Frame skipping enabled (like libwebrtc)
    param.bEnableFrameSkip = true;

    // Keyframe interval
    param.uiIntraPeriod = config->keyframe_interval > 0 ? config->keyframe_interval : 300;

    // Single spatial layer config (matching libwebrtc)
    param.sSpatialLayers[0].iVideoWidth = config->width;
    param.sSpatialLayers[0].iVideoHeight = config->height;
    param.sSpatialLayers[0].fFrameRate = param.fMaxFrameRate;
    param.sSpatialLayers[0].iSpatialBitrate = config->bitrate_bps;
    param.sSpatialLayers[0].iMaxSpatialBitrate = config->bitrate_bps;

    // Profile: Constrained Baseline (matching "42e01f" - Level 3.1)
    // OpenH264 only supports Baseline/Constrained Baseline anyway
    param.sSpatialLayers[0].uiProfileIdc = PRO_BASELINE;
    param.sSpatialLayers[0].uiLevelIdc = LEVEL_3_1;

    // Slice mode: single slice per frame
    param.sSpatialLayers[0].uiSliceMode = 0;  // SM_SINGLE_SLICE

    // Additional settings matching libwebrtc
    param.iNumRefFrame = 1;
    param.iMultipleThreadIdc = 1;  // Single thread for simplicity
    param.bEnableDenoise = false;
    param.bEnableBackgroundDetection = false;
    param.bEnableAdaptiveQuant = false;
    param.bEnableFrameCroppingFlag = true;
    param.bEnableSceneChangeDetect = false;

    // Initialize with extended params
    ret = CallEncoderMethod<int, SEncParamExt*>(encoder_, kEncoderVtable_InitializeExt, &param);
    if (ret != 0) {
        Release();
        return SetErrorMessage(error_out, "InitializeExt failed: " + std::to_string(ret), SHIM_ERROR_INIT_FAILED);
    }

    // Set video format to I420
    int videoFormat = videoFormatI420;
    CallEncoderMethod<int, ENCODER_OPTION, void*>(encoder_, kEncoderVtable_SetOption, ENCODER_OPTION_DATAFORMAT, &videoFormat);

    width_ = config->width;
    height_ = config->height;
    framerate_ = param.fMaxFrameRate;

    return SHIM_OK;
}

int OpenH264Encoder::Encode(
    const uint8_t* y_plane, const uint8_t* u_plane, const uint8_t* v_plane,
    int y_stride, int u_stride, int v_stride,
    uint32_t timestamp, bool force_keyframe,
    uint8_t* dst_buffer, int dst_buffer_size,
    int* out_size, bool* is_keyframe,
    ShimErrorBuffer* error_out
) {
    std::lock_guard<std::mutex> lock(encode_mutex_);

    if (!encoder_) {
        return SetErrorMessage(error_out, "Encoder not initialized", SHIM_ERROR_INIT_FAILED);
    }

    if (!y_plane || !u_plane || !v_plane || !dst_buffer || !out_size || !is_keyframe) {
        return SetErrorMessage(error_out, "Invalid encode parameters", SHIM_ERROR_INVALID_PARAM);
    }

    // Force keyframe if requested
    if (force_keyframe || force_keyframe_.exchange(false)) {
        // ForceIntraFrame(bool bIDR, int iLayerID) - iLayerID = -1 means all layers
        CallEncoderMethod<int, bool, int>(encoder_, kEncoderVtable_ForceIntraFrame, true, -1);
    }

    // Set up source picture
    SSourcePicture src;
    memset(&src, 0, sizeof(src));
    src.iColorFormat = videoFormatI420;
    src.iPicWidth = width_;
    src.iPicHeight = height_;
    src.iStride[0] = y_stride;
    src.iStride[1] = u_stride;
    src.iStride[2] = v_stride;
    src.pData[0] = const_cast<uint8_t*>(y_plane);
    src.pData[1] = const_cast<uint8_t*>(u_plane);
    src.pData[2] = const_cast<uint8_t*>(v_plane);
    src.uiTimeStamp = timestamp;

    // Output bitstream info
    SFrameBSInfo info;
    memset(&info, 0, sizeof(info));

    // Encode
    int ret = CallEncoderMethod<int, const SSourcePicture*, SFrameBSInfo*>(encoder_, kEncoderVtable_EncodeFrame, &src, &info);
    if (ret != 0) {
        return SetErrorMessage(error_out, "EncodeFrame failed: " + std::to_string(ret), SHIM_ERROR_ENCODE_FAILED);
    }

    // Check if frame was skipped
    if (info.eFrameType == videoFrameTypeSkip) {
        *out_size = 0;
        *is_keyframe = false;
        return SHIM_OK;
    }

    // Calculate total size and check buffer
    int total_size = 0;
    for (int layer = 0; layer < info.iLayerNum; ++layer) {
        SLayerBSInfo& layer_info = info.sLayerInfo[layer];
        for (int nal = 0; nal < layer_info.iNalCount; ++nal) {
            total_size += layer_info.pNalLengthInByte[nal];
        }
    }

    if (total_size > dst_buffer_size) {
        return SetErrorMessage(error_out, "Buffer too small for encoded frame", SHIM_ERROR_BUFFER_TOO_SMALL);
    }

    // Copy NAL units to output buffer
    // OpenH264 outputs NALs with Annex B start codes already
    int offset = 0;
    for (int layer = 0; layer < info.iLayerNum; ++layer) {
        SLayerBSInfo& layer_info = info.sLayerInfo[layer];
        int layer_offset = 0;
        for (int nal = 0; nal < layer_info.iNalCount; ++nal) {
            int nal_len = layer_info.pNalLengthInByte[nal];
            memcpy(dst_buffer + offset, layer_info.pBsBuf + layer_offset, nal_len);
            offset += nal_len;
            layer_offset += nal_len;
        }
    }

    *out_size = offset;
    *is_keyframe = (info.eFrameType == videoFrameTypeIDR);

    return SHIM_OK;
}

int OpenH264Encoder::SetBitrate(uint32_t bitrate_bps) {
    std::lock_guard<std::mutex> lock(encode_mutex_);

    if (!encoder_) {
        return SHIM_ERROR_INIT_FAILED;
    }

    SBitrateInfo bitrate_info;
    memset(&bitrate_info, 0, sizeof(bitrate_info));
    bitrate_info.iBitrate = bitrate_bps;

    CallEncoderMethod<int, ENCODER_OPTION, void*>(encoder_, kEncoderVtable_SetOption, ENCODER_OPTION_BITRATE, &bitrate_info);
    CallEncoderMethod<int, ENCODER_OPTION, void*>(encoder_, kEncoderVtable_SetOption, ENCODER_OPTION_MAX_BITRATE, &bitrate_info);

    return SHIM_OK;
}

int OpenH264Encoder::SetFramerate(float framerate) {
    std::lock_guard<std::mutex> lock(encode_mutex_);

    if (!encoder_) {
        return SHIM_ERROR_INIT_FAILED;
    }

    framerate_ = framerate;

    CallEncoderMethod<int, ENCODER_OPTION, void*>(encoder_, kEncoderVtable_SetOption, ENCODER_OPTION_FRAME_RATE, &framerate);

    return SHIM_OK;
}

void OpenH264Encoder::RequestKeyframe() {
    force_keyframe_ = true;
}

// ============================================================================
// OpenH264Decoder implementation
// ============================================================================

OpenH264Decoder::OpenH264Decoder() = default;

OpenH264Decoder::~OpenH264Decoder() {
    Release();
}

void OpenH264Decoder::Release() {
    if (decoder_ && g_destroy_decoder) {
        CallDecoderMethod<long>(decoder_, kDecoderVtable_Uninitialize);
        g_destroy_decoder(decoder_);
        decoder_ = nullptr;
    }
}

int OpenH264Decoder::Initialize(ShimErrorBuffer* error_out) {
    if (!Load()) {
        return SetErrorMessage(error_out, "OpenH264 library not loaded", SHIM_ERROR_NOT_SUPPORTED);
    }

    // Create decoder
    long ret = g_create_decoder(&decoder_);
    if (ret != 0 || !decoder_) {
        return SetErrorMessage(error_out, "WelsCreateDecoder failed", SHIM_ERROR_INIT_FAILED);
    }

    // Configure decoder
    SDecodingParam param;
    memset(&param, 0, sizeof(param));
    param.uiTargetDqLayer = 0xFF;  // All layers
    param.eEcActiveIdc = ERROR_CON_SLICE_COPY;  // Error concealment
    param.sVideoProperty.eVideoFormat = videoFormatI420;

    // Initialize
    ret = CallDecoderMethod<long, const SDecodingParam*>(decoder_, kDecoderVtable_Initialize, &param);
    if (ret != 0) {
        Release();
        return SetErrorMessage(error_out, "Decoder Initialize failed: " + std::to_string(ret), SHIM_ERROR_INIT_FAILED);
    }

    return SHIM_OK;
}

int OpenH264Decoder::Decode(
    const uint8_t* data, int size,
    uint32_t timestamp, bool is_keyframe,
    uint8_t* y_dst, uint8_t* u_dst, uint8_t* v_dst,
    int* out_width, int* out_height,
    int* out_y_stride, int* out_u_stride, int* out_v_stride,
    ShimErrorBuffer* error_out
) {
    std::lock_guard<std::mutex> lock(decode_mutex_);

    if (!decoder_) {
        return SetErrorMessage(error_out, "Decoder not initialized", SHIM_ERROR_INIT_FAILED);
    }

    if (!data || size <= 0 || !y_dst || !u_dst || !v_dst) {
        return SetErrorMessage(error_out, "Invalid decode parameters", SHIM_ERROR_INVALID_PARAM);
    }

    unsigned char* yuv_data[3] = {nullptr, nullptr, nullptr};
    SBufferInfo buf_info;
    memset(&buf_info, 0, sizeof(buf_info));

    // DecodeFrameNoDelay - lower latency than DecodeFrame
    int ret = CallDecoderMethod<int, const unsigned char*, int, unsigned char**, SBufferInfo*>(
        decoder_, kDecoderVtable_DecodeFrameNoDelay, data, size, yuv_data, &buf_info);

    if (ret != 0) {
        return SetErrorMessage(error_out, "DecodeFrameNoDelay failed: " + std::to_string(ret), SHIM_ERROR_DECODE_FAILED);
    }

    // Check if we got output
    if (buf_info.iBufferStatus != 1) {
        return SetErrorMessage(error_out, "Need more data", SHIM_ERROR_NEED_MORE_DATA);
    }

    // Extract dimensions and strides
    int width = buf_info.UsrData.sSystemBuffer.iWidth;
    int height = buf_info.UsrData.sSystemBuffer.iHeight;
    int y_stride = buf_info.UsrData.sSystemBuffer.iStride[0];
    int uv_stride = buf_info.UsrData.sSystemBuffer.iStride[1];

    int uv_width = (width + 1) / 2;
    int uv_height = (height + 1) / 2;

    // Copy Y plane (row by row to handle stride differences)
    for (int row = 0; row < height; ++row) {
        memcpy(y_dst + row * width, yuv_data[0] + row * y_stride, width);
    }

    // Copy U plane
    for (int row = 0; row < uv_height; ++row) {
        memcpy(u_dst + row * uv_width, yuv_data[1] + row * uv_stride, uv_width);
    }

    // Copy V plane
    for (int row = 0; row < uv_height; ++row) {
        memcpy(v_dst + row * uv_width, yuv_data[2] + row * uv_stride, uv_width);
    }

    // Output dimensions (tight-packed strides like libwebrtc)
    *out_width = width;
    *out_height = height;
    *out_y_stride = width;
    *out_u_stride = uv_width;
    *out_v_stride = uv_width;

    return SHIM_OK;
}

}  // namespace openh264
}  // namespace shim
