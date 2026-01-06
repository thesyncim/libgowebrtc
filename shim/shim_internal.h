/*
 * shim_internal.h - Internal shared structures
 *
 * Contains internal structure definitions shared between shim modules.
 */

#ifndef SHIM_INTERNAL_H_
#define SHIM_INTERNAL_H_

#include "shim_common.h"

#include <mutex>
#include <vector>

#include "api/peer_connection_interface.h"
#include "api/rtp_sender_interface.h"
#include "api/scoped_refptr.h"

/* ============================================================================
 * PeerConnection Internal Structure
 * ========================================================================== */

struct ShimPeerConnection {
    webrtc::scoped_refptr<webrtc::PeerConnectionFactoryInterface> factory;
    webrtc::scoped_refptr<webrtc::PeerConnectionInterface> peer_connection;
    std::mutex mutex;

    // Callbacks
    ShimOnICECandidate on_ice_candidate = nullptr;
    void* on_ice_candidate_ctx = nullptr;
    ShimOnConnectionStateChange on_connection_state_change = nullptr;
    void* on_connection_state_change_ctx = nullptr;
    ShimOnTrack on_track = nullptr;
    void* on_track_ctx = nullptr;
    ShimOnDataChannel on_data_channel = nullptr;
    void* on_data_channel_ctx = nullptr;
    ShimOnSignalingStateChange on_signaling_state_change = nullptr;
    void* on_signaling_state_change_ctx = nullptr;
    ShimOnICEConnectionStateChange on_ice_connection_state_change = nullptr;
    void* on_ice_connection_state_change_ctx = nullptr;
    ShimOnICEGatheringStateChange on_ice_gathering_state_change = nullptr;
    void* on_ice_gathering_state_change_ctx = nullptr;
    ShimOnNegotiationNeeded on_negotiation_needed = nullptr;
    void* on_negotiation_needed_ctx = nullptr;

    // Track senders
    std::vector<webrtc::scoped_refptr<webrtc::RtpSenderInterface>> senders;
};

// Alias for internal struct reference
typedef ShimPeerConnection ShimPeerConnectionInternal;

#endif  // SHIM_INTERNAL_H_
