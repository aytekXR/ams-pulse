/**
 * F6 — Usage & Billing Reports (/reports)
 *
 * Usage view: per app/stream/tenant table (viewer-minutes, peak concurrency,
 * egress GB with method tooltip, recording GB; date range).
 * Statement generation + download (CSV/PDF).
 * Schedules CRUD (cron presets: monthly/weekly + raw cron field).
 * Tenants CRUD (pattern/tag mapping with live preview of matched streams).
 * Gate-aware: Business-tier upsell when entitlement missing.
 */
import { useState, useEffect, useCallback } from "react";
import { reportsApi, adminApi, ApiError } from "@/api/client";
import { DateRangePicker, defaultDateRange } from "@/features/analytics/DateRangePicker";
import { Tabs } from "@/components/Tabs";
import { LoadingSpinner } from "@/components/LoadingSpinner";
import { ErrorBanner } from "@/components/ErrorBanner";
import { EmptyState } from "@/components/EmptyState";
import { Badge } from "@/components/Badge";
import { StatCard } from "@/features/live/StatCard";
import { useToast } from "@/components/Toast";
import { TierGate } from "@/components/TierGate";
import { canUseReports } from "@/lib/entitlements";
import type {
  UsageReportResponse,
  ReportSchedule,
  ReportScheduleWrite,
  Tenant,
  TenantWrite,
  LicenseInfo,
} from "@/lib/api/types";

type Tab = "usage" | "schedules" | "tenants";

// ─── Schedule form ────────────────────────────────────────────────────────────

const CRON_PRESETS = [
  { label: "Monthly (1st of month, 6 AM UTC)", value: "0 6 1 * *" },
  { label: "Weekly (Monday, 6 AM UTC)", value: "0 6 * * 1" },
  { label: "Daily (6 AM UTC)", value: "0 6 * * *" },
  { label: "Custom…", value: "custom" },
];

interface ScheduleFormProps {
  initial?: ReportSchedule;
  onSave: (data: ReportScheduleWrite) => Promise<void>;
  onCancel: () => void;
}

