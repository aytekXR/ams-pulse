/**
 * Re-exports for commonly used generated schema types.
 * All shapes come from the generated schema.d.ts — never hand-roll API shapes.
 */
import type { components, operations } from "./schema.d.ts";

// ─── Schema component aliases ────────────────────────────────────────────────
export type Schemas = components["schemas"];

export type LiveOverview = Schemas["LiveOverview"];
export type LiveStream = Schemas["LiveStream"];
export type LiveStreamList = Schemas["LiveStreamList"];
export type ProtocolMix = Schemas["ProtocolMix"];
export type NodeHealth = Schemas["NodeHealth"];
export type AppOverview = Schemas["AppOverview"];

export type AlertRule = Schemas["AlertRule"];
export type AlertRuleWrite = Schemas["AlertRuleWrite"];
export type AlertRuleList = Schemas["AlertRuleList"];
export type AlertChannel = Schemas["AlertChannel"];
export type AlertChannelWrite = Schemas["AlertChannelWrite"];
export type AlertChannelList = Schemas["AlertChannelList"];
export type AlertChannelConfig = Schemas["AlertChannelConfig"];
export type AlertHistoryEntry = Schemas["AlertHistoryEntry"];
export type AlertHistoryList = Schemas["AlertHistoryList"];
export type AlertSeverity = AlertRule["severity"];
export type AlertState = AlertHistoryEntry["state"];
export type AlertScope = Schemas["AlertScope"];
export type MaintenanceWindow = Schemas["MaintenanceWindow"];
export type ChannelTestResult = Schemas["ChannelTestResult"];

export type Source = Schemas["Source"];
export type SourceWrite = Schemas["SourceWrite"];
export type SourceList = Schemas["SourceList"];
export type AmsSourceStatus = Schemas["AmsSourceStatus"];

export type Token = Schemas["Token"];
export type TokenWrite = Schemas["TokenWrite"];
export type TokenCreated = Schemas["TokenCreated"];
export type TokenList = Schemas["TokenList"];

export type LicenseInfo = Schemas["LicenseInfo"];
export type TierLimits = Schemas["TierLimits"];

export type AudienceResponse = Schemas["AudienceResponse"];
export type AudienceTotals = Schemas["AudienceTotals"];
export type AudienceBucket = Schemas["AudienceBucket"];
export type GeoResponse = Schemas["GeoResponse"];
export type GeoRow = Schemas["GeoRow"];
export type DeviceResponse = Schemas["DeviceResponse"];
export type DeviceRow = Schemas["DeviceRow"];

export type ErrorResponse = Schemas["Error"];
export type PaginatedMeta = Schemas["PaginatedMeta"];

// ─── Audit trail (D-102) ──────────────────────────────────────────────────────
export type AuditEntry = Schemas["AuditEntry"];
export type AuditLogPage = Schemas["AuditLogPage"];

export type HealthStatus = Schemas["HealthStatus"];

export type QoeSummaryResponse = Schemas["QoeSummaryResponse"];
export type QoeTotals = Schemas["QoeTotals"];
export type BitrateBucket = Schemas["BitrateBucket"];

export type IngestHealthResponse = Schemas["IngestHealthResponse"];
export type IngestStream = Schemas["IngestStream"];
export type IngestBucket = Schemas["IngestBucket"];
export type DropEvent = Schemas["DropEvent"];

export type FleetNode = Schemas["FleetNode"];
export type FleetNodeList = Schemas["FleetNodeList"];

export type UsageReportResponse = Schemas["UsageReportResponse"];
export type UsageRow = Schemas["UsageRow"];
export type UsageTotals = Schemas["UsageTotals"];
export type ReportSchedule = Schemas["ReportSchedule"];
export type ReportScheduleList = Schemas["ReportScheduleList"];
export type ReportScheduleWrite = Schemas["ReportScheduleWrite"];

// ─── F6 — Tenant management ───────────────────────────────────────────────────
export type Tenant = Schemas["Tenant"];
export type TenantWrite = Schemas["TenantWrite"];
export type TenantList = Schemas["TenantList"];

export type User = Schemas["User"];
export type UserWrite = Schemas["UserWrite"];
export type UserList = Schemas["UserList"];

// ─── F9 — Anomaly detection ───────────────────────────────────────────────────
export type AnomalyFlag = Schemas["AnomalyFlag"];
export type AnomalyList = Schemas["AnomalyList"];

// ─── F10 — Synthetic probes ───────────────────────────────────────────────────
export type Probe = Schemas["Probe"];
export type ProbeWrite = Schemas["ProbeWrite"];
export type ProbeList = Schemas["ProbeList"];
export type ProbeResult = Schemas["ProbeResult"];
export type ProbeResultList = Schemas["ProbeResultList"];

// ─── WS envelope types (per spec /live/ws description) ───────────────────────
export interface WsMessage<T = unknown> {
  type: "snapshot" | "delta" | "heartbeat";
  ts: number;
  payload?: T;
}

export type WsSnapshotMessage = WsMessage<LiveOverview>;
export type WsDeltaMessage = WsMessage<Partial<LiveOverview>>;
export type WsHeartbeatMessage = WsMessage<never>;

// ─── Operation response helpers (using actual generated operationIds) ─────────
export type GetLiveOverviewResponse =
  operations["getLiveOverview"]["responses"]["200"]["content"]["application/json"];
export type GetLiveStreamsResponse =
  operations["getLiveStreams"]["responses"]["200"]["content"]["application/json"];
export type ListAlertRulesResponse =
  operations["listAlertRules"]["responses"]["200"]["content"]["application/json"];
export type ListAlertChannelsResponse =
  operations["listAlertChannels"]["responses"]["200"]["content"]["application/json"];
export type GetAlertHistoryResponse =
  operations["getAlertHistory"]["responses"]["200"]["content"]["application/json"];
export type GetAudienceAnalyticsResponse =
  operations["getAudienceAnalytics"]["responses"]["200"]["content"]["application/json"];
export type ListSourcesResponse =
  operations["listSources"]["responses"]["200"]["content"]["application/json"];
export type ListTokensResponse =
  operations["listTokens"]["responses"]["200"]["content"]["application/json"];
export type GetLicenseResponse =
  operations["getLicense"]["responses"]["200"]["content"]["application/json"];

// Aliases used by client.ts (kept for backward compat)
export type GetAlertRulesResponse = ListAlertRulesResponse;
export type GetAlertChannelsResponse = ListAlertChannelsResponse;
export type GetAdminSourcesResponse = ListSourcesResponse;
export type GetAdminTokensResponse = ListTokensResponse;
export type GetAdminLicenseResponse = GetLicenseResponse;
export type GetAudienceResponse = GetAudienceAnalyticsResponse;

// DateRange utility type used by AnalyticsPage / DateRangePicker
export interface DateRange {
  from: number;
  to: number;
}

