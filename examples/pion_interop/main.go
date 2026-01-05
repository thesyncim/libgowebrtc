// Example: Pion WebRTC interoperability
//
// This example demonstrates:
// 1. Creating a libwebrtc-backed video track
// 2. Adding it to a Pion PeerConnection
// 3. Writing raw frames to the track
//
// Run: go run ./examples/pion_interop/
// Note: Requires libwebrtc_shim library to be available
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/pion/webrtc/v4"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/track"
)

func main() {
	fmt.Println("libgowebrtc Pion Interop Example")
	fmt.Println("=================================")

	// Create a Pion PeerConnection
	fmt.Println("\n1. Creating Pion PeerConnection...")
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
		},
	}

	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		log.Fatalf("Failed to create PeerConnection: %v", err)
	}
	defer pc.Close()
	fmt.Println("   PeerConnection created")

	// Create libwebrtc-backed video track
	fmt.Println("\n2. Creating libwebrtc video track (VP8)...")
	videoTrack, err := track.NewVideoTrack(track.VideoTrackConfig{
		ID:      "video-0",
		Codec:   codec.VP8,
		Width:   1280,
		Height:  720,
		Bitrate: 2_000_000,
		FPS:     30,
	})
	if err != nil {
		log.Fatalf("Failed to create video track: %v", err)
	}
	defer videoTrack.Close()
	fmt.Printf("   Track ID: %s\n", videoTrack.ID())

	// Create audio track
	fmt.Println("\n3. Creating libwebrtc audio track (Opus)...")
	audioTrack, err := track.NewAudioTrack(track.AudioTrackConfig{
		ID:         "audio-0",
		SampleRate: 48000,
		Channels:   2,
		Bitrate:    64000,
	})
	if err != nil {
		log.Fatalf("Failed to create audio track: %v", err)
	}
	defer audioTrack.Close()
	fmt.Printf("   Track ID: %s\n", audioTrack.ID())

	// Add tracks to Pion PeerConnection
	fmt.Println("\n4. Adding tracks to PeerConnection...")
	videoSender, err := pc.AddTrack(videoTrack)
	if err != nil {
		log.Fatalf("Failed to add video track: %v", err)
	}
	fmt.Printf("   Video track added, sender: %v\n", videoSender != nil)

	audioSender, err := pc.AddTrack(audioTrack)
	if err != nil {
		log.Fatalf("Failed to add audio track: %v", err)
	}
	fmt.Printf("   Audio track added, sender: %v\n", audioSender != nil)

	// Create offer
	fmt.Println("\n5. Creating SDP offer...")
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		log.Fatalf("Failed to create offer: %v", err)
	}
	fmt.Printf("   Offer created: %d bytes\n", len(offer.SDP))

	// Set local description
	err = pc.SetLocalDescription(offer)
	if err != nil {
		log.Fatalf("Failed to set local description: %v", err)
	}
	fmt.Println("   Local description set")

	// Demonstrate writing frames
	fmt.Println("\n6. Writing test frames...")
	videoFrame := frame.NewI420Frame(1280, 720)
	audioFrame := frame.NewAudioFrameS16(48000, 2, 960) // 20ms

	// Fill video frame with test pattern
	yPlane := videoFrame.Data[0]
	uPlane := videoFrame.Data[1]
	vPlane := videoFrame.Data[2]
	for i := range yPlane {
		yPlane[i] = byte(i % 256)
	}
	for i := range uPlane {
		uPlane[i] = 128
		vPlane[i] = 128
	}

	// Write a few frames
	for i := 0; i < 5; i++ {
		videoFrame.PTS = uint32(i * 33)
		err := videoTrack.WriteFrame(videoFrame, i == 0)
		if err != nil {
			fmt.Printf("   Video frame %d: %v\n", i, err)
		} else {
			fmt.Printf("   Video frame %d: written\n", i)
		}

		audioFrame.PTS = uint32(i * 20)
		err = audioTrack.WriteFrame(audioFrame)
		if err != nil {
			fmt.Printf("   Audio frame %d: %v\n", i, err)
		} else {
			fmt.Printf("   Audio frame %d: written\n", i)
		}

		time.Sleep(33 * time.Millisecond)
	}

	// Runtime control demo
	fmt.Println("\n7. Testing runtime controls...")
	err = videoTrack.SetBitrate(4_000_000)
	if err != nil {
		fmt.Printf("   SetBitrate: %v\n", err)
	} else {
		fmt.Println("   SetBitrate(4Mbps): OK")
	}

	videoTrack.RequestKeyFrame()
	fmt.Println("   RequestKeyFrame: OK")

	// Print SDP (truncated)
	fmt.Println("\n8. SDP Offer preview:")
	sdp := offer.SDP
	if len(sdp) > 500 {
		sdp = sdp[:500] + "..."
	}
	fmt.Println(sdp)

	fmt.Println("\n9. Done!")
	fmt.Println("   libwebrtc tracks work seamlessly with Pion PeerConnection.")
}
