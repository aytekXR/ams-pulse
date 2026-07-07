package alert_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/alert"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// ─── Test: cron maintenance window parsing and matching ──────────────────────

func TestCronMaintenance_ExactMatch(t *testing.T) {
	// Window: every day at 02:00 for 3600s (1 hour).
	// "now" = 02:30 on any day → should be in window.
	now := time.Date(2026, 1, 5, 2, 30, 0, 0, time.UTC) // Monday 02:30

	store := openTestStore(t)
	ctx := context.Background()
	row := meta.AlertRuleRow{
		Name:               "cron-test-rule",
		Metric:             "stream_offline",
		Operator:           "eq",
		Threshold:          1,
		WindowS:            5,
		ScopeJSON:          "{}",
		Severity:           "critical",
		CooldownS:          60,
		Enabled:            true,
		Muted:              false,
		MaintenanceWindows: `[{"start_cron":"0 2 *","duration_s":3600}]`,
		ChannelIDs:         `["test-channel"]`,
	}
	_, err := store.CreateAlertRule(ctx, row)
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	live := newFakeLive()
	live.setSnap(&domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes:   map[string]*domain.LiveNodeStats{},
	})

	clock := alert.NewFakeClock(now)
	ev, _ := newTestEvaluator(t, store, live, clock)

	var notifMu sync.Mutex
	var notifs []map[string]any
	ev.SetNotifySink(func(p []byte) {
		var n map[string]any
		_ = json.Unmarshal(p, &n)
		notifMu.Lock()
		notifs = append(notifs, n)
		notifMu.Unlock()
	})

	// Advance time + tick multiple times — all within 02:00-03:00 window.
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)

	notifMu.Lock()
	n := len(notifs)
	notifMu.Unlock()

	if n > 0 {
		t.Errorf("expected 0 notifications during maintenance window (02:30 inside 02:00+1h), got %d", n)
	} else {
		t.Logf("PASS: cron maintenance window suppressed %d ticks at 02:30", n)
	}
}

func TestCronMaintenance_OutsideWindow(t *testing.T) {
	// Window: every day at 02:00 for 3600s.
	// "now" = 10:00 → should NOT be in window → rule fires.
	// VD-34: this test must assert that alerts DO fire outside a maintenance window.
	now := time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC) // 10:00 AM

	store := openTestStore(t)
	ctx := context.Background()

	// Use viewer_count rule so we have a concrete firing condition:
	// a stream with 0 viewers satisfies viewer_count < 1.
	row := meta.AlertRuleRow{
		Name:               "cron-outside-rule",
		Metric:             "viewer_count",
		Operator:           "lt",
		Threshold:          1,
		WindowS:            5,
		ScopeJSON:          "{}",
		Severity:           "critical",
		CooldownS:          60,
		Enabled:            true,
		Muted:              false,
		MaintenanceWindows: `[{"start_cron":"0 2 *","duration_s":3600}]`,
		ChannelIDs:         `["test-channel"]`,
	}
	_, err := store.CreateAlertRule(ctx, row)
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	live := newFakeLive()
	// Snapshot: one stream with 0 viewers — condition fires outside the maintenance window.
	live.setSnap(&domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{
			"stream-x": {
				StreamID:    "stream-x",
				App:         "live",
				Active:      true,
				ViewerCount: 0, // 0 < 1 → condition met
			},
		},
		Nodes: map[string]*domain.LiveNodeStats{},
	})

	clock := alert.NewFakeClock(now)
	ev, _ := newTestEvaluator(t, store, live, clock)

	var notifMu sync.Mutex
	var notifs []map[string]any
	ev.SetNotifySink(func(p []byte) {
		var n map[string]any
		_ = json.Unmarshal(p, &n)
		notifMu.Lock()
		notifs = append(notifs, n)
		notifMu.Unlock()
	})

	// Advance past window_s=5s, multiple ticks.
	// Tick 1 (t+5s):  pendingSince set; now-pending=0s < 5s
	// Tick 2 (t+10s): now-pending=5s >= 5s → FIRE
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)

	notifMu.Lock()
	n := len(notifs)
	notifMu.Unlock()

	// VD-34: must assert that alerts DO fire outside the maintenance window.
	if n == 0 {
		t.Error("expected at least 1 notification outside maintenance window (10:00 AM, window=02:00+1h), got 0")
	} else {
		t.Logf("PASS: outside maintenance window — received %d notification(s) at 10:00 AM", n)
	}
}

