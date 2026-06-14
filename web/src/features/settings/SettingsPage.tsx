/**
 * Settings page.
 *
 * Wave 1 tabs: sources, tokens (API), license, users
 * Wave 2 additions:
 *   - "ingest" tab: ingest tokens management (create/revoke; copy SDK snippet)
 *   - "integrations" tab: Prometheus endpoint info panel + S3 export config
 */
import { useState, useEffect, useCallback } from "react";
import { adminApi, ApiError } from "@/api/client";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { ErrorBanner } from "@/components/ErrorBanner";
import { Badge } from "@/components/Badge";
import { EmptyState } from "@/components/EmptyState";
import { useToast } from "@/components/Toast";
import type { Source, Token, LicenseInfo, TokenCreated } from "@/lib/api/types";

type Tab = "sources" | "tokens" | "ingest" | "integrations" | "license" | "users";

// ─── Ingest token snippet generator ──────────────────────────────────────────

function IngestSnippet({ token }: { token: string }) {
  const { toast } = useToast();
  const snippet = `import Pulse from '@pulse/beacon-js';

Pulse.init({
  token: '${token}',
  endpoint: window.location.origin + '/ingest/beacon',
});`;

  const copy = () => {
    void navigator.clipboard.writeText(snippet).then(() => toast("Snippet copied", "success"));
  };

  return (
    <div style={{
      background: "var(--color-bg)",
      border: "1px solid var(--color-border)",
      borderRadius: 6,
      padding: 12,
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
          color: "var(--color-muted)",
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
    await adminApi.deleteSource(id);
    toast("Source removed", "info");
    void loadAll();
  };

  const createApiToken = async () => {
    const name = prompt("Token name:");
    if (!name) return;
    const result = await adminApi.createToken({ kind: "api", name, scopes: ["read"] });
    toast(`Token created: ${result.token}`, "success");
    void loadAll();
  };

  const createIngestToken = async () => {
    const name = prompt("Ingest token name (e.g. player-prod):");
    if (!name) return;
    const result = await adminApi.createToken({ kind: "ingest", name, scopes: ["ingest"] });
    setNewIngestToken(result);
    toast("Ingest token created — copy it now, it won't be shown again", "success");
    void loadAll();
  };

  const deleteToken = async (id: string) => {
    if (!confirm("Revoke this token?")) return;
    await adminApi.deleteToken(id);
    toast("Token revoked", "info");
    void loadAll();
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
    color: "var(--color-muted)",
    borderRadius: 4,
    padding: "4px 10px",
    cursor: "pointer",
    fontSize: 11,
  };

  const infoBox: React.CSSProperties = {
    background: "var(--color-surface-2)",
    border: "1px solid var(--color-border)",
    borderRadius: 6,
    padding: "12px 16px",
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

      {/* Tabs */}
      <div style={{ display: "flex", flexWrap: "wrap", gap: 0, borderBottom: "1px solid var(--color-border)" }}>
        {tabs.map((t) => (
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
              whiteSpace: "nowrap",
            }}
          >
            {tabLabels[t]}
          </button>
        ))}
      </div>

      {error && <ErrorBanner message={error} onRetry={loadAll} />}

      {loading ? (
        <LoadingSpinner />
      ) : (
        <>
          {/* ── Sources tab ── */}
          {tab === "sources" && (
            <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
              <div style={{ display: "flex", justifyContent: "flex-end" }}>
                <button
                  style={{
                    background: "var(--color-accent)",
                    border: "none",
                    color: "#fff",
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
                <p style={{ color: "var(--color-muted)", fontSize: 13 }}>No AMS sources configured.</p>
              ) : (
                <div style={{ background: "var(--color-surface)", border: "1px solid var(--color-border)", borderRadius: 8, overflow: "hidden" }}>
                  {sources.map((src, i) => (
                    <div
                      key={src.id}
                      style={{
                        display: "flex",
                        alignItems: "center",
                        gap: 12,
                        padding: "12px 16px",
                        borderTop: i === 0 ? "none" : "1px solid var(--color-border)",
                      }}
                    >
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <div style={{ fontWeight: 600, fontSize: 13 }}>{src.name}</div>
                        <div style={{ fontSize: 12, color: "var(--color-muted)", marginTop: 2 }}>{src.rest_url ?? src.type}</div>
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
          )}

          {/* ── API Tokens tab ── */}
          {tab === "tokens" && (
            <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
              <div style={{ display: "flex", justifyContent: "flex-end" }}>
                <button
                  style={{
                    background: "var(--color-accent)",
                    border: "none",
                    color: "#fff",
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
                        gap: 12,
                        padding: "12px 16px",
                        borderTop: i === 0 ? "none" : "1px solid var(--color-border)",
                      }}
                    >
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <div style={{ fontWeight: 600, fontSize: 13 }}>{tok.name}</div>
                        <div style={{ fontSize: 12, color: "var(--color-muted)", marginTop: 2, fontFamily: "var(--font-mono)" }}>
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
          )}

          {/* ── Ingest Tokens tab (Wave-2 addition) ── */}
          {tab === "ingest" && (
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
                    color: "#fff",
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
                  background: "#0f2744",
                  border: "1px solid #60a5fa",
                  borderRadius: 8,
                  padding: 16,
                  display: "flex",
                  flexDirection: "column",
                  gap: 12,
                }}>
                  <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                    <span style={{ fontWeight: 700, color: "#60a5fa", fontSize: 13 }}>
                      Token created — copy it now, it won't be shown again
                    </span>
                    <button
                      onClick={() => setNewIngestToken(null)}
                      style={{ marginLeft: "auto", background: "none", border: "none", color: "#60a5fa", cursor: "pointer", fontSize: 18, lineHeight: 1 }}
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
                        gap: 12,
                        padding: "12px 16px",
                        borderTop: i === 0 ? "none" : "1px solid var(--color-border)",
                      }}
                    >
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <div style={{ fontWeight: 600, fontSize: 13 }}>{tok.name}</div>
                        <div style={{ fontSize: 12, color: "var(--color-muted)", marginTop: 2 }}>
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

              <div style={{ fontSize: 12, color: "var(--color-muted)" }}>
                Ingest endpoint: <code style={{ fontFamily: "var(--font-mono)", fontSize: 12 }}>{window.location.origin}/ingest/beacon</code>
              </div>
            </div>
          )}

          {/* ── Integrations tab (Wave-2 addition) ── */}
          {tab === "integrations" && (
            <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
              {/* Prometheus */}
              <div style={{
                background: "var(--color-surface)",
                border: "1px solid var(--color-border)",
                borderRadius: 8,
                padding: 20,
                display: "flex",
                flexDirection: "column",
                gap: 12,
              }}>
                <h3 style={{ margin: 0, fontSize: 14, fontWeight: 700 }}>Prometheus Metrics</h3>
                <p style={{ margin: 0, fontSize: 13, color: "var(--color-muted)" }}>
                  Pulse exposes Prometheus metrics at the endpoint below. Unauthenticated by default;
                  set <code style={{ fontFamily: "var(--font-mono)" }}>PULSE_METRICS_TOKEN</code> to require a bearer token.
                </p>
                <div style={infoBox}>
                  <div style={{ fontSize: 11, color: "var(--color-muted)", marginBottom: 4, textTransform: "uppercase", letterSpacing: "0.06em" }}>Scrape URL</div>
                  <div style={{ fontFamily: "var(--font-mono)", fontSize: 13, wordBreak: "break-all" }}>{prometheusUrl}</div>
                </div>
                <p style={{ margin: 0, fontSize: 12, color: "var(--color-muted)" }}>
                  Example scrape config:
                </p>
                <pre style={{
                  margin: 0,
                  background: "var(--color-bg)",
                  border: "1px solid var(--color-border)",
                  borderRadius: 6,
                  padding: 12,
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
                <p style={{ margin: 0, fontSize: 13, color: "var(--color-muted)" }}>
                  Configure automatic report uploads to an S3 bucket. Credentials are referenced by
                  environment variable name — the actual credential values are never stored or echoed.
                </p>
                <form onSubmit={(e) => { e.preventDefault(); toast("S3 export config saved (server-side TBD in wave 3)", "info"); }} style={{ display: "flex", flexDirection: "column", gap: 12 }}>
                  <label style={{ fontSize: 12, color: "var(--color-muted)", display: "flex", flexDirection: "column", gap: 4 }}>
                    S3 Bucket
                    <input
                      type="text"
                      value={s3Bucket}
                      onChange={(e) => setS3Bucket(e.target.value)}
                      placeholder="my-pulse-reports"
                      style={{ ...inputStyle }}
                    />
                  </label>
                  <label style={{ fontSize: 12, color: "var(--color-muted)", display: "flex", flexDirection: "column", gap: 4 }}>
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
                    <label style={{ fontSize: 12, color: "var(--color-muted)", display: "flex", flexDirection: "column", gap: 4 }}>
                      Access Key env var name
                      <input
                        type="text"
                        value={s3KeyEnvRef}
                        onChange={(e) => setS3KeyEnvRef(e.target.value)}
                        placeholder="AWS_ACCESS_KEY_ID"
                        style={{ ...inputStyle }}
                      />
                    </label>
                    <label style={{ fontSize: 12, color: "var(--color-muted)", display: "flex", flexDirection: "column", gap: 4 }}>
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
                  <p style={{ margin: 0, fontSize: 12, color: "var(--color-muted)" }}>
                    The credentials at those env var names must be available to the Pulse process at runtime.
                    Never enter credential values directly here.
                  </p>
                  <div style={{ display: "flex", justifyContent: "flex-end" }}>
                    <button
                      type="submit"
                      style={{
                        background: "var(--color-accent)",
                        border: "none",
                        color: "#fff",
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
          )}

          {/* ── License tab ── */}
          {tab === "license" && (
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
                    gap: 12,
                  }}
                >
                  <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
                    <span style={{ fontWeight: 700, fontSize: 15 }}>Current license</span>
                    <Badge
                      label={license.tier}
                      variant={license.tier === "enterprise" ? "success" : license.tier === "pro" ? "info" : "muted"}
                    />
                  </div>
                  {license.expires_at && (
                    <p style={{ margin: 0, fontSize: 13, color: "var(--color-muted)" }}>
                      Expires: {new Date(license.expires_at).toLocaleDateString()}
                    </p>
                  )}
                  {license.limits && (
                    <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(150px, 1fr))", gap: 10 }}>
                      {Object.entries(license.limits).map(([k, v]) => (
                        <div key={k} style={{ background: "var(--color-surface-2)", borderRadius: 6, padding: "8px 12px" }}>
                          <div style={{ fontSize: 11, color: "var(--color-muted)", textTransform: "uppercase", letterSpacing: "0.06em" }}>{k.replace(/_/g, " ")}</div>
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
                      color: "#fff",
                      borderRadius: 6,
                      padding: "8px 16px",
                      cursor: "pointer",
                      fontSize: 13,
                      fontWeight: 600,
                      opacity: savingLicense ? 0.7 : 1,
                    }}
                  >
                    {savingLicense ? "Activating…" : "Activate"}
                  </button>
                </form>
                <p style={{ margin: "12px 0 0", fontSize: 12, color: "var(--color-muted)" }}>
                  Free tier requires no license key. Contact sales for Pro/Enterprise keys.
                </p>
              </div>
            </div>
          )}

          {/* ── Users tab ── */}
          {tab === "users" && (
            <div style={{ color: "var(--color-muted)", fontSize: 13 }}>
              User management — coming in a future update.
            </div>
          )}
        </>
      )}
    </div>
  );
}
