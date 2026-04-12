package pionsend

import (
	"testing"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/track"
)

func TestPublishAudioValidation(t *testing.T) {
	if _, err := PublishAudio(nil, AudioPublishConfig{TrackID: "audio"}); err != ErrNilPeerConnection {
		t.Fatalf("PublishAudio(nil) error = %v, want %v", err, ErrNilPeerConnection)
	}
	if _, err := PublishAudio(&webrtc.PeerConnection{}, AudioPublishConfig{}); err != ErrInvalidConfig {
		t.Fatalf("PublishAudio(empty cfg) error = %v, want %v", err, ErrInvalidConfig)
	}
}

func TestRequiredVideoHeaderExtensionURIs(t *testing.T) {
	t.Run("NoSVC", func(t *testing.T) {
		got := RequiredVideoHeaderExtensionURIs(VideoPublishConfig{})
		want := []string{midRTPHeaderExtensionURI}
		if len(got) != len(want) || got[0] != want[0] {
			t.Fatalf("RequiredVideoHeaderExtensionURIs() = %v, want %v", got, want)
		}
	})

	t.Run("LayeredSVC", func(t *testing.T) {
		got := RequiredVideoHeaderExtensionURIs(VideoPublishConfig{
			SVC: &codec.SVCConfig{Mode: codec.SVCModeL3T3_KEY},
		})
		want := []string{
			midRTPHeaderExtensionURI,
			track.DependencyDescriptorRTPHeaderExtensionURI,
		}
		if len(got) != len(want) {
			t.Fatalf("len(RequiredVideoHeaderExtensionURIs()) = %d, want %d", len(got), len(want))
		}
		for i, uri := range want {
			if got[i] != uri {
				t.Fatalf("uri[%d] = %q, want %q", i, got[i], uri)
			}
		}
	})

	t.Run("Simulcast", func(t *testing.T) {
		got := RequiredVideoHeaderExtensionURIs(VideoPublishConfig{
			SVC: &codec.SVCConfig{Mode: codec.SVCModeS3T3},
		})
		want := []string{
			midRTPHeaderExtensionURI,
			rtpStreamIDHeaderExtensionURI,
			repairedStreamIDHeaderExtensionURI,
		}
		if len(got) != len(want) {
			t.Fatalf("len(RequiredVideoHeaderExtensionURIs()) = %d, want %d", len(got), len(want))
		}
		for i, uri := range want {
			if got[i] != uri {
				t.Fatalf("uri[%d] = %q, want %q", i, got[i], uri)
			}
		}
	})
}

func TestCodecFromPreferencesRequiresRecognizedCodec(t *testing.T) {
	if got, ok := codecFromPreferences([]webrtc.RTPCodecParameters{{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: "video/unknown"},
	}}); ok {
		t.Fatalf("codecFromPreferences(unknown) = %v, want no match", got)
	}
	if got, ok := codecFromPreferences([]webrtc.RTPCodecParameters{{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP9},
	}}); !ok || got != codec.VP9 {
		t.Fatalf("codecFromPreferences(VP9) = (%v, %v), want (%v, true)", got, ok, codec.VP9)
	}
}

func TestDeriveEncodingConfigsForSimulcast(t *testing.T) {
	layers := deriveEncodingConfigs(VideoPublishConfig{
		Width:   1280,
		Height:  720,
		Bitrate: 700_000,
		SVC: &codec.SVCConfig{
			Mode: codec.SVCModeS3T3,
			Layers: []codec.SVCLayerConfig{
				{RID: "q", Width: 320, Height: 180, Bitrate: 120_000, Active: true},
				{RID: "h", Width: 640, Height: 360, Bitrate: 240_000, Active: false},
				{RID: "f", Width: 1280, Height: 720, Bitrate: 480_000, Active: true},
			},
		},
	})
	if len(layers) != 3 {
		t.Fatalf("len(deriveEncodingConfigs()) = %d, want 3", len(layers))
	}

	wantRIDs := []string{"q", "h", "f"}
	wantWidths := []int{320, 640, 1280}
	wantHeights := []int{180, 360, 720}
	wantBitrates := []uint32{120_000, 240_000, 480_000}
	wantActive := []bool{true, false, true}

	for i := range layers {
		if layers[i].RID != wantRIDs[i] {
			t.Fatalf("layers[%d].RID = %q, want %q", i, layers[i].RID, wantRIDs[i])
		}
		if layers[i].Width != wantWidths[i] || layers[i].Height != wantHeights[i] {
			t.Fatalf("layers[%d] size = %dx%d, want %dx%d", i, layers[i].Width, layers[i].Height, wantWidths[i], wantHeights[i])
		}
		if layers[i].Active != wantActive[i] {
			t.Fatalf("layers[%d].Active = %v, want %v", i, layers[i].Active, wantActive[i])
		}
		if layers[i].Bitrate != wantBitrates[i] {
			t.Fatalf("layers[%d].Bitrate = %d, want %d", i, layers[i].Bitrate, wantBitrates[i])
		}
		if layers[i].SVC == nil || layers[i].SVC.Mode != codec.SVCModeL1T3 {
			t.Fatalf("layers[%d].SVC = %+v, want L1T3", i, layers[i].SVC)
		}
	}
}

func TestDeriveEncodingConfigsForSimulcastRequiresExplicitLayerLayout(t *testing.T) {
	if layers := deriveEncodingConfigs(VideoPublishConfig{
		Width:   1280,
		Height:  720,
		Bitrate: 700_000,
		SVC: &codec.SVCConfig{
			Mode: codec.SVCModeS3T3,
			Layers: []codec.SVCLayerConfig{
				{RID: "q", Bitrate: 120_000},
				{RID: "h", Width: 640, Height: 360, Bitrate: 240_000},
				{RID: "f", Width: 1280, Height: 720, Bitrate: 480_000},
			},
		},
	}); layers != nil {
		t.Fatalf("deriveEncodingConfigs(missing simulcast layer layout) = %+v, want nil", layers)
	}
}