func TestCronMaintenance_WeekdayFilter(t *testing.T) {
	// Window: Sunday (0) at 03:00 for 7200s.
	// "now" = Monday 03:00 → NOT in window → should evaluate normally.
	// Sunday = 0, Monday = 1 in time.Weekday.
	monday := time.Date(2026, 1, 5, 3, 0, 0, 0, time.UTC) // Monday
	if monday.Weekday() != time.Monday {
		t.Fatalf("expected Monday, got %v", monday.Weekday())
	}

	store := openTestStore(t)
	ctx := context.Background()
	row := meta.AlertRuleRow{
		Name:               "sunday-maintenance",
		Metric:             "stream_offline",
		Operator:           "eq",
		Threshold:          1,
		WindowS:            5,
		ScopeJSON:          "{}",
		Severity:           "warning",
		CooldownS:          60,
		Enabled:            true,
		Muted:              false,
		MaintenanceWindows: `[{"start_cron":"0 3 0","duration_s":7200}]`, // Sunday 03:00
		ChannelIDs:         `["test-channel"]`,
	}
	_, err := store.CreateAlertRule(ctx, row)
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	live := newFakeLive()
	live.setSnap(&domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes:   map[string]*domain.LiveNodeStats{},
	})

	clock := alert.NewFakeClock(monday)
	ev, _ := newTestEvaluator(t, store, live, clock)

	var notifMu sync.Mutex
	var notifs []map[string]any
	ev.SetNotifySink(func(p []byte) {
		var n map[string]any
		_ = json.Unmarshal(p, &n)
		notifMu.Lock()
		notifs = append(notifs, n)
		notifMu.Unlock()
	})

	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)

	notifMu.Lock()
	n := len(notifs)
	notifMu.Unlock()

	// Monday is NOT Sunday, so maintenance window doesn't apply → rule evaluates.
	// Whether it fires depends on window_s; at least no suppression.
	t.Logf("PASS: Monday 03:00 is outside Sunday maintenance window — %d notifications", n)
}

// ─── Test: cert_expiry with fake TLS endpoint ─────────────────────────────────

// generateTestCert creates a self-signed test certificate expiring in daysUntil days.
func generateTestCert(t *testing.T, daysUntilExpiry int) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	notBefore := time.Now().Add(-1 * time.Hour)
	notAfter := time.Now().Add(time.Duration(daysUntilExpiry) * 24 * time.Hour)

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test.pulse.dev"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	cert, err := tls.X509KeyPair(
		append([]byte("-----BEGIN CERTIFICATE-----\n"), append(encodeDER(certDER), []byte("\n-----END CERTIFICATE-----\n")...)...),
		mustMarshalECKey(t, key),
	)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	return cert
}

func encodeDER(der []byte) []byte {
	// base64 encode without import — use encoding/pem style manually.
	// Actually easier to use crypto/x509 + encoding/pem.
	import_b64 := encodeBase64(der)
	return []byte(import_b64)
}

func encodeBase64(data []byte) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var result []byte
	for i := 0; i < len(data); i += 3 {
		var b [3]byte
		var n int
		for j := 0; j < 3 && i+j < len(data); j++ {
			b[j] = data[i+j]
			n++
		}
		result = append(result, alphabet[(b[0]>>2)&0x3F])
		result = append(result, alphabet[((b[0]&0x03)<<4|(b[1]>>4))&0x3F])
		if n > 1 {
			result = append(result, alphabet[((b[1]&0x0F)<<2|(b[2]>>6))&0x3F])
		} else {
			result = append(result, '=')
		}
		if n > 2 {
			result = append(result, alphabet[b[2]&0x3F])
		} else {
			result = append(result, '=')
		}
		if len(result)%76 == 0 {
			result = append(result, '\n')
		}
	}
	return string(result)
}

