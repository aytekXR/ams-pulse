/**
 * F9 — Anomaly flags (/anomalies)
 *
 * List of baseline-deviation anomaly flags: metric, scope (node/app/stream),
 * observed value, expected (baseline mean), sigma (z-score), detection time.
 * Severity-ish styling by sigma magnitude (≥4 = high, ≥3 = medium, ≥2 = low).
 * Empty state: "no anomalies — baselines still learning" when nothing is returned.
 * Optional sigma-sensitivity selector (min_sigma query param).
 * Gate-aware: Enterprise-tier upsell when not entitled.
 */
import { useState, useEffect, useCallback } from "react";
import { anomaliesApi, adminApi, ApiError } from "@/api/client";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { ErrorBanner } from "@/components/ErrorBanner";
import { EmptyState } from "@/components/EmptyState";
import { Badge } from "@/components/Badge";
import { TierGate } from "@/components/TierGate";
import type { AnomalyFlag, LicenseInfo, AlertScope } from "@/lib/api/types";

// ─── Sigma severity ───────────────────────────────────────────────────────────

function sigmaSeverity(sigma: number): "error" | "warning" | "info" {
  if (sigma >= 4) return "error";
  if (sigma >= 3) return "warning";
  return "info";
}

function sigmaLabel(sigma: number): string {
  if (sigma >= 4) return "high";
  if (sigma >= 3) return "medium";
  return "low";
}

// ─── Scope display ────────────────────────────────────────────────────────────

function formatScope(scope: AlertScope): string {
  const parts: string[] = [];
  if (scope.node_id) parts.push(`node:${scope.node_id}`);
  if (scope.app) parts.push(`app:${scope.app}`);
  if (scope.stream_id) parts.push(`stream:${scope.stream_id}`);
  return parts.length > 0 ? parts.join(", ") : "global";
}

function formatTs(ts: number): string {
  return new Date(ts).toLocaleString();
}

// ─── Sigma selector ───────────────────────────────────────────────────────────

const SIGMA_OPTIONS = [
  { label: "All (σ ≥ 2)", value: 2 },
  { label: "Medium+ (σ ≥ 3)", value: 3 },
  { label: "High (σ ≥ 4)", value: 4 },
];

// ─── Anomaly row ─────────────────────────────────────────────────────────────

interface AnomalyRowProps {
  flag: AnomalyFlag;
}

function AnomalyRow({ flag }: AnomalyRowProps) {
  const severity = sigmaSeverity(flag.sigma);
  const label = sigmaLabel(flag.sigma);
  const sigmaAbs = Math.abs(flag.sigma);

  const delta = flag.observed - flag.expected;
  const deltaSign = delta >= 0 ? "+" : "";

  return (
    <tr
      style={{
        borderBottom: "1px solid var(--color-border)",
        background: severity === "error" ? "rgba(255,92,104,0.04)" : "transparent",
      }}
    >
      {/* Metric */}
      <td
        style={{
          padding: "10px 12px",
          fontFamily: "var(--font-mono)",
          fontSize: 12,
          fontWeight: 600,
          color: "var(--color-text)",
        }}
      >
        {flag.metric}
      </td>
      {/* Scope */}
      <td
        style={{
          padding: "10px 12px",
          fontSize: 12,
          color: "var(--color-muted)",
          maxWidth: 200,
          overflow: "hidden",
          textOverflow: "ellipsis",
          whiteSpace: "nowrap",
        }}
        title={formatScope(flag.scope)}
      >
        {formatScope(flag.scope)}
      </td>
      {/* Observed / Expected */}
      <td style={{ padding: "10px 12px", fontSize: 12, textAlign: "right" }}>
        <span style={{ color: "var(--color-text)", fontWeight: 600 }}>
          {flag.observed.toFixed(2)}
        </span>
        <span
          style={{
            fontSize: 11,
            color: "var(--color-muted)",
            marginLeft: 4,
          }}
        >
          (expected {flag.expected.toFixed(2)})
        </span>
      </td>
      {/* Delta */}
      <td
        style={{
          padding: "10px 12px",
          fontSize: 12,
          textAlign: "right",
          color:
            delta > 0
              ? "var(--color-error)"
              : delta < 0
                ? "var(--color-info)"
                : "var(--color-muted)",
          fontFamily: "var(--font-mono)",
        }}
      >
        {deltaSign}
        {delta.toFixed(2)}
      </td>
      {/* Sigma */}
      <td style={{ padding: "10px 12px", textAlign: "right" }}>
        <span
          style={{
            fontSize: 13,
            fontWeight: 700,
            fontFamily: "var(--font-mono)",
            color:
              severity === "error"
                ? "var(--color-error)"
                : severity === "warning"
                  ? "var(--color-warning)"
                  : "var(--color-info)",
          }}
          title={`${sigmaAbs.toFixed(2)}σ deviation`}
        >
          {sigmaAbs.toFixed(2)}σ
        </span>
      </td>
      {/* Severity badge */}
      <td style={{ padding: "10px 12px" }}>
        <Badge label={label} variant={severity} />
      </td>
      {/* Detection time */}
      <td
        style={{
          padding: "10px 12px",
          fontSize: 11,
          color: "var(--color-muted)",
          whiteSpace: "nowrap",
        }}
      >
        {formatTs(flag.ts)}
      </td>
    </tr>
  );
}

