package pioncodec

import (
	"errors"
	"sync"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/decoder"
	"github.com/thesyncim/libgowebrtc/pkg/encoder"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

// EncodedVideoSample describes a Pion-shaped encoded video sample.
type EncodedVideoSample struct {
	Data            []byte                    // Data contains the encoded access unit bytes.
	CodecParameters webrtc.RTPCodecParameters // CodecParameters identifies the codec that produced Data.
	PayloadType     webrtc.PayloadType        // PayloadType is the RTP payload type associated with the sample.
	Timestamp       uint32                    // Timestamp is the RTP timestamp associated with the sample.
	IsKeyframe      bool                      // IsKeyframe reports whether the sample starts a keyframe.
}

// EncodedAudioSample describes a Pion-shaped encoded audio sample.
type EncodedAudioSample struct {
	Data            []byte                    // Data contains the encoded audio access unit bytes.
	CodecParameters webrtc.RTPCodecParameters // CodecParameters identifies the codec that produced Data.
	PayloadType     webrtc.PayloadType        // PayloadType is the RTP payload type associated with the sample.
}

// MultiVideoEncoder lazily creates and switches between multiple video encoders.
type MultiVideoEncoder struct {
	codecs  []webrtc.RTPCodecParameters
	cfg     VideoFactoryConfig
	current webrtc.RTPCodecParameters

	mu       sync.Mutex
	encoders map[string]encoder.VideoEncoder
}

// NewMultiVideoEncoder creates a switchable-one multicodec video encoder.
func NewMultiVideoEncoder(codecs []webrtc.RTPCodecParameters, cfg VideoFactoryConfig) (*MultiVideoEncoder, error) {
	if len(codecs) == 0 {
		return nil, ErrEmptyCodecList
	}
	normalized := make([]webrtc.RTPCodecParameters, len(codecs))
	for i, codec := range codecs {
		normalized[i] = normalizeCodecParameters(codec)
	}
	return &MultiVideoEncoder{
		codecs:   normalized,
		cfg:      cfg,
		current:  normalized[0],
		encoders: make(map[string]encoder.VideoEncoder),
	}, nil
}

// Codecs returns the configured codecs.
func (e *MultiVideoEncoder) Codecs() []webrtc.RTPCodecParameters {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]webrtc.RTPCodecParameters, len(e.codecs))
	copy(out, e.codecs)
	return out
}

// CurrentCodec returns the active codec parameters.
func (e *MultiVideoEncoder) CurrentCodec() webrtc.RTPCodecParameters {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.current
}

// SetCodec switches the active codec without discarding cached encoders.
func (e *MultiVideoEncoder) SetCodec(params webrtc.RTPCodecParameters) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	params = normalizeCodecParameters(params)
	for _, codec := range e.codecs {
		if codecParametersMatch(codec, params) {
			e.current = codec
			return nil
		}
	}
	return encoder.ErrUnsupportedCodec
}

// EncodeInto encodes with the currently selected codec.
func (e *MultiVideoEncoder) EncodeInto(src *frame.VideoFrame, dst []byte, forceKeyframe bool) (EncodedVideoSample, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	current, enc, err := e.currentVideoEncoderLocked()
	if err != nil {
		return EncodedVideoSample{}, err
	}

	result, err := enc.EncodeInto(src, dst, forceKeyframe)
	if err != nil {
		return EncodedVideoSample{}, err
	}
	return EncodedVideoSample{
		Data:            dst[:result.N],
		CodecParameters: current,
		PayloadType:     current.PayloadType,
		Timestamp:       src.PTS,
		IsKeyframe:      result.IsKeyframe,
	}, nil
}

// Close closes all cached encoders.
func (e *MultiVideoEncoder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	var firstErr error
	for key, enc := range e.encoders {
		if err := enc.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(e.encoders, key)
	}
	return firstErr
}

func (e *MultiVideoEncoder) currentVideoEncoderLocked() (webrtc.RTPCodecParameters, encoder.VideoEncoder, error) {
	key, err := codecCacheKey(e.current)
	if err != nil {
		return webrtc.RTPCodecParameters{}, nil, err
	}
	if enc, ok := e.encoders[key]; ok {
		return e.current, enc, nil
	}
	enc, err := NewVideoEncoder(e.current, e.cfg)
	if err != nil {
		return webrtc.RTPCodecParameters{}, nil, err
	}
	e.encoders[key] = enc
	return e.current, enc, nil
}

// MultiAudioEncoder lazily creates and switches between multiple audio encoders.
type MultiAudioEncoder struct {
	codecs  []webrtc.RTPCodecParameters
	cfg     AudioFactoryConfig
	current webrtc.RTPCodecParameters

	mu       sync.Mutex
	encoders map[string]encoder.AudioEncoder
}

// NewMultiAudioEncoder creates a switchable-one multicodec audio encoder.
func NewMultiAudioEncoder(codecs []webrtc.RTPCodecParameters, cfg AudioFactoryConfig) (*MultiAudioEncoder, error) {
	if len(codecs) == 0 {
		return nil, ErrEmptyCodecList
	}
	normalized := make([]webrtc.RTPCodecParameters, len(codecs))
	for i, codec := range codecs {
		normalized[i] = normalizeCodecParameters(codec)
	}
	return &MultiAudioEncoder{
		codecs:   normalized,
		cfg:      cfg,
		current:  normalized[0],
		encoders: make(map[string]encoder.AudioEncoder),
	}, nil
}

// SetCodec switches the active codec without discarding cached encoders.
func (e *MultiAudioEncoder) SetCodec(params webrtc.RTPCodecParameters) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	params = normalizeCodecParameters(params)
	for _, codec := range e.codecs {
		if codecParametersMatch(codec, params) {
			e.current = codec
			return nil
		}
	}
	return encoder.ErrUnsupportedCodec
}

