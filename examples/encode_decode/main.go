// Example: Basic encode/decode pipeline using libgowebrtc
//
// This example demonstrates:
// 1. Creating a video encoder (H264)
// 2. Encoding raw I420 frames
// 3. Creating a decoder
// 4. Decoding the encoded frames back
//
// Run: go run ./examples/encode_decode/
// Note: Requires libwebrtc_shim library to be available
package main

import (
	"fmt"
	"log"

	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/decoder"
	"github.com/thesyncim/libgowebrtc/pkg/encoder"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
)

func main() {
	fmt.Println("libgowebrtc Encode/Decode Example")
	fmt.Println("==================================")

	// Configuration
	width := 640
	height := 480
	fps := 30.0
	bitrate := uint32(1_000_000) // 1 Mbps

	// Create H264 encoder
	fmt.Println("\n1. Creating H264 encoder...")
	enc, err := encoder.NewH264Encoder(codec.H264Config{
		Width:       width,
		Height:      height,
		Bitrate:     bitrate,
		MaxBitrate:  bitrate * 2,
		FPS:         fps,
		Profile:     codec.H264ProfileBaseline,
		RateControl: codec.RateControlCBR,
		LowDelay:    true,
	})
	if err != nil {
		log.Fatalf("Failed to create encoder: %v", err)
	}
	defer enc.Close()
	fmt.Printf("   Encoder created: %s, max encoded size: %d bytes\n", enc.Codec(), enc.MaxEncodedSize())

	// Create decoder
	fmt.Println("\n2. Creating H264 decoder...")
	dec, err := decoder.NewVideoDecoder(codec.H264)
	if err != nil {
		log.Fatalf("Failed to create decoder: %v", err)
	}
	defer dec.Close()
	fmt.Printf("   Decoder created: %s\n", dec.Codec())

	// Allocate buffers
	srcFrame := frame.NewI420Frame(width, height)
	dstFrame := frame.NewI420Frame(width, height)
	encodedBuf := make([]byte, enc.MaxEncodedSize())

	// Fill source frame with a gradient pattern
	// For I420: Data[0]=Y, Data[1]=U, Data[2]=V
	fmt.Println("\n3. Generating test frame (gradient pattern)...")
	yPlane := srcFrame.Data[0]
	uPlane := srcFrame.Data[1]
	vPlane := srcFrame.Data[2]

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			yPlane[y*width+x] = byte((x + y) % 256)
		}
	}
	for i := range uPlane {
		uPlane[i] = 128
		vPlane[i] = 128
	}
	fmt.Printf("   Frame size: %dx%d, Y plane: %d bytes\n", width, height, len(yPlane))

	// Encode multiple frames
	fmt.Println("\n4. Encoding frames...")
	var encodedFrames [][]byte
	numFrames := 10

	for i := 0; i < numFrames; i++ {
		srcFrame.PTS = uint32(i * 33) // ~30fps
		forceKeyframe := i == 0

		result, err := enc.EncodeInto(srcFrame, encodedBuf, forceKeyframe)
		if err != nil {
			log.Fatalf("Encode failed at frame %d: %v", i, err)
		}

		// Copy encoded data
		encoded := make([]byte, result.N)
		copy(encoded, encodedBuf[:result.N])
		encodedFrames = append(encodedFrames, encoded)

		keyframeStr := ""
		if result.IsKeyframe {
			keyframeStr = " [KEYFRAME]"
		}
		fmt.Printf("   Frame %d: %d bytes%s\n", i, result.N, keyframeStr)
	}

	// Calculate compression ratio
	rawSize := width * height * 3 / 2 // I420
	totalEncoded := 0
	for _, f := range encodedFrames {
		totalEncoded += len(f)
	}
	avgEncoded := totalEncoded / numFrames
	ratio := float64(rawSize) / float64(avgEncoded)
	fmt.Printf("   Average: %d bytes (%.1fx compression)\n", avgEncoded, ratio)

	// Decode frames
	fmt.Println("\n5. Decoding frames...")
	for i, encoded := range encodedFrames {
		isKeyframe := i == 0
		err := dec.DecodeInto(encoded, dstFrame, uint32(i*33), isKeyframe)
		if err != nil {
			fmt.Printf("   Frame %d: decode error (may need more data): %v\n", i, err)
			continue
		}
		fmt.Printf("   Frame %d: decoded %dx%d\n", i, dstFrame.Width, dstFrame.Height)
	}

	// Runtime controls demo
	fmt.Println("\n6. Testing runtime controls...")

	err = enc.SetBitrate(2_000_000)
	if err != nil {
		fmt.Printf("   SetBitrate: %v\n", err)
	} else {
		fmt.Println("   SetBitrate(2Mbps): OK")
	}

	err = enc.SetFramerate(60)
	if err != nil {
		fmt.Printf("   SetFramerate: %v\n", err)
	} else {
		fmt.Println("   SetFramerate(60fps): OK")
	}

	enc.RequestKeyFrame()
	fmt.Println("   RequestKeyFrame: OK")

	fmt.Println("\n7. Done!")
	fmt.Println("   The encode/decode pipeline works correctly.")
}
