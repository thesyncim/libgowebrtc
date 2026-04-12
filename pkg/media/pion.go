package media

import "github.com/pion/webrtc/v4"

// pionTrackProvider is an internal interface for tracks that can provide
// their underlying Pion TrackLocal. This is not part of the public API.
type pionTrackProvider interface {
	pionTrack() webrtc.TrackLocal
}

// PionTrackLocal extracts the underlying Pion TrackLocal from a MediaStreamTrack.
// Use it when you want to hand the track to Pion directly and spell the
// stream ID at the AddTrack call site.
//
// Returns (nil, false) if the track doesn't support Pion integration.
//
// Example usage:
//
//	track := stream.GetVideoTracks()[0]
//	if pionTrack, ok := media.PionTrackLocal(track); ok {
//	    _, _ = pc.AddTrack(pionTrack, "stream-0")
//	}
func PionTrackLocal(t MediaStreamTrack) (webrtc.TrackLocal, bool) {
	if p, ok := t.(pionTrackProvider); ok {
		return p.pionTrack(), true
	}
	return nil, false
}
