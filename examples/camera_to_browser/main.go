// Package main demonstrates libgowebrtc streaming video to a browser.
//
// This example shows:
// - WebSocket signaling for offer/answer/ICE exchange
// - libwebrtc PeerConnection with video track
// - Frame generation (gradient pattern)
// - DataChannel for bidirectional messaging
// - Real-time statistics display
//
// Usage:
//
//	LIBWEBRTC_SHIM_PATH=/path/to/libwebrtc_shim.dylib go run .
//
// Then open http://localhost:8080 in your browser.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/pc"
)

var (
	addr       = flag.String("addr", ":8080", "HTTP server address")
	width      = flag.Int("width", 1280, "Video width")
	height     = flag.Int("height", 720, "Video height")
	fps        = flag.Int("fps", 30, "Frames per second")
	codecName  = flag.String("codec", "vp8", "Video codec (vp8, vp9, h264, av1)")
	stunServer = flag.String("stun", "stun:stun.l.google.com:19302", "STUN server URL")
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// SignalingMessage represents a WebSocket signaling message.
type SignalingMessage struct {
	Type      string          `json:"type"`
	SDP       string          `json:"sdp,omitempty"`
	Candidate json.RawMessage `json:"candidate,omitempty"`
	Error     string          `json:"error,omitempty"`
	Stats     *pc.RTCStats    `json:"stats,omitempty"`
	Message   string          `json:"message,omitempty"`
}

// Session manages a single WebRTC session with a browser.
type Session struct {
	conn        *websocket.Conn
	peerConn    *pc.PeerConnection
	videoTrack  *pc.Track
	dataChannel *pc.DataChannel
	cancel      context.CancelFunc
	mu          sync.Mutex
	closed      bool
}

func main() {
	flag.Parse()

	http.HandleFunc("/", serveIndex)
	http.HandleFunc("/ws", handleWebSocket)

	log.Printf("Starting libgowebrtc browser example")
	log.Printf("   Address: http://localhost%s", *addr)
	log.Printf("   Resolution: %dx%d @ %d fps", *width, *height, *fps)
	log.Printf("   Codec: %s", *codecName)
	log.Printf("")
	log.Printf("Open your browser to http://localhost%s", *addr)

	if err := http.ListenAndServe(*addr, nil); err != nil {
		log.Fatal(err)
	}
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(indexHTML))
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("New browser connection from %s", r.RemoteAddr)

	session := &Session{conn: conn}
	if err := session.run(); err != nil {
		log.Printf("Session error: %v", err)
	}

	log.Printf("Browser disconnected: %s", r.RemoteAddr)
}

func (s *Session) run() error {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	defer cancel()

	// Create PeerConnection
	peerConn, err := pc.NewPeerConnection(pc.Configuration{
		ICEServers: []pc.ICEServer{
			{URLs: []string{*stunServer}},
		},
	})
	if err != nil {
		return fmt.Errorf("create peer connection: %w", err)
	}
	s.peerConn = peerConn
	defer peerConn.Close()

	// Set up callbacks
	s.setupCallbacks()

	// Create video track
	codecType := parseCodec(*codecName)
	videoTrack, err := peerConn.CreateVideoTrack("video0", codecType, *width, *height)
	if err != nil {
		return fmt.Errorf("create video track: %w", err)
	}
	s.videoTrack = videoTrack

	// Add track to PeerConnection
	if _, err := peerConn.AddTrack(videoTrack); err != nil {
		return fmt.Errorf("add track: %w", err)
	}

	// Create DataChannel for messaging
	dc, err := peerConn.CreateDataChannel("chat", nil)
	if err != nil {
		log.Printf("Warning: could not create data channel: %v", err)
	} else {
		s.dataChannel = dc
		s.setupDataChannel(dc)
	}

	// Start frame generation
	go s.generateFrames(ctx)

	// Start stats reporting
	go s.reportStats(ctx)

	// Handle signaling messages
	for {
		var msg SignalingMessage
		if err := s.conn.ReadJSON(&msg); err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return nil
			}
			return fmt.Errorf("read message: %w", err)
		}

		if err := s.handleMessage(msg); err != nil {
			log.Printf("Handle message error: %v", err)
			s.sendError(err.Error())
		}
	}
}

