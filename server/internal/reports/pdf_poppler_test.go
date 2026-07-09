package reports_test

// pdf_poppler_test.go — generates PDFs to PDF_OUT_DIR for external poppler validation.
// Run with: go test -v -run TestGeneratePDFsForPoppler -count=1 with PDF_OUT_DIR set.

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/reports"
)

// TestGeneratePDFsForPoppler generates default.pdf and custom.pdf into
// the directory specified by the PDF_OUT_DIR environment variable.
// If PDF_OUT_DIR is unset, the test is skipped.
func TestGeneratePDFsForPoppler(t *testing.T) {
	outDir := os.Getenv("PDF_OUT_DIR")
	if outDir == "" {
		t.Skip("PDF_OUT_DIR not set; skipping poppler-target PDF generation")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", outDir, err)
	}

	sessions, _, _ := reports.SyntheticMonth(5, 1500)
	report := reports.ComputeUsageFromSessions(sessions, nil)
	now := time.Now()
	opts := reports.StatementOptions{
		From:   now.AddDate(0, -1, 0),
		To:     now,
		Format: reports.FormatPDF,
	}

	// 1. Default logo PDF.
	stmt, err := reports.GenerateStatement(report, opts)
	if err != nil {
		t.Fatalf("GenerateStatement default: %v", err)
	}
	defaultPath := filepath.Join(outDir, "default.pdf")
	if err := os.WriteFile(defaultPath, stmt.Data, 0o644); err != nil {
		t.Fatalf("WriteFile default.pdf: %v", err)
	}
	t.Logf("wrote %s (%d bytes)", defaultPath, len(stmt.Data))

	// 2. Custom logo PDF (2×2 blue PNG).
	customPNG := makeSolidPNG(t, 2, 2, 0, 100, 200)
	customLogoPath := filepath.Join(outDir, "custom_logo.png")
	if err := os.WriteFile(customLogoPath, customPNG, 0o644); err != nil {
		t.Fatalf("WriteFile custom_logo.png: %v", err)
	}
	opts2 := opts
	opts2.LogoPath = customLogoPath
	stmt2, err := reports.GenerateStatement(report, opts2)
	if err != nil {
		t.Fatalf("GenerateStatement custom: %v", err)
	}
	customPDFPath := filepath.Join(outDir, "custom.pdf")
	if err := os.WriteFile(customPDFPath, stmt2.Data, 0o644); err != nil {
		t.Fatalf("WriteFile custom.pdf: %v", err)
	}
	t.Logf("wrote %s (%d bytes)", customPDFPath, len(stmt2.Data))
}

// makeSolidPNG creates a w×h solid-colour PNG using stdlib image/png.
func makeSolidPNG(t *testing.T, w, h int, r, g, b uint8) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	c := color.NRGBA{R: r, G: g, B: b, A: 255}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("makeSolidPNG: png.Encode: %v", err)
	}
	return buf.Bytes()
}

// newSolidPNG is a plain image.Image helper used by makeMinimalPNG in logo_test.go.
func newSolidPNG(w, h int, r, g, b uint8) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	c := color.NRGBA{R: r, G: g, B: b, A: 255}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}
