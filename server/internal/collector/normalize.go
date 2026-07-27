// Package collector — normalization from AMS DTOs to domain types.
//
// This file is the ONLY place where AMS wire shapes (from amsclient) are
// interpreted and mapped to domain.ServerEvent. Architecture rule 2.
//
// AMS isolation constraint (VD-16 / ARCHITECTURE §3):
// The AMS REST broadcast-statistics API is a server-side aggregate endpoint.
// It returns viewer counts and bitrate totals but has NO per-viewer information
// (IP addresses, User-Agent strings). Therefore buildEnrichment is always called
// with empty IP and UA on the REST path, and Enrichment will be nil for all
// REST-polled events. This is an architectural constraint, not a bug — per-viewer
// geo/UA enrichment is ONLY possible via the beacon path (POST /ingest/beacon)
// where the HTTP request carries the actual viewer's IP and User-Agent header.
package collector

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"time"

	"github.com/aytekXR/ams-pulse/server/internal/domain"
	"github.com/aytekXR/ams-pulse/server/pkg/amsclient"
)

// NormalizeBroadcast converts an AMS BroadcastDTO into one or more
// domain.ServerEvents.  It emits:
//   - stream_publish_start  (status == broadcasting, not seen before)
//   - stream_publish_end    (status == finished, was seen before)
//   - stream_stats          (status == broadcasting, always)
func NormalizeBroadcast(
	b amsclient.BroadcastDTO,
	nodeID string,
	prevStatus string,
	geo GeoResolver,
	ua UAParser,
) []domain.ServerEvent {
	// FIX 3 (VD-??): empty StreamID would key the aggregator map to "" and corrupt
	// live state — reject early so we never emit events with a blank stream ID.
	if b.StreamID == "" {
		return nil
	}

	now := time.Now().UnixMilli()
	var events []domain.ServerEvent

	app := b.AppName
	if app == "" {
		app = "live"
	}

	enrich := buildEnrichment("", "", geo, ua)

	switch b.Status {
	case "broadcasting":
		// Emit publish_start if this is a new broadcast.
		if prevStatus != "broadcasting" {
			pt := normalizePublishType(b.PublishType, b.Type)
			events = append(events, domain.ServerEvent{
				Version:  1,
				Type:     domain.EventStreamPublishStart,
				TS:       ifZero(b.StartTime, now),
				Source:   domain.SourceRestPoll,
				NodeID:   nodeID,
				App:      app,
				StreamID: b.StreamID,
				Data: map[string]any{
					"publish_type": pt,
				},
				Enrichment: enrich,
			})
		}

		// D-029v: AMS reports the broadcast "bitrate" in BITS/sec (curl-verified on
		// AMS 3.0.3: bitrate=624016 ≈ receivedBytes*8/duration ≈ 624 kbps). Convert
		// to kbps here — the single normalization boundary. The previous code stored
		// it raw into bitrate_kbps (a 1000× inflation) AND fell back to b.Speed when
		// bitrate==0; but AMS "speed" is a realtime RATIO (≈1.0), not a bitrate, so
		// that fallback emitted ~1 "kbps" of garbage. Both are removed.
		bitrateKbps := b.BitRate / 1000.0

		// Always emit stream_stats.
		statsData := map[string]any{
			"viewer_count": b.HlsViewerCount + b.WebRTCViewerCount + b.RTMPViewerCount + b.DashViewerCount,
			"viewer_count_by_protocol": map[string]any{
				"webrtc": b.WebRTCViewerCount,
				"hls":    b.HlsViewerCount,
				"rtmp":   b.RTMPViewerCount,
				"dash":   b.DashViewerCount,
				"other":  0,
			},
			"bitrate_kbps":    bitrateKbps,
			"speed_read_kbps": b.Speed, // NOTE: AMS "speed" is a ratio, not kbps (key name is legacy)
		}
		events = append(events, domain.ServerEvent{
			Version:    1,
			Type:       domain.EventStreamStats,
			TS:         now,
			Source:     domain.SourceRestPoll,
			NodeID:     nodeID,
			App:        app,
			StreamID:   b.StreamID,
			Data:       statsData,
			Enrichment: enrich,
		})

		// VD-22 / D-029v: emit ingest_stats from BroadcastDTO fields so REST-only
		// deployments surface ingest health. The real AMS 3.0.3 broadcast object DOES
		// carry ingest-side packetLostRatio/jitterMs/rttMs (curl-verified) — wire them
		// through instead of hardcoding 0 (which made the health score blind to ingest
		// packet loss/jitter). It does NOT carry currentFPS, so "fps" is attached only
		// when AMS actually reported it (>0); when absent, ComputeHealthScore
		// redistributes the FPS weight rather than scoring a phantom 0 fps (which had
		// pinned every REST-polled stream to "Warning").
		if b.CurrentFPS > 0 || bitrateKbps > 0 {
			ingestData := map[string]any{
				"bitrate_kbps":        bitrateKbps,
				"packet_loss_pct":     b.PacketLostRatio * 100.0, // AMS ratio 0..1 → percent
				"jitter_ms":           b.JitterMs,                // AMS jitterMs already in ms
				"rtt_ms":              b.RttMs,                   // AMS rttMs already in ms
				"keyframe_interval_s": 0.0,
			}
			if b.CurrentFPS > 0 {
				ingestData["fps"] = float64(b.CurrentFPS)
			}
			events = append(events, domain.ServerEvent{
				Version:    1,
				Type:       domain.EventIngestStats,
				TS:         now,
				Source:     domain.SourceRestPoll,
				NodeID:     nodeID,
				App:        app,
				StreamID:   b.StreamID,
				Data:       ingestData,
				Enrichment: enrich,
			})
		}

	case "finished", "ended", "terminated_unexpectedly":
		// D-029v: "terminated_unexpectedly" is a real AMS 3.0.3 status (curl-verified
		// in the meet app) for crashed/dropped ingests. Without it, a crashed stream
		// stayed "live" on the dashboard until stale eviction (~3 min) instead of
		// emitting publish_end on the next poll cycle (≤5 s).
		if prevStatus == "broadcasting" {
			events = append(events, domain.ServerEvent{
				Version:  1,
				Type:     domain.EventStreamPublishEnd,
				TS:       ifZero(b.EndTime, now),
				Source:   domain.SourceRestPoll,
				NodeID:   nodeID,
				App:      app,
				StreamID: b.StreamID,
				Data: map[string]any{
					"reason": b.Status,
				},
				Enrichment: enrich,
			})
		}
	}
	return events
}

