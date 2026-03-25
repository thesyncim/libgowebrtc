package pionsend

import (
	"testing"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/pioncodec"
	"github.com/thesyncim/libgowebrtc/pkg/track"
)

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

func TestDefaultCodecPreferencesFollowBrowserBehavior(t *testing.T) {
	t.Run("LayeredPrefersDDCapableCodecs", func(t *testing.T) {
		got := defaultCodecPreferences(VideoPublishConfig{
			Browser: pioncodec.BrowserChrome,
			SVC:     &codec.SVCConfig{Mode: codec.SVCModeL3T3_KEY},
		})
		if len(got) < 2 {
			t.Fatalf("len(defaultCodecPreferences) = %d, want at least 2", len(got))
		}
		if got[0].MimeType != webrtc.MimeTypeVP9 {
			t.Fatalf("defaultCodecPreferences()[0] = %q, want %q", got[0].MimeType, webrtc.MimeTypeVP9)
		}
		if got[1].MimeType != webrtc.MimeTypeAV1 {
			t.Fatalf("defaultCodecPreferences()[1] = %q, want %q", got[1].MimeType, webrtc.MimeTypeAV1)
		}
	})

	t.Run("SimulcastKeepsBrowserOrder", func(t *testing.T) {
		got := defaultCodecPreferences(VideoPublishConfig{
			Browser: pioncodec.BrowserChrome,
			SVC:     &codec.SVCConfig{Mode: codec.SVCModeS3T3},
		})
		if len(got) == 0 {
			t.Fatal("defaultCodecPreferences() returned no codecs")
		}
		if got[0].MimeType != webrtc.MimeTypeVP8 {
			t.Fatalf("defaultCodecPreferences()[0] = %q, want browser-shaped %q", got[0].MimeType, webrtc.MimeTypeVP8)
		}
	})
}

func TestDeriveEncodingConfigsForSimulcast(t *testing.T) {
	layers := deriveEncodingConfigs(VideoPublishConfig{
		Width:   1280,
		Height:  720,
		Bitrate: 700_000,
		SVC: &codec.SVCConfig{
			Mode: codec.SVCModeS3T3,
			Layers: []codec.SVCLayerConfig{
				{Active: true},
				{Active: false},
				{Active: true},
			},
		},
	})
	if len(layers) != 3 {
		t.Fatalf("len(deriveEncodingConfigs()) = %d, want 3", len(layers))
	}

	wantRIDs := []string{"q", "h", "f"}
	wantWidths := []int{320, 640, 1280}
	wantHeights := []int{180, 360, 720}
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
		if layers[i].SVC == nil || layers[i].SVC.Mode != codec.SVCModeL1T3 {
			t.Fatalf("layers[%d].SVC = %+v, want L1T3", i, layers[i].SVC)
		}
	}

	if layers[0].Bitrate >= layers[1].Bitrate || layers[1].Bitrate >= layers[2].Bitrate {
		t.Fatalf("bitrates = [%d %d %d], want strictly increasing low->high", layers[0].Bitrate, layers[1].Bitrate, layers[2].Bitrate)
	}
}
