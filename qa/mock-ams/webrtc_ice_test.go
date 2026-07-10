package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// newTestServerICE creates a test server with -webrtc-ice=true.
// The caller is responsible for calling ts.Close().
func newTestServerICE(t *testing.T) (*httptest.Server, *State) {
	t.Helper()
	cfg := Config{AppName: "live", WebRTCICE: true}
	state := NewState(cfg.AppName)
	srv := NewServer(cfg, state)
	return httptest.NewServer(srv), state
}

// TestWSSignaling_ICELoopback is the TDD loopback ICE test (WO-B phase-2a).
// An in-process pion CLIENT acting as the answerer dials the enabled handler
// over an httptest WS, completes signaling + candidate exchange, and asserts
// that the client-side ICE connection state reaches connected within 15 s.
func TestWSSignaling_ICELoopback(t *testing.T) {
	ts, _ := newTestServerICE(t)
	defer ts.Close()

	// 20 s overall budget; the assertion deadline is 15 s.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/live/websocket"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial %s: %v", wsURL, err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")

	// ── Create pion answerer (client PC) ──────────────────────────────────────
	m := &webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		t.Fatalf("RegisterDefaultCodecs: %v", err)
	}
	api := webrtc.NewAPI(webrtc.WithMediaEngine(m))
	clientPC, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("client NewPeerConnection: %v", err)
	}
	defer clientPC.Close()

	// Channel signalling ICE-connected.
	connected := make(chan struct{}, 1)
	clientPC.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		t.Logf("client ICE state: %s", state)
		if state == webrtc.ICEConnectionStateConnected ||
			state == webrtc.ICEConnectionStateCompleted {
			select {
			case connected <- struct{}{}:
			default:
			}
		}
	})

	// ── Send play command ─────────────────────────────────────────────────────
	play := map[string]interface{}{
		"command":  "play",
		"streamId": "ice-loopback",
		"token":    "",
	}
	if err := wsjson.Write(ctx, conn, play); err != nil {
		t.Fatalf("write play: %v", err)
	}

	// ── Receive server offer (skipping notifications, like real AMS clients) ──
	var offerMsg struct {
		Command  string `json:"command"`
		StreamID string `json:"streamId"`
		Type     string `json:"type"`
		SDP      string `json:"sdp"`
	}
	for {
		if err := wsjson.Read(ctx, conn, &offerMsg); err != nil {
			t.Fatalf("read server offer: %v", err)
		}
		if offerMsg.Command != "notification" {
			break
		}
	}
	if offerMsg.Command != "takeConfiguration" || offerMsg.Type != "offer" {
		t.Fatalf("expected takeConfiguration/offer, got command=%q type=%q",
			offerMsg.Command, offerMsg.Type)
	}

	// ── SetRemoteDescription (server offer) ───────────────────────────────────
	if err := clientPC.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offerMsg.SDP,
	}); err != nil {
		t.Fatalf("client SetRemoteDescription: %v", err)
	}

	// ── CreateAnswer ──────────────────────────────────────────────────────────
	answer, err := clientPC.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("client CreateAnswer: %v", err)
	}

	// Register OnICECandidate BEFORE SetLocalDescription so no candidates are
	// missed; SetLocalDescription triggers ICE gathering.
	clientPC.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return // gathering complete
		}
		ci := c.ToJSON()
		msg := map[string]interface{}{
			"command":   "takeCandidate",
			"streamId":  "ice-loopback",
			"label":     c.SDPMLineIndex,
			"id":        c.SDPMid,
			"candidate": ci.Candidate,
		}
		if err := wsjson.Write(ctx, conn, msg); err != nil {
			t.Logf("client send candidate (may be normal on teardown): %v", err)
		}
	})

	if err := clientPC.SetLocalDescription(answer); err != nil {
		t.Fatalf("client SetLocalDescription: %v", err)
	}

	// ── Send answer to server ─────────────────────────────────────────────────
	answerMsg := map[string]interface{}{
		"command":  "takeConfiguration",
		"streamId": "ice-loopback",
		"type":     "answer",
		"sdp":      answer.SDP,
	}
	if err := wsjson.Write(ctx, conn, answerMsg); err != nil {
		t.Fatalf("write answer: %v", err)
	}

	// ── WS read loop: deliver server takeCandidate messages ───────────────────
	msgCh := make(chan map[string]json.RawMessage, 64)
	go func() {
		for {
			var m map[string]json.RawMessage
			if err := wsjson.Read(ctx, conn, &m); err != nil {
				// WS closed or timeout — normal on teardown.
				return
			}
			msgCh <- m
		}
	}()

	// ── Wait for ICE connected (15 s) ─────────────────────────────────────────
	deadline := time.After(15 * time.Second)
	for {
		select {
		case <-connected:
			t.Log("client ICE reached connected")
			return

		case <-deadline:
			t.Fatal("ICE did not reach connected state within 15 s")

		case msg, ok := <-msgCh:
			if !ok {
				t.Fatal("WS channel closed before ICE connected")
			}
			var cmd string
			if raw, ok := msg["command"]; ok {
				_ = json.Unmarshal(raw, &cmd)
			}
			if cmd != "takeCandidate" {
				continue
			}

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
			if err := clientPC.AddICECandidate(webrtc.ICECandidateInit{
				Candidate:     candidateStr,
				SDPMLineIndex: &idx,
				SDPMid:        &midCopy,
			}); err != nil {
				t.Logf("client AddICECandidate: %v", err)
			}
		}
	}
}

