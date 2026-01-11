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
    SHIM_ERROR_NOT_FOUND = -9,
    SHIM_ERROR_RENEGOTIATION_NEEDED = -10,
} ShimError;

/*
 * Error Message Buffer:
 * Functions that can fail with detailed errors take an optional ShimErrorBuffer* parameter.
 * - If error_out is NULL, no message is written
 * - Messages are null-terminated and truncated if necessary
 */
#define SHIM_MAX_ERROR_MSG_LEN 512

typedef struct {
    char message[SHIM_MAX_ERROR_MSG_LEN];
} ShimErrorBuffer;

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

typedef struct {
    ShimCodecType codec;
    const ShimVideoEncoderConfig* config;
    ShimErrorBuffer* error_out;  /* Optional: buffer for error message */
} ShimVideoEncoderCreateParams;

SHIM_EXPORT ShimVideoEncoder* shim_video_encoder_create(
    ShimVideoEncoderCreateParams* params
);

/*
 * Encode a video frame into a pre-allocated buffer.
 *
 * @param encoder Encoder handle
 * @param params Encode parameters (inputs + outputs)
 * @return SHIM_OK on success, SHIM_ERROR_BUFFER_TOO_SMALL if buffer insufficient
 */
/* Encode parameters. Caller-owned buffers; shim uses them only during the call. */
typedef struct {
    const uint8_t* y_plane;
    const uint8_t* u_plane;
    const uint8_t* v_plane;
    int y_stride;
    int u_stride;
    int v_stride;
    uint32_t timestamp;
    int force_keyframe;
    uint8_t* dst_buffer;
    int dst_buffer_size;
    int out_size;
    int out_is_keyframe;
    ShimErrorBuffer* error_out;
} ShimVideoEncoderEncodeParams;

SHIM_EXPORT int shim_video_encoder_encode(
    ShimVideoEncoder* encoder,
    ShimVideoEncoderEncodeParams* params
);

typedef struct {
    ShimVideoEncoder* encoder;
    uint32_t bitrate_bps;
    ShimErrorBuffer* error_out;  /* Optional: buffer for error message */
} ShimVideoEncoderSetBitrateParams;

SHIM_EXPORT int shim_video_encoder_set_bitrate(
    ShimVideoEncoderSetBitrateParams* params
);

typedef struct {
    ShimVideoEncoder* encoder;
    float framerate;
    ShimErrorBuffer* error_out;  /* Optional: buffer for error message */
} ShimVideoEncoderSetFramerateParams;

SHIM_EXPORT int shim_video_encoder_set_framerate(
    ShimVideoEncoderSetFramerateParams* params
);
SHIM_EXPORT int shim_video_encoder_request_keyframe(ShimVideoEncoder* encoder);
SHIM_EXPORT void shim_video_encoder_destroy(ShimVideoEncoder* encoder);

/* ============================================================================
 * Video Decoder API (Allocation-Free)
 * ========================================================================== */

typedef struct {
    ShimCodecType codec;
    ShimErrorBuffer* error_out;  /* Optional: buffer for error message */
} ShimVideoDecoderCreateParams;

SHIM_EXPORT ShimVideoDecoder* shim_video_decoder_create(
    ShimVideoDecoderCreateParams* params
);

/*
 * Decode video into pre-allocated frame buffers.
 *
 * @param decoder Decoder handle
 * @param params Decode parameters (inputs + outputs)
 * @return SHIM_OK on success, SHIM_ERROR_NEED_MORE_DATA if buffering
 */
/* Decode parameters. Caller-owned buffers; shim uses them only during the call. */
typedef struct {
    const uint8_t* data;
    int size;
    uint32_t timestamp;
    int is_keyframe;
    uint8_t* y_dst;
    uint8_t* u_dst;
    uint8_t* v_dst;
    int out_width;
    int out_height;
    int out_y_stride;
    int out_u_stride;
    int out_v_stride;
    ShimErrorBuffer* error_out;
} ShimVideoDecoderDecodeParams;

