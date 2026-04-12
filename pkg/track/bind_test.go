package track

import (
	"errors"
	"testing"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/internal/testutil"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/encoder"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

type collectingWriter struct {
	err    error
	writes [][]byte
}

func (w *collectingWriter) WriteRTP(header *rtp.Header, payload []byte) (int, error) {
	pkt := &rtp.Packet{Header: *header, Payload: payload}
	buf, err := pkt.Marshal()
	if err != nil {
		return 0, err
	}
	return w.Write(buf)
}

func (w *collectingWriter) Write(b []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	copyBuf := append([]byte(nil), b...)
	w.writes = append(w.writes, copyBuf)
	return len(b), nil
}

type fakeTrackContext struct {
	id     string
	ssrc   webrtc.SSRC
	codecs []webrtc.RTPCodecParameters
	writer webrtc.TrackLocalWriter
}

func (c *fakeTrackContext) CodecParameters() []webrtc.RTPCodecParameters {
	return c.codecs
}

func (c *fakeTrackContext) HeaderExtensions() []webrtc.RTPHeaderExtensionParameter {
	return nil
}

func (c *fakeTrackContext) SSRC() webrtc.SSRC {
	return c.ssrc
}

func (c *fakeTrackContext) SSRCRetransmission() webrtc.SSRC {
	return 0
}

func (c *fakeTrackContext) SSRCForwardErrorCorrection() webrtc.SSRC {
	return 0
}

func (c *fakeTrackContext) WriteStream() webrtc.TrackLocalWriter {
	return c.writer
}

func (c *fakeTrackContext) ID() string {
	return c.id
}

func (c *fakeTrackContext) RTCPReader() interceptor.RTCPReader {
	return nil
}

func newVideoContext(writer webrtc.TrackLocalWriter, codecType codec.Type, payloadType uint8) webrtc.TrackLocalContext {
	return &fakeTrackContext{
		id:   "ctx-video",
		ssrc: 1234,
		codecs: []webrtc.RTPCodecParameters{{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  codecType.MimeType(),
				ClockRate: codecType.ClockRate(),
			},
			PayloadType: webrtc.PayloadType(payloadType),
		}},
		writer: writer,
	}
}

func newAudioContext(writer webrtc.TrackLocalWriter, payloadType uint8) webrtc.TrackLocalContext {
	return &fakeTrackContext{
		id:   "ctx-audio",
		ssrc: 5678,
		codecs: []webrtc.RTPCodecParameters{{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  codec.Opus.MimeType(),
				ClockRate: 48_000,
				Channels:  2,
			},
			PayloadType: webrtc.PayloadType(payloadType),
		}},
		writer: writer,
	}
}

func newVideoPreferenceContext(writer webrtc.TrackLocalWriter, codecs []webrtc.RTPCodecParameters) webrtc.TrackLocalContext {
	return &fakeTrackContext{
		id:     "ctx-video-preferences",
		ssrc:   4321,
		codecs: codecs,
		writer: writer,
	}
}

func encodeH264ForTrackTest(t *testing.T) []byte {
	t.Helper()
	cfg := codec.DefaultH264Config(320, 240)
	cfg.PreferHW = false
	enc, err := encoder.NewH264Encoder(cfg)
	if err != nil {
		t.Fatalf("NewH264Encoder: %v", err)
	}
	defer enc.Close()

	dst := make([]byte, enc.MaxEncodedSize())
	result, err := enc.EncodeInto(testutil.CreateTestVideoFrame(320, 240), dst, true)
	if err != nil {
		t.Fatalf("EncodeInto: %v", err)
	}
	return append([]byte(nil), dst[:result.N]...)
}

func encodeVP8ForTrackTest(t *testing.T) []byte {
	t.Helper()
	cfg := codec.DefaultVP8Config(320, 240)
	cfg.PreferHW = false
	enc, err := encoder.NewVP8Encoder(cfg)
	if err != nil {
		t.Fatalf("NewVP8Encoder: %v", err)
	}
	defer enc.Close()

	dst := make([]byte, enc.MaxEncodedSize())
	result, err := enc.EncodeInto(testutil.CreateTestVideoFrame(320, 240), dst, true)
	if err != nil {
		t.Fatalf("EncodeInto: %v", err)
	}
	return append([]byte(nil), dst[:result.N]...)
}