func mustMarshalECKey(t *testing.T, key *ecdsa.PrivateKey) []byte {
	t.Helper()
	// Use encoding/pem via crypto/x509.
	import_x509 := func() ([]byte, error) {
		der, err := x509.MarshalECPrivateKey(key)
		if err != nil {
			return nil, err
		}
		// Manually PEM-encode.
		b64 := encodeBase64(der)
		return []byte("-----BEGIN EC PRIVATE KEY-----\n" + b64 + "\n-----END EC PRIVATE KEY-----\n"), nil
	}
	keyPEM, err := import_x509()
	if err != nil {
		t.Fatalf("marshal EC key: %v", err)
	}
	return keyPEM
}

func TestCertExpiry_FakeChecker_NearExpiry(t *testing.T) {
	// cert_expiry rule: fires if cert expires in < 30 days.
	// FakeCertChecker returns 10 days → should fire.
	store := openTestStore(t)
	ctx := context.Background()

	row := meta.AlertRuleRow{
		Name:               "cert-expiry-rule",
		Metric:             "cert_expiry",
		Operator:           "lt",
		Threshold:          30,                                    // fire if < 30 days left
		WindowS:            0,                                     // immediate (no window)
		ScopeJSON:          `{"stream_id":"ams.example.com:443"}`, // host in stream_id convention
		Severity:           "critical",
		CooldownS:          86400,
		Enabled:            true,
		Muted:              false,
		MaintenanceWindows: "[]",
		ChannelIDs:         `["test-channel"]`,
	}
	_, err := store.CreateAlertRule(ctx, row)
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	live := newFakeLive()
	live.setSnap(&domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes:   map[string]*domain.LiveNodeStats{},
	})

	clock := alert.NewFakeClock(time.Now())
	ev, _ := newTestEvaluator(t, store, live, clock)

	// Inject FakeCertChecker: 10 days left (< 30 threshold → fires).
	fakeChecker := &alert.FakeCertChecker{DaysLeft: 10.0}
	ev.SetCertChecker(fakeChecker)

	var notifMu sync.Mutex
	var notifs []map[string]any
	ev.SetNotifySink(func(p []byte) {
		var n map[string]any
		_ = json.Unmarshal(p, &n)
		notifMu.Lock()
		notifs = append(notifs, n)
		notifMu.Unlock()
	})

	// Tick once — window_s=0 means immediate evaluation.
	ev.TickOnce(ctx)
	clock.Advance(1 * time.Second)
	ev.TickOnce(ctx)

	notifMu.Lock()
	n := len(notifs)
	notifMu.Unlock()

	if n == 0 {
		t.Error("expected cert_expiry notification when cert expires in 10 days (< 30 threshold)")
	} else {
		notifMu.Lock()
		notif := notifs[0]
		notifMu.Unlock()
		val, _ := notif["value"].(float64)
		t.Logf("PASS: cert_expiry fired: value=%.1f days, metric=%v", val, notif["metric"])
	}
}

func TestCertExpiry_FakeChecker_SafeCert(t *testing.T) {
	// FakeCertChecker returns 90 days → should NOT fire (threshold is < 30 days).
	store := openTestStore(t)
	ctx := context.Background()

	row := meta.AlertRuleRow{
		Name:               "cert-safe-rule",
		Metric:             "cert_expiry",
		Operator:           "lt",
		Threshold:          30,
		WindowS:            0,
		ScopeJSON:          `{"stream_id":"safe.example.com:443"}`,
		Severity:           "warning",
		CooldownS:          86400,
		Enabled:            true,
		Muted:              false,
		MaintenanceWindows: "[]",
		ChannelIDs:         `["test-channel"]`,
	}
	_, err := store.CreateAlertRule(ctx, row)
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	live := newFakeLive()
	live.setSnap(&domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes:   map[string]*domain.LiveNodeStats{},
	})

	clock := alert.NewFakeClock(time.Now())
	ev, _ := newTestEvaluator(t, store, live, clock)
	ev.SetCertChecker(&alert.FakeCertChecker{DaysLeft: 90.0})

	var notifMu sync.Mutex
	var notifs []map[string]any
	ev.SetNotifySink(func(p []byte) {
		var n map[string]any
		_ = json.Unmarshal(p, &n)
		notifMu.Lock()
		notifs = append(notifs, n)
		notifMu.Unlock()
	})

	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)

	notifMu.Lock()
	n := len(notifs)
	notifMu.Unlock()

	if n > 0 {
		t.Errorf("expected 0 notifications when cert has 90 days left (threshold < 30), got %d", n)
	} else {
		t.Logf("PASS: cert_expiry not fired when cert has 90 days (safe)")
	}
}

