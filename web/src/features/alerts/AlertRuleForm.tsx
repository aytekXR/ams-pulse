import { useState, useRef } from "react";
import type { AlertChannel, AlertRule, AlertRuleWrite, MaintenanceWindow } from "@/lib/api/types";
import { ANOMALY_METRICS, THRESHOLD_METRICS, isSupportedMetric, metricForRuleType } from "./metrics";

interface Props {
  initial?: AlertRule;
  /** Every configured channel: the rule notifies the ones ticked in the form. */
  channels: AlertChannel[];
  onSave: (data: AlertRuleWrite) => Promise<void>;
  onCancel: () => void;
}

// Metric lists live in ./metrics.ts, pinned to the server's lists by a test. Anomaly rules
// take only what the Welford detector tracks, and window_s must be 3600 (its fixed window).

const OPERATORS = ["gt", "lt", "gte", "lte", "eq"] as const;
const SEVERITIES = ["info", "warning", "critical"] as const;
const WINDOWS = [60, 300, 600, 1800, 3600];
const DAYS = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];

function fmtDuration(s: number): string {
  if (s >= 3600 && s % 3600 === 0) return `${s / 3600} h`;
  if (s >= 3600) return `${Math.floor(s / 3600)} h ${Math.round((s % 3600) / 60)} min`;
  if (s >= 60 && s % 60 === 0) return `${s / 60} min`;
  return `${s} s`;
}

/** "0 22 6" → "Every Saturday at 22:00 UTC for 2 h"; a form it cannot read is shown as written. */
function describeWindow(w: MaintenanceWindow): string {
  const raw = `"${w.start_cron}" for ${fmtDuration(w.duration_s)}`;
  const f = w.start_cron.trim().split(/\s+/);
  if (f.length < 2 || f.length > 3) return raw;
  // "*" in the minute or hour means 0: a window starts at one time of day.
  const num = (v: string, hi: number) => (v === "*" ? 0 : /^\d+$/.test(v) && Number(v) <= hi ? Number(v) : NaN);
  const min = num(f[0], 59);
  const hour = num(f[1], 23);
  if (Number.isNaN(min) || Number.isNaN(hour)) return raw;
  let days = "Every day";
  if (f.length === 3 && f[2] !== "*") {
    const m = /^([0-6])(?:-([0-6]))?$/.exec(f[2]);
    if (!m || (m[2] !== undefined && Number(m[2]) < Number(m[1]))) return raw;
    days = m[2] === undefined ? `Every ${DAYS[Number(m[1])]}` : `${DAYS[Number(m[1])]}–${DAYS[Number(m[2])]}`;
  }
  const hhmm = `${String(hour).padStart(2, "0")}:${String(min).padStart(2, "0")}`;
  return `${days} at ${hhmm} UTC for ${fmtDuration(w.duration_s)}`;
}

