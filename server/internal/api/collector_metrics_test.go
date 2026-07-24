// ROADMAP §2.45 — collector-freshness scrape metrics.
//
// The D-164 outage passed unnoticed because nothing pages when the collector goes
// blind: the alert engine evaluates metrics DERIVED from the collector, so when it
// stops there is nothing to evaluate. /metrics now exposes the poll freshness so a
// Prometheus user can alert on it directly:
//
//	time() - pulse_collector_last_success_timestamp > 180
//
// These tests pin the two gauges against the same freshness reference /healthz uses.
package api_test

import (
	"fmt"
	"io"
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

// scrapeCollectorMetrics builds a Business-tier server (so the /metrics Prometheus
// gate passes) with the given freshness source (nil = unwired) and returns the raw
// /metrics body.
func scrapeCollectorMetrics(t *testing.T, ch domain.CollectorHealth) string {
	t.Helper()

	licKey, licCleanup := makeTestBusinessLicense(t)
	defer licCleanup()
	lic, err := license.New(licKey, "")
	if err != nil {
		t.Fatalf("license.New (business): %v", err)
	}

	live := &fakeLiveProvider{}
	qsvc := query.New(live, nil, lic)
	srv := api.New(api.Config{ListenAddr: ":0"}, nil, live, qsvc, lic, nil)
	if ch != nil {
		srv.SetCollectorHealth(ch)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET /metrics: want 200, got %d: %s", resp.StatusCode, b)
	}
	body, _ := io.ReadAll(resp.Body)
	return string(body)
}

// metricValue extracts the value of a bare (unlabeled) metric sample line.
func metricValue(t *testing.T, body, name string) (string, bool) {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		if fields := strings.Fields(line); len(fields) == 2 && fields[0] == name {
			return fields[1], true
		}
	}
	return "", false
}

func TestCollectorMetrics_FreshPoll_UpAndTimestamp(t *testing.T) {
	last := time.Now().Add(-5 * time.Second)
	body := scrapeCollectorMetrics(t, stubCollectorHealth{domain.PollHealthSnapshot{
		StartedAt:   time.Now().Add(-time.Hour),
		LastSuccess: last,
		StaleAfter:  30 * time.Second,
	}})

	if v, ok := metricValue(t, body, "pulse_collector_up"); !ok || v != "1" {
		t.Errorf("pulse_collector_up = %q (ok=%v), want 1", v, ok)
	}
	want := fmt.Sprintf("%d", last.Unix())
	if v, ok := metricValue(t, body, "pulse_collector_last_success_timestamp"); !ok || v != want {
		t.Errorf("pulse_collector_last_success_timestamp = %q (ok=%v), want %s", v, ok, want)
	}
}

func TestCollectorMetrics_StalePoll_Down(t *testing.T) {
	last := time.Now().Add(-10 * time.Minute) // well past StaleAfter
	body := scrapeCollectorMetrics(t, stubCollectorHealth{domain.PollHealthSnapshot{
		StartedAt:   time.Now().Add(-time.Hour),
		LastSuccess: last,
		StaleAfter:  30 * time.Second,
	}})

	if v, ok := metricValue(t, body, "pulse_collector_up"); !ok || v != "0" {
		t.Errorf("pulse_collector_up = %q (ok=%v), want 0 for a stale poll", v, ok)
	}
	// The timestamp still reports the last (old) success so time()-ts shows the age.
	want := fmt.Sprintf("%d", last.Unix())
	if v, _ := metricValue(t, body, "pulse_collector_last_success_timestamp"); v != want {
		t.Errorf("pulse_collector_last_success_timestamp = %q, want %s", v, want)
	}
}

func TestCollectorMetrics_NeverSucceededSinceBoot_ZeroAndDown(t *testing.T) {
	// LastSuccess zero + a StartedAt older than StaleAfter = the D-164 signature
	// (booted against an unreachable AMS, never a single successful poll).
	body := scrapeCollectorMetrics(t, stubCollectorHealth{domain.PollHealthSnapshot{
		StartedAt:   time.Now().Add(-2 * time.Minute),
		LastSuccess: time.Time{},
		StaleAfter:  30 * time.Second,
	}})

	if v, ok := metricValue(t, body, "pulse_collector_last_success_timestamp"); !ok || v != "0" {
		t.Errorf("pulse_collector_last_success_timestamp = %q (ok=%v), want 0 (never succeeded)", v, ok)
	}
	if v, ok := metricValue(t, body, "pulse_collector_up"); !ok || v != "0" {
		t.Errorf("pulse_collector_up = %q (ok=%v), want 0 (never succeeded, past grace)", v, ok)
	}
}

func TestCollectorMetrics_NotWired_MetricsAbsent(t *testing.T) {
	// No collector-health source (pure-beacon deployment / tests): the gauges must
	// be omitted entirely rather than reporting a misleading up=1/ts=0.
	body := scrapeCollectorMetrics(t, nil)

	if _, ok := metricValue(t, body, "pulse_collector_up"); ok {
		t.Error("pulse_collector_up must be absent when no collector-health source is wired")
	}
	if _, ok := metricValue(t, body, "pulse_collector_last_success_timestamp"); ok {
		t.Error("pulse_collector_last_success_timestamp must be absent when unwired")
	}
}
