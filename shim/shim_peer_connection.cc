/*
 * shim_peer_connection.cc - PeerConnection implementation
 *
 * Core PeerConnection functionality including offer/answer,
 * track management, and ICE handling.
 * Updated for libwebrtc M141 API.
 */

#include "shim_common.h"

#include <condition_variable>
#include <cstring>
#include <map>
#include <vector>

#include "rtc_base/thread.h"
#include "api/peer_connection_interface.h"
#include "api/create_peerconnection_factory.h"
#include "api/audio_codecs/builtin_audio_encoder_factory.h"
#include "api/audio_codecs/builtin_audio_decoder_factory.h"
#include "api/media_types.h"
#include "api/video_codecs/builtin_video_encoder_factory.h"
#include "api/video_codecs/builtin_video_decoder_factory.h"
#include "api/data_channel_interface.h"
#include "api/rtp_sender_interface.h"
#include "api/rtp_receiver_interface.h"
#include "api/rtp_transceiver_interface.h"
#include "api/stats/rtc_stats.h"
#include "api/stats/rtc_stats_report.h"
#include "api/stats/rtcstats_objects.h"
#include "api/scoped_refptr.h"
#include "rtc_base/ref_counted_object.h"
#include "media/base/media_channel.h"
#include "media/engine/internal_encoder_factory.h"
#include "media/engine/internal_decoder_factory.h"
#include "rtc_base/time_utils.h"

// Include internal structure definition
#include "shim_internal.h"

/* ============================================================================
 * PeerConnection Observer
 * ========================================================================== */

class PeerConnectionObserver : public webrtc::PeerConnectionObserver {
public:
    explicit PeerConnectionObserver(ShimPeerConnection* pc) : pc_(pc) {}

    void OnSignalingChange(webrtc::PeerConnectionInterface::SignalingState state) override {
        if (pc_->on_signaling_state_change) {
            pc_->on_signaling_state_change(pc_->on_signaling_state_change_ctx, static_cast<int>(state));
        }
    }

    void OnDataChannel(webrtc::scoped_refptr<webrtc::DataChannelInterface> channel) override {
        if (pc_->on_data_channel) {
            // Store in PC's data_channels vector to maintain proper reference count
            pc_->data_channels.push_back(channel);
            // Pass raw pointer (PC owns the reference)
            pc_->on_data_channel(pc_->on_data_channel_ctx, channel.get());
        }
    }

    void OnRenegotiationNeeded() override {
        if (pc_->on_negotiation_needed) {
            pc_->on_negotiation_needed(pc_->on_negotiation_needed_ctx);
        }
    }

    void OnIceConnectionChange(webrtc::PeerConnectionInterface::IceConnectionState state) override {
        if (pc_->on_ice_connection_state_change) {
            pc_->on_ice_connection_state_change(pc_->on_ice_connection_state_change_ctx, static_cast<int>(state));
        }
    }

    void OnIceGatheringChange(webrtc::PeerConnectionInterface::IceGatheringState state) override {
        if (pc_->on_ice_gathering_state_change) {
            pc_->on_ice_gathering_state_change(pc_->on_ice_gathering_state_change_ctx, static_cast<int>(state));
        }
    }

    void OnIceCandidate(const webrtc::IceCandidateInterface* candidate) override {
        if (pc_->on_ice_candidate) {
            std::string sdp;
            candidate->ToString(&sdp);

            ShimICECandidate shim_candidate;
            shim_candidate.candidate = sdp.c_str();
            shim_candidate.sdp_mid = candidate->sdp_mid().c_str();
            shim_candidate.sdp_mline_index = candidate->sdp_mline_index();

            pc_->on_ice_candidate(pc_->on_ice_candidate_ctx, &shim_candidate);
        }
    }

    void OnConnectionChange(webrtc::PeerConnectionInterface::PeerConnectionState state) override {
        if (pc_->on_connection_state_change) {
            pc_->on_connection_state_change(pc_->on_connection_state_change_ctx, static_cast<int>(state));
        }
    }

    void OnTrack(webrtc::scoped_refptr<webrtc::RtpTransceiverInterface> transceiver) override {
        if (pc_->on_track) {
            auto receiver = transceiver->receiver();
            auto track = receiver->track();
            pc_->on_track(pc_->on_track_ctx, track.get(), receiver.get(), "");
        }
    }

private:
    ShimPeerConnection* pc_;
};

// Keep observer alive with PC
static std::map<ShimPeerConnection*, std::unique_ptr<PeerConnectionObserver>> g_pc_observers;
static std::mutex g_pc_observers_mutex;

/* ============================================================================
 * C API Implementation
 * ========================================================================== */

