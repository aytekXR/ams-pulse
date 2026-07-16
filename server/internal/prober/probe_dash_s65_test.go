// S65 (D-127) — prober untrusted-input hardening for the DASH probe.
//
//   - [3] The MPD manifest body is now read through io.LimitReader(maxMPDBodyBytes)
//     before xml.Decode, so a hostile probed server cannot return a giant manifest
//     and drive the decoder into a multi-GB allocation. An over-cap body truncates
//     → the decode fails → the probe is a "parse" failure (Success=false).
//   - [4] expandSegmentTemplate no longer feeds the manifest's $Number%<spec>$
//     verbatim to fmt.Sprintf. Only a bounded DASH %0<width>d form is honoured; a
//     hostile width like %999999999d (which would pad the number to ~1 GB) degrades
//     to plain decimal.
//
// Mutation proofs:
//   - [3] revert the LimitReader (parseMPD(resp.Body, …)) → the padded manifest
//     decodes successfully, the probe proceeds to the segment fetch, and Success
//     flips to true (ErrorCode no longer "parse") → the oversized-manifest test reddens.
//   - [4] revert the reSafeNumberSpec guard → fmt.Sprintf("%9999999d", n) allocates
//     ~10 MB and the output length blows past the bound → the oversized-width test reddens.
package prober

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestProbeDASH_OversizedManifest_ParseFailure serves a valid MPD (with a usable
// SegmentTemplate) padded past maxMPDBodyBytes. With the manifest cap the decode
// truncates → "parse" failure; without it the manifest would decode and the probe
// would move on to the segment fetch.
func TestProbeDASH_OversizedManifest_ParseFailure(t *testing.T) {
	// Comment padding just over the 16 MiB cap. 'A' only — an XML comment may not
	// contain "--". The closing </MPD> sits AFTER the padding, so a 16 MiB-capped
	// read never reaches it and the decode fails with an unexpected EOF.
	padding := strings.Repeat("A", maxMPDBodyBytes+(1<<20)) // 17 MiB

	mux := http.NewServeMux()
	mux.HandleFunc("/manifest.mpd", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/dash+xml")
		// Prefix with a fully-formed SegmentTemplate so that, absent the cap, the
		// manifest parses and yields a derivable segment URL.
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static">
  <Period start="PT0S">
    <AdaptationSet mimeType="video/mp4">
      <SegmentTemplate media="seg$Number%05d$.m4s" timescale="1" duration="2" startNumber="1"/>
      <Representation id="1" bandwidth="1000000"/>
    </AdaptationSet>
  </Period>
  <!-- ` + padding + ` -->
</MPD>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	r := dashTestRunner()
	res := r.probeDASH(context.Background(), dashConfig(srv.URL+"/manifest.mpd"), dashResult())

	if res.Success {
		t.Fatalf("oversized manifest must fail the probe (cap truncates the decode); got Success=true ErrorCode=%q", res.ErrorCode)
	}
	if res.ErrorCode != "parse" {
		t.Fatalf("want ErrorCode=parse for a truncated manifest, got %q (msg=%q)", res.ErrorCode, res.ErrorMsg)
	}
	if !strings.Contains(res.ErrorMsg, "parse MPD") {
		t.Errorf("want a parse-MPD error message, got %q", res.ErrorMsg)
	}
}

func TestExpandSegmentTemplate_RejectsOversizedNumberWidth_S65(t *testing.T) {
	// A hostile manifest supplies a huge printf width. Honouring it would make
	// fmt.Sprintf pad the number to ~10 MB; the guard must degrade to plain decimal.
	out := expandSegmentTemplate("seg_$Number%9999999d$.m4s", "1", 5)
	if len(out) > 64 {
		t.Fatalf("oversized $Number width was honoured: output length=%d (expected bounded fallback)", len(out))
	}
	if out != "seg_5.m4s" {
		t.Errorf("want plain-decimal fallback seg_5.m4s, got %q", out)
	}
}

func TestExpandSegmentTemplate_HonoursValidWidth_S65(t *testing.T) {
	// Positive control: a legitimate bounded DASH width is still applied.
	if out := expandSegmentTemplate("seg_$Number%05d$.m4s", "1", 5); out != "seg_00005.m4s" {
		t.Errorf("valid %%05d width not honoured: got %q, want seg_00005.m4s", out)
	}
	// Plain $Number$ (no spec) and $RepresentationID$ still expand.
	if out := expandSegmentTemplate("$RepresentationID$_$Number$.m4s", "vid", 42); out != "vid_42.m4s" {
		t.Errorf("plain expansion regressed: got %q, want vid_42.m4s", out)
	}
	// A 3-digit width (spec-legal, ~100 bytes) is honoured after widening the bound
	// from 2 to 3 digits — no longer wrongly degraded to plain decimal.
	if out := expandSegmentTemplate("seg_$Number%100d$.m4s", "1", 5); len(out) != len("seg_.m4s")+100 {
		t.Errorf("width-100 not honoured (should pad to 100 chars): got len %d, want %d", len(out), len("seg_.m4s")+100)
	}
}

// TestExpandSegmentTemplate_BoundsRepresentationIDExpansion_S65 proves the
// $RepresentationID$ substitution is size-bounded: strings.ReplaceAll would
// otherwise allocate count×len(repID) up front (found by the S65 adversarial
// review — a sibling of the [4] printf sink).
func TestExpandSegmentTemplate_BoundsRepresentationIDExpansion_S65(t *testing.T) {
	// 2000 tokens (~36 KB media, under the 64 KiB raw cap) × a 64 KiB id →
	// unguarded output would be ~131 MiB. The product guard must refuse it.
	media := strings.Repeat("$RepresentationID$", 2000)
	repID := strings.Repeat("A", 64<<10)
	if out := expandSegmentTemplate(media, repID, 1); out != "" {
		t.Fatalf("oversized $RepresentationID$ expansion not bounded: output len=%d (want empty)", len(out))
	}
}
