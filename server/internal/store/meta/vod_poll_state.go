package meta

// vod_poll_state.go — seen-set methods for BUG-002 VoD REST poll dedup.
//
// The vod_poll_state table is a simple (app, vod_id) seen-set: once a VoD is
// marked seen it is never emitted again as a recording_ready event, preventing
// SummingMergeTree double-counts across Pulse restarts.
//
// OQ-1 resolution: AMS 3.0.3 vods/list exposes a stable vodId field, making
// a seen-set safer than a HWM-by-creationDate approach.

import "context"

// ListSeenVodIDs returns the set of VoD IDs already marked as seen for the
// given app. Returns an empty (non-nil) map and no error if the app has no
// entries yet.
func (s *Store) ListSeenVodIDs(ctx context.Context, app string) (map[string]struct{}, error) {
	rows, err := s.queryContext(ctx,
		`SELECT vod_id FROM vod_poll_state WHERE app = ?`, app)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := make(map[string]struct{})
	for rows.Next() {
		var vodID string
		if err := rows.Scan(&vodID); err != nil {
			return nil, err
		}
		seen[vodID] = struct{}{}
	}
	return seen, rows.Err()
}

// MarkVodSeen records (app, vodID) as seen in the vod_poll_state table.
// The call is idempotent: if the row already exists, the INSERT is silently
// ignored (ON CONFLICT DO NOTHING works on SQLite 3.24+ and Postgres).
// createdMS is the VoD creation timestamp in Unix epoch milliseconds (stored
// for diagnostic purposes; not used for deduplication).
func (s *Store) MarkVodSeen(ctx context.Context, app, vodID string, createdMS int64) error {
	_, err := s.execContext(ctx,
		`INSERT INTO vod_poll_state (app, vod_id, created_ms) VALUES (?, ?, ?)
		 ON CONFLICT(app, vod_id) DO NOTHING`,
		app, vodID, createdMS)
	return err
}
