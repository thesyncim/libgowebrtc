package media

import (
	"strings"
	"sync"
	"sync/atomic"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/pc"
	"github.com/thesyncim/libgowebrtc/pkg/pionrecv"
)

// RemoteTrack mirrors the browser ontrack event's track surface while keeping
// track-to-stream association available for higher-level routing.
type RemoteTrack interface {
	MediaStreamTrack
	StreamID() string // StreamID returns the associated remote MediaStream ID, if any.
	RID() string      // RID returns the RTP stream ID for simulcast/SVC tracks when available.
}

// RemoteVideoTrack is a browser-like remote video track that emits decoded
// frames regardless of whether the source is backed by Pion or libwebrtc.
type RemoteVideoTrack interface {
	RemoteTrack
	SetOnVideoFrame(func(*frame.VideoFrame)) error // SetOnVideoFrame installs the decoded video-frame callback.
}

// RemoteAudioTrack is a browser-like remote audio track that emits decoded
// frames regardless of whether the source is backed by Pion or libwebrtc.
type RemoteAudioTrack interface {
	RemoteTrack
	SetOnAudioFrame(func(*frame.AudioFrame)) error // SetOnAudioFrame installs the decoded audio-frame callback.
}

// RemoteCodecTrack exposes codec metadata for backends that can surface it.
// Today this is implemented by the Pion-backed remote track wrappers.
type RemoteCodecTrack interface {
	RemoteTrack
	Codec() codec.Type                          // Codec returns the normalized libgowebrtc codec type.
	CodecParameters() webrtc.RTPCodecParameters // CodecParameters returns the full negotiated RTP codec description.
	PayloadType() webrtc.PayloadType            // PayloadType returns the current RTP payload type.
}

// PionRemoteVideoTrack is the rich Pion-backed variant of RemoteVideoTrack.
type PionRemoteVideoTrack interface {
	RemoteVideoTrack
	RemoteCodecTrack
	SetOnCodecChange(func(pionrecv.CodecChange)) // SetOnCodecChange installs a callback for runtime codec switches.
	RequestKeyframe() error                      // RequestKeyframe asks the remote sender for a fresh keyframe.
	DecodedTrack() *pionrecv.DecodedTrack        // DecodedTrack exposes the underlying Pion bridge for advanced control.
}

// PionRemoteAudioTrack is the rich Pion-backed variant of RemoteAudioTrack.
type PionRemoteAudioTrack interface {
	RemoteAudioTrack
	RemoteCodecTrack
	SetOnCodecChange(func(pionrecv.CodecChange)) // SetOnCodecChange installs a callback for runtime codec switches.
	DecodedTrack() *pionrecv.DecodedTrack        // DecodedTrack exposes the underlying Pion bridge for advanced control.
}

// PCRemoteTrack exposes the underlying native libwebrtc receiver objects when
// callers need to drop down from the browser-like media layer.
type PCRemoteTrack interface {
	RemoteTrack
	PCTrack() *pc.Track          // PCTrack returns the underlying native libwebrtc track wrapper.
	PCReceiver() *pc.RTPReceiver // PCReceiver returns the underlying native libwebrtc receiver wrapper.
}

// PCRemoteVideoTrack is a browser-like remote video track backed by pkg/pc.
type PCRemoteVideoTrack interface {
	RemoteVideoTrack
	PCRemoteTrack
}

// PCRemoteAudioTrack is a browser-like remote audio track backed by pkg/pc.
type PCRemoteAudioTrack interface {
	RemoteAudioTrack
	PCRemoteTrack
}

// BindPionRemoteTrack binds a Pion OnTrack pair into a browser-like remote
// track wrapper without inventing stream-grouping state.
func BindPionRemoteTrack(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver, opts ...pionrecv.Option) (RemoteTrack, error) {
	decoded, err := pionrecv.BindRemoteTrack(track, receiver, opts...)
	if err != nil {
		return nil, err
	}
	return bindRemoteTrackSource(&decodedTrackAdapter{decoded: decoded})
}

// BindDecodedRemoteTrack binds a caller-managed pionrecv.DecodedTrack into the
// browser-like remote track wrapper used by BindPionRemoteTrack.
func BindDecodedRemoteTrack(decoded *pionrecv.DecodedTrack) (RemoteTrack, error) {
	if decoded == nil {
		return nil, ErrTrackNotFound
	}
	return bindRemoteTrackSource(&decodedTrackAdapter{decoded: decoded})
}

