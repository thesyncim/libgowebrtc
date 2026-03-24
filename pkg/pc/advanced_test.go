package pc

import (
	"errors"
	"strings"
	"testing"

	"github.com/thesyncim/libgowebrtc/internal/testutil"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/pioncodec"
)

func TestPeerConnectionWrapperStringers(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"datachannel connecting", DataChannelStateConnecting.String(), "connecting"},
		{"datachannel open", DataChannelStateOpen.String(), "open"},
		{"datachannel closing", DataChannelStateClosing.String(), "closing"},
		{"datachannel closed", DataChannelStateClosed.String(), "closed"},
		{"datachannel unknown", DataChannelState(99).String(), "unknown"},
		{"quality none", QualityLimitationNone.String(), "none"},
		{"quality cpu", QualityLimitationCPU.String(), "cpu"},
		{"quality bandwidth", QualityLimitationBandwidth.String(), "bandwidth"},
		{"quality other", QualityLimitationOther.String(), "other"},
		{"quality unknown", QualityLimitationReason(99).String(), "unknown"},
		{"transceiver sendrecv", TransceiverDirectionSendRecv.String(), "sendrecv"},
		{"transceiver sendonly", TransceiverDirectionSendOnly.String(), "sendonly"},
		{"transceiver recvonly", TransceiverDirectionRecvOnly.String(), "recvonly"},
		{"transceiver inactive", TransceiverDirectionInactive.String(), "inactive"},
		{"transceiver unknown", TransceiverDirection(99).String(), "unknown"},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Fatalf("%s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestSplitStreamIDs(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect []string
	}{
		{name: "empty", input: "", expect: nil},
		{name: "single", input: "stream-1", expect: []string{"stream-1"}},
		{name: "multiple", input: "stream-a,stream-b,stream-c", expect: []string{"stream-a", "stream-b", "stream-c"}},
		{name: "trim and skip empty", input: " stream-a, ,stream-b ,, stream-c ", expect: []string{"stream-a", "stream-b", "stream-c"}},
	}

	for _, tt := range tests {
		got := splitStreamIDs(tt.input)
		if len(got) != len(tt.expect) {
			t.Fatalf("%s len = %d, want %d", tt.name, len(got), len(tt.expect))
		}
		for i := range got {
			if got[i] != tt.expect[i] {
				t.Fatalf("%s[%d] = %q, want %q", tt.name, i, got[i], tt.expect[i])
			}
		}
	}
}

func TestStreamIDsForTrackID(t *testing.T) {
	const sdp = `v=0
o=- 1 2 IN IP4 127.0.0.1
s=-
t=0 0
a=msid-semantic: WMS stream-a stream-b
m=video 9 UDP/TLS/RTP/SAVPF 96
a=mid:0
a=msid:stream-a video-track
a=msid:stream-b video-track
a=msid:stream-b video-track
a=ssrc:123456 msid:stream-c audio-track
`

	videoStreams := streamIDsForTrackID(sdp, "video-track")
	if len(videoStreams) != 2 || videoStreams[0] != "stream-a" || videoStreams[1] != "stream-b" {
		t.Fatalf("streamIDsForTrackID(video-track) = %v, want [stream-a stream-b]", videoStreams)
	}

	audioStreams := streamIDsForTrackID(sdp, "audio-track")
	if len(audioStreams) != 1 || audioStreams[0] != "stream-c" {
		t.Fatalf("streamIDsForTrackID(audio-track) = %v, want [stream-c]", audioStreams)
	}

	if got := streamIDsForTrackID(sdp, "missing-track"); got != nil {
		t.Fatalf("streamIDsForTrackID(missing-track) = %v, want nil", got)
	}
}

func TestPeerConnectionRemoteTrackStreamIDs(t *testing.T) {
	pc := &PeerConnection{
		remoteDescription: &SessionDescription{
			SDP: "a=msid:stream-1 video-track\r\na=ssrc:42 msid:stream-2 audio-track\r\n",
		},
	}

	videoStreams := pc.remoteTrackStreamIDs("video-track")
	if len(videoStreams) != 1 || videoStreams[0] != "stream-1" {
		t.Fatalf("remoteTrackStreamIDs(video-track) = %v, want [stream-1]", videoStreams)
	}

	audioStreams := pc.remoteTrackStreamIDs("audio-track")
	if len(audioStreams) != 1 || audioStreams[0] != "stream-2" {
		t.Fatalf("remoteTrackStreamIDs(audio-track) = %v, want [stream-2]", audioStreams)
	}
}