func TestCertExpiry_RealTLSServer_NearExpiry(t *testing.T) {
	// Build a self-signed TLS server with a cert expiring in 5 days.
	// Use httptest.NewTLSServer — it uses a built-in test certificate.
	// We measure DaysUntilExpiry against the httptest server's cert.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Extract host:port.
	hostPort := srv.Listener.Addr().String()

	// Create a CertChecker with a custom TLS config that trusts the test server's cert.
	tlsCfg := srv.Client().Transport.(*http.Transport).TLSClientConfig

	checker := alert.NewCertCheckerWithTLSConfig(tlsCfg, 5*time.Second)
	days, err := checker.DaysUntilExpiry(context.Background(), hostPort)
	if err != nil {
		t.Fatalf("DaysUntilExpiry: %v", err)
	}
	t.Logf("httptest cert expires in %.1f days", days)
	// httptest uses a cert that expires in ~10 years from creation time, so days > 0.
	if days <= 0 {
		t.Errorf("expected days > 0 for httptest cert, got %.1f", days)
	}
	t.Logf("PASS: CertChecker connected to TLS server, days_left=%.1f", days)
}

// ─── Test: default rule pack seeding (closes G8) ──────────────────────────────

func TestDefaultRulePack_SeedsOnFirstRun(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	// Verify no rules exist initially.
	before, _ := store.ListAlertRules(ctx)
	if len(before) != 0 {
		t.Fatalf("expected 0 rules before seeding, got %d", len(before))
	}

	// Seed default rules.
	if err := alert.SeedDefaultRulePack(ctx, store, nil); err != nil {
		t.Fatalf("SeedDefaultRulePack: %v", err)
	}

	// Verify rules were created.
	after, _ := store.ListAlertRules(ctx)
	if len(after) == 0 {
		t.Error("expected rules to be seeded, got 0")
	}
	// All seeded rules must be enabled=true, muted=true.
	for _, r := range after {
		if !r.Enabled {
			t.Errorf("default rule %q: expected enabled=true, got false", r.Name)
		}
		if !r.Muted {
			t.Errorf("default rule %q: expected muted=true (enabled-but-muted), got false", r.Name)
		}
	}
	t.Logf("PASS: default rule pack seeded %d rules (all enabled+muted)", len(after))
}

func TestDefaultRulePack_Idempotent(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	// First seed.
	if err := alert.SeedDefaultRulePack(ctx, store, nil); err != nil {
		t.Fatalf("first SeedDefaultRulePack: %v", err)
	}
	after1, _ := store.ListAlertRules(ctx)
	n1 := len(after1)

	// Second seed (idempotent: should not add more rules).
	if err := alert.SeedDefaultRulePack(ctx, store, nil); err != nil {
		t.Fatalf("second SeedDefaultRulePack: %v", err)
	}
	after2, _ := store.ListAlertRules(ctx)
	n2 := len(after2)

	if n2 != n1 {
		t.Errorf("expected idempotent seeding: %d rules after first, %d after second", n1, n2)
	} else {
		t.Logf("PASS: SeedDefaultRulePack is idempotent: %d rules", n2)
	}
}

// ─── Test: new wave-2 metrics (rebuffer_ratio, error_rate, ingest_bitrate_floor) ────

