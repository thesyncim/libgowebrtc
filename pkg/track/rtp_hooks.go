package track

import (
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

// DependencyDescriptorRTPHeaderExtensionURI is the RTP header extension URI
// used for AV1/VP9 dependency descriptors.
const DependencyDescriptorRTPHeaderExtensionURI = "https://aomediacodec.github.io/av1-rtp-spec/#dependency-descriptor-rtp-header-extension"

// RTPPacketContext describes the negotiated RTP state for an outbound packet.
//
// Experimental: this surface is intended for send-side validation and custom
// RTP header extension work.
type RTPPacketContext struct {
	TrackID          string
	StreamID         string
	RID              string
	SSRC             webrtc.SSRC
	PayloadType      webrtc.PayloadType
	CodecParameters  webrtc.RTPCodecParameters
	HeaderExtensions []webrtc.RTPHeaderExtensionParameter
	PacketIndex      int
	PacketCount      int
}

// HeaderExtensionID returns the negotiated ID for the given RTP header
// extension URI.
func (c RTPPacketContext) HeaderExtensionID(uri string) (uint8, bool) {
	return headerExtensionIDFromParameters(c.HeaderExtensions, uri)
}

// RTPPacketMutator can inspect or modify a packet before it is written.
//
// Experimental: mutators should return quickly and avoid long blocking work.
type RTPPacketMutator func(pkt *rtp.Packet, ctx RTPPacketContext) error

// RTPPacketObserver receives a clone of the final RTP packet about to be sent.
//
// Experimental: observers should return quickly and avoid long blocking work.
type RTPPacketObserver func(pkt *rtp.Packet, ctx RTPPacketContext)

// SetRTPPacketMutator configures a send-side RTP mutator for this video track.
func (t *VideoTrack) SetRTPPacketMutator(mutator RTPPacketMutator) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.rtpPacketMutator = mutator
}

// SetOnRTPPacket configures a send-side RTP observer for this video track.
func (t *VideoTrack) SetOnRTPPacket(observer RTPPacketObserver) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.rtpPacketObs = observer
}

// RTPContext returns the current negotiated RTP context for this video track.
func (t *VideoTrack) RTPContext() (RTPPacketContext, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.rtpContextLocked(), t.writer != nil
}

// SetRTPPacketMutator configures a send-side RTP mutator for this audio track.
func (t *AudioTrack) SetRTPPacketMutator(mutator RTPPacketMutator) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.rtpPacketMutator = mutator
}

// SetOnRTPPacket configures a send-side RTP observer for this audio track.
func (t *AudioTrack) SetOnRTPPacket(observer RTPPacketObserver) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.rtpPacketObs = observer
}

// RTPContext returns the current negotiated RTP context for this audio track.
func (t *AudioTrack) RTPContext() (RTPPacketContext, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.rtpContextLocked(), t.writer != nil
}

func (t *VideoTrack) rtpContextLocked() RTPPacketContext {
	return RTPPacketContext{
		TrackID:          t.id,
		StreamID:         t.streamID,
		RID:              t.config.RID,
		SSRC:             t.ssrc,
		PayloadType:      t.payloadType,
		CodecParameters:  t.codecParams,
		HeaderExtensions: cloneRTPHeaderExtensions(t.headerExts),
	}
}

func (t *AudioTrack) rtpContextLocked() RTPPacketContext {
	return RTPPacketContext{
		TrackID:          t.id,
		StreamID:         t.streamID,
		SSRC:             t.ssrc,
		PayloadType:      t.payloadType,
		CodecParameters:  t.codecParams,
		HeaderExtensions: cloneRTPHeaderExtensions(t.headerExts),
	}
}

func cloneRTPHeaderExtensions(in []webrtc.RTPHeaderExtensionParameter) []webrtc.RTPHeaderExtensionParameter {
	if len(in) == 0 {
		return nil
	}
	out := make([]webrtc.RTPHeaderExtensionParameter, len(in))
	copy(out, in)
	return out
}

func cloneRTPPacket(pkt *rtp.Packet) *rtp.Packet {
	if pkt == nil {
		return nil
	}
	return pkt.Clone()
}