func TestNormalizeTrackStreamIDs(t *testing.T) {
	got := normalizeTrackStreamIDs("track-1", []string{" stream-a ", "", "stream-b", "stream-a"})
	if len(got) != 2 || got[0] != "stream-a" || got[1] != "stream-b" {
		t.Fatalf("normalizeTrackStreamIDs(...) = %v, want [stream-a stream-b]", got)
	}

	got = normalizeTrackStreamIDs("track-1", nil)
	if len(got) != 1 || got[0] != "track-1" {
		t.Fatalf("normalizeTrackStreamIDs(nil) = %v, want [track-1]", got)
	}
}

func TestExpandLocalTrackStreamIDs(t *testing.T) {
	pc := &PeerConnection{
		senders: []*RTPSender{
			{
				track:   &Track{id: "video-track"},
				streams: []string{"stream-a", "stream-b"},
			},
			{
				track:   &Track{id: "audio-track"},
				streams: []string{"stream-c"},
			},
		},
	}

	const input = `v=0
o=- 1 2 IN IP4 127.0.0.1
s=-
t=0 0
a=group:BUNDLE 0 1
a=msid-semantic: WMS stream-a stream-c
m=video 9 UDP/TLS/RTP/SAVPF 96
a=mid:0
a=msid:stream-a video-track
a=ssrc:1234 msid:stream-a video-track
a=ssrc:5678 msid:stream-a video-track
m=audio 9 UDP/TLS/RTP/SAVPF 111
a=mid:1
a=msid:stream-c audio-track
`

	munged := pc.expandLocalTrackStreamIDs(input)
	if !strings.Contains(munged, "a=msid-semantic: WMS stream-a stream-b stream-c") {
		t.Fatalf("expanded SDP missing multi-stream msid-semantic line:\n%s", munged)
	}
	if !strings.Contains(munged, "a=msid:stream-b video-track") {
		t.Fatalf("expanded SDP missing extra a=msid line:\n%s", munged)
	}
	if !strings.Contains(munged, "a=ssrc:1234 msid:stream-b video-track") {
		t.Fatalf("expanded SDP missing first extra a=ssrc msid line:\n%s", munged)
	}
	if !strings.Contains(munged, "a=ssrc:5678 msid:stream-b video-track") {
		t.Fatalf("expanded SDP missing second extra a=ssrc msid line:\n%s", munged)
	}
	if strings.Count(munged, "a=msid:stream-c audio-track") != 1 {
		t.Fatalf("expanded SDP should not duplicate single-stream audio track:\n%s", munged)
	}
}

func TestCreateDataChannelValidatesOptionCombinations(t *testing.T) {
	pc := &PeerConnection{}

	maxPacketLifeTime := uint16(100)
	maxRetransmits := uint16(3)
	if _, err := pc.CreateDataChannel("invalid", &DataChannelInit{
		MaxPacketLifeTime: &maxPacketLifeTime,
		MaxRetransmits:    &maxRetransmits,
	}); err == nil {
		t.Fatal("CreateDataChannel() with both lifetime and retransmits = nil, want error")
	}

	negotiated := true
	if _, err := pc.CreateDataChannel("negotiated", &DataChannelInit{
		Negotiated: negotiated,
	}); err == nil {
		t.Fatal("CreateDataChannel() negotiated without ID = nil, want error")
	}
}

