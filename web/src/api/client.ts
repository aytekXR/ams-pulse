/**
 * Typed API client for the Pulse REST API and live WebSocket.
 *
 * Contract: contracts/openapi/pulse-api.yaml — request/response types are
 * GENERATED from the spec (openapi-typescript) into src/lib/api/schema.d.ts.
 * Hand-written shapes that drift from the spec are a contract violation.
 *
 * Token storage: localStorage key "pulse_token".
 */

import type {
  GetLiveOverviewResponse,
  GetLiveStreamsResponse,
  GetAlertRulesResponse,
  GetAlertChannelsResponse,
  GetAlertHistoryResponse,
  GetAudienceResponse,
  GetAdminSourcesResponse,
  GetAdminTokensResponse,
  GetAdminLicenseResponse,
  AlertRuleWrite,
  AlertChannelWrite,
  SourceWrite,
  TokenWrite,
  TokenCreated,
  LicenseInfo,
  ChannelTestResult,
  AmsSourceStatus,
  WsMessage,
  LiveOverview,
  ErrorResponse,
  UserList,
  UserWrite,
  User,
  QoeSummaryResponse,
  IngestHealthResponse,
  FleetNodeList,
  UsageReportResponse,
  ReportScheduleList,
  ReportSchedule,
  ReportScheduleWrite,
  Tenant,
  TenantList,
  TenantWrite,
  AnomalyList,
  ProbeList,
  Probe,
  ProbeWrite,
  ProbeResultList,
  AuditLogPage,
} from "@/lib/api/types";
import type { components } from "@/lib/api/schema.d.ts";

// ─── Token management ─────────────────────────────────────────────────────────

const TOKEN_KEY = "pulse_token";

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY);
}

// ─── Core fetch wrapper ───────────────────────────────────────────────────────

export class ApiError extends Error {
  constructor(
    public status: number,
    public body: ErrorResponse,
  ) {
    super(body.message ?? `HTTP ${status}`);
    this.name = "ApiError";
  }
}

async function apiFetch<T>(
  path: string,
  init: RequestInit = {},
): Promise<T> {
  const token = getToken();
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(init.headers as Record<string, string>),
  };
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  const res = await fetch(`/api/v1${path}`, { ...init, headers });

  if (!res.ok) {
    let body: ErrorResponse;
    try {
      body = (await res.json()) as ErrorResponse;
    } catch {
      body = { message: res.statusText, code: String(res.status) };
    }
    // Wave-2 fix: dispatch a custom event on 401 so AuthGate can redirect
    // to the token entry screen without a full page reload.
    if (res.status === 401) {
      window.dispatchEvent(new Event("pulse:auth:401"));
    }
    throw new ApiError(res.status, body);
  }

  if (res.status === 204) {
    return undefined as T;
  }

  return res.json() as Promise<T>;
}

/**
 * Download a file from an authenticated endpoint.
 *
 * A plain `window.location.href = url` cannot carry an Authorization header, so
 * the only way to authenticate such a navigation is `?token=` in the query
 * string — which leaks the token into access logs, proxy caches and browser
 * history. `bearerAuthMiddleware` rejects it for exactly that reason
 * (TestTokenInURL_Ignored). Fetching the body ourselves and handing the browser
 * an object URL keeps the token in the header where it belongs, and works
 * against every authenticated route rather than only the ones opted into
 * `downloadAuthMiddleware`.
 */
async function downloadFile(path: string, fallbackName: string): Promise<void> {
  const token = getToken();
  const res = await fetch(`/api/v1${path}`, {
    headers: token ? { Authorization: `Bearer ${token}` } : {},
  });

  if (!res.ok) {
    let body: ErrorResponse;
    try {
      body = (await res.json()) as ErrorResponse;
    } catch {
      body = { message: res.statusText, code: String(res.status) };
    }
    if (res.status === 401) {
      window.dispatchEvent(new Event("pulse:auth:401"));
    }
    throw new ApiError(res.status, body);
  }

  const disposition = res.headers.get("Content-Disposition") ?? "";
  const match = /filename="?([^";]+)"?/.exec(disposition);
  const filename = match?.[1] ?? fallbackName;

  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