// TestWSSignaling_RTPSend is the TDD RTP-send test (WO-B phase-2b).
// An in-process pion CLIENT acting as the answerer dials the enabled handler,
// registers OnTrack BEFORE signaling, and asserts that >= 40 RTP packets are
// received within 10 s of ICE-connected.
//
// Domination arithmetic (D-074 budget-inversion rule):
//
//	send window = 2 s (~66 pkts at 30 ms/pkt)
//	assert window = 10 s (5 × send window — dominates)
//	ICE deadline = 15 s (inner)
//	outer ctx = 30 s (dominates ICE + assert windows combined)
func TestWSSignaling_RTPSend(t *testing.T) {
	ts, _ := newTestServerICE(t)
	defer ts.Close()

	// 30 s outer budget — dominates ICE setup (15 s) + RTP assert (10 s).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/live/websocket"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial %s: %v", wsURL, err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")

	// ── Create pion answerer (client PC) ──────────────────────────────────────
	m := &webrtc.MediaEngine{}
	if err := m.RegisterDefaultCodecs(); err != nil {
		t.Fatalf("RegisterDefaultCodecs: %v", err)
	}
	api := webrtc.NewAPI(webrtc.WithMediaEngine(m))
	clientPC, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("client NewPeerConnection: %v", err)
	}
	defer clientPC.Close()

	// Atomic RTP packet counter — incremented by the OnTrack drain goroutine.
	var pktCount atomic.Int64

	// Register OnTrack BEFORE signaling so no early packets are missed.
	// The drain goroutine returns when the track is closed (after ctx cancel).
	clientPC.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		t.Logf("client OnTrack: kind=%s codec=%s", track.Kind(), track.Codec().MimeType)
		for {
			if _, _, err := track.ReadRTP(); err != nil {
				return // track closed; io.EOF expected on teardown
			}
			pktCount.Add(1)
		}
	})

	// Channel signalling ICE-connected.
	connected := make(chan struct{}, 1)
	clientPC.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		t.Logf("client ICE state: %s", state)
		if state == webrtc.ICEConnectionStateConnected ||
			state == webrtc.ICEConnectionStateCompleted {
			select {
			case connected <- struct{}{}:
			default:
			}
		}
	})

	// ── Send play command ─────────────────────────────────────────────────────
	play := map[string]interface{}{
		"command":  "play",
		"streamId": "rtp-send-test",
		"token":    "",
	}
	if err := wsjson.Write(ctx, conn, play); err != nil {
		t.Fatalf("write play: %v", err)
	}

	// ── Receive server offer (skipping notifications, like real AMS clients) ──
	var offerMsg struct {
		Command  string `json:"command"`
		StreamID string `json:"streamId"`
		Type     string `json:"type"`
		SDP      string `json:"sdp"`
	}
	for {
		if err := wsjson.Read(ctx, conn, &offerMsg); err != nil {
			t.Fatalf("read server offer: %v", err)
		}
		if offerMsg.Command != "notification" {
			break
		}
	}
	if offerMsg.Command != "takeConfiguration" || offerMsg.Type != "offer" {
		t.Fatalf("expected takeConfiguration/offer, got command=%q type=%q",
			offerMsg.Command, offerMsg.Type)
	}

	// ── SetRemoteDescription (server offer) ───────────────────────────────────
	if err := clientPC.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offerMsg.SDP,
	}); err != nil {
		t.Fatalf("client SetRemoteDescription: %v", err)
	}

	// ── CreateAnswer ──────────────────────────────────────────────────────────
	answer, err := clientPC.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("client CreateAnswer: %v", err)
	}

	// Register OnICECandidate BEFORE SetLocalDescription so no candidates are
	// missed; SetLocalDescription triggers ICE gathering.
	clientPC.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return // gathering complete
		}
		ci := c.ToJSON()
		msg := map[string]interface{}{
			"command":   "takeCandidate",
			"streamId":  "rtp-send-test",
			"label":     c.SDPMLineIndex,
			"id":        c.SDPMid,
			"candidate": ci.Candidate,
		}
		if err := wsjson.Write(ctx, conn, msg); err != nil {
			t.Logf("client send candidate (may be normal on teardown): %v", err)
		}
	})

	if err := clientPC.SetLocalDescription(answer); err != nil {
		t.Fatalf("client SetLocalDescription: %v", err)
	}

	// ── Send answer to server ─────────────────────────────────────────────────
	answerMsg := map[string]interface{}{
		"command":  "takeConfiguration",
		"streamId": "rtp-send-test",
		"type":     "answer",
		"sdp":      answer.SDP,
	}
	if err := wsjson.Write(ctx, conn, answerMsg); err != nil {
		t.Fatalf("write answer: %v", err)
	}

	// ── WS read loop: deliver server takeCandidate messages ───────────────────
	msgCh := make(chan map[string]json.RawMessage, 64)
	go func() {
		for {
			var m map[string]json.RawMessage
			if err := wsjson.Read(ctx, conn, &m); err != nil {
				// WS closed or timeout — normal on teardown.
				return
			}
			msgCh <- m
		}
	}()

	// ── Wait for ICE connected (15 s inner deadline) ──────────────────────────
	iceDeadline := time.After(15 * time.Second)
