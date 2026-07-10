package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/pion/webrtc/v4"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// runPionOfferer implements the WebRTC ICE offerer path for wsSignalingHandler
// when -webrtc-ice is enabled (WO-B phase-2a).
//
// It creates a pion PeerConnection with RegisterDefaultCodecs, adds a VP8
// TrackLocalStaticRTP m-line (required for ICE+DTLS negotiation; future-proofs
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
	// ICE+DTLS negotiation requires at least one m-line; this also future-proofs
	// phase-2b (RTP frame sending).
	videoTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8},
		"video", "pulse",
	)
	if err != nil {
		log.Printf("mock-ams: ice: NewTrackLocalStaticRTP: %v", err)
		return
	}
	if _, err := pc.AddTrack(videoTrack); err != nil {
		log.Printf("mock-ams: ice: AddTrack: %v", err)
		return
	}

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