func encodeOpusForTrackTest(t *testing.T) []byte {
	t.Helper()
	enc, err := encoder.NewOpusEncoder(codec.DefaultOpusConfig())
	if err != nil {
		t.Fatalf("NewOpusEncoder: %v", err)
	}
	defer enc.Close()

	dst := make([]byte, enc.MaxEncodedSize())
	for attempt := 0; attempt < 4; attempt++ {
		n, err := enc.EncodeInto(testutil.CreateTestAudioFrame(48_000, 2, 960), dst)
		if err != nil {
			t.Fatalf("EncodeInto: %v", err)
		}
		if n > 0 {
			return append([]byte(nil), dst[:n]...)
		}
	}
	t.Fatal("EncodeInto produced no Opus payload after retries")
	return nil
}

func TestVideoTrackBindWriteAndUnbind(t *testing.T) {
	defer testutil.WithSerialExecution(t)()
	testutil.SkipIfNoShim(t)

	track, err := NewVideoTrack(VideoTrackConfig{
		ID:      "video-bind",
		Codec:   codec.VP8,
		Width:   320,
		Height:  240,
		Bitrate: 400_000,
		FPS:     30,
		MTU:     1200,
	})
	if err != nil {
		t.Fatalf("NewVideoTrack: %v", err)
	}
	defer track.Close()

	writer := &collectingWriter{}
	ctx := newVideoContext(writer, codec.VP8, 96)

	params, err := track.Bind(ctx)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if params.MimeType != codec.VP8.MimeType() {
		t.Fatalf("Bind mime type = %q, want %q", params.MimeType, codec.VP8.MimeType())
	}

	if err := track.SetBitrate(500_000); err != nil {
		t.Fatalf("SetBitrate while bound: %v", err)
	}
	if err := track.SetFramerate(24); err != nil {
		t.Fatalf("SetFramerate while bound: %v", err)
	}

	track.RequestKeyFrame()
	if err := track.WriteFrame(testutil.CreateTestVideoFrame(320, 240), false); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	if len(writer.writes) == 0 {
		t.Fatal("WriteFrame did not write any RTP packets")
	}

	if err := track.WriteEncodedData(encodeVP8ForTrackTest(t), 12_345, true); err != nil {
		t.Fatalf("WriteEncodedData: %v", err)
	}
	if len(writer.writes) < 2 {
		t.Fatal("WriteEncodedData did not add RTP packets")
	}

	if err := track.Unbind(ctx); err != nil {
		t.Fatalf("Unbind: %v", err)
	}
	if err := track.WriteFrame(testutil.CreateTestVideoFrame(320, 240), false); err != ErrNotBound {
		t.Fatalf("WriteFrame after Unbind error = %v, want %v", err, ErrNotBound)
	}
}

func TestVideoTrackBindDoubleBindAndWriteRTPError(t *testing.T) {
	defer testutil.WithSerialExecution(t)()
	testutil.SkipIfNoShim(t)

	track, err := NewVideoTrack(VideoTrackConfig{
		ID:      "video-double-bind",
		Codec:   codec.VP8,
		Width:   320,
		Height:  240,
		Bitrate: 400_000,
		FPS:     30,
		MTU:     1200,
	})
	if err != nil {
		t.Fatalf("NewVideoTrack: %v", err)
	}
	defer track.Close()

	ctx := newVideoContext(&collectingWriter{}, codec.VP8, 97)
	if _, err := track.Bind(ctx); err != nil {
		t.Fatalf("first Bind: %v", err)
	}
	if _, err := track.Bind(ctx); err != ErrAlreadyBound {
		t.Fatalf("second Bind error = %v, want %v", err, ErrAlreadyBound)
	}
	if err := track.WriteFrame(nil, false); err != ErrNilVideoFrame {
		t.Fatalf("WriteFrame(nil) error = %v, want %v", err, ErrNilVideoFrame)
	}
	if err := track.WriteRTP(nil); err != ErrNilRTPPacket {
		t.Fatalf("WriteRTP(nil) error = %v, want %v", err, ErrNilRTPPacket)
	}

	track.writer = &collectingWriter{err: errors.New("write failure")}
	rtpPacket := &rtp.Packet{
		Header:  rtp.Header{Version: 2, PayloadType: 97, SequenceNumber: 1},
		Payload: []byte{0x01, 0x02, 0x03},
	}
	if err := track.WriteRTP(rtpPacket); err == nil {
		t.Fatal("WriteRTP should surface writer errors")
	}
}

