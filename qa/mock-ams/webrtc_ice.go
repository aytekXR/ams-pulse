package main

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// runPionOfferer implements the WebRTC ICE offerer path for wsSignalingHandler
// when -webrtc-ice is enabled (WO-B phase-2a/2b).
//
// It creates a pion PeerConnection with RegisterDefaultCodecs, adds a VP8
// TrackLocalStaticRTP m-line (required for ICE+DTLS negotiation; carries the
// phase-2b RTP sending), sends a real SDP offer to the client, and then loops
// reading client messages:
//
//   - takeConfiguration/answer → SetRemoteDescription
//   - takeCandidate            → AddICECandidate
//
// Our OnICECandidate sends server-side candidates back to the client with the
// AMS-spec shape (command/streamId/label/id/candidate per
// agents/handoffs/real-ams-captures/webrtc-signaling-play-offer.json).
//
// Overall deadline: 30 s from the parent context.  pc.Close() + WS close are
// called on all exit paths (no goroutine leaks).
//
// Callers: wsSignalingHandler in main.go, after the play command is read.
// conn ownership stays with the caller; this function does not close it.
func runPionOfferer(parentCtx context.Context, conn *websocket.Conn, streamID string) {
	ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
	defer cancel()

	// ── PeerConnection setup ──────────────────────────────────────────────────
	m := &webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		log.Printf("mock-ams: ice: RegisterDefaultCodecs: %v", err)
		return
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(m))

	// Empty Configuration — no external STUN; loopback host candidates suffice
	// for in-process CI tests.
	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		log.Printf("mock-ams: ice: NewPeerConnection: %v", err)
		return
	}
	defer pc.Close()

	// VP8 TrackLocalStaticRTP — adds a sendonly video m-line to the offer.
	// ICE+DTLS negotiation requires at least one m-line; runRTPSender writes
	// the phase-2b RTP frames on this track after DTLS completes.
	videoTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8},
		"video", "pulse",
	)
	if err != nil {
		log.Printf("mock-ams: ice: NewTrackLocalStaticRTP: %v", err)
		return
	}
	// Capture sender so we can read the negotiated SSRC (phase-2b).
	sender, err := pc.AddTrack(videoTrack)
	if err != nil {
		log.Printf("mock-ams: ice: AddTrack: %v", err)
		return
	}

	// ── OnConnectionStateChange: start RTP sender after DTLS is done ─────────
	// PeerConnectionStateConnected (not ICEConnectionStateConnected) guarantees
	// DTLS is complete and SRTP keys are available before we call WriteRTP.
	// sync.Once guards against spurious re-fires of the Connected state.
	var rtpOnce sync.Once
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateConnected {
			rtpOnce.Do(func() {
				go runRTPSender(ctx, sender, videoTrack)
			})
		}
	})

	// ── OnICECandidate: forward server candidates to the client ───────────────
	// Called from a pion goroutine; nhooyr/websocket serialises concurrent
	// writes internally, so calling wsjson.Write here is safe.
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return // nil signals gathering complete
		}
		ci := c.ToJSON()
		msg := map[string]interface{}{
			"command":   "takeCandidate",
			"streamId":  streamID,
			"label":     c.SDPMLineIndex, // m-line index (uint16 → JSON integer)
			"id":        c.SDPMid,        // mid string (e.g. "0")
			"candidate": ci.Candidate,    // raw candidate string without "a="
		}
		if err := wsjson.Write(ctx, conn, msg); err != nil {
			log.Printf("mock-ams: ice: send candidate: %v", err)
		}
	})

	// ── CreateOffer → send offer → SetLocalDescription ────────────────────────
	// Send the offer BEFORE SetLocalDescription so the WS write for the offer
	// completes before ICE gathering (triggered by SetLocalDescription) fires
	// OnICECandidate concurrently.
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		log.Printf("mock-ams: ice: CreateOffer: %v", err)
		return
	}

	offerMsg := map[string]interface{}{
		"command":  "takeConfiguration",
		"streamId": streamID,
		"type":     "offer",
		"sdp":      offer.SDP,
	}
	if err := wsjson.Write(ctx, conn, offerMsg); err != nil {
		log.Printf("mock-ams: ice: send offer: %v", err)
		return
	}

	// SetLocalDescription triggers ICE gathering; OnICECandidate may fire
	// concurrently with the read loop below.
	if err := pc.SetLocalDescription(offer); err != nil {
		log.Printf("mock-ams: ice: SetLocalDescription: %v", err)
		return
	}
	log.Printf("mock-ams: ice: sent pion offer for streamId=%q", streamID)

	// ── Message loop ──────────────────────────────────────────────────────────
	// Process takeConfiguration/answer and takeCandidate messages from the
	// client until the deadline fires or the client closes the WS.
	for {
		var msg map[string]json.RawMessage
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			// Normal exit: client disconnected or 30 s deadline.
			log.Printf("mock-ams: ice: ws read exit: %v", err)
			return
		}

		var cmd string
		if raw, ok := msg["command"]; ok {
			_ = json.Unmarshal(raw, &cmd)
		}

		switch cmd {
		case "takeConfiguration":
			var msgType, sdp string
			if raw, ok := msg["type"]; ok {
				_ = json.Unmarshal(raw, &msgType)
			}
			if raw, ok := msg["sdp"]; ok {
				_ = json.Unmarshal(raw, &sdp)
			}
			if msgType != "answer" {
				continue
			}
			if err := pc.SetRemoteDescription(webrtc.SessionDescription{
				Type: webrtc.SDPTypeAnswer,
				SDP:  sdp,
			}); err != nil {
				log.Printf("mock-ams: ice: SetRemoteDescription: %v", err)
			}

		case "takeCandidate":
			var candidateStr, mid string
			var labelF float64
			if raw, ok := msg["candidate"]; ok {
				_ = json.Unmarshal(raw, &candidateStr)
			}
			if raw, ok := msg["id"]; ok {
				_ = json.Unmarshal(raw, &mid)
			}
			if raw, ok := msg["label"]; ok {
				_ = json.Unmarshal(raw, &labelF)
			}
			idx := uint16(labelF)
			midCopy := mid
			if err := pc.AddICECandidate(webrtc.ICECandidateInit{
				Candidate:     candidateStr,
				SDPMLineIndex: &idx,
				SDPMid:        &midCopy,
			}); err != nil {
				log.Printf("mock-ams: ice: AddICECandidate: %v", err)
			}
		}
	}
}

