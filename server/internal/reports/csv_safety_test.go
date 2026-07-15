// S44 (D-106) — CSV formula-injection neutralization tests.
//
// The usage export (GET /api/v1/reports/export) and the white-label statement
// generator write publisher-controlled columns (app, stream_id, tenant — an AMS
// application/stream name is chosen by whoever publishes) into CSV. A cell that
// begins with = + - @, a tab, or a carriage return is evaluated as a live formula
// by Excel/Google Sheets/LibreOffice on open, and docs/known-limitations.md tells
// operators to open the export in a spreadsheet. These tests pin that every text
// column is neutralized (single-quote prefix) while numeric columns are untouched.
//
// Mutation proof: delete the CSVSafeCell calls inside UsageCSVRecord and
// TestWriteUsageCSV_FormulaInjectionNeutralized + TestGenerateStatement_CSV_
// FormulaInjectionNeutralized both go RED.
package reports_test

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/reports"
)

func TestCSVSafeCell(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"=1+1", "'=1+1"},
		{"=HYPERLINK(\"http://x\",\"a\")", "'=HYPERLINK(\"http://x\",\"a\")"},
		{"+SUM(A1)", "'+SUM(A1)"},
		{"-2+3", "'-2+3"},
		{"@evil", "'@evil"},
		{"\tinjected", "'\tinjected"},
		{"\rinjected", "'\rinjected"},
		// Safe inputs — returned unchanged.
		{"LiveApp", "LiveApp"},
		{"stream-42", "stream-42"}, // interior '-' is fine; only leading triggers
		{"tenant_a", "tenant_a"},
		{"123", "123"},
		{"", ""},
	}
	for _, c := range cases {
		if got := reports.CSVSafeCell(c.in); got != c.want {
			t.Errorf("CSVSafeCell(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestUsageCSVRecord_NeutralizesTextColumnsOnly(t *testing.T) {
	sp := func(s string) *string { return &s }
	rec := reports.UsageCSVRecord(reports.UsageRow{
		App:             "=cmd|'/c calc'!A0",
		StreamID:        sp("+SUM(1)"),
		Tenant:          sp("@x"),
		ViewerMinutes:   12.5,
		PeakConcurrency: 3,
		EgressGB:        1.5,
		RecordingGB:     0.25,
		EgressMethod:    reports.EgressMethodBitrateXWatchTime,
	})
	// Columns: app, stream_id, tenant, viewer_minutes, peak_concurrency, egress_gb, recording_gb, egress_method
	if len(rec) != 8 {
		t.Fatalf("expected 8 cells, got %d: %v", len(rec), rec)
	}
	for i, idx := range []int{0, 1, 2} {
		if !strings.HasPrefix(rec[idx], "'") {
			t.Errorf("text column %d = %q, expected single-quote neutralization", i, rec[idx])
		}
	}
	// Numeric columns must be untouched (no injected quote).
	for _, idx := range []int{3, 4, 5, 6} {
		if strings.HasPrefix(rec[idx], "'") {
			t.Errorf("numeric column %d = %q should not be quoted", idx, rec[idx])
		}
	}
}

func TestWriteUsageCSV_FormulaInjectionNeutralized(t *testing.T) {
	sp := func(s string) *string { return &s }
	report := &reports.UsageReport{
		Rows: []reports.UsageRow{{
			App:             "=HYPERLINK(\"http://attacker\",\"pwn\")",
			StreamID:        sp("+cmd"),
			Tenant:          sp("@t"),
			ViewerMinutes:   1,
			PeakConcurrency: 1,
			EgressGB:        1,
			RecordingGB:     1,
			EgressMethod:    reports.EgressMethodBitrateXWatchTime,
		}},
		EgressMethod: reports.EgressMethodBitrateXWatchTime,
	}
	var buf bytes.Buffer
	if err := reports.WriteUsageCSV(&buf, report); err != nil {
		t.Fatalf("WriteUsageCSV: %v", err)
	}
	records, err := csv.NewReader(bytes.NewReader(buf.Bytes())).ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}
	if len(records) != 2 { // header + 1 detail row
		t.Fatalf("expected header + 1 row, got %d records", len(records))
	}
	if records[0][0] != "app" {
		t.Errorf("header[0] = %q, want app", records[0][0])
	}
	row := records[1]
	for _, idx := range []int{0, 1, 2} {
		if !strings.HasPrefix(row[idx], "'") {
			t.Errorf("detail cell %d = %q not neutralized (formula would execute on open)", idx, row[idx])
		}
	}
}

func TestGenerateStatement_CSV_FormulaInjectionNeutralized(t *testing.T) {
	sp := func(s string) *string { return &s }
	report := &reports.UsageReport{
		Rows: []reports.UsageRow{{
			App:             "=1+2",
			StreamID:        sp("-9"),
			Tenant:          sp("@tenant"),
			ViewerMinutes:   5,
			PeakConcurrency: 2,
			EgressGB:        2,
			RecordingGB:     1,
			EgressMethod:    reports.EgressMethodBitrateXWatchTime,
		}},
		EgressMethod: reports.EgressMethodBitrateXWatchTime,
	}
	stmt, err := reports.GenerateStatement(report, reports.StatementOptions{
		From:   time.Now().AddDate(0, -1, 0),
		To:     time.Now(),
		Format: reports.FormatCSV,
	})
	if err != nil {
		t.Fatalf("GenerateStatement CSV: %v", err)
	}
	body := string(stmt.Data)
	// The raw bytes must not contain a cell that begins a line with a formula
	// trigger; every detail text cell is prefixed with a single quote.
	if strings.Contains(body, "\n=1+2") || strings.Contains(body, ",=1+2") {
		t.Errorf("statement CSV contains un-neutralized formula cell; body:\n%s", body)
	}
	if !strings.Contains(body, "'=1+2") {
		t.Errorf("expected neutralized app cell '=1+2 in statement CSV; body:\n%s", body)
	}
	if !strings.Contains(body, "'@tenant") {
		t.Errorf("expected neutralized tenant cell '@tenant in statement CSV; body:\n%s", body)
	}
}
