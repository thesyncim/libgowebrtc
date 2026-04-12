package pioncodec

import (
	"errors"
	"testing"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/internal/testutil"
	"github.com/thesyncim/libgowebrtc/pkg/decoder"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

func explicitOpusParams() webrtc.RTPCodecParameters {
	return webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeOpus,
			ClockRate: 48_000,
			Channels:  2,
		},
		PayloadType: 111,
	}
}

func TestNewAudioEncoderRequiresExplicitFactoryConfig(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  AudioFactoryConfig
	}{
		{
			name: "missing sample rate",
			cfg: AudioFactoryConfig{
				Channels: 2,
				Bitrate:  64_000,
			},
		},
		{
			name: "missing channels",
			cfg: AudioFactoryConfig{
				SampleRate: 48_000,
				Bitrate:    64_000,
			},
		},
		{
			name: "missing bitrate",
			cfg: AudioFactoryConfig{
				SampleRate: 48_000,
				Channels:   2,
			},
		},
	} {
		enc, err := NewAudioEncoder(explicitOpusParams(), tc.cfg)
		if !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("%s: NewAudioEncoder() error = %v, want %v", tc.name, err, ErrInvalidConfig)
		}
		if enc != nil {
			t.Fatalf("%s: NewAudioEncoder() = %v, want nil encoder", tc.name, enc)
		}
	}
}

func TestNewAudioEncoderAcceptsExplicitOpusConfig(t *testing.T) {
	testutil.SkipIfNoShim(t)

	enc, err := NewAudioEncoder(explicitOpusParams(), AudioFactoryConfig{
		SampleRate: 48_000,
		Channels:   2,
		Bitrate:    64_000,
	})
	if err != nil {
		t.Fatalf("NewAudioEncoder(Opus) error = %v", err)
	}
	defer enc.Close()
}

func TestNewAudioDecoderRequiresExplicitClockRateAndChannels(t *testing.T) {
	for _, tc := range []struct {
		name   string
		params webrtc.RTPCodecParameters
	}{
		{
			name: "missing clock rate",
			params: webrtc.RTPCodecParameters{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType: webrtc.MimeTypeOpus,
					Channels: 2,
				},
			},
		},
		{
			name: "missing channels",
			params: webrtc.RTPCodecParameters{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:  webrtc.MimeTypeOpus,
					ClockRate: 48_000,
				},
			},
		},
	} {
		dec, err := NewAudioDecoder(tc.params)
		if !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("%s: NewAudioDecoder() error = %v, want %v", tc.name, err, ErrInvalidConfig)
		}
		if dec != nil {
			t.Fatalf("%s: NewAudioDecoder() = %v, want nil decoder", tc.name, dec)
		}
	}
}

func TestNewAudioDecoderAcceptsExplicitOpusParams(t *testing.T) {
	testutil.SkipIfNoShim(t)

	dec, err := NewAudioDecoder(explicitOpusParams())
	if err != nil {
		t.Fatalf("NewAudioDecoder(Opus) error = %v", err)
	}
	defer dec.Close()
}

func TestNewVideoEncoderAcceptsExplicitH264HighProfile(t *testing.T) {
	testutil.SkipIfNoShim(t)

	enc, err := NewVideoEncoder(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeH264,
			ClockRate:   90000,
			SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=64001f",
		},
	}, VideoFactoryConfig{Width: 320, Height: 240, Bitrate: 400_000, FPS: 30, KeyInterval: 30})
	if err != nil {
		t.Fatalf("NewVideoEncoder(H264 high) error = %v", err)
	}
	defer enc.Close()
}

func TestNewVideoEncoderAcceptsVP9Profile0(t *testing.T) {
	testutil.SkipIfNoShim(t)

	enc, err := NewVideoEncoder(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeVP9,
			ClockRate:   90000,
			SDPFmtpLine: "profile-id=0",
		},
	}, VideoFactoryConfig{Width: 320, Height: 240, Bitrate: 400_000, FPS: 30, KeyInterval: 30})
	if err != nil {
		t.Fatalf("NewVideoEncoder(VP9 profile0) error = %v", err)
	}
	defer enc.Close()
}