func headerExtensionIDFromParameters(exts []webrtc.RTPHeaderExtensionParameter, uri string) (uint8, bool) {
	for _, ext := range exts {
		if ext.URI == uri && ext.ID > 0 && ext.ID < 256 {
			return uint8(ext.ID), true
		}
	}
	return 0, false
}

func applyDependencyDescriptorExtension(pkt *rtp.Packet, extID uint8, payload []byte, packetCount int) error {
	if pkt == nil || extID == 0 || len(payload) == 0 {
		return nil
	}

	extPayload := payload
	if packetCount > 1 {
		var adjusted [256]byte
		copy(adjusted[:], payload)
		adjusted[0] = (adjusted[0] | 0x80) &^ 0x40
		extPayload = adjusted[:len(payload)]
	}
	return pkt.SetExtension(extID, extPayload)
}

func (t *VideoTrack) writePacketDataWithHooks(pktData []byte, packetIndex, packetCount int, dependencyDescriptorExtID uint8, dependencyDescriptor []byte) error {
	mutator := t.rtpPacketMutator
	observer := t.rtpPacketObs
	if mutator == nil && observer == nil && (packetIndex != 0 || dependencyDescriptorExtID == 0 || len(dependencyDescriptor) == 0) {
		_, err := t.writer.Write(pktData)
		return err
	}

	var pkt rtp.Packet
	if err := pkt.Unmarshal(pktData); err != nil {
		return err
	}

	ctx := t.rtpContextLocked()
	ctx.PacketIndex = packetIndex
	ctx.PacketCount = packetCount

	if packetIndex == 0 && dependencyDescriptorExtID != 0 && len(dependencyDescriptor) > 0 {
		if err := applyDependencyDescriptorExtension(&pkt, dependencyDescriptorExtID, dependencyDescriptor, packetCount); err != nil {
			return err
		}
	}

	if mutator != nil {
		if err := mutator(&pkt, ctx); err != nil {
			return err
		}
	}

	if _, err := t.writer.WriteRTP(&pkt.Header, pkt.Payload); err != nil {
		return err
	}

	if observer != nil {
		observer(cloneRTPPacket(&pkt), ctx)
	}
	return nil
}

func (t *AudioTrack) writePacketDataWithHooks(pktData []byte, packetIndex, packetCount int, dependencyDescriptorExtID uint8, dependencyDescriptor []byte) error {
	_ = dependencyDescriptorExtID
	_ = dependencyDescriptor
	mutator := t.rtpPacketMutator
	observer := t.rtpPacketObs
	if mutator == nil && observer == nil {
		_, err := t.writer.Write(pktData)
		return err
	}

	var pkt rtp.Packet
	if err := pkt.Unmarshal(pktData); err != nil {
		return err
	}

	ctx := t.rtpContextLocked()
	ctx.PacketIndex = packetIndex
	ctx.PacketCount = packetCount

	if mutator != nil {
		if err := mutator(&pkt, ctx); err != nil {
			return err
		}
	}

	if _, err := t.writer.WriteRTP(&pkt.Header, pkt.Payload); err != nil {
		return err
	}

	if observer != nil {
		observer(cloneRTPPacket(&pkt), ctx)
	}
	return nil
}

func (t *VideoTrack) writeRTPPacketWithHooks(pkt *rtp.Packet) error {
	mutator := t.rtpPacketMutator
	observer := t.rtpPacketObs
	if mutator == nil && observer == nil {
		buf, err := pkt.Marshal()
		if err != nil {
			return err
		}
		_, err = t.writer.Write(buf)
		return err
	}

	out := cloneRTPPacket(pkt)
	ctx := t.rtpContextLocked()
	ctx.PacketCount = 1

	if mutator != nil {
		if err := mutator(out, ctx); err != nil {
			return err
		}
	}
	if _, err := t.writer.WriteRTP(&out.Header, out.Payload); err != nil {
		return err
	}
	if observer != nil {
		observer(cloneRTPPacket(out), ctx)
	}
	return nil
}
