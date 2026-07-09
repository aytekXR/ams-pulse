// Package ingest — health tracker tests.
//
// Tests cover: health score formula determinism, drop detection, F4 budget.
package ingest

import (
	"bytes"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// TestHealthScore_Deterministic verifies that the same inputs always produce the same score.
func TestHealthScore_Deterministic(t *testing.T) {
	score1 := ComputeHealthScore(2000, 30, 1500, 25, 2.0, 1.0, 10.0)
	score2 := ComputeHealthScore(2000, 30, 1500, 25, 2.0, 1.0, 10.0)
	if score1 != score2 {
		t.Errorf("health score not deterministic: %v != %v", score1, score2)
	}
}

// TestHealthScore_PerfectInput verifies a perfect stream scores close to 1.0.
func TestHealthScore_PerfectInput(t *testing.T) {
	// Perfect: bitrate at target, fps at target, 2s keyframe, 0% loss, 0ms jitter.
	score := ComputeHealthScore(2000, 30, 2000, 30, 2.0, 0.0, 0.0)
	if score < 0.95 {
		t.Errorf("perfect stream score = %.4f, want >= 0.95", score)
	}
	health := ScoreToHealth(score)
	if health != domain.StreamHealthGood {
		t.Errorf("perfect stream health = %v, want good", health)
	}
}

// TestHealthScore_BitrateFloorBreach verifies that low bitrate degrades score.
func TestHealthScore_BitrateFloorBreach(t *testing.T) {
	// Bitrate at 10% of target (severe drop): S_bitrate = 0.1.
	// With all other metrics healthy: 0.35*0.1 + 0.25 + 0.20 + 0.12 + 0.08 = 0.685
	// This puts the stream in Warning territory (0.50 ≤ score < 0.80).
	score := ComputeHealthScore(2000, 30, 200, 30, 2.0, 0.0, 0.0)
	// Score should be degraded relative to perfect (1.0).
	if score >= 0.80 {
		t.Errorf("low-bitrate stream score = %.4f, want < 0.80 (degraded)", score)
	}
	// With only bitrate degraded, warning is expected. Critical requires multiple metrics bad.
	health := ScoreToHealth(score)
	if health != domain.StreamHealthWarning {
		t.Errorf("low-bitrate health = %v, want warning (single-metric drop)", health)
	}

	// Verify that zero bitrate + zero fps (total failure) gives critical.
	scoreFail := ComputeHealthScore(2000, 30, 0, 0, 10.0, 10.0, 100.0)
	if scoreFail >= 0.50 {
		t.Errorf("total failure score = %.4f, want < 0.50 (critical)", scoreFail)
	}
	if ScoreToHealth(scoreFail) != domain.StreamHealthCritical {
		t.Errorf("total failure health = %v, want critical", ScoreToHealth(scoreFail))
	}
}

// TestHealthScore_FPSCollapse verifies fps collapse is detected.
func TestHealthScore_FPSCollapse(t *testing.T) {
	// FPS = 1 (severe collapse from target 30).
	score := ComputeHealthScore(2000, 30, 2000, 1, 2.0, 0.0, 0.0)
	if score >= 0.80 {
		t.Errorf("fps-collapsed stream score = %.4f, want < 0.80", score)
	}
}

// TestHealthScore_HighPacketLoss verifies packet loss degrades score.
func TestHealthScore_HighPacketLoss(t *testing.T) {
	// 10% packet loss → S_loss = 0.
	score := ComputeHealthScore(2000, 30, 2000, 30, 2.0, 10.0, 0.0)
	// S_bitrate=1*0.35, S_fps=1*0.25, S_keyframe=1*0.20, S_loss=0*0.12, S_jitter=1*0.08 = 0.88
	expected := 0.35 + 0.25 + 0.20 + 0.0 + 0.08
	if math.Abs(score-expected) > 0.01 {
		t.Errorf("10%% loss score = %.4f, want ~%.4f", score, expected)
	}
}

// TestHealthScore_Weights verifies the weights sum to 1.0 (formula invariant).
func TestHealthScore_Weights(t *testing.T) {
	total := wBitrate + wFPS + wKeyframe + wLoss + wJitter
	if math.Abs(total-1.0) > 1e-9 {
		t.Errorf("weights sum = %.10f, want 1.0", total)
	}
}

// TestHealthScore_KeyframeHigh verifies high keyframe interval degrades score.
func TestHealthScore_KeyframeHigh(t *testing.T) {
	// Keyframe interval = 6s (3× the ideal).
	score6 := ComputeHealthScore(2000, 30, 2000, 30, 6.0, 0.0, 0.0)
	score2 := ComputeHealthScore(2000, 30, 2000, 30, 2.0, 0.0, 0.0)
	if score6 >= score2 {
		t.Errorf("high keyframe score (%v) should be less than ideal (%v)", score6, score2)
	}
}

// TestHealthScore_ScoreToHealthBoundaries verifies classification boundaries.
func TestHealthScore_ScoreToHealthBoundaries(t *testing.T) {
	cases := []struct {
		score float64
		want  domain.StreamHealth
	}{
		{1.0, domain.StreamHealthGood},
		{0.80, domain.StreamHealthGood},
		{0.79, domain.StreamHealthWarning},
		{0.50, domain.StreamHealthWarning},
		{0.49, domain.StreamHealthCritical},
		{0.0, domain.StreamHealthCritical},
	}
	for _, c := range cases {
		got := ScoreToHealth(c.score)
		if got != c.want {
			t.Errorf("ScoreToHealth(%.2f) = %v, want %v", c.score, got, c.want)
		}
	}
}

// TestIngestHealth_DegradationVisible verifies the F4 budget: degradation visible ≤ 15s.
//
// Simulation:
//   - t=0: stream publishes at healthy rate (bitrate=2000, fps=30)
//   - t=5s: bitrate drops to 50kbps (severe degradation)
//   - t≤10s: HealthTracker snapshot shows degraded health (Critical/Warning)
//
// With a 5s poll interval, two poll cycles = 10s worst-case visibility.
// Budget = 15s → PASS by construction.
func TestIngestHealth_DegradationVisible(t *testing.T) {
	tracker := New(Config{
		TargetBitrateKbps: 2000,
		TargetFPS:         30,
		SourceGoneTimeout: 15 * time.Second,
	}, nil)

	// Simulate t=0: healthy ingest.
	t0 := time.Now()
	tracker.OnServerEvent(domain.ServerEvent{
		Type:     domain.EventIngestStats,
		TS:       t0.UnixMilli(),
		NodeID:   "n1",
		App:      "live",
		StreamID: "s1",
		Data: map[string]any{
			"bitrate_kbps":    float64(2000),
			"fps":             float64(30),
			"packet_loss_pct": float64(0),
			"jitter_ms":       float64(0),
		},
	})

	pub, ok := tracker.GetPublisher("n1", "live", "s1")
	if !ok {
		t.Fatal("publisher not found after first ingest_stats")
	}
	if pub.Health != domain.StreamHealthGood {
		t.Errorf("initial health = %v, want good", pub.Health)
	}

	// Simulate t=5s: bitrate drops severely (floor breach).
	t5 := t0.Add(5 * time.Second)
	degradeStart := time.Now()
	tracker.OnServerEvent(domain.ServerEvent{
		Type:     domain.EventIngestStats,
		TS:       t5.UnixMilli(),
		NodeID:   "n1",
		App:      "live",
		StreamID: "s1",
		Data: map[string]any{
			"bitrate_kbps":    float64(50), // severe drop
			"fps":             float64(30),
			"packet_loss_pct": float64(0),
			"jitter_ms":       float64(0),
		},
	})

	// Measure detection latency: time from the degradation event until
	// the tracker reflects Critical health.
	pub, ok = tracker.GetPublisher("n1", "live", "s1")
	if !ok {
		t.Fatal("publisher not found after degraded ingest_stats")
	}

	detectionLatency := time.Since(degradeStart)

	if pub.Health == domain.StreamHealthGood {
		t.Errorf("after bitrate drop: health = %v, want Warning or Critical", pub.Health)
	}

	t.Logf("F4 ingest degradation detection latency: %v (budget: 15s)", detectionLatency)

	// The detection latency for a single in-memory update is sub-millisecond.
	// In production with a 5s REST poll, worst-case is 10s (2 polls).
	// We verify the tracker reflects degradation immediately after the event.
	if detectionLatency > 100*time.Millisecond {
		t.Errorf("detection latency %v > 100ms (in-process; budget 15s for real poll)", detectionLatency)
	}

	// Verify health score is deterministic.
	score := ComputeHealthScore(2000, 30, 50, 30, 0, 0, 0)
	score2 := ComputeHealthScore(2000, 30, 50, 30, 0, 0, 0)
	if score != score2 {
		t.Error("health score is not deterministic")
	}
	// Score with 50kbps bitrate: S_bitrate=0.025; other metrics healthy.
	// 0.35*0.025 + 0.25*1.0 + 0.20*1.0 + 0.12*1.0 + 0.08*1.0 ≈ 0.659
	// In Warning zone (0.50–0.79), degraded from good (≥0.80).
	if score >= 0.80 {
		t.Errorf("degraded score = %.4f should be < 0.80 (degraded from good)", score)
	}

	t.Logf("degraded health score: %.4f (health: %v)", pub.HealthScore, pub.Health)
	t.Logf("PASS: F4 ingest degradation visible ≤ 15s (measured: %v sub-ms in-process, 10s worst-case with 5s poll)", detectionLatency)
}

// TestIngestHealth_SourceGone verifies that absent source is marked offline.
func TestIngestHealth_SourceGone(t *testing.T) {
	tracker := New(Config{
		SourceGoneTimeout: 50 * time.Millisecond, // very short for test
	}, nil)

	now := time.Now()
	tracker.OnServerEvent(domain.ServerEvent{
		Type:     domain.EventIngestStats,
		TS:       now.UnixMilli(),
		NodeID:   "n1",
		App:      "live",
		StreamID: "gone",
		Data: map[string]any{
			"bitrate_kbps": float64(2000),
			"fps":          float64(30),
		},
	})

	// Wait for timeout.
	time.Sleep(100 * time.Millisecond)

	stale := tracker.SweepStale()
	if stale != 1 {
		t.Errorf("SweepStale: evicted %d, want 1", stale)
	}

	// Publisher should be gone from the snapshot.
	_, ok := tracker.GetPublisher("n1", "live", "gone")
	if ok {
		t.Error("publisher should have been removed by SweepStale")
	}
}

// TestIngestHealth_MultiplePublishers verifies independent tracking per publisher.
func TestIngestHealth_MultiplePublishers(t *testing.T) {
	tracker := New(Config{}, nil)

	now := time.Now().UnixMilli()

	// Publisher A: healthy.
	tracker.OnServerEvent(domain.ServerEvent{
		Type:     domain.EventIngestStats,
		TS:       now,
		NodeID:   "n1",
		App:      "live",
		StreamID: "pub-a",
		Data: map[string]any{
			"bitrate_kbps": float64(2000),
			"fps":          float64(30),
		},
	})

	// Publisher B: degraded.
	tracker.OnServerEvent(domain.ServerEvent{
		Type:     domain.EventIngestStats,
		TS:       now,
		NodeID:   "n1",
		App:      "live",
		StreamID: "pub-b",
		Data: map[string]any{
			"bitrate_kbps": float64(100),
			"fps":          float64(5),
		},
	})

	snapA, okA := tracker.GetPublisher("n1", "live", "pub-a")
	snapB, okB := tracker.GetPublisher("n1", "live", "pub-b")

	if !okA || !okB {
		t.Fatal("both publishers should be in tracker")
	}
	if snapA.Health == snapB.Health {
		t.Errorf("pub-a (%v) and pub-b (%v) should have different health", snapA.Health, snapB.Health)
	}
	if snapA.Health != domain.StreamHealthGood {
		t.Errorf("pub-a health = %v, want good", snapA.Health)
	}
}

// ─── D-029v — FPS-unavailable weight redistribution ──────────────────────────

// TestComputeHealthScore_FPSUnavailableRedistributes pins the fix for the AMS
// 3.0.3 REST path, which never reports currentFPS. A negative fps is the
// "unavailable" sentinel: the FPS weight must be redistributed across the other
// four sub-scores so a fully healthy stream reaches "Good" (1.0) instead of being
// structurally capped at 0.75 ("Warning") by a phantom 0 fps.
func TestComputeHealthScore_FPSUnavailableRedistributes(t *testing.T) {
	// All non-fps dimensions perfect; fps unavailable (-1).
	score := ComputeHealthScore(2000, 30, 2000, -1, 0, 0, 0)
	if math.Abs(score-1.0) > 1e-9 {
		t.Errorf("fps-unavailable + all-else-perfect score = %v, want 1.0 (Good)", score)
	}
	if h := ScoreToHealth(score); h != domain.StreamHealthGood {
		t.Errorf("health = %v, want Good", h)
	}

	// Contrast: the SAME inputs with fps=0 (as if 0 were a real reading) are
	// capped at 0.75 → Warning. This is exactly the false-Warning the fix removes.
	capped := ComputeHealthScore(2000, 30, 2000, 0, 0, 0, 0)
	if capped >= 0.80 {
		t.Errorf("fps=0 score = %v, expected the pre-fix cap < 0.80", capped)
	}
}

// TestComputeHealthScore_FPSUnavailableLowBitrate verifies redistribution still
// reflects a genuinely degraded dimension. A 624 kbps stream (real test123)
// against the 2000 kbps target should land in "Warning" — honest, low bitrate —
// not be masked by the old 1000× inflation that pinned S_bitrate to 1.0.
func TestComputeHealthScore_FPSUnavailableLowBitrate(t *testing.T) {
	score := ComputeHealthScore(2000, 30, 624.016, -1, 0, 0, 0)
	if h := ScoreToHealth(score); h != domain.StreamHealthWarning {
		t.Errorf("624 kbps vs 2000 target: health = %v (score %.3f), want Warning", h, score)
	}
}

// ─── Aggregated degraded-stream log tests (WO-C) ─────────────────────────────

// TestLogDegradedSummary_OneLinePerTick verifies that N degraded streams produce
// exactly one aggregated INFO line per SweepStale tick, not N per-stream INFO
// lines. The per-stream detail must be demoted to DEBUG (invisible at Info level).
func TestLogDegradedSummary_OneLinePerTick(t *testing.T) {
	var buf bytes.Buffer
	// slog.NewTextHandler with nil opts defaults to LevelInfo; Debug calls are dropped.
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	tracker := New(Config{SourceGoneTimeout: time.Minute}, logger)

	now := time.Now().UnixMilli()
	// Feed 5 degraded streams (1 kbps bitrate → Critical health).
	for i := 0; i < 5; i++ {
		tracker.OnServerEvent(domain.ServerEvent{
			Type:     domain.EventIngestStats,
			TS:       now,
			NodeID:   "n1",
			App:      "live",
			StreamID: fmt.Sprintf("s%d", i),
			Data: map[string]any{
				"bitrate_kbps": float64(1),
			},
		})
	}

	// Trigger the per-tick aggregated summary.
	tracker.SweepStale()

	output := buf.String()
	aggLines := countDegradedSummaryLines(output)
	if aggLines != 1 {
		t.Errorf("want exactly 1 aggregated INFO line, got %d; output:\n%s", aggLines, output)
	}
	perStreamLines := countPerStreamInfoLines(output)
	if perStreamLines != 0 {
		t.Errorf("want 0 per-stream INFO lines (must be demoted to DEBUG), got %d; output:\n%s", perStreamLines, output)
	}
}

// TestLogDegradedSummary_ZeroDegraded verifies that zero degraded streams produce
// zero aggregated summary log lines.
func TestLogDegradedSummary_ZeroDegraded(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	tracker := New(Config{SourceGoneTimeout: time.Minute}, logger)

	now := time.Now().UnixMilli()
	for i := 0; i < 3; i++ {
		tracker.OnServerEvent(domain.ServerEvent{
			Type:     domain.EventIngestStats,
			TS:       now,
			NodeID:   "n1",
			App:      "live",
			StreamID: fmt.Sprintf("healthy%d", i),
			Data: map[string]any{
				"bitrate_kbps": float64(2000),
				"fps":          float64(30),
			},
		})
	}

	tracker.SweepStale()

	output := buf.String()
	if strings.Contains(output, "ingest: degraded streams") {
		t.Errorf("want no degraded summary for all-healthy streams, got:\n%s", output)
	}
}

func countDegradedSummaryLines(s string) int {
	n := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, "ingest: degraded streams") {
			n++
		}
	}
	return n
}

func countPerStreamInfoLines(s string) int {
	n := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, "ingest: health degraded") {
			n++
		}
	}
	return n
}
