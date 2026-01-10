/*
 * openh264_types.h - OpenH264 type definitions
 *
 * These types are copied from OpenH264's codec_api.h and codec_app_def.h
 * to avoid requiring OpenH264 headers at compile time.
 * The library is loaded dynamically at runtime.
 *
 * OpenH264 version: 2.x (API stable)
 * Source: https://github.com/cisco/openh264/blob/master/codec/api/wels/
 */

#ifndef OPENH264_TYPES_H_
#define OPENH264_TYPES_H_

#include <stdint.h>
#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

// ============================================================================
// Enums from codec_app_def.h
// ============================================================================

typedef enum {
    CAMERA_VIDEO_REAL_TIME,
    SCREEN_CONTENT_REAL_TIME,
    CAMERA_VIDEO_NON_REAL_TIME,
    SCREEN_CONTENT_NON_REAL_TIME,
    INPUT_CONTENT_TYPE_ALL
} EUsageType;

typedef enum {
    RC_QUALITY_MODE = 0,
    RC_BITRATE_MODE = 1,
    RC_BUFFERBASED_MODE = 2,
    RC_TIMESTAMP_MODE = 3,
    RC_BITRATE_MODE_POST_SKIP = 4,
    RC_OFF_MODE = -1
} RC_MODES;

typedef enum {
    videoFormatRGB = 1,
    videoFormatRGBA = 2,
    videoFormatRGB555 = 3,
    videoFormatRGB565 = 4,
    videoFormatBGR = 5,
    videoFormatBGRA = 6,
    videoFormatABGR = 7,
    videoFormatARGB = 8,
    videoFormatYUY2 = 20,
    videoFormatYVYU = 21,
    videoFormatUYVY = 22,
    videoFormatI420 = 23,
    videoFormatYV12 = 24,
    videoFormatInternal = 25,
    videoFormatNV12 = 26,
    videoFormatVFlip = 0x80000000
} EVideoFormatType;

typedef enum {
    videoFrameTypeInvalid,
    videoFrameTypeIDR,
    videoFrameTypeI,
    videoFrameTypeP,
    videoFrameTypeSkip,
    videoFrameTypeIPMixed
} EVideoFrameType;

typedef enum {
    cmResultSuccess,
    cmInitParaError,
    cmUnknownReason,
    cmMallocMemeError,
    cmInitExpected,
    cmUnsupportedData
} CM_RETURN;

typedef enum {
    dsErrorFree = 0x00,
    dsFramePending = 0x01,
    dsRefLost = 0x02,
    dsBitstreamError = 0x04,
    dsDepLayerLost = 0x08,
    dsNoParamSets = 0x10,
    dsDataErrorConcealed = 0x20,
    dsRefListNullPtrs = 0x40
} DECODING_STATE;

typedef enum {
    ERROR_CON_DISABLE = 0,
    ERROR_CON_FRAME_COPY,
    ERROR_CON_SLICE_COPY,
    ERROR_CON_FRAME_COPY_CROSS_IDR,
    ERROR_CON_SLICE_COPY_CROSS_IDR,
    ERROR_CON_SLICE_COPY_CROSS_IDR_FREEZE_RES_CHANGE,
    ERROR_CON_SLICE_MV_COPY_CROSS_IDR,
    ERROR_CON_SLICE_MV_COPY_CROSS_IDR_FREEZE_RES_CHANGE
} ERROR_CON_IDC;

typedef enum {
    VIDEO_BITSTREAM_AVC = 0,
    VIDEO_BITSTREAM_SVC = 1,
    VIDEO_BITSTREAM_DEFAULT = VIDEO_BITSTREAM_SVC
} VIDEO_BITSTREAM_TYPE;

typedef enum {
    SPATIAL_LAYER_0 = 0,
    SPATIAL_LAYER_1 = 1,
    SPATIAL_LAYER_2 = 2,
    SPATIAL_LAYER_3 = 3,
    SPATIAL_LAYER_ALL = 4
} LAYER_NUM;

