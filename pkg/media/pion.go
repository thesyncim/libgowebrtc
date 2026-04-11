package media

import "github.com/pion/webrtc/v4"

// pionTrackProvider is an internal interface for tracks that can provide
// their underlying Pion TrackLocal. This is not part of the public API.
type pionTrackProvider interface {
	pionTrack() webrtc.TrackLocal
}

// TrackLocal extracts the underlying Pion TrackLocal from a MediaStreamTrack.
// This is the raw escape hatch for callers who want direct Pion integration.
//
// Returns (nil, false) if the track doesn't support Pion integration.
//
// Example usage:
//
//	track := stream.GetVideoTracks()[0]
//	if pionTrack, ok := media.TrackLocal(track); ok {
//	    pc.AddTrack(pionTrack)
//	}
func TrackLocal(t MediaStreamTrack) (webrtc.TrackLocal, bool) {
	if p, ok := t.(pionTrackProvider); ok {
		return p.pionTrack(), true
	}
	return nil, false
}
