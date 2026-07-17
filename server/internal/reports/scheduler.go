// Package reports — scheduled report export (F6, WO-204 item 5).
//
// Scheduler polls report_schedules for due entries, runs the accountant,
// generates the artifact (CSV or PDF), stores it, and optionally uploads to S3.
// Failure → alert_history entry (severity info) + log.
package reports

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// SchedulerConfig holds configuration for the report scheduler.
type SchedulerConfig struct {
	// ArtifactsDir is the base directory where generated reports are written.
	// Default: ./pulse-reports
	ArtifactsDir string

	// RetentionDays prunes artifacts older than this many days on each tick.
	// 0 or negative disables pruning (keep forever). Default 0 here; the server
	// wires PULSE_REPORT_ARTIFACT_RETENTION_DAYS (default 90). Only files matching
	// the pulse-usage-*.{csv,pdf} artifact pattern are ever removed.
	RetentionDays int

	// TickInterval is how often to check for due schedules.
	// Default: 60 s.
	TickInterval time.Duration

	// S3 config (optional; empty = no S3 upload).
	S3 S3Config

	// LogoPath is the filesystem path for the PDF logo override.
	// Corresponds to PULSE_REPORT_LOGO_PATH. Empty = embedded default waveform.
	LogoPath string
}

// Scheduler runs report export jobs on their cron schedules.
type Scheduler struct {
	cfg        SchedulerConfig
	accountant *Accountant
	meta       *meta.Store
	alertStore HistoryWriter  // may be nil
	lic        LicenseChecker // may be nil (no gating — tests, and pre-license callers)
	s3         *S3Uploader    // may be nil
	logger     *slog.Logger
	mu         sync.Mutex
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

// HistoryWriter is the interface for writing alert history entries.
// Defined here to avoid an import cycle with the alert package.
// meta.Store satisfies this interface.
type HistoryWriter interface {
	CreateAlertHistory(ctx context.Context, h meta.AlertHistoryRow) error
}

// LicenseChecker gates scheduled execution by tier. Defined here to avoid an
// import cycle with the license package; *license.Manager satisfies it. The HTTP
// handlers gate schedule CRUD, but a schedule created while licensed keeps firing
// after a downgrade — this is where that is caught on the execution path.
type LicenseChecker interface {
	CheckReports() error
	CheckWhiteLabel() error
}

// NewScheduler creates a Scheduler.
func NewScheduler(cfg SchedulerConfig, accountant *Accountant, ms *meta.Store, logger *slog.Logger) *Scheduler {
	if cfg.ArtifactsDir == "" {
		cfg.ArtifactsDir = "pulse-reports"
	}
	if cfg.TickInterval == 0 {
		cfg.TickInterval = 60 * time.Second
	}
	s := &Scheduler{
		cfg:        cfg,
		accountant: accountant,
		meta:       ms,
		logger:     logger,
		stopCh:     make(chan struct{}),
	}
	if cfg.S3.Endpoint != "" {
		s.s3 = NewS3Uploader(cfg.S3, logger)
	}
	return s
}

// SetAlertStore wires an alert history writer for schedule failure notifications.
func (s *Scheduler) SetAlertStore(w HistoryWriter) {
	s.alertStore = w
}

// SetLicense wires the license checker that gates scheduled execution. Nil (the
// default) disables gating, preserving existing test behaviour.
func (s *Scheduler) SetLicense(l LicenseChecker) {
	s.lic = l
}

// Start launches the scheduler loop in a background goroutine.
func (s *Scheduler) Start(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.cfg.TickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.stopCh:
				return
			case <-ticker.C:
				s.runDue(ctx)
			}
		}
	}()
	s.logger.Info("reports: scheduler started", "interval", s.cfg.TickInterval, "artifacts_dir", s.cfg.ArtifactsDir)
}

// Stop signals the scheduler to stop and waits for it to finish.
func (s *Scheduler) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

// RunOnce fires the scheduler loop once (for testing with fake clock).
func (s *Scheduler) RunOnce(ctx context.Context) {
	s.runDue(ctx)
}

// runDue checks for due schedules and runs them.
func (s *Scheduler) runDue(ctx context.Context) {
	// Prune runs every tick regardless of whether listing/running due schedules
	// succeeds. It is a pure filesystem op with no dependency on the meta store, so
	// gating it behind a ListDueReportSchedules error would let a persistent DB or
	// volume error (e.g. a full volume causing SQLite I/O errors) defeat retention
	// exactly when artifacts are piling up. (D-143 review finding.)
	defer func() { s.pruneArtifacts(time.Now()) }()
	if s.meta == nil {
		return
	}
	due, err := s.meta.ListDueReportSchedules(ctx, time.Now().UnixMilli())
	if err != nil {
		s.logger.Warn("reports: list due schedules failed", "error", err)
		return
	}
	for _, sched := range due {
		if err := s.runSchedule(ctx, sched); err != nil {
			s.logger.Warn("reports: schedule run failed",
				"schedule_id", sched.ID,
				"format", sched.Format,
				"error", err)
			s.writeFailureAlert(ctx, sched.ID, err)
		}
	}
}

