package media

import (
	"sync"
	"sync/atomic"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/pionrecv"
)

// RemoteTrack mirrors the browser ontrack event's track surface while keeping
// access to the live codec state coming from pionrecv.
type RemoteTrack interface {
	MediaStreamTrack
	StreamID() string
	RID() string
	Codec() codec.Type
	CodecParameters() webrtc.RTPCodecParameters
	PayloadType() webrtc.PayloadType
}

// RemoteVideoTrack is a browser-like remote video track that emits decoded
// frames from a Pion remote track.
type RemoteVideoTrack interface {
	RemoteTrack
	SetOnVideoFrame(func(*frame.VideoFrame)) error
	SetOnCodecChange(func(pionrecv.CodecChange))
	RequestKeyframe() error
	DecodedTrack() *pionrecv.DecodedTrack
}

// RemoteAudioTrack is a browser-like remote audio track that emits decoded
// frames from a Pion remote track.
type RemoteAudioTrack interface {
	RemoteTrack
	SetOnAudioFrame(func(*frame.AudioFrame)) error
	SetOnCodecChange(func(pionrecv.CodecChange))
	DecodedTrack() *pionrecv.DecodedTrack
}

// RemoteStreamRegistry groups incoming remote tracks into stable MediaStream
// instances keyed by their remote stream IDs, mirroring browser ontrack events.
type RemoteStreamRegistry struct {
	mu      sync.Mutex
	streams map[string]*MediaStream
}

// NewRemoteStreamRegistry creates a registry for browser-like remote streams.
func NewRemoteStreamRegistry() *RemoteStreamRegistry {
	return &RemoteStreamRegistry{
		streams: make(map[string]*MediaStream),
	}
}

// BindPionTrack binds a Pion OnTrack pair into a browser-like remote track and
// the stable MediaStreams it belongs to.
func (r *RemoteStreamRegistry) BindPionTrack(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver, opts ...pionrecv.Option) (RemoteTrack, []*MediaStream, error) {
	decoded, err := pionrecv.BindRemoteTrack(track, receiver, opts...)
	if err != nil {
		return nil, nil, err
	}
	return r.bindDecodedTrack(&decodedTrackAdapter{decoded: decoded})
}

func (r *RemoteStreamRegistry) bindDecodedTrack(decoded remoteDecodedTrack) (RemoteTrack, []*MediaStream, error) {
	source, err := newRemoteDecodedSource(decoded)
	if err != nil {
		return nil, nil, err
	}

	view := source.newTrack(decoded.ID())
	streams := r.streamsForTrack(view)
	source.start()
	return view, streams, nil
}