// rtpStatsHold is how long the deterministic RTP stream runs after DTLS connects.
// Meaningful WebRTC probes need TimeoutS >= 4 s (hold + ICE setup overhead).
const rtpStatsHold = 2 * time.Second

// runRTPSender sends a deterministic VP8 RTP stream for ~rtpStatsHold then
// returns. The goroutine never outlives ctx.
//
// Shared RTP spec (mock-ams sender AND prober test-helper offerer must match):
//
//   - Version=2, PayloadType=96 (VP8 default), Marker=true
//   - SequenceNumber: starts at 1, increments by 1 per packet
//   - Timestamp: starts at 0, increments by 2700 per packet (90 kHz @ 30 ms)
//   - Ticker: 30 ms; send window ~2 s (~66 packets)
//   - Payload: 64 bytes with payload[i]=byte(i)
//   - SSRC: read lazily from sender.GetParameters() after negotiation
//
// On WriteRTP error the goroutine returns; io.ErrClosedPipe after close is expected.
func runRTPSender(ctx context.Context, sender *webrtc.RTPSender, track *webrtc.TrackLocalStaticRTP) {
	// Read SSRC lazily: called after PeerConnectionStateConnected, so DTLS
	// and SDP negotiation are both complete and Encodings is populated.
	params := sender.GetParameters()
	if len(params.Encodings) == 0 {
		log.Printf("mock-ams: ice: RTP sender: no encodings in GetParameters; skipping send")
		return
	}
	ssrc := uint32(params.Encodings[0].SSRC)

	// Build fixed payload once: 64 bytes with payload[i]=byte(i).
	payload := make([]byte, 64)
	for i := range payload {
		payload[i] = byte(i)
	}

	var (
		seq uint16 = 1
		ts  uint32 = 0
	)

	ticker := time.NewTicker(30 * time.Millisecond)
	defer ticker.Stop()
	window := time.NewTimer(rtpStatsHold)
	defer window.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-window.C:
			return
		case <-ticker.C:
			pkt := &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					PayloadType:    96,
					Marker:         true,
					SequenceNumber: seq,
					Timestamp:      ts,
					SSRC:           ssrc,
				},
				Payload: payload,
			}
			if err := track.WriteRTP(pkt); err != nil {
				// io.ErrClosedPipe after close is expected; return silently.
				return
			}
			seq++
			ts += 2700
		}
	}
}
