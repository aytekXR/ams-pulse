// probe_dash.go — MPEG-DASH/MPD manifest probe (D-073 ruling).
//
// Mirrors probeHLS (prober.go:339-512) in:
//   - function signature and result construction pattern
//   - error codes: parse | http_4xx | http_5xx | http_NNN | timeout | dns |
//     conn_refused | network | read  (all via classifyHTTPError)
//   - TTFB measurement points and 1 ms floor (D-013)
//   - SUCCESS SEMANTICS: Success=true once the manifest returns 2xx and a
//     segment URL is derivable; segment fetch is a bonus measurement —
//     failures leave Success=true, just no segment metrics (prober.go:460-512)
//
// Divergences from HLS (documented inline and in notes to the wiring author):
//   - MPD parsing uses encoding/xml, not a line-scanner.
//   - Relative URL resolution uses net/url.ResolveReference (RFC 3986), NOT
//     resolveURI's string-truncation; this correctly handles ".." segments and
//     authority-relative refs that resolveURI would mishandle.
//   - "No derivable segment" is a parse failure with Success=false, because
//     'parses as MPD' requires a segment URL; unlike HLS, there is no
//     master/variant indirection layer that can succeed with Success=true.
//
// Fixture provenance note (SPEC-DERIVED, DASH-IF):
// The real AMS has DASH muxing disabled (404 verified live 2026-07-10, D-073)
// so live capture is impossible without mutating prod AMS.  All MPD fixtures
// in probe_dash_test.go are derived from DASH-IF IOP test vectors and the
// ISO/IEC 23009-1 §5 specification examples.
package prober

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// reNumberFmt matches DASH SegmentTemplate $Number<fmt>$ tokens that carry a
// printf-width format specifier (e.g. $Number%05d$).  Must be matched and
// replaced BEFORE the plain $Number$ substitution to prevent partial expansion.
var reNumberFmt = regexp.MustCompile(`\$Number%[^$]+\$`)

// reSafeNumberSpec matches the ONLY printf specifier DASH defines for $Number$:
// an optional zero-pad flag and a small decimal width, conversion 'd'
// (ISO/IEC 23009-1 §5.3.9.4.4, e.g. %d, %5d, %05d).  Width digits are bounded to
// three (≤ 999) so a hostile manifest cannot smuggle a huge width like
// $Number%999999999d$ — which would make fmt.Sprintf pad the number to ~1 GB.
// (Real DASH widths are 1–2 digits; 3 leaves comfortable headroom while the ≤999
// bound keeps any single expansion under ~1 KB.)
var reSafeNumberSpec = regexp.MustCompile(`^%0?\d{0,3}d$`)

// maxMPDBodyBytes caps the MPD manifest body read before XML decode.  Unlike the
// segment body (segBodyCapBytes), the manifest is metadata — a live or short-VOD
// manifest is well under this.  A hostile probed server can otherwise return a
// manifest with millions of elements and drive xml.Decoder into a multi-GB
// allocation → OOM.  A body larger than this cap truncates, the decode fails, and
// the probe is reported as a parse failure (Success=false) — the same outcome as
// any malformed manifest.  Tradeoff (accepted): a pathologically large archive
// manifest (e.g. a dense multi-representation SegmentList > 16 MiB) is reported as
// a parse failure rather than probed — a deliberate bound favouring prober
// stability over covering giant-archive DASH origins (AMS itself is live-first).
const maxMPDBodyBytes = 16 << 20 // 16 MiB

// maxExpandedTemplateBytes bounds the output of expandSegmentTemplate.  A real
// SegmentTemplate @media is a short URL template, but both it and the
// Representation @id are attacker-controlled; strings.ReplaceAll allocates
// count×len(id) up front, so many "$RepresentationID$" tokens × a long id could
// reach TB-scale even within the 16 MiB body cap.  64 KiB is far above any real
// segment URL yet keeps the expansion bounded.
const maxExpandedTemplateBytes = 64 << 10 // 64 KiB

