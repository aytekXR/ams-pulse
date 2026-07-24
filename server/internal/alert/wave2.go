// Package alert — Wave 2 additions:
//   - New rule metrics: rebuffer_ratio, error_rate, ingest_bitrate_floor,
//     cert_expiry, node_up (from cluster.Discovery node events)
//   - Cron-expression maintenance windows (closes G2)
//   - Default rule pack seeding on bootstrap (closes G8)
//
// The Evaluator handles the new metrics via the evalGenericMetric fallback
// plus the cert_expiry special handler added in this file.
package alert

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// ─── Wave 2: QoE reader interface (D-062) ────────────────────────────────────

// QoEReader queries per-stream QoE metrics from ClickHouse aggregate rollups.
// Implemented by query.Service; use FakeQoEReader in tests.
//
// conn == nil → (0, 0, nil) fall-through: caller must treat (0, 0, nil) as
// "no data" and evaluate normally; 0.0 is a legitimate rebuffer_ratio/error_rate.
// tenant scopes the QoE read to one tenant (F6 Phase 2); "" = all tenants. This
// closes S73 finding [5]: without it, a QoE rule for a stream that two tenants
// reuse (same app+stream) blends both tenants' rebuffer/error numbers.
type QoEReader interface {
	QoEForStream(ctx context.Context, streamID, app, tenant string, lookback time.Duration) (rebufferRatio, errorRate float64, err error)
}

// FakeQoEReader returns fixed values for testing.
// Set Err to simulate a reader error (stream is skipped, no panic).
type FakeQoEReader struct {
	RebufferRatio float64
	ErrorRate     float64
	Err           error
	// LastTenant records the tenant the evaluator last passed, so tests can
	// prove the rule's tenant scope is threaded through to the reader.
	LastTenant string
}

// QoEForStream returns the pre-configured values for tests and records the
// tenant it was called with.
func (f *FakeQoEReader) QoEForStream(_ context.Context, _, _, tenant string, _ time.Duration) (float64, float64, error) {
	f.LastTenant = tenant
	return f.RebufferRatio, f.ErrorRate, f.Err
}

// ─── Wave 2: New metric evaluators ───────────────────────────────────────────

// evalQoEMetric evaluates QoE-derived metrics:
//   - rebuffer_ratio: from rollup_qoe_1h via QoEReader (D-062: proxy removed, G6)
//   - error_rate: from rollup_qoe_1h via QoEReader (D-062: proxy removed, G6)
//   - ingest_bitrate_floor: from live snapshot IngestBitrate (real poller data)
//
// Reader errors (Fix C): when the QoE reader returns an error, the affected
// stream emits a hold=true result so processEvaluation preserves the current
// alert state without firing, resolving, or flapping. The WARN log is
// rate-limited to once per transition (entering/leaving error mode) instead of
// once per 15 s tick. A nil reader always skips (logs once per tick — config
// issue, not a transient error). A value of 0.0 from the reader is legitimate.
func (e *Evaluator) evalQoEMetric(ctx context.Context, snap *domain.LiveSnapshot, scope domain.AlertScope, rule meta.AlertRuleRow) []evalResult {
	var results []evalResult
	var warnedNil bool
	hadReaderErr := false

	for sid, s := range snap.Streams {
		if scope.App != "" && s.App != scope.App {
			continue
		}
		if scope.StreamID != "" && sid != scope.StreamID {
			continue
		}
		if scope.NodeID != "" && s.NodeID != scope.NodeID {
			continue
		}

		var val float64
		switch rule.Metric {
		case "rebuffer_ratio", "error_rate":
			// D-062: values come from ClickHouse aggregate rollups via QoEReader.
			// HealthScore proxy formulas are removed (ROADMAP G6: no silently-approximated metrics).
			e.mu.Lock()
			reader := e.qoeReader
			e.mu.Unlock()
			if reader == nil {
				if !warnedNil {
					e.logger.Warn("alert: qoe_reader not configured — rebuffer_ratio/error_rate rules skipped this tick (D-062: G6)")
					warnedNil = true
				}
				continue
			}
			rebuf, errRate, err := reader.QoEForStream(ctx, sid, s.App, scope.Tenant, 1*time.Hour)
			if err != nil {
				// Fix C: hold state — emit a hold result so the state machine keeps the
				// current firing/pending state without transitions. Do not resolve or
				// re-fire: the underlying condition has not changed, we just cannot read it.
				hadReaderErr = true
				results = append(results, evalResult{groupKey: sid, hold: true})
				continue
			}
			if rule.Metric == "rebuffer_ratio" {
				val = rebuf
			} else {
				val = errRate
			}
		case "ingest_bitrate_floor":
			// Direct: use ingest bitrate from the live health tracker (real poller data).
			val = s.IngestBitrate
		default:
			continue
		}
		results = append(results, evalResult{
			groupKey: sid,
			value:    val,
			ok:       compare(val, rule.Operator, rule.Threshold),
		})
	}

	// Fix C: rate-limited WARN — log once on entry into error mode, once on recovery.
	e.mu.Lock()
	prevErrMode := e.qoeErrMode
	e.qoeErrMode = hadReaderErr
	e.mu.Unlock()
	switch {
	case hadReaderErr && !prevErrMode:
		e.logger.Warn("alert: qoe_reader error — holding alert states until reader recovers")
	case !hadReaderErr && prevErrMode:
		e.logger.Warn("alert: qoe_reader recovered — normal QoE evaluation resumed")
	}

	return results
}

