// Package alert implements the rule engine (F5): rules evaluated on streaming
// aggregates held in memory with ClickHouse fallback for window queries.
//
// # Detection-to-notification latency
//
// The evaluator tick is ≤5 s. Cooldowns are at minimum 1 s. The total
// detection-to-notification path is:
//
//	collector poll → aggregator update → evaluator tick → channel.Send
//
// With default tick=5 s and default poll=5 s, worst-case latency is:
//
//	5 s (poll) + 5 s (tick) + ~0.1 s (send) = ~10.1 s
//
// This is well within the 30 s budget (PRD F5 / ARCHITECTURE §4).
// The fake-clock test in evaluator_test.go proves this by construction.
//
// # State machine
//
//	pending → firing (condition met for window_s)
//	firing  → resolved (condition no longer met)
//	firing  → firing (suppressed by cooldown / maintenance window)
//
// # Storm protection
//
// group_by dimension: all streams matching a rule with group_by=stream_id
// produce one notification per stream_id group key, not one per stream.
// Without group_by, every distinct scope triggers independently.
package alert

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand"
	"regexp"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/pulse-analytics/pulse/server/internal/alert/channels"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// Clock is a time source — allows fake-clock injection in tests.
type Clock interface {
	Now() time.Time
}

// RealClock is the wall-clock implementation.
type RealClock struct{}

// Now returns time.Now().
func (RealClock) Now() time.Time { return time.Now() }

// FakeClock is a controllable clock for tests.
type FakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// NewFakeClock creates a fake clock starting at t.
func NewFakeClock(t time.Time) *FakeClock { return &FakeClock{now: t} }

// Now returns the current fake time.
func (f *FakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Advance moves the clock forward by d.
func (f *FakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

// ─── Rule state tracking ───────────────────────────────────────────────────────

// ruleState tracks the evaluation state for one (rule, group_key) pair.
type ruleState struct {
	ruleID        string
	groupKey      string
	alertID       string // current firing instance ID
	state         string // "pending" | "firing" | "resolved"
	firedAt       time.Time
	lastCheck     time.Time
	cooldownUntil time.Time
	pendingSince  time.Time
}

// offlineTracker holds the cross-tick state a WILDCARD stream_offline rule needs
// (D-157). A wildcard "any stream went offline" alert is an EDGE event: the
// aggregator removes an ended stream from the snapshot (aggregator.onPublishEnd),
// so "offline" cannot be read from a single snapshot — it must be inferred by
// diffing the scope-matching present-stream set across ticks. One tracker per
// wildcard stream_offline rule (keyed by rule ID).
type offlineTracker struct {
	// prevPresent is the set of scope-matching stream IDs present last tick.
	prevPresent map[string]bool
	// offlineAt maps a stream ID that went present→gone to the time it went
	// offline. The rule emits value 1.0 for it until the hold window elapses,
	// then one resolving 0.0, then it is dropped.
	offlineAt map[string]time.Time
	// holdUntil is the ABSOLUTE hold deadline for each offline stream, frozen at
	// detection time (D-159). The value 1.0 is emitted while now < holdUntil, then
	// one resolving 0.0. Freezing it at detection (rather than recomputing
	// streamOfflineHold(rule.WindowS, …) every tick) makes the hold immune to a
	// mid-event WindowS edit — a decreased WindowS would otherwise retroactively
	// expire an in-flight offline event and silently swallow its page.
	holdUntil map[string]time.Time
}

// ─── Evaluator ────────────────────────────────────────────────────────────────

// Config holds evaluator configuration.
type Config struct {
	// TickInterval is the evaluation loop interval (default 5s, max 30s).
	// Detection→notification is bounded by tick + one poll interval.
	TickInterval time.Duration

	// BaseURL is used to build dashboard_url in notifications.
	BaseURL string

	// RetryBaseDelay is the initial backoff delay before the first retry (default 500ms).
	// Tests should set this to a small value (e.g. 1ms) to avoid sleeping.
	RetryBaseDelay time.Duration

	// RetryCap is the maximum backoff delay before any single retry (default 5s).
	RetryCap time.Duration

	// RetryMaxAttempts is the number of retries after the initial attempt (default 3).
	// Total delivery attempts = 1 + RetryMaxAttempts.
	RetryMaxAttempts int
}

// Evaluator runs alert rules against live aggregates.
type Evaluator struct {
	cfg      Config
	store    *meta.Store
	live     domain.LiveProvider
	registry *channels.Registry
	clock    Clock
	logger   *slog.Logger

	mu     sync.Mutex
	states map[string]*ruleState // key = ruleID+":"+groupKey

	// offlineTrackers holds per-wildcard-stream_offline-rule edge-detection state
	// (D-157). Keyed by rule ID. Guarded by mu. Pruned in evaluate() once a rule is
	// no longer an actively-evaluated wildcard stream_offline rule.
	offlineTrackers map[string]*offlineTracker

	// Wave 2: TLS cert expiry checker (nil = cert_expiry rules skipped).
	certChecker CertExpiryChecker

	// S39: licence-key expiry checker (nil = license_expiry rules skipped).
	licenseChecker LicenseExpiryChecker

	// D-062: QoE reader for rebuffer_ratio / error_rate from ClickHouse rollups.
	// nil = these rules are skipped with a WARN log (one per tick).
	qoeReader QoEReader

	// S11 WO-B: baseline reader for anomaly rules (nil = skipped with WARN).
	anomalyReader AnomalyBaselineReader

	// Notification sink for tests.
	notifySink func([]byte)

	// deliveryWg tracks all in-flight delivery goroutines so Stop() can wait
	// for a bounded shutdown (all goroutines exit after ctx is cancelled).
	deliveryWg sync.WaitGroup

	// syncedChannelIDs is the set of channel IDs most recently synced from the
	// meta store by syncRegistryFromStore. Used to remove deleted channels from
	// the registry without touching manually-registered channels (e.g. test fakes).
	syncedChannelIDs map[string]bool
}

// deliveryCtx carries the alert event data used if a delivery_failure must
// be recorded. It is populated from the firing/resolved event context and
// threaded through the async delivery goroutine.
type deliveryCtx struct {
	alertID   string
	ruleID    string
	severity  string
	metric    string
	value     float64
	threshold float64
	scopeJSON string
}

// New creates an Evaluator. If clock is nil, RealClock is used.
// If logger is nil, a discard logger is used (useful for tests).
func New(cfg Config, live domain.LiveProvider, store *meta.Store, registry *channels.Registry, clock Clock, logger *slog.Logger) *Evaluator {
	if cfg.TickInterval <= 0 || cfg.TickInterval > 30*time.Second {
		cfg.TickInterval = 5 * time.Second
	}
	if cfg.RetryBaseDelay <= 0 {
		cfg.RetryBaseDelay = 500 * time.Millisecond
	}
	if cfg.RetryCap <= 0 {
		cfg.RetryCap = 5 * time.Second
	}
	if cfg.RetryMaxAttempts <= 0 {
		cfg.RetryMaxAttempts = 3
	}
	if clock == nil {
		clock = RealClock{}
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Evaluator{
		cfg:             cfg,
		store:           store,
		live:            live,
		registry:        registry,
		clock:           clock,
		logger:          logger,
		states:          make(map[string]*ruleState),
		offlineTrackers: make(map[string]*offlineTracker),
	}
}

// SetNotifySink registers a function that receives every notification payload.
// Used in tests to capture notifications without real channels.
func (e *Evaluator) SetNotifySink(fn func([]byte)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.notifySink = fn
}

// SetCertChecker configures the TLS cert expiry checker for cert_expiry rules.
// If not set, cert_expiry rules are silently skipped.
func (e *Evaluator) SetCertChecker(checker CertExpiryChecker) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.certChecker = checker
}

// SetLicenseChecker configures the licence-key expiry checker for license_expiry
// rules. If not set, license_expiry rules are silently skipped.
func (e *Evaluator) SetLicenseChecker(checker LicenseExpiryChecker) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.licenseChecker = checker
}

// SetQoEReader wires the ClickHouse QoE reader for rebuffer_ratio and error_rate rules.
// If not set, those rules are skipped with a WARN log (at most one per tick).
// Call after New, before Start (D-062).
func (e *Evaluator) SetQoEReader(reader QoEReader) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.qoeReader = reader
}