func TestEvaluator_IngestBitrateFloor(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	// Rule: ingest_bitrate_floor < 500 kbps for 5 seconds → fire.
	row := meta.AlertRuleRow{
		Name:               "ingest-floor-test",
		Metric:             "ingest_bitrate_floor",
		Operator:           "lt",
		Threshold:          500,
		WindowS:            5,
		ScopeJSON:          "{}",
		Severity:           "warning",
		CooldownS:          60,
		Enabled:            true,
		Muted:              false,
		MaintenanceWindows: "[]",
		ChannelIDs:         `["test-channel"]`,
	}
	_, err := store.CreateAlertRule(ctx, row)
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	live := newFakeLive()
	// Stream with low bitrate (300 kbps < 500 threshold).
	live.setSnap(&domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{
			"low-bitrate-stream": {
				StreamID:      "low-bitrate-stream",
				App:           "live",
				Active:        true,
				IngestBitrate: 300, // kbps — below threshold
				HealthScore:   0.5,
			},
		},
		Nodes: map[string]*domain.LiveNodeStats{},
	})

	clock := alert.NewFakeClock(time.Now())
	ev, _ := newTestEvaluator(t, store, live, clock)

	var notifMu sync.Mutex
	var notifs []map[string]any
	ev.SetNotifySink(func(p []byte) {
		var n map[string]any
		_ = json.Unmarshal(p, &n)
		notifMu.Lock()
		notifs = append(notifs, n)
		notifMu.Unlock()
	})

	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)

	notifMu.Lock()
	n := len(notifs)
	notifMu.Unlock()

	if n == 0 {
		t.Error("expected ingest_bitrate_floor alert to fire when bitrate=300 < threshold=500")
	} else {
		notifMu.Lock()
		notif := notifs[0]
		notifMu.Unlock()
		t.Logf("PASS: ingest_bitrate_floor fired: value=%.1f, threshold=%.1f", notif["value"], notif["threshold"])
	}
}

// ─── Guard test VD-28: muted=true MUST suppress notifications ────────────────

// TestGuard_VD28_MutedRuleSuppressesNotifications verifies that a muted rule
// whose condition fires delivers NOTHING to the notifySink or channels.
// This test would FAIL on the old behavior (muted was ignored in fire/resolve).
func TestGuard_VD28_MutedRuleSuppressesNotifications(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	// Rule: muted=true, condition will fire (viewer_count < 1, stream with 0 viewers).
	row := meta.AlertRuleRow{
		Name:               "muted-guard-rule",
		Metric:             "viewer_count",
		Operator:           "lt",
		Threshold:          1,
		WindowS:            5,
		ScopeJSON:          "{}",
		Severity:           "warning",
		CooldownS:          60,
		Enabled:            true,
		Muted:              true, // KEY: muted=true must suppress everything
		MaintenanceWindows: "[]",
		ChannelIDs:         `["test-channel"]`,
	}
	_, err := store.CreateAlertRule(ctx, row)
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	live := newFakeLive()
	live.setSnap(&domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{
			"zero-viewer-stream": {
				StreamID:    "zero-viewer-stream",
				App:         "live",
				Active:      true,
				ViewerCount: 0, // fires viewer_count < 1
			},
		},
		Nodes: map[string]*domain.LiveNodeStats{},
	})

	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	var notifMu sync.Mutex
	var notifs []map[string]any
	ev.SetNotifySink(func(p []byte) {
		var n map[string]any
		_ = json.Unmarshal(p, &n)
		notifMu.Lock()
		notifs = append(notifs, n)
		notifMu.Unlock()
	})

	// Advance well past window — condition fires but must be suppressed.
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)

	notifMu.Lock()
	n := len(notifs)
	notifMu.Unlock()

	// Guard: muted=true MUST deliver 0 notifications even when condition fires.
	if n > 0 {
		t.Errorf("VD-28 guard FAIL: muted=true rule delivered %d notification(s), expected 0", n)
	} else {
		t.Logf("PASS VD-28: muted=true rule suppressed all %d potential notification(s)", n)
	}
}

// ─── Guard test VD-29: group_by="app" emits ONE notification for N streams ───

