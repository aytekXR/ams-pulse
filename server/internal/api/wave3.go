// Package api — Wave 3 (WO-302) handlers: F9 anomaly detection + F10 synthetic probes.
//
// Tier gating (§7.11 pricing table):
//   - /anomalies: Enterprise only
//   - /probes CRUD + /probes/{id}/results: Pro+ (Pro and Enterprise)
//
// All handlers delegate business logic to the meta store (probe CRUD) and the
// anomaly detector. The API layer is thin: auth, gating, HTTP marshaling only.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// ─── F9 Anomaly detection ────────────────────────────────────────────────────

// handleAnomalies implements GET /api/v1/anomalies.
// Enterprise tier required per §7.11.
//
// BUG-008 Group A fix (S22/D-084): reads and applies app, stream, limit, cursor.
// Pagination cursor is a plain decimal integer offset over the in-memory filtered+sorted
// slice — chosen because ComputeFlags returns an ephemeral point-in-time list; an
// opaque offset is sufficient and avoids the complexity of a keyset cursor on volatile
// data. Invalid or absent cursor → first page (offset 0).
//
// BUG-008 Group B fix (S24/D-086, ADR-0009): when ?from or ?to is present in the
// raw query string, the request is routed to flagHistoryQuerier.QueryFlagHistory.
// Routing uses raw string presence (q.Get != "") — NEVER the parsed zero-time — to
// prevent malformed params from silently falling through to ComputeFlags (which would
// re-introduce the dishonest behavior the ADR forbids).
// Branch runs BEFORE the nil-anomalyDetector early-return so servers without an
// anomaly detector can still serve history queries.
func (s *Server) handleAnomalies(w http.ResponseWriter, r *http.Request) {
	// Tier gate: anomaly detection is Enterprise only.
	if err := s.lic.CheckAnomalies(); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
		return
	}

	q := r.URL.Query()

	// Parse min_sigma query param (OpenAPI default 2.0; use detector default when absent).
	var sigmaThreshold float64
	if minSigmaStr := q.Get("min_sigma"); minSigmaStr != "" {
		v, err := strconv.ParseFloat(minSigmaStr, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_PARAM", "min_sigma must be a number")
			return
		}
		sigmaThreshold = v
	}

	// Parse Group-A filter params (BUG-008 partial fix S22/D-084).
	metricFilter := q.Get("metric")
	appFilter := q.Get("app")
	streamFilter := q.Get("stream")

	// Parse limit: OpenAPI default 50, max 500.
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	// Parse cursor as a decimal integer offset for in-memory pagination.
	// Invalid or absent cursor (Atoi error or negative) → first page (offset 0).
	offset, _ := strconv.Atoi(q.Get("cursor"))
	if offset < 0 {
		offset = 0
	}

	// ── BUG-008 Group B: history routing (ADR-0009 §6) ───────────────────────
	// Route on raw string presence, not parsed value: a malformed ?from=abc
	// parses to zero time — falling through to ComputeFlags would silently drop
	// the param, recreating the dishonest behavior this fix addresses.
	fromRaw := q.Get("from")
	toRaw := q.Get("to")
	if fromRaw != "" || toRaw != "" {
		if s.flagHistoryQuerier == nil {
			writeError(w, http.StatusBadRequest, "FLAG_STORE_NOT_CONFIGURED",
				"anomaly flag-event store not configured; ?from/?to requires persistent history (ADR-0009)")
			return
		}
		from := parseTimeParam(fromRaw)
		if fromRaw != "" && from.IsZero() {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST",
				"invalid from: must be epoch-milliseconds or RFC3339")
			return
		}
		to := parseTimeParam(toRaw)
		if toRaw != "" && to.IsZero() {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST",
				"invalid to: must be epoch-milliseconds or RFC3339")
			return
		}
		// Pass raw cursor — the querier decodes base64; the ComputeFlags
		// decimal-offset cursor is a different namespace (never Atoi here).
		page, err := s.flagHistoryQuerier.QueryFlagHistory(
			r.Context(), from, to,
			metricFilter, appFilter, streamFilter,
			sigmaThreshold, limit, q.Get("cursor"),
		)
		if err != nil {
			if errors.Is(err, ErrBadCursor) {
				writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid cursor")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		// make() is never nil, so an empty page serializes as [] not null.
		items := make([]any, len(page.Items))
		for i, f := range page.Items {
			items[i] = f
		}
		// next_cursor: "" (last page) → null per spec [string,null].
		var nextCursor *string
		if page.NextCursor != "" {
			nc := page.NextCursor
			nextCursor = &nc
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"items": items,
			"meta":  map[string]any{"next_cursor": nextCursor},
		})
		return
	}
	// ── End history routing ───────────────────────────────────────────────────

	if s.anomalyDetector == nil {
		// Detector not wired (e.g., ClickHouse not available in test env).
		writeJSON(w, http.StatusOK, map[string]any{
			"items": []any{},
			"meta":  map[string]any{"next_cursor": nil},
		})
		return
	}

	flags, err := s.anomalyDetector.ComputeFlags(r.Context(), sigmaThreshold)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	// Apply optional filters: metric, app, stream.
	filtered := make([]AnomalyFlagAPI, 0, len(flags))
	for _, f := range flags {
		if metricFilter != "" && f.Metric != metricFilter {
			continue
		}
		if appFilter != "" && f.Scope.App != appFilter {
			continue
		}
		if streamFilter != "" && f.Scope.StreamID != streamFilter {
			continue
		}
		filtered = append(filtered, f)
	}

	// Deterministic sort: ascending TS, then by ID for tie-breaking.
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].TS != filtered[j].TS {
			return filtered[i].TS < filtered[j].TS
		}
		return filtered[i].ID < filtered[j].ID
	})

	// Clamp offset to valid range; past-end cursor → empty last page, no cursor.
	if offset > len(filtered) {
		offset = len(filtered)
	}
	page := filtered[offset:]

	// Emit next_cursor only when more items remain beyond this page.
	var nextCursor *string
	if len(page) > limit {
		page = page[:limit]
		nc := strconv.Itoa(offset + limit)
		nextCursor = &nc
	}

	items := make([]any, len(page))
	for i, f := range page {
		items[i] = f
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"meta":  map[string]any{"next_cursor": nextCursor},
	})
}

