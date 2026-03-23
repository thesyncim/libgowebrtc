#!/usr/bin/env bash

set -euo pipefail

packages=(
  ./internal/ffi
  ./pkg/codec
  ./pkg/decoder
  ./pkg/depacketizer
  ./pkg/encoder
  ./pkg/frame
  ./pkg/media
  ./pkg/packetizer
  ./pkg/pc
  ./pkg/track
)

floors=(
  "github.com/thesyncim/libgowebrtc/internal/ffi:45"
  "github.com/thesyncim/libgowebrtc/pkg/codec:85"
  "github.com/thesyncim/libgowebrtc/pkg/decoder:80"
  "github.com/thesyncim/libgowebrtc/pkg/depacketizer:70"
  "github.com/thesyncim/libgowebrtc/pkg/encoder:70"
  "github.com/thesyncim/libgowebrtc/pkg/frame:60"
  "github.com/thesyncim/libgowebrtc/pkg/media:65"
  "github.com/thesyncim/libgowebrtc/pkg/packetizer:70"
  "github.com/thesyncim/libgowebrtc/pkg/pc:60"
  "github.com/thesyncim/libgowebrtc/pkg/track:60"
)

output="$(go test -cover "${packages[@]}")"
printf '%s\n' "$output"

status=0
for spec in "${floors[@]}"; do
  pkg="${spec%%:*}"
  want="${spec##*:}"
  line="$(printf '%s\n' "$output" | awk -v pkg="$pkg" '$2 == pkg { print; exit }')"
  if [[ -z "$line" ]]; then
    echo "::error::Missing coverage output for ${pkg}"
    status=1
    continue
  fi

  got="$(printf '%s\n' "$line" | sed -nE 's/.*coverage: ([0-9.]+)%.*/\1/p')"
  if [[ -z "$got" ]]; then
    echo "::error::Unable to parse coverage for ${pkg}: ${line}"
    status=1
    continue
  fi

  if awk -v got="$got" -v want="$want" 'BEGIN { exit !(got + 0 >= want + 0) }'; then
    echo "coverage ok: ${pkg} ${got}% >= ${want}%"
  else
    echo "::error::Coverage floor not met for ${pkg}: ${got}% < ${want}%"
    status=1
  fi
done

exit "$status"