func TestSenderReceiverAndTransceiverGuardPaths(t *testing.T) {
	peerConnection := &PeerConnection{}
	if err := peerConnection.AddICECandidate(nil); err == nil {
		t.Fatal("AddICECandidate(nil) expected error")
	}
	if err := peerConnection.AddICECandidate(&ICECandidate{Candidate: "candidate:1"}); err != ErrPeerConnectionClosed {
		t.Fatalf("AddICECandidate() on zero-handle pc = %v, want ErrPeerConnectionClosed", err)
	}

	track := &Track{id: "track-1", kind: "video", label: "camera"}

	sender := &RTPSender{track: track}
	if sender.IsValid() {
		t.Fatal("zero-value sender should be invalid")
	}
	if sender.Track() != track {
		t.Fatal("Track() did not return stored track")
	}
	sender.streams = []string{"stream-a", "stream-b"}
	if got := sender.StreamIDs(); len(got) != 2 || got[0] != "stream-a" || got[1] != "stream-b" {
		t.Fatalf("StreamIDs() = %v, want [stream-a stream-b]", got)
	}
	if err := sender.ReplaceTrack(nil); err == nil {
		t.Fatal("ReplaceTrack on uninitialized sender expected error")
	}
	if err := sender.SetParameters(RTPSendParameters{}); err == nil {
		t.Fatal("SetParameters on uninitialized sender expected error")
	}
	if got := sender.GetParameters(); len(got.Encodings) != 0 {
		t.Fatalf("GetParameters() len = %d, want 0", len(got.Encodings))
	}
	if _, err := sender.GetStats(); err == nil {
		t.Fatal("GetStats on uninitialized sender expected error")
	}
	if err := sender.SetLayerActive("f", true); err == nil {
		t.Fatal("SetLayerActive on uninitialized sender expected error")
	}
	if err := sender.SetLayerBitrate("f", 200_000); err == nil {
		t.Fatal("SetLayerBitrate on uninitialized sender expected error")
	}
	if _, _, err := sender.GetActiveLayers(); err == nil {
		t.Fatal("GetActiveLayers on uninitialized sender expected error")
	}
	sender.SetOnRTCPFeedback(func(feedbackType RTCPFeedbackType, ssrc uint32) {})
	if err := sender.SetScalabilityMode("L1T2"); err == nil {
		t.Fatal("SetScalabilityMode on uninitialized sender expected error")
	}
	if _, err := sender.GetScalabilityMode(); err == nil {
		t.Fatal("GetScalabilityMode on uninitialized sender expected error")
	}
	if _, err := sender.GetNegotiatedCodecs(); err == nil {
		t.Fatal("GetNegotiatedCodecs on uninitialized sender expected error")
	}
	if err := sender.SetPreferredCodec("video/VP8", 96); err == nil {
		t.Fatal("SetPreferredCodec on uninitialized sender expected error")
	}

	receiver := &RTPReceiver{track: track}
	if receiver.IsValid() {
		t.Fatal("zero-value receiver should be invalid")
	}
	if receiver.Track() != track {
		t.Fatal("Track() did not return stored track")
	}
	if _, err := receiver.GetStats(); err == nil {
		t.Fatal("GetStats on uninitialized receiver expected error")
	}
	if err := receiver.SetJitterBufferMinDelay(50); err == nil {
		t.Fatal("SetJitterBufferMinDelay on uninitialized receiver expected error")
	}

	transceiver := &RTPTransceiver{
		sender:    sender,
		receiver:  receiver,
		direction: TransceiverDirectionRecvOnly,
		mid:       "mid-1",
		kind:      "video",
	}
	if transceiver.IsValid() {
		t.Fatal("zero-value transceiver should be invalid")
	}
	if transceiver.Sender() != sender {
		t.Fatal("Sender() did not return stored sender")
	}
	if transceiver.Receiver() != receiver {
		t.Fatal("Receiver() did not return stored receiver")
	}
	if got := transceiver.Direction(); got != TransceiverDirectionRecvOnly {
		t.Fatalf("Direction() = %v, want %v", got, TransceiverDirectionRecvOnly)
	}
	if err := transceiver.SetDirection(TransceiverDirectionSendOnly); err != nil {
		t.Fatalf("SetDirection() unexpected error: %v", err)
	}
	if got := transceiver.CurrentDirection(); got != TransceiverDirectionSendOnly {
		t.Fatalf("CurrentDirection() = %v, want %v", got, TransceiverDirectionSendOnly)
	}
	if got := transceiver.Mid(); got != "mid-1" {
		t.Fatalf("Mid() = %q, want %q", got, "mid-1")
	}
	if err := transceiver.Stop(); err != nil {
		t.Fatalf("Stop() unexpected error: %v", err)
	}
	if err := transceiver.SetCodecPreferences([]CodecCapability{{MimeType: "video/VP8", ClockRate: 90000}}); err == nil {
		t.Fatal("SetCodecPreferences on uninitialized transceiver expected error")
	}
	if err := transceiver.SetCodecSet(pioncodec.BrowserPreset(pioncodec.BrowserChrome, pioncodec.DirectionEncode, pioncodec.PresetModeSupported)); err == nil {
		t.Fatal("SetCodecSet on uninitialized transceiver expected error")
	}
}

