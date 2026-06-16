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
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/pkg/amsclient"
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

		// FIX 2 (VD-??): AMS v2.10 sends speed and omits bitrate; fall back to
		// b.Speed when b.BitRate is zero so we never emit 0 kbps for live streams.
		effectiveBitrate := b.BitRate
		if effectiveBitrate == 0 && b.Speed > 0 {
			effectiveBitrate = b.Speed
		}

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
			"bitrate_kbps":    effectiveBitrate,
			"speed_read_kbps": b.Speed,
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

		// VD-22: emit ingest_stats from BroadcastDTO fields so REST-only
		// deployments surface FPS, bitrate, and other ingest metrics
		// (previously only Kafka/logtail sources emitted this event type).
		// AMS BroadcastDTO does not expose packet_loss or jitter via REST;
		// those fields are left zero and may be populated by WebRTC stats events.
		// FIX 2: use effectiveBitrate (Speed fallback) for the emit condition too.
		if b.CurrentFPS > 0 || effectiveBitrate > 0 {
			events = append(events, domain.ServerEvent{
				Version:  1,
				Type:     domain.EventIngestStats,
				TS:       now,
				Source:   domain.SourceRestPoll,
				NodeID:   nodeID,
				App:      app,
				StreamID: b.StreamID,
				Data: map[string]any{
					"bitrate_kbps": effectiveBitrate,
					"fps":          float64(b.CurrentFPS),
					// packet_loss_pct and jitter_ms are not available via AMS REST;
					// they are populated by WebRTC peer-stats events when available.
					"packet_loss_pct":    0.0,
					"jitter_ms":          0.0,
					"keyframe_interval_s": 0.0,
				},
				Enrichment: enrich,
			})
		}

	case "finished", "ended":
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
					"reason": "finished",
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
	rtt := (s.VideoRoundTripTime + s.AudioRoundTripTime) / 2
	jitter := (s.VideoJitter + s.AudioJitter) / 2
	loss := (s.VideoPacketLostRatio + s.AudioPacketLostRatio) / 2

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

// NormalizeClusterNode converts an AMS ClusterNodeDTO to a domain.ServerEvent of
// type node_stats.
func NormalizeClusterNode(n amsclient.ClusterNodeDTO) domain.ServerEvent {
	nodeID := n.NodeID
	if nodeID == "" {
		nodeID = n.IP
	}
	// FIX 1 (VD-40): Version was decoded from the DTO but never written into Data,
	// so aggregator.onNodeStats always read Data["version"] == "". Write it now.
	data := map[string]any{
		"cpu_pct":          n.CPUUsage,
		"mem_pct":          n.MemoryUsage,
		"disk_pct":         n.DiskUsage,
		"net_in_mbps":      n.NetworkInputBps / 1_000_000,
		"net_out_mbps":     n.NetworkOutputBps / 1_000_000,
		"jvm_heap_used_mb": n.JvmMemoryUsage,
		"version":          n.Version,
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