// evalNodeUpDown evaluates node_up / node_down rule types.
//
// VD-30: node_down fires when a node is absent from the snapshot, i.e. it was
// evicted by EvictStaleNodes() because no stats arrived within 3×PollInterval.
// The previous CPU>95 proxy is replaced with real absence detection.
//
// How absence detection works:
//   - EvictStaleNodes() removes nodes from Nodes map when LastSeenAt is stale.
//   - Once evicted, a node no longer appears in snap.Nodes.
//   - Scoped rules (scope.NodeID set): fire immediately when the named node is absent.
//   - Wildcard rules (scope.NodeID ""): use cross-tick tracking via nodeDownTracker
//     (Fix B) — diff prevPresent vs current to detect new absences, and continue
//     firing for nodes that remain absent across ticks.
//
// now is the current evaluation tick time, used to record when a node first went down.
func (e *Evaluator) evalNodeUpDown(snap *domain.LiveSnapshot, scope domain.AlertScope, rule meta.AlertRuleRow, now time.Time) []evalResult {
	var results []evalResult

	switch rule.Metric {
	case "node_down":
		if scope.NodeID != "" {
			// Scoped: fire if the named node is absent from the snapshot.
			if _, present := snap.Nodes[scope.NodeID]; !present {
				results = append(results, evalResult{
					groupKey: scope.NodeID,
					value:    1.0,  // 1 = down
					ok:       true, // condition met (node is down)
				})
			} else {
				// Node is present — it's up; resolves any firing alert.
				results = append(results, evalResult{
					groupKey: scope.NodeID,
					value:    0.0,
					ok:       false,
				})
			}
		} else {
			// Wildcard: cross-tick node-presence tracking (Fix B — previously inert).
			results = e.evalWildcardNodeDown(snap, rule, now)
		}

	case "node_degraded":
		for nid, n := range snap.Nodes {
			if scope.NodeID != "" && nid != scope.NodeID {
				continue
			}
			var val float64
			// D-088: single predicate shared with query FleetNodes/LiveOverview.
			if n.Degraded() {
				val = 1.0
			}
			results = append(results, evalResult{
				groupKey: nid,
				value:    val,
				ok:       compare(val, rule.Operator, rule.Threshold),
			})
		}
	}

	return results
}

