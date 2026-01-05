/*
 * libwebrtc_shim - C API wrapper for libwebrtc
 *
 * This header defines the C interface for libwebrtc's encoding, decoding,
 * and RTP packetization functionality. Designed for Go via purego (FFI without CGO).
 *
 * DESIGN: Allocation-free API
 * - Caller allocates all buffers (input and output)
 * - Shim writes directly into caller-provided buffers
 * - No memory allocation in hot paths
 */

#ifndef LIBWEBRTC_SHIM_H
#define LIBWEBRTC_SHIM_H

#include <stdint.h>
#include <stdbool.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

#ifdef _WIN32
    #define SHIM_EXPORT __declspec(dllexport)
#else
    #define SHIM_EXPORT __attribute__((visibility("default")))
#endif

/* ============================================================================
 * Error codes
 * ========================================================================== */

typedef enum {
    SHIM_OK = 0,
    SHIM_ERROR_INVALID_PARAM = -1,
    SHIM_ERROR_INIT_FAILED = -2,
    SHIM_ERROR_ENCODE_FAILED = -3,
    SHIM_ERROR_DECODE_FAILED = -4,
    SHIM_ERROR_OUT_OF_MEMORY = -5,
    SHIM_ERROR_NOT_SUPPORTED = -6,
    SHIM_ERROR_NEED_MORE_DATA = -7,
    SHIM_ERROR_BUFFER_TOO_SMALL = -8,
} ShimError;

/* ============================================================================
 * Codec types
 * ========================================================================== */

typedef enum {
    SHIM_CODEC_H264 = 0,
    SHIM_CODEC_VP8 = 1,
    SHIM_CODEC_VP9 = 2,
    SHIM_CODEC_AV1 = 3,
    SHIM_CODEC_OPUS = 10,
} ShimCodecType;

/* ============================================================================
 * Opaque handles
 * ========================================================================== */

typedef struct ShimVideoEncoder ShimVideoEncoder;
typedef struct ShimVideoDecoder ShimVideoDecoder;
typedef struct ShimAudioEncoder ShimAudioEncoder;
typedef struct ShimAudioDecoder ShimAudioDecoder;
typedef struct ShimPacketizer ShimPacketizer;
typedef struct ShimDepacketizer ShimDepacketizer;

/* ============================================================================
 * Video Encoder Configuration
 * ========================================================================== */

typedef struct {
    int32_t width;
    int32_t height;
    uint32_t bitrate_bps;
    float framerate;
    int32_t keyframe_interval;
    const char* h264_profile;   /* For H.264: profile-level-id hex string */
    int32_t vp9_profile;        /* For VP9: 0, 1, 2, or 3 */
    int32_t prefer_hw;          /* Non-zero to prefer hardware encoder */
} ShimVideoEncoderConfig;

/* ============================================================================
 * Video Encoder API (Allocation-Free)
 * ========================================================================== */

SHIM_EXPORT ShimVideoEncoder* shim_video_encoder_create(
    ShimCodecType codec,
    const ShimVideoEncoderConfig* config
);

/*
 * Encode a video frame into a pre-allocated buffer.
 *
 * @param encoder Encoder handle
 * @param y_plane Y plane data (I420 input)
 * @param u_plane U plane data
 * @param v_plane V plane data
 * @param y_stride Y plane stride
 * @param u_stride U plane stride
 * @param v_stride V plane stride
 * @param timestamp RTP timestamp (90kHz clock)
 * @param force_keyframe Force this frame to be a keyframe
 * @param dst_buffer Pre-allocated output buffer (caller provides)
 * @param out_size Output: number of bytes written to dst_buffer
 * @param out_is_keyframe Output: true if encoded frame is a keyframe
 * @return SHIM_OK on success, error code otherwise
 */
SHIM_EXPORT int shim_video_encoder_encode(
    ShimVideoEncoder* encoder,
    const uint8_t* y_plane,
    const uint8_t* u_plane,
    const uint8_t* v_plane,
    int y_stride,
    int u_stride,
    int v_stride,
    uint32_t timestamp,
    int force_keyframe,
    uint8_t* dst_buffer,        /* Caller-provided output buffer */
    int* out_size,
    int* out_is_keyframe
);

SHIM_EXPORT int shim_video_encoder_set_bitrate(ShimVideoEncoder* encoder, uint32_t bitrate_bps);
SHIM_EXPORT int shim_video_encoder_set_framerate(ShimVideoEncoder* encoder, float framerate);
SHIM_EXPORT int shim_video_encoder_request_keyframe(ShimVideoEncoder* encoder);
SHIM_EXPORT void shim_video_encoder_destroy(ShimVideoEncoder* encoder);

