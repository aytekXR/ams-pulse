// Gap-closure test (E2E-validation doc §5, gap G-13): WriteUsageCSV backs
// GET /api/v1/reports/export. The existing export_test.go asserts status codes
// and the header row; csv_safety_test.go proves formula-neutralization. Neither
// pins the DETAIL-ROW cell VALUES, so a refactor of column order, number
// formatting, or the nil-pointer StreamID/Tenant handling could silently corrupt
// exported billing data. This golden test seeds a known UsageReport and asserts
// every emitted cell.
package reports_test

import (
	"bytes"
	"encoding/csv"
	"reflect"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/reports"
)

func TestWriteUsageCSV_DetailRowGoldenValues(t *testing.T) {
	sid := "stream-1"
	tenant := "acme"
	report := &reports.UsageReport{
		EgressMethod: reports.EgressMethodBitrateXWatchTime,
		Rows: []reports.UsageRow{
			{
				App:             "live",
				StreamID:        &sid,
				Tenant:          &tenant,
				ViewerMinutes:   123.4567,
				PeakConcurrency: 42,
				EgressGB:        1.234567,
				RecordingGB:     0.5,
				EgressMethod:    reports.EgressMethodBitrateXWatchTime,
			},
			{
				// nil StreamID + nil Tenant must render as empty cells, not "<nil>".
				App:             "vod",
				ViewerMinutes:   0,
				PeakConcurrency: 0,
				EgressGB:        0,
				RecordingGB:     2.75,
				EgressMethod:    reports.EgressMethodAMSRestStatsByteCounter,
			},
		},
	}

	var buf bytes.Buffer
	if err := reports.WriteUsageCSV(&buf, report); err != nil {
		t.Fatalf("WriteUsageCSV: %v", err)
	}

	recs, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}
	// Header + 2 detail rows (WriteUsageCSV emits no totals/comment lines — that is
	// generateCSV; keep this distinction pinned).
	if len(recs) != 3 {
		t.Fatalf("got %d records, want 3 (header + 2 rows): %v", len(recs), recs)
	}

	wantHeader := []string{
		"app", "stream_id", "tenant",
		"viewer_minutes", "peak_concurrency",
		"egress_gb", "recording_gb", "egress_method",
	}
	if !reflect.DeepEqual(recs[0], wantHeader) {
		t.Fatalf("header = %v, want %v", recs[0], wantHeader)
	}

	wantRow0 := []string{"live", "stream-1", "acme", "123.4567", "42", "1.234567", "0.500000", "bitrate_x_watch_time"}
	if !reflect.DeepEqual(recs[1], wantRow0) {
		t.Fatalf("row0 = %v, want %v", recs[1], wantRow0)
	}

	wantRow1 := []string{"vod", "", "", "0.0000", "0", "0.000000", "2.750000", "ams_rest_stats_byte_counter"}
	if !reflect.DeepEqual(recs[2], wantRow1) {
		t.Fatalf("row1 (nil stream/tenant) = %v, want %v", recs[2], wantRow1)
	}
}
