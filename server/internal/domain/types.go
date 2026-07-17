// Package domain holds Pulse's core types, shared by collector, store, query,
// alerting and reports. Types here mirror the contracts in /contracts/events —
// when a contract changes, this package changes first and the compiler finds
// the rest.
//
// Rule: no AMS-specific field names in this package. AMS shapes are translated
// at the collector boundary (pkg/amsclient + internal/collector) into these
// normalized types. That boundary is the Phase 3 multi-server portability play.
package domain

import (
	"context"
	"time"
)

// ─── Event type constants ─────────────────────────────────────────────────────

const (
	EventStreamPublishStart = "stream_publish_start"
	EventStreamPublishEnd   = "stream_publish_end"
	EventStreamStats        = "stream_stats"
	EventWebRTCClientStats  = "webrtc_client_stats"
	EventIngestStats        = "ingest_stats"
	EventNodeStats          = "node_stats"
	EventRecordingReady     = "recording_ready"
	EventViewerJoin         = "viewer_join"
	EventViewerLeave        = "viewer_leave"
)

// Source identifies which AMS data-collection pathway produced an event.
const (
	SourceRestPoll  = "rest_poll"
	SourceKafka     = "kafka"
	SourceWebhook   = "webhook"
	SourceHostAgent = "host_agent"
)

// ─── Core event type ──────────────────────────────────────────────────────────

// ServerEvent is the normalized event from any server-side source.
// Contract: contracts/events/ams-server-event.schema.json
// Version 1 — bump on breaking change.
type ServerEvent struct {
	Version  int    `json:"version"`
	Type     string `json:"type"`
	TS       int64  `json:"ts"` // Unix epoch ms
	Source   string `json:"source"`
	NodeID   string `json:"node_id"`
	App      string `json:"app,omitempty"`
	StreamID string `json:"stream_id,omitempty"`

	// Type-specific payload — one of the *Data structs below, marshalled as map.
	Data       map[string]any   `json:"data,omitempty"`
	Enrichment *EnrichmentBlock `json:"enrichment,omitempty"`
}

// Time returns TS as a time.Time (UTC).
func (e ServerEvent) Time() time.Time {
	return time.UnixMilli(e.TS).UTC()
}

// ─── Data payload types (one per event type) ─────────────────────────────────

// StreamPublishStartData carries stream_publish_start payload.
type StreamPublishStartData struct {
	PublishType string `json:"publish_type"` // webrtc|rtmp|hls|mp4|other — required
}