// ─── F10 Probes ───────────────────────────────────────────────────────────────

// probeToAPI converts a meta.ProbeRow to the API Probe shape.
// last_result is nil here; callers populate it separately if needed.
func probeToAPI(p meta.ProbeRow) map[string]any {
	m := map[string]any{
		"id":         p.ID,
		"name":       p.Name,
		"url":        p.URL,
		"protocol":   p.Protocol,
		"interval_s": p.IntervalS,
		"timeout_s":  p.TimeoutS,
		"enabled":    p.Enabled,
		"created_at": p.CreatedAt,
	}
	if p.LastResultID.Valid || p.LastSuccess.Valid || p.LastRunAt.Valid {
		m["last_result"] = buildLastResult(p)
	}
	return m
}

// buildLastResult builds the last_result summary from denorm probe fields.
func buildLastResult(p meta.ProbeRow) map[string]any {
	lr := map[string]any{
		"id":       p.LastResultID.String,
		"probe_id": p.ID,
	}
	if p.LastRunAt.Valid {
		lr["ts"] = p.LastRunAt.Int64
	}
	if p.LastSuccess.Valid {
		lr["success"] = p.LastSuccess.Int64 != 0
	}
	lr["ttfb_ms"] = nil
	return lr
}

// handleListProbes implements GET /api/v1/probes.
// Pro+ required.
func (s *Server) handleListProbes(w http.ResponseWriter, r *http.Request) {
	if err := s.lic.CheckProbes(); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
		return
	}

	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	cursor := q.Get("cursor")
	probes, err := s.store.ListProbes(r.Context(), limit+1, cursor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	var nextCursor *string
	if len(probes) > limit {
		probes = probes[:limit]
		last := probes[len(probes)-1]
		c := fmt.Sprintf("%d:%s", last.CreatedAt, last.ID)
		nextCursor = &c
	}
	items := make([]any, 0, len(probes))
	for _, p := range probes {
		items = append(items, probeToAPI(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"meta":  map[string]any{"next_cursor": nextCursor},
	})
}

// handleCreateProbe implements POST /api/v1/probes.
// Pro+ required. Validates interval_s >= 30.
func (s *Server) handleCreateProbe(w http.ResponseWriter, r *http.Request) {
	if err := s.lic.CheckProbes(); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
		return
	}

	var body struct {
		Name      string `json:"name"`
		URL       string `json:"url"`
		Protocol  string `json:"protocol"`
		IntervalS int    `json:"interval_s"`
		TimeoutS  int    `json:"timeout_s"`
		Enabled   *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}

	// Validate required fields.
	if body.Name == "" {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_PROBE", "name is required")
		return
	}
	if body.URL == "" {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_PROBE", "url is required")
		return
	}
	if body.IntervalS < 30 {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_PROBE", "interval_s must be >= 30")
		return
	}
	if body.Protocol != "" {
		switch body.Protocol {
		case "hls", "webrtc", "rtmp", "dash":
			// valid
		default:
			writeError(w, http.StatusUnprocessableEntity, "INVALID_PROBE",
				"protocol must be one of: hls, webrtc, rtmp, dash")
			return
		}
	}

	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	timeoutS := body.TimeoutS
	if timeoutS == 0 {
		timeoutS = 10
	}

	row := meta.ProbeRow{
		Name:      body.Name,
		URL:       body.URL,
		Protocol:  body.Protocol,
		IntervalS: body.IntervalS,
		TimeoutS:  timeoutS,
		Enabled:   enabled,
	}
	created, err := s.store.CreateProbe(r.Context(), row)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	s.audit(r, "probe.create", "probe", created.ID, map[string]any{"name": created.Name, "protocol": created.Protocol})
	writeJSON(w, http.StatusCreated, probeToAPI(created))
}