func TestTrackAndDataChannelGuardPaths(t *testing.T) {
	video := &Track{id: "video-1", kind: "video", label: "camera"}
	if video.IsValid() {
		t.Fatal("zero-value video track should be invalid")
	}
	if got := video.ID(); got != "video-1" {
		t.Fatalf("ID() = %q, want %q", got, "video-1")
	}
	if got := video.Label(); got != "camera" {
		t.Fatalf("Label() = %q, want %q", got, "camera")
	}
	if got := video.Muted(); got {
		t.Fatal("Muted() = true, want false")
	}
	video.SetEnabled(false)
	if got := video.Enabled(); got {
		t.Fatal("Enabled() = true, want false")
	}
	if err := video.SetOnAudioFrame(func(*frame.AudioFrame) {}); err == nil {
		t.Fatal("SetOnAudioFrame on video track expected error")
	}
	if err := video.SetOnVideoFrame(func(*frame.VideoFrame) {}); err == nil {
		t.Fatal("SetOnVideoFrame without handle expected error")
	}
	if err := video.WriteVideoFrame(testutil.CreateTestVideoFrame(32, 32)); err != nil {
		t.Fatalf("WriteVideoFrame when disabled should be ignored, got %v", err)
	}
	video.SetEnabled(true)
	if err := video.WriteVideoFrame(testutil.CreateTestVideoFrame(32, 32)); err == nil {
		t.Fatal("WriteVideoFrame without source expected error")
	}
	video.sourceHandle = 1
	if err := video.WriteVideoFrame(&frame.VideoFrame{Format: frame.PixelFormatNV12}); err == nil {
		t.Fatal("WriteVideoFrame with unsupported format expected error")
	}
	if err := video.WriteVideoFrame(&frame.VideoFrame{Format: frame.PixelFormatI420}); err == nil {
		t.Fatal("WriteVideoFrame with missing planes expected error")
	}

	audio := &Track{id: "audio-1", kind: "audio", label: "mic"}
	audio.SetEnabled(false)
	if err := audio.WriteAudioFrame(frame.NewAudioFrameS16(48_000, 2, 960)); err != nil {
		t.Fatalf("WriteAudioFrame when disabled should be ignored, got %v", err)
	}
	audio.SetEnabled(true)
	if err := audio.SetOnVideoFrame(func(*frame.VideoFrame) {}); err == nil {
		t.Fatal("SetOnVideoFrame on audio track expected error")
	}
	if err := audio.SetOnAudioFrame(func(*frame.AudioFrame) {}); err == nil {
		t.Fatal("SetOnAudioFrame without handle expected error")
	}
	if err := audio.WriteAudioFrame(frame.NewAudioFrameS16(48_000, 2, 960)); err == nil {
		t.Fatal("WriteAudioFrame without source expected error")
	}
	audio.sourceHandle = 1
	if err := audio.WriteAudioFrame(frame.NewAudioFrameF32(48_000, 2, 960)); err == nil {
		t.Fatal("WriteAudioFrame with unsupported sample format expected error")
	}

	dataChannel := &DataChannel{label: "control", id: 7}
	if dataChannel.IsValid() {
		t.Fatal("zero-value data channel should be invalid")
	}
	if got := dataChannel.Label(); got != "control" {
		t.Fatalf("Label() = %q, want %q", got, "control")
	}
	if got := dataChannel.ID(); got != 7 {
		t.Fatalf("ID() = %d, want 7", got)
	}
	if got := dataChannel.ReadyState(); got != DataChannelStateClosed {
		t.Fatalf("ReadyState() = %v, want closed", got)
	}
	dataChannel.SetOnOpen(func() {})
	dataChannel.SetOnClose(func() {})
	dataChannel.SetOnMessage(func([]byte) {})
	dataChannel.SetOnError(func(err error) {})
	if err := dataChannel.Send([]byte("hi")); err == nil {
		t.Fatal("Send on uninitialized data channel expected error")
	}
	if err := dataChannel.SendText("hi"); err == nil {
		t.Fatal("SendText on uninitialized data channel expected error")
	}
	if err := dataChannel.Close(); err != nil {
		t.Fatalf("Close() unexpected error: %v", err)
	}
}