function ScheduleForm({ initial, onSave, onCancel }: ScheduleFormProps) {
  const [cronPreset, setCronPreset] = useState(() => {
    if (!initial) return CRON_PRESETS[0].value;
    const match = CRON_PRESETS.find((p) => p.value === initial.cron);
    return match ? match.value : "custom";
  });
  const [cronRaw, setCronRaw] = useState(initial?.cron ?? CRON_PRESETS[0].value);
  const [format, setFormat] = useState<"csv" | "pdf">(initial?.format ?? "csv");
  const [appScope, setAppScope] = useState(initial?.scope?.app ?? "");
  const [tenantScope, setTenantScope] = useState(initial?.scope?.tenant ?? "");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const effectiveCron = cronPreset === "custom" ? cronRaw : cronPreset;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const cron = effectiveCron.trim();
    if (!cron) {
      setError("Cron expression is required");
      return;
    }
    setSaving(true);
    setError(null);
    try {
      await onSave({
        cron,
        format,
        scope: {
          app: appScope || null,
          tenant: tenantScope || null,
        },
      });
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to save schedule");
      setSaving(false);
    }
  };

  // s111 D4-pattern: outline:"none" removed — inputs carry
  // className="filter-input" so the shared :focus-visible ring applies.
  const inputStyle: React.CSSProperties = {
    background: "var(--color-surface-2)",
    border: "1px solid var(--color-border)",
    borderRadius: "var(--radius-control)",
    padding: "7px 10px",
    color: "var(--color-text)",
    fontSize: 13,
    width: "100%",
    boxSizing: "border-box",
  };

  const labelStyle: React.CSSProperties = {
    display: "flex",
    flexDirection: "column",
    gap: "var(--space-1)",
    fontSize: 12,
    color: "var(--color-secondary)",
    fontWeight: 500,
  };

  return (
    <form onSubmit={(e) => void handleSubmit(e)} className="panel-enter" style={{
      background: "var(--color-surface)",
      border: "1px solid var(--color-border)",
      borderRadius: "var(--radius-card)",
      padding: "var(--space-5)",
      display: "flex",
      flexDirection: "column",
      gap: 14,
    }}>
      <h3 style={{ margin: 0, fontSize: 14, fontWeight: 700 }}>
        {initial ? "Edit schedule" : "New schedule"}
      </h3>

      {error && <ErrorBanner message={error} />}

      {/* Cron preset */}
      <label style={labelStyle}>
        Frequency
        <select
          value={cronPreset}
          onChange={(e) => {
            setCronPreset(e.target.value);
            if (e.target.value !== "custom") setCronRaw(e.target.value);
          }}
          className="filter-input" style={inputStyle}
        >
          {CRON_PRESETS.map((p) => (
            <option key={p.value} value={p.value}>{p.label}</option>
          ))}
        </select>
      </label>

      {/* Raw cron */}
      {cronPreset === "custom" && (
        <label style={labelStyle}>
          Cron expression (UTC)
          <input
            type="text"
            value={cronRaw}
            onChange={(e) => setCronRaw(e.target.value)}
            placeholder="0 6 1 * *"
            className="filter-input" style={inputStyle}
          />
        </label>
      )}

      {/* Format */}
      <label style={labelStyle}>
        Format
        <select value={format} onChange={(e) => setFormat(e.target.value as "csv" | "pdf")} className="filter-input" style={inputStyle}>
          <option value="csv">CSV</option>
          <option value="pdf">PDF</option>
        </select>
      </label>

      {/* Scope */}
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 10 }}>
        <label style={labelStyle}>
          App scope (optional)
          <input
            type="text"
            value={appScope}
            onChange={(e) => setAppScope(e.target.value)}
            placeholder="e.g. live"
            className="filter-input" style={inputStyle}
          />
        </label>
        <label style={labelStyle}>
          Tenant scope (optional)
          <input
            type="text"
            value={tenantScope}
            onChange={(e) => setTenantScope(e.target.value)}
            placeholder="e.g. acme-corp"
            className="filter-input" style={inputStyle}
          />
        </label>
      </div>

      <div style={{ display: "flex", gap: 10, justifyContent: "flex-end" }}>
        <button
          type="button"
          onClick={onCancel}
          className="btn-secondary"
          style={{
            background: "none",
            borderRadius: "var(--radius-control)",
            padding: "var(--space-2) var(--space-4)",
            cursor: "pointer",
            fontSize: 13,
          }}
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={saving}
          className="btn-primary"
          style={{
            border: "none",
            color: "var(--color-on-signal)",
            borderRadius: "var(--radius-control)",
            padding: "var(--space-2) var(--space-4)",
            fontSize: 13,
            fontWeight: 600,
          }}
        >
          {saving ? "Saving…" : "Save schedule"}
        </button>
      </div>
    </form>
  );
}

// ─── Tenant form ──────────────────────────────────────────────────────────────

interface TenantFormProps {
  initial?: Tenant;
  onSave: (data: TenantWrite) => Promise<void>;
  onCancel: () => void;
}