func (s *Session) setupCallbacks() {
	s.peerConn.OnConnectionStateChange = func(state pc.PeerConnectionState) {
		log.Printf("Connection state: %s", state)
		s.sendJSON(SignalingMessage{
			Type:    "connection-state",
			Message: state.String(),
		})
	}

	s.peerConn.OnICECandidate = func(candidate *pc.ICECandidate) {
		if candidate == nil {
			return
		}
		candidateJSON, _ := json.Marshal(candidate)
		s.sendJSON(SignalingMessage{
			Type:      "candidate",
			Candidate: candidateJSON,
		})
	}

	s.peerConn.OnICEConnectionStateChange = func(state pc.ICEConnectionState) {
		log.Printf("ICE connection state: %s", state)
	}

	s.peerConn.OnNegotiationNeeded = func() {
		log.Printf("Negotiation needed")
	}
}

func (s *Session) setupDataChannel(dc *pc.DataChannel) {
	dc.OnOpen = func() {
		log.Printf("DataChannel opened: %s", dc.Label())
		dc.Send([]byte("Welcome to libgowebrtc!"))
	}

	dc.OnMessage = func(data []byte) {
		log.Printf("DataChannel message: %s", string(data))
		// Echo back with timestamp
		response := fmt.Sprintf("[%s] Echo: %s", time.Now().Format("15:04:05"), string(data))
		dc.Send([]byte(response))
	}

	dc.OnClose = func() {
		log.Printf("DataChannel closed")
	}
}

func (s *Session) handleMessage(msg SignalingMessage) error {
	switch msg.Type {
	case "offer":
		return s.handleOffer(msg.SDP)
	case "answer":
		return s.handleAnswer(msg.SDP)
	case "candidate":
		return s.handleCandidate(msg.Candidate)
	case "request-offer":
		return s.createAndSendOffer()
	default:
		return fmt.Errorf("unknown message type: %s", msg.Type)
	}
}

func (s *Session) handleOffer(sdp string) error {
	// DEBUG: Log browser offer (first 500 chars)
	log.Printf("DEBUG: Browser offer (first 500 chars):\n%s...", sdp[:min(500, len(sdp))])

	if err := s.peerConn.SetRemoteDescription(&pc.SessionDescription{
		Type: pc.SDPTypeOffer,
		SDP:  sdp,
	}); err != nil {
		return fmt.Errorf("set remote description: %w", err)
	}

	answer, err := s.peerConn.CreateAnswer(nil)
	if err != nil {
		return fmt.Errorf("create answer: %w", err)
	}

	// DEBUG: Log our answer
	log.Printf("DEBUG: Our answer SDP:\n%s", answer.SDP)

	if err := s.peerConn.SetLocalDescription(answer); err != nil {
		return fmt.Errorf("set local description: %w", err)
	}

	return s.sendJSON(SignalingMessage{
		Type: "answer",
		SDP:  answer.SDP,
	})
}

func (s *Session) handleAnswer(sdp string) error {
	return s.peerConn.SetRemoteDescription(&pc.SessionDescription{
		Type: pc.SDPTypeAnswer,
		SDP:  sdp,
	})
}

func (s *Session) handleCandidate(candidateJSON json.RawMessage) error {
	var candidate pc.ICECandidate
	if err := json.Unmarshal(candidateJSON, &candidate); err != nil {
		return fmt.Errorf("parse candidate: %w", err)
	}
	return s.peerConn.AddICECandidate(&candidate)
}

func (s *Session) createAndSendOffer() error {
	offer, err := s.peerConn.CreateOffer(nil)
	if err != nil {
		return fmt.Errorf("create offer: %w", err)
	}

	// DEBUG: Print SDP to see codec negotiation
	log.Printf("DEBUG: Offer SDP:\n%s", offer.SDP)

	if err := s.peerConn.SetLocalDescription(offer); err != nil {
		return fmt.Errorf("set local description: %w", err)
	}

	return s.sendJSON(SignalingMessage{
		Type: "offer",
		SDP:  offer.SDP,
	})
}

