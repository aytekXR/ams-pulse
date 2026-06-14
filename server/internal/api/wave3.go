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
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/pulse-analytics/pulse/server/internal/domain"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// ─── F9 Anomaly detection ────────────────────────────────────────────────────

// handleAnomalies implements GET /api/v1/anomalies.
// Enterprise tier required per §7.11.
func (s *Server) handleAnomalies(w http.ResponseWriter, r *http.Request) {
	// Tier gate: anomaly detection is Enterprise only.
	if err := s.lic.CheckAnomalies(); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
		return
	}

	// Parse min_sigma query param (OpenAPI default 2.0, but we use detector's default if absent).
	var sigmaThreshold float64
	if minSigmaStr := r.URL.Query().Get("min_sigma"); minSigmaStr != "" {
		v, err := strconv.ParseFloat(minSigmaStr, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_PARAM", "min_sigma must be a number")
			return
		}
		sigmaThreshold = v
	}

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

	// Apply optional metric filter.
	metricFilter := r.URL.Query().Get("metric")
	items := make([]any, 0, len(flags))
	for _, f := range flags {
		if metricFilter != "" && f.Metric != metricFilter {
			continue
		}
		items = append(items, f)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"meta":  map[string]any{"next_cursor": nil},
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

	probes, err := s.store.ListProbes(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]any, 0, len(probes))
	for _, p := range probes {
		items = append(items, probeToAPI(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"meta":  map[string]any{"next_cursor": nil},
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

	results, err := s.qsvc.QueryProbeResults(r.Context(), probeID, from, to, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	items := make([]any, 0, len(results))
	for _, res := range results {
		items = append(items, probeResultToAPI(res))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"meta":  map[string]any{"next_cursor": nil},
	})
}

// probeResultToAPI converts a domain.ProbeResult to the API shape.
func probeResultToAPI(r domain.ProbeResult) map[string]any {
	m := map[string]any{
		"id":       r.ID,
		"probe_id": r.ProbeID,
		"ts":       r.TS.UnixMilli(),
		"success":  r.Success,
		"ttfb_ms":  nil,
	}
	if r.Success || r.TTFBMs > 0 {
		m["ttfb_ms"] = r.TTFBMs
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
	return m
}