// SetAnomalyBaselineReader wires the meta store baseline reader for anomaly rules.
// If not set, anomaly rules are skipped with a WARN log (at most one per tick).
// Call after New, before Start (S11 WO-B).
//
// D-WOB wiring pin: serve_wiring_test.go references wireAlertAnomalyReader which
// calls this function; deleting it breaks the wiring test compilation.
func (e *Evaluator) SetAnomalyBaselineReader(r AnomalyBaselineReader) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.anomalyReader = r
}

// Start runs the evaluator loop until ctx is cancelled.
func (e *Evaluator) Start(ctx context.Context) {
	go e.loop(ctx)
}

// Stop waits for all in-flight delivery goroutines to finish.
// Cancel the evaluator's context before calling Stop so that goroutines
// blocked in backoff sleeps exit promptly rather than waiting out full delays.
func (e *Evaluator) Stop() {
	e.deliveryWg.Wait()
}

// TickOnce runs a single evaluation cycle synchronously (for tests).
func (e *Evaluator) TickOnce(ctx context.Context) {
	e.evaluate(ctx)
}

// StateCount returns the number of live ruleState entries. Test-only observability for
// the D-160 e.states eviction — a production run never inspects this.
func (e *Evaluator) StateCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.states)
}

// ─── Evaluation loop ──────────────────────────────────────────────────────────

func (e *Evaluator) loop(ctx context.Context) {
	ticker := time.NewTicker(e.cfg.TickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.evaluate(ctx)
		}
	}
}

func (e *Evaluator) evaluate(ctx context.Context) {
	// Sync-on-tick: rebuild the channel registry from the meta store before each
	// evaluation cycle. This ensures that channels created, updated, or deleted via
	// the API are reflected within one tick interval (≤5 s). The rebuild is cheap
	// because channel counts are small, and it is self-healing: a misconfigured or
	// deleted channel is handled gracefully (logged + skipped) without crashing the tick.
	e.syncRegistryFromStore(ctx)

	rules, err := e.store.ListAlertRules(ctx, 0, "")
	if err != nil {
		e.logger.Warn("alert evaluator: list rules failed", "error", err)
		return
	}

	snap := e.live.CurrentSnapshot()
	if snap == nil {
		return
	}

	now := e.clock.Now()

	keepOffline := make(map[string]bool)    // active wildcard offline → full cross-tick state kept
	suspendOffline := make(map[string]bool) // existing-but-suspended wildcard offline → offlineAt preserved
	for _, rule := range rules {
		// A wildcard stream_offline rule carries cross-tick edge-detection state that
		// must be handled specially when the rule is temporarily suspended (D-159 —
		// see pruneOfflineTrackers). Detect the shape BEFORE the suspend guards so a
		// disabled / maintenance-window rule is still recognised as offline-shaped.
		isWildcardOffline := rule.Metric == "stream_offline" && isWildcardOfflineScope(rule.ScopeJSON)

		// enabled=false: rule is completely suspended — not evaluated at all.
		// This is distinct from muted=true (evaluated, but notifications suppressed).
		if !rule.Enabled {
			if isWildcardOffline {
				suspendOffline[rule.ID] = true
			}
			continue
		}
		// Maintenance window suppression.
		if e.inMaintenanceWindow(rule, now) {
			if isWildcardOffline {
				suspendOffline[rule.ID] = true
			}
			continue
		}

		// D-157/D-159: keep an offline tracker at FULL fidelity only while its rule is
		// CONTINUOUSLY an active wildcard stream_offline rule. A rule whose metric/scope
		// changed away from wildcard stream_offline (neither keep nor suspend) has its
		// tracker discarded below.
		if isWildcardOffline {
			keepOffline[rule.ID] = true
		}

		e.evaluateRule(ctx, rule, snap, now)
	}

	e.pruneOfflineTrackers(keepOffline, suspendOffline)
	e.pruneStaleStates(now)
}