SHIM_EXPORT int shim_video_decoder_decode(
    ShimVideoDecoder* decoder,
    ShimVideoDecoderDecodeParams* params
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

typedef struct {
    const ShimAudioEncoderConfig* config;
    ShimErrorBuffer* error_out;  /* Optional: buffer for error message */
} ShimAudioEncoderCreateParams;

SHIM_EXPORT ShimAudioEncoder* shim_audio_encoder_create(
    ShimAudioEncoderCreateParams* params
);

/*
 * Encode audio samples into a pre-allocated buffer.
 *
 * @param encoder Encoder handle
 * @param params Encode parameters (inputs + outputs)
 * @return SHIM_OK on success
 */
/* Encode parameters. Caller-owned buffers; shim uses them only during the call. */
typedef struct {
    const uint8_t* samples;
    int num_samples;
    uint8_t* dst_buffer;
    int out_size;
} ShimAudioEncoderEncodeParams;

SHIM_EXPORT int shim_audio_encoder_encode(
    ShimAudioEncoder* encoder,
    ShimAudioEncoderEncodeParams* params
);

typedef struct {
    ShimAudioEncoder* encoder;
    uint32_t bitrate_bps;
    ShimErrorBuffer* error_out;  /* Optional: buffer for error message */
} ShimAudioEncoderSetBitrateParams;

SHIM_EXPORT int shim_audio_encoder_set_bitrate(
    ShimAudioEncoderSetBitrateParams* params
);
SHIM_EXPORT void shim_audio_encoder_destroy(ShimAudioEncoder* encoder);

/* ============================================================================
 * Audio Decoder API (Allocation-Free)
 * ========================================================================== */

typedef struct {
    int sample_rate;
    int channels;
    ShimErrorBuffer* error_out;  /* Optional: buffer for error message */
} ShimAudioDecoderCreateParams;

SHIM_EXPORT ShimAudioDecoder* shim_audio_decoder_create(
    ShimAudioDecoderCreateParams* params
);

/*
 * Decode audio into a pre-allocated buffer.
 *
 * @param decoder Decoder handle
 * @param params Decode parameters (inputs + outputs)
 * @return SHIM_OK on success
 */
/* Decode parameters. Caller-owned buffers; shim uses them only during the call. */
typedef struct {
    const uint8_t* data;
    int size;
    uint8_t* dst_samples;
    int out_num_samples;
    ShimErrorBuffer* error_out;
} ShimAudioDecoderDecodeParams;

SHIM_EXPORT int shim_audio_decoder_decode(
    ShimAudioDecoder* decoder,
    ShimAudioDecoderDecodeParams* params
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
 * @param params Packetize parameters (inputs + outputs)
 * @return SHIM_OK on success
 */
/* Packetize parameters. Caller-owned buffers; shim uses them only during the call. */
typedef struct {
    ShimPacketizer* packetizer;
    const uint8_t* data;
    int size;
    uint32_t timestamp;
    int is_keyframe;
    uint8_t* dst_buffer;
    int* dst_offsets;
    int* dst_sizes;
    int max_packets;
    int out_count;
} ShimPacketizerPacketizeParams;

SHIM_EXPORT int shim_packetizer_packetize(
    ShimPacketizerPacketizeParams* params
);

SHIM_EXPORT uint16_t shim_packetizer_sequence_number(ShimPacketizer* packetizer);
SHIM_EXPORT void shim_packetizer_destroy(ShimPacketizer* packetizer);

/* ============================================================================
 * RTP Depacketizer API
 * ========================================================================== */

SHIM_EXPORT ShimDepacketizer* shim_depacketizer_create(ShimCodecType codec);

/* Push parameters. Caller-owned buffers; shim uses them only during the call. */
typedef struct {
    ShimDepacketizer* depacketizer;
    const uint8_t* data;
    int size;
} ShimDepacketizerPushParams;

SHIM_EXPORT int shim_depacketizer_push(
    ShimDepacketizerPushParams* params
);

/*
 * Pop a complete frame from the depacketizer.
 *
 * @param params Pop parameters (inputs + outputs)
 * @return SHIM_OK if frame available, SHIM_ERROR_NEED_MORE_DATA if not
 */
/* Pop parameters. Caller-owned buffers; shim uses them only during the call. */
typedef struct {
    ShimDepacketizer* depacketizer;
    uint8_t* dst_buffer;
    int out_size;
    uint32_t out_timestamp;
    int out_is_keyframe;
} ShimDepacketizerPopParams;

SHIM_EXPORT int shim_depacketizer_pop(
    ShimDepacketizerPopParams* params
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
typedef struct {
    const ShimPeerConnectionConfig* config;
    ShimErrorBuffer* error_out;  /* Optional: buffer for error message */
} ShimPeerConnectionCreateParams;

SHIM_EXPORT ShimPeerConnection* shim_peer_connection_create(
    ShimPeerConnectionCreateParams* params
);
SHIM_EXPORT void shim_peer_connection_destroy(ShimPeerConnection* pc);

/* Set callbacks */
typedef struct {
    ShimPeerConnection* pc;
    ShimOnICECandidate callback;
    void* ctx;
} ShimPeerConnectionSetOnICECandidateParams;

SHIM_EXPORT void shim_peer_connection_set_on_ice_candidate(
    ShimPeerConnectionSetOnICECandidateParams* params
);

typedef struct {
    ShimPeerConnection* pc;
    ShimOnConnectionStateChange callback;
    void* ctx;
} ShimPeerConnectionSetOnConnectionStateChangeParams;

SHIM_EXPORT void shim_peer_connection_set_on_connection_state_change(
    ShimPeerConnectionSetOnConnectionStateChangeParams* params
);

typedef struct {
    ShimPeerConnection* pc;
    ShimOnTrack callback;
    void* ctx;
} ShimPeerConnectionSetOnTrackParams;

SHIM_EXPORT void shim_peer_connection_set_on_track(
    ShimPeerConnectionSetOnTrackParams* params
);

typedef struct {
    ShimPeerConnection* pc;
    ShimOnDataChannel callback;
    void* ctx;
} ShimPeerConnectionSetOnDataChannelParams;

SHIM_EXPORT void shim_peer_connection_set_on_data_channel(
    ShimPeerConnectionSetOnDataChannelParams* params
);

typedef struct {
    ShimPeerConnection* pc;
    ShimOnSignalingStateChange callback;
    void* ctx;
} ShimPeerConnectionSetOnSignalingStateChangeParams;

SHIM_EXPORT void shim_peer_connection_set_on_signaling_state_change(
    ShimPeerConnectionSetOnSignalingStateChangeParams* params
);

typedef struct {
    ShimPeerConnection* pc;
    ShimOnICEConnectionStateChange callback;
    void* ctx;
} ShimPeerConnectionSetOnICEConnectionStateChangeParams;

SHIM_EXPORT void shim_peer_connection_set_on_ice_connection_state_change(
    ShimPeerConnectionSetOnICEConnectionStateChangeParams* params
);

typedef struct {
    ShimPeerConnection* pc;
    ShimOnICEGatheringStateChange callback;
    void* ctx;
} ShimPeerConnectionSetOnICEGatheringStateChangeParams;

SHIM_EXPORT void shim_peer_connection_set_on_ice_gathering_state_change(
    ShimPeerConnectionSetOnICEGatheringStateChangeParams* params
);

/* Negotiation needed callback */
typedef void (*ShimOnNegotiationNeeded)(void* ctx);

typedef struct {
    ShimPeerConnection* pc;
    ShimOnNegotiationNeeded callback;
    void* ctx;
} ShimPeerConnectionSetOnNegotiationNeededParams;

SHIM_EXPORT void shim_peer_connection_set_on_negotiation_needed(
    ShimPeerConnectionSetOnNegotiationNeededParams* params
);

/* Create offer/answer */
typedef struct {
    ShimPeerConnection* pc;
    char* sdp_out;              /* Caller-provided buffer for SDP */
    int sdp_out_size;
    int out_sdp_len;
    ShimErrorBuffer* error_out; /* Optional: buffer for error message */
} ShimPeerConnectionCreateOfferParams;

SHIM_EXPORT int shim_peer_connection_create_offer(
    ShimPeerConnectionCreateOfferParams* params
);

typedef struct {
    ShimPeerConnection* pc;
    char* sdp_out;
    int sdp_out_size;
    int out_sdp_len;
    ShimErrorBuffer* error_out; /* Optional: buffer for error message */
} ShimPeerConnectionCreateAnswerParams;

SHIM_EXPORT int shim_peer_connection_create_answer(
    ShimPeerConnectionCreateAnswerParams* params
);

/* Set local/remote description */
typedef struct {
    ShimPeerConnection* pc;
    int type;                   /* SDP type */
    const char* sdp;
    ShimErrorBuffer* error_out; /* Optional: buffer for error message */
} ShimPeerConnectionSetLocalDescriptionParams;

SHIM_EXPORT int shim_peer_connection_set_local_description(
    ShimPeerConnectionSetLocalDescriptionParams* params
);

typedef struct {
    ShimPeerConnection* pc;
    int type;
    const char* sdp;
    ShimErrorBuffer* error_out; /* Optional: buffer for error message */
} ShimPeerConnectionSetRemoteDescriptionParams;

SHIM_EXPORT int shim_peer_connection_set_remote_description(
    ShimPeerConnectionSetRemoteDescriptionParams* params
);

/* Add ICE candidate */
typedef struct {
    ShimPeerConnection* pc;
    const char* candidate;
    const char* sdp_mid;
    int sdp_mline_index;
    ShimErrorBuffer* error_out; /* Optional: buffer for error message */
} ShimPeerConnectionAddICECandidateParams;

SHIM_EXPORT int shim_peer_connection_add_ice_candidate(
    ShimPeerConnectionAddICECandidateParams* params
);

/* Get connection states */
SHIM_EXPORT int shim_peer_connection_signaling_state(ShimPeerConnection* pc);
SHIM_EXPORT int shim_peer_connection_ice_connection_state(ShimPeerConnection* pc);
SHIM_EXPORT int shim_peer_connection_ice_gathering_state(ShimPeerConnection* pc);
SHIM_EXPORT int shim_peer_connection_connection_state(ShimPeerConnection* pc);

/* Add track */
typedef struct {
    ShimPeerConnection* pc;
    ShimCodecType codec;
    const char* track_id;
    const char* stream_id;
    ShimErrorBuffer* error_out; /* Optional: buffer for error message */
} ShimPeerConnectionAddTrackParams;

SHIM_EXPORT ShimRTPSender* shim_peer_connection_add_track(
    ShimPeerConnectionAddTrackParams* params
);

/* Remove track */
typedef struct {
    ShimPeerConnection* pc;
    ShimRTPSender* sender;
    ShimErrorBuffer* error_out; /* Optional: buffer for error message */
} ShimPeerConnectionRemoveTrackParams;

SHIM_EXPORT int shim_peer_connection_remove_track(
    ShimPeerConnectionRemoveTrackParams* params
);

/* Create data channel */
typedef struct {
    ShimPeerConnection* pc;
    const char* label;
    int ordered;
    int max_retransmits;
    const char* protocol;
    ShimErrorBuffer* error_out; /* Optional: buffer for error message */
} ShimPeerConnectionCreateDataChannelParams;

SHIM_EXPORT ShimDataChannel* shim_peer_connection_create_data_channel(
    ShimPeerConnectionCreateDataChannelParams* params
);

/* Close peer connection */
SHIM_EXPORT void shim_peer_connection_close(ShimPeerConnection* pc);

/* ============================================================================
 * RTPSender API
 * ========================================================================== */

typedef struct {
    ShimRTPSender* sender;
    uint32_t bitrate;
    ShimErrorBuffer* error_out; /* Optional: buffer for error message */
} ShimRTPSenderSetBitrateParams;

SHIM_EXPORT int shim_rtp_sender_set_bitrate(
    ShimRTPSenderSetBitrateParams* params
);

typedef struct {
    ShimRTPSender* sender;
    void* track;
} ShimRTPSenderReplaceTrackParams;

SHIM_EXPORT int shim_rtp_sender_replace_track(ShimRTPSenderReplaceTrackParams* params);
SHIM_EXPORT void shim_rtp_sender_destroy(ShimRTPSender* sender);

/* ============================================================================
 * RTPSender Parameters API (SetParameters/GetParameters)
 * ========================================================================== */

typedef struct {
    char rid[64];                   /* RID for simulcast */
    uint32_t max_bitrate_bps;
    uint32_t min_bitrate_bps;
    double max_framerate;
    double scale_resolution_down_by;
    int active;                     /* 0=inactive, 1=active */
    char scalability_mode[32];      /* e.g., "L3T3_KEY" */
} ShimRTPEncodingParameters;

typedef struct {
    ShimRTPEncodingParameters* encodings;
    int encoding_count;
    char transaction_id[64];
} ShimRTPSendParameters;

typedef struct {
    ShimRTPSender* sender;
    ShimRTPEncodingParameters* encodings;
    int max_encodings;
    ShimRTPSendParameters out_params;
} ShimRTPSenderGetParametersParams;

typedef struct {
    ShimRTPSender* sender;
    const ShimRTPSendParameters* params;
    ShimErrorBuffer* error_out; /* Optional: buffer for error message */
} ShimRTPSenderSetParametersParams;

/*
 * Get current RTP send parameters.
 *
 * @param params Output parameters (encodings + limits + out params)
 * @return SHIM_OK on success
 */
SHIM_EXPORT int shim_rtp_sender_get_parameters(
    ShimRTPSenderGetParametersParams* params
);

/*
 * Set RTP send parameters (for bitrate/simulcast control).
 *
 * @param params Parameters to set
 * @return SHIM_OK on success
 */
SHIM_EXPORT int shim_rtp_sender_set_parameters(
    ShimRTPSenderSetParametersParams* params
);

/* Get the track associated with this sender */
SHIM_EXPORT void* shim_rtp_sender_get_track(ShimRTPSender* sender);

/* ============================================================================
 * RTPReceiver API
 * ========================================================================== */

/* Get the track associated with this receiver */
SHIM_EXPORT void* shim_rtp_receiver_get_track(ShimRTPReceiver* receiver);

/* ============================================================================
 * RTPTransceiver API
 * ========================================================================== */

typedef struct ShimRTPTransceiver ShimRTPTransceiver;

/* Transceiver direction enum */
typedef enum {
    SHIM_TRANSCEIVER_DIRECTION_SENDRECV = 0,
    SHIM_TRANSCEIVER_DIRECTION_SENDONLY = 1,
    SHIM_TRANSCEIVER_DIRECTION_RECVONLY = 2,
    SHIM_TRANSCEIVER_DIRECTION_INACTIVE = 3,
    SHIM_TRANSCEIVER_DIRECTION_STOPPED = 4,
} ShimTransceiverDirection;

/*
 * Get the current direction of the transceiver.
 */
SHIM_EXPORT int shim_transceiver_get_direction(ShimRTPTransceiver* transceiver);

/*
 * Set the direction of the transceiver.
 *
 * @param transceiver Transceiver handle
 * @param direction New direction (ShimTransceiverDirection)
 * @return SHIM_OK on success
 */
typedef struct {
    ShimRTPTransceiver* transceiver;
    int direction;
    ShimErrorBuffer* error_out; /* Optional: buffer for error message */
} ShimTransceiverSetDirectionParams;

SHIM_EXPORT int shim_transceiver_set_direction(
    ShimTransceiverSetDirectionParams* params
);

/*
 * Get the current direction as negotiated in SDP.
 */
SHIM_EXPORT int shim_transceiver_get_current_direction(ShimRTPTransceiver* transceiver);

/*
 * Stop the transceiver.
 */
typedef struct {
    ShimRTPTransceiver* transceiver;
    ShimErrorBuffer* error_out; /* Optional: buffer for error message */
} ShimTransceiverStopParams;

SHIM_EXPORT int shim_transceiver_stop(
    ShimTransceiverStopParams* params
);

/*
 * Get the mid (media ID) of the transceiver.
 */
SHIM_EXPORT const char* shim_transceiver_mid(ShimRTPTransceiver* transceiver);

/*
 * Get the sender associated with this transceiver.
 */
SHIM_EXPORT ShimRTPSender* shim_transceiver_get_sender(ShimRTPTransceiver* transceiver);

/*
 * Get the receiver associated with this transceiver.
 */
SHIM_EXPORT ShimRTPReceiver* shim_transceiver_get_receiver(ShimRTPTransceiver* transceiver);

/* ============================================================================
 * PeerConnection Extended API
 * ========================================================================== */

/*
 * Add a transceiver with specified media kind and direction.
 *
 * @param params Parameters (pc + kind + direction)
 * @return Transceiver handle, or NULL on failure
 */
typedef struct {
    ShimPeerConnection* pc;
    int kind;
    int direction;
    ShimErrorBuffer* error_out; /* Optional: buffer for error message */
} ShimPeerConnectionAddTransceiverParams;

SHIM_EXPORT ShimRTPTransceiver* shim_peer_connection_add_transceiver(
    ShimPeerConnectionAddTransceiverParams* params
);

/*
 * Get all senders associated with this PeerConnection.
 *
 * @param params Output parameters (senders + counts)
 * @return SHIM_OK on success
 */
typedef struct {
    ShimPeerConnection* pc;
    ShimRTPSender** senders;
    int max_senders;
    int out_count;
} ShimPeerConnectionGetSendersParams;

SHIM_EXPORT int shim_peer_connection_get_senders(
    ShimPeerConnectionGetSendersParams* params
);

/*
 * Get all receivers associated with this PeerConnection.
 *
 * @param params Output parameters (receivers + counts)
 * @return SHIM_OK on success
 */
typedef struct {
    ShimPeerConnection* pc;
    ShimRTPReceiver** receivers;
    int max_receivers;
    int out_count;
} ShimPeerConnectionGetReceiversParams;

SHIM_EXPORT int shim_peer_connection_get_receivers(
    ShimPeerConnectionGetReceiversParams* params
);

/*
 * Get all transceivers associated with this PeerConnection.
 *
 * @param params Output parameters (transceivers + counts)
 * @return SHIM_OK on success
 */
typedef struct {
    ShimPeerConnection* pc;
    ShimRTPTransceiver** transceivers;
    int max_transceivers;
    int out_count;
} ShimPeerConnectionGetTransceiversParams;

SHIM_EXPORT int shim_peer_connection_get_transceivers(
    ShimPeerConnectionGetTransceiversParams* params
);

/*
 * Trigger an ICE restart on the next offer.
 *
 * @param pc PeerConnection handle
 * @return SHIM_OK on success
 */
SHIM_EXPORT int shim_peer_connection_restart_ice(ShimPeerConnection* pc);

/* ============================================================================
 * Statistics API
 * ========================================================================== */

typedef struct {
    /* Timestamp of this stats report */
    int64_t timestamp_us;

    /* Transport stats */
    int64_t bytes_sent;
    int64_t bytes_received;
    int64_t packets_sent;
    int64_t packets_received;
    int64_t packets_lost;

    /* Connection quality */
    double round_trip_time_ms;
    double jitter_ms;
    double available_outgoing_bitrate;
    double available_incoming_bitrate;

    /* ICE candidate pair stats */
    int64_t current_rtt_ms;
    int64_t total_rtt_ms;
    int64_t responses_received;

    /* Video specific */
    int frames_encoded;
    int frames_decoded;
    int frames_dropped;
    int key_frames_encoded;
    int key_frames_decoded;
    int nack_count;
    int pli_count;
    int fir_count;
    int qp_sum;

    /* Audio specific */
    double audio_level;
    double total_audio_energy;
    int concealment_events;

    /* SCTP/DataChannel stats */
    int64_t data_channels_opened;
    int64_t data_channels_closed;
    int64_t messages_sent;
    int64_t messages_received;
    int64_t bytes_sent_data_channel;
    int64_t bytes_received_data_channel;

    /* Quality limitation */
    int quality_limitation_reason;  /* 0=none, 1=cpu, 2=bandwidth, 3=other */
    int quality_limitation_duration_ms;

    /* Remote inbound/outbound RTP stats */
    int64_t remote_packets_lost;
    double remote_jitter_ms;
    double remote_round_trip_time_ms;

    /* Jitter buffer stats (from RTCInboundRtpStreamStats) */
    double jitter_buffer_delay_ms;          /* Total time spent in jitter buffer / emitted count */
    double jitter_buffer_target_delay_ms;   /* Target delay for adaptive buffer */
    double jitter_buffer_minimum_delay_ms;  /* User-configured minimum delay */
    int64_t jitter_buffer_emitted_count;    /* Number of samples/frames emitted from buffer */
} ShimRTCStats;

/* Quality limitation reasons */
#define SHIM_QUALITY_LIMITATION_NONE      0
#define SHIM_QUALITY_LIMITATION_CPU       1
#define SHIM_QUALITY_LIMITATION_BANDWIDTH 2
#define SHIM_QUALITY_LIMITATION_OTHER     3

/* ============================================================================
 * Codec Capability API
 * ========================================================================== */

/* Codec capability info */
typedef struct {
    char mime_type[64];        /* e.g., "video/VP9", "audio/opus" */
    int clock_rate;            /* Clock rate in Hz */
    int channels;              /* Audio channels (0 for video) */
    char sdp_fmtp_line[256];   /* SDP format parameters */
    int payload_type;          /* Preferred payload type */
} ShimCodecCapability;

/*
 * Get supported video send codecs.
 *
 * @param params Output parameters (codecs + counts)
 * @return SHIM_OK on success
 */
/* Codec query parameters. Caller-owned buffers; shim uses them only during the call. */
typedef struct {
    ShimCodecCapability* codecs;
    int max_codecs;
    int out_count;
} ShimGetSupportedVideoCodecsParams;

SHIM_EXPORT int shim_get_supported_video_codecs(
    ShimGetSupportedVideoCodecsParams* params
);

/*
 * Get supported audio send codecs.
 *
 * @param params Output parameters (codecs + counts)
 * @return SHIM_OK on success
 */
/* Codec query parameters. Caller-owned buffers; shim uses them only during the call. */
typedef struct {
    ShimCodecCapability* codecs;
    int max_codecs;
    int out_count;
} ShimGetSupportedAudioCodecsParams;

SHIM_EXPORT int shim_get_supported_audio_codecs(
    ShimGetSupportedAudioCodecsParams* params
);

/*
 * Check if a specific codec is supported for encoding.
 *
 * @param mime_type MIME type (e.g., "video/VP9")
 * @return 1 if supported, 0 otherwise
 */
SHIM_EXPORT int shim_is_codec_supported(const char* mime_type);

/* ============================================================================
 * RTPSender Codec API
 * ========================================================================== */

/*
 * Get negotiated codecs for this sender from RtpParameters.
 * These are the codecs that were actually negotiated in SDP.
 *
 * @param params Output parameters (sender + codecs + counts)
 * @return SHIM_OK on success
 */
/* Negotiated codec query parameters. Caller-owned buffers; shim uses them only during the call. */
typedef struct {
    ShimRTPSender* sender;
    ShimCodecCapability* codecs;
    int max_codecs;
    int out_count;
} ShimRTPSenderGetNegotiatedCodecsParams;

SHIM_EXPORT int shim_rtp_sender_get_negotiated_codecs(
    ShimRTPSenderGetNegotiatedCodecsParams* params
);

/*
 * Set the preferred codec for this sender.
 * This reorders the codec list in RtpParameters to prefer the specified codec.
 * The codec must have been negotiated in SDP.
 * After calling this, a renegotiation is typically needed for the change to take effect.
 *
 * @param params Input parameters (sender + codec identifiers)
 * @return SHIM_OK on success, SHIM_ERROR_NOT_FOUND if codec not negotiated
 */
typedef struct {
    ShimRTPSender* sender;
    const char* mime_type;
    int payload_type;
    ShimErrorBuffer* error_out; /* Optional: buffer for error message */
} ShimRTPSenderSetPreferredCodecParams;

SHIM_EXPORT int shim_rtp_sender_set_preferred_codec(
    ShimRTPSenderSetPreferredCodecParams* params
);

/* ============================================================================
 * Transceiver Codec Preferences API
 * ========================================================================== */

/*
 * Set codec preferences for a transceiver.
 * This controls which codecs are negotiated in SDP.
 * Must be called before creating offer/answer.
 *
 * @param params Input parameters (transceiver + codecs + count)
 * @return SHIM_OK on success
 */
typedef struct {
    ShimRTPTransceiver* transceiver;
    const ShimCodecCapability* codecs;
    int count;
    ShimErrorBuffer* error_out; /* Optional: buffer for error message */
} ShimTransceiverSetCodecPreferencesParams;

SHIM_EXPORT int shim_transceiver_set_codec_preferences(
    ShimTransceiverSetCodecPreferencesParams* params
);

/*
 * Get codec preferences for a transceiver.
 *
 * @param params Output parameters (codecs + counts)
 * @return SHIM_OK on success
 */
typedef struct {
    ShimRTPTransceiver* transceiver;
    ShimCodecCapability* codecs;
    int max_codecs;
    int out_count;
} ShimTransceiverGetCodecPreferencesParams;

SHIM_EXPORT int shim_transceiver_get_codec_preferences(
    ShimTransceiverGetCodecPreferencesParams* params
);

/* ============================================================================
 * Bandwidth Estimation API
 * ========================================================================== */

/* Bandwidth estimation info */
typedef struct {
    int64_t timestamp_us;
    int64_t target_bitrate_bps;      /* Target bitrate from BWE */
    int64_t available_send_bps;       /* Available send bandwidth */
    int64_t available_recv_bps;       /* Available receive bandwidth */
    int64_t pacing_rate_bps;          /* Current pacing rate */
    int congestion_window;            /* Congestion window size */
    double loss_rate;                 /* Observed packet loss rate (0.0-1.0) */
} ShimBandwidthEstimate;

/* Callback for bandwidth estimate updates */
typedef void (*ShimOnBandwidthEstimate)(void* ctx, const ShimBandwidthEstimate* estimate);

/*
 * Set bandwidth estimate callback.
 *
 * @param params Input parameters (pc + callback + ctx)
 */
typedef struct {
    ShimPeerConnection* pc;
    ShimOnBandwidthEstimate callback;
    void* ctx;
} ShimPeerConnectionSetOnBandwidthEstimateParams;

SHIM_EXPORT void shim_peer_connection_set_on_bandwidth_estimate(
    ShimPeerConnectionSetOnBandwidthEstimateParams* params
);

/*
 * Get current bandwidth estimate.
 *
 * @param params Output parameters (pc + estimate)
 * @return SHIM_OK on success
 */
/* Bandwidth estimate parameters. Caller-owned buffers; shim uses them only during the call. */
typedef struct {
    ShimPeerConnection* pc;
    ShimBandwidthEstimate out_estimate;
} ShimPeerConnectionGetBandwidthEstimateParams;

SHIM_EXPORT int shim_peer_connection_get_bandwidth_estimate(
    ShimPeerConnectionGetBandwidthEstimateParams* params
);

/*
 * Get connection statistics.
 *
 * @param params Output parameters (pc + stats)
 * @return SHIM_OK on success
 */
/* Stats parameters. Caller-owned buffers; shim uses them only during the call. */
typedef struct {
    ShimPeerConnection* pc;
    ShimRTCStats out_stats;
} ShimPeerConnectionGetStatsParams;

SHIM_EXPORT int shim_peer_connection_get_stats(
    ShimPeerConnectionGetStatsParams* params
);

/*
 * Get statistics for a specific sender.
 *
 * @param params Output parameters (sender + stats)
 * @return SHIM_OK on success
 */
/* Stats parameters. Caller-owned buffers; shim uses them only during the call. */
typedef struct {
    ShimRTPSender* sender;
    ShimRTCStats out_stats;
} ShimRTPSenderGetStatsParams;

SHIM_EXPORT int shim_rtp_sender_get_stats(
    ShimRTPSenderGetStatsParams* params
);

/*
 * Get statistics for a specific receiver.
 *
 * @param params Output parameters (receiver + stats)
 * @return SHIM_OK on success
 */
/* Stats parameters. Caller-owned buffers; shim uses them only during the call. */
typedef struct {
    ShimRTPReceiver* receiver;
    ShimRTCStats out_stats;
} ShimRTPReceiverGetStatsParams;

SHIM_EXPORT int shim_rtp_receiver_get_stats(
    ShimRTPReceiverGetStatsParams* params
);

/* ============================================================================
 * RTCP Feedback API
 * ========================================================================== */

/*
 * Request a keyframe from the sender (send PLI).
 *
 * @param receiver RTPReceiver handle
 * @return SHIM_OK on success
 */

/*
 * Callback for RTCP feedback events.
 *
 * @param ctx User context
 * @param type Feedback type (0=PLI, 1=FIR, 2=NACK)
 * @param ssrc SSRC of the affected stream
 */
typedef void (*ShimOnRTCPFeedback)(void* ctx, int type, uint32_t ssrc);

/*
 * Set RTCP feedback callback on a sender.
 *
 * @param params Input parameters (sender + callback + ctx)
 */
typedef struct {
    ShimRTPSender* sender;
    ShimOnRTCPFeedback callback;
    void* ctx;
} ShimRTPSenderSetOnRTCPFeedbackParams;

SHIM_EXPORT void shim_rtp_sender_set_on_rtcp_feedback(
    ShimRTPSenderSetOnRTCPFeedbackParams* params
);

/* ============================================================================
 * Simulcast/SVC Layer Control API
 * ========================================================================== */

/*
 * Enable or disable a specific simulcast layer.
 *
 * @param params Input parameters (sender + rid + active)
 * @return SHIM_OK on success
 */
typedef struct {
    ShimRTPSender* sender;
    const char* rid;
    int active;
    ShimErrorBuffer* error_out; /* Optional: buffer for error message */
} ShimRTPSenderSetLayerActiveParams;

SHIM_EXPORT int shim_rtp_sender_set_layer_active(
    ShimRTPSenderSetLayerActiveParams* params
);

/*
 * Set the maximum bitrate for a specific layer.
 *
 * @param params Input parameters (sender + rid + max bitrate)
 * @return SHIM_OK on success
 */
typedef struct {
    ShimRTPSender* sender;
    const char* rid;
    uint32_t max_bitrate_bps;
    ShimErrorBuffer* error_out; /* Optional: buffer for error message */
} ShimRTPSenderSetLayerBitrateParams;

SHIM_EXPORT int shim_rtp_sender_set_layer_bitrate(
    ShimRTPSenderSetLayerBitrateParams* params
);

/*
 * Get the number of active layers.
 *
 * @param params Output parameters (sender + counts)
 * @return SHIM_OK on success
 */
typedef struct {
    ShimRTPSender* sender;
    int out_spatial;
    int out_temporal;
} ShimRTPSenderGetActiveLayersParams;

SHIM_EXPORT int shim_rtp_sender_get_active_layers(
    ShimRTPSenderGetActiveLayersParams* params
);

/*
 * Set the scalability mode for a sender (e.g., "L3T3_KEY", "L1T2").
 *
 * @param params Input parameters (sender + mode)
 * @return SHIM_OK on success
 */
typedef struct {
    ShimRTPSender* sender;
    const char* scalability_mode;
    ShimErrorBuffer* error_out; /* Optional: buffer for error message */
} ShimRTPSenderSetScalabilityModeParams;

SHIM_EXPORT int shim_rtp_sender_set_scalability_mode(
    ShimRTPSenderSetScalabilityModeParams* params
);

/*
 * Get the current scalability mode for a sender.
 *
 * @param params Output parameters (mode buffer + size)
 * @return SHIM_OK on success
 */
typedef struct {
    ShimRTPSender* sender;
    char* mode_out;
    int mode_out_size;
} ShimRTPSenderGetScalabilityModeParams;

SHIM_EXPORT int shim_rtp_sender_get_scalability_mode(
    ShimRTPSenderGetScalabilityModeParams* params
);

/* ============================================================================
 * Video Track Source API (for frame injection)
 * ========================================================================== */

typedef struct ShimVideoTrackSource ShimVideoTrackSource;

/*
 * Create a video track source that can receive pushed frames.
 * This source can be added to a PeerConnection to send video.
 *
 * @param pc PeerConnection to associate with
 * @param width Video width
 * @param height Video height
 * @return Track source handle, or NULL on failure
 */
typedef struct {
    ShimPeerConnection* pc;
    int width;
    int height;
} ShimVideoTrackSourceCreateParams;

SHIM_EXPORT ShimVideoTrackSource* shim_video_track_source_create(
    ShimVideoTrackSourceCreateParams* params
);

/*
 * Push an I420 video frame to the source.
 * The frame will be encoded and sent via the PeerConnection.
 *
 * @param source Track source handle
 * @param y_plane Y plane data
 * @param u_plane U plane data
 * @param v_plane V plane data
 * @param y_stride Y plane stride
 * @param u_stride U plane stride
 * @param v_stride V plane stride
 * @param timestamp_us Timestamp in microseconds
 * @return SHIM_OK on success
 */
typedef struct {
    ShimVideoTrackSource* source;
    const uint8_t* y_plane;
    const uint8_t* u_plane;
    const uint8_t* v_plane;
    int y_stride;
    int u_stride;
    int v_stride;
    int64_t timestamp_us;
} ShimVideoTrackSourcePushFrameParams;

SHIM_EXPORT int shim_video_track_source_push_frame(
    ShimVideoTrackSourcePushFrameParams* params
);

/*
 * Add a video track to the PeerConnection using this source.
 *
 * @param pc PeerConnection handle
 * @param source Video track source
 * @param track_id Track ID
 * @param stream_id Stream ID
 * @return RTPSender handle, or NULL on failure
 */
typedef struct {
    ShimPeerConnection* pc;
    ShimVideoTrackSource* source;
    const char* track_id;
    const char* stream_id;
    ShimErrorBuffer* error_out;  /* Optional: buffer for error message */
} ShimPeerConnectionAddVideoTrackFromSourceParams;

SHIM_EXPORT ShimRTPSender* shim_peer_connection_add_video_track_from_source(
    ShimPeerConnectionAddVideoTrackFromSourceParams* params
);

SHIM_EXPORT void shim_video_track_source_destroy(ShimVideoTrackSource* source);

/* ============================================================================
 * Audio Track Source API (for frame injection)
 * ========================================================================== */

typedef struct ShimAudioTrackSource ShimAudioTrackSource;

/*
 * Create an audio track source that can receive pushed audio frames.
 *
 * @param pc PeerConnection to associate with
 * @param sample_rate Audio sample rate (e.g., 48000)
 * @param channels Number of channels (1 or 2)
 * @return Track source handle, or NULL on failure
 */
typedef struct {
    ShimPeerConnection* pc;
    int sample_rate;
    int channels;
} ShimAudioTrackSourceCreateParams;

SHIM_EXPORT ShimAudioTrackSource* shim_audio_track_source_create(
    ShimAudioTrackSourceCreateParams* params
);

/*
 * Push audio samples to the source.
 *
 * @param source Track source handle
 * @param samples PCM samples (S16LE interleaved)
 * @param num_samples Number of samples per channel
 * @param timestamp_us Timestamp in microseconds
 * @return SHIM_OK on success
 */
typedef struct {
    ShimAudioTrackSource* source;
    const int16_t* samples;
    int num_samples;
    int64_t timestamp_us;
} ShimAudioTrackSourcePushFrameParams;

SHIM_EXPORT int shim_audio_track_source_push_frame(
    ShimAudioTrackSourcePushFrameParams* params
);

/*
 * Add an audio track to the PeerConnection using this source.
 *
 * @param pc PeerConnection handle
 * @param source Audio track source
 * @param track_id Track ID
 * @param stream_id Stream ID
 * @return RTPSender handle, or NULL on failure
 */
typedef struct {
    ShimPeerConnection* pc;
    ShimAudioTrackSource* source;
    const char* track_id;
    const char* stream_id;
    ShimErrorBuffer* error_out;  /* Optional: buffer for error message */
} ShimPeerConnectionAddAudioTrackFromSourceParams;

SHIM_EXPORT ShimRTPSender* shim_peer_connection_add_audio_track_from_source(
    ShimPeerConnectionAddAudioTrackFromSourceParams* params
);

SHIM_EXPORT void shim_audio_track_source_destroy(ShimAudioTrackSource* source);

/* ============================================================================
 * Remote Track Frame Receiving API (for reading frames from received tracks)
 * ========================================================================== */

/*
 * Callback for receiving video frames from a remote track.
 * Called on the WebRTC worker thread - should return quickly!
 *
 * @param ctx User context
 * @param width Frame width
 * @param height Frame height
 * @param y_plane Y plane data
 * @param u_plane U plane data
 * @param v_plane V plane data
 * @param y_stride Y plane stride
 * @param u_stride U plane stride
 * @param v_stride V plane stride
 * @param timestamp_us Timestamp in microseconds
 */
typedef void (*ShimOnVideoFrame)(
    void* ctx,
    int width,
    int height,
    const uint8_t* y_plane,
    const uint8_t* u_plane,
    const uint8_t* v_plane,
    int y_stride,
    int u_stride,
    int v_stride,
    int64_t timestamp_us
);

/*
 * Callback for receiving audio frames from a remote track.
 * Called on the WebRTC worker thread - should return quickly!
 *
 * @param ctx User context
 * @param samples PCM samples (S16LE interleaved)
 * @param num_samples Number of samples per channel
 * @param sample_rate Sample rate
 * @param channels Number of channels
 * @param timestamp_us Timestamp in microseconds
 */
typedef void (*ShimOnAudioFrame)(
    void* ctx,
    const int16_t* samples,
    int num_samples,
    int sample_rate,
    int channels,
    int64_t timestamp_us
);

/*
 * Set a video frame callback on a remote video track.
 * The track pointer comes from the OnTrack callback.
 *
 * @param track Track pointer from OnTrack callback
 * @param callback Frame callback function
 * @param ctx User context passed to callback
 * @return SHIM_OK on success
 */
typedef struct {
    void* track;
    ShimOnVideoFrame callback;
    void* ctx;
} ShimTrackSetVideoSinkParams;

SHIM_EXPORT int shim_track_set_video_sink(
    ShimTrackSetVideoSinkParams* params
);

/*
 * Set an audio frame callback on a remote audio track.
 * The track pointer comes from the OnTrack callback.
 *
 * @param track Track pointer from OnTrack callback
 * @param callback Frame callback function
 * @param ctx User context passed to callback
 * @return SHIM_OK on success
 */
typedef struct {
    void* track;
    ShimOnAudioFrame callback;
    void* ctx;
} ShimTrackSetAudioSinkParams;

SHIM_EXPORT int shim_track_set_audio_sink(
    ShimTrackSetAudioSinkParams* params
);

/*
 * Remove video sink from a track.
 */
SHIM_EXPORT void shim_track_remove_video_sink(void* track);

/*
 * Remove audio sink from a track.
 */
SHIM_EXPORT void shim_track_remove_audio_sink(void* track);

/*
 * Get track kind ("audio" or "video").
 */
SHIM_EXPORT const char* shim_track_kind(void* track);

/*
 * Get track ID.
 */
SHIM_EXPORT const char* shim_track_id(void* track);

/* ============================================================================
 * DataChannel API
 * ========================================================================== */

typedef void (*ShimOnDataChannelMessage)(void* ctx, const uint8_t* data, int size, int is_binary);
typedef void (*ShimOnDataChannelOpen)(void* ctx);
typedef void (*ShimOnDataChannelClose)(void* ctx);

typedef struct {
    ShimDataChannel* dc;
    ShimOnDataChannelMessage callback;
    void* ctx;
} ShimDataChannelSetOnMessageParams;

SHIM_EXPORT void shim_data_channel_set_on_message(
    ShimDataChannelSetOnMessageParams* params
);

typedef struct {
    ShimDataChannel* dc;
    ShimOnDataChannelOpen callback;
    void* ctx;
} ShimDataChannelSetOnOpenParams;

SHIM_EXPORT void shim_data_channel_set_on_open(
    ShimDataChannelSetOnOpenParams* params
);

typedef struct {
    ShimDataChannel* dc;
    ShimOnDataChannelClose callback;
    void* ctx;
} ShimDataChannelSetOnCloseParams;

SHIM_EXPORT void shim_data_channel_set_on_close(
    ShimDataChannelSetOnCloseParams* params
);

typedef struct {
    ShimDataChannel* dc;
    const uint8_t* data;
    int size;
    int is_binary;
    ShimErrorBuffer* error_out; /* Optional: buffer for error message */
} ShimDataChannelSendParams;

SHIM_EXPORT int shim_data_channel_send(
    ShimDataChannelSendParams* params
);
SHIM_EXPORT const char* shim_data_channel_label(ShimDataChannel* dc);
SHIM_EXPORT int shim_data_channel_ready_state(ShimDataChannel* dc);
SHIM_EXPORT void shim_data_channel_close(ShimDataChannel* dc);
SHIM_EXPORT void shim_data_channel_destroy(ShimDataChannel* dc);

/* ============================================================================
 * Device Enumeration API
 * ========================================================================== */

typedef struct {
    char device_id[256];
    char label[256];
    int kind;  /* 0=videoinput, 1=audioinput, 2=audiooutput */
} ShimDeviceInfo;

/*
 * Enumerate all available media devices.
 *
 * @param devices Pre-allocated array of ShimDeviceInfo
 * @param max_devices Maximum number of devices to return
 * @param out_count Output: actual number of devices found
 * @return SHIM_OK on success
 */
typedef struct {
    ShimDeviceInfo* devices;
    int max_devices;
    int out_count;
    ShimErrorBuffer* error_out;  /* Optional: buffer for error message */
} ShimEnumerateDevicesParams;

SHIM_EXPORT int shim_enumerate_devices(
    ShimEnumerateDevicesParams* params
);

/*
 * Check if camera access is authorized.
 * @return 1 if authorized, 0 if not authorized or undetermined, -1 on error
 */
SHIM_EXPORT int shim_check_camera_permission(void);

/*
 * Check if microphone access is authorized.
 * @return 1 if authorized, 0 if not authorized or undetermined, -1 on error
 */
SHIM_EXPORT int shim_check_microphone_permission(void);

/*
 * Request camera access permission (blocking).
 * On macOS, this shows the system permission dialog if needed.
 * On other platforms, returns 1 immediately.
 * @return 1 if authorized, 0 if denied
 */
SHIM_EXPORT int shim_request_camera_permission(void);

/*
 * Request microphone access permission (blocking).
 * On macOS, this shows the system permission dialog if needed.
 * On other platforms, returns 1 immediately.
 * @return 1 if authorized, 0 if denied
 */
SHIM_EXPORT int shim_request_microphone_permission(void);

/* ============================================================================
 * Video Capture API
 * ========================================================================== */

typedef struct ShimVideoCapture ShimVideoCapture;

/*
 * Callback for video frames captured from camera/screen.
 * Called from capture thread - must be thread-safe.
 */
typedef void (*ShimVideoCaptureCallback)(
    void* ctx,
    const uint8_t* y_plane,
    const uint8_t* u_plane,
    const uint8_t* v_plane,
    int width,
    int height,
    int y_stride,
    int u_stride,
    int v_stride,
    int64_t timestamp_us
);

/*
 * Create a video capture device.
 *
 * @param device_id Device ID from enumeration, or NULL for default
 * @param width Desired capture width
 * @param height Desired capture height
 * @param fps Desired framerate
 * @return Capture handle, or NULL on failure
 */
typedef struct {
    const char* device_id;
    int width;
    int height;
    int fps;
    ShimErrorBuffer* error_out;  /* Optional: buffer for error message */
} ShimVideoCaptureCreateParams;

SHIM_EXPORT ShimVideoCapture* shim_video_capture_create(
    ShimVideoCaptureCreateParams* params
);

/*
 * Start video capture with callback.
 *
 * @param cap Capture handle
 * @param callback Function called for each frame
 * @param ctx User context passed to callback
 * @return SHIM_OK on success
 */
typedef struct {
    ShimVideoCapture* cap;
    ShimVideoCaptureCallback callback;
    void* ctx;
    ShimErrorBuffer* error_out;  /* Optional: buffer for error message */
} ShimVideoCaptureStartParams;

SHIM_EXPORT int shim_video_capture_start(
    ShimVideoCaptureStartParams* params
);

SHIM_EXPORT void shim_video_capture_stop(ShimVideoCapture* cap);
SHIM_EXPORT void shim_video_capture_destroy(ShimVideoCapture* cap);

/* ============================================================================
 * Audio Capture API
 * ========================================================================== */

typedef struct ShimAudioCapture ShimAudioCapture;

/*
 * Callback for audio samples captured from microphone.
 * Called from capture thread - must be thread-safe.
 */
typedef void (*ShimAudioCaptureCallback)(
    void* ctx,
    const int16_t* samples,
    int num_samples,
    int num_channels,
    int sample_rate,
    int64_t timestamp_us
);

/*
 * Create an audio capture device.
 *
 * @param device_id Device ID from enumeration, or NULL for default
 * @param sample_rate Desired sample rate (e.g., 48000)
 * @param channels Number of channels (1 or 2)
 * @return Capture handle, or NULL on failure
 */
typedef struct {
    const char* device_id;
    int sample_rate;
    int channels;
    ShimErrorBuffer* error_out;  /* Optional: buffer for error message */
} ShimAudioCaptureCreateParams;

SHIM_EXPORT ShimAudioCapture* shim_audio_capture_create(
    ShimAudioCaptureCreateParams* params
);

/*
 * Start audio capture with callback.
 *
 * @param cap Capture handle
 * @param callback Function called for each audio buffer
 * @param ctx User context passed to callback
 * @return SHIM_OK on success
 */
typedef struct {
    ShimAudioCapture* cap;
    ShimAudioCaptureCallback callback;
    void* ctx;
    ShimErrorBuffer* error_out;  /* Optional: buffer for error message */
} ShimAudioCaptureStartParams;

SHIM_EXPORT int shim_audio_capture_start(
    ShimAudioCaptureStartParams* params
);

SHIM_EXPORT void shim_audio_capture_stop(ShimAudioCapture* cap);
SHIM_EXPORT void shim_audio_capture_destroy(ShimAudioCapture* cap);

/* ============================================================================
 * Screen/Window Capture API
 * ========================================================================== */

typedef struct ShimScreenCapture ShimScreenCapture;

typedef struct {
    int64_t id;         /* Screen or window ID */
    char title[256];    /* Window title or screen name */
    int is_window;      /* 0=screen, 1=window */
} ShimScreenInfo;

/*
 * Enumerate available screens and windows for capture.
 *
 * @param screens Pre-allocated array of ShimScreenInfo
 * @param max_screens Maximum number to return
 * @param out_count Output: actual count found
 * @return SHIM_OK on success
 */
typedef struct {
    ShimScreenInfo* screens;
    int max_screens;
    int out_count;
    ShimErrorBuffer* error_out;  /* Optional: buffer for error message */
} ShimEnumerateScreensParams;

SHIM_EXPORT int shim_enumerate_screens(
    ShimEnumerateScreensParams* params
);

/*
 * Create a screen or window capture.
 *
 * @param screen_or_window_id ID from enumeration
 * @param is_window 0 for screen capture, 1 for window capture
 * @param fps Desired capture framerate
 * @return Capture handle, or NULL on failure
 */
typedef struct {
    int64_t screen_or_window_id;
    int is_window;
    int fps;
    ShimErrorBuffer* error_out;  /* Optional: buffer for error message */
} ShimScreenCaptureCreateParams;

SHIM_EXPORT ShimScreenCapture* shim_screen_capture_create(
    ShimScreenCaptureCreateParams* params
);

/*
 * Start screen capture with callback.
 * Uses same callback type as video capture (I420 frames).
 */
typedef struct {
    ShimScreenCapture* cap;
    ShimVideoCaptureCallback callback;
    void* ctx;
    ShimErrorBuffer* error_out;  /* Optional: buffer for error message */
} ShimScreenCaptureStartParams;

SHIM_EXPORT int shim_screen_capture_start(
    ShimScreenCaptureStartParams* params
);

SHIM_EXPORT void shim_screen_capture_stop(ShimScreenCapture* cap);
SHIM_EXPORT void shim_screen_capture_destroy(ShimScreenCapture* cap);

/* ============================================================================
 * Jitter Buffer Control API
 *
 * NOTE: libwebrtc provides limited jitter buffer control via RtpReceiverInterface.
 * Only SetJitterBufferMinimumDelay() is available - this sets a floor for the
 * adaptive jitter buffer algorithm.
 *
 * For full jitter buffer stats, use PeerConnection::GetStats() which provides
 * RTCInboundRtpStreamStats with jitterBufferDelay, jitterBufferTargetDelay, etc.
 * ========================================================================== */

/*
 * Set the minimum jitter buffer delay for a receiver.
 *
 * This sets a floor for libwebrtc's adaptive jitter buffer. The actual delay
 * may be higher based on network conditions, but won't go below this value.
 *
 * Note: This calls RtpReceiverInterface::SetJitterBufferMinimumDelay() internally.
 * There is no API to set a maximum delay or disable adaptive mode.
 *
 * @param params Input parameters (receiver + delay)
 * @return SHIM_OK on success
 */
typedef struct {
    ShimRTPReceiver* receiver;
    int min_delay_ms;
} ShimRTPReceiverSetJitterBufferMinDelayParams;

SHIM_EXPORT int shim_rtp_receiver_set_jitter_buffer_min_delay(
    ShimRTPReceiverSetJitterBufferMinDelayParams* params
);

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