func TestMultiVideoEncoderLazyCreationAndSwitch(t *testing.T) {
	testutil.SkipIfNoShim(t)

	codecs := CodecSetFromParameters([]webrtc.RTPCodecParameters{
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
			PayloadType: 120,
		},
	}).VideoCodecs()
	multi, err := NewMultiVideoEncoder(codecs, VideoFactoryConfig{
		Width: 320, Height: 240, Bitrate: 400_000, FPS: 30, KeyInterval: 30,
	})
	if err != nil {
		t.Fatalf("NewMultiVideoEncoder: %v", err)
	}
	defer multi.Close()

	if got := len(multi.encoders); got != 0 {
		t.Fatalf("initial encoder cache len = %d, want 0", got)
	}

	src := testutil.CreateTestVideoFrame(320, 240)
	// Leave headroom above raw I420 size for codec overhead on keyframes.
	dst := make([]byte, 320*240*2)

	first, err := encodeVideoUntilOutput(multi, src, dst, true)
	if err != nil {
		t.Fatalf("first EncodeInto: %v", err)
	}
	if first.CodecParameters.MimeType != webrtc.MimeTypeVP8 {
		t.Fatalf("first codec = %q, want VP8", first.CodecParameters.MimeType)
	}
	if got := len(multi.encoders); got != 1 {
		t.Fatalf("encoder cache len after first encode = %d, want 1", got)
	}

	if err := multi.SetCodec(codecs[1]); err != nil {
		t.Fatalf("SetCodec(H264) error = %v", err)
	}
	second, err := encodeVideoUntilOutput(multi, src, dst, true)
	if err != nil {
		t.Fatalf("second EncodeInto: %v", err)
	}
	if second.CodecParameters.MimeType != webrtc.MimeTypeH264 {
		t.Fatalf("second codec = %q, want H264", second.CodecParameters.MimeType)
	}
	if got := len(multi.encoders); got != 2 {
		t.Fatalf("encoder cache len after switch = %d, want 2", got)
	}
}

func TestMultiVideoDecoderCachesByCodecParameters(t *testing.T) {
	testutil.SkipIfNoShim(t)

	vp8Params := webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000},
		PayloadType:        96,
	}
	h264Params := webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeH264,
			ClockRate:   90000,
			SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
		},
		PayloadType: 106,
	}

	vp8Sample := mustEncodedVideoSample(t, vp8Params)
	h264Sample := mustEncodedVideoSample(t, h264Params)

	multi := NewMultiVideoDecoder()
	defer multi.Close()

	dst := frame.NewI420Frame(320, 240)
	if err := decodeVideoUntilOutput(multi, vp8Sample, dst); err != nil {
		t.Fatalf("decode VP8: %v", err)
	}
	if got := len(multi.decoders); got != 1 {
		t.Fatalf("decoder cache len after VP8 = %d, want 1", got)
	}

	if err := decodeVideoUntilOutput(multi, h264Sample, dst); err != nil {
		t.Fatalf("decode H264: %v", err)
	}
	if got := len(multi.decoders); got != 2 {
		t.Fatalf("decoder cache len after H264 = %d, want 2", got)
	}

	if multi.CurrentCodec().MimeType != webrtc.MimeTypeH264 {
		t.Fatalf("CurrentCodec().MimeType = %q, want H264", multi.CurrentCodec().MimeType)
	}
}

func encodeVideoUntilOutput(multi *MultiVideoEncoder, src *frame.VideoFrame, dst []byte, forceKeyframe bool) (EncodedVideoSample, error) {
	var lastErr error
	for i := 0; i < 8; i++ {
		sample, err := multi.EncodeInto(src, dst, forceKeyframe && i == 0)
		if err == nil {
			return sample, nil
		}
		if !errors.Is(err, ffi.ErrNeedMoreData) {
			return EncodedVideoSample{}, err
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = ffi.ErrNeedMoreData
	}
	return EncodedVideoSample{}, lastErr
}

func decodeVideoUntilOutput(multi *MultiVideoDecoder, sample EncodedVideoSample, dst *frame.VideoFrame) error {
	var lastErr error
	for i := 0; i < 4; i++ {
		err := multi.DecodeInto(sample, dst)
		if err == nil {
			return nil
		}
		if !errors.Is(err, decoder.ErrNeedMoreData) {
			return err
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = decoder.ErrNeedMoreData
	}
	return lastErr
}

func mustEncodedVideoSample(t testing.TB, params webrtc.RTPCodecParameters) EncodedVideoSample {
	t.Helper()

	enc, err := NewVideoEncoder(params, VideoFactoryConfig{
		Width: 320, Height: 240, Bitrate: 400_000, FPS: 30, KeyInterval: 30,
	})
	if err != nil {
		t.Fatalf("NewVideoEncoder: %v", err)
	}
	defer enc.Close()

	src := testutil.CreateTestVideoFrame(320, 240)
	dst := make([]byte, enc.MaxEncodedSize())

	for i := 0; i < 8; i++ {
		result, err := enc.EncodeInto(src, dst, i == 0)
		if err == nil {
			return EncodedVideoSample{
				Data:            append([]byte(nil), dst[:result.N]...),
				CodecParameters: params,
				PayloadType:     params.PayloadType,
				Timestamp:       src.PTS,
				IsKeyframe:      result.IsKeyframe,
			}
		}
		if !errors.Is(err, ffi.ErrNeedMoreData) {
			t.Fatalf("EncodeInto: %v", err)
		}
	}

	t.Fatal("EncodeInto never produced output")
	return EncodedVideoSample{}
}
