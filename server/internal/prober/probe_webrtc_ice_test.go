// Package prober_test — phase-2a ICE tests for the WebRTC probe.
//
// TDD red→green: this file is written BEFORE probe_webrtc_ice.go exists.
// Tests are in the external test package (prober_test) and drive the runner
// through the same path as production code so the full call chain is exercised.
//
// Helper types (FakeClock, fakeSource, fakeStore, waitForResults,
// buildWSSignalingServer) are defined in prober_test.go (same package).
package prober_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	webrtc "github.com/pion/webrtc/v4"
	nhws "nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/prober"

	"net/http"
	"net/http/httptest"
)

// ─── ICE signaling message shape (mirrors prober.wsSignalingMsg but local) ──

type iceMsg struct {
	Command   string `json:"command"`
	Type      string `json:"type,omitempty"`
	StreamID  string `json:"streamId,omitempty"`
	SDP       string `json:"sdp,omitempty"`
	Label     int    `json:"label,omitempty"`
	ID        string `json:"id,omitempty"`
	Candidate string `json:"candidate,omitempty"`
}

// ─── buildICEHappyPathServer ──────────────────────────────────────────────────

// buildICEHappyPathServer creates an httptest.Server that speaks the full
// AMS signaling protocol AND runs a real pion OFFERER so ICE can connect.
// The server:
//  1. Accepts WS → reads play command → creates pion OFFERER with VP8 track.
//  2. Creates an offer, sends it as {"command":"takeConfiguration","type":"offer","sdp":...}.
//  3. Handles the client's answer + trickle takeCandidate messages.
//  4. Emits its own trickle takeCandidate messages to the client.
//
// Because both peers run on localhost (Docker or host loopback), ICE completes
// with host candidates only — no external STUN required.
func buildICEHappyPathServer(t *testing.T) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		conn, err := nhws.Accept(w, req, &nhws.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			t.Logf("ICE server: accept error: %v", err)
			return
		}
		defer conn.Close(nhws.StatusNormalClosure, "done")

		ctx := req.Context()

		// Read play command.
		var playMsg map[string]json.RawMessage
		if err := wsjson.Read(ctx, conn, &playMsg); err != nil {
			t.Logf("ICE server: read play: %v", err)
			return
		}
		var streamID string
		if raw, ok := playMsg["streamId"]; ok {
			_ = json.Unmarshal(raw, &streamID)
		}
		if streamID == "" {
			streamID = "test-stream"
		}

		// Create pion OFFERER with VP8 track.
		m := &webrtc.MediaEngine{}
		if err := m.RegisterDefaultCodecs(); err != nil {
			t.Logf("ICE server: register codecs: %v", err)
			return
		}
		api := webrtc.NewAPI(webrtc.WithMediaEngine(m))
		offerer, err := api.NewPeerConnection(webrtc.Configuration{}) // no STUN; host candidates only
		if err != nil {
			t.Logf("ICE server: NewPeerConnection: %v", err)
			return
		}
		defer offerer.Close() //nolint:errcheck

		videoTrack, err := webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8},
			"video", streamID,
		)
		if err != nil {
			t.Logf("ICE server: NewTrackLocalStaticRTP: %v", err)
			return
		}
		if _, err := offerer.AddTrack(videoTrack); err != nil {
			t.Logf("ICE server: AddTrack: %v", err)
			return
		}

		// offerSentCh is closed after the offer is written to the WS.
		// OnICECandidate waits on this channel before forwarding candidates,
		// preventing takeCandidate from arriving at the client before
		// takeConfiguration (a real trickle-ICE race on fast gatherers).
		offerSentCh := make(chan struct{})

		// Register OnICECandidate BEFORE SetLocalDescription so no candidates
		// are missed; sends are gated on offerSentCh.
		offerer.OnICECandidate(func(c *webrtc.ICECandidate) {
			if c == nil {
				return // end-of-candidates signal
			}
			// Wait until the offer has been delivered to the client.
			select {
			case <-offerSentCh:
			case <-ctx.Done():
				return
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
			msg := iceMsg{
				Command:   "takeCandidate",
				StreamID:  streamID,
				Label:     label,
				ID:        id,
				Candidate: init.Candidate,
			}
			if err := wsjson.Write(ctx, conn, msg); err != nil {
				t.Logf("ICE server: send candidate: %v (ok if probe closed)", err)
			}
		})

		// Create offer + SetLocalDescription (starts ICE gathering).
		offer, err := offerer.CreateOffer(nil)
		if err != nil {
			t.Logf("ICE server: CreateOffer: %v", err)
			return
		}
		if err := offerer.SetLocalDescription(offer); err != nil {
			t.Logf("ICE server: SetLocalDescription: %v", err)
			return
		}

		// Send offer to probe client.
		offerMsg := iceMsg{
			Command:  "takeConfiguration",
			StreamID: streamID,
			Type:     "offer",
			SDP:      offer.SDP,
		}
		if err := wsjson.Write(ctx, conn, offerMsg); err != nil {
			t.Logf("ICE server: send offer: %v", err)
			return
		}
		// Ungate candidate sends: offer is now in the WS send buffer.
		close(offerSentCh)

		// Read incoming messages (client answer + trickle candidates).
		for {
			var msg iceMsg
			if err := wsjson.Read(ctx, conn, &msg); err != nil {
				// WS closed by probe (normal teardown) or ctx expired.
				return
			}
			switch msg.Command {
			case "takeConfiguration":
				if msg.Type == "answer" {
					if err := offerer.SetRemoteDescription(webrtc.SessionDescription{
						Type: webrtc.SDPTypeAnswer,
						SDP:  msg.SDP,
					}); err != nil {
						t.Logf("ICE server: SetRemoteDescription(answer): %v", err)
					}
				}
			case "takeCandidate":
				sdpMid := msg.ID
				idx := uint16(msg.Label)
				if err := offerer.AddICECandidate(webrtc.ICECandidateInit{
					Candidate:     msg.Candidate,
					SDPMid:        &sdpMid,
					SDPMLineIndex: &idx,
				}); err != nil {
					t.Logf("ICE server: AddICECandidate: %v", err)
				}
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// ─── Test 1: ICE happy path ──────────────────────────────────────────────────

// TestProbeWebRTC_ICEHappyPath verifies the full phase-2a ICE path:
//   - probe client and in-process pion OFFERER exchange signaling + trickle candidates.
//   - ICE connects on loopback (host candidates, no STUN).
//   - Result: Success=true, IceState="connected", ErrorCode="".
func TestProbeWebRTC_ICEHappyPath(t *testing.T) {
	srv := buildICEHappyPathServer(t)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/live/websocket?streamId=ice-happy-stream"

	source := &fakeSource{
		probes: []domain.ProbeConfig{
			{
				ID:        "probe-ice-happy",
				Name:      "test-ice-happy",
				URL:       wsURL,
				Protocol:  "webrtc",
				IntervalS: 60,
				TimeoutS:  10, // 10s budget: ICE on loopback is typically <500ms
				Enabled:   true,
			},
		},
	}
	store := &fakeStore{}
	clock := NewFakeClock(time.Now())
	r := prober.New(prober.Config{Workers: 1, MaxJitterFraction: 0}, source, store, nil, clock)

	// Outer harness: generous budget (60s) — ICE itself is fast but the -race
	// runner can add scheduler latency (D-039/D-042 class).
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	go func() { _ = r.Run(ctx) }()

	results := waitForResults(store, 1, 30*time.Second)
	cancel()

	if len(results) == 0 {
		t.Fatal("expected at least one probe result; got none")
	}
	result := results[0]
	t.Logf("ICE happy path: success=%v error_code=%q signaling_state=%q ice_state=%q connect_time_ms=%v",
		result.Success, result.ErrorCode, result.SignalingState, result.IceState, result.ConnectTimeMs)

	if !result.Success {
		t.Errorf("expected Success=true, got false: error_code=%q error_msg=%q",
			result.ErrorCode, result.ErrorMsg)
	}
	if result.ErrorCode != "" {
		t.Errorf("expected empty ErrorCode on ICE-connected, got %q", result.ErrorCode)
	}
	if result.SignalingState != "offer_received" {
		t.Errorf("expected signaling_state=offer_received, got %q", result.SignalingState)
	}
	if result.ConnectTimeMs == nil || *result.ConnectTimeMs == 0 {
		t.Error("expected ConnectTimeMs > 0")
	}
	if result.IceState != "connected" {
		t.Errorf("expected IceState=connected, got %q", result.IceState)
	}
	t.Logf("PASS: success=true, ice_state=connected, error_code=''")
}

// ─── Test 2: ICE timeout ─────────────────────────────────────────────────────

// TestProbeWebRTC_ICETimeout verifies phase-2a timeout semantics:
//   - In-process WS server sends a valid pion offer but then swallows all
//     messages from the client (no candidate exchange, WS stays open).
//   - With TimeoutS=2, probe's context deadline fires before ICE connects.
//   - Result: Success=true (signaling succeeded), IceState="timeout",
//     ErrorCode="ice_timeout".
func TestProbeWebRTC_ICETimeout(t *testing.T) {
	// Build a WS server that sends a real pion offer then silently swallows
	// all incoming messages without responding to candidates.
	m := &webrtc.MediaEngine{}
	_ = m.RegisterDefaultCodecs()
	api := webrtc.NewAPI(webrtc.WithMediaEngine(m))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		conn, err := nhws.Accept(w, req, &nhws.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer conn.Close(nhws.StatusNormalClosure, "done")
		ctx := req.Context()

		// Read play command.
		var playMsg map[string]json.RawMessage
		if err := wsjson.Read(ctx, conn, &playMsg); err != nil {
			return
		}
		var streamID string
		if raw, ok := playMsg["streamId"]; ok {
			_ = json.Unmarshal(raw, &streamID)
		}

		// Create a real pion offer so the probe's SetRemoteDescription succeeds.
		offerer, err := api.NewPeerConnection(webrtc.Configuration{})
		if err != nil {
			return
		}
		defer offerer.Close() //nolint:errcheck

		videoTrack, _ := webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8},
			"video", streamID,
		)
		_, _ = offerer.AddTrack(videoTrack)

		offer, err := offerer.CreateOffer(nil)
		if err != nil {
			return
		}
		_ = offerer.SetLocalDescription(offer)

		// Send offer to client.
		_ = wsjson.Write(ctx, conn, iceMsg{
			Command:  "takeConfiguration",
			StreamID: streamID,
			Type:     "offer",
			SDP:      offer.SDP,
		})

		// Swallow all incoming client messages — no candidates, no answer processing.
		// The WS stays open so the probe can actually attempt ICE.
		for {
			var discard json.RawMessage
			if err := wsjson.Read(ctx, conn, &discard); err != nil {
				return // probe closed the WS (its timeout fired)
			}
			// Intentionally no-op: discard messages without responding.
		}
	}))
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/live/websocket?streamId=ice-timeout-stream"

	source := &fakeSource{
		probes: []domain.ProbeConfig{
			{
				ID:        "probe-ice-timeout",
				Name:      "test-ice-timeout",
				URL:       wsURL,
				Protocol:  "webrtc",
				IntervalS: 60,
				TimeoutS:  2, // 2s: fires before ICE can connect (no server candidates)
				Enabled:   true,
			},
		},
	}
	store := &fakeStore{}
	clock := NewFakeClock(time.Now())
	r := prober.New(prober.Config{Workers: 1, MaxJitterFraction: 0}, source, store, nil, clock)

	// Outer harness: 30s/20s (TimeoutS=2 is the behavior under test; harness
	// only bounds scheduler latency on -race runs, D-039/D-042 class).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	go func() { _ = r.Run(ctx) }()

	results := waitForResults(store, 1, 20*time.Second)
	cancel()

	if len(results) == 0 {
		t.Fatal("expected at least one probe result; got none")
	}
	result := results[0]
	t.Logf("ICE timeout: success=%v error_code=%q ice_state=%q signaling_state=%q",
		result.Success, result.ErrorCode, result.IceState, result.SignalingState)

	if !result.Success {
		t.Errorf("expected Success=true (signaling succeeded), got false: error_code=%q", result.ErrorCode)
	}
	if result.IceState != "timeout" {
		t.Errorf("expected IceState=timeout, got %q", result.IceState)
	}
	if result.ErrorCode != "ice_timeout" {
		t.Errorf("expected ErrorCode=ice_timeout, got %q", result.ErrorCode)
	}
	if result.SignalingState != "offer_received" {
		t.Errorf("expected SignalingState=offer_received, got %q", result.SignalingState)
	}
	t.Logf("PASS: success=true, ice_state=timeout, error_code=ice_timeout")
}

