package prober

// probe_webrtc_ice.go — phase-2a ICE path for the WebRTC probe.
//
// continueWebRTCICE is called by probeWebRTC (prober.go) immediately after
// signaling succeeds (offer received).  It:
//   1. Creates a pion PeerConnection (pure-Go ANSWERER, no external STUN —
//      host candidates suffice for same-host and CI container-network checks).
//   2. SetRemoteDescription(offer) → CreateAnswer → SetLocalDescription.
//   3. Sends {"command":"takeConfiguration","type":"answer","sdp":...} on the
//      still-open WS connection.
//   4. Exchanges trickle ICE candidates both ways:
//      server → client via takeCandidate messages (AddICECandidate),
//      client → server via OnICECandidate (wsjson.Write takeCandidate).
//   5. Waits on OnICEConnectionStateChange until a terminal state or ctx deadline.
//
// Outcome semantics (D-074 binding):
//   connected/completed → IceState="connected"  (ErrorCode unchanged, stays "")
//   failed              → IceState="failed"      ErrorCode="ice_failed"
//   ctx deadline first  → IceState="timeout"     ErrorCode="ice_timeout"
//   Success always stays TRUE (signaling succeeded — bonus-measurement semantics,
//   same philosophy as HLS segment and DASH segment being bonus over manifest).
//
// If the WS becomes unavailable before we can even begin ICE (e.g., the server
// closed the connection immediately after the offer), continueWebRTCICE returns
// the result unchanged (IceState="", no additional ErrorCode) — ICE was not
// attempted, and the phase-1 signaling result is preserved.

import (
	"context"
	"sync"

	webrtc "github.com/pion/webrtc/v4"
	nhws "nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// continueWebRTCICE advances the WebRTC probe from signaling into ICE
// negotiation.  conn is the still-open WS connection from probeWebRTC;
// streamID and offerSDP come from the parsed takeConfiguration/offer message.
// result already has Success=true, SignalingState="offer_received", and
// ConnectTimeMs set; this function annotates it with IceState (and ErrorCode
// on ICE failure/timeout).
func continueWebRTCICE(
	ctx context.Context,
	conn *nhws.Conn,
	streamID string,
	offerSDP string,
	result domain.ProbeResult,
) domain.ProbeResult {
	// ── 1. Create pion PeerConnection (ANSWERER) ──────────────────────────────

	m := &webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		// Codec registration should never fail — treat as ICE setup failure.
		return result // IceState stays "" (ICE not attempted)
	}
	api := webrtc.NewAPI(webrtc.WithMediaEngine(m))
	pc, err := api.NewPeerConnection(webrtc.Configuration{}) // no ICEServer = host-only
	if err != nil {
		return result // IceState stays "" (ICE not attempted)
	}
	defer pc.Close() //nolint:errcheck

	// ── 2. Wire ICE state callback ─────────────────────────────────────────────

	// Buffered channel: pion may fire multiple state-change events rapidly.
	// We read from it in the main select loop below.
	iceStateCh := make(chan webrtc.ICEConnectionState, 8)
	var iceOnce sync.Once // ensure only the first terminal state is recorded
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		switch state {
		case webrtc.ICEConnectionStateConnected,
			webrtc.ICEConnectionStateCompleted,
			webrtc.ICEConnectionStateFailed:
			// Only send terminal states; buffer avoids blocking pion's internal goroutine.
			iceOnce.Do(func() {
				select {
				case iceStateCh <- state:
				default:
				}
			})
		}
	})

	// ── 3. Wire our candidate sender ──────────────────────────────────────────

	// innerCtx is used for WS I/O during ICE so we can cancel it without
	// affecting the caller's ctx.  It is cancelled by the deferred cleanup below.
	innerCtx, innerCancel := context.WithCancel(ctx)

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return // end-of-candidates signal; no-op (trickle ICE)
		}
		init := c.ToJSON()
		label := 0
		id := "0"
		if init.SDPMLineIndex != nil {
			label = int(*init.SDPMLineIndex)
		}
		if init.SDPMid != nil {
			id = *init.SDPMid
		}
		candidateMsg := map[string]interface{}{
			"command":   "takeCandidate",
			"streamId":  streamID,
			"label":     label,
			"id":        id,
			"candidate": init.Candidate,
		}
		// Write errors here are non-fatal (WS may close while ICE is in flight).
		_ = wsjson.Write(innerCtx, conn, candidateMsg)
	})

	// ── 4. SDP exchange ───────────────────────────────────────────────────────

	if err := pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offerSDP,
	}); err != nil {
		innerCancel()
		return result // IceState stays "" (ICE not attempted)
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		innerCancel()
		return result // IceState stays ""
	}

	if err := pc.SetLocalDescription(answer); err != nil {
		innerCancel()
		return result // IceState stays ""
	}

	// Send our answer to the server.  If the WS is already closed (e.g., the
	// server closed the connection immediately after the offer), this fails and
	// we return the signaling-only result unchanged — ICE was never attempted.
	answerMsg := map[string]interface{}{
		"command":  "takeConfiguration",
		"streamId": streamID,
		"type":     "answer",
		"sdp":      answer.SDP,
	}
	if err := wsjson.Write(ctx, conn, answerMsg); err != nil {
		innerCancel()
		return result // IceState stays "" — ICE not attempted (WS gone)
	}

	// ── 5. Reader goroutine: incoming takeCandidate from server ───────────────

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for {
			var msg wsSignalingMsg
			if err := wsjson.Read(innerCtx, conn, &msg); err != nil {
				return // innerCtx cancelled or WS closed — exit cleanly
			}
			if msg.Command == "takeCandidate" {
				sdpMid := msg.ID
				idx := uint16(msg.Label)
				if err := pc.AddICECandidate(webrtc.ICECandidateInit{
					Candidate:     msg.Candidate,
					SDPMid:        &sdpMid,
					SDPMLineIndex: &idx,
				}); err != nil {
					// Non-fatal: candidate may be redundant or the ICE agent
					// may have already closed — ignore and keep reading.
					continue
				}
			}
		}
	}()

	// Cleanup: cancel innerCtx first (stops goroutine), then wait for it.
	// The two-step order is critical: cancel → goroutine exits → readDone closes.
	// LIFO defer order: defer B fires before defer A.
	//   defer A: <-readDone  (registered first → fires last, after cancel)
	//   defer B: innerCancel (registered second → fires first)
	defer func() { <-readDone }()
	defer innerCancel()

	// ── 6. Wait for terminal ICE state or context deadline ────────────────────

	for {
		select {
		case <-ctx.Done():
			result.IceState = "timeout"
			result.ErrorCode = "ice_timeout"
			return result

		case state := <-iceStateCh:
			switch state {
			case webrtc.ICEConnectionStateConnected, webrtc.ICEConnectionStateCompleted:
				result.IceState = "connected"
				return result
			case webrtc.ICEConnectionStateFailed:
				result.IceState = "failed"
				result.ErrorCode = "ice_failed"
				return result
			}
			// Any other state (Disconnected, Closed, etc.) — keep waiting.
		}
	}
}