// evalWildcardNodeDown implements wildcard node_down evaluation (Fix B).
//
// A node absent from snap.Nodes has been evicted by EvictStaleNodes() because no
// stats arrived within 3×PollInterval — the same eviction that the scoped case
// detects. Wildcard rules cannot enumerate absent nodes from a single snapshot;
// instead, the nodeDownTracker diffs the present set across ticks:
//
//   - New absence (was in prevPresent, gone now): recorded in downSince; emits 1.0.
//   - Persistent absence (in downSince, still gone): continues emitting 1.0 each tick.
//   - Return (appears in snap.Nodes): removed from downSince; emits 0.0 (resolves alert).
//   - Present node: emits 0.0 unconditionally.
//
// Holds e.mu for the duration to guard shared nodeDownTrackers map access.
func (e *Evaluator) evalWildcardNodeDown(snap *domain.LiveSnapshot, rule meta.AlertRuleRow, now time.Time) []evalResult {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.nodeDownTrackers == nil {
		e.nodeDownTrackers = make(map[string]*nodeDownTracker)
	}
	tr := e.nodeDownTrackers[rule.ID]
	if tr == nil {
		tr = &nodeDownTracker{
			prevPresent: map[string]bool{},
			downSince:   map[string]time.Time{},
		}
		e.nodeDownTrackers[rule.ID] = tr
	}

	// Current node set from snapshot.
	present := make(map[string]bool, len(snap.Nodes))
	for nid := range snap.Nodes {
		present[nid] = true
	}

	// Detect new down edges: nodes present last tick that are now absent.
	for nid := range tr.prevPresent {
		if !present[nid] {
			if _, tracked := tr.downSince[nid]; !tracked {
				tr.downSince[nid] = now
			}
		}
	}

	results := make([]evalResult, 0, len(present)+len(tr.downSince))

	// Present nodes: up. Remove from downSince if they returned (resolves alert).
	for nid := range present {
		delete(tr.downSince, nid)
		results = append(results, evalResult{
			groupKey: nid,
			value:    0.0,
			ok:       compare(0.0, rule.Operator, rule.Threshold),
		})
	}

	// Tracked-down nodes: continuously fire until they return to the snapshot.
	for nid := range tr.downSince {
		results = append(results, evalResult{
			groupKey: nid,
			value:    1.0,
			ok:       compare(1.0, rule.Operator, rule.Threshold),
		})
	}

	tr.prevPresent = present
	return results
}

// ─── Wave 2: Cert expiry checker ─────────────────────────────────────────────

// CertChecker performs TLS certificate expiry checks.
// Called by the evaluator on a daily tick for cert_expiry rules.
type CertChecker struct {
	dialTimeout time.Duration
	tlsConfig   *tls.Config // optional; nil = use default (server cert validation)
}

// NewCertChecker creates a CertChecker with the given dial timeout.
func NewCertChecker(dialTimeout time.Duration) *CertChecker {
	if dialTimeout <= 0 {
		dialTimeout = 10 * time.Second
	}
	return &CertChecker{dialTimeout: dialTimeout, tlsConfig: nil}
}

// NewCertCheckerWithTLSConfig creates a CertChecker with a custom TLS config.
// Use for testing with self-signed certificates.
func NewCertCheckerWithTLSConfig(cfg *tls.Config, dialTimeout time.Duration) *CertChecker {
	if dialTimeout <= 0 {
		dialTimeout = 10 * time.Second
	}
	return &CertChecker{dialTimeout: dialTimeout, tlsConfig: cfg}
}