// BindPCRemoteTrack binds a libwebrtc-backed pkg/pc track into the same
// browser-like remote track wrapper used by BindPionRemoteTrack.
func BindPCRemoteTrack(track *pc.Track, receiver *pc.RTPReceiver, streamID string) (RemoteTrack, error) {
	if track == nil {
		return nil, ErrTrackNotFound
	}
	return bindRemoteTrackSource(newPCTrackAdapter(track, receiver, streamID))
}

func bindRemoteTrackSource(source remoteFrameTrack) (RemoteTrack, error) {
	input, err := newRemoteFrameSource(source)
	if err != nil {
		return nil, err
	}

	view := input.newTrack(source.ID())
	if err := input.start(); err != nil {
		return nil, err
	}
	return view, nil
}

type remoteFrameTrack interface {
	ID() string
	StreamID() string
	RID() string
	Kind() string
	Label() string
	SetOnVideoFrame(func(*frame.VideoFrame)) error
	SetOnAudioFrame(func(*frame.AudioFrame)) error
	Start(func()) error
	Close() error
}

type remoteCodecTrack interface {
	Codec() codec.Type
	CodecParameters() webrtc.RTPCodecParameters
	PayloadType() webrtc.PayloadType
}

type remoteCodecChangeTrack interface {
	SetOnCodecChange(func(pionrecv.CodecChange))
}

type remoteKeyframeTrack interface {
	RequestKeyframe() error
}

type remoteDecodedTrack interface {
	RawDecodedTrack() *pionrecv.DecodedTrack
}

type remotePCTrack interface {
	RawPCTrack() *pc.Track
	RawPCReceiver() *pc.RTPReceiver
}

type decodedTrackAdapter struct {
	decoded *pionrecv.DecodedTrack
}

func (a *decodedTrackAdapter) ID() string        { return a.decoded.ID() }
func (a *decodedTrackAdapter) StreamID() string  { return a.decoded.StreamID() }
func (a *decodedTrackAdapter) RID() string       { return a.decoded.RID() }
func (a *decodedTrackAdapter) Kind() string      { return a.decoded.Kind().String() }
func (a *decodedTrackAdapter) Label() string     { return a.decoded.ID() }
func (a *decodedTrackAdapter) Codec() codec.Type { return a.decoded.Codec() }
func (a *decodedTrackAdapter) CodecParameters() webrtc.RTPCodecParameters {
	return a.decoded.CodecParameters()
}
func (a *decodedTrackAdapter) PayloadType() webrtc.PayloadType { return a.decoded.PayloadType() }
func (a *decodedTrackAdapter) RequestKeyframe() error          { return a.decoded.RequestKeyframe() }
func (a *decodedTrackAdapter) SetOnVideoFrame(h func(*frame.VideoFrame)) error {
	return a.decoded.SetOnVideoFrame(h)
}
func (a *decodedTrackAdapter) SetOnAudioFrame(h func(*frame.AudioFrame)) error {
	return a.decoded.SetOnAudioFrame(h)
}
func (a *decodedTrackAdapter) SetOnCodecChange(h func(pionrecv.CodecChange)) {
	a.decoded.SetOnCodecChange(h)
}
func (a *decodedTrackAdapter) Start(onEnd func()) error {
	go func() {
		_ = a.decoded.Run()
		onEnd()
	}()
	return nil
}
func (a *decodedTrackAdapter) Close() error                            { return a.decoded.Close() }
func (a *decodedTrackAdapter) RawDecodedTrack() *pionrecv.DecodedTrack { return a.decoded }

type pcTrackAdapter struct {
	track    *pc.Track
	receiver *pc.RTPReceiver
	streamID string
}

func newPCTrackAdapter(track *pc.Track, receiver *pc.RTPReceiver, streamID string) *pcTrackAdapter {
	return &pcTrackAdapter{
		track:    track,
		receiver: receiver,
		streamID: strings.TrimSpace(streamID),
	}
}