// NormalizeWebRTCStats converts AMS WebRTC peer stats to domain.ServerEvent.
func NormalizeWebRTCStats(
	s amsclient.WebRTCClientStatsDTO,
	app, streamID, nodeID string,
) domain.ServerEvent {
	// D-029v: average ONLY the tracks AMS actually reported. A single-track viewer
	// (audio-only or video-only) leaves the other track's value at 0; a constant /2
	// then halved the real metric (e.g. 40 ms RTT reported as 20 ms), under-reporting
	// QoE degradation. avgNonZero averages over the non-zero tracks instead.
	rtt := avgNonZero(s.VideoRoundTripTime, s.AudioRoundTripTime)
	jitter := avgNonZero(s.VideoJitter, s.AudioJitter)
	loss := avgNonZero(s.VideoPacketLostRatio, s.AudioPacketLostRatio)

	return domain.ServerEvent{
		Version:  1,
		Type:     domain.EventWebRTCClientStats,
		TS:       time.Now().UnixMilli(),
		Source:   domain.SourceRestPoll,
		NodeID:   nodeID,
		App:      app,
		StreamID: streamID,
		Data: map[string]any{
			"client_id":       s.StatID,
			"rtt_ms":          rtt * 1000, // AMS reports in seconds
			"jitter_ms":       jitter * 1000,
			"packet_loss_pct": loss * 100,
		},
	}
}