// ─── Main page ────────────────────────────────────────────────────────────────

export function AnomaliesPage() {
  const [license, setLicense] = useState<LicenseInfo | null>(null);
  const [licenseLoading, setLicenseLoading] = useState(true);
  const [licenseError, setLicenseError] = useState<string | null>(null);

  const [flags, setFlags] = useState<AnomalyFlag[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [minSigma, setMinSigma] = useState<number>(2);

  // Load license tier on mount
  useEffect(() => {
    let cancelled = false;
    setLicenseLoading(true);
    adminApi
      .getLicense()
      .then((lic) => {
        if (!cancelled) {
          setLicense(lic);
          setLicenseLoading(false);
        }
      })
      .catch((err) => {
        if (!cancelled) {
          setLicenseError(err instanceof Error ? err.message : "Failed to load license");
          setLicenseLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const fetchAnomalies = useCallback(() => {
    setLoading(true);
    setError(null);
    anomaliesApi
      .list({ min_sigma: minSigma, limit: 100 })
      .then((res) => {
        setFlags(res.items);
        setLoading(false);
      })
      .catch((err) => {
        const msg =
          err instanceof ApiError
            ? `API error ${err.status}: ${err.message}`
            : err instanceof Error
              ? err.message
              : "Unknown error";
        setError(msg);
        setLoading(false);
      });
  }, [minSigma]);

  // Fetch anomalies once license is confirmed as enterprise
  useEffect(() => {
    if (license?.tier === "enterprise") {
      fetchAnomalies();
    }
  }, [license, fetchAnomalies]);

  if (licenseLoading) {
    return <LoadingSpinner label="Loading license…" />;
  }

  if (licenseError) {
    return <ErrorBanner message={licenseError} />;
  }

  // Gate: only Enterprise
  if (license && license.tier !== "enterprise") {
    return (
      <div style={{ maxWidth: 700, margin: "0 auto", paddingTop: 40 }}>
        <h1 style={{ fontSize: 20, fontWeight: 700, margin: "0 0 24px" }}>Anomaly Detection</h1>
        <TierGate
          icon={
            <svg
              width="48"
              height="48"
              viewBox="0 0 24 24"
              fill="none"
              stroke="var(--color-accent)"
              strokeWidth="1.5"
              aria-hidden
            >
              <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
              <line x1="12" y1="9" x2="12" y2="13" />
              <line x1="12" y1="17" x2="12.01" y2="17" />
            </svg>
          }
          heading="Anomaly Detection requires Enterprise tier"
          tier={license.tier}
          upgradeText="Upgrade to Enterprise to unlock anomaly detection, baseline learning, and deviation alerts."
        />
      </div>
    );
  }

  return (
    <div style={{ maxWidth: 1100, margin: "0 auto" }}>
      {/* Page header */}
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 16,
          marginBottom: 20,
        }}
      >
        <h1 style={{ flex: 1, fontSize: 20, fontWeight: 700, margin: 0 }}>
          Anomaly Detection
        </h1>
        {/* Sigma selector */}
        <div style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 13 }}>
          <label
            htmlFor="min-sigma-select"
            style={{ color: "var(--color-muted)", fontWeight: 500 }}
          >
            Sensitivity:
          </label>
          <select
            id="min-sigma-select"
            value={minSigma}
            onChange={(e) => setMinSigma(Number(e.target.value))}
            style={{
              background: "var(--color-surface)",
              border: "1px solid var(--color-border)",
              borderRadius: 4,
              color: "var(--color-text)",
              padding: "4px 8px",
              fontSize: 13,
              cursor: "pointer",
            }}
            aria-label="Minimum sigma threshold"
          >
            {SIGMA_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
        </div>
        <button
          onClick={fetchAnomalies}
          disabled={loading}
          style={{
            background: "var(--color-accent)",
            color: "var(--color-on-signal)",
            border: "none",
            borderRadius: 6,
            padding: "6px 14px",
            fontSize: 13,
            fontWeight: 600,
            cursor: loading ? "not-allowed" : "pointer",
            opacity: loading ? 0.7 : 1,
          }}
        >
          Refresh
        </button>
      </div>

      {error && (
        <div style={{ marginBottom: 16 }}>
          <ErrorBanner message={error} onRetry={fetchAnomalies} />
        </div>
      )}

      {loading && !error && (
        <div style={{ marginBottom: 16 }}>
          <LoadingSpinner label="Loading anomaly flags…" />
        </div>
      )}

      {!loading && !error && flags.length === 0 && (
        <EmptyState
          title="No anomalies detected"
          description="Baselines are still learning — anomaly flags appear once enough samples have been collected (typically a few hours of traffic). No action needed while baselines are building."
        />
      )}

      {!loading && flags.length > 0 && (
        <div
          style={{
            background: "var(--color-surface)",
            border: "1px solid var(--color-border)",
            borderRadius: 8,
            overflow: "hidden",
          }}
        >
          <div style={{ padding: "12px 16px", borderBottom: "1px solid var(--color-border)", fontSize: 12, color: "var(--color-muted)" }}>
            {flags.length} flag{flags.length !== 1 ? "s" : ""} · sensitivity σ ≥ {minSigma}
          </div>
          <div style={{ overflowX: "auto" }}>
            <table
              style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}
              aria-label="Anomaly flags table"
            >
              <thead>
                <tr style={{ background: "var(--color-surface-2)" }}>
                  <th
                    style={{
                      padding: "8px 12px",
                      textAlign: "left",
                      fontSize: 11,
                      fontWeight: 600,
                      color: "var(--color-muted)",
                      textTransform: "uppercase",
                      letterSpacing: "0.06em",
                    }}
                  >
                    Metric
                  </th>
                  <th
                    style={{
                      padding: "8px 12px",
                      textAlign: "left",
                      fontSize: 11,
                      fontWeight: 600,
                      color: "var(--color-muted)",
                      textTransform: "uppercase",
                      letterSpacing: "0.06em",
                    }}
                  >
                    Scope
                  </th>
                  <th
                    style={{
                      padding: "8px 12px",
                      textAlign: "right",
                      fontSize: 11,
                      fontWeight: 600,
                      color: "var(--color-muted)",
                      textTransform: "uppercase",
                      letterSpacing: "0.06em",
                    }}
                  >
                    Observed / Expected
                  </th>
                  <th
                    style={{
                      padding: "8px 12px",
                      textAlign: "right",
                      fontSize: 11,
                      fontWeight: 600,
                      color: "var(--color-muted)",
                      textTransform: "uppercase",
                      letterSpacing: "0.06em",
                    }}
                  >
                    Delta
                  </th>
                  <th
                    style={{
                      padding: "8px 12px",
                      textAlign: "right",
                      fontSize: 11,
                      fontWeight: 600,
                      color: "var(--color-muted)",
                      textTransform: "uppercase",
                      letterSpacing: "0.06em",
                    }}
                  >
                    Sigma
                  </th>
                  <th
                    style={{
                      padding: "8px 12px",
                      textAlign: "left",
                      fontSize: 11,
                      fontWeight: 600,
                      color: "var(--color-muted)",
                      textTransform: "uppercase",
                      letterSpacing: "0.06em",
                    }}
                  >
                    Severity
                  </th>
                  <th
                    style={{
                      padding: "8px 12px",
                      textAlign: "left",
                      fontSize: 11,
                      fontWeight: 600,
                      color: "var(--color-muted)",
                      textTransform: "uppercase",
                      letterSpacing: "0.06em",
                    }}
                  >
                    Detected At
                  </th>
                </tr>
              </thead>
              <tbody>
                {flags.map((flag) => (
                  <AnomalyRow key={flag.id} flag={flag} />
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}