typedef enum {
    PRO_UNKNOWN = 0,
    PRO_BASELINE = 66,
    PRO_MAIN = 77,
    PRO_EXTENDED = 88,
    PRO_HIGH = 100,
    PRO_HIGH10 = 110,
    PRO_HIGH422 = 122,
    PRO_HIGH444 = 244,
    PRO_CAVLC444 = 44,
    PRO_SCALABLE_BASELINE = 83,
    PRO_SCALABLE_HIGH = 86
} EProfileIdc;

typedef enum {
    LEVEL_UNKNOWN = 0,
    LEVEL_1_0 = 10,
    LEVEL_1_B = 9,
    LEVEL_1_1 = 11,
    LEVEL_1_2 = 12,
    LEVEL_1_3 = 13,
    LEVEL_2_0 = 20,
    LEVEL_2_1 = 21,
    LEVEL_2_2 = 22,
    LEVEL_3_0 = 30,
    LEVEL_3_1 = 31,
    LEVEL_3_2 = 32,
    LEVEL_4_0 = 40,
    LEVEL_4_1 = 41,
    LEVEL_4_2 = 42,
    LEVEL_5_0 = 50,
    LEVEL_5_1 = 51,
    LEVEL_5_2 = 52
} ELevelIdc;

typedef enum {
    WELS_LOG_QUIET = 0x00,
    WELS_LOG_ERROR = 1 << 0,
    WELS_LOG_WARNING = 1 << 1,
    WELS_LOG_INFO = 1 << 2,
    WELS_LOG_DEBUG = 1 << 3,
    WELS_LOG_DETAIL = 1 << 4,
    WELS_LOG_RESV = 1 << 5,
    WELS_LOG_LEVEL_COUNT = 6,
    WELS_LOG_DEFAULT = WELS_LOG_WARNING
} WELS_LOG;

typedef enum {
    ENCODER_OPTION_DATAFORMAT = 0,
    ENCODER_OPTION_IDR_INTERVAL,
    ENCODER_OPTION_SVC_ENCODE_PARAM_BASE,
    ENCODER_OPTION_SVC_ENCODE_PARAM_EXT,
    ENCODER_OPTION_FRAME_RATE,
    ENCODER_OPTION_BITRATE,
    ENCODER_OPTION_MAX_BITRATE,
    ENCODER_OPTION_INTER_SPATIAL_PRED,
    ENCODER_OPTION_RC_MODE,
    ENCODER_OPTION_RC_FRAME_SKIP,
    ENCODER_PADDING_PADDING,
    ENCODER_OPTION_PROFILE,
    ENCODER_OPTION_LEVEL,
    ENCODER_OPTION_NUMBER_REF,
    ENCODER_OPTION_DELIVERY_STATUS,
    ENCODER_LTR_RECOVERY_REQUEST,
    ENCODER_LTR_MARKING_FEEDBACK,
    ENCODER_LTR_MARKING_PERIOD,
    ENCODER_OPTION_LTR,
    ENCODER_OPTION_COMPLEXITY,
    ENCODER_OPTION_ENABLE_SSEI,
    ENCODER_OPTION_ENABLE_PREFIX_NAL_ADDING,
    ENCODER_OPTION_SPS_PPS_ID_STRATEGY,
    ENCODER_OPTION_CURRENT_PATH,
    ENCODER_OPTION_DUMP_FILE,
    ENCODER_OPTION_TRACE_LEVEL,
    ENCODER_OPTION_TRACE_CALLBACK,
    ENCODER_OPTION_TRACE_CALLBACK_CONTEXT,
    ENCODER_OPTION_GET_STATISTICS,
    ENCODER_OPTION_STATISTICS_LOG_INTERVAL,
    ENCODER_OPTION_IS_LOSSLESS_LINK,
    ENCODER_OPTION_BITS_VARY_PERCENTAGE
} ENCODER_OPTION;