function TenantForm({ initial, onSave, onCancel }: TenantFormProps) {
  const [name, setName] = useState(initial?.name ?? "");
  const [streamPattern, setStreamPattern] = useState(initial?.stream_pattern ?? "");
  const [metaTagKey, setMetaTagKey] = useState(initial?.meta_tag_key ?? "");
  const [metaTagValue, setMetaTagValue] = useState(initial?.meta_tag_value ?? "");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // s111 D4-pattern: outline:"none" removed — inputs carry
  // className="filter-input" so the shared :focus-visible ring applies.
  const inputStyle: React.CSSProperties = {
    background: "var(--color-surface-2)",
    border: "1px solid var(--color-border)",
    borderRadius: "var(--radius-control)",
    padding: "7px 10px",
    color: "var(--color-text)",
    fontSize: 13,
    width: "100%",
    boxSizing: "border-box",
  };

  const labelStyle: React.CSSProperties = {
    display: "flex",
    flexDirection: "column",
    gap: "var(--space-1)",
    fontSize: 12,
    color: "var(--color-secondary)",
    fontWeight: 500,
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) {
      setError("Name is required");
      return;
    }
    const hasPattern = streamPattern.trim().length > 0;
    const hasTag = metaTagKey.trim().length > 0 && metaTagValue.trim().length > 0;
    if (!hasPattern && !hasTag) {
      setError("At least one matcher is required: stream pattern OR both meta tag key and value");
      return;
    }
    setSaving(true);
    setError(null);
    try {
      await onSave({
        name: name.trim(),
        stream_pattern: streamPattern.trim() || null,
        meta_tag_key: metaTagKey.trim() || null,
        meta_tag_value: metaTagValue.trim() || null,
      });
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to save tenant");
      setSaving(false);
    }
  };

  return (
    <form
      onSubmit={(e) => void handleSubmit(e)}
      data-testid="tenant-form"
      className="panel-enter"
      style={{
        background: "var(--color-surface)",
        border: "1px solid var(--color-border)",
        borderRadius: "var(--radius-card)",
        padding: "var(--space-5)",
        display: "flex",
        flexDirection: "column",
        gap: 14,
      }}
    >
      <h3 style={{ margin: 0, fontSize: 14, fontWeight: 700 }}>
        {initial ? "Edit tenant" : "New tenant"}
      </h3>

      {error && <ErrorBanner message={error} />}

      {/* Name */}
      <label style={labelStyle}>
        Name *
        <input
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="e.g. Acme Corp"
          className="filter-input" style={inputStyle}
          aria-label="Tenant name"
        />
      </label>

      <div style={{
        background: "var(--color-surface-2)",
        borderRadius: "var(--radius-control)",
        padding: "var(--space-2) var(--space-3)",
        fontSize: 12,
        color: "var(--color-secondary)",
      }}>
        Provide at least one matcher: a stream pattern OR both meta tag key and value.
      </div>

      {/* Stream pattern */}
      <label style={labelStyle}>
        Stream pattern (SQL LIKE / regex on stream_id)
        <input
          type="text"
          value={streamPattern}
          onChange={(e) => setStreamPattern(e.target.value)}
          placeholder="e.g. live/acme-% or ^live/acme"
          className="filter-input" style={inputStyle}
          aria-label="Stream pattern"
        />
      </label>

      {/* Meta tag key + value */}
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 10 }}>
        <label style={labelStyle}>
          Meta tag key
          <input
            type="text"
            value={metaTagKey}
            onChange={(e) => setMetaTagKey(e.target.value)}
            placeholder="e.g. tenant_id"
            className="filter-input" style={inputStyle}
            aria-label="Meta tag key"
          />
        </label>
        <label style={labelStyle}>
          Meta tag value
          <input
            type="text"
            value={metaTagValue}
            onChange={(e) => setMetaTagValue(e.target.value)}
            placeholder="e.g. acme"
            className="filter-input" style={inputStyle}
            aria-label="Meta tag value"
          />
        </label>
      </div>

      <div style={{ display: "flex", gap: 10, justifyContent: "flex-end" }}>
        <button
          type="button"
          onClick={onCancel}
          className="btn-secondary"
          style={{
            background: "none",
            borderRadius: "var(--radius-control)",
            padding: "var(--space-2) var(--space-4)",
            cursor: "pointer",
            fontSize: 13,
          }}
        >
          Cancel
        </button>
        <button
          type="submit"
          disabled={saving}
          className="btn-primary"
          style={{
            border: "none",
            color: "var(--color-on-signal)",
            borderRadius: "var(--radius-control)",
            padding: "var(--space-2) var(--space-4)",
            fontSize: 13,
            fontWeight: 600,
          }}
        >
          {saving ? "Saving…" : "Save tenant"}
        </button>
      </div>
    </form>
  );
}

// ─── Delete confirm dialog ─────────────────────────────────────────────────────

interface DeleteConfirmProps {
  tenant: Tenant;
  onConfirm: () => void;
  onCancel: () => void;
  deleting: boolean;
}

