package reports_test

// pdf_layout_test.go — every line of a statement PDF must land ON the page.
//
// S126 found that scheduled PDF statements showed only their first line: the renderer
// placed each line with "50 <y> Td", but Td moves RELATIVE to the previous line start,
// so line 2 sat at (100, ~1488), above the 792-pt page, and every later line climbed
// further. The existing tests only checked that the strings were somewhere in the file,
// and the poppler check is skipped in CI (no poppler), so nothing noticed for months.
//
// This test needs no poppler: it replays the content stream's text-positioning
// operators (Tm, Td, T*, Tj) and asserts the position of every string drawn.

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aytekXR/ams-pulse/server/internal/reports"
)

type placedText struct {
	x, y float64
	text string
}

// textPlacements replays the text operators between the first BT and its ET.
func textPlacements(t *testing.T, pdf []byte) []placedText {
	t.Helper()
	start := bytes.Index(pdf, []byte("BT\n"))
	end := bytes.Index(pdf[start:], []byte("ET\n"))
	if start < 0 || end < 0 {
		t.Fatalf("no BT/ET text object in the PDF")
	}
	tjRe := regexp.MustCompile(`^\((.*)\) Tj$`)
	// Text and line matrices start as identity at BT (only e, f matter here: a=d=1, b=c=0).
	var lineE, lineF float64
	var out []placedText
	for _, raw := range strings.Split(string(pdf[start:start+end]), "\n") {
		line := strings.TrimSpace(raw)
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		switch f[len(f)-1] {
		case "Tm": // a b c d e f Tm — absolute
			if len(f) != 7 {
				t.Fatalf("malformed Tm: %q", line)
			}
			lineE, _ = strconv.ParseFloat(f[4], 64)
			lineF, _ = strconv.ParseFloat(f[5], 64)
		case "Td": // tx ty Td — RELATIVE to the current line start
			tx, _ := strconv.ParseFloat(f[0], 64)
			ty, _ := strconv.ParseFloat(f[1], 64)
			lineE += tx
			lineF += ty
		case "T*": // 0 -TL Td; the renderer never sets TL, so TL = 0
		case "Tj":
			m := tjRe.FindStringSubmatch(line)
			if m == nil {
				t.Fatalf("unparsed Tj: %q", line)
			}
			out = append(out, placedText{x: lineE, y: lineF, text: m[1]})
		}
	}
	return out
}

// statementPDF renders a statement with `rows` stream rows (SyntheticMonth spreads its
// sessions over only 10 streams, so the report is built directly).
func statementPDF(t *testing.T, rows int, wl *reports.WhitelabelHeader) []byte {
	t.Helper()
	report := &reports.UsageReport{EgressMethod: "bitrate_x_watch_time"}
	for i := 0; i < rows; i++ {
		id := fmt.Sprintf("stream-%03d", i)
		report.Rows = append(report.Rows, reports.UsageRow{
			App: "live", StreamID: &id, ViewerMinutes: float64(100 + i), PeakConcurrency: int64(i%40 + 1),
			EgressGB: float64(i) / 10, EgressMethod: "bitrate_x_watch_time",
		})
	}
	now := time.Now()
	stmt, err := reports.GenerateStatement(report, reports.StatementOptions{
		From: now.AddDate(0, -1, 0), To: now, Format: reports.FormatPDF, Whitelabel: wl,
	})
	if err != nil {
		t.Fatalf("GenerateStatement: %v", err)
	}
	return stmt.Data
}

func TestStatementPDF_EveryLineIsOnThePage(t *testing.T) {
	for _, tc := range []struct {
		name string
		rows int
		wl   *reports.WhitelabelHeader
	}{
		{"default header", 5, nil},
		{"white-label header", 5, &reports.WhitelabelHeader{Name: "Acme Streaming", Address: "1 Main St\nSpringfield"}},
		{"more rows than one page holds", 200, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			placed := textPlacements(t, statementPDF(t, tc.rows, tc.wl))
			if len(placed) < 4 {
				t.Fatalf("only %d strings drawn: %+v", len(placed), placed)
			}
			const pageW, pageH, margin = 612.0, 792.0, 50.0
			prevY := pageH
			for i, p := range placed {
				if p.x < 0 || p.x > pageW || p.y < margin || p.y > pageH {
					t.Errorf("string %d %q drawn at (%.1f, %.1f) — off the %vx%v page (bottom margin %v)",
						i, p.text, p.x, p.y, pageW, pageH, margin)
				}
				if p.y >= prevY {
					t.Errorf("string %d %q at y=%.1f is not below the previous line (y=%.1f)", i, p.text, p.y, prevY)
				}
				prevY = p.y
			}
			if tc.wl != nil {
				// Name, then each address line as its own line (Tj draws no line breaks).
				want := []string{tc.wl.Name, "1 Main St", "Springfield"}
				for i, w := range want {
					if i >= len(placed) || placed[i].text != w {
						t.Fatalf("white-label header line %d = %q, want %q (all: %+v)", i, placed[i].text, w, placed[:3])
					}
				}
			}
		})
	}
}

func TestStatementPDF_TruncationIsAnnounced(t *testing.T) {
	placed := textPlacements(t, statementPDF(t, 200, nil))
	last := placed[len(placed)-1].text
	if !strings.Contains(last, "more rows not shown") || !strings.Contains(last, "CSV") {
		t.Errorf("a statement with more rows than fit must say so on its last line; got %q", last)
	}
	short := textPlacements(t, statementPDF(t, 5, nil))
	for _, p := range short {
		if strings.Contains(p.text, "more rows not shown") {
			t.Errorf("a statement that fits must not carry the truncation note: %q", p.text)
		}
	}
}
