import { useState } from "react";
import type { AlertRule, AlertRuleWrite } from "@/lib/api/types";

interface Props {
  initial?: AlertRule;
  onSave: (data: AlertRuleWrite) => Promise<void>;
  onCancel: () => void;
}

// All metrics supported by threshold rules.
const METRICS = [
  "viewer_count",
  "ingest_bitrate_kbps",
  "cpu_pct",
  "mem_pct",
  "packet_loss_pct",
  "jitter_ms",
  "rtt_ms",
  "health_score",
  "rebuffer_ratio",
];

// Anomaly rules: only metrics tracked by the Welford Detector.
// Detector tracks: viewers (-> "viewer_count"), ingest_bitrate_kbps (stream-scoped),
// cpu_pct, mem_pct, disk_pct (node-scoped).
// window_s must be 3600 (anomaly.go hardcoded Detector window).
const ANOMALY_METRICS = ["viewer_count", "ingest_bitrate_kbps", "cpu_pct", "mem_pct", "disk_pct"];

const OPERATORS = ["gt", "lt", "gte", "lte", "eq"] as const;
const SEVERITIES = ["info", "warning", "critical"] as const;
const WINDOWS = [60, 300, 600, 1800, 3600];

export function AlertRuleForm({ initial, onSave, onCancel }: Props) {
  // S11 WO-B: rule type state (threshold | anomaly).
  const [ruleType, setRuleType] = useState<"threshold" | "anomaly">(
    initial?.rule_type ?? "threshold",
  );
  const [sigma, setSigma] = useState(String(initial?.sigma ?? "4.0"));
  const [minSamples, setMinSamples] = useState(String(initial?.min_samples ?? "30"));

  const [name, setName] = useState(initial?.name ?? "");
  // In anomaly mode, metric is restricted to ANOMALY_METRICS.
  const [metric, setMetric] = useState(() => {
    const m = initial?.metric ?? METRICS[0];
    if (initial?.rule_type === "anomaly" && !ANOMALY_METRICS.includes(m)) {
      return ANOMALY_METRICS[0];
    }
    return m;
  });
  const [operator, setOperator] = useState<"gt" | "lt" | "gte" | "lte" | "eq">(
    initial?.operator ?? "gt",
  );
  const [threshold, setThreshold] = useState(String(initial?.threshold ?? ""));
  // In anomaly mode, window_s is forced to 3600 (server rejects other values).
  const [windowS, setWindowS] = useState(initial?.window_s ?? 300);
  const [severity, setSeverity] = useState(initial?.severity ?? "warning");
  const [cooldownS, setCooldownS] = useState(String(initial?.cooldown_s ?? "300"));
  // CR-2: enabled and muted are distinct controls
  const [enabled, setEnabled] = useState(initial?.enabled ?? true);
  const [muted, setMuted] = useState(initial?.muted ?? false);
  // group_by is the real grouping dimension (e.g. "stream_id", "app")
  const [groupBy, setGroupBy] = useState(initial?.group_by ?? "");
  const [scopeStreamId, setScopeStreamId] = useState(initial?.scope?.stream_id ?? "");
  const [scopeApp, setScopeApp] = useState(initial?.scope?.app ?? "");
  const [scopeNodeId, setScopeNodeId] = useState(initial?.scope?.node_id ?? "");
  const [saving, setSaving] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});

  // Handle rule type switch: enforce constraints when switching to anomaly.
  const handleRuleTypeChange = (newType: "threshold" | "anomaly") => {
    setRuleType(newType);
    if (newType === "anomaly") {
      // Lock window to 3600 (only valid window for anomaly rules).
      setWindowS(3600);
      // Restrict metric to anomaly-supported metrics.
      if (!ANOMALY_METRICS.includes(metric)) {
        setMetric(ANOMALY_METRICS[0]);
      }
    }
  };

  const validate = (): boolean => {
    const errs: Record<string, string> = {};
    if (!name.trim()) errs.name = "Name is required";

    if (ruleType === "threshold") {
      if (!threshold.trim() || isNaN(Number(threshold)))
        errs.threshold = "Valid number required";
    } else {
      // anomaly mode: validate sigma is a positive number.
      const sigmaNum = Number(sigma);
      if (!sigma.trim() || isNaN(sigmaNum) || sigmaNum <= 0)
        errs.sigma = "Sigma must be a positive number";
    }

    setErrors(errs);
    return Object.keys(errs).length === 0;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!validate()) return;
    setSaving(true);
    try {
      const scope: AlertRuleWrite["scope"] = {};
      if (scopeStreamId) scope.stream_id = scopeStreamId;
      if (scopeApp) scope.app = scopeApp;
      if (scopeNodeId) scope.node_id = scopeNodeId;

      await onSave({
        name: name.trim(),
        metric,
        rule_type: ruleType,
        // Anomaly fields: send configured values for anomaly; defaults for threshold.
        sigma: ruleType === "anomaly" ? Number(sigma) || 4.0 : 4.0,
        min_samples: ruleType === "anomaly" ? Number(minSamples) || 30 : 30,
        // Threshold fields: send configured values for threshold; neutral values for anomaly.
        operator: ruleType === "threshold" ? operator : "gt",
        threshold: ruleType === "threshold" ? Number(threshold) : 0,
        // Anomaly rules must use window_s=3600 (Detector window).
        window_s: ruleType === "anomaly" ? 3600 : windowS,
        severity,
        cooldown_s: Number(cooldownS) || 300,
        enabled,
        muted,
        group_by: groupBy.trim() || undefined,
        scope: Object.keys(scope).length > 0 ? scope : undefined,
        maintenance_windows: [],
      });
    } finally {
      setSaving(false);
    }
  };

  const fieldStyle: React.CSSProperties = {
    display: "flex",
    flexDirection: "column",
    gap: 4,
  };

  const labelStyle: React.CSSProperties = {
    fontSize: 12,
    fontWeight: 500,
    color: "var(--color-muted)",
  };

  const inputStyle: React.CSSProperties = {
    background: "var(--color-surface-2)",
    border: "1px solid var(--color-border)",
    borderRadius: 6,
    padding: "7px 10px",
    color: "var(--color-text)",
    fontSize: 13,
    outline: "none",
  };

  return (
    <form onSubmit={(e) => void handleSubmit(e)} style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      <h3 style={{ margin: 0, fontSize: 15, fontWeight: 600 }}>{initial ? "Edit rule" : "New alert rule"}</h3>

      {/* Rule type -- S11 WO-B: anomaly mode switch */}
      <div style={fieldStyle}>
        <label style={labelStyle}>Rule type</label>
        <select
          aria-label="Rule type"
          style={inputStyle}
          value={ruleType}
          onChange={(e) => handleRuleTypeChange(e.target.value as "threshold" | "anomaly")}
        >
          <option value="threshold">threshold</option>
          <option value="anomaly">anomaly</option>
        </select>
      </div>

      <div style={fieldStyle}>
        <label style={labelStyle}>Name *</label>
        <input
          style={{ ...inputStyle, borderColor: errors.name ? "var(--color-error)" : "var(--color-border)" }}
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="e.g. High CPU alert"
        />
        {errors.name && <span style={{ fontSize: 11, color: "var(--color-error)" }}>{errors.name}</span>}
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 12 }}>
        <div style={fieldStyle}>
          <label style={labelStyle}>Metric</label>
          <select style={inputStyle} value={metric} onChange={(e) => setMetric(e.target.value)}>
            {(ruleType === "anomaly" ? ANOMALY_METRICS : METRICS).map((m) => (
              <option key={m} value={m}>{m}</option>
            ))}
          </select>
        </div>

        {ruleType === "threshold" ? (
          <>
            <div style={fieldStyle}>
              <label style={labelStyle}>Operator</label>
              <select
                style={inputStyle}
                value={operator}
                onChange={(e) => setOperator(e.target.value as "gt" | "lt" | "gte" | "lte" | "eq")}
              >
                {OPERATORS.map((o) => <option key={o} value={o}>{o}</option>)}
              </select>
            </div>
            <div style={fieldStyle}>
              <label style={labelStyle}>Threshold *</label>
              <input
                style={{ ...inputStyle, borderColor: errors.threshold ? "var(--color-error)" : "var(--color-border)" }}
                type="number"
                value={threshold}
                onChange={(e) => setThreshold(e.target.value)}
                placeholder="0"
              />
              {errors.threshold && (
                <span style={{ fontSize: 11, color: "var(--color-error)" }}>{errors.threshold}</span>
              )}
            </div>
          </>
        ) : (
          <>
            {/* Anomaly mode: sigma and min_samples replace operator+threshold */}
            <div style={fieldStyle}>
              <label style={labelStyle}>Sigma</label>
              <input
                aria-label="Sigma"
                style={{ ...inputStyle, borderColor: errors.sigma ? "var(--color-error)" : "var(--color-border)" }}
                type="number"
                step="0.1"
                value={sigma}
                onChange={(e) => setSigma(e.target.value)}
                placeholder="4.0"
              />
              {errors.sigma && (
                <span style={{ fontSize: 11, color: "var(--color-error)" }}>{errors.sigma}</span>
              )}
            </div>
            <div style={fieldStyle}>
              <label style={labelStyle}>Min Samples</label>
              <input
                aria-label="Min Samples"
                style={inputStyle}
                type="number"
                value={minSamples}
                onChange={(e) => setMinSamples(e.target.value)}
                placeholder="30"
              />
            </div>
          </>
        )}
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 12 }}>
        <div style={fieldStyle}>
          <label style={labelStyle}>Severity</label>
          <select style={inputStyle} value={severity} onChange={(e) => setSeverity(e.target.value as typeof severity)}>
            {SEVERITIES.map((s) => <option key={s} value={s}>{s}</option>)}
          </select>
        </div>
        <div style={fieldStyle}>
          <label style={labelStyle}>
            Window{ruleType === "anomaly" ? " (locked 3600 s)" : ""}
          </label>
          <select
            style={{ ...inputStyle, opacity: ruleType === "anomaly" ? 0.6 : 1 }}
            value={ruleType === "anomaly" ? 3600 : windowS}
            onChange={(e) => setWindowS(Number(e.target.value))}
            disabled={ruleType === "anomaly"}
          >
            {WINDOWS.map((w) => <option key={w} value={w}>{w}s ({Math.round(w / 60)}m)</option>)}
          </select>
        </div>
        <div style={fieldStyle}>
          <label style={labelStyle}>Cooldown (s)</label>
          <input
            style={inputStyle}
            type="number"
            value={cooldownS}
            onChange={(e) => setCooldownS(e.target.value)}
            min="0"
          />
        </div>
      </div>

      {/* Scope (optional) */}
      <details style={{ background: "var(--color-surface-2)", borderRadius: 6, padding: "12px" }}>
        <summary style={{ cursor: "pointer", fontSize: 13, color: "var(--color-muted)", fontWeight: 500 }}>
          Scope (optional -- leave blank to match all)
        </summary>
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 12, marginTop: 12 }}>
          <div style={fieldStyle}>
            <label style={labelStyle}>Stream ID</label>
            <input style={inputStyle} value={scopeStreamId} onChange={(e) => setScopeStreamId(e.target.value)} placeholder="any" />
          </div>
          <div style={fieldStyle}>
            <label style={labelStyle}>App</label>
            <input style={inputStyle} value={scopeApp} onChange={(e) => setScopeApp(e.target.value)} placeholder="any" />
          </div>
          <div style={fieldStyle}>
            <label style={labelStyle}>Node ID</label>
            <input style={inputStyle} value={scopeNodeId} onChange={(e) => setScopeNodeId(e.target.value)} placeholder="any" />
          </div>
        </div>
        <div style={{ marginTop: 12, ...fieldStyle }}>
          <label style={labelStyle}>Group by dimension (e.g. stream_id, app, node_id)</label>
          <input
            style={inputStyle}
            value={groupBy}
            onChange={(e) => setGroupBy(e.target.value)}
            placeholder="stream_id"
          />
        </div>
      </details>

      {/* enabled / muted -- distinct controls per CR-2 */}
      <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
        <label style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 13, cursor: "pointer" }}>
          <input
            type="checkbox"
            checked={enabled}
            onChange={(e) => setEnabled(e.target.checked)}
            style={{ width: 14, height: 14, accentColor: "var(--color-accent)" }}
          />
          Enabled (rule is evaluated; uncheck to pause without deleting)
        </label>
        <label style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 13, cursor: "pointer" }}>
          <input
            type="checkbox"
            checked={muted}
            onChange={(e) => setMuted(e.target.checked)}
            style={{ width: 14, height: 14, accentColor: "var(--color-accent)" }}
          />
          Muted (evaluated and recorded, but no notifications sent)
        </label>
      </div>

      <div style={{ display: "flex", gap: 10, justifyContent: "flex-end", paddingTop: 4 }}>
        <button
          type="button"
          onClick={onCancel}
          style={{
            background: "var(--color-surface-2)",
            border: "1px solid var(--color-border)",
            color: "var(--color-muted)",
            borderRadius: 6,
            padding: "8px 16px",
            cursor: "pointer",
            fontSize: 13,
          }}
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={saving}
          style={{
            background: "var(--color-accent)",
            border: "none",
            color: "var(--color-on-signal)",
            borderRadius: 6,
            padding: "8px 20px",
            cursor: "pointer",
            fontSize: 13,
            fontWeight: 600,
            opacity: saving ? 0.7 : 1,
          }}
        >
          {saving ? "Saving…" : "Save rule"}
        </button>
      </div>
    </form>
  );
}