// NormalizeSystemStats converts a raw AMS /rest/v2/system-status map into a
// domain.ServerEvent of type node_stats for a standalone (non-cluster) AMS node.
//
// REAL AMS 3.x system-status shape (confirmed by agents/handoffs/real-ams-captures/
// system-status.json and docs/AMS-INTEGRATION.md): ONLY {osName, osArch, javaVersion,
// processorCount} — NO cpu/mem/disk/network metrics. The old parsing of cpuUsage/
// jvmMemoryUsage/systemMemoryInfo/fileSystemInfo was reading non-existent fields
// and emitting all-zeros to ClickHouse. This rewrite maps the REAL fields and does
// NOT emit absent metrics (honest reporting; the UI already guards cpu_pct==null).
//
// The version param comes from a separate /rest/v2/version call (best-effort by the
// caller) and is written to Data["version"] so the fleet card can render it.
//
// Defensive: missing or wrong-typed fields default to zero/empty — never panic.
func NormalizeSystemStats(stats map[string]any, nodeID, version string) domain.ServerEvent {
	// Safely read string fields.
	osName, _ := stats["osName"].(string)
	osArch, _ := stats["osArch"].(string)
	javaVersion, _ := stats["javaVersion"].(string)

	// processorCount may arrive as float64 (JSON numbers always decode as float64).
	processorCount := 0
	if pc, ok := stats["processorCount"]; ok {
		switch v := pc.(type) {
		case float64:
			processorCount = int(v)
		case int:
			processorCount = v
		}
	}

	// Build data map with only the fields that are present (honest reporting).
	data := make(map[string]any)
	if osName != "" {
		data["os_name"] = osName
	}
	if osArch != "" {
		data["os_arch"] = osArch
	}
	if javaVersion != "" {
		data["java_version"] = javaVersion
	}
	if processorCount > 0 {
		data["processor_count"] = processorCount
	}
	if version != "" {
		data["version"] = version
	}
	// cpu_pct / mem_pct / disk_pct / net_* are NOT present in the real AMS 3.x
	// system-status response — do not emit zeros (those are fabricated metrics).

	return domain.ServerEvent{
		Version: 1,
		Type:    domain.EventNodeStats,
		TS:      time.Now().UnixMilli(),
		Source:  domain.SourceRestPoll,
		NodeID:  nodeID,
		Data:    data,
	}
}

// ─── /rest/v2/system-resources (D-179) ───────────────────────────────────────

// nestedMap returns m[key] as a map when it is one, else nil. Wrong-typed or
// absent values yield nil rather than panicking (hostile-input discipline).
func nestedMap(m map[string]any, key string) map[string]any {
	if m == nil {
		return nil
	}
	sub, _ := m[key].(map[string]any)
	return sub
}

// floatOK reads a JSON number field. ok=false when the key is absent or is not
// a finite number — the caller must then OMIT the derived metric rather than
// emit a zero (see the honesty rule on NormalizeSystemStats).
func floatOK(m map[string]any, key string) (float64, bool) {
	if m == nil {
		return 0, false
	}
	switch v := m[key].(type) {
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0, false
		}
		return v, true
	case int:
		return float64(v), true
	default:
		return 0, false
	}
}

// pctOK computes inUse/total*100, capped at 100. ok=false when either field is
// missing or total is zero — an unmeasurable ratio is absent, never 0%.
func pctOK(m map[string]any, inUseKey, totalKey string) (float64, bool) {
	inUse, okIn := floatOK(m, inUseKey)
	total, okTotal := floatOK(m, totalKey)
	if !okIn || !okTotal || total == 0 {
		return 0, false
	}
	v := inUse / total * 100
	if v > 100 {
		v = 100
	}
	return v, true
}

// HasResourceMetrics reports whether a /rest/v2/system-resources payload carries
// at least one of the three gauges. When it does not, the payload is no more
// informative than /rest/v2/system-status and the caller should fall back.
func HasResourceMetrics(res map[string]any) bool {
	if res == nil {
		return false
	}
	if _, ok := floatOK(nestedMap(res, "cpuUsage"), "systemCPULoad"); ok {
		return true
	}
	if _, ok := pctOK(nestedMap(res, "systemMemoryInfo"), "inUseMemory", "totalMemory"); ok {
		return true
	}
	_, ok := pctOK(nestedMap(res, "fileSystemInfo"), "inUseSpace", "totalSpace")
	return ok
}

