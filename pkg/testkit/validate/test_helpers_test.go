package validate

import (
	"reflect"
	"testing"
	"unsafe"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/media"
	"github.com/thesyncim/libgowebrtc/pkg/pionrecv"
)

func mustNewPionPeerConnection(t *testing.T) *webrtc.PeerConnection {
	t.Helper()

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}
	t.Cleanup(func() {
		_ = pc.Close()
	})
	return pc
}

func setUnexportedField(t *testing.T, target any, field string, value any) {
	t.Helper()

	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		t.Fatalf("target must be a non-nil pointer, got %T", target)
	}
	elem := rv.Elem()
	fv := elem.FieldByName(field)
	if !fv.IsValid() {
		t.Fatalf("field %q not found on %T", field, target)
	}

	reflect.NewAt(fv.Type(), unsafe.Pointer(fv.UnsafeAddr())).Elem().Set(reflect.ValueOf(value))
}

type fakeRemoteBaseTrack struct {
	id         string
	kind       string
	label      string
	streamID   string
	rid        string
	enabled    bool
	muted      bool
	readyState string
}

func (f *fakeRemoteBaseTrack) ID() string        { return f.id }
func (f *fakeRemoteBaseTrack) Kind() string      { return f.kind }
func (f *fakeRemoteBaseTrack) Label() string     { return f.label }
func (f *fakeRemoteBaseTrack) Enabled() bool     { return f.enabled }
func (f *fakeRemoteBaseTrack) SetEnabled(v bool) { f.enabled = v }
func (f *fakeRemoteBaseTrack) Muted() bool       { return f.muted }
func (f *fakeRemoteBaseTrack) ReadyState() string {
	if f.readyState == "" {
		return "live"
	}
	return f.readyState
}
func (f *fakeRemoteBaseTrack) Stop() {}
func (f *fakeRemoteBaseTrack) StreamID() string {
	return f.streamID
}
func (f *fakeRemoteBaseTrack) RID() string { return f.rid }

type fakeRemoteVideoTrack struct {
	fakeRemoteBaseTrack
	codecType     codec.Type
	codecParams   webrtc.RTPCodecParameters
	payloadType   webrtc.PayloadType
	onVideoFrame  func(*frame.VideoFrame)
	onCodecChange func(pionrecv.CodecChange)
}

func newFakeRemoteVideoTrack(id, streamID, rid string, codecType codec.Type, params webrtc.RTPCodecParameters) *fakeRemoteVideoTrack {
	return &fakeRemoteVideoTrack{
		fakeRemoteBaseTrack: fakeRemoteBaseTrack{
			id:       id,
			kind:     "video",
			label:    id,
			streamID: streamID,
			rid:      rid,
			enabled:  true,
		},
		codecType:   codecType,
		codecParams: params,
		payloadType: params.PayloadType,
	}
}

func (f *fakeRemoteVideoTrack) Clone() media.MediaStreamTrack {
	clone := *f
	clone.onVideoFrame = nil
	clone.onCodecChange = nil
	return &clone
}
func (f *fakeRemoteVideoTrack) SetOnVideoFrame(cb func(*frame.VideoFrame)) error {
	f.onVideoFrame = cb
	return nil
}
func (f *fakeRemoteVideoTrack) Codec() codec.Type { return f.codecType }
func (f *fakeRemoteVideoTrack) CodecParameters() webrtc.RTPCodecParameters {
	return f.codecParams
}
func (f *fakeRemoteVideoTrack) PayloadType() webrtc.PayloadType { return f.payloadType }
func (f *fakeRemoteVideoTrack) SetOnCodecChange(cb func(pionrecv.CodecChange)) {
	f.onCodecChange = cb
}
func (f *fakeRemoteVideoTrack) RequestKeyframe() error               { return nil }
func (f *fakeRemoteVideoTrack) DecodedTrack() *pionrecv.DecodedTrack { return nil }
func (f *fakeRemoteVideoTrack) emitFrame(frame *frame.VideoFrame) {
	if f.onVideoFrame != nil {
		f.onVideoFrame(frame)
	}
}
func (f *fakeRemoteVideoTrack) emitCodecChange(change pionrecv.CodecChange) {
	if f.onCodecChange != nil {
		f.onCodecChange(change)
	}
}

type fakeRemoteAudioTrack struct {
	fakeRemoteBaseTrack
	codecType     codec.Type
	codecParams   webrtc.RTPCodecParameters
	payloadType   webrtc.PayloadType
	onAudioFrame  func(*frame.AudioFrame)
	onCodecChange func(pionrecv.CodecChange)
}

func newFakeRemoteAudioTrack(id, streamID, rid string, codecType codec.Type, params webrtc.RTPCodecParameters) *fakeRemoteAudioTrack {
	return &fakeRemoteAudioTrack{
		fakeRemoteBaseTrack: fakeRemoteBaseTrack{
			id:       id,
			kind:     "audio",
			label:    id,
			streamID: streamID,
			rid:      rid,
			enabled:  true,
		},
		codecType:   codecType,
		codecParams: params,
		payloadType: params.PayloadType,
	}
}

func (f *fakeRemoteAudioTrack) Clone() media.MediaStreamTrack {
	clone := *f
	clone.onAudioFrame = nil
	clone.onCodecChange = nil
	return &clone
}
func (f *fakeRemoteAudioTrack) SetOnAudioFrame(cb func(*frame.AudioFrame)) error {
	f.onAudioFrame = cb
	return nil
}
func (f *fakeRemoteAudioTrack) Codec() codec.Type { return f.codecType }
func (f *fakeRemoteAudioTrack) CodecParameters() webrtc.RTPCodecParameters {
	return f.codecParams
}
func (f *fakeRemoteAudioTrack) PayloadType() webrtc.PayloadType { return f.payloadType }
func (f *fakeRemoteAudioTrack) SetOnCodecChange(cb func(pionrecv.CodecChange)) {
	f.onCodecChange = cb
}
func (f *fakeRemoteAudioTrack) DecodedTrack() *pionrecv.DecodedTrack { return nil }
func (f *fakeRemoteAudioTrack) emitFrame(frame *frame.AudioFrame) {
	if f.onAudioFrame != nil {
		f.onAudioFrame(frame)
	}
}
func (f *fakeRemoteAudioTrack) emitCodecChange(change pionrecv.CodecChange) {
	if f.onCodecChange != nil {
		f.onCodecChange(change)
	}
}

type failingRemoteVideoTrack struct {
	*fakeRemoteVideoTrack
	err error
}

func (f *failingRemoteVideoTrack) SetOnVideoFrame(func(*frame.VideoFrame)) error {
	return f.err
}

type failingRemoteAudioTrack struct {
	*fakeRemoteAudioTrack
	err error
}

func (f *failingRemoteAudioTrack) SetOnAudioFrame(func(*frame.AudioFrame)) error {
	return f.err
}
