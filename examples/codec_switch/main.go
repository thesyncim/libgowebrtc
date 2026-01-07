// Package main demonstrates codec negotiation and switching API in pure libgowebrtc.
//
// This example demonstrates:
// 1. SetCodecPreferences API - control which codecs appear in SDP
// 2. Negotiating multiple codecs upfront (VP8, VP9, H264, AV1)
// 3. GetNegotiatedCodecs() - query which codecs were actually negotiated
// 4. SetPreferredCodec() - attempt to switch codec (may need renegotiation)
// 5. Sending video with the negotiated codec
//
// NOTE: Runtime codec switching via SetPreferredCodec uses RtpSender::SetParameters
// to reorder codecs. This typically returns ErrRenegotiationNeeded since WebRTC
// spec doesn't allow codec changes without renegotiation. However, the API is
// provided for completeness and may work in some implementations.
package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/pc"
)

//go:embed index.html
var staticFiles embed.FS

const (
	width   = 640
	height  = 480
	fps     = 30
	bitrate = 1_000_000
)

func main() {
	// Load FFI library
	if err := ffi.LoadLibrary(); err != nil {
		log.Fatalf("Failed to load library: %v", err)
	}

	// Show supported codecs
	videoCodecs, err := pc.GetSupportedVideoCodecs()
	if err != nil {
		log.Fatalf("Failed to get supported codecs: %v", err)
	}
	log.Println("Supported video codecs:")
	for _, c := range videoCodecs {
		log.Printf("  - %s (clock: %d, PT: %d)", c.MimeType, c.ClockRate, c.PayloadType)
	}

	// HTTP handlers
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data, _ := staticFiles.ReadFile("index.html")
		w.Header().Set("Content-Type", "text/html")
		w.Write(data)
	})

	http.HandleFunc("/offer", handleOffer)

	log.Println("Server starting on http://localhost:8080")
	log.Println("This demonstrates SetCodecPreferences with both VP8 and AV1 negotiated")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func handleOffer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var offer struct {
		Type string `json:"type"`
		SDP  string `json:"sdp"`
	}
	if err := json.Unmarshal(body, &offer); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Println("Received offer from browser")
	log.Printf("Browser offer SDP:\n%s\n", offer.SDP[:min(500, len(offer.SDP))])

	// Create PeerConnection
	peerConn, err := pc.NewPeerConnection(pc.DefaultConfiguration())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	peerConn.OnConnectionStateChange = func(state pc.PeerConnectionState) {
		log.Printf("Connection state: %s", state.String())
	}

	// Create video track - uses libwebrtc's internal encoder
	// The codec will be determined by SDP negotiation
	track, err := peerConn.CreateVideoTrack("video", codec.VP8, width, height)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Add track to get sender
	sender, err := peerConn.AddTrack(track)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("Added track with sender, handle: %v", sender.IsValid())

	// Get transceivers to demonstrate SetCodecPreferences
	transceivers := peerConn.GetTransceivers()
	log.Printf("Number of transceivers: %d", len(transceivers))

	// If we have a transceiver, we can set codec preferences
	// This controls which codecs appear in our answer SDP
	for _, t := range transceivers {
		if t.IsValid() {
			log.Printf("Transceiver mid=%s, direction=%s", t.Mid(), t.Direction().String())

			// Set codec preferences - prefer VP8 first, then AV1
			// Both will be in the SDP, allowing the remote to accept either
			prefs := []pc.CodecCapability{
				{MimeType: "video/VP8", ClockRate: 90000},
				{MimeType: "video/AV1", ClockRate: 90000},
			}

			if err := t.SetCodecPreferences(prefs); err != nil {
				log.Printf("Warning: SetCodecPreferences failed: %v", err)
			} else {
				log.Println("Set codec preferences: VP8, AV1")
			}
		}
	}

	// Set remote description (browser's offer)
	if err := peerConn.SetRemoteDescription(&pc.SessionDescription{
		Type: pc.SDPTypeOffer,
		SDP:  offer.SDP,
	}); err != nil {
		http.Error(w, fmt.Sprintf("SetRemoteDescription failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Create answer
	answer, err := peerConn.CreateAnswer(nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("CreateAnswer failed: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Created answer SDP:\n%s\n", answer.SDP[:min(500, len(answer.SDP))])

	// Set local description
	if err := peerConn.SetLocalDescription(answer); err != nil {
		http.Error(w, fmt.Sprintf("SetLocalDescription failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Wait for ICE gathering to complete (has candidates in SDP)
	gatheringDone := make(chan struct{})
	var gatheringOnce sync.Once
	peerConn.OnICEGatheringStateChange = func(state pc.ICEGatheringState) {
		log.Printf("ICE gathering state: %s", state.String())
		if state == pc.ICEGatheringStateComplete {
			gatheringOnce.Do(func() { close(gatheringDone) })
		}
	}

	// Check if already complete
	if peerConn.ICEGatheringState() == pc.ICEGatheringStateComplete {
		gatheringOnce.Do(func() { close(gatheringDone) })
	}

	// Wait up to 3 seconds for ICE gathering
	select {
	case <-gatheringDone:
		log.Println("ICE gathering complete")
	case <-time.After(3 * time.Second):
		log.Println("ICE gathering timeout, proceeding anyway")
	}

	// Get local description with gathered candidates
	localDesc := peerConn.LocalDescription()
	if localDesc == nil {
		http.Error(w, "LocalDescription is nil", http.StatusInternalServerError)
		return
	}

	// Return answer with candidates
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"type": "answer",
		"sdp":  localDesc.SDP,
	})

	// Start sending video
	go sendVideo(peerConn, track, sender)
}

func sendVideo(peerConn *pc.PeerConnection, track *pc.Track, sender *pc.RTPSender) {
	log.Println("Starting video send goroutine...")

	// Wait for connected state
	for i := 0; i < 100; i++ {
		state := peerConn.ConnectionState()
		if state == pc.PeerConnectionStateConnected {
			break
		}
		if state == pc.PeerConnectionStateFailed || state == pc.PeerConnectionStateClosed {
			log.Println("Connection failed or closed before starting video")
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	if peerConn.ConnectionState() != pc.PeerConnectionStateConnected {
		log.Println("Connection did not reach connected state")
		return
	}

	log.Println("Connected! Starting video...")

	// Query negotiated codecs from sender
	log.Println("=== Querying Negotiated Codecs ===")
	codecs, err := sender.GetNegotiatedCodecs()
	if err != nil {
		log.Printf("Warning: GetNegotiatedCodecs failed: %v", err)
	} else {
		log.Printf("Negotiated %d codec(s):", len(codecs))
		for i, c := range codecs {
			log.Printf("  [%d] %s (PT=%d, clock=%d, fmtp=%s)",
				i, c.MimeType, c.PayloadType, c.ClockRate, c.SDPFmtpLine)
		}
	}

	// Demonstrate SetPreferredCodec API
	// Try to switch to a different codec (this typically needs renegotiation)
	if len(codecs) > 1 {
		log.Println("=== Attempting Codec Switch ===")
		// Try to switch to the second negotiated codec
		targetCodec := codecs[1]
		log.Printf("Attempting to switch to: %s (PT=%d)", targetCodec.MimeType, targetCodec.PayloadType)

		err := sender.SetPreferredCodec(targetCodec.MimeType, targetCodec.PayloadType)
		if err == pc.ErrRenegotiationNeeded {
			log.Println("  -> Result: Renegotiation needed (expected per WebRTC spec)")
			log.Println("  -> To switch codecs mid-stream, you would need to trigger a new offer/answer exchange")
		} else if err == pc.ErrNotFound {
			log.Println("  -> Result: Codec not found in negotiated list")
		} else if err != nil {
			log.Printf("  -> Result: Unexpected error: %v", err)
		} else {
			log.Println("  -> Result: Success! Codec switch applied without renegotiation")
		}
	}

	// Create I420 frame
	f := frame.NewI420Frame(width, height)

	ticker := time.NewTicker(time.Second / fps)
	defer ticker.Stop()

	startTime := time.Now()
	frameNum := 0
	codecIndex := 0 // Track which codec to try next

	for range ticker.C {
		if peerConn.ConnectionState() != pc.PeerConnectionStateConnected {
			log.Println("Connection no longer connected, stopping video")
			return
		}

		// Generate test pattern
		generateTestPattern(f, frameNum)
		frameNum++

		// Set timestamp (90kHz RTP clock)
		elapsed := time.Since(startTime)
		f.PTS = uint32(elapsed.Seconds() * 90000)
		f.Timestamp = elapsed

		// Write frame to track
		// libwebrtc handles encoding with the negotiated codec
		if err := track.WriteVideoFrame(f); err != nil {
			log.Printf("WriteVideoFrame error: %v", err)
			continue
		}

		// Every 5 seconds: show stats and attempt codec switch
		if frameNum%(fps*5) == 0 {
			log.Printf("Sent %d frames (%.1f seconds)", frameNum, elapsed.Seconds())

			// Show stats
			stats, err := peerConn.GetStats()
			if err == nil && stats != nil {
				log.Printf("  Stats: frames=%d, bytes=%d, packets=%d",
					stats.FramesEncoded, stats.BytesSent, stats.PacketsSent)
			}

			// Get current negotiated codecs and attempt to switch
			currentCodecs, err := sender.GetNegotiatedCodecs()
			if err == nil && len(currentCodecs) > 0 {
				log.Printf("  Current active codec: %s (PT=%d)", currentCodecs[0].MimeType, currentCodecs[0].PayloadType)

				// Try to switch to next codec in the list
				if len(currentCodecs) > 1 {
					codecIndex = (codecIndex + 1) % len(currentCodecs)
					targetCodec := currentCodecs[codecIndex]

					log.Printf("=== Attempting codec switch to: %s (PT=%d) ===", targetCodec.MimeType, targetCodec.PayloadType)
					err := sender.SetPreferredCodec(targetCodec.MimeType, targetCodec.PayloadType)
					if err == pc.ErrRenegotiationNeeded {
						log.Println("  -> Renegotiation needed (expected per WebRTC spec)")
					} else if err == pc.ErrNotFound {
						log.Println("  -> Codec not found")
					} else if err != nil {
						log.Printf("  -> Error: %v", err)
					} else {
						log.Println("  -> Success! Codec switch applied")
					}
				}
			}
		}
	}
}

func generateTestPattern(f *frame.VideoFrame, frameNum int) {
	y := f.YPlane()
	u := f.UPlane()
	v := f.VPlane()

	w := f.Width
	h := f.Height

	// Moving gradient
	offset := frameNum % 256

	// Y plane - gradient with moving bars
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			// Base gradient
			val := (col + row + offset) % 256

			// Add horizontal moving bar
			barY := (frameNum * 2) % h
			if row >= barY && row < barY+20 {
				val = 255 // White bar
			}

			y[row*w+col] = uint8(val)
		}
	}

	// U/V planes - color cycling
	uvW := w / 2
	uvH := h / 2

	// Use sine waves for smooth color transitions
	uVal := uint8(128 + 100*math.Sin(float64(frameNum)*0.05))
	vVal := uint8(128 + 100*math.Cos(float64(frameNum)*0.05))

	for row := 0; row < uvH; row++ {
		for col := 0; col < uvW; col++ {
			u[row*uvW+col] = uVal
			v[row*uvW+col] = vVal
		}
	}
}