func (r *RemoteStreamRegistry) streamsForTrack(track MediaStreamTrack) []*MediaStream {
	remote, ok := track.(RemoteTrack)
	if !ok || remote.StreamID() == "" {
		return []*MediaStream{}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	stream, ok := r.streams[remote.StreamID()]
	if !ok {
		stream = newMediaStreamWithID(remote.StreamID())
		r.streams[remote.StreamID()] = stream
	}
	if stream.GetTrackByID(track.ID()) == nil {
		stream.AddTrack(track)
	}
	return []*MediaStream{stream}
}

type remoteDecodedTrack interface {
	ID() string
	StreamID() string
	RID() string
	Kind() webrtc.RTPCodecType
	Codec() codec.Type
	CodecParameters() webrtc.RTPCodecParameters
	PayloadType() webrtc.PayloadType
	RequestKeyframe() error
	SetOnVideoFrame(func(*frame.VideoFrame)) error
	SetOnAudioFrame(func(*frame.AudioFrame)) error
	SetOnCodecChange(func(pionrecv.CodecChange))
	Run() error
	Close() error
	RawDecodedTrack() *pionrecv.DecodedTrack
}

type decodedTrackAdapter struct {
	decoded *pionrecv.DecodedTrack
}

func (a *decodedTrackAdapter) ID() string                { return a.decoded.ID() }
func (a *decodedTrackAdapter) StreamID() string          { return a.decoded.StreamID() }
func (a *decodedTrackAdapter) RID() string               { return a.decoded.RID() }
func (a *decodedTrackAdapter) Kind() webrtc.RTPCodecType { return a.decoded.Kind() }
func (a *decodedTrackAdapter) Codec() codec.Type         { return a.decoded.Codec() }
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
func (a *decodedTrackAdapter) Run() error                              { return a.decoded.Run() }
func (a *decodedTrackAdapter) Close() error                            { return a.decoded.Close() }
func (a *decodedTrackAdapter) RawDecodedTrack() *pionrecv.DecodedTrack { return a.decoded }

type remoteDecodedSource struct {
	decoded remoteDecodedTrack

	mu      sync.Mutex
	views   map[*remoteTrackView]struct{}
	started bool
	closed  bool

	closeOnce sync.Once
}

func newRemoteDecodedSource(decoded remoteDecodedTrack) (*remoteDecodedSource, error) {
	s := &remoteDecodedSource{
		decoded: decoded,
		views:   make(map[*remoteTrackView]struct{}),
	}

	switch decoded.Kind() {
	case webrtc.RTPCodecTypeVideo:
		if err := decoded.SetOnVideoFrame(s.handleVideoFrame); err != nil {
			return nil, err
		}
	case webrtc.RTPCodecTypeAudio:
		if err := decoded.SetOnAudioFrame(s.handleAudioFrame); err != nil {
			return nil, err
		}
	default:
		return nil, pionrecv.ErrUnsupportedTrackKind
	}
	decoded.SetOnCodecChange(s.handleCodecChange)
	return s, nil
}

func (s *remoteDecodedSource) newTrack(id string) RemoteTrack {
	return s.wrapView(s.newView(id))
}

func (s *remoteDecodedSource) wrapView(view *remoteTrackView) RemoteTrack {
	switch s.decoded.Kind() {
	case webrtc.RTPCodecTypeVideo:
		return &remoteVideoTrackView{remoteTrackView: view}
	case webrtc.RTPCodecTypeAudio:
		return &remoteAudioTrackView{remoteTrackView: view}
	default:
		return view
	}
}

func (s *remoteDecodedSource) newView(id string) *remoteTrackView {
	if id == "" {
		id = generateID()
	}

	view := &remoteTrackView{
		source: s,
		id:     id,
		label:  s.decoded.ID(),
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

func (s *remoteDecodedSource) start() {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()

	go func() {
		_ = s.decoded.Run()
		s.end()
	}()
}

func (s *remoteDecodedSource) end() {
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
	s.closeDecoded()
}

func (s *remoteDecodedSource) detach(view *remoteTrackView) {
	s.mu.Lock()
	delete(s.views, view)
	remaining := len(s.views)
	closed := s.closed
	s.mu.Unlock()

	if remaining == 0 && !closed {
		s.closeDecoded()
	}
}

func (s *remoteDecodedSource) closeDecoded() {
	s.closeOnce.Do(func() {
		_ = s.decoded.Close()
	})
}

func (s *remoteDecodedSource) snapshotViews() []*remoteTrackView {
	s.mu.Lock()
	defer s.mu.Unlock()

	views := make([]*remoteTrackView, 0, len(s.views))
	for view := range s.views {
		views = append(views, view)
	}
	return views
}

func (s *remoteDecodedSource) handleVideoFrame(f *frame.VideoFrame) {
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

func (s *remoteDecodedSource) handleAudioFrame(f *frame.AudioFrame) {
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

func (s *remoteDecodedSource) handleCodecChange(change pionrecv.CodecChange) {
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
	source *remoteDecodedSource
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

func (t *remoteTrackView) ID() string         { return t.id }
func (t *remoteTrackView) Kind() string       { return t.source.decoded.Kind().String() }
func (t *remoteTrackView) Label() string      { return t.label }
func (t *remoteTrackView) Enabled() bool      { return t.enabled.Load() }
func (t *remoteTrackView) SetEnabled(v bool)  { t.enabled.Store(v) }
func (t *remoteTrackView) Muted() bool        { return t.muted.Load() }
func (t *remoteTrackView) ReadyState() string { return t.readyState.Load().(string) }

func (t *remoteTrackView) Stop() {
	t.stopOnce.Do(func() {
		t.readyState.Store("ended")
		t.source.detach(t)
	})
}

func (t *remoteTrackView) Clone() MediaStreamTrack {
	return t.source.newTrack(generateID())
}

func (t *remoteTrackView) StreamID() string  { return t.source.decoded.StreamID() }
func (t *remoteTrackView) RID() string       { return t.source.decoded.RID() }
func (t *remoteTrackView) Codec() codec.Type { return t.source.decoded.Codec() }
func (t *remoteTrackView) CodecParameters() webrtc.RTPCodecParameters {
	return t.source.decoded.CodecParameters()
}
func (t *remoteTrackView) PayloadType() webrtc.PayloadType { return t.source.decoded.PayloadType() }
func (t *remoteTrackView) SetOnCodecChange(handler func(pionrecv.CodecChange)) {
	t.handlerMu.Lock()
	defer t.handlerMu.Unlock()
	t.onCodecChange = handler
}

func (t *remoteTrackView) shouldDispatch() bool {
	return t.Enabled() && t.ReadyState() == "live"
}

var _ RemoteTrack = (*remoteTrackView)(nil)

type remoteVideoTrackView struct {
	*remoteTrackView
}

func (t *remoteVideoTrackView) SetOnVideoFrame(handler func(*frame.VideoFrame)) error {
	t.handlerMu.Lock()
	defer t.handlerMu.Unlock()
	t.onVideo = handler
	return nil
}

func (t *remoteVideoTrackView) RequestKeyframe() error {
	return t.source.decoded.RequestKeyframe()
}

func (t *remoteVideoTrackView) DecodedTrack() *pionrecv.DecodedTrack {
	return t.source.decoded.RawDecodedTrack()
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

func (t *remoteAudioTrackView) DecodedTrack() *pionrecv.DecodedTrack {
	return t.source.decoded.RawDecodedTrack()
}

var _ RemoteVideoTrack = (*remoteVideoTrackView)(nil)
var _ RemoteAudioTrack = (*remoteAudioTrackView)(nil)