func (a *pcTrackAdapter) ID() string       { return a.track.ID() }
func (a *pcTrackAdapter) StreamID() string { return a.streamID }
func (a *pcTrackAdapter) RID() string      { return "" }
func (a *pcTrackAdapter) Kind() string     { return a.track.Kind() }
func (a *pcTrackAdapter) Label() string {
	if label := a.track.Label(); label != "" {
		return label
	}
	return a.track.ID()
}
func (a *pcTrackAdapter) SetOnVideoFrame(h func(*frame.VideoFrame)) error {
	return a.track.SetOnVideoFrame(h)
}
func (a *pcTrackAdapter) SetOnAudioFrame(h func(*frame.AudioFrame)) error {
	return a.track.SetOnAudioFrame(h)
}
func (a *pcTrackAdapter) Start(func()) error { return nil }
func (a *pcTrackAdapter) Close() error {
	switch a.track.Kind() {
	case "video":
		return a.track.SetOnVideoFrame(nil)
	case "audio":
		return a.track.SetOnAudioFrame(nil)
	default:
		return nil
	}
}
func (a *pcTrackAdapter) RawPCTrack() *pc.Track          { return a.track }
func (a *pcTrackAdapter) RawPCReceiver() *pc.RTPReceiver { return a.receiver }

type remoteFrameSource struct {
	track remoteFrameTrack

	mu      sync.Mutex
	views   map[*remoteTrackView]struct{}
	started bool
	closed  bool

	closeOnce sync.Once
}

func newRemoteFrameSource(track remoteFrameTrack) (*remoteFrameSource, error) {
	s := &remoteFrameSource{
		track: track,
		views: make(map[*remoteTrackView]struct{}),
	}

	switch track.Kind() {
	case "video":
		if err := track.SetOnVideoFrame(s.handleVideoFrame); err != nil {
			return nil, err
		}
	case "audio":
		if err := track.SetOnAudioFrame(s.handleAudioFrame); err != nil {
			return nil, err
		}
	default:
		return nil, pionrecv.ErrUnsupportedTrackKind
	}

	if codecTrack, ok := track.(remoteCodecChangeTrack); ok {
		codecTrack.SetOnCodecChange(s.handleCodecChange)
	}

	return s, nil
}

func (s *remoteFrameSource) newTrack(id string) RemoteTrack {
	return s.wrapView(s.newView(id))
}

func (s *remoteFrameSource) wrapView(view *remoteTrackView) RemoteTrack {
	switch s.track.Kind() {
	case "video":
		base := &remoteVideoTrackView{remoteTrackView: view}
		switch {
		case supportsDecodedTrack(s.track):
			return &pionRemoteVideoTrackView{remoteVideoTrackView: base}
		case supportsPCTrack(s.track):
			return &pcRemoteVideoTrackView{remoteVideoTrackView: base}
		default:
			return base
		}
	case "audio":
		base := &remoteAudioTrackView{remoteTrackView: view}
		switch {
		case supportsDecodedTrack(s.track):
			return &pionRemoteAudioTrackView{remoteAudioTrackView: base}
		case supportsPCTrack(s.track):
			return &pcRemoteAudioTrackView{remoteAudioTrackView: base}
		default:
			return base
		}
	default:
		return view
	}
}

func (s *remoteFrameSource) newView(id string) *remoteTrackView {
	if id == "" {
		id = generateID()
	}

	view := &remoteTrackView{
		source: s,
		id:     id,
		label:  s.track.Label(),
	}
	view.enabled.Store(true)
	view.readyState.Store("live")

	s.mu.Lock()
	s.views[view] = struct{}{}
	closed := s.closed
	s.mu.Unlock()

	if closed {
		view.readyState.Store("ended")
	}

	return view
}

func (s *remoteFrameSource) start() error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return nil
	}
	s.started = true
	s.mu.Unlock()

	if err := s.track.Start(s.end); err != nil {
		s.end()
		return err
	}
	return nil
}

func (s *remoteFrameSource) end() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	views := make([]*remoteTrackView, 0, len(s.views))
	for view := range s.views {
		views = append(views, view)
	}
	s.mu.Unlock()

	for _, view := range views {
		view.readyState.Store("ended")
	}
	s.closeTrack()
}

func (s *remoteFrameSource) detach(view *remoteTrackView) {
	s.mu.Lock()
	delete(s.views, view)
	remaining := len(s.views)
	closed := s.closed
	if remaining == 0 && !closed {
		s.closed = true
	}
	s.mu.Unlock()

	if remaining == 0 && !closed {
		s.closeTrack()
	}
}