// pruneStaleStates evicts ruleState entries whose (rule, group) produced NO evalResult
// this tick (their stream/node has vanished, or the rule was removed) AND which are
// behaviorally INERT — i.e. deleting them is indistinguishable from keeping them,
// because the next evalResult for that key would recreate a fresh "pending" entry that
// behaves identically (D-160). Without this, e.states grew one permanent entry per
// unique (rule, stream_id) ever seen — an unbounded leak on high-stream-churn systems.
//
// An entry is inert iff it is NOT actively firing and NOT mid-pending toward a fire,
// AND its cooldown has expired (evicting a "resolved" entry whose cooldown is still
// active would let a quick re-appearance bypass the remaining cooldown — so those are
// kept until the cooldown lapses). A "firing" entry is never evicted here: dropping it
// would silently discard an unresolved alert (that firing-orphan case is a separate,
// pre-existing concern, deliberately out of scope).
func (e *Evaluator) pruneStaleStates(now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for key, st := range e.states {
		if st.lastCheck.Equal(now) {
			continue // evaluated this tick — actively tracked
		}
		// Inert = NO accumulated re-fire window progress AND settled (resolved or
		// idle-pending). A "resolved" entry can carry a NON-zero pendingSince when its
		// condition has re-met and is accumulating toward a re-fire that a still-active
		// cooldown is currently suppressing; evicting that would discard the progress and
		// delay or miss the re-fire on the stream's return (a behavior a fresh entry does
		// NOT reproduce). So require pendingSince zero to evict. (Residual: a resolved
		// entry that re-met then vanished mid-window is retained — a narrow bounded case,
		// vs. the general one-entry-per-unique-stream-id leak this closes.)
		inert := st.pendingSince.IsZero() && (st.state == "resolved" || st.state == "pending")
		cooldownExpired := st.cooldownUntil.IsZero() || !now.Before(st.cooldownUntil)
		if inert && cooldownExpired {
			delete(e.states, key)
		}
	}
}

// pruneOfflineTrackers reconciles the offline-tracker map after a tick (D-157/D-159).
// Three cases per existing tracker:
//   - keep:    the rule is an actively-evaluated wildcard offline rule. evalStreamOffline
//     already maintained its state this tick; nothing to do.
//   - suspend: the rule still EXISTS and is still wildcard-offline-shaped but is
//     currently suspended (disabled or inside a maintenance window). Reset
//     prevPresent so a stale present set cannot fabricate a present→gone edge
//     on resume (the original D-157 spurious-fire concern), but PRESERVE
//     offlineAt/holdUntil so an offline event already IN FLIGHT when the rule
//     was suspended can still satisfy WindowS and fire — or auto-resolve at its
//     hold — once the rule resumes. Without this, a brief disable / maintenance
//     window between the offline edge and WindowS silently swallowed the page
//     (missed-fire), and a disable AFTER firing left the alert stuck "firing"
//     forever because the fresh empty tracker produced no result to resolve it.
//   - discard: the rule is gone, or is no longer wildcard-offline (metric/scope changed).
//     Drop the whole tracker; a stale offlineAt must not resurrect as a
//     spurious fire if the rule later becomes wildcard-offline again.
func (e *Evaluator) pruneOfflineTrackers(keep, suspend map[string]bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for ruleID, tr := range e.offlineTrackers {
		switch {
		case keep[ruleID]:
			// active — full state already maintained by evalStreamOffline.
		case suspend[ruleID]:
			tr.prevPresent = map[string]bool{}
		default:
			delete(e.offlineTrackers, ruleID)
		}
	}
}

// isWildcardOfflineScope reports whether a rule's scope has no stream_id — the
// wildcard form that uses cross-tick edge detection. A malformed scope parses to
// the zero value (empty StreamID) → wildcard, matching evaluateRule's tolerant parse.
func isWildcardOfflineScope(scopeJSON string) bool {
	var scope domain.AlertScope
	_ = json.Unmarshal([]byte(scopeJSON), &scope)
	return scope.StreamID == ""
}

// syncRegistryFromStore updates the channel registry from the meta store.
// Called at the start of every evaluation tick (sync-on-tick pattern).
//
// Design notes:
//   - Incremental sync: only channels previously synced from the store are
//     removed when they disappear. Channels manually registered before the first
//     sync (e.g. test fakes injected via reg.Register) are never removed.
//   - Decrypt failure or unknown type: logged + skipped; other channels still delivered.
//   - Channels removed from the store are removed from the registry on the next tick,
//     causing deliver() to skip them (no delivery_failure row for deleted channels).
func (e *Evaluator) syncRegistryFromStore(ctx context.Context) {
	storedChannels, err := e.store.ListAlertChannels(ctx, 0, "")
	if err != nil {
		e.logger.Warn("alert evaluator: list channels failed — registry not updated", "error", err)
		return
	}

	if e.syncedChannelIDs == nil {
		e.syncedChannelIDs = make(map[string]bool)
	}

	// Build the desired set from the store.
	desiredIDs := make(map[string]bool, len(storedChannels))
	for i := range storedChannels {
		row := &storedChannels[i]
		desiredIDs[row.ID] = true
		built, err := BuildChannelFromRow(e.store, row)
		if err != nil {
			e.logger.Warn("alert evaluator: skip channel (build error)",
				"channel_id", row.ID, "type", row.Type, "error", err)
			continue
		}
		e.registry.Register(row.ID, built)
	}

	// Remove channels that were previously synced from the store but are now gone.
	// We only remove IDs we previously added — never touching manually-registered ones.
	for id := range e.syncedChannelIDs {
		if !desiredIDs[id] {
			e.registry.Remove(id)
		}
	}

	e.syncedChannelIDs = desiredIDs
}