// isReportArtifact reports whether name is a scheduler-generated report file
// (the pulse-usage-<from>-to-<to>.{csv,pdf} pattern emitted by GenerateStatement).
// The prune is filename-gated to this shape so it can NEVER remove a non-artifact
// file — in particular the SQLite metastore (pulse_meta.db + its -wal/-shm sidecars)
// and the secret-key file, which post-D-142 share the parent pulse-data volume when
// ArtifactsDir is /var/lib/pulse/reports.
func isReportArtifact(name string) bool {
	return strings.HasPrefix(name, "pulse-usage-") &&
		(strings.HasSuffix(name, ".csv") || strings.HasSuffix(name, ".pdf"))
}

// pruneArtifacts removes report artifacts older than the retention window. It is a
// no-op when RetentionDays <= 0. Strictly bounded: it lists ONLY the top level of
// ArtifactsDir (os.ReadDir does not recurse) and removes ONLY entries that are
// REGULAR files (Type().IsRegular() excludes directories AND symlinks/devices, so a
// pattern-named symlink is never unlinked) matching isReportArtifact and older than
// the cutoff by their own mtime — so non-artifact files sharing the directory are
// never touched.
func (s *Scheduler) pruneArtifacts(now time.Time) {
	if s.cfg.RetentionDays <= 0 {
		return
	}
	entries, err := os.ReadDir(s.cfg.ArtifactsDir)
	if err != nil {
		if !os.IsNotExist(err) {
			s.logger.Warn("reports: prune list failed", "dir", s.cfg.ArtifactsDir, "error", err)
		}
		return
	}
	cutoff := now.Add(-time.Duration(s.cfg.RetentionDays) * 24 * time.Hour)
	pruned := 0
	for _, e := range entries {
		if !e.Type().IsRegular() || !isReportArtifact(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(s.cfg.ArtifactsDir, e.Name())
			if err := os.Remove(path); err != nil {
				s.logger.Warn("reports: prune remove failed", "file", path, "error", err)
				continue
			}
			pruned++
		}
	}
	if pruned > 0 {
		s.logger.Info("reports: pruned old artifacts",
			"count", pruned, "retention_days", s.cfg.RetentionDays, "dir", s.cfg.ArtifactsDir)
	}
}

// runSchedule executes one schedule entry.
func (s *Scheduler) runSchedule(ctx context.Context, sched meta.ReportScheduleRow) error {
	// Reports are Business+. A schedule created while licensed must not keep
	// generating after a downgrade — the HTTP CRUD gate cannot cover the timer.
	if s.lic != nil {
		if err := s.lic.CheckReports(); err != nil {
			s.logger.Warn("reports: skipping scheduled run — tier no longer licensed",
				"schedule_id", sched.ID, "error", err)
			return nil
		}
	}

	sc := parseScheduleScope(sched.ScopeJSON)

	// Determine time range: the previous calendar month, inclusive on both ends.
	from, to := previousCalendarMonthUTC(time.Now())

	params := UsageParams{
		From:     from,
		To:       to,
		Interval: "day",
	}
	if sc.App != nil {
		params.App = *sc.App
	}
	if sc.Tenant != nil {
		params.Tenant = *sc.Tenant
	}

	report, err := s.accountant.ComputeUsage(ctx, params)
	if err != nil {
		return fmt.Errorf("compute usage: %w", err)
	}

	// Determine format.
	format := StatementFormat(sched.Format)
	if format == "" {
		format = FormatCSV
	}

	// Parse white-label header — but only honour it if the tier still licenses
	// white-label branding (Enterprise). A downgraded schedule generates plain.
	var wlHeader *WhitelabelHeader
	if sched.WhitelabelHeader.Valid {
		if s.lic == nil || s.lic.CheckWhiteLabel() == nil {
			wlHeader = ParseWhitelabelHeader(sched.WhitelabelHeader.String)
		}
	}

	opts := StatementOptions{
		From:       from,
		To:         to,
		Format:     format,
		Whitelabel: wlHeader,
		LogoPath:   s.cfg.LogoPath,
	}

	stmt, err := GenerateStatement(report, opts)
	if err != nil {
		return fmt.Errorf("generate statement: %w", err)
	}

	// Store artifact.
	if err := s.storeArtifact(stmt); err != nil {
		return fmt.Errorf("store artifact: %w", err)
	}

	// Optional S3 upload.
	if s.s3 != nil {
		key := s.cfg.S3.Prefix + stmt.Filename
		if err := s.s3.Upload(ctx, key, stmt.ContentType, stmt.Data); err != nil {
			// Log S3 failure but don't fail the whole schedule run.
			s.logger.Warn("reports: S3 upload failed", "key", key, "error", err)
			s.writeFailureAlert(ctx, sched.ID, fmt.Errorf("S3 upload: %w", err))
		}
	}

	// Mark as ran and compute next run.
	nextRun := nextCronTime(sched.Cron, time.Now())
	nextRunMS := nextRun.UnixMilli()
	lastRunMS := time.Now().UnixMilli()
	_ = s.meta.MarkScheduleRan(ctx, sched.ID, lastRunMS, nextRunMS)

	s.logger.Info("reports: schedule ran",
		"schedule_id", sched.ID,
		"rows", stmt.RowCount,
		"format", format,
		"file", stmt.Filename)

	return nil
}

