package meta

// probe.go — probes table CRUD + ProbeConfigSource implementation (F10, WO-302).
//
// ProbeConfig is stored in the meta store (probes table). The runner reads
// ListEnabled() to discover active probes and writes back last_* fields via
// RecordResult() after each check.
//
// Implements domain.ProbeConfigSource over this table.

import (
	"context"
	"database/sql"

	"github.com/pulse-analytics/pulse/server/internal/domain"
)

// ─── ProbeRow ─────────────────────────────────────────────────────────────────

// ProbeRow mirrors the probes table.
type ProbeRow struct {
	ID           string
	Name         string
	URL          string
	Protocol     string
	IntervalS    int
	TimeoutS     int
	Enabled      bool
	LastResultID sql.NullString
	LastSuccess  sql.NullInt64
	LastRunAt    sql.NullInt64
	CreatedAt    int64
	UpdatedAt    int64
}

// ─── CRUD ─────────────────────────────────────────────────────────────────────

// CreateProbe inserts a new probe row.
func (s *Store) CreateProbe(ctx context.Context, p ProbeRow) (ProbeRow, error) {
	if p.ID == "" {
		p.ID = newUUID()
	}
	now := nowMS()
	p.CreatedAt = now
	p.UpdatedAt = now
	if p.TimeoutS == 0 {
		p.TimeoutS = 10
	}
	if p.IntervalS == 0 {
		p.IntervalS = 60
	}
	_, err := s.execContext(ctx,
		`INSERT INTO probes (id, name, url, protocol, interval_s, timeout_s, enabled, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		p.ID, p.Name, p.URL, p.Protocol, p.IntervalS, p.TimeoutS,
		boolToInt(p.Enabled), p.CreatedAt, p.UpdatedAt)
	return p, err
}

// GetProbe fetches a probe by ID.
func (s *Store) GetProbe(ctx context.Context, id string) (*ProbeRow, error) {
	row := s.queryRowContext(ctx,
		`SELECT id, name, url, protocol, interval_s, timeout_s, enabled,
		        last_result_id, last_success, last_run_at, created_at, updated_at
		 FROM probes WHERE id=?`, id)
	return scanProbe(row)
}

// ListProbes returns all probe rows ordered by created_at.
func (s *Store) ListProbes(ctx context.Context) ([]ProbeRow, error) {
	rows, err := s.queryContext(ctx,
		`SELECT id, name, url, protocol, interval_s, timeout_s, enabled,
		        last_result_id, last_success, last_run_at, created_at, updated_at
		 FROM probes ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var probes []ProbeRow
	for rows.Next() {
		p, err := scanProbe(rows)
		if err != nil {
			return nil, err
		}
		probes = append(probes, *p)
	}
	return probes, rows.Err()
}

// UpdateProbe updates a probe by ID.
func (s *Store) UpdateProbe(ctx context.Context, p ProbeRow) error {
	p.UpdatedAt = nowMS()
	_, err := s.execContext(ctx,
		`UPDATE probes SET name=?, url=?, protocol=?, interval_s=?, timeout_s=?, enabled=?, updated_at=?
		 WHERE id=?`,
		p.Name, p.URL, p.Protocol, p.IntervalS, p.TimeoutS, boolToInt(p.Enabled), p.UpdatedAt, p.ID)
	return err
}

// DeleteProbe removes a probe by ID.
func (s *Store) DeleteProbe(ctx context.Context, id string) error {
	_, err := s.execContext(ctx, `DELETE FROM probes WHERE id=?`, id)
	return err
}

func scanProbe(row scanner) (*ProbeRow, error) {
	var p ProbeRow
	var enabled int
	if err := row.Scan(
		&p.ID, &p.Name, &p.URL, &p.Protocol, &p.IntervalS, &p.TimeoutS, &enabled,
		&p.LastResultID, &p.LastSuccess, &p.LastRunAt, &p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	p.Enabled = enabled != 0
	return &p, nil
}

// ─── ProbeConfigSource implementation ────────────────────────────────────────

// MetaProbeConfigSource implements domain.ProbeConfigSource over the meta store
// probes table. It is the production seam between the probe runner (BE-01) and
// the meta store (BE-02).
type MetaProbeConfigSource struct {
	store *Store
}

// NewProbeConfigSource creates a MetaProbeConfigSource.
func NewProbeConfigSource(store *Store) *MetaProbeConfigSource {
	return &MetaProbeConfigSource{store: store}
}

// ListEnabled returns all probes where enabled = 1.
// Implements domain.ProbeConfigSource.
func (s *MetaProbeConfigSource) ListEnabled(ctx context.Context) ([]domain.ProbeConfig, error) {
	rows, err := s.store.queryContext(ctx,
		`SELECT id, name, url, protocol, interval_s, timeout_s
		 FROM probes WHERE enabled = 1 ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var configs []domain.ProbeConfig
	for rows.Next() {
		var p domain.ProbeConfig
		if err := rows.Scan(&p.ID, &p.Name, &p.URL, &p.Protocol, &p.IntervalS, &p.TimeoutS); err != nil {
			return nil, err
		}
		p.Enabled = true
		configs = append(configs, p)
	}
	return configs, rows.Err()
}

// RecordResult updates the denormalized last_result_id, last_success, and
// last_run_at fields after a probe check completes.
// Implements domain.ProbeConfigSource.
func (s *MetaProbeConfigSource) RecordResult(ctx context.Context, r domain.ProbeResult) error {
	var lastSuccess int
	if r.Success {
		lastSuccess = 1
	}
	lastRunAtMS := r.TS.UnixMilli()
	_, err := s.store.execContext(ctx,
		`UPDATE probes SET last_result_id=?, last_success=?, last_run_at=?, updated_at=?
		 WHERE id=?`,
		r.ID, lastSuccess, lastRunAtMS, nowMS(), r.ProbeID)
	return err
}