// StreamPublishEndData carries stream_publish_end payload.
type StreamPublishEndData struct {
	DurationS int    `json:"duration_s,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// ProtocolViewerCounts is the per-protocol viewer breakdown.
type ProtocolViewerCounts struct {
	WebRTC int `json:"webrtc,omitempty"`
	HLS    int `json:"hls,omitempty"`
	RTMP   int `json:"rtmp,omitempty"`
	DASH   int `json:"dash,omitempty"`
	Other  int `json:"other,omitempty"`
}

// StreamStatsData carries stream_stats payload.
type StreamStatsData struct {
	ViewerCount           int                   `json:"viewer_count"` // required
	ViewerCountByProtocol *ProtocolViewerCounts `json:"viewer_count_by_protocol,omitempty"`
	BitrateKbps           float64               `json:"bitrate_kbps,omitempty"`
	SpeedReadKbps         float64               `json:"speed_read_kbps,omitempty"`
}

// WebRTCClientStatsData carries webrtc_client_stats payload.
type WebRTCClientStatsData struct {
	ClientID      string  `json:"client_id"` // required
	RTTMS         float64 `json:"rtt_ms,omitempty"`
	JitterMS      float64 `json:"jitter_ms,omitempty"`
	PacketLossPct float64 `json:"packet_loss_pct,omitempty"`
}

// IngestStatsData carries ingest_stats payload.
type IngestStatsData struct {
	BitrateKbps       float64 `json:"bitrate_kbps,omitempty"`
	FPS               float64 `json:"fps,omitempty"`
	KeyframeIntervalS float64 `json:"keyframe_interval_s,omitempty"`
	PacketLossPct     float64 `json:"packet_loss_pct,omitempty"`
	JitterMS          float64 `json:"jitter_ms,omitempty"`
}

// NodeStatsData carries node_stats payload.
type NodeStatsData struct {
	CPUPCT        float64 `json:"cpu_pct,omitempty"`
	MemPCT        float64 `json:"mem_pct,omitempty"`
	DiskPCT       float64 `json:"disk_pct,omitempty"`
	NetInMbps     float64 `json:"net_in_mbps,omitempty"`
	NetOutMbps    float64 `json:"net_out_mbps,omitempty"`
	JVMHeapUsedMB float64 `json:"jvm_heap_used_mb,omitempty"`
}

// RecordingReadyData carries recording_ready payload.
type RecordingReadyData struct {
	Path      string `json:"path"` // required
	SizeBytes int64  `json:"size_bytes,omitempty"`
	DurationS int    `json:"duration_s,omitempty"`
}

// ViewerJoinData carries viewer_join payload.
type ViewerJoinData struct {
	ViewerID  string `json:"viewer_id"` // required — opaque, collector-generated
	Protocol  string `json:"protocol"`  // required
	IPHash    string `json:"ip_hash,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
	Referrer  string `json:"referrer,omitempty"`
}

// ViewerLeaveData carries viewer_leave payload.
type ViewerLeaveData struct {
	ViewerID   string `json:"viewer_id"` // required
	Protocol   string `json:"protocol"`  // required
	WatchTimeS int    `json:"watch_time_s,omitempty"`
}

// ─── Enrichment block ─────────────────────────────────────────────────────────

// EnrichmentBlock holds collector-added metadata (geo-IP, UA parsing).
// AMS never sends this; it is appended during normalization.
type EnrichmentBlock struct {
	Geo    *GeoEnrichment    `json:"geo,omitempty"`
	Client *ClientEnrichment `json:"client,omitempty"`
}

// GeoEnrichment holds geo-IP lookup results.
type GeoEnrichment struct {
	Country string `json:"country,omitempty"` // ISO 3166-1 alpha-2
	Region  string `json:"region,omitempty"`
}

// ClientEnrichment holds UA-parsed client info.
type ClientEnrichment struct {
	Device  string `json:"device,omitempty"` // desktop|mobile|tablet|tv|other
	OS      string `json:"os,omitempty"`
	Browser string `json:"browser,omitempty"`
}

// ─── Beacon event type ───────────────────────────────────────────────────────

// BeaconEvent is a viewer-side QoE event batch from the beacon SDK.
// Contract: contracts/events/beacon-event.schema.json
type BeaconEvent struct {
	Version   int          `json:"version"`
	SessionID string       `json:"session_id"`
	StreamID  string       `json:"stream_id"`
	App       string       `json:"app,omitempty"`
	SDK       string       `json:"sdk,omitempty"`
	Events    []BeaconItem `json:"events"`

	// Envelope metadata
	PlayerKind string `json:"player_kind,omitempty"`
	Tenant     string `json:"tenant,omitempty"`

	// Collector-added enrichment
	Enrichment *EnrichmentBlock `json:"enrichment,omitempty"`
}

// BeaconItem is a single event within a beacon batch.
type BeaconItem struct {
	Type string         `json:"type"`
	TS   int64          `json:"ts"` // Unix epoch ms
	Data map[string]any `json:"data,omitempty"`
}

// ─── Viewer session type ─────────────────────────────────────────────────────

