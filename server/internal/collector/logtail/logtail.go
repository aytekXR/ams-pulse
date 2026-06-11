// Package logtail tails the AMS analytics log
// (/var/log/antmedia/ant-media-server-analytics.log, structured JSON, AMS
// v2.10+) with rotation awareness, and emits normalized events. Richer than
// REST polling for keyframe/bitrate ingest health (F4).
package logtail

// TODO(BE-01): Tailer implementing collector.Source. Handle log rotation,
// partial lines, and unknown event types (forward-compatible: skip + count).