iceLoop:
	for {
		select {
		case <-connected:
			t.Log("client ICE reached connected")
			break iceLoop

		case <-iceDeadline:
			t.Fatal("ICE did not reach connected state within 15 s")

		case msg, ok := <-msgCh:
			if !ok {
				t.Fatal("WS channel closed before ICE connected")
			}
			var cmd string
			if raw, ok := msg["command"]; ok {
				_ = json.Unmarshal(raw, &cmd)
			}
			if cmd != "takeCandidate" {
				continue
			}
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
			if err := clientPC.AddICECandidate(webrtc.ICECandidateInit{
				Candidate:     candidateStr,
				SDPMLineIndex: &idx,
				SDPMid:        &midCopy,
			}); err != nil {
				t.Logf("client AddICECandidate: %v", err)
			}
		}
	}

	// ── Poll for >= 40 RTP packets within 10 s of ICE connect ────────────────
	// Domination: 10 s >> 2 s send window (5×); assert 40/66 pkts (>60%).
	// The OnTrack drain goroutine above accumulates into pktCount atomically.
	const wantPkts = 40
	rtpDeadline := time.After(10 * time.Second)
	pollTick := time.NewTicker(100 * time.Millisecond)
	defer pollTick.Stop()
	for {
		select {
		case <-rtpDeadline:
			got := pktCount.Load()
			t.Fatalf("want >= %d RTP packets within 10 s of ICE connect; got %d", wantPkts, got)
		case <-pollTick.C:
			if got := pktCount.Load(); got >= wantPkts {
				t.Logf("received %d RTP packets (wanted >= %d) — PASS", got, wantPkts)
				return
			}
		}
	}
}

// TestWSSignaling_DisabledModeRegression pins the phase-1 (flag=false) behavior:
// the server sends a static offer and then closes the WS connection without
// waiting for an answer or candidates. This ensures the -webrtc-ice flag does not
// alter existing behavior when off.
func TestWSSignaling_DisabledModeRegression(t *testing.T) {
	ts, _ := newTestServer(t) // WebRTCICE = false (zero value = disabled)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/live/websocket"
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("ws dial %s: %v", wsURL, err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")

	// Send play.
	play := map[string]interface{}{
		"command":  "play",
		"streamId": "ice-disabled-test",
		"token":    "",
	}
	if err := wsjson.Write(ctx, conn, play); err != nil {
		t.Fatalf("write play: %v", err)
	}

	// Must receive offer (after the AMS-mirroring notification, D-074).
	var offer struct {
		Command string `json:"command"`
		Type    string `json:"type"`
	}
	for {
		if err := wsjson.Read(ctx, conn, &offer); err != nil {
			t.Fatalf("read offer: %v", err)
		}
		if offer.Command != "notification" {
			break
		}
	}
	if offer.Command != "takeConfiguration" {
		t.Errorf("command = %q, want takeConfiguration", offer.Command)
	}
	if offer.Type != "offer" {
		t.Errorf("type = %q, want offer", offer.Type)
	}

	// Server must close the WS after sending the offer in disabled mode —
	// a subsequent read must return a non-nil error (connection closed).
	var extra map[string]interface{}
	if err := wsjson.Read(ctx, conn, &extra); err == nil {
		t.Error("expected WS to be closed by server after offer in disabled mode, got nil error")
	}
}