func (s *remoteFrameSource) closeTrack() {
	s.closeOnce.Do(func() {
		_ = s.track.Close()
	})
}

func (s *remoteFrameSource) snapshotViews() []*remoteTrackView {
	s.mu.Lock()
	defer s.mu.Unlock()

	views := make([]*remoteTrackView, 0, len(s.views))
	for view := range s.views {
		views = append(views, view)
	}
	return views
}

func (s *remoteFrameSource) handleVideoFrame(f *frame.VideoFrame) {
	if f == nil {
		return
	}
	for _, view := range s.snapshotViews() {
		if !view.shouldDispatch() {
			continue
		}
		view.handlerMu.RLock()
		handler := view.onVideo
		view.handlerMu.RUnlock()
		if handler != nil {
			handler(f.Clone())
		}
	}
}

func (s *remoteFrameSource) handleAudioFrame(f *frame.AudioFrame) {
	if f == nil {
		return
	}
	for _, view := range s.snapshotViews() {
		if !view.shouldDispatch() {
			continue
		}
		view.handlerMu.RLock()
		handler := view.onAudio
		view.handlerMu.RUnlock()
		if handler != nil {
			handler(f.Clone())
		}
	}
}

func (s *remoteFrameSource) handleCodecChange(change pionrecv.CodecChange) {
	for _, view := range s.snapshotViews() {
		if view.ReadyState() != "live" {
			continue
		}
		view.handlerMu.RLock()
		handler := view.onCodecChange
		view.handlerMu.RUnlock()
		if handler != nil {
			handler(change)
		}
	}
}

type remoteTrackView struct {
	source *remoteFrameSource
	id     string
	label  string

	enabled    atomic.Bool
	muted      atomic.Bool
	readyState atomic.Value

	handlerMu     sync.RWMutex
	onVideo       func(*frame.VideoFrame)
	onAudio       func(*frame.AudioFrame)
	onCodecChange func(pionrecv.CodecChange)

	stopOnce sync.Once
}

func (t *remoteTrackView) ID() string        { return t.id }
func (t *remoteTrackView) Kind() string      { return t.source.track.Kind() }
func (t *remoteTrackView) Label() string     { return t.label }
func (t *remoteTrackView) Enabled() bool     { return t.enabled.Load() }
func (t *remoteTrackView) SetEnabled(v bool) { t.enabled.Store(v) }
func (t *remoteTrackView) Muted() bool       { return t.muted.Load() }
func (t *remoteTrackView) ReadyState() string {
	return t.readyState.Load().(string)
}

func (t *remoteTrackView) Stop() {
	t.stopOnce.Do(func() {
		t.readyState.Store("ended")
		t.source.detach(t)
	})
}

func (t *remoteTrackView) Clone() MediaStreamTrack {
	clone := t.source.newTrack(generateID())
	if cloneView := remoteTrackViewFor(clone); cloneView != nil {
		cloneView.enabled.Store(t.enabled.Load())
		cloneView.muted.Store(t.muted.Load())
		if t.ReadyState() == "ended" {
			cloneView.Stop()
		}
	}
	return clone
}

func (t *remoteTrackView) StreamID() string { return strings.TrimSpace(t.source.track.StreamID()) }
func (t *remoteTrackView) RID() string      { return t.source.track.RID() }

func (t *remoteTrackView) shouldDispatch() bool {
	return t.Enabled() && t.ReadyState() == "live"
}

var _ RemoteTrack = (*remoteTrackView)(nil)

func remoteTrackViewFor(track MediaStreamTrack) *remoteTrackView {
	switch t := track.(type) {
	case *remoteTrackView:
		return t
	case *remoteVideoTrackView:
		return t.remoteTrackView
	case *remoteAudioTrackView:
		return t.remoteTrackView
	case *pionRemoteVideoTrackView:
		return t.remoteTrackView
	case *pionRemoteAudioTrackView:
		return t.remoteTrackView
	case *pcRemoteVideoTrackView:
		return t.remoteTrackView
	case *pcRemoteAudioTrackView:
		return t.remoteTrackView
	default:
		return nil
	}
}

type remoteVideoTrackView struct {
	*remoteTrackView
}