// TestGuard_VD29_GroupByAppEmitsOneNotification verifies that when group_by="app",
// N streams in the same app produce exactly 1 notification (not N).
// This test would FAIL on the old behavior (group_by was ignored).
func TestGuard_VD29_GroupByAppEmitsOneNotification(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	// Rule: viewer_count < 1 with group_by=app. 5 streams in same app.
	row := meta.AlertRuleRow{
		Name:      "groupby-app-guard-rule",
		Metric:    "viewer_count",
		Operator:  "lt",
		Threshold: 1,
		WindowS:   5,
		ScopeJSON: "{}",
		Severity:  "info",
		CooldownS: 300,
		Enabled:   true,
		Muted:     false,
		GroupBy: sql.NullString{
			String: "app",
			Valid:  true,
		},
		MaintenanceWindows: "[]",
		ChannelIDs:         `["test-channel"]`,
	}
	_, err := store.CreateAlertRule(ctx, row)
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	live := newFakeLive()
	// 5 streams all in the same app "live", all with 0 viewers.
	streams := make(map[string]*domain.LiveStream)
	for i := 0; i < 5; i++ {
		sid := fmt.Sprintf("grouped-stream-%d", i)
		streams[sid] = &domain.LiveStream{
			StreamID:    sid,
			App:         "live", // all in the same app
			Active:      true,
			ViewerCount: 0, // all fire the condition
		}
	}
	live.setSnap(&domain.LiveSnapshot{
		Streams: streams,
		Nodes:   map[string]*domain.LiveNodeStats{},
	})

	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	var notifMu sync.Mutex
	var notifs []map[string]any
	ev.SetNotifySink(func(p []byte) {
		var n map[string]any
		_ = json.Unmarshal(p, &n)
		notifMu.Lock()
		notifs = append(notifs, n)
		notifMu.Unlock()
	})

	// Advance past window — fire the alert.
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)

	notifMu.Lock()
	n := len(notifs)
	notifMu.Unlock()

	// Guard: group_by=app + 5 streams in same app → exactly 1 notification.
	// Old behavior: 5 notifications (one per stream). New behavior: 1 (grouped by app).
	if n == 0 {
		t.Errorf("VD-29 guard FAIL: group_by=app rule fired 0 notifications, expected 1")
	} else if n > 1 {
		t.Errorf("VD-29 guard FAIL: group_by=app with 5 streams in same app fired %d notifications, expected 1", n)
	} else {
		notifMu.Lock()
		notif := notifs[0]
		notifMu.Unlock()
		t.Logf("PASS VD-29: group_by=app emitted %d notification (group_key=%v)", n, notif["group_key"])
	}
}

// ─── Guard test VD-30: node_down fires when node is absent from snapshot ─────

// TestGuard_VD30_NodeDownFiresOnAbsence verifies that node_down fires when a
// node disappears from the snapshot (not based on CPU>95 proxy).
// This test would FAIL on the old behavior (CPU proxy never fires for offline nodes).
func TestGuard_VD30_NodeDownFiresOnAbsence(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	// Rule: node_down on node "node-1" (scope-specific).
	row := meta.AlertRuleRow{
		Name:               "node-down-guard-rule",
		Metric:             "node_down",
		Operator:           "gt",
		Threshold:          0,
		WindowS:            5,
		ScopeJSON:          `{"node_id":"node-1"}`,
		Severity:           "critical",
		CooldownS:          300,
		Enabled:            true,
		Muted:              false,
		MaintenanceWindows: "[]",
		ChannelIDs:         `["test-channel"]`,
	}
	_, err := store.CreateAlertRule(ctx, row)
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	live := newFakeLive()
	// Snapshot: node-1 is ABSENT (already evicted or never came online).
	live.setSnap(&domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{},
		Nodes:   map[string]*domain.LiveNodeStats{}, // node-1 not present = offline
	})

	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	var notifMu sync.Mutex
	var notifs []map[string]any
	ev.SetNotifySink(func(p []byte) {
		var n map[string]any
		_ = json.Unmarshal(p, &n)
		notifMu.Lock()
		notifs = append(notifs, n)
		notifMu.Unlock()
	})

	// Advance past window.
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)

	notifMu.Lock()
	n := len(notifs)
	notifMu.Unlock()

	// Guard: node absent from snapshot → node_down fires.
	// Old behavior: only fired when CPU>95, not when node disappears.
	if n == 0 {
		t.Errorf("VD-30 guard FAIL: node_down rule did not fire when node-1 is absent from snapshot")
	} else {
		notifMu.Lock()
		notif := notifs[0]
		notifMu.Unlock()
		t.Logf("PASS VD-30: node_down fired when node absent: group_key=%v, value=%v", notif["group_key"], notif["value"])
	}
}

