import { useState, useEffect, useCallback } from "react";
import { alertsApi, ApiError } from "@/api/client";
import { AlertRuleForm } from "./AlertRuleForm";
import { AlertChannelForm } from "./AlertChannelForm";
import { Badge } from "@/components/Badge";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { ErrorBanner } from "@/components/ErrorBanner";
import { EmptyState } from "@/components/EmptyState";
import { useToast } from "@/components/Toast";
import type {
  AlertRule,
  AlertChannel,
  AlertHistoryEntry,
  AlertRuleWrite,
  AlertChannelWrite,
} from "@/lib/api/types";

type Tab = "rules" | "channels" | "history";

type SeverityVariant = "info" | "warning" | "error" | "muted" | "default" | "success";

function severityVariant(s: string | undefined): SeverityVariant {
  switch (s) {
    case "info": return "info";
    case "warning": return "warning";
    case "critical": return "error";
    default: return "muted";
  }
}

function stateVariant(s: string | undefined): SeverityVariant {
  switch (s) {
    case "firing": return "error";
    case "resolved": return "success";
    default: return "info";
  }
}

function fmtTs(ts: number | null | undefined): string {
  if (!ts) return "—";
  return new Date(ts).toLocaleString();
}

/** Display name for a rule — uses the required name field (CR-1). */
function ruleDisplayName(rule: AlertRule): string {
  return rule.name;
}

