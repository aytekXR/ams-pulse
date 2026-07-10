package alert

// wave3.go — S11 WO-B: anomaly alert rule engine.
//
// Adds an AnomalyBaselineReader interface (parallel to QoEReader in wave2.go)
// that the Evaluator uses to look up Welford baselines from the meta store.
// When a rule has RuleType="anomaly", evaluateRule dispatches to evalAnomalyMetric
// instead of the existing metric switch.
//
// Supported metrics: viewer_count (streams), cpu_pct, mem_pct (nodes).
// viewer_count maps to the Detector baseline key "viewers" (see metricAliases).

import (
	"context"
	"fmt"
	"math"
	"sync"

	"github.com/pulse-analytics/pulse/server/internal/anomaly"
	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// ─── AnomalyBaselineReader ────────────────────────────────────────────────────

// AnomalyBaselineReader reads pre-computed Welford baselines from the meta store.
// Satisfied by *meta.Store (GetAnomalyBaseline at store/meta/anomaly.go:57).
// Injected via Evaluator.SetAnomalyBaselineReader.
// nil = anomaly rules are skipped with a WARN log each tick.
type AnomalyBaselineReader interface {
	GetAnomalyBaseline(ctx context.Context, metric, scope string, windowS int) (*anomaly.AnomalyBaselineRow, error)
}

// FakeAnomalyBaselineReader is a test stub for AnomalyBaselineReader.
// Set Row to the baseline to return (nil = no baseline found).
// Set Err to simulate a reader error (causes the stream/node to be skipped).
// Set CalledWith to a non-nil *[]string to record the metric arguments passed.
type FakeAnomalyBaselineReader struct {
	Row        *anomaly.AnomalyBaselineRow
	Err        error
	CalledWith *[]string // optional: if non-nil, metric args are appended here
	Mu         sync.Mutex
}

// GetAnomalyBaseline implements AnomalyBaselineReader.
func (f *FakeAnomalyBaselineReader) GetAnomalyBaseline(_ context.Context, metric, _ string, _ int) (*anomaly.AnomalyBaselineRow, error) {
	if f.CalledWith != nil {
		f.Mu.Lock()
		*f.CalledWith = append(*f.CalledWith, metric)
		f.Mu.Unlock()
	}
	return f.Row, f.Err
}

// ─── AnomalyRuleValidationError ──────────────────────────────────────────────

// AnomalyRuleValidationError is returned by ValidateAnomalyRule when an anomaly
// rule fails validation. The API layer maps this to HTTP 400.
type AnomalyRuleValidationError struct {
	Field   string
	Message string
}

func (e *AnomalyRuleValidationError) Error() string {
	return fmt.Sprintf("anomaly rule validation: %s: %s", e.Field, e.Message)
}

// supportedAnomalyMetrics is the set of alert rule metric names accepted for
// anomaly rules. These must have a corresponding Detector observation path:
//   - viewer_count → Detector tracks "viewers" per stream (metricAliases maps it)
//   - cpu_pct, mem_pct → Detector tracks these per node
var supportedAnomalyMetrics = map[string]bool{
	"viewer_count": true,
	"cpu_pct":      true,
	"mem_pct":      true,
}

// ValidateAnomalyRule validates anomaly-specific constraints on a rule.
// Returns nil for non-anomaly rules (threshold/empty RuleType always passes).
// Returns *AnomalyRuleValidationError for anomaly rules that fail:
//   - metric must be in the supported set (viewer_count, cpu_pct, mem_pct)
//   - window_s must be exactly 3600 (matches the Detector's fixed windowS)
func ValidateAnomalyRule(row meta.AlertRuleRow) error {
	if row.RuleType != "anomaly" {
		return nil // only validate anomaly rules
	}
	if !supportedAnomalyMetrics[row.Metric] {
		return &AnomalyRuleValidationError{
			Field:   "metric",
			Message: fmt.Sprintf("metric %q is not supported for anomaly rules; supported: viewer_count, cpu_pct, mem_pct", row.Metric),
		}
	}
	if row.WindowS != 3600 {
		return &AnomalyRuleValidationError{
			Field:   "window_s",
			Message: "anomaly rules require window_s=3600 (matches the Welford Detector's rolling window)",
		}
	}
	return nil
}

// ─── Metric alias table ───────────────────────────────────────────────────────

// metricAliases maps alert rule metric names to the Detector's baseline storage keys.
// The Detector (anomaly.go:242) stores viewer counts under "viewers", not "viewer_count".
var metricAliases = map[string]string{
	"viewer_count": "viewers",
	// cpu_pct and mem_pct match the Detector keys directly — no alias needed.
}

// ─── scopeJSONAnomaly ─────────────────────────────────────────────────────────

// scopeJSONAnomaly builds the canonical scope string used as the anomaly_baselines key.
// Mirrors the unexported scopeJSON helper in the anomaly package (anomaly.go:414).
// Only one of nodeID or streamID should be non-empty for the supported metrics.
func scopeJSONAnomaly(nodeID, streamID string) string {
	if nodeID != "" {
		return `{"node_id":"` + nodeID + `"}`
	}
	if streamID != "" {
		return `{"stream_id":"` + streamID + `"}`
	}
	return "{}"
}

// ─── evalAnomalyMetric ───────────────────────────────────────────────────────

// anomalyEvalInfo carries anomaly-specific context from evalAnomalyMetric to
// the fire()/buildNotification path. Present only on evalResults from anomaly rules.
type anomalyEvalInfo struct {
	// Expected is the baseline mean; used as "expected" and "threshold" in notifications.
	Expected float64
	// SigmaMultiplier is the effective sigma that was configured for this rule.
	SigmaMultiplier float64
}

// evalAnomalyMetric evaluates an anomaly alert rule against the current live snapshot.
//
// Metrics:
//   - viewer_count → iterates snap.Streams, baseline key "viewers", groupKey = stream_id
//   - cpu_pct, mem_pct → iterates snap.Nodes, baseline key = metric, groupKey = node_id
//
// Stddev floor formula (identical to anomaly.Detector.ComputeFlags, anomaly.go:383):
//
//	effStddev = max(stddev, max(StddevRelEpsilon*|mean|, StddevAbsEpsilon))
//
// A result with ok=true means z > effectiveSigma (condition met for fire() path).
// The anomalyInfo field carries baseline mean and sigma for the notification payload.
func (e *Evaluator) evalAnomalyMetric(ctx context.Context, snap *domain.LiveSnapshot, scope domain.AlertScope, rule meta.AlertRuleRow) []evalResult {
	// Obtain the reader under the lock.
	e.mu.Lock()
	reader := e.anomalyReader
	e.mu.Unlock()

	if reader == nil {
		// Warn once per tick (matches QoE reader pattern in wave2.go).
		e.logger.Warn("alert: anomaly_reader not configured — anomaly rules skipped this tick (S11 WO-B)")
		return nil
	}

	// Effective parameters.
	effectiveSigma := rule.Sigma
	if effectiveSigma <= 0 {
		effectiveSigma = anomaly.DefaultSigma
	}
	effectiveMinSamples := rule.MinSamples
	if effectiveMinSamples <= 0 {
		effectiveMinSamples = anomaly.MinSamples
	}

	// Resolve baseline lookup metric name (apply alias if needed).
	lookupMetric := rule.Metric
	if alias, ok := metricAliases[rule.Metric]; ok {
		lookupMetric = alias
	}

	switch rule.Metric {
	case "viewer_count":
		return e.evalAnomalyStreams(ctx, snap, scope, rule, lookupMetric, effectiveSigma, effectiveMinSamples, rule.WindowS, reader)
	case "cpu_pct", "mem_pct":
		return e.evalAnomalyNodes(ctx, snap, scope, rule, lookupMetric, effectiveSigma, effectiveMinSamples, rule.WindowS, reader)
	default:
		e.logger.Warn("alert: anomaly rule metric not supported", "metric", rule.Metric)
		return nil
	}
}

// evalAnomalyStreams handles stream-level anomaly metrics (viewer_count → "viewers").
func (e *Evaluator) evalAnomalyStreams(ctx context.Context, snap *domain.LiveSnapshot, scope domain.AlertScope, rule meta.AlertRuleRow,
	lookupMetric string, effectiveSigma float64, effectiveMinSamples, windowS int, reader AnomalyBaselineReader) []evalResult {
	var results []evalResult
	var warnedErr bool

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

		val := float64(s.ViewerCount)
		scopeStr := scopeJSONAnomaly("", sid)

		baseline, err := reader.GetAnomalyBaseline(ctx, lookupMetric, scopeStr, windowS)
		if err != nil {
			if !warnedErr {
				e.logger.Warn("alert: anomaly_reader error — stream skipped", "stream_id", sid, "error", err)
				warnedErr = true
			}
			continue
		}
		if baseline == nil {
			continue // no baseline yet → skip
		}
		if baseline.SampleCount < effectiveMinSamples {
			continue // baseline not warmed up yet
		}

		effStddev := math.Max(baseline.Stddev, math.Max(anomaly.StddevRelEpsilon*math.Abs(baseline.Mean), anomaly.StddevAbsEpsilon))
		z := math.Abs(val-baseline.Mean) / effStddev
		ok := z > effectiveSigma

		results = append(results, evalResult{
			groupKey: sid,
			value:    val,
			ok:       ok,
			anomalyInfo: &anomalyEvalInfo{
				Expected:        baseline.Mean,
				SigmaMultiplier: effectiveSigma,
			},
		})
	}
	return results
}