// ViewerSession is a stitched per-viewer playback session.
// Written to ClickHouse viewer_sessions table via ReplacingMergeTree.
type ViewerSession struct {
	SessionID     string    `json:"session_id"`
	StreamID      string    `json:"stream_id"`
	App           string    `json:"app,omitempty"`
	NodeID        string    `json:"node_id,omitempty"`
	StartedAt     time.Time `json:"started_at"`
	EndedAt       time.Time `json:"ended_at,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
	StartupMS     uint32    `json:"startup_ms,omitempty"`
	WatchTimeS    uint32    `json:"watch_time_s,omitempty"`
	RebufferCount uint16    `json:"rebuffer_count,omitempty"`
	RebufferMS    uint32    `json:"rebuffer_ms,omitempty"`
	ErrorCount    uint16    `json:"error_count,omitempty"`
	PeakBitrate   float32   `json:"peak_bitrate,omitempty"`
	Protocol      string    `json:"protocol,omitempty"`
	GeoCountry    string    `json:"geo_country,omitempty"`
	GeoRegion     string    `json:"geo_region,omitempty"`
	ClientDevice  string    `json:"client_device,omitempty"`
	ClientOS      string    `json:"client_os,omitempty"`
	ClientBrowser string    `json:"client_browser,omitempty"`
	Tenant        string    `json:"tenant,omitempty"`
}

// ─── Alert types (BE-02 fills logic; BE-01 owns shapes per meta DDL) ─────────

// AlertRule is a user-defined alerting rule.
// Mirrors the alert_rules table in contracts/db/meta/0001_init.sql.
type AlertRule struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Metric     string     `json:"metric"`
	Condition  string     `json:"condition"` // gt|lt|eq|gte|lte
	Threshold  float64    `json:"threshold"`
	WindowS    int        `json:"window_s"`
	Scope      AlertScope `json:"scope"`
	Severity   string     `json:"severity"` // info|warning|critical
	ChannelIDs []string   `json:"channel_ids"`
	Enabled    bool       `json:"enabled"`
	CooldownS  int        `json:"cooldown_s,omitempty"`
	CreatedAt  int64      `json:"created_at"` // Unix epoch ms
	UpdatedAt  int64      `json:"updated_at"` // Unix epoch ms
}

// AlertScope limits an alert rule to a specific node/app/stream.
type AlertScope struct {
	NodeID   string `json:"node_id,omitempty"`
	App      string `json:"app,omitempty"`
	StreamID string `json:"stream_id,omitempty"`
	// Tenant scopes a rule to one tenant (F6 Phase 2). Empty = all tenants
	// (backward-compatible: existing rules have no tenant in ScopeJSON). Stored
	// inside AlertRuleRow.ScopeJSON, so no DB migration is required.
	Tenant string `json:"tenant,omitempty"`
}

// Notification is an alert delivery payload.
// Contract: contracts/events/alert-notification.schema.json
type Notification struct {
	AlertID       string     `json:"alert_id"`
	RuleID        string     `json:"rule_id"`
	RuleName      string     `json:"rule_name"`
	State         string     `json:"state"` // firing|resolved|test
	Severity      string     `json:"severity"`
	Metric        string     `json:"metric"`
	Value         float64    `json:"value"`
	Threshold     float64    `json:"threshold"`
	Condition     string     `json:"condition"`
	Scope         AlertScope `json:"scope"`
	FiredAt       int64      `json:"fired_at"` // Unix epoch ms
	ResolvedAt    *int64     `json:"resolved_at,omitempty"`
	CooldownUntil *int64     `json:"cooldown_until,omitempty"`
	GroupKey      string     `json:"group_key,omitempty"`
	Test          bool       `json:"test,omitempty"`
}

// ─── Live snapshot types (BE-02 consumes via LiveProvider) ───────────────────

// StreamHealth represents the health state of an active stream.
type StreamHealth string

const (
	StreamHealthGood     StreamHealth = "good"
	StreamHealthWarning  StreamHealth = "warning"
	StreamHealthCritical StreamHealth = "critical"
	StreamHealthOffline  StreamHealth = "offline"
)

// LiveStream holds real-time state for one active stream.
type LiveStream struct {
	StreamID       string               `json:"stream_id"`
	App            string               `json:"app"`
	NodeID         string               `json:"node_id"`
	PublishType    string               `json:"publish_type"`
	Active         bool                 `json:"active"`
	StartedAt      time.Time            `json:"started_at"`
	LastSeenAt     time.Time            `json:"last_seen_at"`
	ViewerCount    int                  `json:"viewer_count"`
	ViewersByProto ProtocolViewerCounts `json:"viewers_by_proto"`
	IngestBitrate  float64              `json:"ingest_bitrate_kbps"`
	FPS            float64              `json:"fps"`
	Health         StreamHealth         `json:"health"`

	// Ingest health metrics (populated by ingest.HealthTracker, Wave 2).
	// HealthScore is the weighted composite score (0.0–1.0); see ingest.ComputeHealthScore.
	HealthScore       float64 `json:"health_score,omitempty"`
	PacketLossPct     float64 `json:"packet_loss_pct,omitempty"`
	JitterMS          float64 `json:"jitter_ms,omitempty"`
	KeyframeIntervalS float64 `json:"keyframe_interval_s,omitempty"`

	// Viewer-side WebRTC QoE metrics (populated from webrtc_client_stats events).
	// Last-write-wins when multiple peer-stat events arrive for the same stream;
	// the aggregator records the most recent poll value per stream, which is
	// sufficient for the live dashboard and alert evaluator.
	ViewerRTTMS    float64 `json:"viewer_rtt_ms,omitempty"`
	ViewerJitterMS float64 `json:"viewer_jitter_ms,omitempty"`
	ViewerLossPct  float64 `json:"viewer_loss_pct,omitempty"`
}

// LiveNodeStats holds real-time state for one cluster node.
type LiveNodeStats struct {
	NodeID string `json:"node_id"`
	// Version is the AMS server version string (e.g. "3.0.3").
	// Populated from ClusterNodeDTO.Version (cluster path) or GetVersion (standalone path).
	Version   string    `json:"version,omitempty"`
	CPUPCT    float64   `json:"cpu_pct"`
	MemPCT    float64   `json:"mem_pct"`
	DiskPCT   float64   `json:"disk_pct"`
	NetIn     float64   `json:"net_in_mbps"`
	NetOut    float64   `json:"net_out_mbps"`
	UpdatedAt time.Time `json:"updated_at"`
	// LastSeenAt is the wall-clock time when the last NodeStats event was received.
	// Used by the aggregator to evict stale nodes (VD-30).
	LastSeenAt time.Time `json:"last_seen_at,omitempty"`

	// Identity fields from real AMS 3.x /rest/v2/system-status (standalone path).
	// Absent for cluster nodes (ClusterNodeDTO does not carry OS identity).
	OsName         string `json:"os_name,omitempty"`
	OsArch         string `json:"os_arch,omitempty"`
	JavaVersion    string `json:"java_version,omitempty"`
	ProcessorCount int    `json:"processor_count,omitempty"`

	// D-087: AMS early-warning ladder (rung 1 & 2 feed).
	// APILatencyMS is the round-trip time of the last successful SystemStats or
	// ClusterNodes call in milliseconds. Absent (zero) when the last call failed.
	APILatencyMS float64 `json:"api_latency_ms,omitempty"`
	// ConsecAPIErrors is the number of consecutive API-call failures since the
	// last success. Reset to 0 on any successful stats call. Drives the
	// node_degraded rule (rung 2: ConsecAPIErrors >= 3 → degraded).
	ConsecAPIErrors int `json:"consec_api_errors,omitempty"`

	// D-088: presence flags — true when the respective key was present in the
	// normalized collector event. Cluster AMS 3.x emits all three; standalone
	// AMS 3.x omits cpu_pct/mem_pct/disk_pct (normalize.go:241). The anomaly
	// detector guards on these flags to skip Welford updates and liveValues
	// insertions for metrics that were never measured, preventing zero-mean
	// baseline poisoning. Tagged json:"-" so the flags never appear in API
	// responses or snapshot JSON payloads.
	CPUPCTReported  bool `json:"-"`
	MemPCTReported  bool `json:"-"`
	DiskPCTReported bool `json:"-"`
}

// Degraded reports whether this node meets the node_degraded condition.
// Single source of truth for node_degraded — alert (wave2 evalNodeUpDown) and
// display (query FleetNodes/LiveOverview) must agree — D-088.
func (n *LiveNodeStats) Degraded() bool {
	return n.CPUPCT > 90 || n.MemPCT > 90 || n.ConsecAPIErrors >= 3
}

// LiveSnapshot is the in-memory aggregate state served to the dashboard.
// It covers totals, per-app, per-stream and per-node views.
type LiveSnapshot struct {
	// Totals
	ActiveStreams int     `json:"active_streams"`
	TotalViewers  int     `json:"total_viewers"`
	IngestBitrate float64 `json:"ingest_bitrate_kbps"`

	// Per-stream detail (map key = stream_id)
	Streams map[string]*LiveStream `json:"streams"`

	// Per-app rollup (map key = app name)
	AppViewers map[string]int `json:"app_viewers"`

	// Per-node (map key = node_id)
	Nodes map[string]*LiveNodeStats `json:"nodes"`

	// Snapshot timestamp
	UpdatedAt time.Time `json:"updated_at"`
}

// ─── Interfaces consumed by BE-02 ────────────────────────────────────────────

// LiveProvider exposes the in-memory live aggregate state to the API/alert layers.
// Implemented by internal/collector/aggregator.Aggregator.
//
// BE-02 reads CurrentSnapshot() for REST /live/summary and subscribes via
// Subscribe() to drive WebSocket push and alert evaluation.
type LiveProvider interface {
	// CurrentSnapshot returns a deep copy of the current live state.
	CurrentSnapshot() *LiveSnapshot

	// Subscribe registers a channel that receives a copy of the snapshot
	// after every update. The caller owns the channel; unsubscribe via the
	// returned cancel function. Buffer the channel appropriately — a slow
	// consumer is dropped, not blocked.
	Subscribe() (<-chan *LiveSnapshot, func())
}

// EventSink accepts normalized events from collectors for fanout to
// ClickHouse writer and live aggregator.
// Implemented by internal/collector/fanout.Fanout.
type EventSink interface {
	// WriteServerEvent accepts a normalized ServerEvent for async fanout.
	WriteServerEvent(event ServerEvent)

	// WriteBeaconEvent accepts a normalized beacon batch for async fanout.
	WriteBeaconEvent(event BeaconEvent)

	// WriteViewerSession upserts a viewer session record.
	WriteViewerSession(session ViewerSession)
}

// ─── F10 Synthetic probe types (WO-301) ──────────────────────────────────────

// ProbeConfig holds the configuration for a single synthetic probe.
// Source of truth lives in the meta store (probes table); read via ProbeConfigSource.
type ProbeConfig struct {
	ID        string // UUID primary key
	Name      string // human-readable label
	URL       string // stream URL to probe
	Protocol  string // hls | webrtc | rtmp | dash
	IntervalS int    // probe interval in seconds (default 60)
	TimeoutS  int    // per-check timeout in seconds (default 10)
	Enabled   bool   // only enabled probes are listed by ListEnabled
}

// ProbeResult holds the outcome of a single probe execution.
// Written to ClickHouse probe_results by the runner; also passed to
// ProbeConfigSource.RecordResult to update probes.last_* denorm fields.
type ProbeResult struct {
	ID            string    // UUID for this result row
	ProbeID       string    // foreign key → ProbeConfig.ID
	TS            time.Time // when the probe ran (UTC)
	Success       bool      // true only on 2xx + parseable response
	TTFBMs        uint32    // time-to-first-byte in milliseconds
	ErrorCode     string    // "timeout" | "dns" | "http_4xx" | "http_5xx" | "parse" | "not_probed" | "ws_timeout" | "ws_refused" | "ws_error" | "rtmp_timeout" | "rtmp_refused" | "rtmp_error" | "segment_too_large" | "ice_failed" | "ice_timeout" | ""
	ErrorMsg      string    // human-readable detail; empty on success
	BitrateKbps   float32   // estimated kbps = segment_bytes / segment_duration_s (HLS/DASH); 0 on failure
	SegmentTTFBMs uint32    // TTFB of the first media segment in ms (HLS/DASH); 0 if not measured

	// Connection-establishment measurement fields (nil/empty for HLS/DASH probes).
	// ConnectTimeMs — WebRTC: WS dial → first server message (offer received) in ms;
	//                 RTMP: TCP dial → S2 handshake byte fully read in ms; nil if not applicable.
	// SignalingState — WebRTC: "offer_received" on success, "ws_*" on failure;
	//                  RTMP: "handshake_complete" on success, "rtmp_*" on failure; "" for HLS/DASH.
	// IceState — WebRTC phase-2a: terminal ICE state as seen by the probe ANSWERER.
	//             "connected" (ICEConnectionStateConnected or Completed),
	//             "failed"    (ICEConnectionStateFailed; ErrorCode="ice_failed"),
	//             "timeout"   (ctx deadline before ICE terminal state; ErrorCode="ice_timeout").
	//             Empty string = not applicable (HLS/DASH/RTMP) or ICE not attempted.
	//             INT-01-signed via the D-074 pre-approved CR (CH migration 0007).
	ConnectTimeMs  *uint32 // nil = not applicable (HLS/DASH, or connection failed before measurement point)
	SignalingState string  // "offer_received" | "handshake_complete" | "ws_*" | "rtmp_*" | ""
	IceState       string  // "connected" | "failed" | "timeout" | "" (empty = not applicable / ICE not attempted)

	// RttMs — WebRTC phase-2b: RTT of the selected ICE candidate pair in
	//          milliseconds (CurrentRoundTripTime seconds × 1000); nil = not measured;
	//          set only after ice_state="connected"; CH migration 0008; D-075.
	RttMs *float32
	// JitterMs — WebRTC phase-2b: inbound-RTP inter-arrival jitter per RFC 3550
	//             in milliseconds (Jitter seconds × 1000); nil = not measured;
	//             set only after ice_state="connected"; CH migration 0008; D-075.
	JitterMs *float32
	// LossPct — WebRTC phase-2b: inbound-RTP packet loss percent, 0-100,
	//            clamped >= 0; nil = not measured (no packets received or lost,
	//            or ice_state != "connected"); CH migration 0008; D-075.
	LossPct *float32
}

// ProbeConfigSource is the seam between the probe runner (BE-01) and the meta
// store implementation (BE-02, WO-302). The runner calls ListEnabled each
// interval to discover which probes to run, and RecordResult after each check
// to update the denormalized last_* columns in the probes table.
//
// This interface follows the EventSink pattern established in Wave 1.
//
// Authoritative signatures — BE-02 implements these against the meta store.
type ProbeConfigSource interface {
	// ListEnabled returns all probes where enabled = 1.
	// Called by the runner at the start of each scheduler tick.
	ListEnabled(ctx context.Context) ([]ProbeConfig, error)

	// RecordResult updates the probes.last_result_id, last_success, and
	// last_run_at denormalized fields after a probe check completes.
	// The full time-series result is written to ClickHouse by the runner itself.
	RecordResult(ctx context.Context, r ProbeResult) error
}
