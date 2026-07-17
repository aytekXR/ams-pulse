// Gap-closure test (E2E-validation doc §5.1, gap G-01): pin the two-parameter
// normalizePublishType switch through the public NormalizeBroadcast boundary.
//
// The webhook package has a separate single-param version that IS tested
// (webhook_more_test.go); this collector-package version had no covering test.
// AMS 3.0.3 mislabels SRT ingest as publishType="RTMP" on the wire, so the
// authoritative live behavior is SRT→"rtmp" (via the RTMP case); the switch's
// own SRT-string / liveStream-fallback / unknown branches are pinned here so a
// refactor of the mapping cannot silently regress the protocol breakdown.
package collector

import (
	"testing"

	"github.com/pulse-analytics/pulse/server/pkg/amsclient"
)

func TestNormalizeBroadcast_PublishTypeMapping(t *testing.T) {
	cases := []struct {
		name        string
		publishType string
		streamType  string
		want        string
	}{
		{"rtmp lower", "rtmp", "", "rtmp"},
		{"rtmp upper (incl. AMS SRT-as-RTMP mislabel)", "RTMP", "", "rtmp"},
		{"webrtc lower", "webrtc", "", "webrtc"},
		{"webrtc mixed", "WebRTC", "", "webrtc"},
		{"hls lower", "hls", "", "hls"},
		{"hls upper", "HLS", "", "hls"},
		{"mp4 lower", "mp4", "", "mp4"},
		{"mp4 upper", "MP4", "", "mp4"},
		{"srt string falls through to other", "SRT", "", "other"},
		{"srt string with liveStream type → rtmp fallback", "SRT", "liveStream", "rtmp"},
		{"empty publishType + liveStream type → rtmp fallback", "", "liveStream", "rtmp"},
		{"unknown publishType, no stream type → other", "gopher", "", "other"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dto := amsclient.BroadcastDTO{
				StreamID:    "s1",
				AppName:     "live",
				Status:      "broadcasting",
				PublishType: tc.publishType,
				Type:        tc.streamType,
			}
			// prevStatus="" so a publish_start event (which carries publish_type) is emitted.
			events := NormalizeBroadcast(dto, "node-x", "", NoopGeoResolver{}, NoopUAParser{})

			var got string
			var found bool
			for _, ev := range events {
				if pt, ok := ev.Data["publish_type"].(string); ok {
					got, found = pt, true
					break
				}
			}
			if !found {
				t.Fatalf("no publish_start event carrying publish_type in %d events", len(events))
			}
			if got != tc.want {
				t.Fatalf("publishType=%q streamType=%q → publish_type=%q, want %q",
					tc.publishType, tc.streamType, got, tc.want)
			}
		})
	}
}
