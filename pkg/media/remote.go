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
	StreamID() string    // StreamID returns the first associated remote MediaStream ID, if any.
	StreamIDs() []string // StreamIDs returns all associated remote MediaStream IDs.
	RID() string         // RID returns the RTP stream ID for simulcast/SVC tracks when available.
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

// RemoteTrackEvent mirrors the browser ontrack event payload after it has been
// normalized into browser-like MediaStream objects.
type RemoteTrackEvent struct {
	Track   RemoteTrack    // Track is the normalized browser-like remote track.
	Streams []*MediaStream // Streams are the stable MediaStream views associated with Track.
}

// PionTrackEvent is the browser-like remote track event for low-level Pion
// OnTrack handlers.
type PionTrackEvent struct {
	RemoteTrackEvent
	TrackRemote *webrtc.TrackRemote // TrackRemote is the original Pion TrackRemote.
	Receiver    *webrtc.RTPReceiver // Receiver is the original Pion RTPReceiver, when available.
}

// PCTrackEvent is the browser-like remote track event for native pkg/pc
// SetOnTrack handlers.
type PCTrackEvent struct {
	RemoteTrackEvent
	PCTrack  *pc.Track       // PCTrack is the original pkg/pc remote track wrapper.
	Receiver *pc.RTPReceiver // Receiver is the original pkg/pc RTPReceiver wrapper.
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
	return r.bindSource(&decodedTrackAdapter{decoded: decoded})
}

// BindDecodedTrack binds a caller-managed pionrecv.DecodedTrack into the same
// browser-like remote track and MediaStream model used by BindPionTrack.
func (r *RemoteStreamRegistry) BindDecodedTrack(decoded *pionrecv.DecodedTrack) (RemoteTrack, []*MediaStream, error) {
	if decoded == nil {
		return nil, nil, ErrTrackNotFound
	}
	return r.bindSource(&decodedTrackAdapter{decoded: decoded})
}

// BindPCTrack binds a libwebrtc-backed pkg/pc track event into the same
// browser-like remote track and MediaStream model used by BindPionTrack.
func (r *RemoteStreamRegistry) BindPCTrack(track *pc.Track, receiver *pc.RTPReceiver, streams []string) (RemoteTrack, []*MediaStream, error) {
	if track == nil {
		return nil, nil, ErrTrackNotFound
	}
	return r.bindSource(newPCTrackAdapter(track, receiver, streams))
}

// PionOnTrack adapts a low-level Pion OnTrack callback into the browser-like
// RemoteTrackEvent model used by this package. When a receiver transport is
// available it automatically enables PLI requests via pionrecv.WithRTCPWriter
// before applying any caller-provided options.
func (r *RemoteStreamRegistry) PionOnTrack(
	handler func(PionTrackEvent),
	onError func(error),
	opts ...pionrecv.Option,
) func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
	return func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		bindOpts := make([]pionrecv.Option, 0, len(opts)+1)
		if receiver != nil && receiver.Transport() != nil {
			bindOpts = append(bindOpts, pionrecv.WithRTCPWriter(receiver.Transport()))
		}
		bindOpts = append(bindOpts, opts...)

		remoteTrack, streams, err := r.BindPionTrack(track, receiver, bindOpts...)
		if err != nil {
			if onError != nil {
				onError(err)
			}
			return
		}
		if handler == nil {
			return
		}
		handler(PionTrackEvent{
			RemoteTrackEvent: RemoteTrackEvent{
				Track:   remoteTrack,
				Streams: append([]*MediaStream(nil), streams...),
			},
			TrackRemote: track,
			Receiver:    receiver,
		})
	}
}

// PCOnTrack adapts a pkg/pc SetOnTrack callback into the browser-like
// RemoteTrackEvent model used by this package.
func (r *RemoteStreamRegistry) PCOnTrack(
	handler func(PCTrackEvent),
	onError func(error),
) func(track *pc.Track, receiver *pc.RTPReceiver, streams []string) {
	return func(track *pc.Track, receiver *pc.RTPReceiver, streams []string) {
		remoteTrack, mediaStreams, err := r.BindPCTrack(track, receiver, streams)
		if err != nil {
			if onError != nil {
				onError(err)
			}
			return
		}
		if handler == nil {
			return
		}
		handler(PCTrackEvent{
			RemoteTrackEvent: RemoteTrackEvent{
				Track:   remoteTrack,
				Streams: append([]*MediaStream(nil), mediaStreams...),
			},
			PCTrack:  track,
			Receiver: receiver,
		})
	}
}