func (e *Evaluator) evaluateRule(ctx context.Context, rule meta.AlertRuleRow, snap *domain.LiveSnapshot, now time.Time) {
	// Parse scope from JSON.
	var scope domain.AlertScope
	_ = json.Unmarshal([]byte(rule.ScopeJSON), &scope)

	var evals []evalResult

	// S11 WO-B: anomaly rules dispatch to evalAnomalyMetric before the threshold switch.
	// RuleType=="" or RuleType=="threshold" falls through to the existing switch (backward compat).
	if rule.RuleType == "anomaly" {
		evals = e.evalAnomalyMetric(ctx, snap, scope, rule)
	} else {
		switch rule.Metric {
		case "stream_offline":
			evals = e.evalStreamOffline(snap, scope, rule, now)
		case "viewer_drop_pct":
			evals = e.evalViewerDrop(snap, scope, rule)
		case "node_cpu":
			evals = e.evalNodeMetric(snap, scope, rule, "cpu_pct")
		case "node_mem":
			evals = e.evalNodeMetric(snap, scope, rule, "mem_pct")
		case "node_disk":
			evals = e.evalNodeMetric(snap, scope, rule, "disk_pct")
		// Wave 2: new metric types.
		case "rebuffer_ratio", "error_rate", "ingest_bitrate_floor":
			evals = e.evalQoEMetric(ctx, snap, scope, rule)
		case "node_down", "node_degraded":
			evals = e.evalNodeUpDown(snap, scope, rule)
		case "cert_expiry":
			// Cert expiry uses a real TLS checker in production; nil checker = skip.
			if e.certChecker != nil {
				evals = e.evalCertExpiry(ctx, rule, scope, e.certChecker)
			}
		case "license_expiry":
			// S39: warn before the licence key expires. nil checker = skip.
			if e.licenseChecker != nil {
				evals = e.evalLicenseExpiry(rule, scope, e.licenseChecker)
			}
		default:
			evals = e.evalGenericMetric(snap, scope, rule)
		}
	}

	// VD-29: Apply group_by storm grouping.
	// When group_by is set, collapse per-stream evals into one per group key.
	// e.g. group_by="app" → one notification per app, not per stream.
	if rule.GroupBy.Valid && rule.GroupBy.String != "" {
		// D-157: wildcard stream_offline is per-stream edge detection. Its offline
		// results are for streams ABSENT from snap.Streams, which applyGroupBy cannot
		// resolve to an app (group key falls back to stream_id) — and when such a
		// stream RECOVERS, applyGroupBy would re-key it to its app, orphaning the
		// stream-id-keyed firing state so it never resolves (permanent stuck-fire).
		// So group_by does not collapse wildcard offline; it stays one alert per stream.
		if rule.Metric != "stream_offline" || scope.StreamID != "" {
			evals = applyGroupBy(evals, rule.GroupBy.String, snap)
		}
	}

	for _, ev := range evals {
		e.processEvaluation(ctx, rule, scope, ev, now)
	}
}

// processEvaluation advances the state machine for one (rule, evalResult) pair.
// The evalResult carries groupKey, value, ok, and (for anomaly rules) anomalyInfo.
func (e *Evaluator) processEvaluation(ctx context.Context, rule meta.AlertRuleRow, scope domain.AlertScope,
	ev evalResult, now time.Time) {
	groupKey := ev.groupKey
	value := ev.value
	conditionMet := ev.ok
	key := rule.ID + ":" + groupKey

	e.mu.Lock()
	st := e.states[key]
	if st == nil {
		st = &ruleState{ruleID: rule.ID, groupKey: groupKey, state: "pending"}
		e.states[key] = st
	}
	// D-160: stamp the tick this (rule, group) was last evaluated. pruneStaleStates
	// uses it to evict entries whose stream/node has vanished (no evalResult this tick)
	// once they are behaviorally inert — bounding e.states, which otherwise grows one
	// permanent entry per unique (rule, stream_id) ever seen.
	st.lastCheck = now
	e.mu.Unlock()

	switch st.state {
	case "pending", "resolved":
		if conditionMet {
			if st.pendingSince.IsZero() {
				st.pendingSince = now
			}
			// S11 WO-B: anomaly rules fire immediately on detection — window_s=3600
			// is the Welford baseline lookback, NOT a "condition must hold" duration.
			// Threshold rules require the condition to hold for WindowS seconds.
			windowElapsed := rule.RuleType == "anomaly" || now.Sub(st.pendingSince) >= time.Duration(rule.WindowS)*time.Second
			if windowElapsed {
				// Check cooldown.
				if !st.cooldownUntil.IsZero() && now.Before(st.cooldownUntil) {
					return // suppressed by cooldown
				}
				// Transition to firing.
				st.state = "firing"
				st.alertID = uuid.New().String()
				st.firedAt = now
				cooldownUntil := now.Add(time.Duration(rule.CooldownS) * time.Second)
				st.cooldownUntil = cooldownUntil
				e.fire(ctx, rule, scope, groupKey, value, st.alertID, now, &cooldownUntil, ev.anomalyInfo)
			}
		} else {
			st.pendingSince = time.Time{}
		}

	case "firing":
		if !conditionMet {
			// Resolved.
			st.state = "resolved"
			st.pendingSince = time.Time{}
			e.resolve(ctx, rule, scope, groupKey, value, st.alertID, st.firedAt, now)
		}
		// If still firing and cooldown expired, allow re-fire on next cycle.
		// (No re-fire needed while already in firing state — prevents storm.)
	}
}