function DeleteConfirm({ tenant, onConfirm, onCancel, deleting }: DeleteConfirmProps) {
  return (
    <div
      data-testid="delete-confirm"
      className="panel-enter"
      style={{
        background: "var(--color-surface)",
        border: "1px solid var(--color-border)",
        borderRadius: "var(--radius-card)",
        padding: "var(--space-5)",
        display: "flex",
        flexDirection: "column",
        gap: 14,
      }}
    >
      <h3 style={{ margin: 0, fontSize: 14, fontWeight: 700, color: "var(--color-error)" }}>
        Delete tenant
      </h3>
      <p style={{ margin: 0, fontSize: 13, color: "var(--color-secondary)" }}>
        Are you sure you want to delete <strong style={{ color: "var(--color-text)" }}>{tenant.name}</strong>?
        Existing usage rows will retain the tenant label but no new streams will be matched.
      </p>
      <div style={{ display: "flex", gap: 10, justifyContent: "flex-end" }}>
        <button
          type="button"
          onClick={onCancel}
          className="btn-secondary"
          style={{
            background: "none",
            borderRadius: "var(--radius-control)",
            padding: "var(--space-2) var(--space-4)",
            cursor: "pointer",
            fontSize: 13,
          }}
        >
          Cancel
        </button>
        <button
          type="button"
          onClick={onConfirm}
          disabled={deleting}
          className="btn-press"
          style={{
            background: "var(--color-error-bg)",
            border: "1px solid rgba(255,92,104,0.4)",
            color: "var(--color-error)",
            borderRadius: "var(--radius-control)",
            padding: "var(--space-2) var(--space-4)",
            fontSize: 13,
            fontWeight: 600,
          }}
        >
          {deleting ? "Deleting…" : "Delete"}
        </button>
      </div>
    </div>
  );
}

// ─── Tenants tab ───────────────────────────────────────────────────────────────

interface TenantsTabProps {
  onToast: (msg: string, variant: "success" | "info" | "error") => void;
}

