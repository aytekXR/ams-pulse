// wave3_phase2b_internal_test.go — Phase-2b WebRTC stats API mapping (D-075).
//
// Tests in this file use package api (not api_test) to access the unexported
// probeResultToAPI helper directly with table-driven cases.
//
// RED pass: rtt_ms/jitter_ms/loss_pct are NOT emitted before the implementation
// below is in place — all "wantPresent=true" cases will fail.
// GREEN pass: all assertions pass after the three pointer-gated emits are added
// to probeResultToAPI in wave3.go.
package api

import (
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// f32ptr returns a pointer to a float32 literal — test helper for pointer fields.
func f32ptr(v float32) *float32 { return &v }

// TestProbeResultToAPI_Phase2bStats pins the rtt_ms/jitter_ms/loss_pct omission
// semantics added in D-075 (WO-B phase-2b):
//
//   - (a) WebRTC result with all three pointers set  → keys present, exact values.
//   - (b) Pointer to 0.0                             → key PRESENT with value 0.0
//     (pins nil-vs-zero: a nil pointer omits the key, a pointer to 0 does not).
//   - (c) failed-ICE result with nil pointers        → all three keys ABSENT.
//   - (d) Non-WebRTC (HLS) result                    → all three keys ABSENT.
func TestProbeResultToAPI_Phase2bStats(t *testing.T) {
	now := time.Now().UTC()

	cases := []struct {
		name          string
		result        domain.ProbeResult
		wantRtt       bool
		wantRttVal    float32
		wantJitter    bool
		wantJitterVal float32
		wantLoss      bool
		wantLossVal   float32
	}{
		{
			// (a) All three pointers non-nil and non-zero: keys must be present.
			name: "webrtc_connected_all_set",
			result: domain.ProbeResult{
				ID:             "r-phase2b-01",
				ProbeID:        "p-phase2b-01",
				TS:             now,
				Success:        true,
				IceState:       "connected",
				SignalingState: "offer_received",
				RttMs:          f32ptr(12.5),
				JitterMs:       f32ptr(3.2),
				LossPct:        f32ptr(0.5),
			},
			wantRtt: true, wantRttVal: 12.5,
			wantJitter: true, wantJitterVal: 3.2,
			wantLoss: true, wantLossVal: 0.5,
		},
		{
			// (b) Pointers to 0.0: keys must still be PRESENT (nil ≠ pointer-to-zero).
			name: "webrtc_connected_all_zero_pointers",
			result: domain.ProbeResult{
				ID:       "r-phase2b-02",
				ProbeID:  "p-phase2b-02",
				TS:       now,
				Success:  true,
				IceState: "connected",
				RttMs:    f32ptr(0.0),
				JitterMs: f32ptr(0.0),
				LossPct:  f32ptr(0.0),
			},
			wantRtt: true, wantRttVal: 0.0,
			wantJitter: true, wantJitterVal: 0.0,
			wantLoss: true, wantLossVal: 0.0,
		},
		{
			// (c) Failed ICE result, nil pointers: all three keys must be ABSENT.
			name: "webrtc_ice_failed_nil_pointers",
			result: domain.ProbeResult{
				ID:        "r-phase2b-03",
				ProbeID:   "p-phase2b-03",
				TS:        now,
				Success:   false,
				IceState:  "failed",
				ErrorCode: "ice_failed",
				// RttMs, JitterMs, LossPct intentionally nil.
			},
			wantRtt:    false,
			wantJitter: false,
			wantLoss:   false,
		},
		{
			// (d) Non-WebRTC (HLS) result, nil pointers: all three keys must be ABSENT.
			name: "non_webrtc_hls_nil_pointers",
			result: domain.ProbeResult{
				ID:      "r-phase2b-04",
				ProbeID: "p-phase2b-04",
				TS:      now,
				Success: true,
				TTFBMs:  80,
				// IceState: "" — non-WebRTC probe.
				// RttMs, JitterMs, LossPct intentionally nil.
			},
			wantRtt:    false,
			wantJitter: false,
			wantLoss:   false,
		},
	}

	for _, tc := range cases {
		tc := tc // capture loop variable
		t.Run(tc.name, func(t *testing.T) {
			m := probeResultToAPI(tc.result)

			// ── rtt_ms ──────────────────────────────────────────────────────────
			rttRaw, rttPresent := m["rtt_ms"]
			if rttPresent != tc.wantRtt {
				t.Errorf("rtt_ms key present=%v, want %v", rttPresent, tc.wantRtt)
			}
			if tc.wantRtt && rttPresent {
				got, ok := rttRaw.(float32)
				if !ok {
					t.Errorf("rtt_ms: value type %T, want float32", rttRaw)
				} else if got != tc.wantRttVal {
					t.Errorf("rtt_ms=%v, want %v", got, tc.wantRttVal)
				}
			}

			// ── jitter_ms ───────────────────────────────────────────────────────
			jitterRaw, jitterPresent := m["jitter_ms"]
			if jitterPresent != tc.wantJitter {
				t.Errorf("jitter_ms key present=%v, want %v", jitterPresent, tc.wantJitter)
			}
			if tc.wantJitter && jitterPresent {
				got, ok := jitterRaw.(float32)
				if !ok {
					t.Errorf("jitter_ms: value type %T, want float32", jitterRaw)
				} else if got != tc.wantJitterVal {
					t.Errorf("jitter_ms=%v, want %v", got, tc.wantJitterVal)
				}
			}

			// ── loss_pct ────────────────────────────────────────────────────────
			lossRaw, lossPresent := m["loss_pct"]
			if lossPresent != tc.wantLoss {
				t.Errorf("loss_pct key present=%v, want %v", lossPresent, tc.wantLoss)
			}
			if tc.wantLoss && lossPresent {
				got, ok := lossRaw.(float32)
				if !ok {
					t.Errorf("loss_pct: value type %T, want float32", lossRaw)
				} else if got != tc.wantLossVal {
					t.Errorf("loss_pct=%v, want %v", got, tc.wantLossVal)
				}
			}
		})
	}
}