// ─── Live endpoints ───────────────────────────────────────────────────────────

export const liveApi = {
  getOverview: () => apiFetch<GetLiveOverviewResponse>("/live/overview"),
  getStreams: (params?: { limit?: number; cursor?: string }) => {
    const q = new URLSearchParams();
    if (params?.limit) q.set("limit", String(params.limit));
    if (params?.cursor) q.set("cursor", params.cursor);
    const qs = q.toString() ? `?${q.toString()}` : "";
    return apiFetch<GetLiveStreamsResponse>(`/live/streams${qs}`);
  },
};

// ─── Analytics endpoints ──────────────────────────────────────────────────────

export const analyticsApi = {
  getAudience: (params: {
    from: number;
    to: number;
    // VD-X3-B: spec/server use 'interval', not 'granularity'
    interval?: string;
    stream_id?: string;
    app?: string;
  }) => {
    const q = new URLSearchParams({
      from: String(params.from),
      to: String(params.to),
    });
    if (params.interval) q.set("interval", params.interval);
    if (params.stream_id) q.set("stream_id", params.stream_id);
    if (params.app) q.set("app", params.app);
    return apiFetch<GetAudienceResponse>(`/analytics/audience?${q}`);
  },

  getGeo: (params: { from: number; to: number; stream_id?: string; app?: string }) => {
    const q = new URLSearchParams({ from: String(params.from), to: String(params.to) });
    if (params.stream_id) q.set("stream_id", params.stream_id);
    if (params.app) q.set("app", params.app);
    return apiFetch<components["schemas"]["GeoResponse"]>(`/analytics/geo?${q}`);
  },

  getDevices: (params: { from: number; to: number; stream_id?: string; app?: string }) => {
    const q = new URLSearchParams({ from: String(params.from), to: String(params.to) });
    if (params.stream_id) q.set("stream_id", params.stream_id);
    if (params.app) q.set("app", params.app);
    return apiFetch<components["schemas"]["DeviceResponse"]>(`/analytics/devices?${q}`);
  },

  exportCsv: (params: { from: number; to: number; stream_id?: string; app?: string }) => {
    const q = new URLSearchParams({ from: String(params.from), to: String(params.to), format: "csv" });
    if (params.stream_id) q.set("stream_id", params.stream_id);
    if (params.app) q.set("app", params.app);
    return downloadFile(`/analytics/audience?${q}`, "audience.csv");
  },
};

// ─── Alerts endpoints ─────────────────────────────────────────────────────────

export const alertsApi = {
  getRules: (params?: { severity?: string; state?: string }) => {
    const q = new URLSearchParams();
    if (params?.severity) q.set("severity", params.severity);
    if (params?.state) q.set("state", params.state);
    const qs = q.toString() ? `?${q}` : "";
    return apiFetch<GetAlertRulesResponse>(`/alerts/rules${qs}`);
  },

  createRule: (body: AlertRuleWrite) =>
    apiFetch<components["schemas"]["AlertRule"]>("/alerts/rules", {
      method: "POST",
      body: JSON.stringify(body),
    }),

  updateRule: (id: string, body: AlertRuleWrite) =>
    apiFetch<components["schemas"]["AlertRule"]>(`/alerts/rules/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),

  deleteRule: (id: string) =>
    apiFetch<void>(`/alerts/rules/${id}`, { method: "DELETE" }),

  getChannels: () =>
    apiFetch<GetAlertChannelsResponse>("/alerts/channels"),

  createChannel: (body: AlertChannelWrite) =>
    apiFetch<components["schemas"]["AlertChannel"]>("/alerts/channels", {
      method: "POST",
      body: JSON.stringify(body),
    }),

  updateChannel: (id: string, body: AlertChannelWrite) =>
    apiFetch<components["schemas"]["AlertChannel"]>(`/alerts/channels/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),

  deleteChannel: (id: string) =>
    apiFetch<void>(`/alerts/channels/${id}`, { method: "DELETE" }),

  testChannel: (id: string) =>
    apiFetch<ChannelTestResult>(`/alerts/channels/${id}/test`, { method: "POST" }),

  getHistory: (params?: { rule_id?: string; state?: string; limit?: number }) => {
    const q = new URLSearchParams();
    if (params?.rule_id) q.set("rule_id", params.rule_id);
    if (params?.state) q.set("state", params.state);
    if (params?.limit) q.set("limit", String(params.limit));
    const qs = q.toString() ? `?${q}` : "";
    return apiFetch<GetAlertHistoryResponse>(`/alerts/history${qs}`);
  },
};

