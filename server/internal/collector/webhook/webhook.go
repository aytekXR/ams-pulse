// Package webhook receives AMS lifecycle webhooks (publish start/stop,
// recording ready) for instant stream state changes — lower latency than the
// REST poll for F1's 10-second publish-visibility criterion and F5's
// 30-second detection-to-notification criterion.
package webhook

// TODO(BE-01): HTTP handler implementing collector.Source; shared-secret
// validation; tolerant parsing across AMS versions.
