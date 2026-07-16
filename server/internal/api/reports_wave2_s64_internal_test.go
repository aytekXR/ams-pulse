// S64 (D-126) — the post-mutation existence-check cluster in reports_wave2.go
// must distinguish a *transient store error* (→500 INTERNAL_ERROR) from a
// *genuinely missing row* (→404 NOT_FOUND). Previously all three handlers
// collapsed both into `if err != nil || row == nil { 404 }`, so a DB-down error
// was reported to clients as a definitive 404 (S62 [19]).
//
// These are in-package (package api) tests so they can call the handlers
// directly with a pre-canceled request context. database/sql returns ctx.Err()
// when acquiring a connection, before touching the driver, so
// GetTenant/GetReportSchedule return (nil, ctx.Err()) — a deterministic
// transient-error path. It is NOT reachable through the full HTTP stack: the
// bearer-auth middleware queries the same store, so breaking the store fails
// auth first. Hence the direct call.
//
// Mutation proof: revert any of the three err/nil splits back to
// `if err != nil || row == nil { 404 }` and the corresponding subtest goes RED
// (it receives 404, wants 500). The genuine-missing-row → 404 positive control
// lives in the external harness (reports_wave2_s64_test.go), which migrates a
// real store so GetTenant returns (nil, nil) via ErrNoRows.
package api

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/pulse-analytics/pulse/server/internal/license"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

// enterpriseLicenseForTest builds a signed enterprise license and points
// PULSE_LICENSE_PUBKEY at its verifying key (restored on cleanup). It mirrors the
// external makeTestEnterpriseLicense, replicated here because that helper lives
// in package api_test and is invisible to these in-package tests.
func enterpriseLicenseForTest(t *testing.T) *license.Manager {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	claims, _ := json.Marshal(map[string]any{
		"tier": "enterprise", "max_nodes": nil, "retention_days": nil,
		"data_api": true, "white_label": true,
	})
	key := base64.StdEncoding.EncodeToString(claims) + "." +
		base64.StdEncoding.EncodeToString(ed25519.Sign(priv, claims))
	orig := os.Getenv("PULSE_LICENSE_PUBKEY")
	os.Setenv("PULSE_LICENSE_PUBKEY", hex.EncodeToString(pub))
	t.Cleanup(func() { os.Setenv("PULSE_LICENSE_PUBKEY", orig) })
	mgr, err := license.New(key, "")
	if err != nil {
		t.Fatalf("license.New: %v", err)
	}
	if mgr.Tier() != license.TierEnterprise {
		t.Fatalf("want enterprise tier, got %q", mgr.Tier())
	}
	return mgr
}

// canceledReqWithParam builds a request whose context is already canceled and
// carries the given chi URL param, so chi.URLParam resolves while any store call
// on r.Context() fails deterministically with ctx.Err().
func canceledReqWithParam(method, key, val string) *http.Request {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, val)
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	req := httptest.NewRequest(method, "http://pulse.test/", strings.NewReader("{}"))
	return req.WithContext(ctx)
}

func TestReportsWave2_TransientStoreError_Returns500Not404(t *testing.T) {
	lic := enterpriseLicenseForTest(t)
	ms, err := meta.New(context.Background(), "sqlite", ":memory:", "s64-refetch-test-secret")
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	t.Cleanup(func() { ms.Close() })
	s := &Server{store: ms, lic: lic}

	cases := []struct {
		name    string
		method  string
		key     string
		val     string
		handler http.HandlerFunc
	}{
		{"handleGetTenant", http.MethodGet, "tenantId", "tenant-x", s.handleGetTenant},
		{"handleUpdateTenant", http.MethodPut, "tenantId", "tenant-x", s.handleUpdateTenant},
		{"handleUpdateReportSchedule", http.MethodPut, "scheduleId", "sched-x", s.handleUpdateReportSchedule},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tc.handler(rec, canceledReqWithParam(tc.method, tc.key, tc.val))
			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("transient store error (canceled ctx) must map to 500, got %d — body=%s",
					rec.Code, rec.Body.String())
			}
			// A 500 with an INTERNAL_ERROR code, never a NOT_FOUND masquerade.
			if body := rec.Body.String(); !strings.Contains(body, "INTERNAL_ERROR") {
				t.Fatalf("want INTERNAL_ERROR code in body, got %s", body)
			}
		})
	}
}
