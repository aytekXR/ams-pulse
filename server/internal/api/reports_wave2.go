// Wave-2 report and tenant handlers (WO-204).
// Real implementations of /reports/usage, /reports/schedules CRUD,
// and /admin/tenants CRUD that replace the wave-1 stubs in server.go.
package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/pulse-analytics/pulse/server/internal/reports"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// ─── Reports (F6) ────────────────────────────────────────────────────────────

// handleReportUsage serves GET /api/v1/reports/usage.
// Business-tier gated; returns usage rows with egress_method field per row.
func (s *Server) handleReportUsage(w http.ResponseWriter, r *http.Request) {
	// VD-35: gate — reports require Business tier or higher.
	if err := s.lic.CheckReports(); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
		return
	}
	q := r.URL.Query()
	from, to := parseTimeRange(q.Get("from"), q.Get("to"))
	interval := q.Get("interval")
	if interval == "" {
		interval = "day"
	}

	params := reports.UsageParams{
		From:     from,
		To:       to,
		App:      q.Get("app"),
		StreamID: q.Get("stream"),
		Tenant:   q.Get("tenant"),
		Interval: interval,
	}

	if s.reportGen != nil && s.reportGen.Accountant != nil {
		report, err := s.reportGen.Accountant.ComputeUsage(r.Context(), params)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, report)
		return
	}

	// No ClickHouse configured — return empty valid response.
	writeJSON(w, http.StatusOK, map[string]any{
		"rows":          []any{},
		"totals":        map[string]any{"viewer_minutes": 0, "peak_concurrency": 0, "egress_gb": 0, "recording_gb": 0},
		"egress_method": reports.EgressMethodBitrateXWatchTime,
	})
}

// ─── Report Schedules ─────────────────────────────────────────────────────────

// handleListReportSchedules serves GET /api/v1/reports/schedules.
// Business-tier gated (reports = Business per PRD §7.11).
func (s *Server) handleListReportSchedules(w http.ResponseWriter, r *http.Request) {
	// VD-35: gate — reports require Business tier or higher.
	if err := s.lic.CheckReports(); err != nil {
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
	schedules, err := s.store.ListReportSchedules(r.Context(), limit+1, cursor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	var nextCursor *string
	if len(schedules) > limit {
		schedules = schedules[:limit]
		last := schedules[len(schedules)-1]
		c := fmt.Sprintf("%d:%s", last.CreatedAt, last.ID)
		nextCursor = &c
	}
	items := make([]any, 0, len(schedules))
	for _, sched := range schedules {
		items = append(items, reportScheduleToAPI(sched))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "meta": map[string]any{"next_cursor": nextCursor}})
}

// handleCreateReportSchedule serves POST /api/v1/reports/schedules.
// Business tier gated (reports = Business per PRD §7.11).
func (s *Server) handleCreateReportSchedule(w http.ResponseWriter, r *http.Request) {
	// VD-35: gate — reports require Business tier or higher.
	if err := s.lic.CheckReports(); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
		return
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}
	row, err := reportScheduleFromAPI(body)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_SCHEDULE", err.Error())
		return
	}
	// White-label headers are an Enterprise entitlement; reports themselves are
	// only Business, so CheckReports above is not sufficient to gate branding.
	if row.WhitelabelHeader.Valid {
		if err := s.lic.CheckWhiteLabel(); err != nil {
			writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
			return
		}
	}

	now := time.Now()
	nextRun := reports.NextCronTime(row.Cron, now)
	nextRunMS := nextRun.UnixMilli()
	row.NextRunAt = &nextRunMS

	created, err := s.store.CreateReportSchedule(r.Context(), row)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	s.audit(r, "report_schedule.create", "report_schedule", created.ID, map[string]any{"cron": created.Cron})
	writeJSON(w, http.StatusCreated, reportScheduleToAPI(created))
}