func TestPeerConnectionRealTrackAndTransceiverOperations(t *testing.T) {
	testutil.SkipIfNoShim(t)
	release := testutil.WithSerialExecution(t)
	defer release()

	pc, err := NewPeerConnection(DefaultConfiguration())
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}
	defer pc.Close()

	pc.SetOnBandwidthEstimate(func(*BandwidthEstimate) {})
	_ = pc.GetCurrentBandwidthEstimate()

	videoTrack, err := pc.CreateVideoTrack("video-real", codec.VP8, 64, 64)
	if err != nil {
		t.Fatalf("CreateVideoTrack: %v", err)
	}
	if !videoTrack.Enabled() {
		t.Fatal("video track should start enabled")
	}

	videoSender, err := pc.AddTrack(videoTrack, "stream-real")
	if err != nil {
		t.Fatalf("AddTrack(video): %v", err)
	}
	if !videoSender.IsValid() {
		t.Fatal("sender should be valid after AddTrack")
	}
	if videoSender.Track() != videoTrack {
		t.Fatal("sender.Track() should return the added video track")
	}
	if err := videoTrack.WriteVideoFrame(testutil.CreateTestVideoFrame(64, 64)); err != nil {
		t.Fatalf("WriteVideoFrame: %v", err)
	}
	_ = videoSender.GetParameters()
	if _, err := videoSender.GetStats(); err != nil {
		t.Fatalf("sender.GetStats: %v", err)
	}

	audioTrack, err := pc.CreateAudioTrack("audio-real")
	if err != nil {
		t.Fatalf("CreateAudioTrack: %v", err)
	}
	if _, err := pc.AddTrack(audioTrack, "stream-real"); err != nil {
		t.Fatalf("AddTrack(audio): %v", err)
	}
	if err := audioTrack.WriteAudioFrame(testutil.CreateSilentAudioFrame(48_000, 2, 960)); err != nil {
		t.Fatalf("WriteAudioFrame: %v", err)
	}

	transceiver, err := pc.AddTransceiver("video", &TransceiverInit{
		Direction: TransceiverDirectionSendRecv,
	})
	if err != nil {
		t.Fatalf("AddTransceiver: %v", err)
	}
	if !transceiver.IsValid() {
		t.Fatal("transceiver should be valid")
	}
	if transceiver.Sender() == nil {
		t.Fatal("transceiver sender should not be nil")
	}
	if transceiver.Receiver() == nil {
		t.Fatal("transceiver receiver should not be nil")
	}
	if err := transceiver.SetDirection(TransceiverDirectionRecvOnly); err != nil {
		t.Fatalf("SetDirection: %v", err)
	}
	_ = transceiver.Direction()
	_ = transceiver.CurrentDirection()
	_ = transceiver.Mid()

	videoCodecs, err := GetSupportedVideoCodecs()
	if err != nil {
		t.Fatalf("GetSupportedVideoCodecs: %v", err)
	}
	if len(videoCodecs) > 0 {
		if err := transceiver.SetCodecPreferences(videoCodecs[:1]); err != nil {
			t.Fatalf("SetCodecPreferences: %v", err)
		}
	}

	if stats, err := pc.GetStats(); err != nil {
		t.Fatalf("GetStats: %v", err)
	} else if stats == nil {
		t.Fatal("GetStats() returned nil stats")
	}
	if err := pc.RestartICE(); err != nil {
		t.Fatalf("RestartICE: %v", err)
	}

	dc, err := pc.CreateDataChannel("control", nil)
	if err != nil {
		t.Fatalf("CreateDataChannel: %v", err)
	}
	if !dc.IsValid() {
		t.Fatal("data channel should be valid")
	}
	dc.SetOnOpen(func() {})
	dc.SetOnClose(func() {})
	dc.SetOnMessage(func([]byte) {})
	dc.SetOnError(func(error) {})
	_ = dc.ReadyState()
	_ = dc.Send([]byte("pre-open-binary"))
	_ = dc.SendText("pre-open-text")
	if err := dc.Close(); err != nil {
		t.Fatalf("data channel Close(): %v", err)
	}

	if err := transceiver.Stop(); err != nil && !errors.Is(err, ErrPeerConnectionClosed) {
		t.Fatalf("transceiver Stop(): %v", err)
	}
}