func (s *Session) generateFrames(ctx context.Context) {
	ticker := time.NewTicker(time.Second / time.Duration(*fps))
	defer ticker.Stop()

	videoFrame := frame.NewI420Frame(*width, *height)
	frameCount := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Generate gradient test pattern with moving bar
			generateTestPattern(videoFrame, frameCount)

			// Set timestamp in 90kHz clock (standard for video RTP)
			videoFrame.PTS = uint32(frameCount * (90000 / *fps))
			frameCount++

			// DEBUG: Log frame write status periodically
			if frameCount%100 == 0 {
				log.Printf("DEBUG: Writing frame %d, PTS=%d", frameCount, videoFrame.PTS)
			}

			if err := s.videoTrack.WriteVideoFrame(videoFrame); err != nil {
				// Only log if not a "not initialized" error (expected before connection)
				if err.Error() != "track source not initialized" {
					log.Printf("Write frame error: %v", err)
				}
			}
		}
	}
}

func generateTestPattern(f *frame.VideoFrame, frameNum int) {
	yPlane := f.YPlane()
	uPlane := f.UPlane()
	vPlane := f.VPlane()

	width := f.Width
	height := f.Height
	yStride := f.Stride[0]
	uvStride := f.Stride[1]

	// Moving bar position
	barPos := (frameNum * 4) % width
	barWidth := width / 10

	// Generate Y plane (luminance)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Gradient background
			luma := uint8((x * 255) / width)

			// Moving white bar
			if x >= barPos && x < barPos+barWidth {
				luma = 235 // White in limited range
			}

			// Add some vertical gradient
			luma = uint8((int(luma)*3 + (y*255)/height) / 4)

			yPlane[y*yStride+x] = luma
		}
	}

	// Generate U and V planes (chrominance)
	for y := 0; y < height/2; y++ {
		for x := 0; x < width/2; x++ {
			// Color varies by position
			srcX := x * 2
			srcY := y * 2

			// Create color bands
			hue := (srcX + frameNum) % 360
			u, v := hueToUV(hue)

			// More saturated in center
			centerDist := abs(srcY-height/2) * 2
			saturation := 1.0 - float64(centerDist)/float64(height)
			if saturation < 0 {
				saturation = 0
			}

			uPlane[y*uvStride+x] = uint8(128 + int(float64(int(u)-128)*saturation))
			vPlane[y*uvStride+x] = uint8(128 + int(float64(int(v)-128)*saturation))
		}
	}
}

func hueToUV(hue int) (u, v uint8) {
	// Simplified hue to YUV conversion
	switch {
	case hue < 60:
		return 128, 200 // Red-ish
	case hue < 120:
		return 80, 128 // Yellow-Green
	case hue < 180:
		return 180, 80 // Cyan
	case hue < 240:
		return 200, 128 // Blue
	case hue < 300:
		return 128, 180 // Magenta
	default:
		return 100, 200 // Red
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func (s *Session) reportStats(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			stats, err := s.peerConn.GetStats()
			if err != nil {
				continue
			}

			// Send stats to browser
			s.sendJSON(SignalingMessage{
				Type:  "stats",
				Stats: stats,
			})

			// Log to console
			if stats.PacketsSent > 0 {
				log.Printf("Stats: sent=%d pkts, %d KB | RTT=%.1fms | loss=%d",
					stats.PacketsSent,
					stats.BytesSent/1024,
					stats.RoundTripTimeMs,
					stats.PacketsLost)
			}
		}
	}
}

func (s *Session) sendJSON(msg SignalingMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	return s.conn.WriteJSON(msg)
}

func (s *Session) sendError(errMsg string) {
	s.sendJSON(SignalingMessage{
		Type:  "error",
		Error: errMsg,
	})
}

func parseCodec(name string) codec.Type {
	switch name {
	case "vp8":
		return codec.VP8
	case "vp9":
		return codec.VP9
	case "h264":
		return codec.H264
	case "av1":
		return codec.AV1
	default:
		return codec.VP8
	}
}