// DaysUntilExpiry returns the number of days until the TLS certificate at
// host:port expires. Returns -1 with a nil error if the certificate is already
// expired — including the common case where TLS verification itself rejects the
// handshake specifically because the (otherwise-trusted) leaf has expired — so a
// cert_expiry lt 0 rule can fire. Any other failure returns (-1, non-nil error).
func (c *CertChecker) DaysUntilExpiry(ctx context.Context, host string) (float64, error) {
	// Parse host:port; default port 443.
	h, port, err := net.SplitHostPort(host)
	if err != nil {
		// No port in host string.
		h = host
		port = "443"
	}
	addr := net.JoinHostPort(h, port)

	dialCtx, cancel := context.WithTimeout(ctx, c.dialTimeout)
	defer cancel()

	tlsCfg := c.tlsConfig
	if tlsCfg == nil {
		tlsCfg = &tls.Config{InsecureSkipVerify: false, ServerName: h}
	}

	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{},
		Config:    tlsCfg,
	}
	conn, err := dialer.DialContext(dialCtx, "tcp", addr)
	if err != nil {
		// A verification failure whose specific reason is Expired is a measurable
		// "already expired" state, not a check failure: return -1 with a nil error so a
		// cert_expiry lt 0 rule fires (this is the common production path — a trusted-CA
		// leaf that has expired fails the handshake before we can read NotAfter). Any
		// other dial/verification failure stays a real error (D-134/S72 [22]).
		var invalid x509.CertificateInvalidError
		if errors.As(err, &invalid) && invalid.Reason == x509.Expired {
			return -1, nil
		}
		return -1, fmt.Errorf("cert check: dial %s: %w", addr, err)
	}
	defer conn.Close()

	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return -1, fmt.Errorf("cert check: expected TLS conn")
	}

	certs := tlsConn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return -1, fmt.Errorf("cert check: no peer certificates from %s", addr)
	}

	// Check the leaf cert (index 0).
	leaf := certs[0]
	now := time.Now()
	if now.After(leaf.NotAfter) {
		// Already expired: return the -1 sentinel promised by the docstring above.
		// Returning 0 made a `cert_expiry lt 0` rule never fire (0 < 0 is false), so an
		// operator watching for an expired cert got no alert (D-134/S72 [22]).
		return -1, nil
	}
	daysLeft := leaf.NotAfter.Sub(now).Hours() / 24
	return daysLeft, nil
}

// FakeCertChecker returns a fixed days value for testing.
type FakeCertChecker struct {
	DaysLeft float64
	Err      error
}

// DaysUntilExpiry returns the pre-configured value for tests.
func (f *FakeCertChecker) DaysUntilExpiry(_ context.Context, _ string) (float64, error) {
	return f.DaysLeft, f.Err
}

// CertExpiryChecker is the interface used by the evaluator.
type CertExpiryChecker interface {
	DaysUntilExpiry(ctx context.Context, host string) (float64, error)
}

// evalCertExpiry evaluates cert_expiry rules.
// Rule metric: cert_expiry, operator: lt, threshold: 30 → fire if cert expires in < 30 days.
// Rule scope: host field in scope JSON (future: add to AlertScope; for now uses stream_id as host).
func (e *Evaluator) evalCertExpiry(ctx context.Context, rule meta.AlertRuleRow, scope domain.AlertScope, checker CertExpiryChecker) []evalResult {
	host := scope.StreamID // convention: scope.stream_id = host:port for cert rules
	if host == "" {
		return nil
	}
	days, err := checker.DaysUntilExpiry(ctx, host)
	if err != nil {
		e.logger.Warn("alert: cert_expiry check failed", "host", host, "error", err)
		return nil
	}
	return []evalResult{{
		groupKey: host,
		value:    days,
		ok:       compare(days, rule.Operator, rule.Threshold),
	}}
}

// ─── Wave 2: Cron maintenance window parsing (closes G2) ─────────────────────

// maintenanceWindow is one parsed maintenance window.
type maintenanceWindow struct {
	// StartCron is a simplified cron expression: "min hour weekday" (5-field subset).
	// Supported patterns:
	//   "0 2 *"    → every day at 02:00
	//   "0 2 0"    → every Sunday at 02:00 (0=Sunday, 6=Saturday)
	//   "0 0 1-5"  → every Mon-Fri at midnight
	StartCron string `json:"start_cron"`
	// DurationS is the maintenance window duration in seconds.
	DurationS int `json:"duration_s"`
}

// parseCronSimple parses a simplified 3-field cron "min hour weekday".
// Returns (min, hour, weekday) where -1 means any (wildcard).
// Weekday 0=Sunday..6=Saturday (matching time.Weekday).
//
// Supported field syntax:
//   - "*"   → -1 (any)
//   - "2"   → 2 (exact match)
//   - "1-5" → the set {1,2,3,4,5}; cronMatches checks membership
//
// VD-33: range syntax "1-5" is now expanded into a set and checked via
// cronMatchesField rather than truncated to the first value.
func parseCronSimple(expr string) (min, hour, weekday int, err error) {
	return parseCronSimpleInternal(strings.Fields(expr))
}