// handleUpdateProbe implements PUT /api/v1/probes/{probeId}.
// Pro+ required.
func (s *Server) handleUpdateProbe(w http.ResponseWriter, r *http.Request) {
	if err := s.lic.CheckProbes(); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
		return
	}

	probeID := chi.URLParam(r, "probeId")
	existing, err := s.store.GetProbe(r.Context(), probeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "probe not found")
		return
	}

	var body struct {
		Name      string `json:"name"`
		URL       string `json:"url"`
		Protocol  string `json:"protocol"`
		IntervalS int    `json:"interval_s"`
		TimeoutS  int    `json:"timeout_s"`
		Enabled   *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}

	if body.IntervalS != 0 && body.IntervalS < 30 {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_PROBE", "interval_s must be >= 30")
		return
	}
	if body.Protocol != "" {
		switch body.Protocol {
		case "hls", "webrtc", "rtmp", "dash":
			// valid
		default:
			writeError(w, http.StatusUnprocessableEntity, "INVALID_PROBE",
				"protocol must be one of: hls, webrtc, rtmp, dash")
			return
		}
	}

	// Merge: only update non-zero fields.
	if body.Name != "" {
		existing.Name = body.Name
	}
	if body.URL != "" {
		existing.URL = body.URL
	}
	if body.Protocol != "" {
		existing.Protocol = body.Protocol
	}
	if body.IntervalS >= 30 {
		existing.IntervalS = body.IntervalS
	}
	if body.TimeoutS > 0 {
		existing.TimeoutS = body.TimeoutS
	}
	if body.Enabled != nil {
		existing.Enabled = *body.Enabled
	}

	if err := s.store.UpdateProbe(r.Context(), *existing); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	// Audit the committed change before the re-fetch, so a failed re-read cannot
	// leave a durable mutation unrecorded (uses the merged in-memory row).
	s.audit(r, "probe.update", "probe", probeID, map[string]any{"name": existing.Name})

	updated, err := s.store.GetProbe(r.Context(), probeID)
	if err != nil || updated == nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to fetch updated probe")
		return
	}
	writeJSON(w, http.StatusOK, probeToAPI(*updated))
}