// Close stops all bound remote tracks and forgets the cached MediaStreams.
func (r *RemoteStreamRegistry) Close() {
	r.mu.Lock()
	streams := make([]*MediaStream, 0, len(r.streams))
	for _, stream := range r.streams {
		streams = append(streams, stream)
	}
	r.streams = make(map[string]*MediaStream)
	r.mu.Unlock()

	seenTracks := make(map[MediaStreamTrack]struct{})
	for _, stream := range streams {
		for _, track := range stream.GetTracks() {
			if _, ok := seenTracks[track]; ok {
				continue
			}
			seenTracks[track] = struct{}{}
			track.Stop()
		}
	}
}

func (r *RemoteStreamRegistry) bindSource(source remoteFrameTrack) (RemoteTrack, []*MediaStream, error) {
	input, err := newRemoteFrameSource(source)
	if err != nil {
		return nil, nil, err
	}

	view := input.newTrack(source.ID())
	streams := r.streamsForTrack(view)
	input.start()
	return view, streams, nil
}

func (r *RemoteStreamRegistry) streamsForTrack(track RemoteTrack) []*MediaStream {
	streamIDs := normalizeStreamIDs(track.StreamIDs())
	if len(streamIDs) == 0 {
		return []*MediaStream{}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	streams := make([]*MediaStream, 0, len(streamIDs))
	for _, streamID := range streamIDs {
		stream, ok := r.streams[streamID]
		if !ok {
			stream = newMediaStreamWithID(streamID)
			r.streams[streamID] = stream
		}
		if stream.GetTrackByID(track.ID()) == nil {
			stream.AddTrack(track)
		}
		streams = append(streams, stream)
	}
	return streams
}

type remoteFrameTrack interface {
	ID() string
	StreamIDs() []string
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

func (a *decodedTrackAdapter) ID() string          { return a.decoded.ID() }
func (a *decodedTrackAdapter) StreamIDs() []string { return singleStreamIDs(a.decoded.StreamID()) }
func (a *decodedTrackAdapter) RID() string         { return a.decoded.RID() }
func (a *decodedTrackAdapter) Kind() string        { return a.decoded.Kind().String() }
func (a *decodedTrackAdapter) Label() string       { return a.decoded.ID() }
func (a *decodedTrackAdapter) Codec() codec.Type   { return a.decoded.Codec() }
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
	streams  []string
}

func newPCTrackAdapter(track *pc.Track, receiver *pc.RTPReceiver, streams []string) *pcTrackAdapter {
	return &pcTrackAdapter{
		track:    track,
		receiver: receiver,
		streams:  normalizeStreamIDs(streams),
	}
}

func (a *pcTrackAdapter) ID() string          { return a.track.ID() }
func (a *pcTrackAdapter) StreamIDs() []string { return append([]string(nil), a.streams...) }
func (a *pcTrackAdapter) RID() string         { return "" }
func (a *pcTrackAdapter) Kind() string        { return a.track.Kind() }
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

func (s *remoteFrameSource) start() {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()

	if err := s.track.Start(s.end); err != nil {
		s.end()
	}
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

func (t *remoteTrackView) StreamID() string { return firstStreamID(t.StreamIDs()) }
func (t *remoteTrackView) StreamIDs() []string {
	return normalizeStreamIDs(t.source.track.StreamIDs())
}
func (t *remoteTrackView) RID() string { return t.source.track.RID() }

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

func normalizeStreamIDs(streamIDs []string) []string {
	if len(streamIDs) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(streamIDs))
	out := make([]string, 0, len(streamIDs))
	for _, streamID := range streamIDs {
		streamID = strings.TrimSpace(streamID)
		if streamID == "" {
			continue
		}
		if _, ok := seen[streamID]; ok {
			continue
		}
		seen[streamID] = struct{}{}
		out = append(out, streamID)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func singleStreamIDs(streamID string) []string {
	if streamID == "" {
		return nil
	}
	return []string{streamID}
}

func firstStreamID(streamIDs []string) string {
	if len(streamIDs) == 0 {
		return ""
	}
	return streamIDs[0]
}
