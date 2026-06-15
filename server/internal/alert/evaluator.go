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
	ruleID     string
	groupKey   string
	alertID    string // current firing instance ID
	state      string // "pending" | "firing" | "resolved"
	firedAt    time.Time
	lastCheck  time.Time
	cooldownUntil time.Time
	pendingSince  time.Time
}

// ─── Evaluator ────────────────────────────────────────────────────────────────

// Config holds evaluator configuration.
type Config struct {
	// TickInterval is the evaluation loop interval (default 5s, max 30s).
	// Detection→notification is bounded by tick + one poll interval.
	TickInterval time.Duration

	// BaseURL is used to build dashboard_url in notifications.
	BaseURL string
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

	// Wave 2: TLS cert expiry checker (nil = cert_expiry rules skipped).
	certChecker CertExpiryChecker

	// Notification sink for tests.
	notifySink func([]byte)
}

// New creates an Evaluator. If clock is nil, RealClock is used.
// If logger is nil, a discard logger is used (useful for tests).
func New(cfg Config, live domain.LiveProvider, store *meta.Store, registry *channels.Registry, clock Clock, logger *slog.Logger) *Evaluator {
	if cfg.TickInterval <= 0 || cfg.TickInterval > 30*time.Second {
		cfg.TickInterval = 5 * time.Second
	}
	if clock == nil {
		clock = RealClock{}
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Evaluator{
		cfg:      cfg,
		store:    store,
		live:     live,
		registry: registry,
		clock:    clock,
		logger:   logger,
		states:   make(map[string]*ruleState),
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

// Start runs the evaluator loop until ctx is cancelled.
func (e *Evaluator) Start(ctx context.Context) {
	go e.loop(ctx)
}

// Stop is a no-op (context cancellation stops the loop).
func (e *Evaluator) Stop() {}

// TickOnce runs a single evaluation cycle synchronously (for tests).
func (e *Evaluator) TickOnce(ctx context.Context) {
	e.evaluate(ctx)
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
	rules, err := e.store.ListAlertRules(ctx)
	if err != nil {
		e.logger.Warn("alert evaluator: list rules failed", "error", err)
		return
	}

	snap := e.live.CurrentSnapshot()
	if snap == nil {
		return
	}

	now := e.clock.Now()

	for _, rule := range rules {
		// enabled=false: rule is completely suspended — not evaluated at all.
		// This is distinct from muted=true (evaluated, but notifications suppressed).
		if !rule.Enabled {
			continue
		}
		// Maintenance window suppression.
		if e.inMaintenanceWindow(rule, now) {
			continue
		}

		e.evaluateRule(ctx, rule, snap, now)
	}
}

func (e *Evaluator) evaluateRule(ctx context.Context, rule meta.AlertRuleRow, snap *domain.LiveSnapshot, now time.Time) {
	// Parse scope from JSON.
	var scope domain.AlertScope
	_ = json.Unmarshal([]byte(rule.ScopeJSON), &scope)

	var evals []evalResult

	switch rule.Metric {
	case "stream_offline":
		evals = e.evalStreamOffline(snap, scope, rule)
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
		evals = e.evalQoEMetric(snap, scope, rule)
	case "node_down", "node_degraded":
		evals = e.evalNodeUpDown(snap, scope, rule)
	case "cert_expiry":
		// Cert expiry uses a real TLS checker in production; nil checker = skip.
		if e.certChecker != nil {
			evals = e.evalCertExpiry(ctx, rule, scope, e.certChecker)
		}
	default:
		evals = e.evalGenericMetric(snap, scope, rule)
	}

	// VD-29: Apply group_by storm grouping.
	// When group_by is set, collapse per-stream evals into one per group key.
	// e.g. group_by="app" → one notification per app, not per stream.
	if rule.GroupBy.Valid && rule.GroupBy.String != "" {
		evals = applyGroupBy(evals, rule.GroupBy.String, snap)
	}

	for _, ev := range evals {
		e.processEvaluation(ctx, rule, scope, ev.groupKey, ev.value, ev.ok, now)
	}
}

// processEvaluation advances the state machine for one (rule, groupKey) pair.
func (e *Evaluator) processEvaluation(ctx context.Context, rule meta.AlertRuleRow, scope domain.AlertScope,
	groupKey string, value float64, conditionMet bool, now time.Time) {
	key := rule.ID + ":" + groupKey

	e.mu.Lock()
	st := e.states[key]
	if st == nil {
		st = &ruleState{ruleID: rule.ID, groupKey: groupKey, state: "pending"}
		e.states[key] = st
	}
	e.mu.Unlock()

	switch st.state {
	case "pending", "resolved":
		if conditionMet {
			if st.pendingSince.IsZero() {
				st.pendingSince = now
			}
			windowElapsed := now.Sub(st.pendingSince) >= time.Duration(rule.WindowS)*time.Second
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
				e.fire(ctx, rule, scope, groupKey, value, st.alertID, now, &cooldownUntil)
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
func (e *Evaluator) fire(ctx context.Context, rule meta.AlertRuleRow, scope domain.AlertScope,
	groupKey string, value float64, alertID string, now time.Time, cooldownUntil *time.Time) {
	// VD-28: muted=true means evaluated but notifications suppressed.
	if rule.Muted {
		return
	}

	var cooldownMS *int64
	if cooldownUntil != nil {
		ms := cooldownUntil.UnixMilli()
		cooldownMS = &ms
	}

	notif := buildNotification(rule, scope, groupKey, "firing", value, alertID, now, cooldownMS, nil, false)
	payload, err := json.Marshal(notif)
	if err != nil {
		e.logger.Error("alert: marshal notification", "error", err)
		return
	}

	// Persist history.
	if e.store != nil {
		histRow := meta.AlertHistoryRow{
			AlertID:    alertID,
			RuleID:     rule.ID,
			State:      "firing",
			Severity:   rule.Severity,
			TS:         now.UnixMilli(),
			Metric:     rule.Metric,
			Value:      value,
			Threshold:  rule.Threshold,
			ScopeJSON:  rule.ScopeJSON,
			CooldownUntil: cooldownMS,
		}
		histRow.GroupKey = nullString(groupKey)
		if err := e.store.CreateAlertHistory(ctx, histRow); err != nil {
			e.logger.Warn("alert: persist history (fire)", "error", err)
		}
	}

	e.deliver(ctx, rule, payload)
}

// resolve sends a resolved notification and persists history.
func (e *Evaluator) resolve(ctx context.Context, rule meta.AlertRuleRow, scope domain.AlertScope,
	groupKey string, value float64, alertID string, firedAt, now time.Time) {
	// VD-28: muted=true suppresses resolve notifications too.
	if rule.Muted {
		return
	}

	resolvedAt := now.UnixMilli()
	notif := buildNotification(rule, scope, groupKey, "resolved", value, alertID, firedAt, nil, &resolvedAt, false)
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

	e.deliver(ctx, rule, payload)
}

// deliver sends payload to all channels configured for the rule.
func (e *Evaluator) deliver(ctx context.Context, rule meta.AlertRuleRow, payload []byte) {
	// Call notify sink if set (for tests).
	e.mu.Lock()
	sink := e.notifySink
	e.mu.Unlock()
	if sink != nil {
		sink(payload)
	}

	// Deliver to registered channels.
	var channelIDs []string
	_ = json.Unmarshal([]byte(rule.ChannelIDs), &channelIDs)
	for _, id := range channelIDs {
		if ch, ok := e.registry.Get(id); ok {
			if err := ch.Send(ctx, payload); err != nil {
				e.logger.Warn("alert: channel send failed", "channel_id", id, "error", err)
			}
		}
	}
}

// ─── Metric evaluators ────────────────────────────────────────────────────────

type evalResult struct {
	groupKey string
	value    float64
	ok       bool
}

func (e *Evaluator) evalStreamOffline(snap *domain.LiveSnapshot, scope domain.AlertScope, rule meta.AlertRuleRow) []evalResult {
	var results []evalResult
	if scope.StreamID != "" {
		_, active := snap.Streams[scope.StreamID]
		results = append(results, evalResult{groupKey: scope.StreamID, value: 0, ok: !active})
		return results
	}
	for sid, s := range snap.Streams {
		if scope.App != "" && s.App != scope.App {
			continue
		}
		if scope.NodeID != "" && s.NodeID != scope.NodeID {
			continue
		}
		results = append(results, evalResult{groupKey: sid, value: 0, ok: !s.Active})
	}
	return results
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
		switch field {
		case "cpu_pct":
			val = n.CPUPCT
		case "mem_pct":
			val = n.MemPCT
		case "disk_pct":
			val = n.DiskPCT
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

func buildNotification(rule meta.AlertRuleRow, scope domain.AlertScope, groupKey, state string,
	value float64, alertID string, firedAt time.Time, cooldownUntil, resolvedAt *int64, isTest bool) map[string]any {
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
		"threshold": rule.Threshold,
		"scope": map[string]any{
			"node_id":   scope.NodeID,
			"app":       scope.App,
			"stream_id": scope.StreamID,
		},
		"test": isTest,
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
		"version":       1,
		"alert_id":      alertID,
		"rule_id":       ruleID,
		"state":         "firing",
		"severity":      "info",
		"ts":            now.UnixMilli(),
		"title":         "Pulse test notification",
		"metric":        "test_fire",
		"value":         0.0,
		"threshold":     0.0,
		"scope":         map[string]any{},
		"test":          true,
		"cooldown_until": nil,
		"group_key":     nil,
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