// NormalizeSystemResources converts a raw AMS /rest/v2/system-resources map into
// a domain.ServerEvent of type node_stats for a standalone (non-cluster) AMS node.
//
// This is the richer sibling of NormalizeSystemStats: same event, same node
// identity keys, but it also carries cpu_pct / mem_pct / disk_pct, which a
// standalone AMS does report — just not on /rest/v2/system-status (D-179; this
// is what closed LIM-01).
//
// REAL AMS 3.0.3 shape (live capture, agents/handoffs/real-ams-captures/
// system-resources.json):
//
//	cpuUsage        {processCPUTime, systemCPULoad, processCPULoad, systemLoadAverageLastMinute}
//	systemMemoryInfo{virtualMemory, totalMemory, freeMemory, inUseMemory, …}
//	fileSystemInfo  {usableSpace, totalSpace, freeSpace, inUseSpace}
//	systemInfo      {osName, osArch, javaVersion, processorCount}   ← the whole system-status body
//	softwareVersion {versionName, versionType, buildNumber}          ← the whole /rest/v2/version body
//
// The percentage formulas are deliberately IDENTICAL to the Kafka
// ams-instance-stats path (internal/collector/kafka): both decode the same
// serialized StatsCollector object, so a node must not report different numbers
// depending on which transport observed it.
//
// Honesty rule (shared with NormalizeSystemStats / NormalizeClusterNode): a
// metric that cannot be computed is ABSENT from Data — never a zero. The UI and
// alert evaluator both treat key-absent as "not measured".
//
// Defensive: missing or wrong-typed fields are skipped — never panics.
func NormalizeSystemResources(res map[string]any, nodeID, version string) domain.ServerEvent {
	data := make(map[string]any)

	// Identity lives under systemInfo here (it is the system-status body verbatim),
	// but tolerate a flattened shape from an older/proxied AMS.
	info := nestedMap(res, "systemInfo")
	if info == nil {
		info = res
	}
	if v, ok := info["osName"].(string); ok && v != "" {
		data["os_name"] = v
	}
	if v, ok := info["osArch"].(string); ok && v != "" {
		data["os_arch"] = v
	}
	if v, ok := info["javaVersion"].(string); ok && v != "" {
		data["java_version"] = v
	}
	if v, ok := floatOK(info, "processorCount"); ok && v > 0 {
		data["processor_count"] = int(v)
	}

	// Version: the caller's explicit value wins (it may have a better source);
	// otherwise softwareVersion.versionName makes a separate /rest/v2/version
	// call unnecessary on this path.
	if version == "" {
		if v, ok := nestedMap(res, "softwareVersion")["versionName"].(string); ok {
			version = v
		}
	}
	if version != "" {
		data["version"] = version
	}

	// cpu_pct: systemCPULoad is ALREADY an integer percent (0-100). AMS's
	// SystemUtils.getSystemCpuLoad() casts (int)(load*100.0) before serializing,
	// so rescaling here would report 3400% CPU — the S105 trap, documented at
	// the Kafka call site too. systemCPULoad (whole node) not processCPULoad (JVM).
	if v, ok := floatOK(nestedMap(res, "cpuUsage"), "systemCPULoad"); ok && v >= 0 {
		// contracts/events/ams-server-event.schema.json bounds cpu_pct to [0,100];
		// clamp so an unexpected wire value cannot emit a contract-invalid event.
		// A negative reading is nonsense, not a measurement — omit it entirely.
		data["cpu_pct"] = math.Min(v, 100)
	}
	if v, ok := pctOK(nestedMap(res, "systemMemoryInfo"), "inUseMemory", "totalMemory"); ok {
		data["mem_pct"] = v
	}
	if v, ok := pctOK(nestedMap(res, "fileSystemInfo"), "inUseSpace", "totalSpace"); ok {
		data["disk_pct"] = v
	}

	return domain.ServerEvent{
		Version: 1,
		Type:    domain.EventNodeStats,
		TS:      time.Now().UnixMilli(),
		Source:  domain.SourceRestPoll,
		NodeID:  nodeID,
		Data:    data,
	}
}