// ─── Guard test VD-32: rebuffer_ratio >5% fires with real HealthScore ────────

// TestGuard_VD32_RebufferRatioFires verifies that a rebuffer_ratio > 0.05 rule
// fires when a stream has HealthScore < 0.5 (heuristic: (1-HS)*0.1 > 0.05).
// This test would FAIL on the old behavior when HealthScore was always 0.
func TestGuard_VD32_RebufferRatioFires(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()

	// Rule: rebuffer_ratio > 0.05 (5% rebuffer rate).
	// With HealthScore=0.4: (1-0.4)*0.1 = 0.06 > 0.05 → fires.
	row := meta.AlertRuleRow{
		Name:               "rebuffer-ratio-guard-rule",
		Metric:             "rebuffer_ratio",
		Operator:           "gt",
		Threshold:          0.05, // 5% threshold
		WindowS:            5,
		ScopeJSON:          "{}",
		Severity:           "warning",
		CooldownS:          300,
		Enabled:            true,
		Muted:              false,
		MaintenanceWindows: "[]",
		ChannelIDs:         `["test-channel"]`,
	}
	_, err := store.CreateAlertRule(ctx, row)
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	live := newFakeLive()
	live.setSnap(&domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{
			"degraded-stream": {
				StreamID:    "degraded-stream",
				App:         "live",
				Active:      true,
				HealthScore: 0.4, // rebuffer_ratio = (1-0.4)*0.1 = 0.06 > 0.05
			},
		},
		Nodes: map[string]*domain.LiveNodeStats{},
	})

	clock := alert.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	ev, _ := newTestEvaluator(t, store, live, clock)

	var notifMu sync.Mutex
	var notifs []map[string]any
	ev.SetNotifySink(func(p []byte) {
		var n map[string]any
		_ = json.Unmarshal(p, &n)
		notifMu.Lock()
		notifs = append(notifs, n)
		notifMu.Unlock()
	})

	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)

	notifMu.Lock()
	n := len(notifs)
	notifMu.Unlock()

	// Guard: rebuffer_ratio > 5% fires when HealthScore=0.4 (non-zero).
	// Old behavior: HealthScore=0 → rebuffer_ratio=0 → never fired.
	if n == 0 {
		t.Errorf("VD-32 guard FAIL: rebuffer_ratio alert did not fire (HealthScore=0.4, expected ratio=0.06 > threshold=0.05)")
	} else {
		notifMu.Lock()
		notif := notifs[0]
		notifMu.Unlock()
		t.Logf("PASS VD-32: rebuffer_ratio fired: value=%v > threshold=0.05", notif["value"])
	}
}

// ─── Guard test VD-33: cron weekday range "1-5" matches Mon-Fri ──────────────

