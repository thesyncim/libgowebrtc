// Package main demonstrates device capture with browser-based device selection.
//
// This example shows:
// - Enumerating video and audio devices
// - Browser UI for device selection
// - Real-time device switching
// - WebRTC streaming to browser
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

	"github.com/gorilla/websocket"
	"github.com/thesyncim/libgowebrtc/internal/ffi"
	"github.com/thesyncim/libgowebrtc/pkg/codec"
	"github.com/thesyncim/libgowebrtc/pkg/frame"
	"github.com/thesyncim/libgowebrtc/pkg/pc"
)

var (
	addr       = flag.String("addr", ":8080", "HTTP server address")
	stunServer = flag.String("stun", "stun:stun.l.google.com:19302", "STUN server URL")
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// DeviceInfo for JSON serialization
type DeviceInfo struct {
	DeviceID string `json:"deviceId"`
	Kind     string `json:"kind"`
	Label    string `json:"label"`
}

// SignalingMessage represents a WebSocket message.
type SignalingMessage struct {
	Type       string          `json:"type"`
	SDP        string          `json:"sdp,omitempty"`
	Candidate  json.RawMessage `json:"candidate,omitempty"`
	Error      string          `json:"error,omitempty"`
	Devices    []DeviceInfo    `json:"devices,omitempty"`
	DeviceID   string          `json:"deviceId,omitempty"`
	DeviceKind string          `json:"deviceKind,omitempty"`
	Stats      *pc.RTCStats    `json:"stats,omitempty"`
	Message    string          `json:"message,omitempty"`
}

// Session manages a single WebRTC session with device capture.
type Session struct {
	conn       *websocket.Conn
	peerConn   *pc.PeerConnection
	videoTrack *pc.Track
	audioTrack *pc.Track
	cancel     context.CancelFunc
	mu         sync.Mutex
	closed     bool

	// Current device settings
	currentVideoDevice string
	currentAudioDevice string

	// Direct FFI capture handles
	videoCapture *ffi.VideoCapture
	audioCapture *ffi.AudioCapture
}

func main() {
	flag.Parse()

	http.HandleFunc("/", serveIndex)
	http.HandleFunc("/ws", handleWebSocket)

	log.Printf("Starting Device Capture Example")
	log.Printf("   Address: http://localhost%s", *addr)
	log.Printf("")
	log.Printf("Open your browser to http://localhost%s", *addr)

	if err := http.ListenAndServe(*addr, nil); err != nil {
		log.Fatal(err)
	}
}

// requestPermissions requests camera and microphone permissions (called on first connection)
var permissionsRequested bool

func requestPermissions() {
	if permissionsRequested {
		return
	}
	permissionsRequested = true

	log.Println("Checking/requesting camera permission...")
	if ffi.RequestCameraPermission() {
		log.Println("Camera permission: granted")
	} else {
		log.Println("Camera permission: denied")
		log.Println("  On macOS, grant camera access to Terminal.app or your IDE in:")
		log.Println("  System Preferences > Privacy & Security > Camera")
	}

	log.Println("Checking/requesting microphone permission...")
	if ffi.RequestMicrophonePermission() {
		log.Println("Microphone permission: granted")
	} else {
		log.Println("Microphone permission: denied")
		log.Println("  On macOS, grant microphone access to Terminal.app or your IDE in:")
		log.Println("  System Preferences > Privacy & Security > Microphone")
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

	// Request permissions on first connection (loads FFI library)
	requestPermissions()

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
	defer s.cleanup()

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

	s.setupCallbacks()

	// Handle signaling messages
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

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

func (s *Session) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.videoCapture != nil {
		s.videoCapture.Close()
		s.videoCapture = nil
	}
	if s.audioCapture != nil {
		s.audioCapture.Close()
		s.audioCapture = nil
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
}

func (s *Session) handleMessage(msg SignalingMessage) error {
	switch msg.Type {
	case "get-devices":
		return s.handleGetDevices()
	case "select-video-device":
		return s.handleSelectVideoDevice(msg.DeviceID)
	case "select-audio-device":
		return s.handleSelectAudioDevice(msg.DeviceID)
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

func (s *Session) handleGetDevices() error {
	devices, err := ffi.EnumerateDevices()
	if err != nil {
		return fmt.Errorf("enumerate devices: %w", err)
	}

	deviceInfos := make([]DeviceInfo, len(devices))
	for i, d := range devices {
		var kind string
		switch d.Kind {
		case ffi.DeviceKindVideoInput:
			kind = "videoinput"
		case ffi.DeviceKindAudioInput:
			kind = "audioinput"
		case ffi.DeviceKindAudioOutput:
			kind = "audiooutput"
		}
		deviceInfos[i] = DeviceInfo{
			DeviceID: d.DeviceID,
			Kind:     kind,
			Label:    d.Label,
		}
	}

	log.Printf("Found %d devices:", len(devices))
	for _, d := range deviceInfos {
		log.Printf("  - %s: %s (%s)", d.Kind, d.Label, d.DeviceID)
	}

	return s.sendJSON(SignalingMessage{
		Type:    "devices",
		Devices: deviceInfos,
	})
}

func (s *Session) handleSelectVideoDevice(deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Stop existing video capture
	if s.videoCapture != nil {
		s.videoCapture.Close()
		s.videoCapture = nil
	}

	if deviceID == "" {
		log.Printf("Video device deselected")
		return nil
	}

	log.Printf("Starting video capture from device: %s", deviceID)

	// Create video track if not exists
	if s.videoTrack == nil {
		track, err := s.peerConn.CreateVideoTrack("video0", codec.VP8, 1280, 720)
		if err != nil {
			return fmt.Errorf("create video track: %w", err)
		}
		s.videoTrack = track

		if _, err := s.peerConn.AddTrack(track); err != nil {
			return fmt.Errorf("add track: %w", err)
		}
	}

	// Create video capture
	capture, err := ffi.NewVideoCapture(deviceID, 1280, 720, 30)
	if err != nil {
		return fmt.Errorf("create video capture: %w", err)
	}

	// Start capture with callback that writes to track
	err = capture.Start(func(captured *ffi.CapturedVideoFrame) {
		// Convert to frame.VideoFrame
		videoFrame := &frame.VideoFrame{
			Width:  int(captured.Width),
			Height: int(captured.Height),
			Format: frame.PixelFormatI420,
			Data:   [][]byte{captured.YPlane, captured.UPlane, captured.VPlane},
			Stride: []int{int(captured.YStride), int(captured.UStride), int(captured.VStride)},
		}

		// Write to track
		if err := s.videoTrack.WriteVideoFrame(videoFrame); err != nil {
			// Ignore "track source not initialized" errors (before connection)
			if err.Error() != "track source not initialized" {
				log.Printf("Write frame error: %v", err)
			}
		}
	})

	if err != nil {
		capture.Close()
		return fmt.Errorf("start capture: %w", err)
	}

	s.videoCapture = capture
	s.currentVideoDevice = deviceID

	log.Printf("Video capture started")
	return s.sendJSON(SignalingMessage{
		Type:       "device-selected",
		DeviceKind: "video",
		DeviceID:   deviceID,
	})
}

func (s *Session) handleSelectAudioDevice(deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Stop existing audio capture
	if s.audioCapture != nil {
		s.audioCapture.Close()
		s.audioCapture = nil
	}

	if deviceID == "" {
		log.Printf("Audio device deselected")
		return nil
	}

	log.Printf("Starting audio capture from device: %s", deviceID)

	// Create audio track if not exists
	if s.audioTrack == nil {
		track, err := s.peerConn.CreateAudioTrackWithOptions("audio0", 48000, 2)
		if err != nil {
			return fmt.Errorf("create audio track: %w", err)
		}
		s.audioTrack = track

		if _, err := s.peerConn.AddTrack(track); err != nil {
			return fmt.Errorf("add track: %w", err)
		}
	}

	// Create audio capture
	capture, err := ffi.NewAudioCapture(deviceID, 48000, 2)
	if err != nil {
		return fmt.Errorf("create audio capture: %w", err)
	}

	// Start capture with callback that writes to track
	err = capture.Start(func(captured *ffi.CapturedAudioFrame) {
		// Convert to frame.AudioFrame
		audioFrame := frame.NewAudioFrameFromS16(
			captured.Samples,
			int(captured.SampleRate),
			int(captured.NumChannels),
		)

		// Write to track
		if err := s.audioTrack.WriteAudioFrame(audioFrame); err != nil {
			if err.Error() != "track source not initialized" {
				log.Printf("Write audio error: %v", err)
			}
		}
	})

	if err != nil {
		capture.Close()
		return fmt.Errorf("start capture: %w", err)
	}

	s.audioCapture = capture
	s.currentAudioDevice = deviceID

	log.Printf("Audio capture started")
	return s.sendJSON(SignalingMessage{
		Type:       "device-selected",
		DeviceKind: "audio",
		DeviceID:   deviceID,
	})
}

func (s *Session) handleOffer(sdp string) error {
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

	if err := s.peerConn.SetLocalDescription(offer); err != nil {
		return fmt.Errorf("set local description: %w", err)
	}

	return s.sendJSON(SignalingMessage{
		Type: "offer",
		SDP:  offer.SDP,
	})
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

const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Device Capture - libgowebrtc</title>
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
            max-width: 1400px;
            margin: 0 auto;
        }
        header {
            text-align: center;
            margin-bottom: 30px;
        }
        h1 {
            font-size: 2.5rem;
            margin-bottom: 10px;
            background: linear-gradient(90deg, #ff6b6b, #feca57);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
        }
        .subtitle {
            color: #888;
            font-size: 1.1rem;
        }
        .main-content {
            display: grid;
            grid-template-columns: 300px 1fr;
            gap: 20px;
        }
        @media (max-width: 900px) {
            .main-content {
                grid-template-columns: 1fr;
            }
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
            display: flex;
            align-items: center;
            gap: 8px;
        }
        .card h3 .icon {
            font-size: 1.2rem;
        }
        .device-list {
            display: flex;
            flex-direction: column;
            gap: 8px;
        }
        .device-item {
            display: flex;
            align-items: center;
            gap: 10px;
            padding: 12px;
            background: rgba(0,0,0,0.2);
            border-radius: 8px;
            cursor: pointer;
            transition: all 0.2s;
            border: 2px solid transparent;
        }
        .device-item:hover {
            background: rgba(255,255,255,0.1);
        }
        .device-item.selected {
            border-color: #ff6b6b;
            background: rgba(255,107,107,0.1);
        }
        .device-item input[type="radio"] {
            accent-color: #ff6b6b;
            width: 18px;
            height: 18px;
        }
        .device-name {
            flex: 1;
            font-size: 0.9rem;
            overflow: hidden;
            text-overflow: ellipsis;
            white-space: nowrap;
        }
        .device-id {
            font-size: 0.7rem;
            color: #666;
            font-family: monospace;
        }
        .video-section {
            display: flex;
            flex-direction: column;
            gap: 20px;
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
        .controls {
            display: flex;
            gap: 10px;
            flex-wrap: wrap;
        }
        button {
            padding: 12px 24px;
            font-size: 1rem;
            font-weight: 600;
            border: none;
            border-radius: 8px;
            cursor: pointer;
            transition: all 0.2s;
        }
        .btn-primary {
            background: linear-gradient(90deg, #ff6b6b, #feca57);
            color: #000;
        }
        .btn-primary:hover {
            transform: translateY(-2px);
            box-shadow: 0 4px 12px rgba(255, 107, 107, 0.3);
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
            color: #ff6b6b;
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
        .no-devices {
            color: #888;
            font-style: italic;
            padding: 20px;
            text-align: center;
        }
        .refresh-btn {
            background: none;
            border: none;
            color: #888;
            cursor: pointer;
            padding: 4px;
            font-size: 1rem;
        }
        .refresh-btn:hover {
            color: #fff;
        }
        .audio-indicator {
            display: flex;
            align-items: center;
            gap: 4px;
            height: 20px;
        }
        .audio-bar {
            width: 4px;
            background: #4caf50;
            border-radius: 2px;
            transition: height 0.1s;
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>Device Capture</h1>
            <p class="subtitle">Select video and audio devices to stream via WebRTC</p>
        </header>

        <div class="main-content">
            <div class="sidebar">
                <div class="card">
                    <h3>
                        <span class="icon">📹</span> Video Devices
                        <button class="refresh-btn" onclick="client.refreshDevices()" title="Refresh">🔄</button>
                    </h3>
                    <div class="device-list" id="videoDevices">
                        <div class="no-devices">Loading devices...</div>
                    </div>
                </div>

                <div class="card">
                    <h3>
                        <span class="icon">🎤</span> Audio Devices
                    </h3>
                    <div class="device-list" id="audioDevices">
                        <div class="no-devices">Loading devices...</div>
                    </div>
                </div>

                <div class="card">
                    <h3><span class="icon">📊</span> Statistics</h3>
                    <div class="stats-grid">
                        <div class="stat-item">
                            <div class="stat-label">Video Frames</div>
                            <div class="stat-value" id="statFrames">0</div>
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

                <div class="card">
                    <h3><span class="icon">📝</span> Log</h3>
                    <div class="log-container" id="logContainer"></div>
                </div>
            </div>

            <div class="video-section">
                <div class="video-container">
                    <video id="remoteVideo" autoplay playsinline></video>
                    <div class="overlay">
                        <div class="status">
                            <div class="status-dot" id="statusDot"></div>
                            <span id="statusText">Disconnected</span>
                        </div>
                    </div>
                </div>

                <div class="controls">
                    <button id="connectBtn" class="btn-primary">Connect</button>
                    <button id="startBtn" class="btn-secondary" disabled>Start Stream</button>
                </div>
            </div>
        </div>
    </div>

    <script>
    class DeviceCaptureClient {
        constructor() {
            this.ws = null;
            this.pc = null;
            this.selectedVideoDevice = null;
            this.selectedAudioDevice = null;
            this.devices = [];

            this.elements = {
                video: document.getElementById('remoteVideo'),
                statusDot: document.getElementById('statusDot'),
                statusText: document.getElementById('statusText'),
                connectBtn: document.getElementById('connectBtn'),
                startBtn: document.getElementById('startBtn'),
                videoDevices: document.getElementById('videoDevices'),
                audioDevices: document.getElementById('audioDevices'),
                logContainer: document.getElementById('logContainer'),
                statFrames: document.getElementById('statFrames'),
                statBytes: document.getElementById('statBytes'),
                statRtt: document.getElementById('statRtt'),
                statLoss: document.getElementById('statLoss'),
            };

            this.setupEventListeners();
        }

        setupEventListeners() {
            this.elements.connectBtn.addEventListener('click', () => this.toggleConnection());
            this.elements.startBtn.addEventListener('click', () => this.startStream());
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
            this.log('Connecting to server...');
            this.setStatus('connecting', 'Connecting...');
            this.elements.connectBtn.textContent = 'Connecting...';
            this.elements.connectBtn.disabled = true;

            try {
                const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
                this.ws = new WebSocket(protocol + '//' + window.location.host + '/ws');

                this.ws.onopen = () => {
                    this.log('Connected to server');
                    this.setStatus('connected', 'Connected');
                    this.elements.connectBtn.textContent = 'Disconnect';
                    this.elements.connectBtn.disabled = false;
                    this.elements.startBtn.disabled = false;

                    // Request device list
                    this.refreshDevices();
                };

                this.ws.onmessage = (event) => {
                    const msg = JSON.parse(event.data);
                    this.handleMessage(msg);
                };

                this.ws.onclose = () => {
                    this.log('Disconnected from server', 'warn');
                    this.disconnect();
                };

                this.ws.onerror = (error) => {
                    this.log('WebSocket error', 'error');
                };
            } catch (error) {
                this.log('Connection failed: ' + error.message, 'error');
                this.disconnect();
            }
        }

        refreshDevices() {
            if (this.ws && this.ws.readyState === WebSocket.OPEN) {
                this.ws.send(JSON.stringify({ type: 'get-devices' }));
            }
        }

        handleMessage(msg) {
            switch (msg.type) {
                case 'devices':
                    this.handleDevices(msg.devices);
                    break;
                case 'device-selected':
                    this.log('Device selected: ' + msg.deviceKind + ' - ' + msg.deviceId);
                    break;
                case 'offer':
                    this.handleOffer(msg.sdp);
                    break;
                case 'answer':
                    this.handleAnswer(msg.sdp);
                    break;
                case 'candidate':
                    this.handleCandidate(msg.candidate);
                    break;
                case 'connection-state':
                    this.log('Server connection: ' + msg.message);
                    break;
                case 'error':
                    this.log('Error: ' + msg.error, 'error');
                    break;
            }
        }

        handleDevices(devices) {
            this.devices = devices;
            this.log('Found ' + devices.length + ' devices');

            const videoDevices = devices.filter(d => d.kind === 'videoinput');
            const audioDevices = devices.filter(d => d.kind === 'audioinput');

            // Render video devices
            if (videoDevices.length === 0) {
                this.elements.videoDevices.innerHTML = '<div class="no-devices">No video devices found</div>';
            } else {
                this.elements.videoDevices.innerHTML = videoDevices.map(function(d, i) {
                    return '<label class="device-item" data-device-id="' + d.deviceId + '">' +
                        '<input type="radio" name="videoDevice" value="' + d.deviceId + '">' +
                        '<div>' +
                            '<div class="device-name">' + (d.label || 'Camera ' + (i + 1)) + '</div>' +
                            '<div class="device-id">' + d.deviceId.substring(0, 20) + '...</div>' +
                        '</div>' +
                    '</label>';
                }).join('');

                // Add click handlers
                this.elements.videoDevices.querySelectorAll('.device-item').forEach(item => {
                    item.addEventListener('click', () => this.selectVideoDevice(item.dataset.deviceId));
                });
            }

            // Render audio devices
            if (audioDevices.length === 0) {
                this.elements.audioDevices.innerHTML = '<div class="no-devices">No audio devices found</div>';
            } else {
                this.elements.audioDevices.innerHTML = audioDevices.map(function(d, i) {
                    return '<label class="device-item" data-device-id="' + d.deviceId + '">' +
                        '<input type="radio" name="audioDevice" value="' + d.deviceId + '">' +
                        '<div>' +
                            '<div class="device-name">' + (d.label || 'Microphone ' + (i + 1)) + '</div>' +
                            '<div class="device-id">' + d.deviceId.substring(0, 20) + '...</div>' +
                        '</div>' +
                    '</label>';
                }).join('');

                // Add click handlers
                this.elements.audioDevices.querySelectorAll('.device-item').forEach(item => {
                    item.addEventListener('click', () => this.selectAudioDevice(item.dataset.deviceId));
                });
            }
        }

        selectVideoDevice(deviceId) {
            this.selectedVideoDevice = deviceId;
            this.log('Selected video device: ' + deviceId.substring(0, 20) + '...');

            // Update UI
            this.elements.videoDevices.querySelectorAll('.device-item').forEach(item => {
                item.classList.toggle('selected', item.dataset.deviceId === deviceId);
                item.querySelector('input').checked = item.dataset.deviceId === deviceId;
            });

            // Send to server
            if (this.ws && this.ws.readyState === WebSocket.OPEN) {
                this.ws.send(JSON.stringify({
                    type: 'select-video-device',
                    deviceId: deviceId
                }));
            }
        }

        selectAudioDevice(deviceId) {
            this.selectedAudioDevice = deviceId;
            this.log('Selected audio device: ' + deviceId.substring(0, 20) + '...');

            // Update UI
            this.elements.audioDevices.querySelectorAll('.device-item').forEach(item => {
                item.classList.toggle('selected', item.dataset.deviceId === deviceId);
                item.querySelector('input').checked = item.dataset.deviceId === deviceId;
            });

            // Send to server
            if (this.ws && this.ws.readyState === WebSocket.OPEN) {
                this.ws.send(JSON.stringify({
                    type: 'select-audio-device',
                    deviceId: deviceId
                }));
            }
        }

        async startStream() {
            if (!this.selectedVideoDevice && !this.selectedAudioDevice) {
                this.log('Please select at least one device', 'warn');
                return;
            }

            this.log('Starting WebRTC stream...');
            await this.setupPeerConnection();
        }

        async setupPeerConnection() {
            this.pc = new RTCPeerConnection({
                iceServers: [{ urls: 'stun:stun.l.google.com:19302' }]
            });

            this.pc.ontrack = (event) => {
                this.log('Received track: ' + event.track.kind);
                if (event.streams && event.streams[0]) {
                    this.elements.video.srcObject = event.streams[0];
                }
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
                this.log('WebRTC state: ' + this.pc.connectionState);
                if (this.pc.connectionState === 'connected') {
                    this.setStatus('connected', 'Streaming');
                    this.startStatsCollection();
                }
            };

            // Add transceivers for receiving
            this.pc.addTransceiver('video', { direction: 'recvonly' });
            this.pc.addTransceiver('audio', { direction: 'recvonly' });

            // Create and send offer
            const offer = await this.pc.createOffer();
            await this.pc.setLocalDescription(offer);

            this.ws.send(JSON.stringify({
                type: 'offer',
                sdp: offer.sdp
            }));

            this.log('Sent offer to server');
        }

        async handleOffer(sdp) {
            if (!this.pc) {
                await this.setupPeerConnection();
            }
            await this.pc.setRemoteDescription({ type: 'offer', sdp: sdp });
            const answer = await this.pc.createAnswer();
            await this.pc.setLocalDescription(answer);
            this.ws.send(JSON.stringify({ type: 'answer', sdp: answer.sdp }));
        }

        async handleAnswer(sdp) {
            await this.pc.setRemoteDescription({ type: 'answer', sdp: sdp });
            this.log('Received answer from server');
        }

        async handleCandidate(candidate) {
            if (this.pc && candidate) {
                await this.pc.addIceCandidate(candidate);
            }
        }

        startStatsCollection() {
            this.statsInterval = setInterval(() => this.collectStats(), 1000);
        }

        async collectStats() {
            if (!this.pc) return;
            try {
                const stats = await this.pc.getStats();
                let framesReceived = 0;
                let bytesReceived = 0;
                let packetsLost = 0;
                let roundTripTime = 0;

                stats.forEach(report => {
                    if (report.type === 'inbound-rtp' && report.kind === 'video') {
                        framesReceived = report.framesReceived || 0;
                        bytesReceived = report.bytesReceived || 0;
                        packetsLost = report.packetsLost || 0;
                    }
                    if (report.type === 'candidate-pair' && report.state === 'succeeded') {
                        roundTripTime = report.currentRoundTripTime ? report.currentRoundTripTime * 1000 : 0;
                    }
                });

                this.elements.statFrames.textContent = framesReceived;
                this.elements.statBytes.textContent = Math.round(bytesReceived / 1024) + ' KB';
                this.elements.statRtt.textContent = roundTripTime.toFixed(1) + ' ms';
                this.elements.statLoss.textContent = packetsLost;
            } catch (e) {
                // Ignore stats errors
            }
        }

        disconnect() {
            if (this.statsInterval) {
                clearInterval(this.statsInterval);
                this.statsInterval = null;
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
            this.elements.startBtn.disabled = true;
            this.elements.video.srcObject = null;
            this.selectedVideoDevice = null;
            this.selectedAudioDevice = null;
        }
    }

    // Initialize
    const client = new DeviceCaptureClient();
    </script>
</body>
</html>
`
