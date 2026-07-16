package prober

// D-131: HLS manifest-parsing correctness.
//   [14] a segment URI preceded by #EXTINF:0 (or a malformed #EXTINF) must be
//        captured — not silently dropped and misreported as a healthy empty master.
//   [15] resolveURI must resolve protocol-relative / absolute-path references to
//        the correct host, not concatenate them onto the base path.

import (
	"errors"
	"strings"
	"testing"
)

func TestParseHLSManifest_ZeroEXTINF_CapturesSegment_S69(t *testing.T) {
	m := "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:6\n#EXTINF:0.000,\nseg001.ts\n#EXT-X-ENDLIST\n"
	uri, dur, isMaster, variantURI, err := parseHLSManifest(strings.NewReader(m), "http://h/live/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isMaster {
		t.Errorf("isMaster=true, want false (zero-EXTINF segment must NOT be misread as a master)")
	}
	if uri != "http://h/live/seg001.ts" {
		t.Errorf("uri=%q, want http://h/live/seg001.ts", uri)
	}
	if dur != 0 {
		t.Errorf("dur=%v, want 0", dur)
	}
	if variantURI != "" {
		t.Errorf("variantURI=%q, want empty", variantURI)
	}
}

func TestParseHLSManifest_MalformedEXTINF_CapturesSegment_S69(t *testing.T) {
	// #EXTINF with an unparseable duration → Sscanf leaves dur=0, but the segment
	// must still be captured (pendingExtInf), not dropped.
	m := "#EXTM3U\n#EXTINF:notanumber,\nseg.ts\n"
	uri, _, isMaster, _, err := parseHLSManifest(strings.NewReader(m), "http://h/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isMaster || uri != "http://h/seg.ts" {
		t.Errorf("got isMaster=%v uri=%q, want isMaster=false uri=http://h/seg.ts", isMaster, uri)
	}
}

func TestParseHLSManifest_NormalEXTINF_S69(t *testing.T) {
	m := "#EXTM3U\n#EXTINF:6.0,\nseg.ts\n"
	uri, dur, isMaster, _, err := parseHLSManifest(strings.NewReader(m), "http://h/")
	if err != nil || isMaster || uri != "http://h/seg.ts" || dur != 6.0 {
		t.Errorf("got uri=%q dur=%v isMaster=%v err=%v; want http://h/seg.ts 6 false nil", uri, dur, isMaster, err)
	}
}

func TestParseHLSManifest_MasterStreamInf_StillMaster_S69(t *testing.T) {
	m := "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1000000\nvariant.m3u8\n"
	uri, _, isMaster, variantURI, err := parseHLSManifest(strings.NewReader(m), "http://h/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isMaster || variantURI != "http://h/variant.m3u8" || uri != "" {
		t.Errorf("got isMaster=%v variantURI=%q uri=%q; want true http://h/variant.m3u8 (empty uri)", isMaster, variantURI, uri)
	}
}

func TestParseHLSManifest_EmptyPlaylist_IsMaster_S69(t *testing.T) {
	m := "#EXTM3U\n#EXT-X-TARGETDURATION:6\n"
	uri, _, isMaster, _, err := parseHLSManifest(strings.NewReader(m), "http://h/")
	if err != nil || !isMaster || uri != "" {
		t.Errorf("got isMaster=%v uri=%q err=%v; want true (empty uri) nil", isMaster, uri, err)
	}
}

func TestResolveURI_S69(t *testing.T) {
	base := "https://origin.com/hls/playlist.m3u8"
	cases := []struct{ uri, want, why string }{
		{"seg.ts", "https://origin.com/hls/seg.ts", "relative path resolves against base dir"},
		{"http://cdn.example.com/seg.ts", "http://cdn.example.com/seg.ts", "absolute URI kept as-is"},
		{"//cdn.example.com/seg.ts", "https://cdn.example.com/seg.ts", "protocol-relative inherits base scheme"},
		{"/seg.ts", "https://origin.com/seg.ts", "absolute-path resolves against base host root"},
		{"../seg.ts", "https://origin.com/seg.ts", "dot-segment resolves correctly"},
	}
	for _, c := range cases {
		if got := resolveURI(c.uri, base); got != c.want {
			t.Errorf("resolveURI(%q) = %q, want %q (%s)", c.uri, got, c.want, c.why)
		}
	}
	// Protocol-relative against an http base inherits http.
	if got := resolveURI("//cdn/seg.ts", "http://origin/hls/p.m3u8"); got != "http://cdn/seg.ts" {
		t.Errorf("protocol-relative over http base = %q, want http://cdn/seg.ts", got)
	}
}

// TestClassifyHTTPError_UnsupportedScheme_S69 pins the D-131 review fix: a segment
// URI with a non-http scheme (rejected by the transport before any dial) is
// classified as a manifest-content ("parse") fault, not a "network" fault.
func TestClassifyHTTPError_UnsupportedScheme_S69(t *testing.T) {
	err := errors.New(`Get "javascript:alert(1)": unsupported protocol scheme "javascript"`)
	if got := classifyHTTPError(err); got != "parse" {
		t.Errorf("classifyHTTPError(unsupported scheme) = %q, want parse", got)
	}
	// Sanity: the existing classifications are unchanged.
	for msg, want := range map[string]string{
		"dial tcp: i/o timeout":                    "timeout",
		"dial tcp: lookup foo: no such host":       "dns",
		"dial tcp 1.2.3.4:443: connection refused": "conn_refused",
		"some other failure":                       "network",
	} {
		if got := classifyHTTPError(errors.New(msg)); got != want {
			t.Errorf("classifyHTTPError(%q) = %q, want %q", msg, got, want)
		}
	}
}
