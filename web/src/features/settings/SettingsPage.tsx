import { useState, useEffect, useCallback } from "react";
import { adminApi, ApiError } from "@/api/client";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { ErrorBanner } from "@/components/ErrorBanner";
import { Badge } from "@/components/Badge";
import { useToast } from "@/components/Toast";
import type { Source, Token, LicenseInfo } from "@/lib/api/types";

type Tab = "sources" | "tokens" | "license" | "users";

export function SettingsPage() {
  const { toast } = useToast();
  const [tab, setTab] = useState<Tab>("sources");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [sources, setSources] = useState<Source[]>([]);
  const [tokens, setTokens] = useState<Token[]>([]);
  const [license, setLicense] = useState<LicenseInfo | null>(null);
  const [licenseKey, setLicenseKey] = useState("");
  const [savingLicense, setSavingLicense] = useState(false);

  const loadAll = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [srcsData, toksData, licData] = await Promise.all([
        adminApi.getSources(),
        adminApi.getTokens(),
        adminApi.getLicense(),
      ]);
      // responses use `items` per generated schema (SourceList, TokenList)
      setSources(srcsData.items ?? []);
      setTokens(toksData.items ?? []);
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

  const createToken = async () => {
    const name = prompt("Token name:");
    if (!name) return;
    const result = await adminApi.createToken({ kind: "api", name, scopes: ["read"] });
    toast(`Token created: ${result.token}`, "success");
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

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
      <h1 style={{ margin: 0, fontSize: 18, fontWeight: 700 }}>Settings</h1>

      {/* Tabs */}
      <div style={{ display: "flex", gap: 0, borderBottom: "1px solid var(--color-border)" }}>
        {(["sources", "tokens", "license", "users"] as Tab[]).map((t) => (
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

      {loading ? (
        <LoadingSpinner />
      ) : (
        <>
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
                  onClick={() => void createToken()}
                >
                  + New token
                </button>
              </div>
              {tokens.length === 0 ? (
                <p style={{ color: "var(--color-muted)", fontSize: 13 }}>No API tokens.</p>
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