// ─── Admin endpoints ──────────────────────────────────────────────────────────

export const adminApi = {
  getSources: () =>
    apiFetch<GetAdminSourcesResponse>("/admin/sources"),

  createSource: (body: SourceWrite) =>
    apiFetch<components["schemas"]["Source"]>("/admin/sources", {
      method: "POST",
      body: JSON.stringify(body),
    }),

  updateSource: (id: string, body: SourceWrite) =>
    apiFetch<components["schemas"]["Source"]>(`/admin/sources/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),

  deleteSource: (id: string) =>
    apiFetch<void>(`/admin/sources/${id}`, { method: "DELETE" }),

  // CR-3: POST /admin/sources/{sourceId}/test — contract added; server implementation
  // deferred to wave 2. Handles 404/501 gracefully: returns a synthetic AmsSourceStatus
  // with reachable=false so the UI can display a friendly "not yet implemented" state.
  testSource: async (id: string): Promise<AmsSourceStatus> => {
    try {
      return await apiFetch<AmsSourceStatus>(`/admin/sources/${id}/test`, {
        method: "POST",
      });
    } catch (err) {
      if (err instanceof ApiError && (err.status === 404 || err.status === 501)) {
        return { reachable: false, error: "Source test not yet implemented (wave 2)" };
      }
      throw err;
    }
  },

  getTokens: () =>
    apiFetch<GetAdminTokensResponse>("/admin/tokens"),

  createToken: (body: TokenWrite) =>
    apiFetch<TokenCreated>("/admin/tokens", {
      method: "POST",
      body: JSON.stringify(body),
    }),

  deleteToken: (id: string) =>
    apiFetch<void>(`/admin/tokens/${id}`, { method: "DELETE" }),

  getLicense: () =>
    apiFetch<GetAdminLicenseResponse>("/admin/license"),

  setLicense: (key: string) =>
    apiFetch<LicenseInfo>("/admin/license", {
      method: "PUT",
      body: JSON.stringify({ key }),
    }),

  getUsers: () =>
    apiFetch<UserList>("/admin/users"),

  createUser: (body: UserWrite) =>
    apiFetch<User>("/admin/users", {
      method: "POST",
      body: JSON.stringify(body),
    }),

  updateUser: (id: string, body: UserWrite) =>
    apiFetch<User>(`/admin/users/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),

  deleteUser: (id: string) =>
    apiFetch<void>(`/admin/users/${id}`, { method: "DELETE" }),

  // ── Audit trail (D-102): read-only "who changed what, when" ──
  listAuditLog: (params?: { limit?: number; cursor?: string }) => {
    const q = new URLSearchParams();
    if (params?.limit) q.set("limit", String(params.limit));
    if (params?.cursor) q.set("cursor", params.cursor);
    const qs = q.toString() ? `?${q}` : "";
    return apiFetch<AuditLogPage>(`/admin/audit-log${qs}`);
  },

  // ── Tenant CRUD (F6 multi-tenant billing; Business tier only) ──
  listTenants: (params?: { limit?: number; cursor?: string }) => {
    const q = new URLSearchParams();
    if (params?.limit) q.set("limit", String(params.limit));
    if (params?.cursor) q.set("cursor", params.cursor);
    const qs = q.toString() ? `?${q}` : "";
    return apiFetch<TenantList>(`/admin/tenants${qs}`);
  },

  createTenant: (body: TenantWrite) =>
    apiFetch<Tenant>("/admin/tenants", {
      method: "POST",
      body: JSON.stringify(body),
    }),

  getTenant: (id: string) =>
    apiFetch<Tenant>(`/admin/tenants/${id}`),

  updateTenant: (id: string, body: TenantWrite) =>
    apiFetch<Tenant>(`/admin/tenants/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),

  deleteTenant: (id: string) =>
    apiFetch<void>(`/admin/tenants/${id}`, { method: "DELETE" }),
};

// ─── QoE endpoints ───────────────────────────────────────────────────────────

export const qoeApi = {
  getSummary: (params: {
    from: number;
    to: number;
    stream_id?: string;
    app?: string;
  }) => {
    const q = new URLSearchParams({ from: String(params.from), to: String(params.to) });
    if (params.stream_id) q.set("stream", params.stream_id);
    if (params.app) q.set("app", params.app);
    return apiFetch<QoeSummaryResponse>(`/qoe/summary?${q}`);
  },

  getIngestHealth: (params: { from: number; to: number; stream_id?: string; app?: string }) => {
    const q = new URLSearchParams({ from: String(params.from), to: String(params.to) });
    if (params.stream_id) q.set("stream", params.stream_id);
    if (params.app) q.set("app", params.app);
    return apiFetch<IngestHealthResponse>(`/qoe/ingest?${q}`);
  },
};

// ─── Fleet endpoints ──────────────────────────────────────────────────────────

export const fleetApi = {
  listNodes: (params?: { limit?: number; cursor?: string }) => {
    const q = new URLSearchParams();
    if (params?.limit) q.set("limit", String(params.limit));
    if (params?.cursor) q.set("cursor", params.cursor);
    const qs = q.toString() ? `?${q}` : "";
    return apiFetch<FleetNodeList>(`/fleet/nodes${qs}`);
  },
};

// ─── Reports endpoints ────────────────────────────────────────────────────────

export const reportsApi = {
  getUsage: (params: { from: number; to: number; app?: string; tenant?: string }) => {
    const q = new URLSearchParams({ from: String(params.from), to: String(params.to) });
    if (params.app) q.set("app", params.app);
    if (params.tenant) q.set("tenant", params.tenant);
    return apiFetch<UsageReportResponse>(`/reports/usage?${q}`);
  },

  listSchedules: () =>
    apiFetch<ReportScheduleList>("/reports/schedules"),

  createSchedule: (body: ReportScheduleWrite) =>
    apiFetch<ReportSchedule>("/reports/schedules", {
      method: "POST",
      body: JSON.stringify(body),
    }),

  updateSchedule: (id: string, body: ReportScheduleWrite) =>
    apiFetch<ReportSchedule>(`/reports/schedules/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),

  deleteSchedule: (id: string) =>
    apiFetch<void>(`/reports/schedules/${id}`, { method: "DELETE" }),

  downloadExport: (params: { from: number; to: number; format: "csv"; app?: string; tenant?: string }) => {
    const q = new URLSearchParams({
      from: String(params.from),
      to: String(params.to),
      format: params.format,
    });
    if (params.app) q.set("app", params.app);
    if (params.tenant) q.set("tenant", params.tenant);
    return downloadFile(`/reports/export?${q}`, "usage.csv");
  },
};