// parseCronSimpleInternal is the shared impl for 2-3 field crons.
func parseCronSimpleInternal(fields []string) (min, hour, weekday int, err error) {
	if len(fields) < 2 || len(fields) > 3 {
		return 0, 0, -1, fmt.Errorf("cron: expected 2-3 fields, got %d", len(fields))
	}
	parseField := func(s string) (int, error) {
		if s == "*" {
			return -1, nil
		}
		// Range like "1-5": return the low bound; cronMatchesField handles set check.
		if idx := strings.Index(s, "-"); idx >= 0 {
			n, err := strconv.Atoi(s[:idx])
			return n, err
		}
		n, err := strconv.Atoi(s)
		return n, err
	}

	min, err = parseField(fields[0])
	if err != nil {
		return 0, 0, -1, fmt.Errorf("cron: invalid minute %q: %w", fields[0], err)
	}
	hour, err = parseField(fields[1])
	if err != nil {
		return 0, 0, -1, fmt.Errorf("cron: invalid hour %q: %w", fields[1], err)
	}
	if len(fields) == 3 {
		weekday, err = parseField(fields[2])
		if err != nil {
			return 0, 0, -1, fmt.Errorf("cron: invalid weekday %q: %w", fields[2], err)
		}
	} else {
		weekday = -1
	}
	return min, hour, weekday, nil
}

// cronFieldSet parses a cron field into a set of matching integers.
// Returns (-1,nil) for "*" (any), a populated set for ranges, or a single value.
// VD-33: ranges like "1-5" expand to all values in [low, high].
func cronFieldSet(s string) (set map[int]struct{}, any bool, err error) {
	if s == "*" {
		return nil, true, nil
	}
	if idx := strings.Index(s, "-"); idx >= 0 {
		low, err1 := strconv.Atoi(s[:idx])
		high, err2 := strconv.Atoi(s[idx+1:])
		if err1 != nil || err2 != nil || low > high {
			return nil, false, fmt.Errorf("cron: invalid range %q", s)
		}
		m := make(map[int]struct{}, high-low+1)
		for v := low; v <= high; v++ {
			m[v] = struct{}{}
		}
		return m, false, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil, false, fmt.Errorf("cron: invalid value %q: %w", s, err)
	}
	return map[int]struct{}{n: {}}, false, nil
}

// cronFieldMatches returns true if val matches the cron field string s.
// Supports "*", exact values, and ranges "lo-hi" (VD-33).
func cronFieldMatches(s string, val int) bool {
	set, any, err := cronFieldSet(s)
	if err != nil {
		return false
	}
	if any {
		return true
	}
	_, ok := set[val]
	return ok
}

// cronMatches returns true if now falls within the maintenance window
// defined by startCron + durationS.
//
// VD-33: uses range-aware field matching so "1-5" in the weekday field
// matches weekdays Monday through Friday.
func cronMatches(startCron string, durationS int, now time.Time) bool {
	fields := strings.Fields(startCron)
	if len(fields) < 2 || len(fields) > 3 {
		return false
	}

	// Resolve min and hour (integers, not ranges — for computing window start time).
	min, hour, _, err := parseCronSimpleInternal(fields)
	if err != nil {
		return false
	}
	if min < 0 {
		min = 0 // treat wildcard minute as :00 for window start computation
	}
	if hour < 0 {
		hour = 0 // treat wildcard hour as midnight for window start computation
	}

	// Check weekday field (supports "*", exact, and "lo-hi" range).
	if len(fields) == 3 {
		if !cronFieldMatches(fields[2], int(now.Weekday())) {
			return false
		}
	}

	// Compute window start: today at hour:min.
	loc := now.Location()
	year, month, day := now.Date()
	windowStart := time.Date(year, month, day, hour, min, 0, 0, loc)
	windowEnd := windowStart.Add(time.Duration(durationS) * time.Second)

	return !now.Before(windowStart) && now.Before(windowEnd)
}

