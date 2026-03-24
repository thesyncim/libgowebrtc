package media

import "github.com/pion/webrtc/v4"

// pionTrackProvider is an internal interface for tracks that can provide
// their underlying Pion TrackLocal. This is not part of the public API.
type pionTrackProvider interface {
	pionTrack() webrtc.TrackLocal
}

// PionTrackLocal extracts the underlying Pion TrackLocal from a MediaStreamTrack.
// This is an escape hatch for users who need direct Pion integration.
//
// Returns (nil, false) if the track doesn't support Pion integration.
//
// Example usage:
//
//	track := stream.GetVideoTracks()[0]
//	if pionTrack, ok := media.PionTrackLocal(track); ok {
//	    pc.AddTrack(pionTrack)
//	}
func PionTrackLocal(t MediaStreamTrack) (webrtc.TrackLocal, bool) {
	if p, ok := t.(pionTrackProvider); ok {
		return p.pionTrack(), true
	}
	return nil, false
}

// PionTrackLocalForStream scopes a media track to a MediaStream ID for Pion
// interop. This preserves browser-like msid semantics when callers want manual
// control instead of AddTracksToPionPeerConnection.
//
// Returns (nil, false) if stream is nil or the track does not expose a Pion
// TrackLocal.
func PionTrackLocalForStream(stream *MediaStream, t MediaStreamTrack) (webrtc.TrackLocal, bool) {
	if stream == nil {
		return nil, false
	}
	return pionTrackLocalWithStreamID(stream.ID(), t)
}

// AddTracksToPionPeerConnection is a convenience function that adds all tracks
// from a MediaStream to a Pion PeerConnection. Tracks that do not expose a
// Pion TrackLocal are skipped.
//
// Returns ErrNilPeerConnection or ErrNilMediaStream for nil inputs, otherwise
// the list of RTPSenders created or an error if any supported track fails to
// add.
func AddTracksToPionPeerConnection(pc *webrtc.PeerConnection, stream *MediaStream) ([]*webrtc.RTPSender, error) {
	if pc == nil {
		return nil, ErrNilPeerConnection
	}
	if stream == nil {
		return nil, ErrNilMediaStream
	}

	tracks := stream.GetTracks()
	senders := make([]*webrtc.RTPSender, 0, len(tracks))

	for _, t := range tracks {
		pionTrack, ok := PionTrackLocalForStream(stream, t)
		if !ok {
			// Skip tracks that don't support Pion integration
			continue
		}

		sender, err := pc.AddTrack(pionTrack)
		if err != nil {
			return senders, err
		}
		senders = append(senders, sender)
	}

	return senders, nil
}

func pionTrackLocalWithStreamID(streamID string, t MediaStreamTrack) (webrtc.TrackLocal, bool) {
	pionTrack, ok := PionTrackLocal(t)
	if !ok {
		return nil, false
	}
	return &streamScopedTrackLocal{
		base:     pionTrack,
		streamID: streamID,
	}, true
}

type streamScopedTrackLocal struct {
	base     webrtc.TrackLocal
	streamID string
}

func (t *streamScopedTrackLocal) Bind(ctx webrtc.TrackLocalContext) (webrtc.RTPCodecParameters, error) {
	return t.base.Bind(ctx)
}

func (t *streamScopedTrackLocal) Unbind(ctx webrtc.TrackLocalContext) error {
	return t.base.Unbind(ctx)
}

func (t *streamScopedTrackLocal) ID() string {
	return t.base.ID()
}

func (t *streamScopedTrackLocal) RID() string {
	return t.base.RID()
}

func (t *streamScopedTrackLocal) StreamID() string {
	return t.streamID
}

func (t *streamScopedTrackLocal) Kind() webrtc.RTPCodecType {
	return t.base.Kind()
}

var _ webrtc.TrackLocal = (*streamScopedTrackLocal)(nil)