// ─── MPD XML types ────────────────────────────────────────────────────────────
// All struct field tags use unqualified local names only; no namespace is
// specified, so encoding/xml matches any DASH-IF namespace variant
// (urn:mpeg:dash:schema:mpd:2011, etc.) without modification.

type mpdDoc struct {
	BaseURL []string    `xml:"BaseURL"`
	Period  []mpdPeriod `xml:"Period"`
}

type mpdPeriod struct {
	BaseURL       []string           `xml:"BaseURL"`
	AdaptationSet []mpdAdaptationSet `xml:"AdaptationSet"`
}

type mpdAdaptationSet struct {
	BaseURL         []string            `xml:"BaseURL"`
	SegmentTemplate *mpdSegmentTemplate `xml:"SegmentTemplate"`
	SegmentList     *mpdSegmentList     `xml:"SegmentList"`
	Representation  []mpdRepresentation `xml:"Representation"`
}

type mpdRepresentation struct {
	ID              string              `xml:"id,attr"`
	BaseURL         []string            `xml:"BaseURL"`
	SegmentTemplate *mpdSegmentTemplate `xml:"SegmentTemplate"`
	SegmentList     *mpdSegmentList     `xml:"SegmentList"`
}

type mpdSegmentTemplate struct {
	Media       string `xml:"media,attr"`
	StartNumber int    `xml:"startNumber,attr"`
	Duration    int64  `xml:"duration,attr"`
	Timescale   int64  `xml:"timescale,attr"`
}

type mpdSegmentList struct {
	Duration   int64           `xml:"duration,attr"`
	Timescale  int64           `xml:"timescale,attr"`
	SegmentURL []mpdSegmentURL `xml:"SegmentURL"`
}

type mpdSegmentURL struct {
	Media string `xml:"media,attr"`
}

// ─── probeDASH ────────────────────────────────────────────────────────────────

