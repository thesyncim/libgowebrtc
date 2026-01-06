/*
 * shim_data_channel.cc - DataChannel implementation
 *
 * Provides DataChannel callbacks and message handling.
 */

#include "shim_common.h"

#include <map>
#include <cstring>

#include "api/data_channel_interface.h"
#include "rtc_base/buffer.h"

/* ============================================================================
 * DataChannel Wrapper and Observer
 * ========================================================================== */

struct ShimDataChannelWrapper {
    // Raw pointer - PC owns the reference via its data_channels vector
    // We don't create a scoped_refptr here to avoid double ref count
    webrtc::DataChannelInterface* channel = nullptr;
    ShimOnDataChannelMessage on_message = nullptr;
    void* on_message_ctx = nullptr;
    ShimOnDataChannelOpen on_open = nullptr;
    void* on_open_ctx = nullptr;
    ShimOnDataChannelClose on_close = nullptr;
    void* on_close_ctx = nullptr;
};

class DataChannelObserverImpl : public webrtc::DataChannelObserver {
public:
    explicit DataChannelObserverImpl(ShimDataChannelWrapper* wrapper)
        : wrapper_(wrapper) {}

    void OnStateChange() override {
        if (!wrapper_) return;
        auto state = wrapper_->channel->state();
        if (state == webrtc::DataChannelInterface::kOpen && wrapper_->on_open) {
            wrapper_->on_open(wrapper_->on_open_ctx);
        } else if (state == webrtc::DataChannelInterface::kClosed && wrapper_->on_close) {
            wrapper_->on_close(wrapper_->on_close_ctx);
        }
    }

    void OnMessage(const webrtc::DataBuffer& buffer) override {
        if (wrapper_ && wrapper_->on_message) {
            wrapper_->on_message(
                wrapper_->on_message_ctx,
                buffer.data.data(),
                static_cast<int>(buffer.data.size()),
                buffer.binary ? 1 : 0
            );
        }
    }

    void OnBufferedAmountChange(uint64_t sent_data_size) override {}

private:
    ShimDataChannelWrapper* wrapper_;
};

/* ============================================================================
 * Global Registry for DataChannel Wrappers
 * ========================================================================== */

namespace {
    std::mutex g_dc_registry_mutex;
    std::map<webrtc::DataChannelInterface*, std::unique_ptr<ShimDataChannelWrapper>> g_dc_registry;
    std::map<ShimDataChannelWrapper*, std::unique_ptr<DataChannelObserverImpl>> g_dc_observers;
}

static ShimDataChannelWrapper* GetOrCreateWrapper(ShimDataChannel* dc) {
    if (!dc) return nullptr;

    auto channel = reinterpret_cast<webrtc::DataChannelInterface*>(dc);

    std::lock_guard<std::mutex> lock(g_dc_registry_mutex);
    auto it = g_dc_registry.find(channel);
    if (it != g_dc_registry.end()) {
        return it->second.get();
    }

    auto wrapper = std::make_unique<ShimDataChannelWrapper>();
    // Store raw pointer - PC owns the scoped_refptr in its data_channels vector
    // We don't create another scoped_refptr here to avoid incrementing ref count
    wrapper->channel = channel;
    auto* raw_wrapper = wrapper.get();

    auto observer = std::make_unique<DataChannelObserverImpl>(raw_wrapper);
    channel->RegisterObserver(observer.get());

    g_dc_observers[raw_wrapper] = std::move(observer);
    g_dc_registry[channel] = std::move(wrapper);

    return raw_wrapper;
}

/* ============================================================================
 * C API Implementation
 * ========================================================================== */

extern "C" {

SHIM_EXPORT void shim_data_channel_set_on_message(
    ShimDataChannel* dc,
    ShimOnDataChannelMessage callback,
    void* ctx
) {
    auto* wrapper = GetOrCreateWrapper(dc);
    if (wrapper) {
        wrapper->on_message = callback;
        wrapper->on_message_ctx = ctx;
    }
}

SHIM_EXPORT void shim_data_channel_set_on_open(
    ShimDataChannel* dc,
    ShimOnDataChannelOpen callback,
    void* ctx
) {
    auto* wrapper = GetOrCreateWrapper(dc);
    if (wrapper) {
        wrapper->on_open = callback;
        wrapper->on_open_ctx = ctx;
    }
}

SHIM_EXPORT void shim_data_channel_set_on_close(
    ShimDataChannel* dc,
    ShimOnDataChannelClose callback,
    void* ctx
) {
    auto* wrapper = GetOrCreateWrapper(dc);
    if (wrapper) {
        wrapper->on_close = callback;
        wrapper->on_close_ctx = ctx;
    }
}

SHIM_EXPORT int shim_data_channel_send(
    ShimDataChannel* dc,
    const uint8_t* data,
    int size,
    int is_binary
) {
    if (!dc || !data) return SHIM_ERROR_INVALID_PARAM;

    auto channel = reinterpret_cast<webrtc::DataChannelInterface*>(dc);

    webrtc::DataBuffer db(webrtc::CopyOnWriteBuffer(data, size), is_binary != 0);

    return channel->Send(db) ? SHIM_OK : SHIM_ERROR_INIT_FAILED;
}

SHIM_EXPORT const char* shim_data_channel_label(ShimDataChannel* dc) {
    if (!dc) return nullptr;
    auto channel = reinterpret_cast<webrtc::DataChannelInterface*>(dc);
    // Use thread-local buffer to avoid returning pointer to temporary
    static thread_local std::string label_buffer;
    label_buffer = channel->label();
    return label_buffer.c_str();
}

SHIM_EXPORT int shim_data_channel_ready_state(ShimDataChannel* dc) {
    if (!dc) return -1;
    auto channel = reinterpret_cast<webrtc::DataChannelInterface*>(dc);
    return static_cast<int>(channel->state());
}

SHIM_EXPORT void shim_data_channel_close(ShimDataChannel* dc) {
    if (dc) {
        auto channel = reinterpret_cast<webrtc::DataChannelInterface*>(dc);
        channel->Close();
    }
}

SHIM_EXPORT void shim_data_channel_destroy(ShimDataChannel* dc) {
    if (!dc) return;

    auto channel = reinterpret_cast<webrtc::DataChannelInterface*>(dc);

    std::lock_guard<std::mutex> lock(g_dc_registry_mutex);
    auto it = g_dc_registry.find(channel);
    if (it != g_dc_registry.end()) {
        auto* wrapper = it->second.get();
        g_dc_observers.erase(wrapper);
        g_dc_registry.erase(it);
    }
}

}  // extern "C"