func TestVideoTrackWriteFrameRejectsUnsupportedScaledInput(t *testing.T) {
	defer testutil.WithSerialExecution(t)()
	testutil.SkipIfNoShim(t)

	track, err := NewVideoTrack(VideoTrackConfig{
		ID:      "video-invalid-scaled",
		Codec:   codec.VP8,
		Width:   640,
		Height:  360,
		Bitrate: 400_000,
		FPS:     30,
		MTU:     1200,
	})
	if err != nil {
		t.Fatalf("NewVideoTrack: %v", err)
	}
	defer track.Close()

	writer := &collectingWriter{}
	ctx := newVideoContext(writer, codec.VP8, 98)
	if _, err := track.Bind(ctx); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if err := track.SetParameters(Parameters{Active: true, ScaleResolutionDownBy: 2.0}); err != nil {
		t.Fatalf("SetParameters(scale): %v", err)
	}

	src := frame.NewNV12Frame(640, 360)
	if err := track.WriteFrame(src, false); !errors.Is(err, encoder.ErrUnsupportedPixelFormat) {
		t.Fatalf("WriteFrame(NV12) error = %v, want %v", err, encoder.ErrUnsupportedPixelFormat)
	}
	if len(writer.writes) != 0 {
		t.Fatalf("len(writer.writes) = %d, want 0", len(writer.writes))
	}
}

func TestVideoTrackBindSelectsPreferredCodecFromPreferences(t *testing.T) {
	defer testutil.WithSerialExecution(t)()
	testutil.SkipIfNoShim(t)

	track, err := NewVideoTrack(VideoTrackConfig{
		ID:      "video-preset-bind",
		Codec:   codec.H264,
		Width:   320,
		Height:  240,
		Bitrate: 400_000,
		FPS:     30,
		MTU:     1200,
		CodecPreferences: []webrtc.RTPCodecParameters{
			{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:  webrtc.MimeTypeVP8,
					ClockRate: 90000,
				},
				PayloadType: 96,
			},
			{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:    webrtc.MimeTypeH264,
					ClockRate:   90000,
					SDPFmtpLine: "packetization-mode=1;profile-level-id=42e01f;level-asymmetry-allowed=1",
				},
				PayloadType: 106,
			},
		},
	})
	if err != nil {
		t.Fatalf("NewVideoTrack: %v", err)
	}
	defer track.Close()
	if track.codec != codec.H264 {
		t.Fatalf("track.codec before Bind = %v, want %v", track.codec, codec.H264)
	}

	writer := &collectingWriter{}
	ctx := newVideoPreferenceContext(writer, []webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeH264,
				ClockRate:   90000,
				SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
			},
			PayloadType: 106,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeVP8,
				ClockRate: 90000,
			},
			PayloadType: 96,
		},
	})

	params, err := track.Bind(ctx)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if params.MimeType != webrtc.MimeTypeVP8 {
		t.Fatalf("Bind mime type = %q, want %q", params.MimeType, webrtc.MimeTypeVP8)
	}
	if track.codec != codec.VP8 {
		t.Fatalf("track.codec after Bind = %v, want %v", track.codec, codec.VP8)
	}
	if err := track.WriteFrame(testutil.CreateTestVideoFrame(320, 240), true); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	if len(writer.writes) == 0 {
		t.Fatal("expected RTP writes after preset-backed bind")
	}
}