/* ============================================================================
 * Video Decoder API (Allocation-Free)
 * ========================================================================== */

SHIM_EXPORT ShimVideoDecoder* shim_video_decoder_create(ShimCodecType codec);

/*
 * Decode video into pre-allocated frame buffers.
 *
 * @param decoder Decoder handle
 * @param data Encoded data
 * @param size Size of encoded data
 * @param timestamp RTP timestamp
 * @param is_keyframe Hint if this is a keyframe
 * @param y_dst Pre-allocated Y plane buffer (caller provides)
 * @param u_dst Pre-allocated U plane buffer
 * @param v_dst Pre-allocated V plane buffer
 * @param out_width Output: decoded frame width
 * @param out_height Output: decoded frame height
 * @param out_y_stride Output: Y stride
 * @param out_u_stride Output: U stride
 * @param out_v_stride Output: V stride
 * @return SHIM_OK on success, SHIM_ERROR_NEED_MORE_DATA if buffering
 */
SHIM_EXPORT int shim_video_decoder_decode(
    ShimVideoDecoder* decoder,
    const uint8_t* data,
    int size,
    uint32_t timestamp,
    int is_keyframe,
    uint8_t* y_dst,             /* Caller-provided output buffers */
    uint8_t* u_dst,
    uint8_t* v_dst,
    int* out_width,
    int* out_height,
    int* out_y_stride,
    int* out_u_stride,
    int* out_v_stride
);

SHIM_EXPORT void shim_video_decoder_destroy(ShimVideoDecoder* decoder);

/* ============================================================================
 * Audio Encoder Configuration
 * ========================================================================== */

typedef struct {
    int32_t sample_rate;      /* 8000, 12000, 16000, 24000, or 48000 */
    int32_t channels;         /* 1 (mono) or 2 (stereo) */
    uint32_t bitrate_bps;
} ShimAudioEncoderConfig;

/* ============================================================================
 * Audio Encoder API (Allocation-Free)
 * ========================================================================== */

SHIM_EXPORT ShimAudioEncoder* shim_audio_encoder_create(const ShimAudioEncoderConfig* config);

/*
 * Encode audio samples into a pre-allocated buffer.
 *
 * @param encoder Encoder handle
 * @param samples PCM samples (S16LE interleaved)
 * @param num_samples Number of total samples (samples_per_channel * channels)
 * @param dst_buffer Pre-allocated output buffer
 * @param out_size Output: number of bytes written
 * @return SHIM_OK on success
 */
SHIM_EXPORT int shim_audio_encoder_encode(
    ShimAudioEncoder* encoder,
    const uint8_t* samples,     /* S16LE as bytes */
    int num_samples,
    uint8_t* dst_buffer,        /* Caller-provided output buffer */
    int* out_size
);

SHIM_EXPORT int shim_audio_encoder_set_bitrate(ShimAudioEncoder* encoder, uint32_t bitrate_bps);
SHIM_EXPORT void shim_audio_encoder_destroy(ShimAudioEncoder* encoder);

/* ============================================================================
 * Audio Decoder API (Allocation-Free)
 * ========================================================================== */

SHIM_EXPORT ShimAudioDecoder* shim_audio_decoder_create(int sample_rate, int channels);

/*
 * Decode audio into a pre-allocated buffer.
 *
 * @param decoder Decoder handle
 * @param data Encoded Opus data
 * @param size Size of encoded data
 * @param dst_samples Pre-allocated output buffer (S16LE)
 * @param out_num_samples Output: total samples decoded (samples_per_channel * channels)
 * @return SHIM_OK on success
 */
SHIM_EXPORT int shim_audio_decoder_decode(
    ShimAudioDecoder* decoder,
    const uint8_t* data,
    int size,
    uint8_t* dst_samples,       /* Caller-provided output buffer */
    int* out_num_samples
);

SHIM_EXPORT void shim_audio_decoder_destroy(ShimAudioDecoder* decoder);

/* ============================================================================
 * RTP Packetizer API (Allocation-Free)
 * ========================================================================== */

typedef struct {
    ShimCodecType codec;
    uint32_t ssrc;
    uint8_t payload_type;
    uint16_t mtu;
    uint32_t clock_rate;
} ShimPacketizerConfig;

SHIM_EXPORT ShimPacketizer* shim_packetizer_create(const ShimPacketizerConfig* config);

/*
 * Packetize encoded data into RTP packets.
 *
 * @param packetizer Packetizer handle
 * @param data Encoded frame data
 * @param size Size of encoded data
 * @param timestamp RTP timestamp
 * @param is_keyframe True if keyframe
 * @param dst_buffer Pre-allocated buffer for all packets (contiguous)
 * @param dst_offsets Pre-allocated array of offsets into dst_buffer for each packet
 * @param dst_sizes Pre-allocated array to receive packet sizes
 * @param max_packets Maximum number of packets (size of dst_offsets/dst_sizes arrays)
 * @param out_count Output: actual number of packets written
 * @return SHIM_OK on success
 */
