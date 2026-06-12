import { useState } from "react";
import type { AlertChannel, AlertChannelWrite } from "@/lib/api/types";

interface Props {
  initial?: AlertChannel;
  onSave: (data: AlertChannelWrite) => Promise<void>;
  onCancel: () => void;
}

type ChannelType = "email" | "slack" | "webhook" | "pagerduty" | "telegram";

const CHANNEL_TYPES: ChannelType[] = ["email", "slack", "webhook", "pagerduty", "telegram"];

export function AlertChannelForm({ initial, onSave, onCancel }: Props) {
  const [name, setName] = useState(initial?.name ?? "");
  const [type, setType] = useState<ChannelType>((initial?.type ?? "email") as ChannelType);
  // Derive initial config values from config_summary (non-secret display fields)
  const summary = initial?.config_summary ?? {};
  const [emailTo, setEmailTo] = useState((summary.email_to as string) ?? "");
  const [webhookUrl, setWebhookUrl] = useState(
    (summary.slack_webhook_url as string) ?? (summary.webhook_url as string) ?? ""
  );
  const [saving, setSaving] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});

  const validate = () => {
    const errs: Record<string, string> = {};
    if (!name.trim()) errs.name = "Name is required";
    if (type === "email" && !emailTo.trim()) errs.emailTo = "Email address required";
    if ((type === "slack" || type === "webhook") && !webhookUrl.trim()) errs.webhookUrl = "URL required";
    setErrors(errs);
    return Object.keys(errs).length === 0;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!validate()) return;
    setSaving(true);
    try {
      // AlertChannelConfig discriminated union — build per selected type
      let config: AlertChannelWrite["config"] = {};
      if (type === "email") config = { email_to: emailTo.trim() };
      if (type === "slack") config = { slack_webhook_url: webhookUrl.trim() };
      if (type === "webhook") config = { webhook_url: webhookUrl.trim() };
      await onSave({ name: name.trim(), type, config });
    } finally {
      setSaving(false);
    }
  };

  const inputStyle: React.CSSProperties = {
    background: "var(--color-surface-2)",
    border: "1px solid var(--color-border)",
    borderRadius: 6,
    padding: "7px 10px",
    color: "var(--color-text)",
    fontSize: 13,
    width: "100%",
    outline: "none",
    boxSizing: "border-box",
  };

  const labelStyle: React.CSSProperties = {
    fontSize: 12,
    fontWeight: 500,
    color: "var(--color-muted)",
    display: "flex",
    flexDirection: "column",
    gap: 4,
  };

  return (
    <form onSubmit={(e) => void handleSubmit(e)} style={{ display: "flex", flexDirection: "column", gap: 14 }}>
      <h3 style={{ margin: 0, fontSize: 15, fontWeight: 600 }}>{initial ? "Edit channel" : "New notification channel"}</h3>

      <label style={labelStyle}>
        Channel name *
        <input
          style={{ ...inputStyle, borderColor: errors.name ? "var(--color-error)" : "var(--color-border)" }}
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="e.g. Ops team Slack"
        />
        {errors.name && <span style={{ fontSize: 11, color: "var(--color-error)" }}>{errors.name}</span>}
      </label>

      <label style={labelStyle}>
        Type
        <select style={inputStyle} value={type} onChange={(e) => setType(e.target.value as ChannelType)}>
          {CHANNEL_TYPES.map((t) => <option key={t} value={t}>{t}</option>)}
        </select>
      </label>

      {type === "email" && (
        <label style={labelStyle}>
          To address *
          <input
            type="email"
            style={{ ...inputStyle, borderColor: errors.emailTo ? "var(--color-error)" : "var(--color-border)" }}
            value={emailTo}
            onChange={(e) => setEmailTo(e.target.value)}
            placeholder="alerts@example.com"
          />
          {errors.emailTo && <span style={{ fontSize: 11, color: "var(--color-error)" }}>{errors.emailTo}</span>}
        </label>
      )}

      {(type === "slack" || type === "webhook") && (
        <label style={labelStyle}>
          {type === "slack" ? "Slack webhook URL *" : "Webhook URL *"}
          <input
            type="url"
            style={{ ...inputStyle, borderColor: errors.webhookUrl ? "var(--color-error)" : "var(--color-border)" }}
            value={webhookUrl}
            onChange={(e) => setWebhookUrl(e.target.value)}
            placeholder="https://hooks.slack.com/services/…"
          />
          {errors.webhookUrl && <span style={{ fontSize: 11, color: "var(--color-error)" }}>{errors.webhookUrl}</span>}
        </label>
      )}

      {(type === "pagerduty" || type === "telegram") && (
        <p style={{ margin: 0, fontSize: 12, color: "var(--color-muted)" }}>
          Configure via environment variables — see documentation.
        </p>
      )}

      <div style={{ display: "flex", gap: 10, justifyContent: "flex-end", paddingTop: 4 }}>
        <button
          type="button"
          onClick={onCancel}
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
          Cancel
        </button>
        <button
          type="submit"
          disabled={saving}
          style={{
            background: "var(--color-accent)",
            border: "none",
            color: "#fff",
            borderRadius: 6,
            padding: "8px 20px",
            cursor: "pointer",
            fontSize: 13,
            fontWeight: 600,
            opacity: saving ? 0.7 : 1,
          }}
        >
          {saving ? "Saving…" : "Save channel"}
        </button>
      </div>
    </form>
  );
}