// fire sends a firing notification and persists history.
// anomalyInfo is non-nil for anomaly rules; it sets the notification threshold to
// the baseline mean and adds expected/sigma_multiplier fields to the payload.
func (e *Evaluator) fire(ctx context.Context, rule meta.AlertRuleRow, scope domain.AlertScope,
	groupKey string, value float64, alertID string, now time.Time, cooldownUntil *time.Time, anomalyInfo *anomalyEvalInfo) {
	// VD-28: muted=true means evaluated but notifications suppressed.
	if rule.Muted {
		return
	}

	var cooldownMS *int64
	if cooldownUntil != nil {
		ms := cooldownUntil.UnixMilli()
		cooldownMS = &ms
	}

	// S11 WO-B: for anomaly rules, use baseline mean as the "threshold" in the notification
	// so webhook recipients see a meaningful expected-vs-actual comparison.
	notifThreshold := rule.Threshold
	if anomalyInfo != nil {
		notifThreshold = anomalyInfo.Expected
	}

	notif := buildNotification(rule, scope, groupKey, "firing", value, alertID, now, cooldownMS, nil, false, notifThreshold, anomalyInfo)
	payload, err := json.Marshal(notif)
	if err != nil {
		e.logger.Error("alert: marshal notification", "error", err)
		return
	}

	// Persist history.
	if e.store != nil {
		histRow := meta.AlertHistoryRow{
			AlertID:       alertID,
			RuleID:        rule.ID,
			State:         "firing",
			Severity:      rule.Severity,
			TS:            now.UnixMilli(),
			Metric:        rule.Metric,
			Value:         value,
			Threshold:     notifThreshold, // baseline mean for anomaly rules
			ScopeJSON:     rule.ScopeJSON,
			CooldownUntil: cooldownMS,
		}
		histRow.GroupKey = nullString(groupKey)
		if err := e.store.CreateAlertHistory(ctx, histRow); err != nil {
			e.logger.Warn("alert: persist history (fire)", "error", err)
		}
	}

	dc := deliveryCtx{
		alertID:   alertID,
		ruleID:    rule.ID,
		severity:  rule.Severity,
		metric:    rule.Metric,
		value:     value,
		threshold: rule.Threshold,
		scopeJSON: rule.ScopeJSON,
	}
	e.deliver(ctx, rule, payload, dc)
}

// resolve sends a resolved notification and persists history.
func (e *Evaluator) resolve(ctx context.Context, rule meta.AlertRuleRow, scope domain.AlertScope,
	groupKey string, value float64, alertID string, firedAt, now time.Time) {
	// VD-28: muted=true suppresses resolve notifications too.
	if rule.Muted {
		return
	}

	resolvedAt := now.UnixMilli()
	notif := buildNotification(rule, scope, groupKey, "resolved", value, alertID, firedAt, nil, &resolvedAt, false, rule.Threshold, nil)
	payload, err := json.Marshal(notif)
	if err != nil {
		return
	}

	if e.store != nil {
		histRow := meta.AlertHistoryRow{
			AlertID:   alertID,
			RuleID:    rule.ID,
			State:     "resolved",
			Severity:  rule.Severity,
			TS:        now.UnixMilli(),
			Metric:    rule.Metric,
			Value:     value,
			Threshold: rule.Threshold,
			ScopeJSON: rule.ScopeJSON,
			GroupKey:  nullString(groupKey),
		}
		if err := e.store.CreateAlertHistory(ctx, histRow); err != nil {
			e.logger.Warn("alert: persist history (resolve)", "error", err)
		}
	}

	dc := deliveryCtx{
		alertID:   alertID,
		ruleID:    rule.ID,
		severity:  rule.Severity,
		metric:    rule.Metric,
		value:     value,
		threshold: rule.Threshold,
		scopeJSON: rule.ScopeJSON,
	}
	e.deliver(ctx, rule, payload, dc)
}

// deliver sends payload to all channels configured for the rule.
//
// The notifySink (test hook) is called synchronously before channel fanout so
// that existing tests that inspect the sink do not observe ordering issues.
//
// Each channel is delivered in its own goroutine so that a slow or failing
// channel never blocks the 5-second evaluate tick.  Goroutines are counted in
// deliveryWg so Stop() can wait for a bounded shutdown.
func (e *Evaluator) deliver(ctx context.Context, rule meta.AlertRuleRow, payload []byte, dc deliveryCtx) {
	// Call notify sink synchronously (kept for backward compatibility with tests).
	e.mu.Lock()
	sink := e.notifySink
	e.mu.Unlock()
	if sink != nil {
		sink(payload)
	}

	// Deliver to registered channels — one goroutine per channel.
	var channelIDs []string
	_ = json.Unmarshal([]byte(rule.ChannelIDs), &channelIDs)
	for _, id := range channelIDs {
		ch, ok := e.registry.Get(id)
		if !ok {
			continue
		}
		e.deliveryWg.Add(1)
		go func(channelID string, ch channels.Channel) {
			defer e.deliveryWg.Done()
			e.retryDeliver(ctx, channelID, ch, payload, dc)
		}(id, ch)
	}
}

