import { useState } from "react";
import { adminApi, ApiError } from "@/api/client";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import type { SourceWrite } from "@/lib/api/types";

type Step = "welcome" | "source" | "verify" | "done";

interface Props {
  onComplete: () => void;
}

export function OnboardingWizard({ onComplete }: Props) {
  const [step, setStep] = useState<Step>("welcome");
  // SourceWrite: { name, type, rest_url?, rest_user?, rest_password?,
  //                log_path?, kafka_brokers?, webhook_secret?, credential_env_ref? }
  const [sourceData, setSourceData] = useState<SourceWrite>({
    name: "",
    type: "rest_poll",
    rest_url: "",
    credential_env_ref: "",
    log_path: "",
  });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [createdSourceId, setCreatedSourceId] = useState<string | null>(null);
  const [testStatus, setTestStatus] = useState<"idle" | "testing" | "ok" | "fail">("idle");
  const [testMessage, setTestMessage] = useState<string>("");

  const inputStyle: React.CSSProperties = {
    display: "block",
    width: "100%",
    background: "var(--color-surface-2)",
    border: "1px solid var(--color-border)",
    borderRadius: 6,
    padding: "8px 12px",
    color: "var(--color-text)",
    fontSize: 13,
    outline: "none",
    boxSizing: "border-box",
    marginTop: 4,
  };

  const labelStyle: React.CSSProperties = {
    fontSize: 12,
    fontWeight: 500,
    color: "var(--color-muted)",
    display: "flex",
    flexDirection: "column",
    gap: 4,
  };

  const cardStyle: React.CSSProperties = {
    background: "var(--color-surface)",
    border: "1px solid var(--color-border)",
    borderRadius: 12,
    padding: "2.5rem 2rem",
    maxWidth: 560,
    margin: "0 auto",
    display: "flex",
    flexDirection: "column",
    gap: 20,
  };

  const handleSourceSave = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!sourceData.name.trim() || !sourceData.rest_url?.trim()) {
      setError("Name and REST URL are required");
      return;
    }
    setSaving(true);
    setError(null);
    try {
      const src = await adminApi.createSource(sourceData);
      setCreatedSourceId(src.id);
      setStep("verify");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to save source");
    } finally {
      setSaving(false);
    }
  };

  const handleTest = async () => {
    if (!createdSourceId) return;
    setTestStatus("testing");
    try {
      // CR-3: now calls the typed POST /admin/sources/{id}/test endpoint.
      // Server returns AmsSourceStatus; wave-2 implements the real check.
      // 404/501 (not yet implemented) returns synthetic reachable=false gracefully.
      const status = await adminApi.testSource(createdSourceId);
      if (status.reachable) {
        setTestStatus("ok");
        setTestMessage(
          status.latency_ms != null
            ? `Connection verified (${status.latency_ms} ms${status.version ? `, AMS ${status.version}` : ""})`
            : "Connection verified successfully",
        );
      } else {
        setTestStatus("fail");
        setTestMessage(status.error ?? "Source unreachable");
      }
    } catch (err) {
      setTestStatus("fail");
      setTestMessage(err instanceof ApiError ? err.message : "Test failed");
    }
  };

  const steps: Step[] = ["welcome", "source", "verify", "done"];
  const stepIdx = steps.indexOf(step);

  return (
    <div style={{ minHeight: "60vh", display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", padding: "2rem" }}>
      {/* Step indicator */}
      <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 32 }}>
        {["Welcome", "Add source", "Verify", "Done"].map((label, i) => (
          <div key={label} style={{ display: "flex", alignItems: "center", gap: 8 }}>
            <div style={{
              width: 28,
              height: 28,
              borderRadius: "50%",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              background: i <= stepIdx ? "var(--color-accent)" : "var(--color-surface-2)",
              border: `1px solid ${i <= stepIdx ? "var(--color-accent)" : "var(--color-border)"}`,
              fontSize: 12,
              fontWeight: 600,
              color: i <= stepIdx ? "var(--color-on-signal)" : "var(--color-muted)",
            }}>
              {i + 1}
            </div>
            <span style={{ fontSize: 12, color: i <= stepIdx ? "var(--color-text)" : "var(--color-muted)", fontWeight: i === stepIdx ? 600 : 400 }}>
              {label}
            </span>
            {i < 3 && <div style={{ width: 24, height: 1, background: "var(--color-border)" }} />}
          </div>
        ))}
      </div>

      {step === "welcome" && (
        <div style={cardStyle}>
          <div style={{ textAlign: "center" }}>
            <h2 style={{ margin: "0 0 8px", fontSize: 22, fontWeight: 700 }}>Welcome to Pulse</h2>
            <p style={{ margin: 0, color: "var(--color-muted)", fontSize: 14 }}>
              Self-hosted analytics and monitoring for Ant Media Server.
              This wizard will help you connect your first AMS instance in under 15 minutes.
            </p>
          </div>
          <ul style={{ margin: 0, padding: "0 0 0 20px", color: "var(--color-muted)", fontSize: 13, lineHeight: 2 }}>
            <li>Connect your AMS REST API endpoint</li>
            <li>Verify the connection and credentials</li>
            <li>Get your ingest and API tokens</li>
          </ul>
          <button
            onClick={() => setStep("source")}
            style={{
              background: "var(--color-accent)",
              border: "none",
              color: "var(--color-on-signal)",
              borderRadius: 8,
              padding: "12px",
              cursor: "pointer",
              fontSize: 15,
              fontWeight: 600,
            }}
          >
            Get started
          </button>
        </div>
      )}

      {step === "source" && (
        <div style={cardStyle}>
          <h2 style={{ margin: 0, fontSize: 18, fontWeight: 700 }}>Add AMS source</h2>
          <form onSubmit={(e) => void handleSourceSave(e)} style={{ display: "flex", flexDirection: "column", gap: 14 }}>
            <label style={labelStyle}>
              Name *
              <input
                style={inputStyle}
                value={sourceData.name}
                onChange={(e) => setSourceData((d) => ({ ...d, name: e.target.value }))}
                placeholder="Production cluster"
              />
            </label>
            <label style={labelStyle}>
              AMS REST URL *
              <input
                style={inputStyle}
                type="url"
                value={sourceData.rest_url ?? ""}
                onChange={(e) => setSourceData((d) => ({ ...d, rest_url: e.target.value }))}
                placeholder="http://your-ams-server:5080"
              />
              <span style={{ fontSize: 11, color: "var(--color-muted)" }}>
                The base URL of your AMS instance — Pulse calls the REST API at port 5080 by default.
              </span>
            </label>
            <label style={labelStyle}>
              AMS REST username (optional)
              <input
                style={inputStyle}
                value={sourceData.rest_user ?? ""}
                onChange={(e) => setSourceData((d) => ({ ...d, rest_user: e.target.value }))}
                placeholder="admin"
              />
            </label>
            <label style={labelStyle}>
              Credential env var (optional)
              <input
                style={inputStyle}
                value={sourceData.credential_env_ref ?? ""}
                onChange={(e) => setSourceData((d) => ({ ...d, credential_env_ref: e.target.value }))}
                placeholder="AMS_ADMIN_PASSWORD"
              />
              <span style={{ fontSize: 11, color: "var(--color-muted)" }}>
                Environment variable name holding the AMS password — never stored plaintext.
              </span>
            </label>
            <label style={labelStyle}>
              Log path (optional — for log_tail mode)
              <input
                style={inputStyle}
                value={sourceData.log_path ?? ""}
                onChange={(e) => setSourceData((d) => ({ ...d, log_path: e.target.value }))}
                placeholder="/var/log/ant-media-server/ant-media-server.log"
              />
            </label>
            {error && <p style={{ margin: 0, fontSize: 12, color: "var(--color-error)" }}>{error}</p>}
            <div style={{ display: "flex", gap: 10, justifyContent: "space-between" }}>
              <button
                type="button"
                onClick={() => setStep("welcome")}
                style={{
                  background: "var(--color-surface-2)",
                  border: "1px solid var(--color-border)",
                  color: "var(--color-muted)",
                  borderRadius: 6,
                  padding: "8px 16px",
                  cursor: "pointer",
                  fontSize: 13,
                }}
              >
                Back
              </button>
              <button
                type="submit"
                disabled={saving}
                style={{
                  background: "var(--color-accent)",
                  border: "none",
                  color: "var(--color-on-signal)",
                  borderRadius: 6,
                  padding: "8px 20px",
                  cursor: "pointer",
                  fontSize: 13,
                  fontWeight: 600,
                  opacity: saving ? 0.7 : 1,
                }}
              >
                {saving ? "Saving…" : "Add source"}
              </button>
            </div>
          </form>
        </div>
      )}

      {step === "verify" && (
        <div style={cardStyle}>
          <h2 style={{ margin: 0, fontSize: 18, fontWeight: 700 }}>Verify connection</h2>
          <p style={{ margin: 0, color: "var(--color-muted)", fontSize: 13 }}>
            Source created. Test the connection to confirm Pulse can reach your AMS instance.
          </p>
          {testStatus === "idle" && (
            <button
              onClick={() => void handleTest()}
              style={{
                background: "var(--color-accent)",
                border: "none",
                color: "var(--color-on-signal)",
                borderRadius: 8,
                padding: "10px 20px",
                cursor: "pointer",
                fontSize: 13,
                fontWeight: 600,
              }}
            >
              Test connection
            </button>
          )}
          {testStatus === "testing" && <LoadingSpinner label="Testing connection…" />}
          {testStatus === "ok" && (
            <div style={{ background: "rgba(44,229,167,0.1)", border: "1px solid var(--color-success)", borderRadius: 8, padding: "12px 16px", color: "var(--color-success)", fontSize: 13 }}>
              {testMessage}
            </div>
          )}
          {testStatus === "fail" && (
            <div style={{ background: "var(--color-error-bg)", border: "1px solid var(--color-error)", borderRadius: 8, padding: "12px 16px", color: "var(--color-error)", fontSize: 13 }}>
              {testMessage}
            </div>
          )}
          <div style={{ display: "flex", gap: 10, justifyContent: "space-between" }}>
            <button
              onClick={() => setStep("source")}
              style={{
                background: "var(--color-surface-2)",
                border: "1px solid var(--color-border)",
                color: "var(--color-muted)",
                borderRadius: 6,
                padding: "8px 16px",
                cursor: "pointer",
                fontSize: 13,
              }}
            >
              Back
            </button>
            <button
              onClick={() => setStep("done")}
              style={{
                background: testStatus === "ok" ? "var(--color-success)" : "var(--color-surface-2)",
                border: `1px solid ${testStatus === "ok" ? "var(--color-success)" : "var(--color-border)"}`,
                color: testStatus === "ok" ? "var(--color-on-signal)" : "var(--color-muted)",
                borderRadius: 6,
                padding: "8px 20px",
                cursor: "pointer",
                fontSize: 13,
                fontWeight: 600,
              }}
            >
              Continue
            </button>
          </div>
        </div>
      )}

      {step === "done" && (
        <div style={cardStyle}>
          <div style={{ textAlign: "center" }}>
            <div style={{ fontSize: 48, marginBottom: 12 }}>&#10003;</div>
            <h2 style={{ margin: "0 0 8px", fontSize: 20, fontWeight: 700 }}>You are connected!</h2>
            <p style={{ margin: 0, color: "var(--color-muted)", fontSize: 14 }}>
              Pulse is now collecting data from your AMS source. Head to the live dashboard to see streams.
            </p>
          </div>
          <button
            onClick={onComplete}
            style={{
              background: "var(--color-accent)",
              border: "none",
              color: "var(--color-on-signal)",
              borderRadius: 8,
              padding: "12px",
              cursor: "pointer",
              fontSize: 15,
              fontWeight: 600,
            }}
          >
            Go to live dashboard
          </button>
        </div>
      )}
    </div>
  );
}
