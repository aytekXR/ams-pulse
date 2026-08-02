import { useState, useEffect, useCallback, useRef } from "react";
import { alertsApi, ApiError } from "@/api/client";
import { AlertRuleForm } from "./AlertRuleForm";
import { AlertChannelForm } from "./AlertChannelForm";
import { Badge } from "@/components/Badge";
import { Tabs } from "@/components/Tabs";
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
  // Confirmation step for destructive rule deletion (replaces window.confirm).
  const [confirmDeleteRuleId, setConfirmDeleteRuleId] = useState<string | null>(null);
  const [confirmDeleteChannelId, setConfirmDeleteChannelId] = useState<string | null>(null);

  // s111 D15: the confirmation renders INSIDE the affected row (the old
  // top-of-list strip could sit off-screen for row 40 of a long list, named no
  // rule, and never moved keyboard focus). Focus lands on "Yes, delete" when a
  // confirmation opens and returns to the row's Delete button on cancel.
  const confirmRuleBtnRef = useRef<HTMLButtonElement | null>(null);
  const ruleDeleteBtnRefs = useRef<Record<string, HTMLButtonElement | null>>({});
  const confirmChannelBtnRef = useRef<HTMLButtonElement | null>(null);
  const channelDeleteBtnRefs = useRef<Record<string, HTMLButtonElement | null>>({});

  useEffect(() => {
    if (confirmDeleteRuleId) confirmRuleBtnRef.current?.focus();
  }, [confirmDeleteRuleId]);

  useEffect(() => {
    if (confirmDeleteChannelId) confirmChannelBtnRef.current?.focus();
  }, [confirmDeleteChannelId]);

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

  /** Initiates the delete confirmation step; does NOT call the API directly. */
  const requestDeleteRule = (id: string) => {
    setConfirmDeleteRuleId(id);
  };

  /** Called when the user confirms deletion in the inline confirmation UI. */
  const confirmDeleteRule = async () => {
    if (!confirmDeleteRuleId) return;
    await alertsApi.deleteRule(confirmDeleteRuleId);
    toast("Rule deleted", "info");
    setConfirmDeleteRuleId(null);
    void loadAll();
  };

  const cancelDeleteRule = () => {
    const id = confirmDeleteRuleId;
    setConfirmDeleteRuleId(null);
    if (id) ruleDeleteBtnRefs.current[id]?.focus();
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

  /**
   * Channel deletion mirrors the rule-deletion flow above.
   *
   * It used to call the native `confirm()`. Wave 4 replaced that with an inline confirmation
   * step for RULES but missed CHANNELS, so the page had two different confirmation models for
   * the same destructive verb (found by the S34 e2e pass). Native confirm() is worth removing
   * on its own terms: it is unstyleable, it blocks the event loop, browsers increasingly
   * suppress it, and — the reason it survived this long undetected — jsdom stubs it, so unit
   * tests never saw a dialog at all.
   */
  const confirmDeleteChannel = async () => {
    if (!confirmDeleteChannelId) return;
    await alertsApi.deleteChannel(confirmDeleteChannelId);
    toast("Channel deleted", "info");
    setConfirmDeleteChannelId(null);
    void loadAll();
  };

  const cancelDeleteChannel = () => {
    const id = confirmDeleteChannelId;
    setConfirmDeleteChannelId(null);
    if (id) channelDeleteBtnRefs.current[id]?.focus();
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

  // s111 D7: pair with className="btn-primary" — the class owns background
  // (accent -> accent-hover on hover); re-adding it inline would kill hover.
  const btnStyle: React.CSSProperties = {
    border: "none",
    color: "var(--color-on-signal)",
    borderRadius: "var(--radius-control)",
    padding: "var(--space-2) var(--space-4)",
    cursor: "pointer",
    fontSize: 12,
    fontWeight: 600,
  };

  // s111 D7/D14: pair with className="btn-secondary" — the class owns
  // color/border (destructive variants override them inline, deliberately
  // keeping their fixed error colour). 28px = the audit's desktop-pointer
  // floor for small row controls.
  const smBtnStyle: React.CSSProperties = {
    background: "var(--color-surface-2)",
    borderRadius: "var(--radius-control)",
    padding: "6px 10px",
    minHeight: 28,
    cursor: "pointer",
    fontSize: 11,
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-5)" }}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
        <h1 className="page-title">Alerts</h1>
        {/* s111 D20: '+' glyph dropped from the labels — a font character
            standing in for an icon drifts in weight against the app's drawn
            2px-stroke SVG system, and "New rule" carries the meaning alone. */}
        {tab === "rules" && (
          <button className="btn-primary" style={btnStyle} onClick={() => setEditingRule("new")}>New rule</button>
        )}
        {tab === "channels" && (
          <button className="btn-primary" style={btnStyle} onClick={() => setEditingChannel("new")}>New channel</button>
        )}
      </div>

      {/* Tabs — shared component emits id="tab-{id}" on each button */}
      <Tabs
        tabs={[
          { id: "rules", label: "Rules" },
          { id: "channels", label: "Channels" },
          { id: "history", label: "History" },
        ]}
        activeTab={tab}
        onTabChange={(id) => setTab(id as Tab)}
      />

      {error && <ErrorBanner message={error} onRetry={loadAll} />}

      {/* Rule form — s111 M5: .panel-enter fades the conditional mount in
          (200ms, "fade never slide"); exit stays instant. */}
      {editingRule !== null && (
        <div
          className="panel-enter"
          style={{
            background: "var(--color-surface)",
            border: "1px solid var(--color-border)",
            borderRadius: "var(--radius-card)",
            padding: "var(--space-5)",
          }}
        >
          <AlertRuleForm
            initial={editingRule === "new" ? undefined : editingRule}
            onSave={saveRule}
            onCancel={() => setEditingRule(null)}
          />
        </div>
      )}

      {/* Channel form — s111 M5: same entrance as the rule form. */}
      {editingChannel !== null && (
        <div
          className="panel-enter"
          style={{
            background: "var(--color-surface)",
            border: "1px solid var(--color-border)",
            borderRadius: "var(--radius-card)",
            padding: "var(--space-5)",
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
          {/* Rules panel — aria-labelledby references the id="tab-rules" button emitted by <Tabs> */}
          {tab === "rules" && (
            <div role="tabpanel" id="panel-rules" aria-labelledby="tab-rules">
              {rules.length === 0 ? (
                <EmptyState
                  title="No alert rules"
                  description="Create a rule to start monitoring your streams and infrastructure."
                  action={<button className="btn-primary" style={btnStyle} onClick={() => setEditingRule("new")}>Create first rule</button>}
                />
              ) : (
                <div
                  style={{
                    background: "var(--color-surface)",
                    border: "1px solid var(--color-border)",
                    borderRadius: "var(--radius-card)",
                    overflow: "hidden",
                  }}
                >
                  {rules.map((rule, i) => (
                    <div
                      key={rule.id}
                      style={{
                        display: "flex",
                        alignItems: "center",
                        gap: "var(--space-3)",
                        padding: "var(--space-3) var(--space-4)",
                        borderTop: i === 0 ? "none" : "1px solid var(--color-border)",
                      }}
                    >
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <div style={{ fontWeight: 600, fontSize: 13 }}>{ruleDisplayName(rule)}</div>
                        <div style={{ fontSize: 12, color: "var(--color-secondary)", marginTop: 2 }}>
                          {rule.metric} {rule.operator} {rule.threshold} · window {rule.window_s}s · cooldown {rule.cooldown_s}s
                        </div>
                      </div>
                      <Badge label={rule.severity} variant={severityVariant(rule.severity)} />
                      {!rule.enabled && <Badge label="disabled" variant="muted" />}
                      {rule.enabled && rule.muted && <Badge label="muted" variant="muted" />}
                      {/* s111 D15: confirmation replaces THIS row's actions —
                          attached to the object it deletes, names the rule, and
                          manages focus (see the refs above). */}
                      {confirmDeleteRuleId === rule.id ? (
                        <div
                          data-testid="delete-rule-confirm"
                          role="group"
                          aria-label={`Confirm deleting rule ${ruleDisplayName(rule)}`}
                          className="panel-enter"
                          style={{ display: "flex", alignItems: "center", gap: "var(--space-2)" }}
                        >
                          <span style={{ fontSize: 12, color: "var(--color-error)" }}>
                            Delete “{ruleDisplayName(rule)}”? This action cannot be undone.
                          </span>
                          <button
                            ref={confirmRuleBtnRef}
                            className="btn-secondary"
                            style={{ ...smBtnStyle, color: "var(--color-error)", borderColor: "var(--color-error)" }}
                            onClick={() => void confirmDeleteRule()}
                          >
                            Yes, delete
                          </button>
                          <button className="btn-secondary" style={smBtnStyle} onClick={cancelDeleteRule}>
                            Cancel
                          </button>
                        </div>
                      ) : (
                        <>
                          <button className="btn-secondary" style={smBtnStyle} onClick={() => setEditingRule(rule)}>Edit</button>
                          <button
                            ref={(el) => { ruleDeleteBtnRefs.current[rule.id] = el; }}
                            className="btn-secondary"
                            style={{ ...smBtnStyle, color: "var(--color-error)", borderColor: "var(--color-error)" }}
                            onClick={() => requestDeleteRule(rule.id)}
                          >
                            Delete
                          </button>
                        </>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}

          {/* Channels panel */}
          {tab === "channels" && (
            <div role="tabpanel" id="panel-channels" aria-labelledby="tab-channels">
              {channels.length === 0 ? (
                <EmptyState
                  title="No notification channels"
                  description="Add a channel to receive alerts via email, Slack, or webhook."
                  action={<button className="btn-primary" style={btnStyle} onClick={() => setEditingChannel("new")}>Add channel</button>}
                />
              ) : (
                <div
                  style={{
                    background: "var(--color-surface)",
                    border: "1px solid var(--color-border)",
                    borderRadius: "var(--radius-card)",
                    overflow: "hidden",
                  }}
                >
                  {channels.map((ch, i) => (
                    <div
                      key={ch.id}
                      style={{
                        display: "flex",
                        alignItems: "center",
                        gap: "var(--space-3)",
                        padding: "var(--space-3) var(--space-4)",
                        borderTop: i === 0 ? "none" : "1px solid var(--color-border)",
                      }}
                    >
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <div style={{ fontWeight: 600, fontSize: 13 }}>{ch.name}</div>
                        <div style={{ fontSize: 12, color: "var(--color-secondary)", marginTop: 2 }}>{ch.type}</div>
                      </div>
                      <Badge label={ch.type} variant="info" />
                      {/* s111 D15: same row-attached confirmation model as rules. */}
                      {confirmDeleteChannelId === ch.id ? (
                        <div
                          data-testid="delete-channel-confirm"
                          role="group"
                          aria-label={`Confirm deleting channel ${ch.name}`}
                          className="panel-enter"
                          style={{ display: "flex", alignItems: "center", gap: "var(--space-2)" }}
                        >
                          <span style={{ fontSize: 12, color: "var(--color-error)" }}>
                            Delete “{ch.name}”? Rules routing to it stop notifying. This action cannot be undone.
                          </span>
                          <button
                            ref={confirmChannelBtnRef}
                            className="btn-secondary"
                            style={{ ...smBtnStyle, color: "var(--color-error)", borderColor: "var(--color-error)" }}
                            onClick={() => void confirmDeleteChannel()}
                          >
                            Yes, delete
                          </button>
                          <button className="btn-secondary" style={smBtnStyle} onClick={cancelDeleteChannel}>
                            Cancel
                          </button>
                        </div>
                      ) : (
                        <>
                          <button
                            className="btn-secondary"
                            style={{ ...smBtnStyle, color: "var(--color-accent-hover)", borderColor: "var(--color-accent)" }}
                            onClick={() => void testChannel(ch.id)}
                            disabled={testingChannel === ch.id}
                          >
                            {testingChannel === ch.id ? "Sending…" : "Test fire"}
                          </button>
                          <button className="btn-secondary" style={smBtnStyle} onClick={() => setEditingChannel(ch)}>Edit</button>
                          <button
                            ref={(el) => { channelDeleteBtnRefs.current[ch.id] = el; }}
                            className="btn-secondary"
                            style={{ ...smBtnStyle, color: "var(--color-error)", borderColor: "var(--color-error)" }}
                            onClick={() => setConfirmDeleteChannelId(ch.id)}
                            aria-label={`Delete channel ${ch.name}`}
                          >
                            Delete
                          </button>
                        </>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}

          {/* History panel */}
          {tab === "history" && (
            <div role="tabpanel" id="panel-history" aria-labelledby="tab-history">
              {history.length === 0 ? (
                <EmptyState
                  title="No alert history"
                  description="Fired alerts will appear here."
                />
              ) : (
                <div
                  style={{
                    background: "var(--color-surface)",
                    border: "1px solid var(--color-border)",
                    borderRadius: "var(--radius-card)",
                    overflow: "hidden",
                  }}
                >
                  <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
                    <thead style={{ background: "var(--color-surface-2)" }}>
                      <tr>
                        {["Rule ID", "Severity", "State", "Time", "Value"].map((h) => (
                          <th key={h} className="label" style={{ padding: "var(--space-3) var(--space-4)", textAlign: "left" }}>{h}</th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {history.map((entry) => (
                        <tr key={entry.id} style={{ borderTop: "1px solid var(--color-border)" }}>
                          <td style={{ padding: "var(--cell-pad)", fontWeight: 500, fontFamily: "var(--font-mono)", fontSize: 12 }}>{entry.rule_id}</td>
                          <td style={{ padding: "var(--cell-pad)" }}><Badge label={entry.severity} variant={severityVariant(entry.severity)} /></td>
                          <td style={{ padding: "var(--cell-pad)" }}><Badge label={entry.state} variant={stateVariant(entry.state)} /></td>
                          <td style={{ padding: "var(--cell-pad)", color: "var(--color-secondary)", fontFamily: "var(--font-mono)", fontSize: 12 }}>{fmtTs(entry.ts)}</td>
                          {/* s111 D12: data-numeric → tabular-nums (global.css) */}
                          <td data-numeric style={{ padding: "var(--cell-pad)", fontFamily: "var(--font-mono)", fontSize: 12 }}>{entry.value != null ? String(entry.value) : "—"}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          )}
        </>
      )}
    </div>
  );
}