// retryDeliver attempts to send payload to one channel with exponential backoff
// and +/-20% jitter.  On total failure (all 1+RetryMaxAttempts attempts
// exhausted), it writes a delivery_failure alert_history row.
// Backoff sleeps abort immediately when ctx is cancelled so shutdown is bounded.
func (e *Evaluator) retryDeliver(ctx context.Context, channelID string, ch channels.Channel, payload []byte, dc deliveryCtx) {
	var lastErr error
	for attempt := 0; attempt <= e.cfg.RetryMaxAttempts; attempt++ {
		if attempt > 0 {
			// delay[n] = min(base * 2^(n-1), cap) * jitter   jitter∈[0.8,1.2)
			rawDelay := time.Duration(float64(e.cfg.RetryBaseDelay) * math.Pow(2, float64(attempt-1)))
			if rawDelay > e.cfg.RetryCap {
				rawDelay = e.cfg.RetryCap
			}
			jitter := 0.8 + 0.4*rand.Float64()
			delay := time.Duration(float64(rawDelay) * jitter)

			select {
			case <-ctx.Done():
				return // abort: evaluator is shutting down
			case <-time.After(delay):
			}
		}

		if err := ch.Send(ctx, payload); err != nil {
			lastErr = err
			e.logger.Warn("alert: channel send failed",
				"channel_id", channelID,
				"attempt", attempt+1,
				"of", e.cfg.RetryMaxAttempts+1,
				"error", err)
		} else {
			return // delivered successfully
		}
	}

	// All attempts exhausted — record the failure so operators can audit it.
	if e.store != nil {
		e.recordDeliveryFailure(ctx, channelID, lastErr, dc)
	}
}

// recordDeliveryFailure inserts a delivery_failure alert_history row.
// The scope JSON from the original firing is merged with channel_id and a
// sanitised error string so operators can correlate the failure.
func (e *Evaluator) recordDeliveryFailure(ctx context.Context, channelID string, lastErr error, dc deliveryCtx) {
	scopeJSON := mergeScopeWithFailure(dc.scopeJSON, channelID, lastErr)
	row := meta.AlertHistoryRow{
		AlertID:   dc.alertID,
		RuleID:    dc.ruleID,
		State:     "delivery_failure",
		Severity:  dc.severity,
		TS:        time.Now().UnixMilli(),
		Metric:    dc.metric,
		Value:     dc.value,
		Threshold: dc.threshold,
		ScopeJSON: scopeJSON,
	}
	if err := e.store.CreateAlertHistory(ctx, row); err != nil {
		e.logger.Warn("alert: persist delivery_failure history", "error", err)
	}
}

// urlPattern matches http/https URLs to strip from error messages.
var urlPattern = regexp.MustCompile(`https?://\S+`)

// sanitizeError returns a safe error string with embedded URLs redacted so that
// channel webhook tokens / credentials are not stored in alert_history.
func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	return urlPattern.ReplaceAllString(err.Error(), "[REDACTED]")
}