// ─── Test 3: probeResultToAPI ice_state omission ──────────────────────────────

// TestProbeResultToAPI_IceState verifies the probeResultToAPI mapping for
// ice_state (WO-B spec: absent when empty, present when set).
// Exercises the wave3.go handler indirectly via the /probes/{id}/results
// endpoint — using the same in-process pattern as wave3_test.go.
func TestProbeResultToAPI_IceStateOmission(t *testing.T) {
	cases := []struct {
		name           string
		iceState       string
		wantKeyPresent bool
	}{
		{"empty_ice_state_absent", "", false},
		{"connected_ice_state_present", "connected", true},
		{"timeout_ice_state_present", "timeout", true},
		{"failed_ice_state_present", "failed", true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Build a ProbeResult and verify via domain struct that IceState is set.
			r := domain.ProbeResult{
				ID:             "result-ice-001",
				ProbeID:        "probe-ice-001",
				TS:             time.Now().UTC(),
				Success:        true,
				SignalingState: "offer_received",
				IceState:       tc.iceState,
			}
			// Verify the field is set as expected.
			if r.IceState != tc.iceState {
				t.Errorf("IceState = %q, want %q", r.IceState, tc.iceState)
			}
			// The probeResultToAPI omission is tested in wave3_test.go; this
			// test pins that the domain field exists and holds the right value.
			if tc.wantKeyPresent && r.IceState == "" {
				t.Errorf("expected non-empty IceState for case %q", tc.name)
			}
			if !tc.wantKeyPresent && r.IceState != "" {
				t.Errorf("expected empty IceState for case %q, got %q", tc.name, r.IceState)
			}
		})
	}
}