// previousCalendarMonthUTC returns the [from, to] range covering the calendar
// month immediately before now, in UTC. Both bounds are day-granular and
// INCLUSIVE: the daily-rollup queries use `bucket >= from AND bucket <= to`
// against a Date column, so `to` must be the LAST day of the previous month.
// Using the first day of the CURRENT month here (the historical bug) let that
// day's rows satisfy `bucket <= to` and bleed into the previous month's
// statement, over-counting viewer-minutes/egress/peak and mislabelling the
// period end in the filename and header.
func previousCalendarMonthUTC(now time.Time) (from, to time.Time) {
	nowUTC := now.UTC()
	firstOfThisMonth := time.Date(nowUTC.Year(), nowUTC.Month(), 1, 0, 0, 0, 0, time.UTC)
	from = firstOfThisMonth.AddDate(0, -1, 0) // first day of the previous month
	to = firstOfThisMonth.AddDate(0, 0, -1)   // last day of the previous month
	return from, to
}

// storeArtifact writes the statement to the artifacts directory.
func (s *Scheduler) storeArtifact(stmt *GeneratedStatement) error {
	if err := os.MkdirAll(s.cfg.ArtifactsDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", s.cfg.ArtifactsDir, err)
	}
	path := filepath.Join(s.cfg.ArtifactsDir, stmt.Filename)
	if err := os.WriteFile(path, stmt.Data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// writeFailureAlert creates an alert_history entry (severity info) for schedule failures.
func (s *Scheduler) writeFailureAlert(ctx context.Context, scheduleID string, err error) {
	if s.alertStore == nil {
		return
	}
	h := meta.AlertHistoryRow{
		RuleID:    "report_scheduler",
		AlertID:   "report_failure_" + scheduleID,
		State:     "firing",
		Severity:  "info",
		TS:        time.Now().UnixMilli(),
		Metric:    "report_schedule_failure",
		Value:     1,
		Threshold: 0,
		ScopeJSON: fmt.Sprintf(`{"schedule_id":%q}`, scheduleID),
	}
	if werr := s.alertStore.CreateAlertHistory(ctx, h); werr != nil {
		s.logger.Warn("reports: write failure alert failed", "error", werr)
	}
}

// NextCronTime is the exported alias for nextCronTime; used by the API layer
// to pre-compute the initial next_run_at when creating a schedule.
func NextCronTime(cronExpr string, from time.Time) time.Time {
	return nextCronTime(cronExpr, from)
}

// nextCronTime computes the next fire time for a cron expression.
// Honors min/hour/day-of-month/weekday (D-107); the month field is ignored.
// If cron parsing fails, defaults to a 1-month interval.
func nextCronTime(cronExpr string, from time.Time) time.Time {
	// The whole reporting pipeline is UTC (period math, ClickHouse storage, and
	// next_run_at is persisted as an absolute UnixMilli). Interpret the cron
	// fields in UTC regardless of the server's local timezone — otherwise a
	// schedule like "0 6 1 * *" created on a non-UTC host fires at 06:00 LOCAL
	// (e.g. 11:00 UTC on America/New_York) instead of 06:00 UTC, drifting from
	// the UTC period boundary. Callers may pass time.Now() (local); normalise here
	// so every call site is correct and this is provable via the exported wrapper.
	from = from.UTC()
	min, hour, dom, weekday, err := parseCronFieldsInternal(cronExpr)
	if err != nil {
		// Unknown format — schedule next month.
		return from.AddDate(0, 1, 0)
	}

	// Find the next minute matching min/hour/dom/weekday from `from`. The window
	// is ~1 year so a day-of-month that skips short months (e.g. dom=31) is still
	// found; a monthly schedule exits within ~31 days.
	t := from.Add(time.Minute)       // start from next minute
	for i := 0; i < 60*24*366; i++ { // search up to ~1 year ahead
		if (min < 0 || t.Minute() == min) &&
			(hour < 0 || t.Hour() == hour) &&
			cronDayMatches(t, dom, weekday) {
			return t.Truncate(time.Minute)
		}
		t = t.Add(time.Minute)
	}
	return from.AddDate(0, 1, 0) // fallback
}

// cronDayMatches implements standard cron day-of-month / day-of-week semantics:
// when BOTH fields are restricted the day matches if EITHER matches (Vixie cron
// OR-semantics); otherwise each restricted field must match and a wildcard (-1)
// always matches.
func cronDayMatches(t time.Time, dom, weekday int) bool {
	domSet := dom >= 0
	wdaySet := weekday >= 0
	if domSet && wdaySet {
		return t.Day() == dom || int(t.Weekday()) == weekday
	}
	return (dom < 0 || t.Day() == dom) && (weekday < 0 || int(t.Weekday()) == weekday)
}