// probeDASH performs a DASH/MPD playback probe:
//  1. GET manifest URL, measure TTFB.
//  2. Parse MPD XML — locate first Period → AdaptationSet → Representation and
//     derive the first segment URL via SegmentTemplate or SegmentList.
//  3. GET first segment (bonus measurement); segment failures leave Success=true.
//
// Wiring: add case "dash": r.probeDASH(ctx, p, result) in executeProbe.
func (r *Runner) probeDASH(ctx context.Context, p domain.ProbeConfig, result domain.ProbeResult) domain.ProbeResult {
	// ── Step 1: fetch manifest ─────────────────────────────────────────────────
	manifestStart := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.URL, nil)
	if err != nil {
		result.Success = false
		result.ErrorCode = "parse"
		result.ErrorMsg = fmt.Sprintf("build request: %v", err)
		return result
	}
	req.Header.Set("User-Agent", r.cfg.HTTPUserAgent)

	resp, err := r.client.Do(req)
	ttfbMs := uint32(time.Since(manifestStart).Milliseconds())
	// 1 ms floor: time.Since().Milliseconds() rounds down; sub-millisecond
	// loopback responses produce 0 even for a real TCP round trip (D-013).
	if ttfbMs == 0 {
		ttfbMs = 1
	}
	result.TTFBMs = ttfbMs

	if err != nil {
		result.Success = false
		result.ErrorCode = classifyHTTPError(err)
		result.ErrorMsg = err.Error()
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 == 4 {
		result.Success = false
		result.ErrorCode = "http_4xx"
		result.ErrorMsg = fmt.Sprintf("manifest HTTP %d", resp.StatusCode)
		return result
	}
	if resp.StatusCode/100 == 5 {
		result.Success = false
		result.ErrorCode = "http_5xx"
		result.ErrorMsg = fmt.Sprintf("manifest HTTP %d", resp.StatusCode)
		return result
	}
	if resp.StatusCode/100 != 2 {
		result.Success = false
		result.ErrorCode = fmt.Sprintf("http_%d", resp.StatusCode)
		result.ErrorMsg = fmt.Sprintf("manifest HTTP %d", resp.StatusCode)
		return result
	}

	// ── Step 2: parse MPD ──────────────────────────────────────────────────────
	// Cap the manifest body: xml.Decoder materialises the whole document, so an
	// unbounded read lets a hostile server OOM the prober (mirrors the segment cap
	// at segBodyCapBytes below).
	segmentURI, segmentDurationS, err := parseMPD(io.LimitReader(resp.Body, maxMPDBodyBytes), p.URL)
	if err != nil {
		result.Success = false
		result.ErrorCode = "parse"
		result.ErrorMsg = fmt.Sprintf("parse MPD: %v", err)
		return result
	}

	// ── Step 3: fetch first segment (bonus measurement) ────────────────────────
	// Mirrors prober.go:460-512: manifest success is already established;
	// segment failures annotate the error but leave Success=true.
	segReq, err := http.NewRequestWithContext(ctx, http.MethodGet, segmentURI, nil)
	if err != nil {
		result.Success = true // manifest OK
		result.ErrorCode = "parse"
		result.ErrorMsg = fmt.Sprintf("segment request: %v", err)
		return result
	}
	segReq.Header.Set("User-Agent", r.cfg.HTTPUserAgent)

	segStart := time.Now()
	segResp, err := r.client.Do(segReq)
	segTTFBMs := uint32(time.Since(segStart).Milliseconds())
	// Same 1 ms floor as manifest TTFB (D-013).
	if segTTFBMs == 0 {
		segTTFBMs = 1
	}
	if err != nil {
		result.Success = true // manifest OK; segment is bonus measurement
		result.ErrorCode = classifyHTTPError(err)
		result.ErrorMsg = fmt.Sprintf("segment: %v", err)
		return result
	}
	defer segResp.Body.Close()

	if segResp.StatusCode/100 != 2 {
		result.Success = true // manifest OK
		result.ErrorCode = fmt.Sprintf("http_%d", segResp.StatusCode)
		result.ErrorMsg = fmt.Sprintf("segment HTTP %d", segResp.StatusCode)
		return result
	}

	// Record segment TTFB on successful 2xx response.
	result.SegmentTTFBMs = segTTFBMs

	segBytes, err := io.ReadAll(io.LimitReader(segResp.Body, segBodyCapBytes+1))
	if err != nil {
		result.Success = true
		result.ErrorCode = "read"
		result.ErrorMsg = fmt.Sprintf("read segment: %v", err)
		return result
	}

	// Enforce the segment body cap: a segment larger than segBodyCapBytes is
	// flagged as segment_too_large.  BitrateKbps stays 0 — a bitrate computed
	// from a truncated body would be meaningless.  Success=true because the
	// manifest was valid ("segment is bonus measurement" invariant, D-074 WO-F).
	if len(segBytes) > segBodyCapBytes {
		result.Success = true
		result.ErrorCode = "segment_too_large"
		result.ErrorMsg = fmt.Sprintf("segment body exceeds %d-byte cap (%d bytes read via LimitReader)",
			segBodyCapBytes, len(segBytes))
		return result
	}

	// Compute bitrate: segment_bytes * 8 bits / segment_duration_s / 1000 = kbps.
	// timescale-adjusted duration; duration=0 means unknown — skip (leave 0).
	if segmentDurationS > 0 {
		result.BitrateKbps = float32(float64(len(segBytes)) * 8.0 / segmentDurationS / 1000.0)
	}

	result.Success = true
	result.ErrorCode = ""
	result.ErrorMsg = ""
	return result
}

// ─── parseMPD ─────────────────────────────────────────────────────────────────

