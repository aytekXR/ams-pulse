import { useState } from "react";
import type { AlertRule, AlertRuleWrite } from "@/lib/api/types";

interface Props {
  initial?: AlertRule;
  onSave: (data: AlertRuleWrite) => Promise<void>;
  onCancel: () => void;
}

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

const OPERATORS = ["gt", "lt", "gte", "lte", "eq"] as const;
const SEVERITIES = ["info", "warning", "critical"] as const;
const WINDOWS = [60, 300, 600, 1800, 3600];

export function AlertRuleForm({ initial, onSave, onCancel }: Props) {
  const [name, setName] = useState(initial?.name ?? "");
  const [metric, setMetric] = useState(initial?.metric ?? METRICS[0]);
  const [operator, setOperator] = useState<"gt" | "lt" | "gte" | "lte" | "eq">(initial?.operator ?? "gt");
  const [threshold, setThreshold] = useState(String(initial?.threshold ?? ""));
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

  const validate = (): boolean => {
    const errs: Record<string, string> = {};
    if (!name.trim()) errs.name = "Name is required";
    if (!threshold.trim() || isNaN(Number(threshold))) errs.threshold = "Valid number required";
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
        operator,
        threshold: Number(threshold),
        window_s: windowS,
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
            {METRICS.map((m) => <option key={m} value={m}>{m}</option>)}
          </select>
        </div>
        <div style={fieldStyle}>
          <label style={labelStyle}>Operator</label>
          <select style={inputStyle} value={operator} onChange={(e) => setOperator(e.target.value as "gt" | "lt" | "gte" | "lte" | "eq")}>
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
          {errors.threshold && <span style={{ fontSize: 11, color: "var(--color-error)" }}>{errors.threshold}</span>}
        </div>
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 12 }}>
        <div style={fieldStyle}>
          <label style={labelStyle}>Severity</label>
          <select style={inputStyle} value={severity} onChange={(e) => setSeverity(e.target.value as typeof severity)}>
            {SEVERITIES.map((s) => <option key={s} value={s}>{s}</option>)}
          </select>
        </div>
        <div style={fieldStyle}>
          <label style={labelStyle}>Window</label>
          <select style={inputStyle} value={windowS} onChange={(e) => setWindowS(Number(e.target.value))}>
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
          Scope (optional — leave blank to match all)
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

      {/* enabled / muted — distinct controls per CR-2 */}
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
            color: "#fff",
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