// NormalizeClusterNode converts an AMS ClusterNodeDTO to a domain.ServerEvent of
// type node_stats. It handles both the real AMS 3.x wire fields (id, cpu, memory,
// status — A-amssource verified 2026-07-26) and the old invented aliases (nodeId,
// cpuUsage, memoryUsage, …) used by test mocks and fixtures.
func NormalizeClusterNode(n amsclient.ClusterNodeDTO) domain.ServerEvent {
	nodeID := n.PrimaryID()
	// cpu_pct / mem_pct come from the real AMS 3.x wire fields, but even those are
	// conditional: a non-finite JMX reading ("NaN"/"Inf") with no usable alias is
	// ABSENT, not zero (REVIEW-MP3-R5). The remaining fields are alias-only (real
	// AMS 3.x does not send them — they decode to zero/empty against a real
	// cluster). Never fabricate zeros — same rule as NormalizeSystemStats above
	// ("Disk 0%" rendered as a measurement is a lie; REVIEW-MP3 N-cluster c).
	data := map[string]any{}
	if v, ok := n.CPUPctOK(); ok {
		data["cpu_pct"] = v
	}
	if v, ok := n.MemPctOK(); ok {
		data["mem_pct"] = v
	}
	if n.DiskUsage != 0 {
		data["disk_pct"] = n.DiskUsage
	}
	if n.NetworkInputBps != 0 {
		data["net_in_mbps"] = n.NetworkInputBps / 1_000_000
	}
	if n.NetworkOutputBps != 0 {
		data["net_out_mbps"] = n.NetworkOutputBps / 1_000_000
	}
	if n.JvmMemoryUsage != 0 {
		data["jvm_heap_used_mb"] = n.JvmMemoryUsage
	}
	// FIX 1 (VD-40): Version was decoded from the DTO but never written into Data.
	// Emit it when present (mock/alias profiles); real AMS 3.x sends no version on
	// this endpoint, so cluster nodes legitimately show no version rather than "".
	if n.Version != "" {
		data["version"] = n.Version
	}
	return domain.ServerEvent{
		Version: 1,
		Type:    domain.EventNodeStats,
		TS:      time.Now().UnixMilli(),
		Source:  domain.SourceRestPoll,
		NodeID:  nodeID,
		Data:    data,
	}
}

// HashIP returns the SHA-256 hex of an IP address (privacy anonymization).
func HashIP(ip string) string {
	if ip == "" {
		return ""
	}
	h := sha256.Sum256([]byte(ip))
	return hex.EncodeToString(h[:])
}

// normalizePublishType maps AMS publish types to the normalized set.
func normalizePublishType(publishType, streamType string) string {
	switch publishType {
	case "webrtc", "WebRTC":
		return "webrtc"
	case "rtmp", "RTMP":
		return "rtmp"
	case "hls", "HLS":
		return "hls"
	case "mp4", "MP4":
		return "mp4"
	}
	// Fall back on stream type.
	switch streamType {
	case "liveStream":
		return "rtmp"
	}
	return "other"
}

// ifZero returns b if a is zero.
func ifZero(a, b int64) int64 {
	if a == 0 {
		return b
	}
	return a
}

// avgNonZero returns the mean of a and b counting only the non-zero values.
// Used for AMS WebRTC per-peer stats where a single-track (audio-only or
// video-only) viewer reports 0 for the absent track; a plain (a+b)/2 would
// halve the real metric. Both zero → 0.
func avgNonZero(a, b float64) float64 {
	switch {
	case a > 0 && b > 0:
		return (a + b) / 2
	case a > 0:
		return a
	case b > 0:
		return b
	default:
		return 0
	}
}

// buildEnrichment creates an EnrichmentBlock from IP and UA.
func buildEnrichment(ip, userAgent string, geo GeoResolver, ua UAParser) *domain.EnrichmentBlock {
	var g domain.GeoEnrichment
	var c domain.ClientEnrichment
	if ip != "" && geo != nil {
		g = geo.Resolve(ip)
	}
	if userAgent != "" && ua != nil {
		c = ua.Parse(userAgent)
	}
	if g.Country == "" && g.Region == "" && c.Device == "" {
		return nil // no enrichment — omit from JSON
	}
	return &domain.EnrichmentBlock{Geo: &g, Client: &c}
}
