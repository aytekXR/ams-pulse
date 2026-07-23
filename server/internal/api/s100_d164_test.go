// S100 / D-164 — /healthz collector freshness.
//
// Regression origin: on 2026-07-23 a prod deploy dropped the real-ams compose
// overlay, reverting PULSE_AMS_URL to an unreachable host. Every AMS poll failed
// for 7 h 46 m, and /healthz reported collector "ok" the whole time, because the
// component was a pure liveness proxy: "the aggregator holds a snapshot object".
// The aggregator holds its last snapshot forever, so that check can never go
// false once the first poll has succeeded. The deploy health-gate consequently
// passed on a collector that had gone completely blind.
//
// These tests pin the freshness semantics that replaced it.
package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/api"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/license"
	"github.com/pulse-analytics/pulse/server/internal/query"
)

// stubCollectorHealth is a domain.CollectorHealth double returning a fixed snapshot.
type stubCollectorHealth struct{ snap domain.PollHealthSnapshot }

func (s stubCollectorHealth) PollHealth() domain.PollHealthSnapshot { return s.snap }

// healthzCollector spins up a server with the given freshness source (nil =
// unwired) and returns the collector component of GET /healthz plus the overall
// status string.
func healthzCollector(t *testing.T, ch domain.CollectorHealth) (component map[string]any, overall string) {
	t.Helper()

	lic, _ := license.New("", "")
	live := &fakeLiveProvider{}
	qsvc := query.New(live, nil, lic)
	srv := api.New(api.Config{ListenAddr: ":0"}, nil, live, qsvc, lic, nil)
	if ch != nil {
		srv.SetCollectorHealth(ch)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /healthz: want 200, got %d", resp.StatusCode)
	}

	var body struct {
		Status     string                    `json:"status"`
		Components map[string]map[string]any `json:"components"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode /healthz body: %v", err)
	}
	comp, ok := body.Components["collector"]
	if !ok {
		t.Fatalf("collector component missing from /healthz: %+v", body.Components)
	}
	return comp, body.Status
}

// A recent successful poll is healthy.
func TestD164_Healthz_CollectorFresh_IsOK(t *testing.T) {
	comp, overall := healthzCollector(t, stubCollectorHealth{domain.PollHealthSnapshot{
		StartedAt:   time.Now().Add(-10 * time.Minute),
		LastSuccess: time.Now().Add(-2 * time.Second),
		StaleAfter:  30 * time.Second,
	}})

	if comp["status"] != "ok" {
		t.Errorf("collector status: want ok for a 2s-old poll, got %v (message=%v)", comp["status"], comp["message"])
	}
	if overall != "ok" {
		t.Errorf("overall status: want ok, got %v", overall)
	}
}

// THE REGRESSION: polls have been failing far longer than StaleAfter. Before
// D-164 this reported "ok" — the whole point of the fix is that it now cannot.
func TestD164_Healthz_CollectorStale_IsDegraded(t *testing.T) {
	comp, overall := healthzCollector(t, stubCollectorHealth{domain.PollHealthSnapshot{
		StartedAt:   time.Now().Add(-8 * time.Hour),
		LastSuccess: time.Now().Add(-7*time.Hour - 46*time.Minute),
		LastError:   `list applications: dial tcp: lookup mock-ams: server misbehaving`,
		StaleAfter:  30 * time.Second,
	}})

	if comp["status"] != "degraded" {
		t.Fatalf("collector status: want degraded after a 7h46m poll gap, got %v", comp["status"])
	}
	msg, _ := comp["message"].(string)
	if msg == "" {
		t.Error("degraded collector must carry a message naming the gap; got empty")
	}
	// The cause must be surfaced, not just the symptom — that is what makes the
	// signal actionable to an operator reading /healthz.
	if want := "server misbehaving"; msg != "" && !strings.Contains(msg, want) {
		t.Errorf("collector message must include the last poll error %q, got %q", want, msg)
	}
	// A degraded collector is reported at the top level but stays HTTP 200:
	// only ClickHouse/meta-store failures are "down" (503). This keeps a
	// transient AMS outage from tripping container/orchestrator liveness probes
	// into a restart loop.
	if overall != "degraded" {
		t.Errorf("overall status: want degraded, got %v", overall)
	}
}

// A collector that has NEVER reached AMS since boot must not read as healthy
// once it is past the threshold — LastSuccess is zero, so StartedAt is the
// age reference.
func TestD164_Healthz_CollectorNeverSucceeded_IsDegraded(t *testing.T) {
	comp, _ := healthzCollector(t, stubCollectorHealth{domain.PollHealthSnapshot{
		StartedAt:  time.Now().Add(-5 * time.Minute),
		LastError:  "connection refused",
		StaleAfter: 30 * time.Second,
	}})

	if comp["status"] != "degraded" {
		t.Errorf("collector status: want degraded when no poll has ever succeeded, got %v", comp["status"])
	}
}

// Cold start must not flap: a freshly started collector inside the threshold is
// still healthy even though no poll has completed yet.
func TestD164_Healthz_CollectorColdStart_IsOK(t *testing.T) {
	comp, _ := healthzCollector(t, stubCollectorHealth{domain.PollHealthSnapshot{
		StartedAt:  time.Now().Add(-2 * time.Second),
		StaleAfter: 30 * time.Second,
	}})

	if comp["status"] != "ok" {
		t.Errorf("collector status: want ok during cold start inside the threshold, got %v (message=%v)",
			comp["status"], comp["message"])
	}
}

// Backward compatibility: with no freshness source wired, the component keeps
// the pre-D-164 liveness-only semantics rather than failing closed.
func TestD164_Healthz_NoCollectorHealthWired_KeepsLivenessSemantics(t *testing.T) {
	comp, overall := healthzCollector(t, nil)

	if comp["status"] != "ok" {
		t.Errorf("collector status: want ok when no freshness source is wired, got %v", comp["status"])
	}
	if overall != "ok" {
		t.Errorf("overall status: want ok, got %v", overall)
	}
}

// StaleAfter=0 explicitly disables the freshness check.
func TestD164_Healthz_StaleAfterZero_DisablesFreshnessCheck(t *testing.T) {
	comp, _ := healthzCollector(t, stubCollectorHealth{domain.PollHealthSnapshot{
		StartedAt:   time.Now().Add(-8 * time.Hour),
		LastSuccess: time.Now().Add(-8 * time.Hour),
		StaleAfter:  0,
	}})

	if comp["status"] != "ok" {
		t.Errorf("collector status: want ok when StaleAfter is 0 (check disabled), got %v", comp["status"])
	}
}