function TenantsTab({ onToast }: TenantsTabProps) {
  const [tenants, setTenants] = useState<Tenant[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [editTenant, setEditTenant] = useState<Tenant | null>(null);
  const [deleteTenant, setDeleteTenant] = useState<Tenant | null>(null);
  const [deleting, setDeleting] = useState(false);

  const loadTenants = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await adminApi.listTenants();
      setTenants(result.items ?? []);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to load tenants");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadTenants();
  }, [loadTenants]);

  const handleCreate = async (data: TenantWrite) => {
    await adminApi.createTenant(data);
    onToast("Tenant created", "success");
    setShowForm(false);
    void loadTenants();
  };

  const handleUpdate = async (id: string, data: TenantWrite) => {
    await adminApi.updateTenant(id, data);
    onToast("Tenant updated", "success");
    setEditTenant(null);
    void loadTenants();
  };

  const handleDelete = async () => {
    if (!deleteTenant) return;
    setDeleting(true);
    try {
      await adminApi.deleteTenant(deleteTenant.id);
      onToast("Tenant deleted", "info");
      setDeleteTenant(null);
      void loadTenants();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to delete tenant");
    } finally {
      setDeleting(false);
    }
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-4)" }}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
        <div style={{ fontSize: 13, color: "var(--color-secondary)" }}>
          Tenants match streams by pattern or beacon meta tag for billing reconciliation.
        </div>
        {/* s111 D20: '+' glyph dropped — the label carries the meaning alone. */}
        <button
          onClick={() => { setShowForm(true); setEditTenant(null); }}
          className="btn-primary"
          style={{
            border: "none",
            color: "var(--color-on-signal)",
            borderRadius: "var(--radius-control)",
            padding: "var(--space-2) var(--space-4)",
            cursor: "pointer",
            fontSize: 12,
            fontWeight: 600,
          }}
        >
          New tenant
        </button>
      </div>

      {error && <ErrorBanner message={error} onRetry={loadTenants} />}

      {showForm && !editTenant && (
        <TenantForm
          onSave={handleCreate}
          onCancel={() => setShowForm(false)}
        />
      )}

      {deleteTenant && (
        <DeleteConfirm
          tenant={deleteTenant}
          onConfirm={() => void handleDelete()}
          onCancel={() => setDeleteTenant(null)}
          deleting={deleting}
        />
      )}

      {loading ? (
        <LoadingSpinner label="Loading tenants…" />
      ) : tenants.length === 0 && !showForm ? (
        <EmptyState
          title="No tenants configured"
          description="Add a tenant to map streams to billing entities. Unmatched streams appear as 'unassigned' in usage reports."
        />
      ) : (
        <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-3)" }}>
          {tenants.map((tenant) => (
            <div key={tenant.id}>
              {editTenant?.id === tenant.id ? (
                <TenantForm
                  initial={tenant}
                  onSave={(data) => handleUpdate(tenant.id, data)}
                  onCancel={() => setEditTenant(null)}
                />
              ) : (
                <div
                  data-testid={`tenant-row-${tenant.id}`}
                  style={{
                    background: "var(--color-surface)",
                    border: "1px solid var(--color-border)",
                    borderRadius: "var(--radius-card)",
                    padding: "14px 16px",
                    display: "flex",
                    alignItems: "flex-start",
                    gap: "var(--space-3)",
                  }}
                >
                  <div style={{ flex: 1 }}>
                    <div style={{ fontWeight: 600, fontSize: 14, marginBottom: 6 }}>
                      {tenant.name}
                    </div>
                    <div style={{ display: "flex", flexWrap: "wrap", gap: "var(--space-2)" }}>
                      {tenant.stream_pattern && (
                        <span style={{
                          background: "var(--color-surface-2)",
                          border: "1px solid var(--color-border)",
                          borderRadius: "var(--radius-pill)",
                          padding: "2px 8px",
                          fontSize: 11,
                          fontFamily: "var(--font-mono)",
                          color: "var(--color-secondary)",
                        }}>
                          pattern: {tenant.stream_pattern}
                        </span>
                      )}
                      {tenant.meta_tag_key && tenant.meta_tag_value && (
                        <span style={{
                          background: "var(--color-surface-2)",
                          border: "1px solid var(--color-border)",
                          borderRadius: "var(--radius-pill)",
                          padding: "2px 8px",
                          fontSize: 11,
                          fontFamily: "var(--font-mono)",
                          color: "var(--color-secondary)",
                        }}>
                          {tenant.meta_tag_key}={tenant.meta_tag_value}
                        </span>
                      )}
                    </div>
                    <div style={{ fontSize: 11, color: "var(--color-secondary)", marginTop: 6 }}>
                      Created {new Date(tenant.created_at).toLocaleDateString()}
                    </div>
                  </div>
                  <div style={{ display: "flex", gap: 6, flexShrink: 0 }}>
                    <button
                      onClick={() => { setEditTenant(tenant); setShowForm(false); }}
                      aria-label={`Edit ${tenant.name}`}
                      className="btn-secondary"
                      style={{
                        background: "none",
                        borderRadius: "var(--radius-control)",
                        padding: "6px 10px",
                        minHeight: 28,
                        cursor: "pointer",
                        fontSize: 11,
                      }}
                    >
                      Edit
                    </button>
                    <button
                      onClick={() => setDeleteTenant(tenant)}
                      aria-label={`Delete ${tenant.name}`}
                      className="btn-press"
                      style={{
                        background: "none",
                        border: "1px solid var(--color-error)",
                        color: "var(--color-error)",
                        borderRadius: "var(--radius-control)",
                        padding: "6px 10px",
                        minHeight: 28,
                        cursor: "pointer",
                        fontSize: 11,
                      }}
                    >
                      Delete
                    </button>
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

// ─── Main page ────────────────────────────────────────────────────────────────

export function ReportsPage() {
  const { toast } = useToast();
  const [tab, setTab] = useState<Tab>("usage");
  const [range, setRange] = useState(defaultDateRange);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [usage, setUsage] = useState<UsageReportResponse | null>(null);
  const [schedules, setSchedules] = useState<ReportSchedule[]>([]);
  const [license, setLicense] = useState<LicenseInfo | null>(null);
  const [licenseLoading, setLicenseLoading] = useState(true);
  const [showScheduleForm, setShowScheduleForm] = useState(false);
  const [editSchedule, setEditSchedule] = useState<ReportSchedule | null>(null);

  // Load license tier first
  useEffect(() => {
    adminApi.getLicense()
      .then((l) => setLicense(l))
      .catch(() => setLicense(null))
      .finally(() => setLicenseLoading(false));
  }, []);

  // VD-01: Reports require Business tier or higher (PRD §7.11). Gate: free and pro → upsell.
  // The rule lives in src/lib/entitlements (mirroring server CheckReports) so the
  // entitlement test can import it instead of keeping its own copy.
  const isGated = license != null && !canUseReports(license.tier);

  const loadUsage = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await reportsApi.getUsage({ from: range.from, to: range.to });
      setUsage(result);
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : "Failed to load usage report";
      setError(msg);
    } finally {
      setLoading(false);
    }
  }, [range]);

  const loadSchedules = useCallback(async () => {
    try {
      const result = await reportsApi.listSchedules();
      setSchedules(result.items ?? []);
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : "Failed to load schedules";
      setError(msg);
    }
  }, []);

  useEffect(() => {
    if (!isGated) {
      if (tab === "usage") void loadUsage();
      if (tab === "schedules") void loadSchedules();
    }
  }, [tab, isGated, loadUsage, loadSchedules]);

  const downloadCsv = () => {
    reportsApi.downloadExport({ from: range.from, to: range.to, format: "csv" });
  };

  const createSchedule = async (data: ReportScheduleWrite) => {
    await reportsApi.createSchedule(data);
    toast("Schedule created", "success");
    setShowScheduleForm(false);
    void loadSchedules();
  };

  const updateSchedule = async (id: string, data: ReportScheduleWrite) => {
    await reportsApi.updateSchedule(id, data);
    toast("Schedule updated", "success");
    setEditSchedule(null);
    void loadSchedules();
  };

  const deleteSchedule = async (id: string) => {
    if (!confirm("Delete this schedule?")) return;
    await reportsApi.deleteSchedule(id);
    toast("Schedule deleted", "info");
    void loadSchedules();
  };

  if (licenseLoading) {
    return <LoadingSpinner label="Checking license…" />;
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-5)" }}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", gap: "var(--space-3)" }}>
        <h1 className="page-title">Reports</h1>
        {license && (
          <Badge
            label={license.tier}
            variant={license.tier === "enterprise" ? "success" : license.tier === "pro" ? "info" : "muted"}
          />
        )}
      </div>

      {/* Gate check */}
      {isGated ? (
        <TierGate
          icon={
            <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="var(--color-accent)" strokeWidth="1.5" aria-hidden>
              <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" />
            </svg>
          }
          heading="Usage Reports requires Business tier"
          tier={license?.tier ?? "free"}
          upgradeText="Upgrade to Business to unlock usage reports, scheduled exports, and tenant mapping."
          descriptionColor="var(--color-secondary)"
        />
      ) : (
        <>
          {/* Tabs */}
          <Tabs
            tabs={[
              { id: "usage", label: "Usage" },
              { id: "schedules", label: "Schedules" },
              { id: "tenants", label: "Tenants" },
            ]}
            activeTab={tab}
            onTabChange={(id) => setTab(id as Tab)}
          />

          {error && tab !== "tenants" && <ErrorBanner message={error} onRetry={tab === "usage" ? loadUsage : loadSchedules} />}

          {/* ── Usage tab ── */}
          {tab === "usage" && (
            <div
              role="tabpanel"
              id="tabpanel-usage"
              aria-labelledby="tab-usage"
              style={{ display: "flex", flexDirection: "column", gap: "var(--space-4)" }}
            >
              <div style={{ display: "flex", flexWrap: "wrap", gap: "var(--space-3)", alignItems: "flex-end" }}>
                <DateRangePicker value={range} onChange={setRange} />
                <div style={{ display: "flex", gap: "var(--space-2)" }}>
                  <button
                    onClick={downloadCsv}
                    className="btn-secondary"
                    style={{
                      background: "var(--color-surface-2)",
                      borderRadius: "var(--radius-control)",
                      padding: "6px 12px",
                      minHeight: 28,
                      cursor: "pointer",
                      fontSize: 12,
                    }}
                  >
                    Export CSV
                  </button>
                </div>
              </div>

              {loading ? (
                <LoadingSpinner label="Generating usage report…" />
              ) : !usage ? (
                <EmptyState title="No usage data" description="Select a date range and data will appear here." />
              ) : (
                <>
                  {/* Egress method notice */}
                  <div style={{
                    background: "var(--color-surface-2)",
                    borderRadius: "var(--radius-control)",
                    padding: "var(--space-2) var(--space-4)",
                    fontSize: 12,
                    color: "var(--color-secondary)",
                  }}>
                    Egress estimation method: <strong style={{ color: "var(--color-text)" }}>{usage.egress_method}</strong>
                  </div>

                  {/* Totals */}
                  {/* s111 D9: shared <StatCard size="compact"> — same totals-row
                      concept as Analytics; kills the fourth ad-hoc metric size (22). */}
                  <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(160px, 1fr))", gap: "var(--space-3)" }}>
                    {[
                      { label: "Viewer-Minutes", value: usage.totals.viewer_minutes.toFixed(0) },
                      { label: "Peak Concurrency", value: usage.totals.peak_concurrency.toLocaleString() },
                      { label: "Egress GB", value: usage.totals.egress_gb.toFixed(2) },
                      { label: "Recording GB", value: usage.totals.recording_gb.toFixed(2) },
                    ].map(({ label, value }) => (
                      <StatCard key={label} size="compact" label={label} value={value} />
                    ))}
                  </div>

                  {/* Per-row table */}
                  {usage.rows.length === 0 ? (
                    <EmptyState title="No rows in range" description="Try widening the date range." />
                  ) : (
                    <div style={{ background: "var(--color-surface)", border: "1px solid var(--color-border)", borderRadius: "var(--radius-card)", overflow: "hidden" }}>
                      <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
                        <thead style={{ background: "var(--color-surface-2)" }}>
                          <tr>
                            {["App", "Stream", "Tenant", "Viewer-Min", "Peak", "Egress GB", "Recording GB"].map((h) => (
                              <th key={h} className="label" style={{
                                padding: "var(--space-3) var(--space-4)",
                                textAlign: h === "App" || h === "Stream" || h === "Tenant" ? "left" : "right",
                              }}>
                                {h === "Egress GB" ? (
                                  <span title={`Estimation method: ${usage.egress_method}`} style={{ cursor: "help", borderBottom: "1px dotted var(--color-muted)" }}>
                                    Egress GB *
                                  </span>
                                ) : h}
                              </th>
                            ))}
                          </tr>
                        </thead>
                        <tbody>
                          {usage.rows.map((row, i) => (
                            <tr key={i} style={{ borderTop: i === 0 ? "none" : "1px solid var(--color-border)" }}>
                              <td style={{ padding: "var(--cell-pad)" }}>{row.app}</td>
                              <td style={{ padding: "var(--cell-pad)", color: "var(--color-secondary)", fontSize: 12, fontFamily: "var(--font-mono)" }}>{row.stream_id ?? "—"}</td>
                              <td style={{ padding: "var(--cell-pad)", color: "var(--color-secondary)", fontSize: 12 }}>{row.tenant ?? "—"}</td>
                              <td data-numeric style={{ padding: "var(--cell-pad)", textAlign: "right" }}>{row.viewer_minutes.toFixed(0)}</td>
                              <td data-numeric style={{ padding: "var(--cell-pad)", textAlign: "right" }}>{row.peak_concurrency.toLocaleString()}</td>
                              <td data-numeric style={{ padding: "var(--cell-pad)", textAlign: "right" }}>{row.egress_gb.toFixed(2)}</td>
                              <td data-numeric style={{ padding: "var(--cell-pad)", textAlign: "right" }}>{row.recording_gb.toFixed(2)}</td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  )}
                </>
              )}
            </div>
          )}

          {/* ── Tenants tab ── */}
          {tab === "tenants" && (
            <div
              role="tabpanel"
              id="tabpanel-tenants"
              aria-labelledby="tab-tenants"
            >
              <TenantsTab onToast={toast} />
            </div>
          )}

          {/* ── Schedules tab ── */}
          {tab === "schedules" && (
            <div
              role="tabpanel"
              id="tabpanel-schedules"
              aria-labelledby="tab-schedules"
              style={{ display: "flex", flexDirection: "column", gap: "var(--space-4)" }}
            >
              <div style={{ display: "flex", justifyContent: "flex-end" }}>
                {/* s111 D20: '+' glyph dropped — the label carries the meaning alone. */}
                <button
                  onClick={() => { setShowScheduleForm(true); setEditSchedule(null); }}
                  className="btn-primary"
                  style={{
                    border: "none",
                    color: "var(--color-on-signal)",
                    borderRadius: "var(--radius-control)",
                    padding: "var(--space-2) var(--space-4)",
                    cursor: "pointer",
                    fontSize: 12,
                    fontWeight: 600,
                  }}
                >
                  New schedule
                </button>
              </div>

              {(showScheduleForm && !editSchedule) && (
                <ScheduleForm
                  onSave={createSchedule}
                  onCancel={() => setShowScheduleForm(false)}
                />
              )}

              {schedules.length === 0 ? (
                <EmptyState
                  title="No scheduled exports"
                  description="Create a schedule to automatically export usage reports as CSV or PDF."
                />
              ) : (
                <div style={{ background: "var(--color-surface)", border: "1px solid var(--color-border)", borderRadius: "var(--radius-card)", overflow: "hidden" }}>
                  {schedules.map((sched, i) => (
                    <div key={sched.id}>
                      {editSchedule?.id === sched.id ? (
                        <div style={{ padding: "var(--space-4)" }}>
                          <ScheduleForm
                            initial={sched}
                            onSave={(data) => updateSchedule(sched.id, data)}
                            onCancel={() => setEditSchedule(null)}
                          />
                        </div>
                      ) : (
                        <div style={{
                          display: "flex",
                          alignItems: "center",
                          gap: "var(--space-3)",
                          padding: "var(--space-3) var(--space-4)",
                          borderTop: i === 0 ? "none" : "1px solid var(--color-border)",
                        }}>
                          <div style={{ flex: 1 }}>
                            <div style={{ fontFamily: "var(--font-mono)", fontSize: 12, fontWeight: 600 }}>{sched.cron}</div>
                            <div style={{ fontSize: 11, color: "var(--color-secondary)", marginTop: 3 }}>
                              {sched.format.toUpperCase()}
                              {sched.scope?.app ? ` · app: ${sched.scope.app}` : ""}
                              {sched.scope?.tenant ? ` · tenant: ${sched.scope.tenant}` : ""}
                            </div>
                          </div>
                          <Badge label={sched.format.toUpperCase()} variant="info" />
                          <div style={{ display: "flex", gap: 6 }}>
                            <button
                              onClick={() => { setEditSchedule(sched); setShowScheduleForm(false); }}
                              className="btn-secondary" style={{ background: "none", borderRadius: "var(--radius-control)", padding: "6px 10px", minHeight: 28, cursor: "pointer", fontSize: 11 }}
                            >
                              Edit
                            </button>
                            <button
                              onClick={() => void deleteSchedule(sched.id)}
                              className="btn-press" style={{ background: "none", border: "1px solid var(--color-error)", color: "var(--color-error)", borderRadius: "var(--radius-control)", padding: "6px 10px", minHeight: 28, cursor: "pointer", fontSize: 11 }}
                            >
                              Delete
                            </button>
                          </div>
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </>
      )}
    </div>
  );
}