// parseMPD reads an MPEG-DASH MPD body and returns the URL of the first
// derivable media segment plus its nominal duration in seconds.
//
// Segment derivation (in priority order for each Representation):
//  1. SegmentTemplate on Representation (wins) or AdaptationSet (fallback):
//     media attr with $RepresentationID$ and $Number[%fmt]$ substitution;
//     startNumber defaults to 1; timescale defaults to 1;
//     segmentDurationS = duration / timescale.
//  2. SegmentList on Representation (wins) or AdaptationSet (fallback):
//     first SegmentURL@media attribute; duration/timescale as above.
//
// Neither found → error (parse failure, Success=false).
// Per D-073 ruling: 'parses as MPD' requires a derivable segment URL;
// a well-formed MPD without SegmentTemplate/SegmentList cannot guide
// playback measurement and is therefore treated as a probe failure.
//
// baseURL is the absolute URL of the MPD; it is the root for BaseURL element
// resolution (MPD-level and/or Representation-level) via net/url.ResolveReference
// (RFC 3986).  This diverges from HLS resolveURI (string truncation after the
// last '/') to correctly handle ".." path segments and authority-relative refs.
func parseMPD(body io.Reader, baseURL string) (segmentURI string, segmentDurationS float64, err error) {
	decoder := xml.NewDecoder(body)

	// Locate the root start element; skip XML declarations, processing
	// instructions, and comments.  A non-XML body will surface an error here.
	var rootStart xml.StartElement
	for {
		tok, decErr := decoder.Token()
		if decErr != nil {
			return "", 0, fmt.Errorf("not a valid XML document: %v", decErr)
		}
		if start, ok := tok.(xml.StartElement); ok {
			rootStart = start
			break
		}
	}
	if rootStart.Name.Local != "MPD" {
		return "", 0, fmt.Errorf("root element is %q, want MPD", rootStart.Name.Local)
	}

	var doc mpdDoc
	if decErr := decoder.DecodeElement(&doc, &rootStart); decErr != nil {
		return "", 0, fmt.Errorf("decode MPD XML: %v", decErr)
	}

	// Resolve MPD-level BaseURL against the manifest URL.
	effectiveBase := baseURL
	if len(doc.BaseURL) > 0 && doc.BaseURL[0] != "" {
		effectiveBase = resolveDASHRef(doc.BaseURL[0], baseURL)
	}

	// Walk Period → AdaptationSet → Representation to find the first derivable
	// segment URL. BaseURL elements chain at every level (ISO/IEC 23009-1 §5.6):
	// MPD → Period → AdaptationSet → Representation, each resolved against its
	// parent's effective base.
	for p := range doc.Period {
		periodBase := effectiveBase
		if len(doc.Period[p].BaseURL) > 0 && doc.Period[p].BaseURL[0] != "" {
			periodBase = resolveDASHRef(doc.Period[p].BaseURL[0], effectiveBase)
		}
		for a := range doc.Period[p].AdaptationSet {
			as := &doc.Period[p].AdaptationSet[a]
			asBase := periodBase
			if len(as.BaseURL) > 0 && as.BaseURL[0] != "" {
				asBase = resolveDASHRef(as.BaseURL[0], periodBase)
			}
			for ri := range as.Representation {
				rep := &as.Representation[ri]

				// Compute the effective base URL for this Representation.
				repBase := asBase
				if len(rep.BaseURL) > 0 && rep.BaseURL[0] != "" {
					repBase = resolveDASHRef(rep.BaseURL[0], asBase)
				}

				// SegmentTemplate: Representation wins over AdaptationSet.
				st := rep.SegmentTemplate
				if st == nil {
					st = as.SegmentTemplate
				}
				if st != nil {
					startNumber := st.StartNumber
					if startNumber == 0 {
						startNumber = 1 // DASH-IF default (ISO/IEC 23009-1 §5.3.9.5.3)
					}
					timescale := st.Timescale
					if timescale == 0 {
						timescale = 1 // DASH-IF default
					}
					var segDurS float64
					if st.Duration > 0 {
						segDurS = float64(st.Duration) / float64(timescale)
					}
					rawMedia := expandSegmentTemplate(st.Media, rep.ID, startNumber)
					segURI := resolveDASHRef(rawMedia, repBase)
					return segURI, segDurS, nil
				}

				// SegmentList: Representation wins over AdaptationSet.
				sl := rep.SegmentList
				if sl == nil {
					sl = as.SegmentList
				}
				if sl != nil && len(sl.SegmentURL) > 0 && sl.SegmentURL[0].Media != "" {
					timescale := sl.Timescale
					if timescale == 0 {
						timescale = 1
					}
					var segDurS float64
					if sl.Duration > 0 {
						segDurS = float64(sl.Duration) / float64(timescale)
					}
					segURI := resolveDASHRef(sl.SegmentURL[0].Media, repBase)
					return segURI, segDurS, nil
				}
			}
		}
	}

	// No derivable segment URL found anywhere in the manifest.
	// Per D-073 ruling: 'parses as MPD' requires a derivable segment;
	// a document with neither SegmentTemplate nor SegmentList is a FAILED probe
	// (Success=false), because it cannot guide playback measurement.
	return "", 0, fmt.Errorf("no SegmentTemplate or SegmentList found in MPD (cannot derive segment URL)")
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// expandSegmentTemplate substitutes DASH SegmentTemplate identifier tokens:
//   - $RepresentationID$ → repID
//   - $Number%<fmt>$     → fmt.Sprintf("%<fmt>", number) (printf-width form,
//     e.g. $Number%05d$ → "00001" for number=1)
//   - $Number$           → decimal number string
//
// The printf-width form is substituted before the plain form to prevent
// partial-match corruption when both patterns are present.
func expandSegmentTemplate(media, repID string, number int) string {
	// media and repID are both attacker-controlled (from the probed server's
	// manifest). strings.ReplaceAll allocates count×len(repID) up front, so a media
	// of many "$RepresentationID$" tokens × a long id can reach TB-scale even within
	// the 16 MiB manifest-body cap. A real segment template is a short URL — refuse
	// to expand one whose $RepresentationID$ product, or raw size, blows past the
	// bound; the empty result yields an unresolvable segment URL (probe fails on the
	// segment fetch) rather than an OOM.
	if len(media) > maxExpandedTemplateBytes ||
		strings.Count(media, "$RepresentationID$")*len(repID) > maxExpandedTemplateBytes {
		return ""
	}
	out := strings.ReplaceAll(media, "$RepresentationID$", repID)
	// Printf-width $Number%<spec>$: replace first (before plain $Number$).
	out = reNumberFmt.ReplaceAllStringFunc(out, func(m string) string {
		// m is e.g. "$Number%05d$"; extract the format specifier "%05d".
		spec := m[len("$Number") : len(m)-1]
		if !reSafeNumberSpec.MatchString(spec) {
			// The spec comes straight from the (untrusted) manifest. A malformed
			// or oversized width like "%999999999d" would make fmt.Sprintf pad the
			// number to ~1 GB. Only a bounded DASH %0<width>d form is honoured;
			// anything else degrades to plain decimal.
			return fmt.Sprintf("%d", number)
		}
		return fmt.Sprintf(spec, number)
	})
	// Plain $Number$ (no format specifier).
	out = strings.ReplaceAll(out, "$Number$", fmt.Sprintf("%d", number))
	return out
}

// resolveDASHRef resolves a DASH URI reference against a base URL using RFC
// 3986 semantics (net/url.URL.ResolveReference).
//
// Diverges from HLS resolveURI (prober.go:591-599) which truncates the base
// path at the last '/'.  That shortcut works for simple relative paths but
// silently mishandles ".." segments and authority-relative references — both
// legal in DASH manifests (D-073 ruling).
func resolveDASHRef(ref, base string) string {
	if ref == "" {
		return base
	}
	refURL, err := url.Parse(ref)
	if err != nil || refURL.IsAbs() {
		// Absolute or unparseable ref — use as-is.
		return ref
	}
	if base == "" {
		return ref
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return ref
	}
	return baseURL.ResolveReference(refURL).String()
}