export function AlertsPage() {
  const { toast } = useToast();
  const [tab, setTab] = useState<Tab>("rules");
  const [rules, setRules] = useState<AlertRule[]>([]);
  const [channels, setChannels] = useState<AlertChannel[]>([]);
  const [history, setHistory] = useState<AlertHistoryEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [editingRule, setEditingRule] = useState<AlertRule | null | "new">(null);
  const [editingChannel, setEditingChannel] = useState<AlertChannel | null | "new">(null);
  const [testingChannel, setTestingChannel] = useState<string | null>(null);

  const loadAll = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [rulesData, channelsData, histData] = await Promise.all([
        alertsApi.getRules(),
        alertsApi.getChannels(),
        alertsApi.getHistory({ limit: 100 }),
      ]);
      // responses use `items` per generated schema (AlertRuleList, AlertChannelList, AlertHistoryList)
      setRules(rulesData.items ?? []);
      setChannels(channelsData.items ?? []);
      setHistory(histData.items ?? []);
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : "Failed to load alerts";
      setError(msg);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadAll();
  }, [loadAll]);

  const saveRule = async (data: AlertRuleWrite) => {
    if (editingRule === "new") {
      await alertsApi.createRule(data);
      toast("Rule created", "success");
    } else if (editingRule) {
      await alertsApi.updateRule(editingRule.id, data);
      toast("Rule updated", "success");
    }
    setEditingRule(null);
    void loadAll();
  };

  const deleteRule = async (id: string) => {
    if (!confirm("Delete this alert rule?")) return;
    await alertsApi.deleteRule(id);
    toast("Rule deleted", "info");
    void loadAll();
  };

  const saveChannel = async (data: AlertChannelWrite) => {
    if (editingChannel === "new") {
      await alertsApi.createChannel(data);
      toast("Channel created", "success");
    } else if (editingChannel) {
      await alertsApi.updateChannel(editingChannel.id, data);
      toast("Channel updated", "success");
    }
    setEditingChannel(null);
    void loadAll();
  };

  const deleteChannel = async (id: string) => {
    if (!confirm("Delete this channel?")) return;
    await alertsApi.deleteChannel(id);
    toast("Channel deleted", "info");
    void loadAll();
  };

  const testChannel = async (id: string) => {
    setTestingChannel(id);
    try {
      const result = await alertsApi.testChannel(id);
      // ChannelTestResult: { accepted: boolean; message?: string | null }
      if (result.accepted) {
        toast(`Test sent${result.message ? `: ${result.message}` : ""}`, "success");
      } else {
        toast(`Test failed${result.message ? `: ${result.message}` : ""}`, "error");
      }
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : "Test failed";
      toast(msg, "error");
    } finally {
      setTestingChannel(null);
    }
  };

  const btnStyle: React.CSSProperties = {
    background: "var(--color-accent)",
    border: "none",
    color: "#fff",
    borderRadius: 6,
    padding: "7px 14px",
    cursor: "pointer",
    fontSize: 12,
    fontWeight: 600,
  };

  const smBtnStyle: React.CSSProperties = {
    background: "var(--color-surface-2)",
    border: "1px solid var(--color-border)",
    color: "var(--color-muted)",
    borderRadius: 4,
    padding: "4px 10px",
    cursor: "pointer",
    fontSize: 11,
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
        <h1 style={{ margin: 0, fontSize: 18, fontWeight: 700 }}>Alerts</h1>
        {tab === "rules" && (
          <button style={btnStyle} onClick={() => setEditingRule("new")}>+ New rule</button>
        )}
        {tab === "channels" && (
          <button style={btnStyle} onClick={() => setEditingChannel("new")}>+ New channel</button>
        )}
      </div>

      {/* Tabs */}
      <div style={{ display: "flex", gap: 0, borderBottom: "1px solid var(--color-border)" }}>
        {(["rules", "channels", "history"] as Tab[]).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            style={{
              background: "none",
              border: "none",
              borderBottom: `2px solid ${tab === t ? "var(--color-accent)" : "transparent"}`,
              color: tab === t ? "var(--color-text)" : "var(--color-muted)",
              padding: "8px 16px",
              cursor: "pointer",
              fontSize: 13,
              fontWeight: tab === t ? 600 : 400,
              textTransform: "capitalize",
            }}
          >
            {t}
          </button>
        ))}
      </div>

      {error && <ErrorBanner message={error} onRetry={loadAll} />}

      {/* Rule form */}
      {editingRule !== null && (
        <div
          style={{
            background: "var(--color-surface)",
            border: "1px solid var(--color-border)",
            borderRadius: 10,
            padding: 24,
          }}
        >
          <AlertRuleForm
            initial={editingRule === "new" ? undefined : editingRule}
            onSave={saveRule}
            onCancel={() => setEditingRule(null)}
          />
        </div>
      )}

      {/* Channel form */}
      {editingChannel !== null && (
        <div
          style={{
            background: "var(--color-surface)",
            border: "1px solid var(--color-border)",
            borderRadius: 10,
            padding: 24,
          }}
        >
          <AlertChannelForm
            initial={editingChannel === "new" ? undefined : editingChannel}
            onSave={saveChannel}
            onCancel={() => setEditingChannel(null)}
          />
        </div>
      )}

      {loading ? (
        <LoadingSpinner />
      ) : (
        <>
          {tab === "rules" && (
            rules.length === 0 ? (
              <EmptyState
                title="No alert rules"
                description="Create a rule to start monitoring your streams and infrastructure."
                action={<button style={btnStyle} onClick={() => setEditingRule("new")}>Create first rule</button>}
              />
            ) : (
              <div
                style={{
                  background: "var(--color-surface)",
                  border: "1px solid var(--color-border)",
                  borderRadius: 8,
                  overflow: "hidden",
                }}
              >
                {rules.map((rule, i) => (
                  <div
                    key={rule.id}
                    style={{
                      display: "flex",
                      alignItems: "center",
                      gap: 12,
                      padding: "12px 16px",
                      borderTop: i === 0 ? "none" : "1px solid var(--color-border)",
                    }}
                  >
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div style={{ fontWeight: 600, fontSize: 13 }}>{ruleDisplayName(rule)}</div>
                      <div style={{ fontSize: 12, color: "var(--color-muted)", marginTop: 2 }}>
                        {rule.metric} {rule.operator} {rule.threshold} · window {rule.window_s}s · cooldown {rule.cooldown_s}s
                      </div>
                    </div>
                    <Badge label={rule.severity} variant={severityVariant(rule.severity)} />
                    {!rule.enabled && <Badge label="disabled" variant="muted" />}
                    {rule.enabled && rule.muted && <Badge label="muted" variant="muted" />}
                    <button style={smBtnStyle} onClick={() => setEditingRule(rule)}>Edit</button>
                    <button
                      style={{ ...smBtnStyle, color: "var(--color-error)", borderColor: "var(--color-error)" }}
                      onClick={() => void deleteRule(rule.id)}
                    >
                      Delete
                    </button>
                  </div>
                ))}
              </div>
            )
          )}

          {tab === "channels" && (
            channels.length === 0 ? (
              <EmptyState
                title="No notification channels"
                description="Add a channel to receive alerts via email, Slack, or webhook."
                action={<button style={btnStyle} onClick={() => setEditingChannel("new")}>Add channel</button>}
              />
            ) : (
              <div
                style={{
                  background: "var(--color-surface)",
                  border: "1px solid var(--color-border)",
                  borderRadius: 8,
                  overflow: "hidden",
                }}
              >
                {channels.map((ch, i) => (
                  <div
                    key={ch.id}
                    style={{
                      display: "flex",
                      alignItems: "center",
                      gap: 12,
                      padding: "12px 16px",
                      borderTop: i === 0 ? "none" : "1px solid var(--color-border)",
                    }}
                  >
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div style={{ fontWeight: 600, fontSize: 13 }}>{ch.name}</div>
                      <div style={{ fontSize: 12, color: "var(--color-muted)", marginTop: 2 }}>{ch.type}</div>
                    </div>
                    <Badge label={ch.type} variant="info" />
                    <button
                      style={{ ...smBtnStyle, color: "var(--color-accent-hover)", borderColor: "var(--color-accent)" }}
                      onClick={() => void testChannel(ch.id)}
                      disabled={testingChannel === ch.id}
                    >
                      {testingChannel === ch.id ? "Sending…" : "Test fire"}
                    </button>
                    <button style={smBtnStyle} onClick={() => setEditingChannel(ch)}>Edit</button>
                    <button
                      style={{ ...smBtnStyle, color: "var(--color-error)", borderColor: "var(--color-error)" }}
                      onClick={() => void deleteChannel(ch.id)}
                    >
                      Delete
                    </button>
                  </div>
                ))}
              </div>
            )
          )}

          {tab === "history" && (
            history.length === 0 ? (
              <EmptyState
                title="No alert history"
                description="Fired alerts will appear here."
              />
            ) : (
              <div
                style={{
                  background: "var(--color-surface)",
                  border: "1px solid var(--color-border)",
                  borderRadius: 8,
                  overflow: "hidden",
                }}
              >
                <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
                  <thead style={{ background: "var(--color-surface-2)" }}>
                    <tr>
                      {["Rule ID", "Severity", "State", "Time", "Value"].map((h) => (
                        <th key={h} style={{ padding: "10px 14px", textAlign: "left", fontSize: 11, color: "var(--color-muted)", textTransform: "uppercase", letterSpacing: "0.06em", fontWeight: 600 }}>{h}</th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {history.map((entry) => (
                      <tr key={entry.id} style={{ borderTop: "1px solid var(--color-border)" }}>
                        <td style={{ padding: "8px 14px", fontWeight: 500, fontFamily: "var(--font-mono)", fontSize: 12 }}>{entry.rule_id}</td>
                        <td style={{ padding: "8px 14px" }}><Badge label={entry.severity} variant={severityVariant(entry.severity)} /></td>
                        <td style={{ padding: "8px 14px" }}><Badge label={entry.state} variant={stateVariant(entry.state)} /></td>
                        <td style={{ padding: "8px 14px", color: "var(--color-muted)", fontFamily: "var(--font-mono)", fontSize: 12 }}>{fmtTs(entry.ts)}</td>
                        <td style={{ padding: "8px 14px", fontFamily: "var(--font-mono)", fontSize: 12 }}>{entry.value != null ? String(entry.value) : "—"}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )
          )}
        </>
      )}
    </div>
  );
}