const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>libgowebrtc Browser Demo</title>
    <style>
        * {
            box-sizing: border-box;
            margin: 0;
            padding: 0;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #1a1a2e 0%, #16213e 100%);
            color: #fff;
            min-height: 100vh;
            padding: 20px;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
        }
        header {
            text-align: center;
            margin-bottom: 30px;
        }
        h1 {
            font-size: 2.5rem;
            margin-bottom: 10px;
            background: linear-gradient(90deg, #00d2ff, #3a7bd5);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }
        .subtitle {
            color: #888;
            font-size: 1.1rem;
        }
        .main-content {
            display: grid;
            grid-template-columns: 1fr 350px;
            gap: 20px;
        }
        @media (max-width: 900px) {
            .main-content {
                grid-template-columns: 1fr;
            }
        }
        .video-container {
            background: #000;
            border-radius: 12px;
            overflow: hidden;
            aspect-ratio: 16/9;
            position: relative;
        }
        video {
            width: 100%;
            height: 100%;
            object-fit: contain;
        }
        .overlay {
            position: absolute;
            top: 10px;
            left: 10px;
            background: rgba(0,0,0,0.7);
            padding: 8px 12px;
            border-radius: 6px;
            font-size: 0.9rem;
        }
        .status {
            display: flex;
            align-items: center;
            gap: 8px;
        }
        .status-dot {
            width: 10px;
            height: 10px;
            border-radius: 50%;
            background: #666;
        }
        .status-dot.connected { background: #4caf50; }
        .status-dot.connecting { background: #ff9800; animation: pulse 1s infinite; }
        .status-dot.failed { background: #f44336; }
        @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.5; }
        }
        .sidebar {
            display: flex;
            flex-direction: column;
            gap: 20px;
        }
        .card {
            background: rgba(255,255,255,0.05);
            border-radius: 12px;
            padding: 20px;
            border: 1px solid rgba(255,255,255,0.1);
        }
        .card h3 {
            margin-bottom: 15px;
            font-size: 1rem;
            color: #888;
            text-transform: uppercase;
            letter-spacing: 1px;
        }
        button {
            width: 100%;
            padding: 14px 20px;
            font-size: 1rem;
            font-weight: 600;
            border: none;
            border-radius: 8px;
            cursor: pointer;
            transition: all 0.2s;
        }
        .btn-primary {
            background: linear-gradient(90deg, #00d2ff, #3a7bd5);
            color: #fff;
        }
        .btn-primary:hover {
            transform: translateY(-2px);
            box-shadow: 0 4px 12px rgba(0, 210, 255, 0.3);
        }
        .btn-primary:disabled {
            opacity: 0.5;
            cursor: not-allowed;
            transform: none;
        }
        .btn-secondary {
            background: rgba(255,255,255,0.1);
            color: #fff;
        }
        .btn-secondary:hover {
            background: rgba(255,255,255,0.2);
        }
        .stats-grid {
            display: grid;
            grid-template-columns: repeat(2, 1fr);
            gap: 10px;
        }
        .stat-item {
            background: rgba(0,0,0,0.2);
            padding: 12px;
            border-radius: 8px;
        }
        .stat-label {
            font-size: 0.75rem;
            color: #888;
            margin-bottom: 4px;
        }
        .stat-value {
            font-size: 1.2rem;
            font-weight: 600;
            color: #00d2ff;
        }
        .chat-container {
            height: 200px;
            display: flex;
            flex-direction: column;
        }
        .chat-messages {
            flex: 1;
            overflow-y: auto;
            background: rgba(0,0,0,0.2);
            border-radius: 8px;
            padding: 10px;
            margin-bottom: 10px;
            font-size: 0.9rem;
        }
        .chat-message {
            padding: 6px 0;
            border-bottom: 1px solid rgba(255,255,255,0.05);
        }
        .chat-message:last-child {
            border-bottom: none;
        }
        .chat-input-container {
            display: flex;
            gap: 10px;
        }
        .chat-input {
            flex: 1;
            padding: 10px 14px;
            border: 1px solid rgba(255,255,255,0.2);
            border-radius: 8px;
            background: rgba(0,0,0,0.2);
            color: #fff;
            font-size: 0.9rem;
        }
        .chat-input:focus {
            outline: none;
            border-color: #00d2ff;
        }
        .btn-send {
            padding: 10px 20px;
            background: #00d2ff;
            color: #000;
            font-weight: 600;
        }
        .log-container {
            height: 150px;
            overflow-y: auto;
            background: rgba(0,0,0,0.3);
            border-radius: 8px;
            padding: 10px;
            font-family: 'Monaco', 'Menlo', monospace;
            font-size: 0.75rem;
            color: #4caf50;
        }
        .log-entry {
            margin-bottom: 4px;
        }
        .log-entry.error { color: #f44336; }
        .log-entry.warn { color: #ff9800; }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>libgowebrtc</h1>
            <p class="subtitle">High-performance WebRTC streaming from Go to Browser</p>
        </header>

        <div class="main-content">
            <div class="video-section">
                <div class="video-container">
                    <video id="remoteVideo" autoplay playsinline muted></video>
                    <div class="overlay">
                        <div class="status">
                            <div class="status-dot" id="statusDot"></div>
                            <span id="statusText">Disconnected</span>
                        </div>
                    </div>
                </div>
            </div>

            <div class="sidebar">
                <div class="card">
                    <h3>Connection</h3>
                    <button id="connectBtn" class="btn-primary">Connect</button>
                </div>

                <div class="card">
                    <h3>Statistics</h3>
                    <div class="stats-grid">
                        <div class="stat-item">
                            <div class="stat-label">Packets Recv</div>
                            <div class="stat-value" id="statPackets">0</div>
                        </div>
                        <div class="stat-item">
                            <div class="stat-label">Bytes Recv</div>
                            <div class="stat-value" id="statBytes">0 KB</div>
                        </div>
                        <div class="stat-item">
                            <div class="stat-label">RTT</div>
                            <div class="stat-value" id="statRtt">- ms</div>
                        </div>
                        <div class="stat-item">
                            <div class="stat-label">Packet Loss</div>
                            <div class="stat-value" id="statLoss">0</div>
                        </div>
                    </div>
                </div>

                <div class="card chat-container">
                    <h3>DataChannel Chat</h3>
                    <div class="chat-messages" id="chatMessages"></div>
                    <div class="chat-input-container">
                        <input type="text" class="chat-input" id="chatInput" placeholder="Type a message..." disabled>
                        <button class="btn-send" id="sendBtn" disabled>Send</button>
                    </div>
                </div>

                <div class="card">
                    <h3>Log</h3>
                    <div class="log-container" id="logContainer"></div>
                </div>
            </div>
        </div>
    </div>

    <script>
    class WebRTCClient {
        constructor() {
            this.ws = null;
            this.pc = null;
            this.dataChannel = null;
            this.elements = {
                video: document.getElementById('remoteVideo'),
                statusDot: document.getElementById('statusDot'),
                statusText: document.getElementById('statusText'),
                connectBtn: document.getElementById('connectBtn'),
                chatMessages: document.getElementById('chatMessages'),
                chatInput: document.getElementById('chatInput'),
                sendBtn: document.getElementById('sendBtn'),
                logContainer: document.getElementById('logContainer'),
                statPackets: document.getElementById('statPackets'),
                statBytes: document.getElementById('statBytes'),
                statRtt: document.getElementById('statRtt'),
                statLoss: document.getElementById('statLoss'),
            };
            this.setupEventListeners();
        }

        setupEventListeners() {
            this.elements.connectBtn.addEventListener('click', () => this.toggleConnection());
            this.elements.sendBtn.addEventListener('click', () => this.sendChatMessage());
            this.elements.chatInput.addEventListener('keypress', (e) => {
                if (e.key === 'Enter') this.sendChatMessage();
            });
        }

        log(message, type = 'info') {
            const entry = document.createElement('div');
            entry.className = 'log-entry' + (type !== 'info' ? ' ' + type : '');
            entry.textContent = '[' + new Date().toLocaleTimeString() + '] ' + message;
            this.elements.logContainer.appendChild(entry);
            this.elements.logContainer.scrollTop = this.elements.logContainer.scrollHeight;
        }

        setStatus(status, text) {
            this.elements.statusDot.className = 'status-dot ' + status;
            this.elements.statusText.textContent = text;
        }

        toggleConnection() {
            if (this.ws) {
                this.disconnect();
            } else {
                this.connect();
            }
        }

        async connect() {
            this.log('Connecting to signaling server...');
            this.setStatus('connecting', 'Connecting...');
            this.elements.connectBtn.textContent = 'Connecting...';
            this.elements.connectBtn.disabled = true;

            try {
                // Connect WebSocket
                const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
                this.ws = new WebSocket(protocol + '//' + window.location.host + '/ws');

                this.ws.onopen = () => {
                    this.log('WebSocket connected');
                    this.setupPeerConnection();
                    this.startStatsCollection();
                };

                this.ws.onmessage = (event) => {
                    const msg = JSON.parse(event.data);
                    this.handleSignalingMessage(msg);
                };

                this.ws.onclose = () => {
                    this.log('WebSocket closed', 'warn');
                    this.disconnect();
                };

                this.ws.onerror = (error) => {
                    this.log('WebSocket error: ' + error.message, 'error');
                };
            } catch (error) {
                this.log('Connection failed: ' + error.message, 'error');
                this.disconnect();
            }
        }

        async setupPeerConnection() {
            this.log('Creating PeerConnection...');

            this.pc = new RTCPeerConnection({
                iceServers: [{ urls: 'stun:stun.l.google.com:19302' }]
            });

            this.pc.ontrack = (event) => {
                this.log('Received remote track: ' + event.track.kind);
                this.elements.video.srcObject = event.streams[0];
            };

            this.pc.onicecandidate = (event) => {
                if (event.candidate) {
                    this.ws.send(JSON.stringify({
                        type: 'candidate',
                        candidate: event.candidate
                    }));
                }
            };

            this.pc.onconnectionstatechange = () => {
                this.log('Connection state: ' + this.pc.connectionState);
                switch (this.pc.connectionState) {
                    case 'connected':
                        this.setStatus('connected', 'Connected');
                        this.elements.connectBtn.textContent = 'Disconnect';
                        this.elements.connectBtn.disabled = false;
                        break;
                    case 'failed':
                        this.setStatus('failed', 'Failed');
                        this.disconnect();
                        break;
                    case 'disconnected':
                        this.setStatus('connecting', 'Reconnecting...');
                        break;
                }
            };

            this.pc.ondatachannel = (event) => {
                this.log('DataChannel received: ' + event.channel.label);
                this.setupDataChannel(event.channel);
            };

            // Add transceiver for receiving video
            this.pc.addTransceiver('video', { direction: 'recvonly' });
            this.pc.addTransceiver('audio', { direction: 'recvonly' });

            // Create offer
            const offer = await this.pc.createOffer();
            await this.pc.setLocalDescription(offer);

            this.ws.send(JSON.stringify({
                type: 'offer',
                sdp: offer.sdp
            }));

            this.log('Sent offer');
        }

        setupDataChannel(channel) {
            this.dataChannel = channel;

            channel.onopen = () => {
                this.log('DataChannel opened');
                this.elements.chatInput.disabled = false;
                this.elements.sendBtn.disabled = false;
            };

            channel.onmessage = (event) => {
                this.addChatMessage('Server: ' + event.data);
            };

            channel.onclose = () => {
                this.log('DataChannel closed');
                this.elements.chatInput.disabled = true;
                this.elements.sendBtn.disabled = true;
            };
        }

        async handleSignalingMessage(msg) {
            switch (msg.type) {
                case 'offer':
                    await this.pc.setRemoteDescription({ type: 'offer', sdp: msg.sdp });
                    const answer = await this.pc.createAnswer();
                    await this.pc.setLocalDescription(answer);
                    this.ws.send(JSON.stringify({ type: 'answer', sdp: answer.sdp }));
                    this.log('Sent answer');
                    break;

                case 'answer':
                    await this.pc.setRemoteDescription({ type: 'answer', sdp: msg.sdp });
                    this.log('Received answer');
                    break;

                case 'candidate':
                    if (msg.candidate) {
                        await this.pc.addIceCandidate(msg.candidate);
                    }
                    break;

                case 'connection-state':
                    this.log('Server connection state: ' + msg.message);
                    break;

                case 'stats':
                    this.updateStats(msg.stats);
                    break;

                case 'error':
                    this.log('Server error: ' + msg.error, 'error');
                    break;
            }
        }

        updateStats(stats) {
            // Ignore server stats, use browser-side stats instead
        }

        // Collect stats from browser's RTCPeerConnection
        async collectBrowserStats() {
            if (!this.pc) return;
            try {
                const stats = await this.pc.getStats();
                let packetsReceived = 0;
                let bytesReceived = 0;
                let packetsLost = 0;
                let roundTripTime = 0;
                let framesReceived = 0;
                let framesDecoded = 0;

                stats.forEach(report => {
                    if (report.type === 'inbound-rtp' && report.kind === 'video') {
                        packetsReceived = report.packetsReceived || 0;
                        bytesReceived = report.bytesReceived || 0;
                        packetsLost = report.packetsLost || 0;
                        framesReceived = report.framesReceived || 0;
                        framesDecoded = report.framesDecoded || 0;
                    }
                    if (report.type === 'candidate-pair' && report.state === 'succeeded') {
                        roundTripTime = report.currentRoundTripTime ? report.currentRoundTripTime * 1000 : 0;
                    }
                });

                this.elements.statPackets.textContent = packetsReceived;
                this.elements.statBytes.textContent = Math.round(bytesReceived / 1024) + ' KB';
                this.elements.statRtt.textContent = roundTripTime.toFixed(1) + ' ms';
                this.elements.statLoss.textContent = packetsLost;

                // Log detailed stats periodically
                if (packetsReceived > 0 || this.lastLoggedPackets !== packetsReceived) {
                    this.log('Stats: pkts=' + packetsReceived + ' frames=' + framesReceived + ' decoded=' + framesDecoded);
                    this.lastLoggedPackets = packetsReceived;
                }
            } catch (e) {
                // Ignore stats errors
            }
        }

        startStatsCollection() {
            this.statsInterval = setInterval(() => this.collectBrowserStats(), 1000);
        }

        stopStatsCollection() {
            if (this.statsInterval) {
                clearInterval(this.statsInterval);
                this.statsInterval = null;
            }
        }

        addChatMessage(message) {
            const msgEl = document.createElement('div');
            msgEl.className = 'chat-message';
            msgEl.textContent = message;
            this.elements.chatMessages.appendChild(msgEl);
            this.elements.chatMessages.scrollTop = this.elements.chatMessages.scrollHeight;
        }

        sendChatMessage() {
            const message = this.elements.chatInput.value.trim();
            if (!message || !this.dataChannel || this.dataChannel.readyState !== 'open') return;

            this.dataChannel.send(message);
            this.addChatMessage('You: ' + message);
            this.elements.chatInput.value = '';
        }

        disconnect() {
            this.stopStatsCollection();
            if (this.dataChannel) {
                this.dataChannel.close();
                this.dataChannel = null;
            }
            if (this.pc) {
                this.pc.close();
                this.pc = null;
            }
            if (this.ws) {
                this.ws.close();
                this.ws = null;
            }

            this.setStatus('', 'Disconnected');
            this.elements.connectBtn.textContent = 'Connect';
            this.elements.connectBtn.disabled = false;
            this.elements.chatInput.disabled = true;
            this.elements.sendBtn.disabled = true;
            this.elements.video.srcObject = null;
        }
    }

    // Initialize
    const client = new WebRTCClient();
    </script>
</body>
</html>
`