// mergeScopeWithFailure merges {"channel_id":…, "error":…} into the existing
// scope JSON object, preserving any existing fields (stream_id, app, …).
func mergeScopeWithFailure(existingScopeJSON, channelID string, lastErr error) string {
	m := make(map[string]any)
	_ = json.Unmarshal([]byte(existingScopeJSON), &m)
	m["channel_id"] = channelID
	if lastErr != nil {
		m["error"] = sanitizeError(lastErr)
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// ─── Metric evaluators ────────────────────────────────────────────────────────

type evalResult struct {
	groupKey string
	value    float64
	ok       bool
	// S11 WO-B: anomaly eval context. Non-nil only for anomaly rule results.
	// Passed through processEvaluation → fire() → buildNotification.
	anomalyInfo *anomalyEvalInfo
}

// evalStreamOffline yields a binary "offline" metric per stream: value 1.0 when the
// stream is offline, 0.0 when online, evaluated against the rule operator/threshold
// via compare (D-129). The default rule is { operator: "eq", threshold: 1 }, so it
// fires exactly when offline. Previously this hardcoded value=0 and ok=!active, which
// (a) reported a misleading value=0 on a firing alert and (b) ignored the rule's
// operator/threshold entirely. Offline detection: scoped = absent from the snapshot
// (sticky); wildcard = a present→gone edge across ticks (D-157 — an inactive stream
// is no longer present in the snapshot, so the old "present-but-inactive" wildcard
// test could never match in production).
func (e *Evaluator) evalStreamOffline(snap *domain.LiveSnapshot, scope domain.AlertScope, rule meta.AlertRuleRow, now time.Time) []evalResult {
	// Scoped: a specific stream is offline iff absent from the snapshot. Sticky —
	// fires (value 1.0) every tick while absent and resolves when it returns.
	if scope.StreamID != "" {
		val := 0.0
		if _, present := snap.Streams[scope.StreamID]; !present {
			val = 1.0 // offline: the scoped stream is absent from the snapshot
		}
		return []evalResult{{groupKey: scope.StreamID, value: val, ok: compare(val, rule.Operator, rule.Threshold)}}
	}

	// Wildcard (D-157): an inactive stream is removed from snap.Streams entirely
	// (aggregator.onPublishEnd calls snapRemoveStream while Active==true, then
	// deletes it), so offline cannot be read from a single snapshot. Detect the
	// present→gone EDGE by diffing the scope-matching present set across ticks: a
	// stream present last tick and gone now has just ended. It emits value 1.0 for
	// a bounded hold window — long enough to satisfy the rule's WindowS and fire —
	// then one resolving 0.0 (the state machine has no stale-sweep, so a fired
	// alert would otherwise stick firing forever). A returning stream also resolves.
	//
	// By-design (differs from the scoped path, which is a sticky level condition):
	// wildcard offline is an EDGE — it pages ONCE per offline event and auto-clears
	// after the hold window. A stream that ends and never returns yields a single
	// alert, not a perpetually-firing one.
	//
	// Known limitation: snap.Streams is keyed by BARE stream_id (last-write-wins when
	// two active streams share an id across apps/nodes — see aggregator.snapAddStream).
	// A partially-scoped wildcard rule (scope.App/NodeID set, StreamID empty) therefore
	// inherits that aliasing: a stream shadowed in the snapshot can read as gone. The
	// full-wildcard form (empty scope — the default rule) and the scoped form are
	// unaffected. A precise fix needs compound-keyed snapshots (out of scope here).
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.offlineTrackers == nil {
		e.offlineTrackers = make(map[string]*offlineTracker)
	}
	tr := e.offlineTrackers[rule.ID]
	if tr == nil {
		tr = &offlineTracker{
			prevPresent: map[string]bool{},
			offlineAt:   map[string]time.Time{},
			holdUntil:   map[string]time.Time{},
		}
		e.offlineTrackers[rule.ID] = tr
	}

	// Scope-matching currently-present streams.
	present := make(map[string]bool, len(snap.Streams))
	for sid, s := range snap.Streams {
		if scope.App != "" && s.App != scope.App {
			continue
		}
		if scope.NodeID != "" && s.NodeID != scope.NodeID {
			continue
		}
		present[sid] = true
	}

	// New offline edges: present last tick, gone now (and not already tracked). The
	// hold deadline is frozen HERE, at detection, from the WindowS in effect now — so
	// a later WindowS edit cannot retroactively expire an in-flight offline event
	// (D-159).
	for sid := range tr.prevPresent {
		if !present[sid] {
			if _, tracked := tr.offlineAt[sid]; !tracked {
				tr.offlineAt[sid] = now
				tr.holdUntil[sid] = now.Add(streamOfflineHold(rule.WindowS, e.cfg.TickInterval))
			}
		}
	}

	results := make([]evalResult, 0, len(present)+len(tr.offlineAt))
	// Present streams are online (value 0.0). A returning stream also clears any
	// pending offline tracking, so its firing alert resolves.
	for sid := range present {
		delete(tr.offlineAt, sid)
		delete(tr.holdUntil, sid)
		results = append(results, evalResult{groupKey: sid, value: 0.0, ok: compare(0.0, rule.Operator, rule.Threshold)})
	}
	// Recently-offline streams: value 1.0 until the absolute hold deadline (fires),
	// then one resolving 0.0 and stop tracking.
	for sid := range tr.offlineAt {
		if now.Before(tr.holdUntil[sid]) {
			results = append(results, evalResult{groupKey: sid, value: 1.0, ok: compare(1.0, rule.Operator, rule.Threshold)})
		} else {
			results = append(results, evalResult{groupKey: sid, value: 0.0, ok: compare(0.0, rule.Operator, rule.Threshold)})
			delete(tr.offlineAt, sid)
			delete(tr.holdUntil, sid)
		}
	}

	tr.prevPresent = present
	return results
}

// streamOfflineHold is how long a wildcard-detected offline stream keeps emitting
// value 1.0 before it auto-resolves (D-157). It must exceed the rule's WindowS so
// the alert can satisfy its hold-to-fire requirement and actually fire; the extra
// grace keeps it visibly firing briefly, then it emits a resolving 0.0. Floored at
// two ticks so a WindowS=0 (fire-immediately) rule still fires then resolves.
func streamOfflineHold(windowS int, tick time.Duration) time.Duration {
	if tick <= 0 {
		tick = 5 * time.Second
	}
	w := time.Duration(windowS) * time.Second
	grace := w
	if grace < 2*tick {
		grace = 2 * tick
	}
	return w + grace
}

func (e *Evaluator) evalViewerDrop(snap *domain.LiveSnapshot, scope domain.AlertScope, rule meta.AlertRuleRow) []evalResult {
	var results []evalResult
	if scope.StreamID != "" {
		if s, ok := snap.Streams[scope.StreamID]; ok {
			results = append(results, evalResult{
				groupKey: scope.StreamID,
				value:    float64(s.ViewerCount),
				ok:       compare(float64(s.ViewerCount), rule.Operator, rule.Threshold),
			})
		}
		return results
	}
	for sid, s := range snap.Streams {
		if scope.App != "" && s.App != scope.App {
			continue
		}
		results = append(results, evalResult{
			groupKey: sid,
			value:    float64(s.ViewerCount),
			ok:       compare(float64(s.ViewerCount), rule.Operator, rule.Threshold),
		})
	}
	return results
}

func (e *Evaluator) evalNodeMetric(snap *domain.LiveSnapshot, scope domain.AlertScope, rule meta.AlertRuleRow, field string) []evalResult {
	var results []evalResult
	for nodeID, n := range snap.Nodes {
		if scope.NodeID != "" && nodeID != scope.NodeID {
			continue
		}
		var val float64
		var reported bool
		switch field {
		case "cpu_pct":
			val, reported = n.CPUPCT, n.CPUPCTReported
		case "mem_pct":
			val, reported = n.MemPCT, n.MemPCTReported
		case "disk_pct":
			val, reported = n.DiskPCT, n.DiskPCTReported
		}
		// D-088 / D-129 presence guard: a node that did not report this field
		// (standalone AMS 3.x omits cpu/mem/disk keys) leaves val at 0. Comparing
		// that phantom 0 to the threshold false-fires lt-rules. But we must NOT just
		// skip the node: processEvaluation has no stale-state sweep, so a previously-
		// firing alert would stick forever if a node stops reporting the field (e.g.
		// an AMS 5.x→3.x downgrade — D-129 review). Emit ok=false instead: never fires
		// on missing data, and lets a firing alert resolve. (evalAnomalyNodes uses a
		// bare `continue` — anomaly rules need a baseline+samples, so skipping is right
		// there; the threshold path can safely treat "unreported" as "not firing".)
		if !reported {
			results = append(results, evalResult{groupKey: nodeID, value: 0, ok: false})
			continue
		}
		results = append(results, evalResult{groupKey: nodeID, value: val, ok: compare(val, rule.Operator, rule.Threshold)})
	}
	return results
}

// applyGroupBy collapses per-stream evals into one per group key value.
// For group_by="app", the group key is the stream's App field.
// For group_by="stream_id" (or anything else), each stream stays independent
// but the groupKey is forced to the stream's stream_id (the default).
// The collapsed eval is conditionMet=true if ANY member stream fires,
// and value = max(values) to represent the worst member.
func applyGroupBy(evals []evalResult, groupByDim string, snap *domain.LiveSnapshot) []evalResult {
	if len(evals) == 0 {
		return evals
	}
	// Build group key → best (worst-value) eval.
	type grouped struct {
		ok    bool
		value float64
	}
	groups := make(map[string]grouped)
	for _, ev := range evals {
		gk := ev.groupKey // default: stream_id dimension
		if groupByDim == "app" {
			// Resolve app name from snapshot.
			if s, ok := snap.Streams[ev.groupKey]; ok {
				gk = s.App
			}
		}
		if gk == "" {
			gk = ev.groupKey // fallback
		}
		prev, exists := groups[gk]
		if !exists {
			groups[gk] = grouped{ok: ev.ok, value: ev.value}
		} else {
			// Condition fires if any member fires; use max value.
			combined := grouped{
				ok:    prev.ok || ev.ok,
				value: maxFloat(prev.value, ev.value),
			}
			groups[gk] = combined
		}
	}
	result := make([]evalResult, 0, len(groups))
	for gk, g := range groups {
		result = append(result, evalResult{groupKey: gk, value: g.value, ok: g.ok})
	}
	return result
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func (e *Evaluator) evalGenericMetric(snap *domain.LiveSnapshot, scope domain.AlertScope, rule meta.AlertRuleRow) []evalResult {
	var results []evalResult
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
		case "viewer_count":
			val = float64(s.ViewerCount)
		case "ingest_bitrate_kbps":
			val = s.IngestBitrate
		case "fps":
			val = s.FPS
		default:
			continue
		}
		results = append(results, evalResult{groupKey: sid, value: val, ok: compare(val, rule.Operator, rule.Threshold)})
	}
	return results
}

// inMaintenanceWindow returns true if now falls within any maintenance window.
// Wave 2: delegates to the cron-based implementation in wave2.go (closes G2).
func (e *Evaluator) inMaintenanceWindow(rule meta.AlertRuleRow, now time.Time) bool {
	return inMaintenanceWindowCron(rule, now)
}

// compare applies a comparison operator.
func compare(value float64, operator string, threshold float64) bool {
	switch operator {
	case "gt":
		return value > threshold
	case "lt":
		return value < threshold
	case "gte":
		return value >= threshold
	case "lte":
		return value <= threshold
	case "eq":
		return value == threshold
	default:
		return false
	}
}

// ─── Notification builder ─────────────────────────────────────────────────────

// buildNotification constructs the alert notification payload.
// notifThreshold is used as the "threshold" field (baseline mean for anomaly rules,
// rule.Threshold for threshold rules). anomalyInfo adds optional anomaly fields.
func buildNotification(rule meta.AlertRuleRow, scope domain.AlertScope, groupKey, state string,
	value float64, alertID string, firedAt time.Time, cooldownUntil, resolvedAt *int64, isTest bool,
	notifThreshold float64, anomalyInfo *anomalyEvalInfo) map[string]any {
	n := map[string]any{
		"version":   1,
		"alert_id":  alertID,
		"rule_id":   rule.ID,
		"state":     state,
		"severity":  rule.Severity,
		"ts":        firedAt.UnixMilli(),
		"title":     buildTitle(rule, scope, state),
		"metric":    rule.Metric,
		"value":     value,
		"threshold": notifThreshold,
		"scope": map[string]any{
			"node_id":   scope.NodeID,
			"app":       scope.App,
			"stream_id": scope.StreamID,
		},
		"test": isTest,
	}
	// S11 WO-B: anomaly rules add expected and sigma_multiplier fields.
	if anomalyInfo != nil {
		n["expected"] = anomalyInfo.Expected
		n["sigma_multiplier"] = anomalyInfo.SigmaMultiplier
	}
	if cooldownUntil != nil {
		n["cooldown_until"] = *cooldownUntil
	} else {
		n["cooldown_until"] = nil
	}
	if groupKey != "" {
		n["group_key"] = groupKey
	} else {
		n["group_key"] = nil
	}
	if resolvedAt != nil {
		n["resolved_at"] = *resolvedAt
	}
	return n
}

func buildTitle(rule meta.AlertRuleRow, scope domain.AlertScope, state string) string {
	target := rule.Metric
	if scope.StreamID != "" {
		target = fmt.Sprintf("stream %s / %s", scope.StreamID, rule.Metric)
	} else if scope.App != "" {
		target = fmt.Sprintf("app %s / %s", scope.App, rule.Metric)
	} else if scope.NodeID != "" {
		target = fmt.Sprintf("node %s / %s", scope.NodeID, rule.Metric)
	}
	switch state {
	case "firing":
		return fmt.Sprintf("FIRING: %s %s %g", target, rule.Operator, rule.Threshold)
	case "resolved":
		return fmt.Sprintf("RESOLVED: %s", target)
	default:
		return fmt.Sprintf("[%s] %s", state, target)
	}
}

// TestFireChannel sends a test notification to a single channel.
func TestFireChannel(ctx context.Context, ch channels.Channel, ruleID, baseURL string) error {
	alertID := uuid.New().String()
	now := time.Now()
	n := map[string]any{
		"version":        1,
		"alert_id":       alertID,
		"rule_id":        ruleID,
		"state":          "firing",
		"severity":       "info",
		"ts":             now.UnixMilli(),
		"title":          "Pulse test notification",
		"metric":         "test_fire",
		"value":          0.0,
		"threshold":      0.0,
		"scope":          map[string]any{},
		"test":           true,
		"cooldown_until": nil,
		"group_key":      nil,
	}
	if baseURL != "" {
		n["dashboard_url"] = baseURL + "/alerts"
	}
	payload, err := json.Marshal(n)
	if err != nil {
		return err
	}
	return ch.Send(ctx, payload)
}

func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
