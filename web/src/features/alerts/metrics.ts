// Alert-rule metric names offered by the rule form.
//
// Every name the form offers is one the server accepts. __tests__/metrics.test.ts compares
// these lists with the server's own (server/internal/alert/validate.go KnownMetricNames,
// wave3.go supportedAnomalyMetrics), so the form cannot drift from the server again
// (D-194: it offered cpu_pct, mem_pct, packet_loss_pct, jitter_ms, rtt_ms and health_score
// for threshold rules — saved but never evaluated before v0.4.1, refused with 422 since —
// and did not offer node_cpu, node_mem, node_disk or stream_offline at all).

export type RuleType = "threshold" | "anomaly";

/** Threshold-rule metrics the form offers (server: alert.KnownMetricNames). */
export const THRESHOLD_METRICS: readonly string[] = [
  "viewer_count",
  "viewer_count_floor",
  "ingest_bitrate_kbps",
  "ingest_bitrate_floor",
  "stream_offline",
  "rebuffer_ratio",
  "error_rate",
  "node_cpu",
  "node_mem",
  "node_disk",
  "node_down",
  "node_degraded",
];

/**
 * Threshold-rule metrics the server accepts that the form does not offer, each for a reason.
 * They stay usable through the API, and a rule that already uses one shows it when edited.
 */
export const API_ONLY_THRESHOLD_METRICS: Readonly<Record<string, string>> = {
  cert_expiry: "needs the host:port to check in scope.stream_id",
  fps: "AMS 3.x does not report FPS (LIM-04), so the value is always 0",
  license_expiry: "watches a Pulse licence key, which Pulse no longer needs",
  viewer_drop_pct: "deprecated alias of viewer_count_floor",
};

/** Anomaly-rule metrics: what the Welford detector tracks (server: alert.SupportedAnomalyMetrics). */
export const ANOMALY_METRICS: readonly string[] = [
  "viewer_count",
  "ingest_bitrate_kbps",
  "cpu_pct",
  "mem_pct",
  "disk_pct",
  "ams_api_latency_ms",
];

// The node metrics carry different names in the two rule types.
const NODE_METRIC_PAIRS: ReadonlyArray<readonly [threshold: string, anomaly: string]> = [
  ["node_cpu", "cpu_pct"],
  ["node_mem", "mem_pct"],
  ["node_disk", "disk_pct"],
];

/** True when the server accepts `metric` for a rule of `ruleType`. */
export function isSupportedMetric(metric: string, ruleType: RuleType): boolean {
  if (ruleType === "anomaly") return ANOMALY_METRICS.includes(metric);
  return THRESHOLD_METRICS.includes(metric) || Object.hasOwn(API_ONLY_THRESHOLD_METRICS, metric);
}

/**
 * The metric to keep when the rule type changes: the same metric when the new type offers
 * it, its counterpart for node metrics (node_cpu ↔ cpu_pct …), otherwise the first offered.
 */
export function metricForRuleType(metric: string, ruleType: RuleType): string {
  const offered = ruleType === "anomaly" ? ANOMALY_METRICS : THRESHOLD_METRICS;
  if (offered.includes(metric)) return metric;
  const pair = NODE_METRIC_PAIRS.find(([t, a]) => (ruleType === "anomaly" ? t : a) === metric);
  if (pair) return ruleType === "anomaly" ? pair[1] : pair[0];
  return offered[0];
}
