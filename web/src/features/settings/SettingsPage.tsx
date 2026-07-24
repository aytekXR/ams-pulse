/**
 * Settings page.
 *
 * Wave 1 tabs: sources, tokens (API), license, users
 * Wave 2 additions:
 *   - "ingest" tab: ingest tokens management (create/revoke; copy SDK snippet)
 *   - "integrations" tab: Prometheus endpoint info panel + S3 export config
 *
 * Wave 4 notes:
 *   - Uses the shared <Tabs wrap> — NOT a hand-rolled tab bar. The `wrap` prop was
 *     added to <Tabs> for this page (six tabs overflow a narrow viewport). An earlier
 *     draft kept a local copy with role="tab" + a roving tabIndex but no key handler:
 *     that is not merely an incomplete ARIA contract, it makes every inactive tab
 *     unreachable by keyboard (tabIndex=-1 removes them from the tab order and nothing
 *     puts them back). The shared component carries Arrow/Home/End navigation.
 *   - Two inline color literals for the info signal replaced with var(--color-info).
 *   - Background/border rgba() values that do not match any token alpha are left
 *     as-is and reported, rather than silently retinted.
 */
import { useState, useEffect, useCallback } from "react";
import { adminApi, ApiError } from "@/api/client";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { ErrorBanner } from "@/components/ErrorBanner";
import { Badge } from "@/components/Badge";
import { EmptyState } from "@/components/EmptyState";
import { Tabs } from "@/components/Tabs";
import { useToast } from "@/components/Toast";
import type { Source, Token, LicenseInfo, TokenCreated } from "@/lib/api/types";

type Tab = "sources" | "tokens" | "ingest" | "integrations" | "license" | "users";

// ─── Ingest token snippet generator ──────────────────────────────────────────

function IngestSnippet({ token }: { token: string }) {
  const { toast } = useToast();
  // Bake the concrete Pulse origin into the snippet — the snippet runs on the
  // player's page, where window.location.origin would be the WRONG host. The
  // SDK appends /ingest/beacon to ingestUrl itself; streamId is required.
  const snippet = `import { Pulse } from '@pulse/beacon';

const session = Pulse.init({
  ingestUrl: '${window.location.origin}',
  token: '${token}',
  streamId: 'your-stream-id', // must match the AMS stream id being played
});`;

  const copy = () => {
    void navigator.clipboard.writeText(snippet).then(() => toast("Snippet copied", "success"));
  };

  return (
    <div style={{
      background: "var(--color-bg)",
      border: "1px solid var(--color-border)",
      borderRadius: 6,
      padding: "var(--space-3)",
      position: "relative",
    }}>
      <pre style={{
        margin: 0,
        fontSize: 12,
        fontFamily: "var(--font-mono)",
        color: "var(--color-text)",
        whiteSpace: "pre-wrap",
        wordBreak: "break-all",
      }}>
        {snippet}
      </pre>
      <button
        onClick={copy}
        style={{
          position: "absolute",
          top: 8,
          right: 8,
          background: "var(--color-surface-2)",
          border: "1px solid var(--color-border)",
          color: "var(--color-secondary)",
          borderRadius: 4,
          padding: "2px 8px",
          cursor: "pointer",
          fontSize: 11,
        }}
      >
        Copy
      </button>
    </div>
  );
}

// ─── Main page ────────────────────────────────────────────────────────────────