func TestVideoTrackSetParametersAndAdaptationMath(t *testing.T) {
	track, err := NewVideoTrack(VideoTrackConfig{
		ID:           "video-adapt",
		Codec:        codec.H264,
		Width:        640,
		Height:       480,
		Bitrate:      1_000_000,
		FPS:          30,
		MTU:          1200,
		MinBitrate:   150_000,
		MaxBitrate:   2_000_000,
		MinWidth:     160,
		MinHeight:    120,
		MinFramerate: 5,
		MaxFramerate: 30,
	})
	if err != nil {
		t.Fatalf("NewVideoTrack: %v", err)
	}
	defer track.Close()

	if err := track.SetParameters(Parameters{
		Active:                false,
		MaxBitrate:            700_000,
		MaxFramerate:          15,
		ScaleResolutionDownBy: 2.0,
	}); err != nil {
		t.Fatalf("SetParameters pause: %v", err)
	}
	if !track.paused.Load() {
		t.Fatal("track should be paused after Active=false")
	}

	if err := track.SetParameters(Parameters{
		Active:                true,
		MaxBitrate:            700_000,
		MaxFramerate:          15,
		ScaleResolutionDownBy: 2.0,
	}); err != nil {
		t.Fatalf("SetParameters resume: %v", err)
	}
	if track.paused.Load() {
		t.Fatal("track should not be paused after Active=true")
	}
	if track.scaleFactor != 2.0 {
		t.Fatalf("scaleFactor = %v, want 2.0", track.scaleFactor)
	}
	if track.scaledFrame == nil || track.scaledFrame.Width != 320 || track.scaledFrame.Height != 240 {
		t.Fatal("scaledFrame was not allocated for 2x downscale")
	}

	if got := track.calculateScale(2_000_000); got != 1.0 {
		t.Fatalf("calculateScale(high bitrate) = %v, want 1.0", got)
	}
	if got := track.calculateScale(40_000); got < 2.0 {
		t.Fatalf("calculateScale(low bitrate) = %v, want at least 2.0", got)
	}
	if got := track.calculateFramerate(2_000_000); got != 30 {
		t.Fatalf("calculateFramerate(high bitrate) = %v, want 30", got)
	}
	if got := track.calculateFramerate(1); got != 5 {
		t.Fatalf("calculateFramerate(low bitrate) = %v, want 5", got)
	}
}

func TestAudioTrackBindWriteAndUnbind(t *testing.T) {
	defer testutil.WithSerialExecution(t)()
	testutil.SkipIfNoShim(t)

	track, err := NewAudioTrack(AudioTrackConfig{
		ID:         "audio-bind",
		SampleRate: 48_000,
		Channels:   2,
		Bitrate:    64_000,
		MTU:        1200,
	})
	if err != nil {
		t.Fatalf("NewAudioTrack: %v", err)
	}
	defer track.Close()

	writer := &collectingWriter{}
	ctx := newAudioContext(writer, 111)

	params, err := track.Bind(ctx)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if params.MimeType != codec.Opus.MimeType() {
		t.Fatalf("Bind mime type = %q, want %q", params.MimeType, codec.Opus.MimeType())
	}

	if err := track.SetBitrate(96_000); err != nil {
		t.Fatalf("SetBitrate while bound: %v", err)
	}
	if err := track.WriteFrame(nil); err != ErrNilAudioFrame {
		t.Fatalf("WriteFrame(nil) error = %v, want %v", err, ErrNilAudioFrame)
	}
	beforeWrites := len(writer.writes)
	if err := track.WriteEncodedData(encodeOpusForTrackTest(t), 9_999); err != nil {
		t.Fatalf("WriteEncodedData: %v", err)
	}
	if len(writer.writes) == beforeWrites {
		t.Fatal("WriteEncodedData did not emit RTP packets")
	}

	if err := track.Unbind(ctx); err != nil {
		t.Fatalf("Unbind: %v", err)
	}
	if err := track.WriteFrame(testutil.CreateSilentAudioFrame(48_000, 2, 960)); err != ErrNotBound {
		t.Fatalf("WriteFrame after Unbind error = %v, want %v", err, ErrNotBound)
	}
}