SHIM_EXPORT int shim_packetizer_packetize(
    ShimPacketizer* packetizer,
    const uint8_t* data,
    int size,
    uint32_t timestamp,
    int is_keyframe,
    uint8_t* dst_buffer,
    int* dst_offsets,
    int* dst_sizes,
    int max_packets,
    int* out_count
);

SHIM_EXPORT uint16_t shim_packetizer_sequence_number(ShimPacketizer* packetizer);
SHIM_EXPORT void shim_packetizer_destroy(ShimPacketizer* packetizer);

/* ============================================================================
 * RTP Depacketizer API
 * ========================================================================== */

SHIM_EXPORT ShimDepacketizer* shim_depacketizer_create(ShimCodecType codec);

SHIM_EXPORT int shim_depacketizer_push(
    ShimDepacketizer* depacketizer,
    const uint8_t* data,
    int size
);

/*
 * Pop a complete frame from the depacketizer.
 *
 * @param depacketizer Depacketizer handle
 * @param dst_buffer Pre-allocated buffer for frame data
 * @param out_size Output: size of frame
 * @param out_timestamp Output: RTP timestamp
 * @param out_is_keyframe Output: true if keyframe
 * @return SHIM_OK if frame available, SHIM_ERROR_NEED_MORE_DATA if not
 */
SHIM_EXPORT int shim_depacketizer_pop(
    ShimDepacketizer* depacketizer,
    uint8_t* dst_buffer,
    int* out_size,
    uint32_t* out_timestamp,
    int* out_is_keyframe
);

SHIM_EXPORT void shim_depacketizer_destroy(ShimDepacketizer* depacketizer);

/* ============================================================================
 * PeerConnection API
 * ========================================================================== */

typedef struct ShimPeerConnection ShimPeerConnection;
typedef struct ShimRTPSender ShimRTPSender;
typedef struct ShimRTPReceiver ShimRTPReceiver;
typedef struct ShimDataChannel ShimDataChannel;

/* ICE Server configuration */
typedef struct {
    const char** urls;          /* Array of ICE server URLs */
    int url_count;
    const char* username;       /* Optional TURN username */
    const char* credential;     /* Optional TURN credential */
} ShimICEServer;

/* PeerConnection configuration */
typedef struct {
    ShimICEServer* ice_servers;
    int ice_server_count;
    int ice_candidate_pool_size;
    const char* bundle_policy;      /* "balanced", "max-compat", "max-bundle" */
    const char* rtcp_mux_policy;    /* "require", "negotiate" */
    const char* sdp_semantics;      /* "unified-plan", "plan-b" */
} ShimPeerConnectionConfig;

/* Session Description */
typedef struct {
    int type;                   /* 0=offer, 1=pranswer, 2=answer, 3=rollback */
    const char* sdp;
} ShimSessionDescription;

/* ICE Candidate */
typedef struct {
    const char* candidate;
    const char* sdp_mid;
    int sdp_mline_index;
} ShimICECandidate;

/* Callback types */
typedef void (*ShimOnICECandidate)(void* ctx, const ShimICECandidate* candidate);
typedef void (*ShimOnConnectionStateChange)(void* ctx, int state);
typedef void (*ShimOnICEConnectionStateChange)(void* ctx, int state);
typedef void (*ShimOnICEGatheringStateChange)(void* ctx, int state);
typedef void (*ShimOnSignalingStateChange)(void* ctx, int state);
typedef void (*ShimOnTrack)(void* ctx, void* track, void* receiver, const char* stream_id);
typedef void (*ShimOnDataChannel)(void* ctx, void* channel);

/* Create/Destroy PeerConnection */
SHIM_EXPORT ShimPeerConnection* shim_peer_connection_create(
    const ShimPeerConnectionConfig* config
);
SHIM_EXPORT void shim_peer_connection_destroy(ShimPeerConnection* pc);

/* Set callbacks */
SHIM_EXPORT void shim_peer_connection_set_on_ice_candidate(
    ShimPeerConnection* pc,
    ShimOnICECandidate callback,
    void* ctx
);
SHIM_EXPORT void shim_peer_connection_set_on_connection_state_change(
    ShimPeerConnection* pc,
    ShimOnConnectionStateChange callback,
    void* ctx
);
SHIM_EXPORT void shim_peer_connection_set_on_track(
    ShimPeerConnection* pc,
    ShimOnTrack callback,
    void* ctx
);
SHIM_EXPORT void shim_peer_connection_set_on_data_channel(
    ShimPeerConnection* pc,
    ShimOnDataChannel callback,
    void* ctx
);

