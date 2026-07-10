package prober

// probe_webrtc_ice.go — phase-2a/2b ICE + RTP-stats path for the WebRTC probe.
//
// continueWebRTCICE is called by probeWebRTC (prober.go) immediately after
// signaling succeeds (offer received).  It:
//   1. Creates a pion PeerConnection (pure-Go ANSWERER, no external STUN —
//      host candidates suffice for same-host and CI container-network checks).
//   2. Registers pc.OnTrack with a drain goroutine (empties the inbound RTP
//      pipeline so pion's built-in stats interceptor can count packets) and
//      captures the inbound SSRC for later stats look-up.
//   3. SetRemoteDescription(offer) → CreateAnswer → SetLocalDescription.
//   4. Sends {"command":"takeConfiguration","type":"answer","sdp":...} on the
//      still-open WS connection.
//   5. Exchanges trickle ICE candidates both ways:
//      server → client via takeCandidate messages (AddICECandidate),
//      client → server via OnICECandidate (wsjson.Write takeCandidate).
//   6. Waits on OnICEConnectionStateChange until a terminal state or ctx deadline.
//   7. On connected: holds for rtpStatsHold (2 s) to accumulate RTP stats, then
//      collects RttMs / JitterMs / LossPct from pc.GetStats() (phase-2b, D-075).
//      If ctx expires during the hold the probe returns with IceState="connected"
//      and all three stats nil.
//
// Outcome semantics (D-074 binding):
//   connected/completed → IceState="connected"  (ErrorCode unchanged, stays "")
//   failed              → IceState="failed"      ErrorCode="ice_failed"
//   ctx deadline first  → IceState="timeout"     ErrorCode="ice_timeout"
//   Success always stays TRUE (signaling succeeded — bonus-measurement semantics,
//   same philosophy as HLS segment and DASH segment being bonus over manifest).
//   ICE/stats outcome NEVER flips result.Success (bonus-measurement rule).
//
// If the WS becomes unavailable before we can even begin ICE (e.g., the server
// closed the connection immediately after the offer), continueWebRTCICE returns
// the result unchanged (IceState="", no additional ErrorCode) — ICE was not
// attempted, and the phase-1 signaling result is preserved.

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	webrtc "github.com/pion/webrtc/v4"
	nhws "nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// rtpStatsHold is the duration the probe waits after ICE connects to accumulate
// enough RTP packets for meaningful jitter / loss measurements.
//
// NOTE: meaningful WebRTC probes need TimeoutS >= 4 s (allow ~1 s for ICE +
// 2 s hold + scheduler margin).  Probes with TimeoutS < 4 s may return with
// stats absent if the context expires during the hold.
const rtpStatsHold = 2 * time.Second

// testRTPStatsHoldOverride lets tests substitute a different hold duration
// without changing the production constant.  Zero means "use rtpStatsHold".
// Stored as nanoseconds in an atomic so a test's write and a probe goroutine's
// read never race, whatever the test ordering (D-075 verifier finding).
var testRTPStatsHoldOverride atomic.Int64

// continueWebRTCICE advances the WebRTC probe from signaling into ICE
// negotiation.  conn is the still-open WS connection from probeWebRTC;
// streamID and offerSDP come from the parsed takeConfiguration/offer message.
// result already has Success=true, SignalingState="offer_received", and
// ConnectTimeMs set; this function annotates it with IceState (and ErrorCode
// on ICE failure/timeout) and phase-2b RTP stats (D-075).
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
	// No explicit interceptor registry: webrtc.NewAPI registers default
	// interceptors automatically (including the stats interceptor), so
	// pc.GetStats() returns live InboundRTPStreamStats — path (a), D-075 SPIKE.
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

	// ── 3. OnTrack drain goroutine + SSRC capture (phase-2b, D-075) ──────────
	//
	// Registered BEFORE SetRemoteDescription so no track events are missed.
	// Two purposes:
	//   a. Drains incoming RTP so pion's built-in stats interceptor can count
	//      packets and compute jitter (ReadRTP() feeds the interceptor pipeline).
	//   b. Captures the inbound SSRC for the stats look-up after the hold.
	//      thread-safe handoff: buffered channel (size 1), non-blocking send;
	//      the select loop reads it after the hold on the same goroutine.
	// The goroutine exits when pc.Close() tears down the track (ReadRTP returns
	// an error — io.ErrClosedPipe / io.EOF are expected on teardown).
	ssrcCh := make(chan uint32, 1)
	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		// Non-blocking: first track wins; subsequent tracks (if any) are ignored
		// for SSRC purposes but still drained below.
		select {
		case ssrcCh <- uint32(track.SSRC()):
		default:
		}
		go func() {
			for {
				if _, _, err := track.ReadRTP(); err != nil {
					return // pc.Close() → SRTP teardown → error; exit cleanly
				}
			}
		}()
	})

	// ── 4. Wire our candidate sender ──────────────────────────────────────────

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

	// ── 5. SDP exchange ───────────────────────────────────────────────────────

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

	// ── 6. Reader goroutine: incoming takeCandidate from server ───────────────

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

	// ── 7. Wait for terminal ICE state or context deadline ────────────────────

	for {
		select {
		case <-ctx.Done():
			// Context deadline fired before ICE reached a terminal state.
			// The failed/timeout ICE paths do NOT hold (bonus-measurement rule).
			result.IceState = "timeout"
			result.ErrorCode = "ice_timeout"
			return result

		case state := <-iceStateCh:
			switch state {
			case webrtc.ICEConnectionStateConnected, webrtc.ICEConnectionStateCompleted:
				result.IceState = "connected"
				// ── Phase-2b stats hold (D-075) ──────────────────────────────
				// Hold for rtpStatsHold to let RTP packets accumulate for jitter
				// and loss.  If ctx expires during the hold, return immediately
				// with IceState=connected and stats nil (context-bounded hold).
				// The failed/timeout paths above do NOT hold.
				hold := rtpStatsHold
				if o := time.Duration(testRTPStatsHoldOverride.Load()); o > 0 {
					hold = o
				}
				select {
				case <-ctx.Done():
					// ctx expired during hold — stats absent, but ICE was connected.
					return result
				case <-time.After(hold):
					// Hold complete; collect stats.
				}
				result = collectWebRTCStats(pc, ssrcCh, result)
				return result

			case webrtc.ICEConnectionStateFailed:
				// The failed path does NOT hold (bonus-measurement rule).
				result.IceState = "failed"
				result.ErrorCode = "ice_failed"
				return result
			}
			// Any other state (Disconnected, Closed, etc.) — keep waiting.
		}
	}
}

