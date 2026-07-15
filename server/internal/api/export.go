// Export handler: GET /api/v1/reports/export
// Business-tier gated; streams real CSV from the same ComputeUsage query
// that backs GET /api/v1/reports/usage. The route is registered under
// downloadAuthMiddleware (not bearerAuthMiddleware) so that browser-initiated
// file downloads via window.location.href can authenticate with ?token=.
// Normal API routes keep their A4 security property (?token= rejected there).
package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/reports"
)

// handleReportExport serves GET /api/v1/reports/export.
// Supports format=csv (default). format=pdf returns 501 — PDF generation is a
// Phase 3 feature (PRD §7 Phase 3: "white-label PDF"); see docs/known-limitations.md.
//
// Query params (all optional except format):
//
//	from, to  — epoch-ms or RFC 3339; default to last 7 days (parseTimeRange)
//	format    — "csv" (only supported value today)
//	app       — filter by AMS application name
//	tenant    — filter by tenant label
//	stream    — filter by stream_id
func (s *Server) handleReportExport(w http.ResponseWriter, r *http.Request) {
	if err := s.lic.CheckReports(); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
		return
	}

	q := r.URL.Query()
	format := q.Get("format")
	if format == "" {
		format = "csv"
	}

	if format != "csv" {
		writeError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED",
			"PDF export is not yet implemented (Phase 3 feature); use format=csv")
		return
	}

	from, to := parseTimeRange(q.Get("from"), q.Get("to"))

	params := reports.UsageParams{
		From:     from,
		To:       to,
		App:      q.Get("app"),
		StreamID: q.Get("stream"),
		Tenant:   q.Get("tenant"),
		Interval: "day",
	}

	var report *reports.UsageReport
	if s.reportGen != nil && s.reportGen.Accountant != nil {
		var err error
		report, err = s.reportGen.Accountant.ComputeUsage(r.Context(), params)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
	} else {
		report = &reports.UsageReport{
			Rows:         []reports.UsageRow{},
			Totals:       reports.UsageTotals{},
			EgressMethod: reports.EgressMethodBitrateXWatchTime,
		}
	}

	filename := fmt.Sprintf("usage-report-%s.csv", time.Now().UTC().Format("2006-01-02"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

	// Row cells are formula-neutralized (CSV injection) by reports.WriteUsageCSV.
	// Headers are already committed, so a write error here can only be logged.
	_ = reports.WriteUsageCSV(w, report)
}
