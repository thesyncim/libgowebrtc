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
 * @param dst_buffer_size Size of dst_buffer in bytes
 * @param out_size Output: number of bytes written to dst_buffer
 * @param out_is_keyframe Output: true if encoded frame is a keyframe
 * @return SHIM_OK on success, SHIM_ERROR_BUFFER_TOO_SMALL if buffer insufficient
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
    int dst_buffer_size,        /* Size of dst_buffer */
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
SHIM_EXPORT void shim_peer_connection_set_on_signaling_state_change(
    ShimPeerConnection* pc,
    ShimOnSignalingStateChange callback,
    void* ctx
);
SHIM_EXPORT void shim_peer_connection_set_on_ice_connection_state_change(
    ShimPeerConnection* pc,
    ShimOnICEConnectionStateChange callback,
    void* ctx
);
SHIM_EXPORT void shim_peer_connection_set_on_ice_gathering_state_change(
    ShimPeerConnection* pc,
    ShimOnICEGatheringStateChange callback,
    void* ctx
);

/* Negotiation needed callback */
typedef void (*ShimOnNegotiationNeeded)(void* ctx);

SHIM_EXPORT void shim_peer_connection_set_on_negotiation_needed(
    ShimPeerConnection* pc,
    ShimOnNegotiationNeeded callback,
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

/*
 * Get current RTP send parameters.
 *
 * @param sender RTPSender handle
 * @param out_params Pre-allocated output structure
 * @param encodings Pre-allocated array for encodings
 * @param max_encodings Maximum number of encodings
 * @return SHIM_OK on success
 */
SHIM_EXPORT int shim_rtp_sender_get_parameters(
    ShimRTPSender* sender,
    ShimRTPSendParameters* out_params,
    ShimRTPEncodingParameters* encodings,
    int max_encodings
);

/*
 * Set RTP send parameters (for bitrate/simulcast control).
 *
 * @param sender RTPSender handle
 * @param params Parameters to set
 * @return SHIM_OK on success
 */
SHIM_EXPORT int shim_rtp_sender_set_parameters(
    ShimRTPSender* sender,
    const ShimRTPSendParameters* params
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
SHIM_EXPORT int shim_transceiver_set_direction(
    ShimRTPTransceiver* transceiver,
    int direction
);

/*
 * Get the current direction as negotiated in SDP.
 */
SHIM_EXPORT int shim_transceiver_get_current_direction(ShimRTPTransceiver* transceiver);

/*
 * Stop the transceiver.
 */
SHIM_EXPORT int shim_transceiver_stop(ShimRTPTransceiver* transceiver);

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
 * @param pc PeerConnection handle
 * @param kind Media kind (0=audio, 1=video)
 * @param direction Initial direction (ShimTransceiverDirection)
 * @return Transceiver handle, or NULL on failure
 */
SHIM_EXPORT ShimRTPTransceiver* shim_peer_connection_add_transceiver(
    ShimPeerConnection* pc,
    int kind,
    int direction
);

/*
 * Get all senders associated with this PeerConnection.
 *
 * @param pc PeerConnection handle
 * @param senders Pre-allocated array for sender pointers
 * @param max_senders Maximum number of senders
 * @param out_count Output: actual number of senders
 * @return SHIM_OK on success
 */
SHIM_EXPORT int shim_peer_connection_get_senders(
    ShimPeerConnection* pc,
    ShimRTPSender** senders,
    int max_senders,
    int* out_count
);

/*
 * Get all receivers associated with this PeerConnection.
 *
 * @param pc PeerConnection handle
 * @param receivers Pre-allocated array for receiver pointers
 * @param max_receivers Maximum number of receivers
 * @param out_count Output: actual number of receivers
 * @return SHIM_OK on success
 */
SHIM_EXPORT int shim_peer_connection_get_receivers(
    ShimPeerConnection* pc,
    ShimRTPReceiver** receivers,
    int max_receivers,
    int* out_count
);

/*
 * Get all transceivers associated with this PeerConnection.
 *
 * @param pc PeerConnection handle
 * @param transceivers Pre-allocated array for transceiver pointers
 * @param max_transceivers Maximum number of transceivers
 * @param out_count Output: actual number of transceivers
 * @return SHIM_OK on success
 */
SHIM_EXPORT int shim_peer_connection_get_transceivers(
    ShimPeerConnection* pc,
    ShimRTPTransceiver** transceivers,
    int max_transceivers,
    int* out_count
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
 * @param codecs Pre-allocated array of codec capabilities
 * @param max_codecs Maximum codecs to return
 * @param out_count Output: actual number of codecs
 * @return SHIM_OK on success
 */
SHIM_EXPORT int shim_get_supported_video_codecs(
    ShimCodecCapability* codecs,
    int max_codecs,
    int* out_count
);

/*
 * Get supported audio send codecs.
 *
 * @param codecs Pre-allocated array of codec capabilities
 * @param max_codecs Maximum codecs to return
 * @param out_count Output: actual number of codecs
 * @return SHIM_OK on success
 */
SHIM_EXPORT int shim_get_supported_audio_codecs(
    ShimCodecCapability* codecs,
    int max_codecs,
    int* out_count
);

/*
 * Check if a specific codec is supported for encoding.
 *
 * @param mime_type MIME type (e.g., "video/VP9")
 * @return 1 if supported, 0 otherwise
 */
SHIM_EXPORT int shim_is_codec_supported(const char* mime_type);

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
 * @param pc PeerConnection handle
 * @param callback Callback function
 * @param ctx User context
 */
SHIM_EXPORT void shim_peer_connection_set_on_bandwidth_estimate(
    ShimPeerConnection* pc,
    ShimOnBandwidthEstimate callback,
    void* ctx
);

/*
 * Get current bandwidth estimate.
 *
 * @param pc PeerConnection handle
 * @param out_estimate Pre-allocated estimate structure
 * @return SHIM_OK on success
 */
SHIM_EXPORT int shim_peer_connection_get_bandwidth_estimate(
    ShimPeerConnection* pc,
    ShimBandwidthEstimate* out_estimate
);

/*
 * Get connection statistics.
 *
 * @param pc PeerConnection handle
 * @param out_stats Pre-allocated stats structure
 * @return SHIM_OK on success
 */
SHIM_EXPORT int shim_peer_connection_get_stats(
    ShimPeerConnection* pc,
    ShimRTCStats* out_stats
);

/*
 * Get statistics for a specific sender.
 *
 * @param sender RTPSender handle
 * @param out_stats Pre-allocated stats structure
 * @return SHIM_OK on success
 */
SHIM_EXPORT int shim_rtp_sender_get_stats(
    ShimRTPSender* sender,
    ShimRTCStats* out_stats
);

/*
 * Get statistics for a specific receiver.
 *
 * @param receiver RTPReceiver handle
 * @param out_stats Pre-allocated stats structure
 * @return SHIM_OK on success
 */
SHIM_EXPORT int shim_rtp_receiver_get_stats(
    ShimRTPReceiver* receiver,
    ShimRTCStats* out_stats
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
SHIM_EXPORT int shim_rtp_receiver_request_keyframe(ShimRTPReceiver* receiver);

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
 * @param sender RTPSender handle
 * @param callback Callback function
 * @param ctx User context
 */
SHIM_EXPORT void shim_rtp_sender_set_on_rtcp_feedback(
    ShimRTPSender* sender,
    ShimOnRTCPFeedback callback,
    void* ctx
);

/* ============================================================================
 * Simulcast/SVC Layer Control API
 * ========================================================================== */

/*
 * Enable or disable a specific simulcast layer.
 *
 * @param sender RTPSender handle
 * @param rid RID of the layer (e.g., "low", "mid", "high")
 * @param active 1 to enable, 0 to disable
 * @return SHIM_OK on success
 */
SHIM_EXPORT int shim_rtp_sender_set_layer_active(
    ShimRTPSender* sender,
    const char* rid,
    int active
);

/*
 * Set the maximum bitrate for a specific layer.
 *
 * @param sender RTPSender handle
 * @param rid RID of the layer
 * @param max_bitrate_bps Maximum bitrate in bps
 * @return SHIM_OK on success
 */
SHIM_EXPORT int shim_rtp_sender_set_layer_bitrate(
    ShimRTPSender* sender,
    const char* rid,
    uint32_t max_bitrate_bps
);

/*
 * Get the number of active layers.
 *
 * @param sender RTPSender handle
 * @param out_spatial Output: number of spatial layers
 * @param out_temporal Output: number of temporal layers
 * @return SHIM_OK on success
 */
SHIM_EXPORT int shim_rtp_sender_get_active_layers(
    ShimRTPSender* sender,
    int* out_spatial,
    int* out_temporal
);

/*
 * Set the scalability mode for a sender (e.g., "L3T3_KEY", "L1T2").
 *
 * @param sender RTPSender handle
 * @param scalability_mode Scalability mode string
 * @return SHIM_OK on success
 */
SHIM_EXPORT int shim_rtp_sender_set_scalability_mode(
    ShimRTPSender* sender,
    const char* scalability_mode
);

/*
 * Get the current scalability mode for a sender.
 *
 * @param sender RTPSender handle
 * @param mode_out Pre-allocated buffer for mode string
 * @param mode_out_size Size of output buffer
 * @return SHIM_OK on success
 */
SHIM_EXPORT int shim_rtp_sender_get_scalability_mode(
    ShimRTPSender* sender,
    char* mode_out,
    int mode_out_size
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
SHIM_EXPORT ShimVideoTrackSource* shim_video_track_source_create(
    ShimPeerConnection* pc,
    int width,
    int height
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
SHIM_EXPORT int shim_video_track_source_push_frame(
    ShimVideoTrackSource* source,
    const uint8_t* y_plane,
    const uint8_t* u_plane,
    const uint8_t* v_plane,
    int y_stride,
    int u_stride,
    int v_stride,
    int64_t timestamp_us
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
SHIM_EXPORT ShimRTPSender* shim_peer_connection_add_video_track_from_source(
    ShimPeerConnection* pc,
    ShimVideoTrackSource* source,
    const char* track_id,
    const char* stream_id
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
SHIM_EXPORT ShimAudioTrackSource* shim_audio_track_source_create(
    ShimPeerConnection* pc,
    int sample_rate,
    int channels
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
SHIM_EXPORT int shim_audio_track_source_push_frame(
    ShimAudioTrackSource* source,
    const int16_t* samples,
    int num_samples,
    int64_t timestamp_us
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
SHIM_EXPORT ShimRTPSender* shim_peer_connection_add_audio_track_from_source(
    ShimPeerConnection* pc,
    ShimAudioTrackSource* source,
    const char* track_id,
    const char* stream_id
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
SHIM_EXPORT int shim_track_set_video_sink(
    void* track,
    ShimOnVideoFrame callback,
    void* ctx
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
SHIM_EXPORT int shim_track_set_audio_sink(
    void* track,
    ShimOnAudioFrame callback,
    void* ctx
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
SHIM_EXPORT int shim_enumerate_devices(
    ShimDeviceInfo* devices,
    int max_devices,
    int* out_count
);

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
SHIM_EXPORT ShimVideoCapture* shim_video_capture_create(
    const char* device_id,
    int width,
    int height,
    int fps
);

/*
 * Start video capture with callback.
 *
 * @param cap Capture handle
 * @param callback Function called for each frame
 * @param ctx User context passed to callback
 * @return SHIM_OK on success
 */
SHIM_EXPORT int shim_video_capture_start(
    ShimVideoCapture* cap,
    ShimVideoCaptureCallback callback,
    void* ctx
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
SHIM_EXPORT ShimAudioCapture* shim_audio_capture_create(
    const char* device_id,
    int sample_rate,
    int channels
);

/*
 * Start audio capture with callback.
 *
 * @param cap Capture handle
 * @param callback Function called for each audio buffer
 * @param ctx User context passed to callback
 * @return SHIM_OK on success
 */
SHIM_EXPORT int shim_audio_capture_start(
    ShimAudioCapture* cap,
    ShimAudioCaptureCallback callback,
    void* ctx
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
SHIM_EXPORT int shim_enumerate_screens(
    ShimScreenInfo* screens,
    int max_screens,
    int* out_count
);

/*
 * Create a screen or window capture.
 *
 * @param screen_or_window_id ID from enumeration
 * @param is_window 0 for screen capture, 1 for window capture
 * @param fps Desired capture framerate
 * @return Capture handle, or NULL on failure
 */
SHIM_EXPORT ShimScreenCapture* shim_screen_capture_create(
    int64_t screen_or_window_id,
    int is_window,
    int fps
);

/*
 * Start screen capture with callback.
 * Uses same callback type as video capture (I420 frames).
 */
SHIM_EXPORT int shim_screen_capture_start(
    ShimScreenCapture* cap,
    ShimVideoCaptureCallback callback,
    void* ctx
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
 * @param receiver RTPReceiver handle
 * @param min_delay_ms Minimum delay in milliseconds (0 = let libwebrtc decide)
 * @return SHIM_OK on success
 */
SHIM_EXPORT int shim_rtp_receiver_set_jitter_buffer_min_delay(
    ShimRTPReceiver* receiver,
    int min_delay_ms
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
