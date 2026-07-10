package reports_test

// S11 WO-A: White-label PDF report logo — TDD test file (RED-first).
// Tests written before logo.go / image embedding exist.

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pulse-analytics/pulse/server/internal/reports"
)

// newTestLoggerDiscard returns a slog.Logger that discards all output.
func newTestLoggerDiscard(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// makeMinimalPNG encodes a w×h solid-colour PNG using stdlib image/png.
func makeMinimalPNG(t *testing.T, w, h int, r, g, b uint8) []byte {
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
		t.Fatalf("makeMinimalPNG: png.Encode: %v", err)
	}
	return buf.Bytes()
}

// ─── resolveLogo unit tests ───────────────────────────────────────────────────

// TestResolveLogo_NoPath_ReturnsDefault: empty path returns the embedded default
// PNG bytes (non-nil, starts with PNG magic \x89PNG).
func TestResolveLogo_NoPath_ReturnsDefault(t *testing.T) {
	got := reports.ResolveLogo("")
	if got == nil {
		t.Fatal("ResolveLogo(\"\") returned nil; expected embedded default bytes")
	}
	if len(got) < 4 || got[0] != 0x89 || got[1] != 'P' || got[2] != 'N' || got[3] != 'G' {
		t.Errorf("ResolveLogo(\"\") does not start with PNG magic bytes; first bytes: %v", got[:clamp(len(got), 4)])
	}
	t.Logf("PASS: default logo len=%d bytes", len(got))
}

// TestResolveLogo_ValidPath_ReturnsFileBytes: a valid path returns exactly the
// file's raw bytes, not the embedded default.
func TestResolveLogo_ValidPath_ReturnsFileBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.png")
	// Synthetic PNG magic + dummy bytes (not a real PNG — just for identity test).
	synthetic := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0xDE, 0xAD, 0xBE, 0xEF}
	if err := os.WriteFile(path, synthetic, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got := reports.ResolveLogo(path)
	if !bytes.Equal(got, synthetic) {
		t.Errorf("ResolveLogo returned %v; expected file bytes %v", got, synthetic)
	}
	t.Logf("PASS: file bytes returned, len=%d", len(got))
}

// TestResolveLogo_MissingFile_FallsBackToDefault: non-existent path returns the
// embedded default — no panic, no error propagation.
func TestResolveLogo_MissingFile_FallsBackToDefault(t *testing.T) {
	got := reports.ResolveLogo("/nonexistent/logo-xxxxxx.png")
	if got == nil {
		t.Fatal("ResolveLogo(missing) returned nil; expected embedded default fallback")
	}
	def := reports.ResolveLogo("")
	if !bytes.Equal(got, def) {
		t.Error("ResolveLogo(missing) did not return defaultLogoBytes")
	}
	t.Logf("PASS: missing path fell back to default, len=%d", len(got))
}

// ─── PDF generation with logo ─────────────────────────────────────────────────

func synthReportSmall(t *testing.T) *reports.UsageReport {
	t.Helper()
	sessions, _, _ := reports.SyntheticMonth(10, 1500)
	return reports.ComputeUsageFromSessions(sessions, nil)
}

// TestGeneratePDF_DefaultLogo_ContainsXObject: PDF with no LogoPath must embed
// the default logo as a PDF image XObject.
func TestGeneratePDF_DefaultLogo_ContainsXObject(t *testing.T) {
	report := synthReportSmall(t)
	now := time.Now()
	stmt, err := reports.GenerateStatement(report, reports.StatementOptions{
		From:   now.AddDate(0, -1, 0),
		To:     now,
		Format: reports.FormatPDF,
	})
	if err != nil {
		t.Fatalf("GenerateStatement: %v", err)
	}
	if !strings.HasPrefix(string(stmt.Data), "%PDF-") {
		t.Errorf("output does not start with %%PDF-")
	}
	if !strings.Contains(string(stmt.Data), "/Subtype /Image") {
		t.Error("PDF does not contain /Subtype /Image — logo XObject missing")
	}
	t.Logf("PASS: default-logo PDF len=%d, contains /Subtype /Image", len(stmt.Data))
}

// TestGeneratePDF_CustomLogoPath_UsesOverrideBytes: LogoPath pointing to a valid
// PNG produces a PDF with /Subtype /Image, and differs from the default-logo PDF.
func TestGeneratePDF_CustomLogoPath_UsesOverrideBytes(t *testing.T) {
	dir := t.TempDir()
	customPath := filepath.Join(dir, "custom_logo.png")
	// 2×2 red PNG — minimal but valid and different from the default waveform PNG.
	if err := os.WriteFile(customPath, makeMinimalPNG(t, 2, 2, 255, 0, 0), 0o644); err != nil {
		t.Fatalf("WriteFile custom PNG: %v", err)
	}

	report := synthReportSmall(t)
	now := time.Now()

	stmtDef, err := reports.GenerateStatement(report, reports.StatementOptions{
		From: now.AddDate(0, -1, 0), To: now, Format: reports.FormatPDF,
	})
	if err != nil {
		t.Fatalf("GenerateStatement default: %v", err)
	}

	stmtCustom, err := reports.GenerateStatement(report, reports.StatementOptions{
		From: now.AddDate(0, -1, 0), To: now, Format: reports.FormatPDF,
		LogoPath: customPath,
	})
	if err != nil {
		t.Fatalf("GenerateStatement custom: %v", err)
	}

	if !strings.Contains(string(stmtCustom.Data), "/Subtype /Image") {
		t.Error("custom-logo PDF does not contain /Subtype /Image")
	}
	if bytes.Equal(stmtDef.Data, stmtCustom.Data) {
		t.Error("custom-logo PDF identical to default — override not applied")
	}
	t.Logf("PASS: default=%d bytes, custom=%d bytes — differ OK", len(stmtDef.Data), len(stmtCustom.Data))
}