// handleUpdateReportSchedule serves PUT /api/v1/reports/schedules/{scheduleId}.
// Business tier gated (reports = Business per PRD §7.11).
func (s *Server) handleUpdateReportSchedule(w http.ResponseWriter, r *http.Request) {
	// VD-35: gate — reports require Business tier or higher.
	if err := s.lic.CheckReports(); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
		return
	}
	id := chi.URLParam(r, "scheduleId")
	existing, err := s.store.GetReportSchedule(r.Context(), id)
	if err != nil {
		// A transient store error (DB down, ctx deadline) must not masquerade as a
		// definitive 404 — clients that treat 404 as "deleted" would drop a live row.
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load schedule")
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "schedule not found")
		return
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}
	row, err := reportScheduleFromAPI(body)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_SCHEDULE", err.Error())
		return
	}
	// White-label headers are Enterprise-only (see create handler).
	if row.WhitelabelHeader.Valid {
		if err := s.lic.CheckWhiteLabel(); err != nil {
			writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
			return
		}
	}
	row.ID = id
	row.CreatedAt = existing.CreatedAt
	// reportScheduleFromAPI parses only the request body, so NextRunAt/LastRunAt
	// come back nil. Writing those as-is NULLs next_run_at, and
	// ListDueReportSchedules filters on `next_run_at IS NOT NULL` — so an edited
	// schedule would never fire again. Preserve the run history and recompute the
	// next fire from the (possibly changed) cron, exactly as the create handler does.
	row.LastRunAt = existing.LastRunAt
	nextRun := reports.NextCronTime(row.Cron, time.Now())
	nextRunMS := nextRun.UnixMilli()
	row.NextRunAt = &nextRunMS
	if err := s.store.UpdateReportSchedule(r.Context(), row); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	s.audit(r, "report_schedule.update", "report_schedule", id, map[string]any{"cron": row.Cron})
	// row already holds every field reportScheduleToAPI emits (id/cron/format/scope/
	// created_at/tenant_mapping/whitelabel_header/last_run_at/next_run_at — all set
	// above before the write; the response carries no updated_at). Render it directly
	// instead of re-fetching: the old re-fetch swallowed its error and nil-dereferenced
	// if a concurrent DELETE landed between the commit and the read (S62 [6]).
	writeJSON(w, http.StatusOK, reportScheduleToAPI(row))
}