/* Create offer/answer */
SHIM_EXPORT int shim_peer_connection_create_offer(
    ShimPeerConnection* pc,
    char* sdp_out,              /* Caller-provided buffer for SDP */
    int sdp_out_size,
    int* out_sdp_len
);
SHIM_EXPORT int shim_peer_connection_create_answer(
    ShimPeerConnection* pc,
    char* sdp_out,
    int sdp_out_size,
    int* out_sdp_len
);

/* Set local/remote description */
SHIM_EXPORT int shim_peer_connection_set_local_description(
    ShimPeerConnection* pc,
    int type,                   /* SDP type */
    const char* sdp
);
SHIM_EXPORT int shim_peer_connection_set_remote_description(
    ShimPeerConnection* pc,
    int type,
    const char* sdp
);

/* Add ICE candidate */
SHIM_EXPORT int shim_peer_connection_add_ice_candidate(
    ShimPeerConnection* pc,
    const char* candidate,
    const char* sdp_mid,
    int sdp_mline_index
);

/* Get connection states */
SHIM_EXPORT int shim_peer_connection_signaling_state(ShimPeerConnection* pc);
SHIM_EXPORT int shim_peer_connection_ice_connection_state(ShimPeerConnection* pc);
SHIM_EXPORT int shim_peer_connection_ice_gathering_state(ShimPeerConnection* pc);
SHIM_EXPORT int shim_peer_connection_connection_state(ShimPeerConnection* pc);

/* Add track */
SHIM_EXPORT ShimRTPSender* shim_peer_connection_add_track(
    ShimPeerConnection* pc,
    ShimCodecType codec,
    const char* track_id,
    const char* stream_id
);

/* Remove track */
SHIM_EXPORT int shim_peer_connection_remove_track(
    ShimPeerConnection* pc,
    ShimRTPSender* sender
);

/* Create data channel */
SHIM_EXPORT ShimDataChannel* shim_peer_connection_create_data_channel(
    ShimPeerConnection* pc,
    const char* label,
    int ordered,
    int max_retransmits,
    const char* protocol
);

/* Close peer connection */
SHIM_EXPORT void shim_peer_connection_close(ShimPeerConnection* pc);

/* ============================================================================
 * RTPSender API
 * ========================================================================== */

SHIM_EXPORT int shim_rtp_sender_set_bitrate(ShimRTPSender* sender, uint32_t bitrate);
SHIM_EXPORT int shim_rtp_sender_replace_track(ShimRTPSender* sender, void* track);
SHIM_EXPORT void shim_rtp_sender_destroy(ShimRTPSender* sender);

/* ============================================================================
 * DataChannel API
 * ========================================================================== */

typedef void (*ShimOnDataChannelMessage)(void* ctx, const uint8_t* data, int size, int is_binary);
typedef void (*ShimOnDataChannelOpen)(void* ctx);
typedef void (*ShimOnDataChannelClose)(void* ctx);

SHIM_EXPORT void shim_data_channel_set_on_message(
    ShimDataChannel* dc,
    ShimOnDataChannelMessage callback,
    void* ctx
);
SHIM_EXPORT void shim_data_channel_set_on_open(
    ShimDataChannel* dc,
    ShimOnDataChannelOpen callback,
    void* ctx
);
SHIM_EXPORT void shim_data_channel_set_on_close(
    ShimDataChannel* dc,
    ShimOnDataChannelClose callback,
    void* ctx
);

SHIM_EXPORT int shim_data_channel_send(
    ShimDataChannel* dc,
    const uint8_t* data,
    int size,
    int is_binary
);
SHIM_EXPORT const char* shim_data_channel_label(ShimDataChannel* dc);
SHIM_EXPORT int shim_data_channel_ready_state(ShimDataChannel* dc);
SHIM_EXPORT void shim_data_channel_close(ShimDataChannel* dc);
SHIM_EXPORT void shim_data_channel_destroy(ShimDataChannel* dc);

/* ============================================================================
 * Memory helpers
 * ========================================================================== */

SHIM_EXPORT void shim_free_buffer(void* buffer);
SHIM_EXPORT void shim_free_packets(void* packets, void* sizes, int count);

/* ============================================================================
 * Version Information
 * ========================================================================== */

SHIM_EXPORT const char* shim_libwebrtc_version(void);
SHIM_EXPORT const char* shim_version(void);

#ifdef __cplusplus
}
#endif

#endif /* LIBWEBRTC_SHIM_H */