// evalAnomalyNodes handles node-level anomaly metrics (cpu_pct, mem_pct).
func (e *Evaluator) evalAnomalyNodes(ctx context.Context, snap *domain.LiveSnapshot, scope domain.AlertScope, rule meta.AlertRuleRow,
	lookupMetric string, effectiveSigma float64, effectiveMinSamples, windowS int, reader AnomalyBaselineReader) []evalResult {
	var results []evalResult
	var warnedErr bool

	for nodeID, n := range snap.Nodes {
		if scope.NodeID != "" && nodeID != scope.NodeID {
			continue
		}

		var val float64
		switch rule.Metric {
		case "cpu_pct":
			val = n.CPUPCT
		case "mem_pct":
			val = n.MemPCT
		}

		scopeStr := scopeJSONAnomaly(nodeID, "")

		baseline, err := reader.GetAnomalyBaseline(ctx, lookupMetric, scopeStr, windowS)
		if err != nil {
			if !warnedErr {
				e.logger.Warn("alert: anomaly_reader error — node skipped", "node_id", nodeID, "error", err)
				warnedErr = true
			}
			continue
		}
		if baseline == nil {
			continue
		}
		if baseline.SampleCount < effectiveMinSamples {
			continue
		}

		effStddev := math.Max(baseline.Stddev, math.Max(anomaly.StddevRelEpsilon*math.Abs(baseline.Mean), anomaly.StddevAbsEpsilon))
		z := math.Abs(val-baseline.Mean) / effStddev
		ok := z > effectiveSigma

		results = append(results, evalResult{
			groupKey: nodeID,
			value:    val,
			ok:       ok,
			anomalyInfo: &anomalyEvalInfo{
				Expected:        baseline.Mean,
				SigmaMultiplier: effectiveSigma,
			},
		})
	}
	return results
}