// ─── Anomalies endpoints (F9) ────────────────────────────────────────────────

export const anomaliesApi = {
  list: (params?: {
    from?: number;
    to?: number;
    app?: string;
    stream?: string;
    metric?: string;
    min_sigma?: number;
    limit?: number;
    cursor?: string;
  }) => {
    const q = new URLSearchParams();
    if (params?.from) q.set("from", String(params.from));
    if (params?.to) q.set("to", String(params.to));
    if (params?.app) q.set("app", params.app);
    if (params?.stream) q.set("stream", params.stream);
    if (params?.metric) q.set("metric", params.metric);
    if (params?.min_sigma != null) q.set("min_sigma", String(params.min_sigma));
    if (params?.limit) q.set("limit", String(params.limit));
    if (params?.cursor) q.set("cursor", params.cursor);
    const qs = q.toString() ? `?${q}` : "";
    return apiFetch<AnomalyList>(`/anomalies${qs}`);
  },
};

// ─── Probes endpoints (F10) ──────────────────────────────────────────────────

export const probesApi = {
  list: (params?: { limit?: number; cursor?: string }) => {
    const q = new URLSearchParams();
    if (params?.limit) q.set("limit", String(params.limit));
    if (params?.cursor) q.set("cursor", params.cursor);
    const qs = q.toString() ? `?${q}` : "";
    return apiFetch<ProbeList>(`/probes${qs}`);
  },

  create: (body: ProbeWrite) =>
    apiFetch<Probe>("/probes", {
      method: "POST",
      body: JSON.stringify(body),
    }),

  update: (id: string, body: ProbeWrite) =>
    apiFetch<Probe>(`/probes/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    }),

  delete: (id: string) =>
    apiFetch<void>(`/probes/${id}`, { method: "DELETE" }),

  getResults: (
    id: string,
    params?: { from?: number; to?: number; limit?: number; cursor?: string },
  ) => {
    const q = new URLSearchParams();
    if (params?.from) q.set("from", String(params.from));
    if (params?.to) q.set("to", String(params.to));
    if (params?.limit) q.set("limit", String(params.limit));
    if (params?.cursor) q.set("cursor", params.cursor);
    const qs = q.toString() ? `?${q}` : "";
    return apiFetch<ProbeResultList>(`/probes/${id}/results${qs}`);
  },
};

// ─── LiveSocket — auto-reconnecting WebSocket for /live/ws ────────────────────

type SocketListener = (msg: WsMessage<LiveOverview>) => void;

interface LiveSocketOptions {
  /** initial reconnect delay in ms; doubles each attempt up to maxDelay */
  baseDelay?: number;
  maxDelay?: number;
  /** Called when the socket transitions between connected/disconnected states */
  onStatusChange?: (connected: boolean) => void;
}

export class LiveSocket {
  private ws: WebSocket | null = null;
  private listeners: Set<SocketListener> = new Set();
  private retryDelay: number;
  private readonly baseDelay: number;
  private readonly maxDelay: number;
  private retryTimer: ReturnType<typeof setTimeout> | null = null;
  private destroyed = false;
  private _connected = false;
  private readonly onStatusChange?: (connected: boolean) => void;

  constructor(options: LiveSocketOptions = {}) {
    this.baseDelay = options.baseDelay ?? 1000;
    this.retryDelay = this.baseDelay;
    this.maxDelay = options.maxDelay ?? 30_000;
    this.onStatusChange = options.onStatusChange;
  }

  get connected(): boolean {
    return this._connected;
  }

  connect(): void {
    if (this.destroyed) return;
    const token = getToken();
    const url = `/live/ws${token ? `?token=${encodeURIComponent(token)}` : ""}`;

    // Use wss:// if page is served over HTTPS
    const wsUrl = (window.location.protocol === "https:" ? "wss://" : "ws://") +
      window.location.host + url;

    this.ws = new WebSocket(wsUrl);

    this.ws.onopen = () => {
      this.retryDelay = this.baseDelay; // reset backoff to configured base on success
      this._setConnected(true);
    };

    this.ws.onmessage = (ev: MessageEvent) => {
      try {
        const msg = JSON.parse(ev.data as string) as WsMessage<LiveOverview>;
        this.listeners.forEach((fn) => fn(msg));
      } catch {
        // malformed frame — ignore
      }
    };

    this.ws.onclose = () => {
      this._setConnected(false);
      if (!this.destroyed) {
        this.scheduleRetry();
      }
    };

    this.ws.onerror = () => {
      // onerror is always followed by onclose; let onclose handle retry
    };
  }

  private _setConnected(value: boolean): void {
    if (this._connected !== value) {
      this._connected = value;
      this.onStatusChange?.(value);
    }
  }

  private scheduleRetry(): void {
    if (this.retryTimer !== null) clearTimeout(this.retryTimer);
    this.retryTimer = setTimeout(() => {
      this.retryTimer = null;
      this.connect();
    }, this.retryDelay);
    this.retryDelay = Math.min(this.retryDelay * 2, this.maxDelay);
  }

  subscribe(fn: SocketListener): () => void {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }

  destroy(): void {
    this.destroyed = true;
    if (this.retryTimer !== null) {
      clearTimeout(this.retryTimer);
      this.retryTimer = null;
    }
    this.ws?.close();
    this.ws = null;
    this.listeners.clear();
  }
}
