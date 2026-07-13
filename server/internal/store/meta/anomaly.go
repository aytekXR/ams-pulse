package meta

// anomaly.go — anomaly_baselines table CRUD for F9 anomaly detection (WO-302).
// The anomaly_baselines table lives in the meta store because baselines are
// low-cardinality, mutated in-place, and config-like (see ARCHITECTURE §3.3).

import (
	"context"
	"database/sql"
	"strings"

	"github.com/pulse-analytics/pulse/server/internal/anomaly"
)

// ListAnomalyBaselines returns all rows from the anomaly_baselines table.
// Implements anomaly.BaselineStore.
func (s *Store) ListAnomalyBaselines(ctx context.Context) ([]anomaly.AnomalyBaselineRow, error) {
	rows, err := s.queryContext(ctx,
		`SELECT id, metric, scope, window_s, mean, stddev, sample_count, last_updated
		 FROM anomaly_baselines ORDER BY metric, scope, window_s`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []anomaly.AnomalyBaselineRow
	for rows.Next() {
		var r anomaly.AnomalyBaselineRow
		if err := rows.Scan(&r.ID, &r.Metric, &r.Scope, &r.WindowS,
			&r.Mean, &r.Stddev, &r.SampleCount, &r.LastUpdated); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

// UpsertAnomalyBaseline inserts or updates a baseline row keyed by the
// unique index (metric, scope, window_s). Implements anomaly.BaselineStore.
func (s *Store) UpsertAnomalyBaseline(ctx context.Context, row anomaly.AnomalyBaselineRow) error {
	if row.ID == "" {
		row.ID = newUUID()
	}
	_, err := s.execContext(ctx,
		`INSERT INTO anomaly_baselines (id, metric, scope, window_s, mean, stddev, sample_count, last_updated)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(metric, scope, window_s) DO UPDATE SET
		   mean=excluded.mean,
		   stddev=excluded.stddev,
		   sample_count=excluded.sample_count,
		   last_updated=excluded.last_updated`,
		row.ID, row.Metric, row.Scope, row.WindowS,
		row.Mean, row.Stddev, row.SampleCount, row.LastUpdated)
	return err
}

// GetAnomalyBaseline fetches a single baseline by (metric, scope, window_s).
// Returns nil, nil when not found.
func (s *Store) GetAnomalyBaseline(ctx context.Context, metric, scope string, windowS int) (*anomaly.AnomalyBaselineRow, error) {
	row := s.queryRowContext(ctx,
		`SELECT id, metric, scope, window_s, mean, stddev, sample_count, last_updated
		 FROM anomaly_baselines WHERE metric=? AND scope=? AND window_s=?`,
		metric, scope, windowS)
	var r anomaly.AnomalyBaselineRow
	if err := row.Scan(&r.ID, &r.Metric, &r.Scope, &r.WindowS,
		&r.Mean, &r.Stddev, &r.SampleCount, &r.LastUpdated); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

// DeleteAnomalyBaseline removes a baseline by ID.
func (s *Store) DeleteAnomalyBaseline(ctx context.Context, id string) error {
	_, err := s.execContext(ctx, `DELETE FROM anomaly_baselines WHERE id=?`, id)
	return err
}

// DeleteZeroMeanNodeBaselines deletes baseline rows where metric is one of the
// given metrics and both mean=0 AND stddev=0. This removes baselines poisoned by
// standalone AMS nodes that never report cpu_pct/mem_pct/disk_pct — they
// accumulated hundreds or thousands of zero observations before D-088 presence
// flags were deployed. The sample_count clause is intentionally absent because
// poisoned rows commonly have n in the hundreds/thousands (live: realams n=733,
// prod n=8813). Implements anomaly.BaselineSweeper; called once by Detector.Run
// on startup after WarmHysteresis. Works on SQLite and Postgres via rebind.
func (s *Store) DeleteZeroMeanNodeBaselines(ctx context.Context, metrics []string) (int64, error) {
	if len(metrics) == 0 {
		return 0, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(metrics)), ",")
	args := make([]any, len(metrics))
	for i, m := range metrics {
		args[i] = m
	}
	res, err := s.execContext(ctx,
		`DELETE FROM anomaly_baselines WHERE metric IN (`+placeholders+`) AND mean=0 AND stddev=0`,
		args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