typedef enum {
    DECODER_OPTION_END_OF_STREAM = 1,
    DECODER_OPTION_VCL_NAL,
    DECODER_OPTION_TEMPORAL_ID,
    DECODER_OPTION_FRAME_NUM,
    DECODER_OPTION_IDR_PIC_ID,
    DECODER_OPTION_LTR_MARKING_FLAG,
    DECODER_OPTION_LTR_MARKED_FRAME_NUM,
    DECODER_OPTION_ERROR_CON_IDC,
    DECODER_OPTION_TRACE_LEVEL,
    DECODER_OPTION_TRACE_CALLBACK,
    DECODER_OPTION_TRACE_CALLBACK_CONTEXT,
    DECODER_OPTION_GET_STATISTICS,
    DECODER_OPTION_GET_SAR_INFO,
    DECODER_OPTION_PROFILE,
    DECODER_OPTION_LEVEL,
    DECODER_OPTION_STATISTICS_LOG_INTERVAL,
    DECODER_OPTION_IS_REF_PIC,
    DECODER_OPTION_NUM_OF_FRAMES_REMAINING_IN_BUFFER,
    DECODER_OPTION_NUM_OF_THREADS
} DECODER_OPTION;

// ============================================================================
// Structs from codec_app_def.h
// ============================================================================

typedef struct {
    int iColorFormat;
    int iStride[4];
    unsigned char* pData[4];
    int iPicWidth;
    int iPicHeight;
    long long uiTimeStamp;
} SSourcePicture;

typedef struct {
    unsigned char uiTemporalId;
    unsigned char uiSpatialId;
    unsigned char uiQualityId;
    unsigned char uiLayerType;
    int iNalCount;
    int* pNalLengthInByte;
    unsigned char* pBsBuf;
    int iSubSeqId;
    EVideoFrameType eFrameType;
} SLayerBSInfo;

#define MAX_LAYER_NUM_OF_FRAME 128

typedef struct {
    int iLayerNum;
    SLayerBSInfo sLayerInfo[MAX_LAYER_NUM_OF_FRAME];
    EVideoFrameType eFrameType;
    int iFrameSizeInBytes;
    long long uiTimeStamp;
} SFrameBSInfo;

typedef struct {
    int iVideoWidth;
    int iVideoHeight;
    float fFrameRate;
    int iSpatialBitrate;
    int iMaxSpatialBitrate;
    EProfileIdc uiProfileIdc;
    ELevelIdc uiLevelIdc;
    int iDLayerQp;
    // Slice config
    unsigned int uiSliceMode;
    unsigned int uiSliceNum;
    unsigned int uiSliceMbNum[35];
    unsigned int uiSliceSizeConstraint;
    // Aspect ratio
    bool bAspectRatioPresent;
    unsigned char uiAspectRatioIdc;
    unsigned short uiAspectSarWidth;
    unsigned short uiAspectSarHeight;
} SSpatialLayerConfig;

typedef struct {
    int iTemporalId;
    int iQualityId;
    int iFrameRate;
    int iBitrate;
} STemporalLayerConfig;

#define MAX_TEMPORAL_LAYER_NUM 4
#define MAX_SPATIAL_LAYER_NUM 4

typedef struct {
    EUsageType iUsageType;
    int iPicWidth;
    int iPicHeight;
    int iTargetBitrate;
    RC_MODES iRCMode;
    float fMaxFrameRate;

    int iTemporalLayerNum;
    int iSpatialLayerNum;
    SSpatialLayerConfig sSpatialLayers[MAX_SPATIAL_LAYER_NUM];

    int iComplexityMode;
    unsigned int uiIntraPeriod;
    int iNumRefFrame;
    unsigned char uiSpsPpsIdStrategy;
    bool bPrefixNalAddingCtrl;
    bool bEnableSSEI;
    bool bSimulcastAVC;
    int iPaddingFlag;
    int iEntropyCodingModeFlag;

    bool bEnableFrameSkip;
    int iMaxBitrate;
    int iMaxQp;
    int iMinQp;
    unsigned int uiMaxNalSize;

    bool bEnableLongTermReference;
    int iLTRRefNum;
    unsigned int iLtrMarkPeriod;

    unsigned short iMultipleThreadIdc;
    bool bUseLoadBalancing;

    int iLoopFilterDisableIdc;
    int iLoopFilterAlphaC0Offset;
    int iLoopFilterBetaOffset;

    bool bEnableDenoise;
    bool bEnableBackgroundDetection;
    bool bEnableAdaptiveQuant;
    bool bEnableFrameCroppingFlag;
    bool bEnableSceneChangeDetect;

    bool bIsLosslessLink;
    bool bFixRCOverShoot;
    int iIdrBitrateRatio;
} SEncParamExt;