// handleDeleteProbe implements DELETE /api/v1/probes/{probeId}.
// Pro+ required.
func (s *Server) handleDeleteProbe(w http.ResponseWriter, r *http.Request) {
	if err := s.lic.CheckProbes(); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
		return
	}

	probeID := chi.URLParam(r, "probeId")
	existing, err := s.store.GetProbe(r.Context(), probeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "probe not found")
		return
	}

	if err := s.store.DeleteProbe(r.Context(), probeID); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	s.audit(r, "probe.delete", "probe", probeID, nil)
	w.WriteHeader(http.StatusNoContent)
}

// handleProbeResults implements GET /api/v1/probes/{probeId}/results.
// Pro+ required. Reads probe_results from ClickHouse via query service.
func (s *Server) handleProbeResults(w http.ResponseWriter, r *http.Request) {
	if err := s.lic.CheckProbes(); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
		return
	}

	probeID := chi.URLParam(r, "probeId")

	// Verify probe exists.
	existing, err := s.store.GetProbe(r.Context(), probeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "probe not found")
		return
	}

	// Parse time range params.
	q := r.URL.Query()
	from, to := parseTimeRange(q.Get("from"), q.Get("to"))
	if from.IsZero() {
		from = time.Now().Add(-24 * time.Hour) // default: last 24h
	}
	if to.IsZero() {
		to = time.Now()
	}

	limit := 100
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if v, err := strconv.Atoi(lStr); err == nil && v > 0 {
			if v > 1000 {
				v = 1000
			}
			limit = v
		}
	}
	cursor := r.URL.Query().Get("cursor")

	results, err := s.qsvc.QueryProbeResults(r.Context(), probeID, from, to, limit+1, cursor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	var nextCursor *string
	if len(results) > limit {
		results = results[:limit]
		last := results[len(results)-1]
		c := fmt.Sprintf("%d:%s", last.TS.UnixMilli(), last.ID)
		nextCursor = &c
	}
	items := make([]any, 0, len(results))
	for _, res := range results {
		items = append(items, probeResultToAPI(res))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"meta":  map[string]any{"next_cursor": nextCursor},
	})
}

// probeResultToAPI converts a domain.ProbeResult to the API shape.
func probeResultToAPI(r domain.ProbeResult) map[string]any {
	m := map[string]any{
		"id":              r.ID,
		"probe_id":        r.ProbeID,
		"ts":              r.TS.UnixMilli(),
		"success":         r.Success,
		"ttfb_ms":         nil,
		"segment_ttfb_ms": nil,
		"connect_time_ms": nil,
		"signaling_state": nil,
	}
	if r.Success || r.TTFBMs > 0 {
		m["ttfb_ms"] = r.TTFBMs
	}
	if r.Success || r.SegmentTTFBMs > 0 {
		m["segment_ttfb_ms"] = r.SegmentTTFBMs
	}
	if r.ErrorCode != "" {
		m["error_code"] = r.ErrorCode
	}
	if r.ErrorMsg != "" {
		m["error_message"] = r.ErrorMsg
	}
	if r.BitrateKbps > 0 {
		m["bitrate_kbps"] = r.BitrateKbps
	}
	// WebRTC phase-1 signaling fields (non-nil only for WebRTC probes).
	if r.ConnectTimeMs != nil {
		m["connect_time_ms"] = *r.ConnectTimeMs
	}
	if r.SignalingState != "" {
		m["signaling_state"] = r.SignalingState
	}
	// WebRTC phase-2a ICE field: absent for non-WebRTC probes and when ICE was
	// not attempted (empty string).  Present when ICE was attempted and reached
	// a terminal state: "connected" | "failed" | "timeout".
	if r.IceState != "" {
		m["ice_state"] = r.IceState
	}
	// WebRTC phase-2b RTP stats (D-075): key-absent semantics — nil pointer omits
	// the key entirely; a pointer to 0.0 emits value 0.  ICE/stats outcome NEVER
	// flips result.Success (bonus-measurement rule).
	if r.RttMs != nil {
		m["rtt_ms"] = *r.RttMs
	}
	if r.JitterMs != nil {
		m["jitter_ms"] = *r.JitterMs
	}
	if r.LossPct != nil {
		m["loss_pct"] = *r.LossPct
	}
	return m
}