export function AlertRuleForm({ initial, channels, onSave, onCancel }: Props) {
  // S11 WO-B: rule type state (threshold | anomaly).
  const [ruleType, setRuleType] = useState<"threshold" | "anomaly">(
    initial?.rule_type ?? "threshold",
  );
  const [sigma, setSigma] = useState(String(initial?.sigma ?? "4.0"));
  const [minSamples, setMinSamples] = useState(String(initial?.min_samples ?? "30"));

  const [name, setName] = useState(initial?.name ?? "");
  // An existing rule keeps its stored metric, even one the form does not offer: the select
  // then lists it too, so it shows what the rule really watches.
  const [metric, setMetric] = useState(initial?.metric ?? THRESHOLD_METRICS[0]);
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
  // PUT replaces the whole rule, so the form sends the channels and maintenance windows too.
  // Before S126 it sent neither: every rule saved here notified no one, and an edit erased
  // the channels and windows set through the API. IDs the checkboxes cannot show (a
  // deleted channel) stay as they are — the checkboxes manage only the listed channels.
  const [channelIds, setChannelIds] = useState<string[]>(initial?.channel_ids ?? []);
  const windows = initial?.maintenance_windows ?? [];
  const [saving, setSaving] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});

  // Refs for auto-focus on first invalid field after submit failure.
  const nameRef = useRef<HTMLInputElement>(null);
  const thresholdRef = useRef<HTMLInputElement>(null);
  const sigmaRef = useRef<HTMLInputElement>(null);

  // Handle rule type switch: the two types accept different metrics (node_cpu ↔ cpu_pct …),
  // and anomaly rules require window_s=3600.
  const handleRuleTypeChange = (newType: "threshold" | "anomaly") => {
    setRuleType(newType);
    setMetric(metricForRuleType(metric, newType));
    if (newType === "anomaly") {
      setWindowS(3600);
    }
  };

  const toggleChannel = (id: string, on: boolean) =>
    setChannelIds((ids) => (on ? [...ids, id] : ids.filter((x) => x !== id)));
  const listedIds = new Set(channels.map((c) => c.id));
  const unlistedCount = channelIds.filter((id) => !listedIds.has(id)).length;
  // A channel-less rule is legitimate (history only), so this is a hint, not an error.
  const notifiesNoOne = enabled && !muted && !channelIds.some((id) => listedIds.has(id));

  const offeredMetrics = ruleType === "anomaly" ? ANOMALY_METRICS : THRESHOLD_METRICS;
  const metricOptions = offeredMetrics.includes(metric) ? offeredMetrics : [...offeredMetrics, metric];

  // Returns the error map and calls setErrors; caller checks Object.keys(errs).length.
  const validate = (): Record<string, string> => {
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
    return errs;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const errs = validate();
    if (Object.keys(errs).length > 0) {
      // Auto-focus the first invalid field so keyboard/AT users land on it.
      if (errs.name) nameRef.current?.focus();
      else if (errs.threshold) thresholdRef.current?.focus();
      else if (errs.sigma) sigmaRef.current?.focus();
      return;
    }
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
        channel_ids: channelIds,
        maintenance_windows: windows,
      });
    } finally {
      setSaving(false);
    }
  };

  const fieldStyle: React.CSSProperties = {
    display: "flex",
    flexDirection: "column",
    gap: "var(--space-1)",
  };

  const labelStyle: React.CSSProperties = {
    fontSize: 12,
    fontWeight: 500,
    color: "var(--color-secondary)",
  };

  // s111 D4-pattern: outline:"none" removed — inputs carry
  // className="filter-input" so the shared :focus-visible ring applies.
  const inputStyle: React.CSSProperties = {
    background: "var(--color-surface-2)",
    border: "1px solid var(--color-border)",
    borderRadius: "var(--radius-control)",
    padding: "7px 10px",
    color: "var(--color-text)",
    fontSize: 13,
  };
  return (
    <form onSubmit={(e) => void handleSubmit(e)} style={{ display: "flex", flexDirection: "column", gap: "var(--space-4)" }}>
      {/* Each inline field error IS its own live region (role="alert" on the message span),
          so it is announced where it appears and again via aria-describedby when the field
          takes focus. An earlier draft ALSO mirrored every message into a separate sr-only
          aria-live div — which put the same text in the DOM twice and made a screen reader
          announce each error twice over. Removed: one error, one node. */}
      <h3 style={{ margin: 0, fontSize: 15, fontWeight: 600 }}>{initial ? "Edit rule" : "New alert rule"}</h3>

      {/* Rule type -- S11 WO-B: anomaly mode switch */}
      <div style={fieldStyle}>
        <label htmlFor="rule-rule-type" style={labelStyle}>Rule type</label>
        <select
          id="rule-rule-type"
          aria-label="Rule type"
          className="filter-input" style={inputStyle}
          value={ruleType}
          onChange={(e) => handleRuleTypeChange(e.target.value as "threshold" | "anomaly")}
        >
          <option value="threshold">threshold</option>
          <option value="anomaly">anomaly</option>
        </select>
      </div>

      {/* Name */}
      <div style={fieldStyle}>
        <label htmlFor="rule-name" style={labelStyle}>Name *</label>
        <input
          id="rule-name"
          ref={nameRef}
          className="filter-input" style={{ ...inputStyle, borderColor: errors.name ? "var(--color-error)" : "var(--color-border)" }}
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="e.g. High CPU alert"
          aria-invalid={errors.name ? true : undefined}
          aria-describedby={errors.name ? "rule-name-error" : undefined}
        />
        {errors.name && (
          <span id="rule-name-error" role="alert" style={{ fontSize: 11, color: "var(--color-error)" }}>
            {errors.name}
          </span>
        )}
      </div>

      {/* Metric / Operator / Threshold  —or—  Metric / Sigma / MinSamples */}
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: "var(--space-3)" }}>
        <div style={fieldStyle}>
          <label htmlFor="rule-metric" style={labelStyle}>Metric</label>
          <select id="rule-metric" className="filter-input" style={inputStyle} value={metric} onChange={(e) => setMetric(e.target.value)}>
            {metricOptions.map((m) => (
              <option key={m} value={m}>
                {isSupportedMetric(m, ruleType) ? m : `${m} (not supported — choose another)`}
              </option>
            ))}
          </select>
        </div>

        {ruleType === "threshold" ? (
          <>
            <div style={fieldStyle}>
              <label htmlFor="rule-operator" style={labelStyle}>Operator</label>
              <select
                id="rule-operator"
                className="filter-input" style={inputStyle}
                value={operator}
                onChange={(e) => setOperator(e.target.value as "gt" | "lt" | "gte" | "lte" | "eq")}
              >
                {OPERATORS.map((o) => <option key={o} value={o}>{o}</option>)}
              </select>
            </div>
            <div style={fieldStyle}>
              <label htmlFor="rule-threshold" style={labelStyle}>Threshold *</label>
              <input
                id="rule-threshold"
                ref={thresholdRef}
                className="filter-input" style={{ ...inputStyle, borderColor: errors.threshold ? "var(--color-error)" : "var(--color-border)" }}
                type="number"
                value={threshold}
                onChange={(e) => setThreshold(e.target.value)}
                placeholder="0"
                aria-invalid={errors.threshold ? true : undefined}
                aria-describedby={errors.threshold ? "rule-threshold-error" : undefined}
              />
              {errors.threshold && (
                <span id="rule-threshold-error" role="alert" style={{ fontSize: 11, color: "var(--color-error)" }}>
                  {errors.threshold}
                </span>
              )}
            </div>
          </>
        ) : (
          <>
            {/* Anomaly mode: sigma and min_samples replace operator+threshold */}
            <div style={fieldStyle}>
              <label htmlFor="rule-sigma" style={labelStyle}>Sigma</label>
              <input
                id="rule-sigma"
                aria-label="Sigma"
                ref={sigmaRef}
                className="filter-input" style={{ ...inputStyle, borderColor: errors.sigma ? "var(--color-error)" : "var(--color-border)" }}
                type="number"
                step="0.1"
                value={sigma}
                onChange={(e) => setSigma(e.target.value)}
                placeholder="4.0"
                aria-invalid={errors.sigma ? true : undefined}
                aria-describedby={errors.sigma ? "rule-sigma-error" : undefined}
              />
              {errors.sigma && (
                <span id="rule-sigma-error" role="alert" style={{ fontSize: 11, color: "var(--color-error)" }}>
                  {errors.sigma}
                </span>
              )}
            </div>
            <div style={fieldStyle}>
              <label htmlFor="rule-min-samples" style={labelStyle}>Min Samples</label>
              <input
                id="rule-min-samples"
                aria-label="Min Samples"
                className="filter-input" style={inputStyle}
                type="number"
                value={minSamples}
                onChange={(e) => setMinSamples(e.target.value)}
                placeholder="30"
              />
            </div>
          </>
        )}
      </div>

      {/* Severity / Window / Cooldown */}
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: "var(--space-3)" }}>
        <div style={fieldStyle}>
          <label htmlFor="rule-severity" style={labelStyle}>Severity</label>
          <select id="rule-severity" className="filter-input" style={inputStyle} value={severity} onChange={(e) => setSeverity(e.target.value as typeof severity)}>
            {SEVERITIES.map((s) => <option key={s} value={s}>{s}</option>)}
          </select>
        </div>
        <div style={fieldStyle}>
          <label htmlFor="rule-window" style={labelStyle}>
            Window{ruleType === "anomaly" ? " (locked 3600 s)" : ""}
          </label>
          <select
            id="rule-window"
            className="filter-input" style={{ ...inputStyle, opacity: ruleType === "anomaly" ? 0.6 : 1 }}
            value={ruleType === "anomaly" ? 3600 : windowS}
            onChange={(e) => setWindowS(Number(e.target.value))}
            disabled={ruleType === "anomaly"}
          >
            {WINDOWS.map((w) => <option key={w} value={w}>{w}s ({Math.round(w / 60)}m)</option>)}
          </select>
        </div>
        <div style={fieldStyle}>
          <label htmlFor="rule-cooldown" style={labelStyle}>Cooldown (s)</label>
          <input
            id="rule-cooldown"
            className="filter-input" style={inputStyle}
            type="number"
            value={cooldownS}
            onChange={(e) => setCooldownS(e.target.value)}
            min="0"
          />
        </div>
      </div>

      {/* Scope (optional) */}
      <details style={{ background: "var(--color-surface-2)", borderRadius: "var(--radius-control)", padding: "var(--space-3)" }}>
        <summary style={{ cursor: "pointer", fontSize: 13, color: "var(--color-secondary)", fontWeight: 500 }}>
          Scope (optional -- leave blank to match all)
        </summary>
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: "var(--space-3)", marginTop: "var(--space-3)" }}>
          <div style={fieldStyle}>
            <label htmlFor="rule-scope-stream" style={labelStyle}>Stream ID</label>
            <input id="rule-scope-stream" className="filter-input" style={inputStyle} value={scopeStreamId} onChange={(e) => setScopeStreamId(e.target.value)} placeholder="any" />
          </div>
          <div style={fieldStyle}>
            <label htmlFor="rule-scope-app" style={labelStyle}>App</label>
            <input id="rule-scope-app" className="filter-input" style={inputStyle} value={scopeApp} onChange={(e) => setScopeApp(e.target.value)} placeholder="any" />
          </div>
          <div style={fieldStyle}>
            <label htmlFor="rule-scope-node" style={labelStyle}>Node ID</label>
            <input id="rule-scope-node" className="filter-input" style={inputStyle} value={scopeNodeId} onChange={(e) => setScopeNodeId(e.target.value)} placeholder="any" />
          </div>
        </div>
        <div style={{ marginTop: "var(--space-3)", ...fieldStyle }}>
          <label htmlFor="rule-group-by" style={labelStyle}>Group by dimension (e.g. stream_id, app, node_id)</label>
          <input
            id="rule-group-by"
            className="filter-input" style={inputStyle}
            value={groupBy}
            onChange={(e) => setGroupBy(e.target.value)}
            placeholder="stream_id"
          />
        </div>
      </details>

      {/* Notify channels — S126: the rule notifies exactly the channels ticked here. */}
      <fieldset style={{ border: "none", margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: "var(--space-2)" }}>
        <legend style={{ ...labelStyle, padding: 0, marginBottom: "var(--space-1)" }}>Notify channels</legend>
        {channels.length === 0 ? (
          <p style={{ margin: 0, fontSize: 12, color: "var(--color-secondary)" }}>
            No channels yet. Add one on the Channels tab, then pick it here.
          </p>
        ) : (
          channels.map((ch) => (
            <label key={ch.id} style={{ display: "flex", alignItems: "center", gap: "var(--space-2)", fontSize: 13, cursor: "pointer" }}>
              <input
                type="checkbox"
                checked={channelIds.includes(ch.id)}
                onChange={(e) => toggleChannel(ch.id, e.target.checked)}
                style={{ width: 14, height: 14, accentColor: "var(--color-accent)" }}
              />
              {ch.name}{" "}
              <span style={{ fontSize: 12, color: "var(--color-secondary)" }}>({ch.type})</span>
            </label>
          ))
        )}
        {unlistedCount > 0 && (
          <p style={{ margin: 0, fontSize: 12, color: "var(--color-secondary)" }}>
            Also linked: {unlistedCount} channel{unlistedCount === 1 ? "" : "s"} not in the list (kept as is).
          </p>
        )}
        {notifiesNoOne && (
          <p
            data-testid="rule-no-channel-hint"
            style={{
              margin: 0,
              fontSize: 12,
              color: "var(--color-text)",
              background: "var(--color-warning-bg)",
              borderLeft: "3px solid var(--color-warning)",
              borderRadius: "var(--radius-control)",
              padding: "var(--space-2) var(--space-3)",
            }}
          >
            No channel selected: this rule records alert history but notifies no one.
          </p>
        )}
      </fieldset>

      {/* Maintenance windows are not edited here (API only); the form shows them and keeps them. */}
      {windows.length > 0 && (
        <div style={fieldStyle}>
          <span style={labelStyle}>Maintenance windows</span>
          <ul style={{ margin: 0, paddingLeft: "var(--space-5)", fontSize: 13 }}>
            {windows.map((w, i) => <li key={i}>{describeWindow(w)}</li>)}
          </ul>
          <span style={{ fontSize: 12, color: "var(--color-secondary)" }}>
            Kept as they are when you save. Change them through the API (maintenance_windows).
          </span>
        </div>
      )}

      {/* enabled / muted -- distinct controls per CR-2 */}
      <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-2)" }}>
        <label style={{ display: "flex", alignItems: "center", gap: "var(--space-2)", fontSize: 13, cursor: "pointer" }}>
          <input
            type="checkbox"
            checked={enabled}
            onChange={(e) => setEnabled(e.target.checked)}
            style={{ width: 14, height: 14, accentColor: "var(--color-accent)" }}
          />
          Enabled (rule is evaluated; uncheck to pause without deleting)
        </label>
        <label style={{ display: "flex", alignItems: "center", gap: "var(--space-2)", fontSize: 13, cursor: "pointer" }}>
          <input
            type="checkbox"
            checked={muted}
            onChange={(e) => setMuted(e.target.checked)}
            style={{ width: 14, height: 14, accentColor: "var(--color-accent)" }}
          />
          Muted (evaluated and recorded, but no notifications sent)
        </label>
      </div>

      <div style={{ display: "flex", gap: 10, justifyContent: "flex-end", paddingTop: "var(--space-1)" }}>
        <button
          type="button"
          onClick={onCancel}
          className="btn-secondary"
          style={{
            background: "var(--color-surface-2)",
            borderRadius: "var(--radius-control)",
            padding: "var(--space-2) var(--space-4)",
            cursor: "pointer",
            fontSize: 13,
          }}
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={saving}
          className="btn-primary"
          style={{
            border: "none",
            color: "var(--color-on-signal)",
            borderRadius: "var(--radius-control)",
            padding: "8px 20px",
            fontSize: 13,
            fontWeight: 600,
          }}
        >
          {saving ? "Saving…" : "Save rule"}
        </button>
      </div>
    </form>
  );
}