typedef struct {
    int iSpatialLayerNum;
    int iTemporalLayerNum;
    unsigned int uiGopSize;
    unsigned int uiTargetFrameRate;
    unsigned int uiFrameNum;
    int iAvgLumaQp;
    long long iTotalByte;
    long long iTotalEncode;
} SEncoderStatistics;

typedef struct {
    int iVideoWidth;
    int iVideoHeight;
    float fFrameRate;
    int iBitrate;
} SBitrateInfo;

// ============================================================================
// Decoder structs
// ============================================================================

typedef struct {
    EVideoFormatType eVideoFormat;
    int iColorMatrix;
    int iColorPrimaries;
    int iTransferCharacteristics;
    int iColorRange;
    int iChromaSampleLocTypeTopField;
    int iChromaSampleLocTypeBottomField;
} SVideoProperty;

typedef struct {
    char* pFileNameRestructed;
    unsigned int uiCpuLoad;
    unsigned char uiTargetDqLayer;
    ERROR_CON_IDC eEcActiveIdc;
    bool bParseOnly;
    SVideoProperty sVideoProperty;
} SDecodingParam;

typedef struct {
    int iWidth;
    int iHeight;
    int iStride[2];
} SSysMEMBuffer;

typedef struct {
    union {
        SSysMEMBuffer sSystemBuffer;
    } UsrData;
    int iBufferStatus;
    long long uiInBsTimeStamp;
    long long uiOutYuvTimeStamp;
} SBufferInfo;

typedef struct {
    unsigned int uiWidth;
    unsigned int uiHeight;
    unsigned int uiProfile;
    unsigned int uiLevel;
    unsigned long long uiTotalByte;
    unsigned long long uiTotalDecFrames;
    unsigned long long uiTotalError;
    unsigned long long uiTotalIDR;
} SDecoderStatistics;

// ============================================================================
// ISVCEncoder and ISVCDecoder interfaces
//
// These are C++ abstract classes with virtual methods. We use them through
// vtable pointers. The vtable layout must match OpenH264's implementation.
// ============================================================================

// ISVCEncoder vtable indices (0-based):
// 0: Initialize(const SEncParamBase*)
// 1: InitializeExt(const SEncParamExt*)
// 2: GetDefaultParams(SEncParamExt*)
// 3: Uninitialize()
// 4: EncodeFrame(const SSourcePicture*, SFrameBSInfo*)
// 5: EncodeParameterSets(SFrameBSInfo*)
// 6: ForceIntraFrame(bool, int)
// 7: SetOption(ENCODER_OPTION, void*)
// 8: GetOption(ENCODER_OPTION, void*)

// ISVCDecoder vtable indices (0-based):
// 0: Initialize(const SDecodingParam*)
// 1: Uninitialize()
// 2: DecodeFrame(const unsigned char*, int, unsigned char**, SBufferInfo*)
// 3: DecodeFrameNoDelay(const unsigned char*, int, unsigned char**, SBufferInfo*)
// 4: DecodeFrame2(const unsigned char*, int, unsigned char**, SBufferInfo*)
// 5: FlushFrame(unsigned char**, SBufferInfo*)
// 6: DecodeParser(const unsigned char*, int, SParserBsInfo*)
// 7: DecodeFrameEx(const unsigned char*, int, unsigned char*, int, unsigned char*, int, unsigned char*, int, int*, int*)
// 8: SetOption(DECODER_OPTION, void*)
// 9: GetOption(DECODER_OPTION, void*)

#ifdef __cplusplus
}
#endif

#endif  // OPENH264_TYPES_H_