func (t *remoteVideoTrackView) SetOnVideoFrame(handler func(*frame.VideoFrame)) error {
	t.handlerMu.Lock()
	defer t.handlerMu.Unlock()
	t.onVideo = handler
	return nil
}

type remoteAudioTrackView struct {
	*remoteTrackView
}

func (t *remoteAudioTrackView) SetOnAudioFrame(handler func(*frame.AudioFrame)) error {
	t.handlerMu.Lock()
	defer t.handlerMu.Unlock()
	t.onAudio = handler
	return nil
}

type pionRemoteVideoTrackView struct {
	*remoteVideoTrackView
}

func (t *pionRemoteVideoTrackView) Codec() codec.Type {
	return t.source.track.(remoteCodecTrack).Codec()
}

func (t *pionRemoteVideoTrackView) CodecParameters() webrtc.RTPCodecParameters {
	return t.source.track.(remoteCodecTrack).CodecParameters()
}

func (t *pionRemoteVideoTrackView) PayloadType() webrtc.PayloadType {
	return t.source.track.(remoteCodecTrack).PayloadType()
}

func (t *pionRemoteVideoTrackView) SetOnCodecChange(handler func(pionrecv.CodecChange)) {
	t.handlerMu.Lock()
	defer t.handlerMu.Unlock()
	t.onCodecChange = handler
}

func (t *pionRemoteVideoTrackView) RequestKeyframe() error {
	return t.source.track.(remoteKeyframeTrack).RequestKeyframe()
}

func (t *pionRemoteVideoTrackView) DecodedTrack() *pionrecv.DecodedTrack {
	return t.source.track.(remoteDecodedTrack).RawDecodedTrack()
}

type pionRemoteAudioTrackView struct {
	*remoteAudioTrackView
}

func (t *pionRemoteAudioTrackView) Codec() codec.Type {
	return t.source.track.(remoteCodecTrack).Codec()
}

func (t *pionRemoteAudioTrackView) CodecParameters() webrtc.RTPCodecParameters {
	return t.source.track.(remoteCodecTrack).CodecParameters()
}

func (t *pionRemoteAudioTrackView) PayloadType() webrtc.PayloadType {
	return t.source.track.(remoteCodecTrack).PayloadType()
}

func (t *pionRemoteAudioTrackView) SetOnCodecChange(handler func(pionrecv.CodecChange)) {
	t.handlerMu.Lock()
	defer t.handlerMu.Unlock()
	t.onCodecChange = handler
}

func (t *pionRemoteAudioTrackView) DecodedTrack() *pionrecv.DecodedTrack {
	return t.source.track.(remoteDecodedTrack).RawDecodedTrack()
}

type pcRemoteVideoTrackView struct {
	*remoteVideoTrackView
}

func (t *pcRemoteVideoTrackView) PCTrack() *pc.Track {
	return t.source.track.(remotePCTrack).RawPCTrack()
}

func (t *pcRemoteVideoTrackView) PCReceiver() *pc.RTPReceiver {
	return t.source.track.(remotePCTrack).RawPCReceiver()
}

type pcRemoteAudioTrackView struct {
	*remoteAudioTrackView
}

func (t *pcRemoteAudioTrackView) PCTrack() *pc.Track {
	return t.source.track.(remotePCTrack).RawPCTrack()
}

func (t *pcRemoteAudioTrackView) PCReceiver() *pc.RTPReceiver {
	return t.source.track.(remotePCTrack).RawPCReceiver()
}

var _ RemoteVideoTrack = (*remoteVideoTrackView)(nil)
var _ RemoteAudioTrack = (*remoteAudioTrackView)(nil)
var _ PionRemoteVideoTrack = (*pionRemoteVideoTrackView)(nil)
var _ PionRemoteAudioTrack = (*pionRemoteAudioTrackView)(nil)
var _ PCRemoteVideoTrack = (*pcRemoteVideoTrackView)(nil)
var _ PCRemoteAudioTrack = (*pcRemoteAudioTrackView)(nil)

func supportsDecodedTrack(track remoteFrameTrack) bool {
	_, ok := track.(remoteDecodedTrack)
	return ok
}

func supportsPCTrack(track remoteFrameTrack) bool {
	_, ok := track.(remotePCTrack)
	return ok
}
