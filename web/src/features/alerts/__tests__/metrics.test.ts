/**
 * The rule form's metric lists must match the server's (D-194).
 *
 * The form once offered six threshold metrics the server never evaluated (and, since v0.4.1,
 * refuses with 422) while omitting node_cpu, node_mem, node_disk and stream_offline. These
 * tests read the server's own lists from the Go source, so a metric added to or removed from
 * either side fails here until the form makes a deliberate choice about it.
 */
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, it, expect } from "vitest";
import {
  ANOMALY_METRICS,
  API_ONLY_THRESHOLD_METRICS,
  THRESHOLD_METRICS,
  isSupportedMetric,
  metricForRuleType,
} from "../metrics";

const here = dirname(fileURLToPath(import.meta.url));
const alertPkg = resolve(here, "../../../../../server/internal/alert");

/** The string literals inside the first block of `src` that starts with `opener`. */
function goStringsAfter(src: string, opener: RegExp, entry: RegExp): string[] {
  const start = src.search(opener);
  expect(start, `marker ${opener} not found in the Go source`).toBeGreaterThanOrEqual(0);
  const body = src.slice(start).match(/\{([\s\S]*?)\n\}/);
  expect(body, `no block after ${opener}`).not.toBeNull();
  return [...body![1].matchAll(entry)].map((m) => m[1]);
}

const serverThreshold = goStringsAfter(
  readFileSync(resolve(alertPkg, "validate.go"), "utf-8"),
  /var KnownMetricNames = func\(\) \[\]string \{/,
  /^\s*"([a-z0-9_]+)",/gm,
);
const serverAnomaly = goStringsAfter(
  readFileSync(resolve(alertPkg, "wave3.go"), "utf-8"),
  /var supportedAnomalyMetrics = map\[string\]bool\{/,
  /^\s*"([a-z0-9_]+)":\s*true,/gm,
);

const sorted = (xs: readonly string[]) => [...xs].sort();

describe("alert metrics — pinned to the server's lists", () => {
  it("read a plausible list from each Go file (guards the parser itself)", () => {
    expect(serverThreshold).toContain("node_cpu");
    expect(serverThreshold.length).toBeGreaterThanOrEqual(10);
    expect(serverAnomaly).toContain("cpu_pct");
    expect(serverAnomaly.length).toBeGreaterThanOrEqual(5);
  });

  it("every threshold metric the server accepts is offered or API-only for a stated reason", () => {
    const formSide = [...THRESHOLD_METRICS, ...Object.keys(API_ONLY_THRESHOLD_METRICS)];
    expect(sorted(formSide)).toEqual(sorted(serverThreshold));
  });

  it("the offered and API-only threshold lists do not overlap or repeat", () => {
    const offered = new Set(THRESHOLD_METRICS);
    expect(offered.size).toBe(THRESHOLD_METRICS.length);
    for (const m of Object.keys(API_ONLY_THRESHOLD_METRICS)) expect(offered.has(m)).toBe(false);
  });

  it("the anomaly list is exactly what the detector supports", () => {
    expect(sorted(ANOMALY_METRICS)).toEqual(sorted(serverAnomaly));
  });

  it("none of the six names the server refused is offered for threshold rules", () => {
    for (const m of ["cpu_pct", "mem_pct", "packet_loss_pct", "jitter_ms", "rtt_ms", "health_score"]) {
      expect(THRESHOLD_METRICS).not.toContain(m);
      expect(isSupportedMetric(m, "threshold")).toBe(false);
    }
  });
});

describe("alert metrics — helpers", () => {
  it("isSupportedMetric follows the server per rule type", () => {
    expect(isSupportedMetric("node_cpu", "threshold")).toBe(true);
    expect(isSupportedMetric("cert_expiry", "threshold")).toBe(true); // API-only, still valid
    expect(isSupportedMetric("node_cpu", "anomaly")).toBe(false);
    expect(isSupportedMetric("cpu_pct", "anomaly")).toBe(true);
  });

  it("metricForRuleType maps node metrics across rule types and keeps shared ones", () => {
    expect(metricForRuleType("node_cpu", "anomaly")).toBe("cpu_pct");
    expect(metricForRuleType("mem_pct", "threshold")).toBe("node_mem");
    expect(metricForRuleType("disk_pct", "threshold")).toBe("node_disk");
    expect(metricForRuleType("viewer_count", "anomaly")).toBe("viewer_count");
    expect(metricForRuleType("ingest_bitrate_kbps", "threshold")).toBe("ingest_bitrate_kbps");
    expect(metricForRuleType("ams_api_latency_ms", "threshold")).toBe(THRESHOLD_METRICS[0]);
    expect(metricForRuleType("stream_offline", "anomaly")).toBe(ANOMALY_METRICS[0]);
  });
});