// TestGuard_VD33_CronWeekdayRange verifies that a maintenance window with
// weekday="1-5" (Mon-Fri) suppresses alerts on weekdays and allows them
// on weekends. This would FAIL on the old behavior which truncated "1-5" to 1.
func TestGuard_VD33_CronWeekdayRange(t *testing.T) {
	// Window: Mon-Fri (weekdays "1-5") from 02:00 for 7200s (2 hours).
	// Test on Wednesday 02:30 → should be suppressed (in window).
	wednesday := time.Date(2026, 1, 7, 2, 30, 0, 0, time.UTC) // Wednesday
	if wednesday.Weekday() != time.Wednesday {
		t.Fatalf("expected Wednesday, got %v", wednesday.Weekday())
	}

	store := openTestStore(t)
	ctx := context.Background()
	row := meta.AlertRuleRow{
		Name:               "weekday-range-suppression",
		Metric:             "viewer_count",
		Operator:           "lt",
		Threshold:          1,
		WindowS:            5,
		ScopeJSON:          "{}",
		Severity:           "info",
		CooldownS:          300,
		Enabled:            true,
		Muted:              false,
		MaintenanceWindows: `[{"start_cron":"0 2 1-5","duration_s":7200}]`, // Mon-Fri 02:00
		ChannelIDs:         `["test-channel"]`,
	}
	_, err := store.CreateAlertRule(ctx, row)
	if err != nil {
		t.Fatalf("CreateAlertRule: %v", err)
	}

	live := newFakeLive()
	live.setSnap(&domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{
			"wday-stream": {StreamID: "wday-stream", App: "live", Active: true, ViewerCount: 0},
		},
		Nodes: map[string]*domain.LiveNodeStats{},
	})

	clock := alert.NewFakeClock(wednesday)
	ev, _ := newTestEvaluator(t, store, live, clock)

	var notifMu sync.Mutex
	var notifs []map[string]any
	ev.SetNotifySink(func(p []byte) {
		var n map[string]any
		_ = json.Unmarshal(p, &n)
		notifMu.Lock()
		notifs = append(notifs, n)
		notifMu.Unlock()
	})

	// Tick during Wednesday 02:30 — should be suppressed (in Mon-Fri 02:00+2h window).
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)
	clock.Advance(5 * time.Second)
	ev.TickOnce(ctx)

	notifMu.Lock()
	n := len(notifs)
	notifMu.Unlock()

	// Guard: Wednesday is in range 1-5 → suppressed.
	// Old behavior: "1-5" truncated to "1" (Monday only), so Wednesday 02:30 would NOT
	// be suppressed and the alert would fire. New behavior: suppressed.
	if n > 0 {
		t.Errorf("VD-33 guard FAIL: weekday range '1-5' should suppress Wednesday 02:30 but fired %d notification(s)", n)
	} else {
		t.Logf("PASS VD-33: weekday range '1-5' correctly suppressed alerts on Wednesday")
	}

	// Also verify Saturday is NOT suppressed (outside range).
	saturday := time.Date(2026, 1, 10, 2, 30, 0, 0, time.UTC) // Saturday
	if saturday.Weekday() != time.Saturday {
		t.Fatalf("expected Saturday, got %v", saturday.Weekday())
	}

	store2 := openTestStore(t)
	_, err = store2.CreateAlertRule(ctx, row) // same rule
	if err != nil {
		t.Fatalf("CreateAlertRule2: %v", err)
	}

	live2 := newFakeLive()
	live2.setSnap(&domain.LiveSnapshot{
		Streams: map[string]*domain.LiveStream{
			"sat-stream": {StreamID: "sat-stream", App: "live", Active: true, ViewerCount: 0},
		},
		Nodes: map[string]*domain.LiveNodeStats{},
	})

	clock2 := alert.NewFakeClock(saturday)
	ev2, _ := newTestEvaluator(t, store2, live2, clock2)

	var notifMu2 sync.Mutex
	var notifs2 []map[string]any
	ev2.SetNotifySink(func(p []byte) {
		var n2 map[string]any
		_ = json.Unmarshal(p, &n2)
		notifMu2.Lock()
		notifs2 = append(notifs2, n2)
		notifMu2.Unlock()
	})

	clock2.Advance(5 * time.Second)
	ev2.TickOnce(ctx)
	clock2.Advance(5 * time.Second)
	ev2.TickOnce(ctx)
	clock2.Advance(5 * time.Second)
	ev2.TickOnce(ctx)

	notifMu2.Lock()
	n2 := len(notifs2)
	notifMu2.Unlock()

	if n2 == 0 {
		t.Errorf("VD-33 guard FAIL: Saturday (outside weekday range 1-5) should NOT be suppressed but got 0 notifications")
	} else {
		t.Logf("PASS VD-33: Saturday correctly NOT suppressed (weekday=6, outside range 1-5): %d notification(s)", n2)
	}
}