// collectWebRTCStats reads RTP/ICE statistics from the answerer PeerConnection
// and populates result.RttMs, result.JitterMs, and result.LossPct.
//
// Stats rules (D-075 DESIGN DECISIONS):
//   - RttMs:    set only when the selected ICE pair's CurrentRoundTripTime > 0
//     (seconds → ms × 1000); nominated pair preferred, fallback to any.
//   - JitterMs: set only when packetsReceived + packetsLost > 0;
//     pion reports Jitter in seconds → ms × 1000.
//   - LossPct:  set only when packetsReceived + packetsLost > 0; clamped >= 0
//     because PacketsLost can be negative per RFC 3550 (duplicates).
//   - ICE/stats outcome NEVER flips result.Success.
func collectWebRTCStats(pc *webrtc.PeerConnection, ssrcCh <-chan uint32, result domain.ProbeResult) domain.ProbeResult {
	report := pc.GetStats()

	// ── RTT from nominated ICE candidate pair ─────────────────────────────────
	// First pass: nominated pair with CurrentRoundTripTime > 0.
	for _, v := range report {
		if s, ok := v.(webrtc.ICECandidatePairStats); ok {
			if s.Nominated && s.CurrentRoundTripTime > 0 {
				rtt := float32(s.CurrentRoundTripTime * 1000)
				result.RttMs = &rtt
				break
			}
		}
	}
	// Second pass (fallback): any pair with CurrentRoundTripTime > 0.
	if result.RttMs == nil {
		for _, v := range report {
			if s, ok := v.(webrtc.ICECandidatePairStats); ok {
				if s.CurrentRoundTripTime > 0 {
					rtt := float32(s.CurrentRoundTripTime * 1000)
					result.RttMs = &rtt
					break
				}
			}
		}
	}

	// ── Jitter + loss from InboundRTPStreamStats ──────────────────────────────
	// Drain the SSRC channel to get the inbound SSRC captured in OnTrack.
	// Non-blocking: if OnTrack hasn't fired (no RTP track negotiated) ssrc=0.
	var ssrc uint32
	select {
	case ssrc = <-ssrcCh:
	default:
	}

	for _, v := range report {
		s, ok := v.(webrtc.InboundRTPStreamStats)
		if !ok {
			continue
		}
		// If we captured the SSRC, only use the matching stream entry.
		if ssrc != 0 && uint32(s.SSRC) != ssrc {
			continue
		}
		// Only populate stats when there is evidence of packets.
		// PacketsLost (int32) can be negative per RFC 3550 (when duplicates
		// inflate received count); clamp to 0 before computing the total.
		received := int64(s.PacketsReceived)
		lost := int64(s.PacketsLost)
		if lost < 0 {
			lost = 0
		}
		if received+lost > 0 {
			// Jitter: pion's stats interceptor computes inter-arrival jitter
			// in seconds (RFC 3550 § 6.4.1 formula, divided by clock rate).
			// Convert to milliseconds.
			jitter := float32(s.Jitter * 1000)
			result.JitterMs = &jitter
			// Loss percent: clamped >= 0 (lost already clamped above).
			lossPct := float32(float64(lost) / float64(received+lost) * 100)
			result.LossPct = &lossPct
		}
		break // only one inbound stream expected for a single VP8 track
	}

	return result
}