// TestGeneratePDF_UnreadableLogoPath_NoErrorAndContainsDefaultLogo: bad LogoPath
// must fall back to the embedded default without returning an error.
func TestGeneratePDF_UnreadableLogoPath_NoErrorAndContainsDefaultLogo(t *testing.T) {
	report := synthReportSmall(t)
	now := time.Now()
	stmt, err := reports.GenerateStatement(report, reports.StatementOptions{
		From:     now.AddDate(0, -1, 0),
		To:       now,
		Format:   reports.FormatPDF,
		LogoPath: "/nonexistent/logo-xxxxxx.png",
	})
	if err != nil {
		t.Fatalf("GenerateStatement with bad path returned error: %v", err)
	}
	if stmt == nil || len(stmt.Data) == 0 {
		t.Fatal("GenerateStatement returned nil/empty")
	}
	if !strings.HasPrefix(string(stmt.Data), "%PDF-") {
		t.Error("output does not start with %PDF-")
	}
	if !strings.Contains(string(stmt.Data), "/Subtype /Image") {
		t.Error("fallback PDF does not contain /Subtype /Image")
	}
	t.Logf("PASS: bad path → default logo fallback, len=%d", len(stmt.Data))
}

// ─── Boot-time validation helper ─────────────────────────────────────────────

// TestValidateLogoPath_UnreadablePath_WarnsAndReturnsDefault: ValidateLogoPath
// logs a WARN when the file is missing and returns the embedded default bytes.
func TestValidateLogoPath_UnreadablePath_WarnsAndReturnsDefault(t *testing.T) {
	var logBuf strings.Builder
	handler := slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn})
	logger := slog.New(handler)

	got := reports.ValidateLogoPath("/nonexistent/logo-xxxxxx.png", logger)
	if got == nil {
		t.Fatal("ValidateLogoPath returned nil")
	}
	def := reports.ResolveLogo("")
	if !bytes.Equal(got, def) {
		t.Error("ValidateLogoPath did not return defaultLogoBytes on missing path")
	}
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "PULSE_REPORT_LOGO_PATH") && !strings.Contains(logOutput, "not readable") {
		t.Errorf("expected WARN log with 'PULSE_REPORT_LOGO_PATH' or 'not readable'; got: %q", logOutput)
	}
	t.Logf("PASS: warn emitted, default returned. log=%q", logOutput)
}

// TestValidateLogoPath_GarbageContent_WarnsAndFallsBack: a READABLE file that is
// neither PNG nor JPEG must WARN at boot and resolve to the embedded default,
// and PDF generation with that path must not error (render() falls back too).
func TestValidateLogoPath_GarbageContent_WarnsAndFallsBack(t *testing.T) {
	garbagePath := filepath.Join(t.TempDir(), "garbage_logo.png")
	if err := os.WriteFile(garbagePath, []byte{0x00, 0x01, 'n', 'o', 't', 'p', 'n', 'g'}, 0o644); err != nil {
		t.Fatalf("WriteFile garbage: %v", err)
	}

	var logBuf strings.Builder
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	got := reports.ValidateLogoPath(garbagePath, logger)
	if !bytes.Equal(got, reports.ResolveLogo("")) {
		t.Error("ValidateLogoPath did not fall back to defaultLogoBytes on garbage content")
	}
	if !strings.Contains(logBuf.String(), "not a PNG or JPEG") {
		t.Errorf("expected WARN containing 'not a PNG or JPEG'; got: %q", logBuf.String())
	}

	// Render path: garbage at a readable path must not error and must still
	// embed an image (default-logo fallback inside render()).
	report := synthReportSmall(t)
	now := time.Now()
	stmt, err := reports.GenerateStatement(report, reports.StatementOptions{
		From:     now.AddDate(0, -1, 0),
		To:       now,
		Format:   reports.FormatPDF,
		LogoPath: garbagePath,
	})
	if err != nil {
		t.Fatalf("GenerateStatement with garbage logo returned error: %v", err)
	}
	if !strings.Contains(string(stmt.Data), "/Subtype /Image") {
		t.Error("garbage-logo PDF does not contain /Subtype /Image (default fallback missing)")
	}
	t.Logf("PASS: garbage content → WARN + default fallback, PDF len=%d", len(stmt.Data))
}

// ─── Asset validity pin ───────────────────────────────────────────────────────

// TestDefaultLogoAsset_ValidPNG pins that the committed default_logo.png asset
// is a valid PNG that stdlib image/png can decode (guards against a corrupt asset).
func TestDefaultLogoAsset_ValidPNG(t *testing.T) {
	data := reports.ResolveLogo("")
	if len(data) == 0 {
		t.Fatal("defaultLogoBytes is empty")
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("image/png.DecodeConfig(defaultLogoBytes) failed: %v", err)
	}
	t.Logf("PASS: default logo PNG valid (%d bytes, %dx%d)", len(data), cfg.Width, cfg.Height)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// clamp returns min(n, max).
func clamp(n, max int) int {
	if n > max {
		return max
	}
	return n
}

// Silence unused-import check for newTestLoggerDiscard (used implicitly by Go).
var _ = newTestLoggerDiscard