export function SettingsPage() {
  const { toast } = useToast();
  const [tab, setTab] = useState<Tab>("sources");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [sources, setSources] = useState<Source[]>([]);
  const [tokens, setTokens] = useState<Token[]>([]);
  const [ingestTokens, setIngestTokens] = useState<Token[]>([]);
  const [license, setLicense] = useState<LicenseInfo | null>(null);
  const [licenseKey, setLicenseKey] = useState("");
  const [savingLicense, setSavingLicense] = useState(false);
  const [newIngestToken, setNewIngestToken] = useState<TokenCreated | null>(null);
  const [newApiToken, setNewApiToken] = useState<TokenCreated | null>(null);
  // S3 export form (env-ref names, never raw creds)
  const [s3Bucket, setS3Bucket] = useState("");
  const [s3Region, setS3Region] = useState("us-east-1");
  const [s3KeyEnvRef, setS3KeyEnvRef] = useState("AWS_ACCESS_KEY_ID");
  const [s3SecretEnvRef, setS3SecretEnvRef] = useState("AWS_SECRET_ACCESS_KEY");

  const loadAll = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [srcsData, toksData, licData] = await Promise.all([
        adminApi.getSources(),
        adminApi.getTokens(),
        adminApi.getLicense(),
      ]);
      const allTokens = toksData.items ?? [];
      setSources(srcsData.items ?? []);
      setTokens(allTokens.filter((t) => t.kind === "api"));
      setIngestTokens(allTokens.filter((t) => t.kind === "ingest"));
      setLicense(licData);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to load settings");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadAll();
  }, [loadAll]);

  const deleteSource = async (id: string) => {
    if (!confirm("Remove this source?")) return;
    try {
      await adminApi.deleteSource(id);
      toast("Source removed", "info");
      void loadAll();
    } catch (err) {
      toast(err instanceof ApiError ? err.message : "Failed to remove source", "error");
    }
  };

  const createApiToken = async () => {
    const name = prompt("Token name:");
    if (!name) return;
    // The scope decides whether the server accepts writes from this token
    // (requireWriteScope). An admin token can mint further admin tokens, so it is
    // never the silent default — the caller has to ask for it.
    const admin = confirm(
      "Grant this token admin (write) access?\n\n" +
        "OK — admin: can change settings and create tokens.\n" +
        "Cancel — read-only: can view data only.",
    );
    try {
      const result = await adminApi.createToken({
        kind: "api",
        name,
        scopes: [admin ? "admin" : "read"],
      });
      setNewApiToken(result);
      toast("API token created — copy it now, it won't be shown again", "success");
      void loadAll();
    } catch (err) {
      toast(err instanceof ApiError ? err.message : "Failed to create token", "error");
    }
  };

  const createIngestToken = async () => {
    const name = prompt("Ingest token name (e.g. player-prod):");
    if (!name) return;
    try {
      const result = await adminApi.createToken({ kind: "ingest", name, scopes: ["ingest"] });
      setNewIngestToken(result);
      toast("Ingest token created — copy it now, it won't be shown again", "success");
      void loadAll();
    } catch (err) {
      toast(err instanceof ApiError ? err.message : "Failed to create ingest token", "error");
    }
  };

  const deleteToken = async (id: string) => {
    if (!confirm("Revoke this token?")) return;
    try {
      await adminApi.deleteToken(id);
      toast("Token revoked", "info");
      void loadAll();
    } catch (err) {
      toast(err instanceof ApiError ? err.message : "Failed to revoke token", "error");
    }
  };

  const saveLicense = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!licenseKey.trim()) return;
    setSavingLicense(true);
    try {
      const result = await adminApi.setLicense(licenseKey.trim());
      setLicense(result);
      setLicenseKey("");
      toast(`License activated — tier: ${result.tier}`, "success");
    } catch (err) {
      toast(err instanceof ApiError ? err.message : "Failed to activate", "error");
    } finally {
      setSavingLicense(false);
    }
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

  const smBtnStyle: React.CSSProperties = {
    background: "var(--color-surface-2)",
    border: "1px solid var(--color-border)",
    color: "var(--color-secondary)",
    borderRadius: 4,
    padding: "4px 10px",
    cursor: "pointer",
    fontSize: 11,
  };

  const infoBox: React.CSSProperties = {
    background: "var(--color-surface-2)",
    border: "1px solid var(--color-border)",
    borderRadius: 6,
    padding: "var(--space-3) var(--space-4)",
    fontSize: 13,
  };

  const tabs: Tab[] = ["sources", "tokens", "ingest", "integrations", "license", "users"];
  const tabLabels: Record<Tab, string> = {
    sources: "Sources",
    tokens: "API Tokens",
    ingest: "Ingest Tokens",
    integrations: "Integrations",
    license: "License",
    users: "Users",
  };

  // Prometheus endpoint URL (served by the binary, unauthenticated by default)
  const prometheusUrl = `${window.location.protocol}//${window.location.host}/metrics`;

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
      <h1 style={{ margin: 0, fontSize: 18, fontWeight: 700 }}>Settings</h1>

      {/*
        Custom tab bar: flexWrap="wrap" supports 6 tabs on narrow screens.
        The shared <Tabs> component has no flexWrap prop, so pixel-safe conversion
        is not possible — ARIA is wired manually instead.
      */}
      {/* The shared <Tabs>, not a hand-rolled copy. The copy this replaces set role="tab"
          and a roving tabIndex but had NO key handler — so every inactive tab was
          unreachable by keyboard (tabIndex=-1 takes them out of the tab order and nothing
          put them back). <Tabs wrap> carries the Arrow/Home/End navigation and keeps the
          six-tab strip wrapping on narrow viewports. */}
      <Tabs
        wrap
        tabs={tabs.map((t) => ({ id: t, label: tabLabels[t] }))}
        activeTab={tab}
        onTabChange={(id) => setTab(id as Tab)}
      />

      {error && <ErrorBanner message={error} onRetry={loadAll} />}

      {loading ? (
        <LoadingSpinner />
      ) : (
        <>
          {/* ── Sources tab ── */}
          {tab === "sources" && (
            <div role="tabpanel" id="settings-panel-sources" aria-labelledby="tab-sources">
              <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-4)" }}>
                <div style={{ display: "flex", justifyContent: "flex-end" }}>
                  <button
                    style={{
                      background: "var(--color-accent)",
                      border: "none",
                      color: "var(--color-on-signal)",
                      borderRadius: 6,
                      padding: "7px 14px",
                      cursor: "pointer",
                      fontSize: 12,
                      fontWeight: 600,
                    }}
                    onClick={() => toast("Use the onboarding wizard to add sources", "info")}
                  >
                    + Add source
                  </button>
                </div>
                {sources.length === 0 ? (
                  <p style={{ color: "var(--color-secondary)", fontSize: 13 }}>No AMS sources configured.</p>
                ) : (
                  <div style={{ background: "var(--color-surface)", border: "1px solid var(--color-border)", borderRadius: 8, overflow: "hidden" }}>
                    {sources.map((src, i) => (
                      <div
                        key={src.id}
                        style={{
                          display: "flex",
                          alignItems: "center",
                          gap: "var(--space-3)",
                          padding: "var(--space-3) var(--space-4)",
                          borderTop: i === 0 ? "none" : "1px solid var(--color-border)",
                        }}
                      >
                        <div style={{ flex: 1, minWidth: 0 }}>
                          <div style={{ fontWeight: 600, fontSize: 13 }}>{src.name}</div>
                          <div style={{ fontSize: 12, color: "var(--color-secondary)", marginTop: 2 }}>{src.rest_url ?? src.type}</div>
                        </div>
                        <Badge label={src.type} variant="info" />
                        <button
                          style={{ ...smBtnStyle, color: "var(--color-error)", borderColor: "var(--color-error)" }}
                          onClick={() => void deleteSource(src.id)}
                        >
                          Remove
                        </button>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
          )}

          {/* ── API Tokens tab ── */}
          {tab === "tokens" && (
            <div role="tabpanel" id="settings-panel-tokens" aria-labelledby="tab-tokens">
              <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-4)" }}>
                <div style={{ display: "flex", justifyContent: "flex-end" }}>
                  <button
                    style={{
                      background: "var(--color-accent)",
                      border: "none",
                      color: "var(--color-on-signal)",
                      borderRadius: 6,
                      padding: "7px 14px",
                      cursor: "pointer",
                      fontSize: 12,
                      fontWeight: 600,
                    }}
                    onClick={() => void createApiToken()}
                  >
                    + New token
                  </button>
                </div>
                {/* Newly created API token (shown once — server hashes on creation and never returns plaintext again) */}
                {newApiToken && (
                  <div style={{
                    background: "rgba(88,166,255,0.08)",
                    border: "1px solid rgba(88,166,255,0.25)",
                    borderRadius: 8,
                    padding: "var(--space-4)",
                    display: "flex",
                    flexDirection: "column",
                    gap: "var(--space-3)",
                  }}>
                    <div style={{ display: "flex", alignItems: "center", gap: "var(--space-2)" }}>
                      <span style={{ fontWeight: 700, color: "var(--color-info)", fontSize: 13 }}>
                        Token created — copy it now, it won't be shown again
                      </span>
                      <button
                        onClick={() => setNewApiToken(null)}
                        aria-label="Dismiss token"
                        style={{ marginLeft: "auto", background: "none", border: "none", color: "var(--color-info)", cursor: "pointer", fontSize: 18, lineHeight: 1 }}
                      >
                        ×
                      </button>
                    </div>
                    <div style={{ ...infoBox, fontFamily: "var(--font-mono)", fontSize: 12, wordBreak: "break-all", position: "relative" }}>
                      {newApiToken.token}
                      <button
                        onClick={() => void navigator.clipboard.writeText(newApiToken.token).then(() => toast("Token copied", "success"))}
                        style={{
                          position: "absolute",
                          top: 8,
                          right: 8,
                          background: "var(--color-surface-2)",
                          border: "1px solid var(--color-border)",
                          color: "var(--color-secondary)",
                          borderRadius: 4,
                          padding: "2px 8px",
                          cursor: "pointer",
                          fontSize: 11,
                        }}
                      >
                        Copy
                      </button>
                    </div>
                  </div>
                )}

                {tokens.length === 0 ? (
                  <EmptyState title="No API tokens" description="API tokens authenticate dashboard and API clients." />
                ) : (
                  <div style={{ background: "var(--color-surface)", border: "1px solid var(--color-border)", borderRadius: 8, overflow: "hidden" }}>
                    {tokens.map((tok, i) => (
                      <div
                        key={tok.id}
                        style={{
                          display: "flex",
                          alignItems: "center",
                          gap: "var(--space-3)",
                          padding: "var(--space-3) var(--space-4)",
                          borderTop: i === 0 ? "none" : "1px solid var(--color-border)",
                        }}
                      >
                        <div style={{ flex: 1, minWidth: 0 }}>
                          <div style={{ fontWeight: 600, fontSize: 13 }}>{tok.name}</div>
                          <div style={{ fontSize: 12, color: "var(--color-secondary)", marginTop: 2, fontFamily: "var(--font-mono)" }}>
                            {(tok.scopes ?? []).join(", ")} · created {new Date(tok.created_at).toLocaleDateString()}
                            {tok.last_used_at && ` · last used ${new Date(tok.last_used_at).toLocaleDateString()}`}
                          </div>
                        </div>
                        <button
                          style={{ ...smBtnStyle, color: "var(--color-error)", borderColor: "var(--color-error)" }}
                          onClick={() => void deleteToken(tok.id)}
                        >
                          Revoke
                        </button>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
          )}

          {/* ── Ingest Tokens tab (Wave-2 addition) ── */}
          {tab === "ingest" && (
            <div role="tabpanel" id="settings-panel-ingest" aria-labelledby="tab-ingest">
              <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
                <div style={infoBox}>
                  <strong>Ingest tokens</strong> authenticate the beacon SDK. Each token can be scoped to a stream
                  or app. Tokens are revocable; never expose raw values in client-side code beyond the SDK init call.
                </div>

                <div style={{ display: "flex", justifyContent: "flex-end" }}>
                  <button
                    style={{
                      background: "var(--color-accent)",
                      border: "none",
                      color: "var(--color-on-signal)",
                      borderRadius: 6,
                      padding: "7px 14px",
                      cursor: "pointer",
                      fontSize: 12,
                      fontWeight: 600,
                    }}
                    onClick={() => void createIngestToken()}
                  >
                    + New ingest token
                  </button>
                </div>

                {/* Newly created token (shown once) */}
                {newIngestToken && (
                  <div style={{
                    background: "rgba(88,166,255,0.08)",
                    border: "1px solid rgba(88,166,255,0.25)",
                    borderRadius: 8,
                    padding: "var(--space-4)",
                    display: "flex",
                    flexDirection: "column",
                    gap: "var(--space-3)",
                  }}>
                    <div style={{ display: "flex", alignItems: "center", gap: "var(--space-2)" }}>
                      <span style={{ fontWeight: 700, color: "var(--color-info)", fontSize: 13 }}>
                        Token created — copy it now, it won't be shown again
                      </span>
                      <button
                        onClick={() => setNewIngestToken(null)}
                        style={{ marginLeft: "auto", background: "none", border: "none", color: "var(--color-info)", cursor: "pointer", fontSize: 18, lineHeight: 1 }}
                      >
                        ×
                      </button>
                    </div>
                    <div style={{ ...infoBox, fontFamily: "var(--font-mono)", fontSize: 12, wordBreak: "break-all" }}>
                      {newIngestToken.token}
                    </div>
                    <IngestSnippet token={newIngestToken.token} />
                  </div>
                )}

                {ingestTokens.length === 0 ? (
                  <EmptyState
                    title="No ingest tokens"
                    description="Create an ingest token to authenticate the beacon SDK in your player."
                  />
                ) : (
                  <div style={{ background: "var(--color-surface)", border: "1px solid var(--color-border)", borderRadius: 8, overflow: "hidden" }}>
                    {ingestTokens.map((tok, i) => (
                      <div
                        key={tok.id}
                        style={{
                          display: "flex",
                          alignItems: "center",
                          gap: "var(--space-3)",
                          padding: "var(--space-3) var(--space-4)",
                          borderTop: i === 0 ? "none" : "1px solid var(--color-border)",
                        }}
                      >
                        <div style={{ flex: 1, minWidth: 0 }}>
                          <div style={{ fontWeight: 600, fontSize: 13 }}>{tok.name}</div>
                          <div style={{ fontSize: 12, color: "var(--color-secondary)", marginTop: 2 }}>
                            ingest · created {new Date(tok.created_at).toLocaleDateString()}
                            {tok.last_used_at && ` · last used ${new Date(tok.last_used_at).toLocaleDateString()}`}
                          </div>
                        </div>
                        <Badge label="ingest" variant="info" />
                        <button
                          style={{ ...smBtnStyle, color: "var(--color-error)", borderColor: "var(--color-error)" }}
                          onClick={() => void deleteToken(tok.id)}
                        >
                          Revoke
                        </button>
                      </div>
                    ))}
                  </div>
                )}

                <div style={{ fontSize: 12, color: "var(--color-secondary)" }}>
                  Ingest endpoint: <code style={{ fontFamily: "var(--font-mono)", fontSize: 12 }}>{window.location.origin}/ingest/beacon</code>
                </div>
              </div>
            </div>
          )}

          {/* ── Integrations tab (Wave-2 addition) ── */}
          {tab === "integrations" && (
            <div role="tabpanel" id="settings-panel-integrations" aria-labelledby="tab-integrations">
              <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
                {/* Prometheus */}
                <div style={{
                  background: "var(--color-surface)",
                  border: "1px solid var(--color-border)",
                  borderRadius: 8,
                  padding: 20,
                  display: "flex",
                  flexDirection: "column",
                  gap: "var(--space-3)",
                }}>
                  <h3 style={{ margin: 0, fontSize: 14, fontWeight: 700 }}>Prometheus Metrics</h3>
                  <p style={{ margin: 0, fontSize: 13, color: "var(--color-secondary)" }}>
                    Pulse exposes Prometheus metrics at the endpoint below. Unauthenticated by default;
                    set <code style={{ fontFamily: "var(--font-mono)" }}>PULSE_METRICS_TOKEN</code> to require a bearer token.
                  </p>
                  <div style={infoBox}>
                    <div style={{ fontSize: 11, color: "var(--color-secondary)", marginBottom: "var(--space-1)", textTransform: "uppercase", letterSpacing: "0.06em" }}>Scrape URL</div>
                    <div style={{ fontFamily: "var(--font-mono)", fontSize: 13, wordBreak: "break-all" }}>{prometheusUrl}</div>
                  </div>
                  <p style={{ margin: 0, fontSize: 12, color: "var(--color-secondary)" }}>
                    Example scrape config:
                  </p>
                  <pre style={{
                    margin: 0,
                    background: "var(--color-bg)",
                    border: "1px solid var(--color-border)",
                    borderRadius: 6,
                    padding: "var(--space-3)",
                    fontSize: 12,
                    fontFamily: "var(--font-mono)",
                    color: "var(--color-text)",
                    overflowX: "auto",
                  }}>
{`scrape_configs:
  - job_name: pulse
    static_configs:
      - targets: ['${window.location.host}']
    metrics_path: /metrics`}
                  </pre>
                </div>

                {/* S3 export */}
                <div style={{
                  background: "var(--color-surface)",
                  border: "1px solid var(--color-border)",
                  borderRadius: 8,
                  padding: 20,
                  display: "flex",
                  flexDirection: "column",
                  gap: 14,
                }}>
                  <h3 style={{ margin: 0, fontSize: 14, fontWeight: 700 }}>S3 Export Destination</h3>
                  <p style={{ margin: 0, fontSize: 13, color: "var(--color-secondary)" }}>
                    Configure automatic report uploads to an S3 bucket. Credentials are referenced by
                    environment variable name — the actual credential values are never stored or echoed.
                  </p>
                  <form onSubmit={(e) => { e.preventDefault(); toast("S3 export config saved (server-side TBD in wave 3)", "info"); }} style={{ display: "flex", flexDirection: "column", gap: "var(--space-3)" }}>
                    <label style={{ fontSize: 12, color: "var(--color-secondary)", display: "flex", flexDirection: "column", gap: "var(--space-1)" }}>
                      S3 Bucket
                      <input
                        type="text"
                        value={s3Bucket}
                        onChange={(e) => setS3Bucket(e.target.value)}
                        placeholder="my-pulse-reports"
                        style={{ ...inputStyle }}
                      />
                    </label>
                    <label style={{ fontSize: 12, color: "var(--color-secondary)", display: "flex", flexDirection: "column", gap: "var(--space-1)" }}>
                      AWS Region
                      <input
                        type="text"
                        value={s3Region}
                        onChange={(e) => setS3Region(e.target.value)}
                        placeholder="us-east-1"
                        style={{ ...inputStyle }}
                      />
                    </label>
                    <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 10 }}>
                      <label style={{ fontSize: 12, color: "var(--color-secondary)", display: "flex", flexDirection: "column", gap: "var(--space-1)" }}>
                        Access Key env var name
                        <input
                          type="text"
                          value={s3KeyEnvRef}
                          onChange={(e) => setS3KeyEnvRef(e.target.value)}
                          placeholder="AWS_ACCESS_KEY_ID"
                          style={{ ...inputStyle }}
                        />
                      </label>
                      <label style={{ fontSize: 12, color: "var(--color-secondary)", display: "flex", flexDirection: "column", gap: "var(--space-1)" }}>
                        Secret Key env var name
                        <input
                          type="text"
                          value={s3SecretEnvRef}
                          onChange={(e) => setS3SecretEnvRef(e.target.value)}
                          placeholder="AWS_SECRET_ACCESS_KEY"
                          style={{ ...inputStyle }}
                        />
                      </label>
                    </div>
                    <p style={{ margin: 0, fontSize: 12, color: "var(--color-secondary)" }}>
                      The credentials at those env var names must be available to the Pulse process at runtime.
                      Never enter credential values directly here.
                    </p>
                    <div style={{ display: "flex", justifyContent: "flex-end" }}>
                      <button
                        type="submit"
                        style={{
                          background: "var(--color-accent)",
                          border: "none",
                          color: "var(--color-on-signal)",
                          borderRadius: 6,
                          padding: "7px 14px",
                          cursor: "pointer",
                          fontSize: 13,
                          fontWeight: 600,
                        }}
                      >
                        Save S3 config
                      </button>
                    </div>
                  </form>
                </div>
              </div>
            </div>
          )}

          {/* ── License tab ── */}
          {tab === "license" && (
            <div role="tabpanel" id="settings-panel-license" aria-labelledby="tab-license">
              <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
                {license && (
                  <div
                    style={{
                      background: "var(--color-surface)",
                      border: "1px solid var(--color-border)",
                      borderRadius: 8,
                      padding: "20px",
                      display: "flex",
                      flexDirection: "column",
                      gap: "var(--space-3)",
                    }}
                  >
                    <div style={{ display: "flex", alignItems: "center", gap: "var(--space-3)" }}>
                      <span style={{ fontWeight: 700, fontSize: 15 }}>Current license</span>
                      <Badge
                        label={license.tier}
                        variant={license.tier === "enterprise" ? "success" : license.tier === "pro" ? "info" : "muted"}
                      />
                    </div>
                    {license.expires_at && (
                      <p style={{ margin: 0, fontSize: 13, color: "var(--color-secondary)" }}>
                        Expires: {new Date(license.expires_at).toLocaleDateString()}
                      </p>
                    )}
                    {license.limits && (
                      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(150px, 1fr))", gap: 10 }}>
                        {Object.entries(license.limits).map(([k, v]) => (
                          <div key={k} style={{ background: "var(--color-surface-2)", borderRadius: 6, padding: "var(--space-2) var(--space-3)" }}>
                            <div style={{ fontSize: 11, color: "var(--color-secondary)", textTransform: "uppercase", letterSpacing: "0.06em" }}>{k.replace(/_/g, " ")}</div>
                            <div style={{ fontSize: 15, fontWeight: 600, marginTop: 2 }}>{v === -1 ? "∞" : String(v)}</div>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                )}

                <div
                  style={{
                    background: "var(--color-surface)",
                    border: "1px solid var(--color-border)",
                    borderRadius: 8,
                    padding: "20px",
                  }}
                >
                  <h3 style={{ margin: "0 0 16px", fontSize: 14, fontWeight: 600 }}>
                    {license?.tier && license.tier !== "free" ? "Update license key" : "Activate license"}
                  </h3>
                  <form onSubmit={(e) => void saveLicense(e)} style={{ display: "flex", gap: 10 }}>
                    <input
                      style={{ ...inputStyle, flex: 1 }}
                      type="text"
                      value={licenseKey}
                      onChange={(e) => setLicenseKey(e.target.value)}
                      placeholder="PULSE-XXXX-XXXX-XXXX"
                    />
                    <button
                      type="submit"
                      disabled={savingLicense || !licenseKey.trim()}
                      style={{
                        background: "var(--color-accent)",
                        border: "none",
                        color: "var(--color-on-signal)",
                        borderRadius: 6,
                        padding: "var(--space-2) var(--space-4)",
                        cursor: "pointer",
                        fontSize: 13,
                        fontWeight: 600,
                        opacity: savingLicense ? 0.7 : 1,
                      }}
                    >
                      {savingLicense ? "Activating…" : "Activate"}
                    </button>
                  </form>
                  <p style={{ margin: "12px 0 0", fontSize: 12, color: "var(--color-secondary)" }}>
                    Free tier requires no license key. Contact sales for Pro/Enterprise keys.
                  </p>
                </div>
              </div>
            </div>
          )}

          {/* ── Users tab ── */}
          {tab === "users" && (
            <div role="tabpanel" id="settings-panel-users" aria-labelledby="tab-users">
              <div style={{ color: "var(--color-secondary)", fontSize: 13 }}>
                User management — coming in a future update.
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}