// inMaintenanceWindowCron returns true if now falls within any cron-based maintenance window.
// Replaces the wave-1 stub in evaluator.go.
func inMaintenanceWindowCron(rule meta.AlertRuleRow, now time.Time) bool {
	if rule.MaintenanceWindows == "[]" || rule.MaintenanceWindows == "" {
		return false
	}
	var windows []maintenanceWindow
	if err := jsonUnmarshal([]byte(rule.MaintenanceWindows), &windows); err != nil {
		return false
	}
	for _, w := range windows {
		if w.StartCron != "" && w.DurationS > 0 {
			if cronMatches(w.StartCron, w.DurationS, now) {
				return true
			}
		}
	}
	return false
}

// jsonUnmarshal decodes JSON data into v (uses encoding/json).
func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// ─── Wave 2: Default rule pack (closes G8) ───────────────────────────────────

// DefaultRulePack is the set of rules seeded on first bootstrap.
// All rules are enabled=true, muted=true initially (active but silent).
// Operators must disable muting explicitly to get notifications.
var DefaultRulePack = []meta.AlertRuleRow{
	{
		Name:               "Stream offline (default)",
		Metric:             "stream_offline",
		Operator:           "eq",
		Threshold:          1,
		WindowS:            30,
		ScopeJSON:          "{}",
		Severity:           "critical",
		CooldownS:          300,
		Enabled:            true,
		Muted:              true, // enabled-but-muted per G8 requirement
		MaintenanceWindows: "[]",
		ChannelIDs:         "[]",
	},
	{
		// Fix D: metric renamed from viewer_drop_pct to viewer_count_floor (honest name).
		// The metric compares an absolute viewer count, not a percentage — threshold 0.5
		// with integer counts is equivalent to threshold 1 (fires when count == 0).
		// Existing stored rules using viewer_drop_pct continue to evaluate via the alias.
		Name:               "Viewer floor breach (default)",
		Metric:             "viewer_count_floor",
		Operator:           "lt",
		Threshold:          1, // fire when viewer count < 1 (0 viewers on any stream)
		WindowS:            60,
		ScopeJSON:          "{}",
		Severity:           "warning",
		CooldownS:          600,
		Enabled:            true,
		Muted:              true,
		MaintenanceWindows: "[]",
		ChannelIDs:         "[]",
	},
	{
		Name:               "Node CPU > 90% (default)",
		Metric:             "node_cpu",
		Operator:           "gt",
		Threshold:          90,
		WindowS:            120,
		ScopeJSON:          "{}",
		Severity:           "warning",
		CooldownS:          300,
		Enabled:            true,
		Muted:              true,
		MaintenanceWindows: "[]",
		ChannelIDs:         "[]",
	},
	{
		Name:               "Ingest bitrate floor breach (default)",
		Metric:             "ingest_bitrate_floor",
		Operator:           "lt",
		Threshold:          500, // < 500 kbps = likely degraded
		WindowS:            30,
		ScopeJSON:          "{}",
		Severity:           "warning",
		CooldownS:          300,
		Enabled:            true,
		Muted:              true,
		MaintenanceWindows: "[]",
		ChannelIDs:         "[]",
	},
}

// SeedDefaultRulePack seeds the default rule pack if no rules exist yet.
// Called on first bootstrap; idempotent (no-op if rules already exist).
func SeedDefaultRulePack(ctx context.Context, store *meta.Store, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	existing, err := store.ListAlertRules(ctx, 0, "")
	if err != nil {
		return fmt.Errorf("seed default rules: list: %w", err)
	}
	if len(existing) > 0 {
		// Rules already exist — skip seeding.
		return nil
	}
	for _, rule := range DefaultRulePack {
		if _, err := store.CreateAlertRule(ctx, rule); err != nil {
			logger.Warn("alert: seed default rule failed", "name", rule.Name, "error", err)
		} else {
			logger.Info("alert: seeded default rule (enabled+muted)", "name", rule.Name)
		}
	}
	return nil
}