extern "C" {

SHIM_EXPORT ShimPeerConnection* shim_peer_connection_create(ShimPeerConnectionCreateParams* params) {
    shim::InitializeGlobals();

    if (!params) {
        return nullptr;
    }

    auto pc = std::make_unique<ShimPeerConnection>();
    const ShimPeerConnectionConfig* config = params->config;
    ShimErrorBuffer* error_out = params->error_out;

    // Create encoder/decoder factories.
    bool use_software = shim::ShouldUseSoftwareCodecs();
    auto video_encoder_factory = use_software
        ? std::make_unique<webrtc::InternalEncoderFactory>()
        : webrtc::CreateBuiltinVideoEncoderFactory();
    auto video_decoder_factory = use_software
        ? std::make_unique<webrtc::InternalDecoderFactory>()
        : webrtc::CreateBuiltinVideoDecoderFactory();

    // Create PeerConnectionFactory
    pc->factory = webrtc::CreatePeerConnectionFactory(
        shim::GetNetworkThread(),
        shim::GetWorkerThread(),
        shim::GetSignalingThread(),
        nullptr,  // default_adm
        webrtc::CreateBuiltinAudioEncoderFactory(),
        webrtc::CreateBuiltinAudioDecoderFactory(),
        std::move(video_encoder_factory),
        std::move(video_decoder_factory),
        nullptr,  // audio_mixer
        nullptr   // audio_processing
    );

    if (!pc->factory) {
        shim::SetErrorMessage(error_out, "PeerConnectionFactory creation failed");
        return nullptr;
    }

    // Configure ICE servers
    webrtc::PeerConnectionInterface::RTCConfiguration rtc_config;
    rtc_config.sdp_semantics = webrtc::SdpSemantics::kUnifiedPlan;

    if (config) {
        for (int i = 0; i < config->ice_server_count; i++) {
            webrtc::PeerConnectionInterface::IceServer server;
            for (int j = 0; j < config->ice_servers[i].url_count; j++) {
                server.urls.push_back(config->ice_servers[i].urls[j]);
            }
            if (config->ice_servers[i].username) {
                server.username = config->ice_servers[i].username;
            }
            if (config->ice_servers[i].credential) {
                server.password = config->ice_servers[i].credential;
            }
            rtc_config.servers.push_back(server);
        }
    }

    // Create observer and PeerConnection
    auto observer = std::make_unique<PeerConnectionObserver>(pc.get());

    webrtc::PeerConnectionDependencies deps(observer.get());

    auto result = pc->factory->CreatePeerConnectionOrError(rtc_config, std::move(deps));
    if (!result.ok()) {
        shim::SetErrorFromRTCError(error_out, result.error());
        return nullptr;
    }

    pc->peer_connection = result.MoveValue();

    // Store observer
    {
        std::lock_guard<std::mutex> lock(g_pc_observers_mutex);
        g_pc_observers[pc.get()] = std::move(observer);
    }

    return pc.release();
}

SHIM_EXPORT void shim_peer_connection_destroy(ShimPeerConnection* pc) {
    if (pc) {
        if (pc->peer_connection) {
            pc->peer_connection->Close();
        }

        // Clean up observer
        {
            std::lock_guard<std::mutex> lock(g_pc_observers_mutex);
            g_pc_observers.erase(pc);
        }

        delete pc;
    }
}

SHIM_EXPORT void shim_peer_connection_set_on_ice_candidate(ShimPeerConnectionSetOnICECandidateParams* params) {
    if (params && params->pc) {
        params->pc->on_ice_candidate = params->callback;
        params->pc->on_ice_candidate_ctx = params->ctx;
    }
}

SHIM_EXPORT void shim_peer_connection_set_on_connection_state_change(ShimPeerConnectionSetOnConnectionStateChangeParams* params) {
    if (params && params->pc) {
        params->pc->on_connection_state_change = params->callback;
        params->pc->on_connection_state_change_ctx = params->ctx;
    }
}

SHIM_EXPORT void shim_peer_connection_set_on_track(ShimPeerConnectionSetOnTrackParams* params) {
    if (params && params->pc) {
        params->pc->on_track = params->callback;
        params->pc->on_track_ctx = params->ctx;
    }
}

SHIM_EXPORT void shim_peer_connection_set_on_data_channel(ShimPeerConnectionSetOnDataChannelParams* params) {
    if (params && params->pc) {
        params->pc->on_data_channel = params->callback;
        params->pc->on_data_channel_ctx = params->ctx;
    }
}

SHIM_EXPORT void shim_peer_connection_set_on_signaling_state_change(ShimPeerConnectionSetOnSignalingStateChangeParams* params) {
    if (params && params->pc) {
        params->pc->on_signaling_state_change = params->callback;
        params->pc->on_signaling_state_change_ctx = params->ctx;
    }
}

SHIM_EXPORT void shim_peer_connection_set_on_ice_connection_state_change(ShimPeerConnectionSetOnICEConnectionStateChangeParams* params) {
    if (params && params->pc) {
        params->pc->on_ice_connection_state_change = params->callback;
        params->pc->on_ice_connection_state_change_ctx = params->ctx;
    }
}

SHIM_EXPORT void shim_peer_connection_set_on_ice_gathering_state_change(ShimPeerConnectionSetOnICEGatheringStateChangeParams* params) {
    if (params && params->pc) {
        params->pc->on_ice_gathering_state_change = params->callback;
        params->pc->on_ice_gathering_state_change_ctx = params->ctx;
    }
}

SHIM_EXPORT void shim_peer_connection_set_on_negotiation_needed(ShimPeerConnectionSetOnNegotiationNeededParams* params) {
    if (params && params->pc) {
        params->pc->on_negotiation_needed = params->callback;
        params->pc->on_negotiation_needed_ctx = params->ctx;
    }
}

SHIM_EXPORT int shim_peer_connection_create_offer(ShimPeerConnectionCreateOfferParams* params) {
    if (!params) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    params->out_sdp_len = 0;
    if (!params->pc || !params->pc->peer_connection || !params->sdp_out) {
        shim::SetErrorMessage(params->error_out, "invalid parameter", SHIM_ERROR_INVALID_PARAM);
        return SHIM_ERROR_INVALID_PARAM;
    }

    class CreateSessionDescriptionObserver
        : public webrtc::CreateSessionDescriptionObserver {
    public:
        std::string sdp;
        std::string error_message;
        bool success = false;
        std::mutex mutex;
        std::condition_variable cv;
        bool done = false;

        void OnSuccess(webrtc::SessionDescriptionInterface* desc) override {
            desc->ToString(&sdp);
            std::lock_guard<std::mutex> lock(mutex);
            success = true;
            done = true;
            cv.notify_one();
        }

        void OnFailure(webrtc::RTCError error) override {
            std::lock_guard<std::mutex> lock(mutex);
            error_message = error.message();
            if (error_message.empty()) {
                error_message = webrtc::ToString(error.type());
            }
            success = false;
            done = true;
            cv.notify_one();
        }
    };

    auto observer = webrtc::make_ref_counted<CreateSessionDescriptionObserver>();

    webrtc::PeerConnectionInterface::RTCOfferAnswerOptions options;
    params->pc->peer_connection->CreateOffer(observer.get(), options);

    // Wait for completion
    {
        std::unique_lock<std::mutex> lock(observer->mutex);
        observer->cv.wait(lock, [&]() { return observer->done; });
    }

    if (!observer->success) {
        shim::SetErrorMessage(params->error_out, observer->error_message);
        return SHIM_ERROR_INIT_FAILED;
    }

    int len = static_cast<int>(observer->sdp.size());
    if (len >= params->sdp_out_size) {
        shim::SetErrorMessage(params->error_out, "SDP buffer too small", SHIM_ERROR_BUFFER_TOO_SMALL);
        return SHIM_ERROR_BUFFER_TOO_SMALL;
    }

    memcpy(params->sdp_out, observer->sdp.c_str(), len + 1);
    params->out_sdp_len = len;

    return SHIM_OK;
}

SHIM_EXPORT int shim_peer_connection_create_answer(ShimPeerConnectionCreateAnswerParams* params) {
    if (!params) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    params->out_sdp_len = 0;
    if (!params->pc || !params->pc->peer_connection || !params->sdp_out) {
        shim::SetErrorMessage(params->error_out, "invalid parameter", SHIM_ERROR_INVALID_PARAM);
        return SHIM_ERROR_INVALID_PARAM;
    }

    class CreateSessionDescriptionObserver
        : public webrtc::CreateSessionDescriptionObserver {
    public:
        std::string sdp;
        std::string error_message;
        bool success = false;
        std::mutex mutex;
        std::condition_variable cv;
        bool done = false;

        void OnSuccess(webrtc::SessionDescriptionInterface* desc) override {
            desc->ToString(&sdp);
            std::lock_guard<std::mutex> lock(mutex);
            success = true;
            done = true;
            cv.notify_one();
        }

        void OnFailure(webrtc::RTCError error) override {
            std::lock_guard<std::mutex> lock(mutex);
            error_message = error.message();
            if (error_message.empty()) {
                error_message = webrtc::ToString(error.type());
            }
            success = false;
            done = true;
            cv.notify_one();
        }
    };

    auto observer = webrtc::make_ref_counted<CreateSessionDescriptionObserver>();

    webrtc::PeerConnectionInterface::RTCOfferAnswerOptions options;
    params->pc->peer_connection->CreateAnswer(observer.get(), options);

    // Wait for completion
    {
        std::unique_lock<std::mutex> lock(observer->mutex);
        observer->cv.wait(lock, [&]() { return observer->done; });
    }

    if (!observer->success) {
        shim::SetErrorMessage(params->error_out, observer->error_message);
        return SHIM_ERROR_INIT_FAILED;
    }

    int len = static_cast<int>(observer->sdp.size());
    if (len >= params->sdp_out_size) {
        shim::SetErrorMessage(params->error_out, "SDP buffer too small", SHIM_ERROR_BUFFER_TOO_SMALL);
        return SHIM_ERROR_BUFFER_TOO_SMALL;
    }

    memcpy(params->sdp_out, observer->sdp.c_str(), len + 1);
    params->out_sdp_len = len;

    return SHIM_OK;
}

SHIM_EXPORT int shim_peer_connection_set_local_description(ShimPeerConnectionSetLocalDescriptionParams* params) {
    if (!params) {
        return SHIM_ERROR_INVALID_PARAM;
    }
    if (!params->pc || !params->pc->peer_connection || !params->sdp) {
        shim::SetErrorMessage(params->error_out, "invalid parameter", SHIM_ERROR_INVALID_PARAM);
        return SHIM_ERROR_INVALID_PARAM;
    }

    webrtc::SdpType sdp_type;
    switch (params->type) {
        case 0: sdp_type = webrtc::SdpType::kOffer; break;
        case 1: sdp_type = webrtc::SdpType::kPrAnswer; break;
        case 2: sdp_type = webrtc::SdpType::kAnswer; break;
        default:
            shim::SetErrorMessage(params->error_out, "invalid SDP type", SHIM_ERROR_INVALID_PARAM);
            return SHIM_ERROR_INVALID_PARAM;
    }

    webrtc::SdpParseError parse_error;
    auto desc = webrtc::CreateSessionDescription(sdp_type, params->sdp, &parse_error);
    if (!desc) {
        std::string msg = "SDP parse error";
        if (!parse_error.description.empty()) {
            msg += ": " + parse_error.description;
        }
        shim::SetErrorMessage(params->error_out, msg, SHIM_ERROR_INVALID_PARAM);
        return SHIM_ERROR_INVALID_PARAM;
    }

    class SetSessionDescriptionObserver
        : public webrtc::SetSessionDescriptionObserver {
    public:
        bool success = false;
        std::string error_message;
        std::mutex mutex;
        std::condition_variable cv;
        bool done = false;

        void OnSuccess() override {
            std::lock_guard<std::mutex> lock(mutex);
            success = true;
            done = true;
            cv.notify_one();
        }

        void OnFailure(webrtc::RTCError error) override {
            std::lock_guard<std::mutex> lock(mutex);
            error_message = error.message();
            if (error_message.empty()) {
                error_message = webrtc::ToString(error.type());
            }
            success = false;
            done = true;
            cv.notify_one();
        }
    };

    auto observer = webrtc::make_ref_counted<SetSessionDescriptionObserver>();
    params->pc->peer_connection->SetLocalDescription(observer.get(), desc.release());

    {
        std::unique_lock<std::mutex> lock(observer->mutex);
        observer->cv.wait(lock, [&]() { return observer->done; });
    }

    if (!observer->success) {
        shim::SetErrorMessage(params->error_out, observer->error_message);
        return SHIM_ERROR_INIT_FAILED;
    }
    return SHIM_OK;
}

SHIM_EXPORT int shim_peer_connection_set_remote_description(ShimPeerConnectionSetRemoteDescriptionParams* params) {
    if (!params) {
        return SHIM_ERROR_INVALID_PARAM;
    }
    if (!params->pc || !params->pc->peer_connection || !params->sdp) {
        shim::SetErrorMessage(params->error_out, "invalid parameter", SHIM_ERROR_INVALID_PARAM);
        return SHIM_ERROR_INVALID_PARAM;
    }

    webrtc::SdpType sdp_type;
    switch (params->type) {
        case 0: sdp_type = webrtc::SdpType::kOffer; break;
        case 1: sdp_type = webrtc::SdpType::kPrAnswer; break;
        case 2: sdp_type = webrtc::SdpType::kAnswer; break;
        default:
            shim::SetErrorMessage(params->error_out, "invalid SDP type", SHIM_ERROR_INVALID_PARAM);
            return SHIM_ERROR_INVALID_PARAM;
    }

    webrtc::SdpParseError parse_error;
    auto desc = webrtc::CreateSessionDescription(sdp_type, params->sdp, &parse_error);
    if (!desc) {
        std::string msg = "SDP parse error";
        if (!parse_error.description.empty()) {
            msg += ": " + parse_error.description;
        }
        shim::SetErrorMessage(params->error_out, msg, SHIM_ERROR_INVALID_PARAM);
        return SHIM_ERROR_INVALID_PARAM;
    }

    class SetSessionDescriptionObserver
        : public webrtc::SetSessionDescriptionObserver {
    public:
        bool success = false;
        std::string error_message;
        std::mutex mutex;
        std::condition_variable cv;
        bool done = false;

        void OnSuccess() override {
            std::lock_guard<std::mutex> lock(mutex);
            success = true;
            done = true;
            cv.notify_one();
        }

        void OnFailure(webrtc::RTCError error) override {
            std::lock_guard<std::mutex> lock(mutex);
            error_message = error.message();
            if (error_message.empty()) {
                error_message = webrtc::ToString(error.type());
            }
            success = false;
            done = true;
            cv.notify_one();
        }
    };

    auto observer = webrtc::make_ref_counted<SetSessionDescriptionObserver>();
    params->pc->peer_connection->SetRemoteDescription(observer.get(), desc.release());

    {
        std::unique_lock<std::mutex> lock(observer->mutex);
        observer->cv.wait(lock, [&]() { return observer->done; });
    }

    if (!observer->success) {
        shim::SetErrorMessage(params->error_out, observer->error_message);
        return SHIM_ERROR_INIT_FAILED;
    }
    return SHIM_OK;
}

SHIM_EXPORT int shim_peer_connection_add_ice_candidate(ShimPeerConnectionAddICECandidateParams* params) {
    if (!params) {
        return SHIM_ERROR_INVALID_PARAM;
    }
    if (!params->pc || !params->pc->peer_connection || !params->candidate) {
        shim::SetErrorMessage(params->error_out, "invalid parameter", SHIM_ERROR_INVALID_PARAM);
        return SHIM_ERROR_INVALID_PARAM;
    }

    webrtc::SdpParseError parse_error;
    webrtc::IceCandidate* ice_candidate = webrtc::CreateIceCandidate(
        params->sdp_mid ? params->sdp_mid : "",
        params->sdp_mline_index,
        params->candidate,
        &parse_error
    );

    if (!ice_candidate) {
        std::string msg = "ICE candidate parse error";
        if (!parse_error.description.empty()) {
            msg += ": " + parse_error.description;
        }
        shim::SetErrorMessage(params->error_out, msg, SHIM_ERROR_INVALID_PARAM);
        return SHIM_ERROR_INVALID_PARAM;
    }

    if (!params->pc->peer_connection->AddIceCandidate(ice_candidate)) {
        delete ice_candidate;
        shim::SetErrorMessage(params->error_out, "AddIceCandidate failed");
        return SHIM_ERROR_INIT_FAILED;
    }
    delete ice_candidate;

    return SHIM_OK;
}

SHIM_EXPORT int shim_peer_connection_signaling_state(ShimPeerConnection* pc) {
    if (!pc || !pc->peer_connection) return -1;
    return static_cast<int>(pc->peer_connection->signaling_state());
}

SHIM_EXPORT int shim_peer_connection_ice_connection_state(ShimPeerConnection* pc) {
    if (!pc || !pc->peer_connection) return -1;
    return static_cast<int>(pc->peer_connection->ice_connection_state());
}

SHIM_EXPORT int shim_peer_connection_ice_gathering_state(ShimPeerConnection* pc) {
    if (!pc || !pc->peer_connection) return -1;
    return static_cast<int>(pc->peer_connection->ice_gathering_state());
}

SHIM_EXPORT int shim_peer_connection_connection_state(ShimPeerConnection* pc) {
    if (!pc || !pc->peer_connection) return -1;
    return static_cast<int>(pc->peer_connection->peer_connection_state());
}

SHIM_EXPORT ShimRTPSender* shim_peer_connection_add_track(ShimPeerConnectionAddTrackParams* params) {
    if (!params) {
        return nullptr;
    }
    if (!params->pc || !params->pc->peer_connection || !params->track_id) {
        shim::SetErrorMessage(params->error_out, "invalid parameter");
        return nullptr;
    }

    // Create transceiver for the media type
    auto result = params->pc->peer_connection->AddTransceiver(
        params->codec == SHIM_CODEC_OPUS ? webrtc::MediaType::AUDIO : webrtc::MediaType::VIDEO
    );

    if (!result.ok()) {
        shim::SetErrorFromRTCError(params->error_out, result.error());
        return nullptr;
    }

    auto sender = result.value()->sender();
    params->pc->senders.push_back(sender);

    return reinterpret_cast<ShimRTPSender*>(sender.get());
}

SHIM_EXPORT int shim_peer_connection_remove_track(ShimPeerConnectionRemoveTrackParams* params) {
    if (!params) {
        return SHIM_ERROR_INVALID_PARAM;
    }
    if (!params->pc || !params->pc->peer_connection || !params->sender) {
        shim::SetErrorMessage(params->error_out, "invalid parameter", SHIM_ERROR_INVALID_PARAM);
        return SHIM_ERROR_INVALID_PARAM;
    }

    auto webrtc_sender = reinterpret_cast<webrtc::RtpSenderInterface*>(params->sender);

    auto result = params->pc->peer_connection->RemoveTrackOrError(
        webrtc::scoped_refptr<webrtc::RtpSenderInterface>(webrtc_sender)
    );

    if (!result.ok()) {
        shim::SetErrorFromRTCError(params->error_out, result);
        return SHIM_ERROR_INIT_FAILED;
    }
    return SHIM_OK;
}

SHIM_EXPORT ShimDataChannel* shim_peer_connection_create_data_channel(ShimPeerConnectionCreateDataChannelParams* params) {
    if (!params) {
        return nullptr;
    }
    if (!params->pc || !params->pc->peer_connection || !params->label) {
        shim::SetErrorMessage(params->error_out, "invalid parameter");
        return nullptr;
    }

    webrtc::DataChannelInit config;
    config.ordered = params->ordered != 0;
    if (params->max_retransmits >= 0) {
        config.maxRetransmits = params->max_retransmits;
    }
    if (params->protocol) {
        config.protocol = params->protocol;
    }

    auto result = params->pc->peer_connection->CreateDataChannelOrError(params->label, &config);
    if (!result.ok()) {
        shim::SetErrorFromRTCError(params->error_out, result.error());
        return nullptr;
    }

    // Store in PC's data_channels vector to maintain proper reference count
    auto channel = result.MoveValue();
    params->pc->data_channels.push_back(channel);

    // Return raw pointer (PC owns the reference)
    return reinterpret_cast<ShimDataChannel*>(channel.get());
}

SHIM_EXPORT void shim_peer_connection_close(ShimPeerConnection* pc) {
    if (pc && pc->peer_connection) {
        pc->peer_connection->Close();
    }
}

SHIM_EXPORT ShimRTPTransceiver* shim_peer_connection_add_transceiver(ShimPeerConnectionAddTransceiverParams* params) {
    if (!params) {
        return nullptr;
    }
    if (!params->pc || !params->pc->peer_connection) {
        shim::SetErrorMessage(params->error_out, "invalid parameter");
        return nullptr;
    }

    webrtc::MediaType media_type = params->kind == 0 ? webrtc::MediaType::AUDIO : webrtc::MediaType::VIDEO;
    webrtc::RtpTransceiverInit init;
    init.direction = static_cast<webrtc::RtpTransceiverDirection>(params->direction);

    auto result = params->pc->peer_connection->AddTransceiver(media_type, init);
    if (!result.ok()) {
        shim::SetErrorFromRTCError(params->error_out, result.error());
        return nullptr;
    }

    return reinterpret_cast<ShimRTPTransceiver*>(result.value().get());
}

SHIM_EXPORT int shim_peer_connection_get_senders(ShimPeerConnectionGetSendersParams* params) {
    if (!params) {
        return SHIM_ERROR_INVALID_PARAM;
    }
    params->out_count = 0;
    if (!params->pc || !params->pc->peer_connection || !params->senders || params->max_senders <= 0) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    auto all_senders = params->pc->peer_connection->GetSenders();
    int count = std::min(static_cast<int>(all_senders.size()), params->max_senders);

    for (int i = 0; i < count; i++) {
        params->senders[i] = reinterpret_cast<ShimRTPSender*>(all_senders[i].get());
    }
    params->out_count = count;

    return SHIM_OK;
}

SHIM_EXPORT int shim_peer_connection_get_receivers(ShimPeerConnectionGetReceiversParams* params) {
    if (!params) {
        return SHIM_ERROR_INVALID_PARAM;
    }
    params->out_count = 0;
    if (!params->pc || !params->pc->peer_connection || !params->receivers || params->max_receivers <= 0) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    auto all_receivers = params->pc->peer_connection->GetReceivers();
    int count = std::min(static_cast<int>(all_receivers.size()), params->max_receivers);

    for (int i = 0; i < count; i++) {
        params->receivers[i] = reinterpret_cast<ShimRTPReceiver*>(all_receivers[i].get());
    }
    params->out_count = count;

    return SHIM_OK;
}

SHIM_EXPORT int shim_peer_connection_get_transceivers(ShimPeerConnectionGetTransceiversParams* params) {
    if (!params) {
        return SHIM_ERROR_INVALID_PARAM;
    }
    params->out_count = 0;
    if (!params->pc || !params->pc->peer_connection || !params->transceivers || params->max_transceivers <= 0) {
        return SHIM_ERROR_INVALID_PARAM;
    }

    auto all_transceivers = params->pc->peer_connection->GetTransceivers();
    int count = std::min(static_cast<int>(all_transceivers.size()), params->max_transceivers);

    for (int i = 0; i < count; i++) {
        params->transceivers[i] = reinterpret_cast<ShimRTPTransceiver*>(all_transceivers[i].get());
    }
    params->out_count = count;

    return SHIM_OK;
}

SHIM_EXPORT int shim_peer_connection_restart_ice(ShimPeerConnection* pc) {
    if (!pc || !pc->peer_connection) return SHIM_ERROR_INVALID_PARAM;
    pc->peer_connection->RestartIce();
    return SHIM_OK;
}

/* RTCStatsCollector callback for synchronous stats retrieval */
class StatsCollectorCallback : public webrtc::RTCStatsCollectorCallback {
public:
    std::mutex mutex;
    std::condition_variable cv;
    bool done = false;
    webrtc::scoped_refptr<const webrtc::RTCStatsReport> report;

    void OnStatsDelivered(const webrtc::scoped_refptr<const webrtc::RTCStatsReport>& r) override {
        std::lock_guard<std::mutex> lock(mutex);
        report = r;
        done = true;
        cv.notify_one();
    }
};

SHIM_EXPORT int shim_peer_connection_get_stats(ShimPeerConnectionGetStatsParams* params) {
    if (!params) {
        return SHIM_ERROR_INVALID_PARAM;
    }
    if (!params->pc || !params->pc->peer_connection) {
        memset(&params->out_stats, 0, sizeof(ShimRTCStats));
        return SHIM_ERROR_INVALID_PARAM;
    }

    memset(&params->out_stats, 0, sizeof(ShimRTCStats));

    // Create callback and get stats asynchronously
    auto callback = webrtc::make_ref_counted<StatsCollectorCallback>();
    params->pc->peer_connection->GetStats(callback.get());

    // Wait for completion
    {
        std::unique_lock<std::mutex> lock(callback->mutex);
        callback->cv.wait(lock, [&]() { return callback->done; });
    }

    if (!callback->report) {
        return SHIM_ERROR_INIT_FAILED;
    }

    params->out_stats.timestamp_us = webrtc::TimeMicros();

    // Iterate through all stats and aggregate them
    for (const auto& stat : *callback->report) {
        // Outbound RTP stream stats (sending)
        if (stat.type() == webrtc::RTCOutboundRtpStreamStats::kType) {
            const auto& outbound = stat.cast_to<webrtc::RTCOutboundRtpStreamStats>();
            if (outbound.bytes_sent.has_value()) {
                params->out_stats.bytes_sent += *outbound.bytes_sent;
            }
            if (outbound.packets_sent.has_value()) {
                params->out_stats.packets_sent += *outbound.packets_sent;
            }
            if (outbound.frames_encoded.has_value()) {
                params->out_stats.frames_encoded += *outbound.frames_encoded;
            }
            if (outbound.key_frames_encoded.has_value()) {
                params->out_stats.key_frames_encoded += *outbound.key_frames_encoded;
            }
            if (outbound.nack_count.has_value()) {
                params->out_stats.nack_count += *outbound.nack_count;
            }
            if (outbound.pli_count.has_value()) {
                params->out_stats.pli_count += *outbound.pli_count;
            }
            if (outbound.fir_count.has_value()) {
                params->out_stats.fir_count += *outbound.fir_count;
            }
            if (outbound.qp_sum.has_value()) {
                params->out_stats.qp_sum += *outbound.qp_sum;
            }
            if (outbound.quality_limitation_reason.has_value()) {
                const std::string& reason = *outbound.quality_limitation_reason;
                if (reason == "none") params->out_stats.quality_limitation_reason = SHIM_QUALITY_LIMITATION_NONE;
                else if (reason == "cpu") params->out_stats.quality_limitation_reason = SHIM_QUALITY_LIMITATION_CPU;
                else if (reason == "bandwidth") params->out_stats.quality_limitation_reason = SHIM_QUALITY_LIMITATION_BANDWIDTH;
                else params->out_stats.quality_limitation_reason = SHIM_QUALITY_LIMITATION_OTHER;
            }
        }

        // Inbound RTP stream stats (receiving) - includes jitter buffer stats
        if (stat.type() == webrtc::RTCInboundRtpStreamStats::kType) {
            const auto& inbound = stat.cast_to<webrtc::RTCInboundRtpStreamStats>();
            if (inbound.bytes_received.has_value()) {
                params->out_stats.bytes_received += *inbound.bytes_received;
            }
            if (inbound.packets_received.has_value()) {
                params->out_stats.packets_received += *inbound.packets_received;
            }
            if (inbound.packets_lost.has_value()) {
                params->out_stats.packets_lost += *inbound.packets_lost;
            }
            if (inbound.jitter.has_value()) {
                // jitter is in seconds, convert to ms
                params->out_stats.jitter_ms = *inbound.jitter * 1000.0;
            }
            if (inbound.frames_decoded.has_value()) {
                params->out_stats.frames_decoded += *inbound.frames_decoded;
            }
            if (inbound.key_frames_decoded.has_value()) {
                params->out_stats.key_frames_decoded += *inbound.key_frames_decoded;
            }
            if (inbound.frames_dropped.has_value()) {
                params->out_stats.frames_dropped += *inbound.frames_dropped;
            }
            if (inbound.nack_count.has_value()) {
                params->out_stats.nack_count += *inbound.nack_count;
            }
            if (inbound.pli_count.has_value()) {
                params->out_stats.pli_count += *inbound.pli_count;
            }
            if (inbound.fir_count.has_value()) {
                params->out_stats.fir_count += *inbound.fir_count;
            }
            if (inbound.qp_sum.has_value()) {
                params->out_stats.qp_sum += *inbound.qp_sum;
            }
            // Audio specific
            if (inbound.audio_level.has_value()) {
                params->out_stats.audio_level = *inbound.audio_level;
            }
            if (inbound.total_audio_energy.has_value()) {
                params->out_stats.total_audio_energy = *inbound.total_audio_energy;
            }
            if (inbound.concealment_events.has_value()) {
                params->out_stats.concealment_events += *inbound.concealment_events;
            }

            // JITTER BUFFER STATS (what the user specifically wants!)
            if (inbound.jitter_buffer_delay.has_value()) {
                // jitter_buffer_delay is total delay in seconds, convert to ms
                params->out_stats.jitter_buffer_delay_ms = *inbound.jitter_buffer_delay * 1000.0;
            }
            if (inbound.jitter_buffer_target_delay.has_value()) {
                params->out_stats.jitter_buffer_target_delay_ms = *inbound.jitter_buffer_target_delay * 1000.0;
            }
            if (inbound.jitter_buffer_minimum_delay.has_value()) {
                params->out_stats.jitter_buffer_minimum_delay_ms = *inbound.jitter_buffer_minimum_delay * 1000.0;
            }
            if (inbound.jitter_buffer_emitted_count.has_value()) {
                params->out_stats.jitter_buffer_emitted_count = *inbound.jitter_buffer_emitted_count;
            }
        }

        // Remote inbound RTP stats (from RTCP receiver reports)
        if (stat.type() == webrtc::RTCRemoteInboundRtpStreamStats::kType) {
            const auto& remote = stat.cast_to<webrtc::RTCRemoteInboundRtpStreamStats>();
            if (remote.packets_lost.has_value()) {
                params->out_stats.remote_packets_lost = *remote.packets_lost;
            }
            if (remote.jitter.has_value()) {
                params->out_stats.remote_jitter_ms = *remote.jitter * 1000.0;
            }
            if (remote.round_trip_time.has_value()) {
                params->out_stats.remote_round_trip_time_ms = *remote.round_trip_time * 1000.0;
            }
        }

        // ICE candidate pair stats
        if (stat.type() == webrtc::RTCIceCandidatePairStats::kType) {
            const auto& pair = stat.cast_to<webrtc::RTCIceCandidatePairStats>();
            if (pair.current_round_trip_time.has_value()) {
                params->out_stats.current_rtt_ms = static_cast<int64_t>(*pair.current_round_trip_time * 1000.0);
            }
            if (pair.total_round_trip_time.has_value()) {
                params->out_stats.total_rtt_ms = static_cast<int64_t>(*pair.total_round_trip_time * 1000.0);
            }
            if (pair.responses_received.has_value()) {
                params->out_stats.responses_received = *pair.responses_received;
            }
            if (pair.available_outgoing_bitrate.has_value()) {
                params->out_stats.available_outgoing_bitrate = *pair.available_outgoing_bitrate;
            }
            if (pair.available_incoming_bitrate.has_value()) {
                params->out_stats.available_incoming_bitrate = *pair.available_incoming_bitrate;
            }
        }

        // Data channel stats
        if (stat.type() == webrtc::RTCDataChannelStats::kType) {
            const auto& dc = stat.cast_to<webrtc::RTCDataChannelStats>();
            if (dc.messages_sent.has_value()) {
                params->out_stats.messages_sent += *dc.messages_sent;
            }
            if (dc.messages_received.has_value()) {
                params->out_stats.messages_received += *dc.messages_received;
            }
            if (dc.bytes_sent.has_value()) {
                params->out_stats.bytes_sent_data_channel += *dc.bytes_sent;
            }
            if (dc.bytes_received.has_value()) {
                params->out_stats.bytes_received_data_channel += *dc.bytes_received;
            }
        }
    }

    // Calculate average RTT if we have data
    if (params->out_stats.responses_received > 0 && params->out_stats.total_rtt_ms > 0) {
        params->out_stats.round_trip_time_ms = static_cast<double>(params->out_stats.total_rtt_ms) /
                                        static_cast<double>(params->out_stats.responses_received);
    }

    return SHIM_OK;
}

/* ============================================================================
 * Bandwidth Estimation API
 * ========================================================================== */

SHIM_EXPORT void shim_peer_connection_set_on_bandwidth_estimate(ShimPeerConnectionSetOnBandwidthEstimateParams* params) {
    if (!params || !params->pc) {
        return;
    }
    // TODO: Wire up BWE callback from libwebrtc's BitrateController
    // This requires implementing a NetworkControllerObserver
    (void)params->callback;
    (void)params->ctx;
}

SHIM_EXPORT int shim_peer_connection_get_bandwidth_estimate(ShimPeerConnectionGetBandwidthEstimateParams* params) {
    if (!params) {
        return SHIM_ERROR_INVALID_PARAM;
    }
    if (!params->pc) {
        memset(&params->out_estimate, 0, sizeof(ShimBandwidthEstimate));
        return SHIM_ERROR_INVALID_PARAM;
    }

    memset(&params->out_estimate, 0, sizeof(ShimBandwidthEstimate));
    params->out_estimate.timestamp_us = webrtc::TimeMicros();

    // TODO: Get actual BWE from libwebrtc's transport controller
    // For now, return placeholder values
    params->out_estimate.target_bitrate_bps = 0;
    params->out_estimate.available_send_bps = 0;
    params->out_estimate.available_recv_bps = 0;
    params->out_estimate.pacing_rate_bps = 0;
    params->out_estimate.congestion_window = 0;
    params->out_estimate.loss_rate = 0.0;

    return SHIM_OK;
}

}  // extern "C"
