/**
 * Audit Log (/audit-log) — D-102 Phase 2.
 *
 * Read-only view of the append-only audit trail: who changed what, when, and from
 * where, for every mutating admin/config API call. Backed by GET /admin/audit-log
 * (keyset pagination, newest-first). Admin-only via auth; no tier gate (the trail
 * is a core admin feature, not a paid capability).
 */
import { useState, useEffect, useCallback } from "react";
import type { CSSProperties } from "react";
import { adminApi, ApiError } from "@/api/client";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { ErrorBanner } from "@/components/ErrorBanner";
import { EmptyState } from "@/components/EmptyState";
import { Badge } from "@/components/Badge";
import type { AuditEntry } from "@/lib/api/types";

const PAGE_SIZE = 50;

function formatTs(ts: number): string {
  return new Date(ts).toLocaleString();
}

// Human label for the actor: prefer the token display name, fall back to a short
// token id, then to a dash. actor_user_id (OIDC) is surfaced in the row title.
function formatActor(e: AuditEntry): string {
  if (e.actor_name) return e.actor_name;
  if (e.actor_token_id) return `token ${e.actor_token_id.slice(0, 8)}…`;
  return "—";
}

const thStyle: CSSProperties = {
  padding: "var(--space-2) var(--space-3)",
  textAlign: "left",
  fontSize: 11,
  fontWeight: 600,
  color: "var(--color-secondary)",
  textTransform: "uppercase",
  letterSpacing: "0.06em",
};

const tdStyle: CSSProperties = {
  padding: "10px 12px",
  fontSize: 12,
  color: "var(--color-text)",
  borderBottom: "1px solid var(--color-border)",
};

const monoSecondary: CSSProperties = {
  fontFamily: "var(--font-mono)",
  color: "var(--color-secondary)",
  whiteSpace: "nowrap",
};

function AuditRow({ entry }: { entry: AuditEntry }) {
  const detailTitle = entry.detail ? JSON.stringify(entry.detail) : "";
  return (
    <tr>
      <td style={{ ...tdStyle, ...monoSecondary }}>{formatTs(entry.ts)}</td>
      <td style={tdStyle} title={entry.actor_user_id ? `user ${entry.actor_user_id}` : undefined}>
        {formatActor(entry)}
      </td>
      <td style={tdStyle}>
        <Badge label={entry.action} variant="info" />
      </td>
      <td style={{ ...tdStyle, color: "var(--color-secondary)" }}>{entry.object_type}</td>
      <td
        style={{
          ...tdStyle,
          fontFamily: "var(--font-mono)",
          color: "var(--color-secondary)",
          maxWidth: 240,
          overflow: "hidden",
          textOverflow: "ellipsis",
          whiteSpace: "nowrap",
        }}
        title={detailTitle ? `${entry.object_id ?? ""}  ${detailTitle}` : (entry.object_id ?? undefined)}
      >
        {entry.object_id || "—"}
      </td>
      <td style={{ ...tdStyle, ...monoSecondary }}>{entry.remote_addr || "—"}</td>
    </tr>
  );
}

export function AuditLogPage() {
  const [entries, setEntries] = useState<AuditEntry[]>([]);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [loaded, setLoaded] = useState(false);

  // cursor === null → fresh first page (replace); otherwise → append next page.
  const load = useCallback((cursor: string | null) => {
    const more = cursor !== null;
    if (more) setLoadingMore(true);
    else setLoading(true);
    setError(null);
    adminApi
      .listAuditLog({ limit: PAGE_SIZE, cursor: cursor ?? undefined })
      .then((res) => {
        setEntries((prev) => (more ? [...prev, ...res.items] : res.items));
        setNextCursor(res.meta?.next_cursor ?? null);
        setLoaded(true);
        setLoading(false);
        setLoadingMore(false);
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
        setLoadingMore(false);
      });
  }, []);

  useEffect(() => {
    load(null);
  }, [load]);

  return (
    <div style={{ maxWidth: 1100, margin: "0 auto" }}>
      <div style={{ display: "flex", alignItems: "center", gap: "var(--space-4)", marginBottom: 12 }}>
        <h1 style={{ flex: 1, fontSize: 20, fontWeight: 700, margin: 0 }}>Audit Log</h1>
        <button
          onClick={() => load(null)}
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

      <p style={{ fontSize: 13, color: "var(--color-secondary)", margin: "0 0 20px", maxWidth: 760 }}>
        Every change to alert rules &amp; channels, users, tokens, probes, report schedules, AMS sources,
        tenants and the licence is recorded here — who made it, when, and from where. Entries are
        append-only and newest first.
      </p>

      {error && (
        <div style={{ marginBottom: 16 }}>
          <ErrorBanner message={error} onRetry={() => load(null)} />
        </div>
      )}

      {loading && !error && (
        <div style={{ marginBottom: 16 }}>
          <LoadingSpinner label="Loading audit log…" />
        </div>
      )}

      {loaded && !loading && !error && entries.length === 0 && (
        <EmptyState
          title="No audit entries yet"
          description="Changes to admin and configuration resources will appear here as they happen — create or edit an alert rule, user, token or source to see the first entry."
        />
      )}

      {entries.length > 0 && (
        <div
          style={{
            background: "var(--color-surface)",
            border: "1px solid var(--color-border)",
            borderRadius: 8,
            overflow: "hidden",
          }}
        >
          <div style={{ overflowX: "auto" }}>
            <table
              style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}
              aria-label="Audit log"
            >
              <thead>
                <tr style={{ background: "var(--color-surface-2)" }}>
                  <th style={thStyle}>Time</th>
                  <th style={thStyle}>Actor</th>
                  <th style={thStyle}>Action</th>
                  <th style={thStyle}>Object</th>
                  <th style={thStyle}>Object ID</th>
                  <th style={thStyle}>Source IP</th>
                </tr>
              </thead>
              <tbody>
                {entries.map((e) => (
                  <AuditRow key={e.id} entry={e} />
                ))}
              </tbody>
            </table>
          </div>
          {nextCursor && (
            <div
              style={{
                padding: "var(--space-3)",
                borderTop: "1px solid var(--color-border)",
                textAlign: "center",
              }}
            >
              <button
                onClick={() => load(nextCursor)}
                disabled={loadingMore}
                style={{
                  background: "var(--color-surface-2)",
                  color: "var(--color-text)",
                  border: "1px solid var(--color-border)",
                  borderRadius: 6,
                  padding: "6px 16px",
                  fontSize: 13,
                  fontWeight: 600,
                  cursor: loadingMore ? "not-allowed" : "pointer",
                  opacity: loadingMore ? 0.7 : 1,
                }}
              >
                {loadingMore ? "Loading…" : "Load more"}
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
