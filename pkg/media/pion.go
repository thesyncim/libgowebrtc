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

// AddTracksToPC is a convenience function that adds all tracks from a MediaStream
// to a Pion PeerConnection.
//
// Returns the list of RTPSenders created, or an error if any track fails to add.
func AddTracksToPC(pc *webrtc.PeerConnection, stream *MediaStream) ([]*webrtc.RTPSender, error) {
	tracks := stream.GetTracks()
	senders := make([]*webrtc.RTPSender, 0, len(tracks))

	for _, t := range tracks {
		pionTrack, ok := PionTrackLocal(t)
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
