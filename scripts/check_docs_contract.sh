#!/usr/bin/env bash

set -euo pipefail

README_PATH="${1:-README.md}"
PC_API_PATH="pkg/pc/peerconnection.go"
SHIM_HEADER_PATH="shim/shim.h"

if [[ ! -f "$README_PATH" ]]; then
    echo "::error::README not found at $README_PATH"
    exit 1
fi

if [[ ! -f "$PC_API_PATH" ]]; then
    echo "::error::PeerConnection API file not found at $PC_API_PATH"
    exit 1
fi

if [[ ! -f "$SHIM_HEADER_PATH" ]]; then
    echo "::error::Shim header not found at $SHIM_HEADER_PATH"
    exit 1
fi

status=0

require_pattern() {
    local pattern="$1"
    local description="$2"
    local path="$3"
    if ! rg -q "$pattern" "$path"; then
        echo "::error::Missing contract marker in $path: $description"
        status=1
    fi
}

forbid_pattern() {
    local pattern="$1"
    local description="$2"
    local path="$3"
    if rg -n "$pattern" "$path" >/dev/null; then
        echo "::error::Unsupported API reference found in $path: $description"
        rg -n "$pattern" "$path" || true
        status=1
    fi
}

require_pattern "Supported Platform Tiers" "supported platform tier table" "$README_PATH"
require_pattern "pkg/diagnostics" "runtime diagnostics package" "$README_PATH"
require_pattern "ErrNotSupported" "unsupported feature contract" "$README_PATH"

forbid_pattern "SetJitterBufferTarget" "SetJitterBufferTarget" "$README_PATH"
forbid_pattern "SetJitterBufferBounds" "SetJitterBufferBounds" "$README_PATH"
forbid_pattern "SetAdaptiveJitterBuffer" "SetAdaptiveJitterBuffer" "$README_PATH"
forbid_pattern "GetJitterBufferStats" "GetJitterBufferStats" "$README_PATH"
forbid_pattern '`GetBandwidthEstimate\(\)`' "deprecated/nonexistent GetBandwidthEstimate() helper" "$README_PATH"

require_pattern "The current shim does not implement sender stats and returns ErrNotSupported." "RTPSender.GetStats doc contract" "$PC_API_PATH"
require_pattern "The current shim does not implement this surface and returns ErrNotSupported." "unsupported callback/bandwidth doc contract" "$PC_API_PATH"

require_pattern "sender statistics are implemented" "sender stats shim header contract" "$SHIM_HEADER_PATH"
require_pattern "bandwidth estimate callbacks are implemented" "bandwidth callback shim header contract" "$SHIM_HEADER_PATH"
require_pattern "bandwidth estimate retrieval is implemented" "bandwidth getter shim header contract" "$SHIM_HEADER_PATH"
require_pattern "RTCP feedback callbacks are implemented" "RTCP feedback shim header contract" "$SHIM_HEADER_PATH"

exit "$status"