// CurrentCodec returns the active codec parameters.
func (e *MultiAudioEncoder) CurrentCodec() webrtc.RTPCodecParameters {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.current
}

// EncodeInto encodes with the currently selected codec.
func (e *MultiAudioEncoder) EncodeInto(src *frame.AudioFrame, dst []byte) (EncodedAudioSample, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	current, enc, err := e.currentAudioEncoderLocked()
	if err != nil {
		return EncodedAudioSample{}, err
	}
	n, err := enc.EncodeInto(src, dst)
	if err != nil {
		return EncodedAudioSample{}, err
	}
	return EncodedAudioSample{
		Data:            dst[:n],
		CodecParameters: current,
		PayloadType:     current.PayloadType,
	}, nil
}

// Close closes all cached encoders.
func (e *MultiAudioEncoder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	var firstErr error
	for key, enc := range e.encoders {
		if err := enc.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(e.encoders, key)
	}
	return firstErr
}

func (e *MultiAudioEncoder) currentAudioEncoderLocked() (webrtc.RTPCodecParameters, encoder.AudioEncoder, error) {
	key, err := codecCacheKey(e.current)
	if err != nil {
		return webrtc.RTPCodecParameters{}, nil, err
	}
	if enc, ok := e.encoders[key]; ok {
		return e.current, enc, nil
	}
	enc, err := NewAudioEncoder(e.current, e.cfg)
	if err != nil {
		return webrtc.RTPCodecParameters{}, nil, err
	}
	e.encoders[key] = enc
	return e.current, enc, nil
}

// MultiVideoDecoder lazily creates and switches between multiple video decoders.
type MultiVideoDecoder struct {
	mu       sync.Mutex
	current  webrtc.RTPCodecParameters
	decoders map[string]decoder.VideoDecoder
}

// NewMultiVideoDecoder creates a multicodec video decoder.
func NewMultiVideoDecoder() *MultiVideoDecoder {
	return &MultiVideoDecoder{decoders: make(map[string]decoder.VideoDecoder)}
}

// CurrentCodec returns the most recently used codec parameters.
func (d *MultiVideoDecoder) CurrentCodec() webrtc.RTPCodecParameters {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.current
}

// DecodeInto decodes the sample with a cached decoder for its codec parameters.
func (d *MultiVideoDecoder) DecodeInto(sample EncodedVideoSample, dst *frame.VideoFrame) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	dec, params, err := d.videoDecoderLocked(sample.CodecParameters)
	if err != nil {
		return err
	}
	d.current = params
	return dec.DecodeInto(sample.Data, dst, sample.Timestamp, sample.IsKeyframe)
}

// Close closes all cached decoders.
func (d *MultiVideoDecoder) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	var firstErr error
	for key, dec := range d.decoders {
		if err := dec.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(d.decoders, key)
	}
	return firstErr
}

func (d *MultiVideoDecoder) videoDecoderLocked(params webrtc.RTPCodecParameters) (decoder.VideoDecoder, webrtc.RTPCodecParameters, error) {
	params = normalizeCodecParameters(params)
	key, err := codecCacheKey(params)
	if err != nil {
		return nil, webrtc.RTPCodecParameters{}, err
	}
	if dec, ok := d.decoders[key]; ok {
		return dec, params, nil
	}
	dec, err := NewVideoDecoder(params)
	if err != nil {
		return nil, webrtc.RTPCodecParameters{}, err
	}
	d.decoders[key] = dec
	return dec, params, nil
}

// MultiAudioDecoder lazily creates and switches between multiple audio decoders.
type MultiAudioDecoder struct {
	mu       sync.Mutex
	current  webrtc.RTPCodecParameters
	decoders map[string]decoder.AudioDecoder
}

// NewMultiAudioDecoder creates a multicodec audio decoder.
func NewMultiAudioDecoder() *MultiAudioDecoder {
	return &MultiAudioDecoder{decoders: make(map[string]decoder.AudioDecoder)}
}

// CurrentCodec returns the most recently used codec parameters.
func (d *MultiAudioDecoder) CurrentCodec() webrtc.RTPCodecParameters {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.current
}

// DecodeInto decodes the sample with a cached decoder for its codec parameters.
func (d *MultiAudioDecoder) DecodeInto(sample EncodedAudioSample, dst *frame.AudioFrame) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	dec, params, err := d.audioDecoderLocked(sample.CodecParameters)
	if err != nil {
		return 0, err
	}
	d.current = params
	return dec.DecodeInto(sample.Data, dst)
}

// Close closes all cached decoders.
func (d *MultiAudioDecoder) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	var firstErr error
	for key, dec := range d.decoders {
		if err := dec.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(d.decoders, key)
	}
	return firstErr
}

func (d *MultiAudioDecoder) audioDecoderLocked(params webrtc.RTPCodecParameters) (decoder.AudioDecoder, webrtc.RTPCodecParameters, error) {
	params = normalizeCodecParameters(params)
	key, err := codecCacheKey(params)
	if err != nil {
		return nil, webrtc.RTPCodecParameters{}, err
	}
	if dec, ok := d.decoders[key]; ok {
		return dec, params, nil
	}
	dec, err := NewAudioDecoder(params)
	if err != nil {
		return nil, webrtc.RTPCodecParameters{}, err
	}
	d.decoders[key] = dec
	return dec, params, nil
}

// ErrEmptyCodecList is returned when a multicodec wrapper is created without codecs.
var ErrEmptyCodecList = errors.New("empty codec list")