// handleDeleteReportSchedule serves DELETE /api/v1/reports/schedules/{scheduleId}.
// Business tier gated (reports = Business per PRD §7.11).
func (s *Server) handleDeleteReportSchedule(w http.ResponseWriter, r *http.Request) {
	// VD-35: gate — reports require Business tier or higher.
	if err := s.lic.CheckReports(); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
		return
	}
	id := chi.URLParam(r, "scheduleId")
	if existing, _ := s.store.GetReportSchedule(r.Context(), id); existing == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "schedule not found")
		return
	}
	if err := s.store.DeleteReportSchedule(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	s.audit(r, "report_schedule.delete", "report_schedule", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

// ─── Tenants (F6) ─────────────────────────────────────────────────────────────

// handleListTenants serves GET /api/v1/admin/tenants.
// Business-tier gated (D-010, §7.11): multi-tenant billing requires Enterprise tier.
func (s *Server) handleListTenants(w http.ResponseWriter, r *http.Request) {
	if err := s.lic.CheckMultiTenant(); err != nil {
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
	tenants, err := s.store.ListTenants(r.Context(), limit+1, cursor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	var nextCursor *string
	if len(tenants) > limit {
		tenants = tenants[:limit]
		last := tenants[len(tenants)-1]
		c := fmt.Sprintf("%d:%s", last.CreatedAt, last.ID)
		nextCursor = &c
	}
	items := make([]any, 0, len(tenants))
	for _, t := range tenants {
		items = append(items, tenantToAPI(t))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "meta": map[string]any{"next_cursor": nextCursor}})
}

// handleCreateTenant serves POST /api/v1/admin/tenants.
// Business-tier gated. Returns 409 on duplicate name; 422 if no matcher field set.
func (s *Server) handleCreateTenant(w http.ResponseWriter, r *http.Request) {
	if err := s.lic.CheckMultiTenant(); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
		return
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}
	row, err := tenantFromAPI(body)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_TENANT", err.Error())
		return
	}
	// Check for duplicate name (unique constraint in DB).
	if existing, _ := s.store.GetTenantByName(r.Context(), row.Name); existing != nil {
		writeError(w, http.StatusConflict, "DUPLICATE_NAME", "tenant name already exists")
		return
	}
	created, err := s.store.CreateTenant(r.Context(), row)
	if err != nil {
		if isUniqueConstraintError(err) {
			writeError(w, http.StatusConflict, "DUPLICATE_NAME", "tenant name already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	s.audit(r, "tenant.create", "tenant", created.ID, map[string]any{"name": created.Name})
	writeJSON(w, http.StatusCreated, tenantToAPI(created))
}

// handleGetTenant serves GET /api/v1/admin/tenants/{tenantId}.
// Business-tier gated. Returns 404 if tenant not found.
func (s *Server) handleGetTenant(w http.ResponseWriter, r *http.Request) {
	if err := s.lic.CheckMultiTenant(); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
		return
	}
	id := chi.URLParam(r, "tenantId")
	t, err := s.store.GetTenant(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load tenant")
		return
	}
	if t == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "tenant not found")
		return
	}
	writeJSON(w, http.StatusOK, tenantToAPI(*t))
}

// handleUpdateTenant serves PUT /api/v1/admin/tenants/{tenantId}.
// Business-tier gated. Returns 404 if not found; 409 if new name conflicts; 422 if no matcher.
func (s *Server) handleUpdateTenant(w http.ResponseWriter, r *http.Request) {
	if err := s.lic.CheckMultiTenant(); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
		return
	}
	id := chi.URLParam(r, "tenantId")
	existing, err := s.store.GetTenant(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to load tenant")
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "tenant not found")
		return
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body")
		return
	}
	row, err := tenantFromAPI(body)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_TENANT", err.Error())
		return
	}
	// Check for duplicate name if name is being changed.
	if row.Name != existing.Name {
		if dup, _ := s.store.GetTenantByName(r.Context(), row.Name); dup != nil {
			writeError(w, http.StatusConflict, "DUPLICATE_NAME", "tenant name already exists")
			return
		}
	}
	row.ID = id
	row.CreatedAt = existing.CreatedAt
	if err := s.store.UpdateTenant(r.Context(), row); err != nil {
		if isUniqueConstraintError(err) {
			writeError(w, http.StatusConflict, "DUPLICATE_NAME", "tenant name already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	s.audit(r, "tenant.update", "tenant", id, map[string]any{"name": row.Name})
	// Re-fetch to return the DB-authoritative row: updated_at is stamped inside
	// UpdateTenant and not reflected in row, and tenantToAPI emits it. Guard the read
	// so a concurrent DELETE or transient store error can't nil-deref (S62 [5];
	// mirrors handleUpdateProbe).
	updated, err := s.store.GetTenant(r.Context(), id)
	if err != nil || updated == nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to fetch updated tenant")
		return
	}
	writeJSON(w, http.StatusOK, tenantToAPI(*updated))
}

// handleDeleteTenant serves DELETE /api/v1/admin/tenants/{tenantId}.
// Business-tier gated. Returns 404 if not found.
func (s *Server) handleDeleteTenant(w http.ResponseWriter, r *http.Request) {
	if err := s.lic.CheckMultiTenant(); err != nil {
		writeError(w, http.StatusForbidden, "LICENSE_REQUIRED", err.Error())
		return
	}
	id := chi.URLParam(r, "tenantId")
	if existing, _ := s.store.GetTenant(r.Context(), id); existing == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "tenant not found")
		return
	}
	if err := s.store.DeleteTenant(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	s.audit(r, "tenant.delete", "tenant", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

// isUniqueConstraintError returns true if the error is a SQLite UNIQUE constraint violation.
func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") || strings.Contains(msg, "unique constraint")
}

// ─── Conversion helpers ───────────────────────────────────────────────────────

func reportScheduleToAPI(r meta.ReportScheduleRow) map[string]any {
	m := map[string]any{
		"id":         r.ID,
		"cron":       r.Cron,
		"format":     r.Format,
		"scope":      jsonOrEmpty(r.ScopeJSON),
		"created_at": r.CreatedAt,
	}
	if r.TenantMapping.Valid {
		m["tenant_mapping"] = r.TenantMapping.String
	} else {
		m["tenant_mapping"] = nil
	}
	if r.WhitelabelHeader.Valid {
		m["whitelabel_header"] = jsonOrEmpty(r.WhitelabelHeader.String)
	} else {
		m["whitelabel_header"] = nil
	}
	if r.LastRunAt != nil {
		m["last_run_at"] = *r.LastRunAt
	}
	if r.NextRunAt != nil {
		m["next_run_at"] = *r.NextRunAt
	}
	return m
}

func reportScheduleFromAPI(body map[string]any) (meta.ReportScheduleRow, error) {
	cronExpr, _ := body["cron"].(string)
	format, _ := body["format"].(string)
	if cronExpr == "" {
		return meta.ReportScheduleRow{}, fmt.Errorf("cron required")
	}
	if format == "" {
		format = "csv"
	}
	if format != "csv" && format != "pdf" {
		return meta.ReportScheduleRow{}, fmt.Errorf("format must be csv or pdf")
	}

	scopeJSON := "{}"
	if scope, ok := body["scope"]; ok && scope != nil {
		if b, err := json.Marshal(scope); err == nil {
			scopeJSON = string(b)
		}
	}

	row := meta.ReportScheduleRow{
		Cron:      cronExpr,
		Format:    format,
		ScopeJSON: scopeJSON,
	}
	if v, ok := body["tenant_mapping"].(string); ok && v != "" {
		row.TenantMapping = sql.NullString{String: v, Valid: true}
	}
	if v, ok := body["whitelabel_header"]; ok && v != nil {
		if b, err := json.Marshal(v); err == nil {
			row.WhitelabelHeader = sql.NullString{String: string(b), Valid: true}
		}
	}
	return row, nil
}

func tenantToAPI(t meta.TenantRow) map[string]any {
	return map[string]any{
		"id":             t.ID,
		"name":           t.Name,
		"stream_pattern": t.StreamPattern,
		"meta_tag_key":   t.MetaTagKey,
		"meta_tag_value": t.MetaTagValue,
		"created_at":     t.CreatedAt,
		"updated_at":     t.UpdatedAt,
	}
}

func tenantFromAPI(body map[string]any) (meta.TenantRow, error) {
	name, _ := body["name"].(string)
	if name == "" {
		return meta.TenantRow{}, fmt.Errorf("name required")
	}
	streamPattern := stringOrEmpty(body["stream_pattern"])
	metaTagKey := stringOrEmpty(body["meta_tag_key"])
	metaTagValue := stringOrEmpty(body["meta_tag_value"])
	// Require at least one matcher: stream_pattern OR (meta_tag_key + meta_tag_value).
	if streamPattern == "" && (metaTagKey == "" || metaTagValue == "") {
		return meta.TenantRow{}, fmt.Errorf("at least one matcher required: stream_pattern or (meta_tag_key + meta_tag_value)")
	}
	return meta.TenantRow{
		Name:          name,
		StreamPattern: streamPattern,
		MetaTagKey:    metaTagKey,
		MetaTagValue:  metaTagValue,
	}, nil
}

func stringOrEmpty(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
