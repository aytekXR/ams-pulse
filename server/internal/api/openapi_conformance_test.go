// Package api_test — RESPONSE-BODY ↔ OpenAPI conformance test.
//
// TestConformance_OpenAPIResponseSchema starts the real API httptest server,
// hits a representative set of GET endpoints, and validates each response body
// against contracts/openapi/pulse-api.yaml using kin-openapi (v0.140.0).
//
// Non-vacuity contract (three hard checks):
//
//	(a) Spec-load guard: the test fatally fails if the spec cannot be loaded,
//	    fails doc.Validate, or has 0 paths — an empty/truncated spec would
//	    silently accept any response without these guards.
//
//	(b) Coverage guard: the test fails if fewer than 3 endpoints were
//	    found-in-spec-AND-ValidateResponse'd. This catches a misconfigured
//	    FindRoute that silently skips all checks.
//
//	(c) Body-schema guard: ValidateResponse is called with the actual body bytes
//	    (SetBodyBytes) and no ExcludeResponseBody flag. If a handler omits a
//	    required field or returns a wrong type, ValidateResponse returns an error
//	    and the test records a failure. For example, removing the `items` field
//	    from a list response would be caught immediately.
package api_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/gorillamux"
)

// TestConformance_OpenAPIResponseSchema validates that the live API server
// responses conform to the contracts/openapi/pulse-api.yaml schema.
//
// Endpoints covered (all CH-free — served from in-memory state or meta store):
//
//	GET /api/v1/live/overview   → LiveOverview schema
//	GET /api/v1/live/streams    → LiveStreamList schema
//	GET /api/v1/fleet/nodes     → FleetNodeList schema
//	GET /api/v1/alerts/rules    → AlertRuleList schema
//	GET /api/v1/admin/tokens    → TokenList schema
//	GET /api/v1/admin/license   → LicenseInfo schema
//
// The test uses a Business-tier license (data_api=true) so that any tier-gated
// GET endpoints return 200 rather than 403.
func TestConformance_OpenAPIResponseSchema(t *testing.T) {
	// ── 1. Locate and load the OpenAPI spec ───────────────────────────────────
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("conformance: runtime.Caller failed — cannot locate spec path")
	}
	// thisFile is server/internal/api/openapi_conformance_test.go
	// spec is at <repo-root>/contracts/openapi/pulse-api.yaml
	// api/ → internal/ → server/ → <repo-root>
	specPath := filepath.Clean(filepath.Join(
		filepath.Dir(thisFile), "..", "..", "..", "contracts", "openapi", "pulse-api.yaml",
	))

	// t.Fatal (not t.Skip): a skipped test shows as PASS on most CI dashboards, so a
	// missing spec would silently turn this whole conformance gate green. The spec is
	// committed at contracts/openapi/pulse-api.yaml and the repo root is mounted in CI.
	if _, statErr := os.Stat(specPath); os.IsNotExist(statErr) {
		t.Fatalf("conformance: spec not found at %s — the conformance gate cannot run without the OpenAPI spec", specPath)
	}

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(specPath)
	if err != nil {
		t.Fatalf("conformance: load spec %s: %v", specPath, err)
	}
	if err := doc.Validate(loader.Context); err != nil {
		t.Fatalf("conformance: spec %s is invalid: %v", specPath, err)
	}

	// ── Guard (a): spec must have at least one path ───────────────────────────
	pathCount := len(doc.Paths.Map())
	if pathCount == 0 {
		t.Fatalf("conformance: spec loaded but has 0 paths — spec appears empty or truncated; file=%s", specPath)
	}
	t.Logf("conformance: spec loaded — %d paths, file=%s", pathCount, specPath)

	// ── 2. Build the kin-openapi route finder ─────────────────────────────────
	router, err := gorillamux.NewRouter(doc)
	if err != nil {
		t.Fatalf("conformance: gorillamux.NewRouter: %v", err)
	}

	// ── 3. Start test server (Business-tier license: data_api=true) ───────────
	// setupBusinessServer is defined in v3b_guard_test.go (same package).
	ts, adminToken, cleanup := setupBusinessServer(t)
	defer cleanup()

	// ── 4. Probe table: GET endpoints that work without ClickHouse ────────────
	type probe struct {
		method string
		path   string // full URL path (/api/v1/...)
		desc   string // schema name expected
	}
	probes := []probe{
		{"GET", "/api/v1/live/overview", "LiveOverview"},
		{"GET", "/api/v1/live/streams", "LiveStreamList"},
		{"GET", "/api/v1/fleet/nodes", "FleetNodeList"},
		{"GET", "/api/v1/alerts/rules", "AlertRuleList"},
		{"GET", "/api/v1/admin/tokens", "TokenList"},
		{"GET", "/api/v1/admin/license", "LicenseInfo"},
	}

	// ── 5. Hit each endpoint, then validate response against spec schema ───────
	validated := 0
	for _, p := range probes {
		p := p // capture loop variable

		// 5a. Real HTTP request to the httptest server.
		realReq, _ := http.NewRequest(p.method, ts.URL+p.path, nil)
		realReq.Header.Set("Authorization", "Bearer "+adminToken)

		resp, httpErr := http.DefaultClient.Do(realReq)
		if httpErr != nil {
			t.Errorf("conformance: %s %s: request failed: %v", p.method, p.path, httpErr)
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			t.Errorf("conformance: %s %s: read body: %v", p.method, p.path, readErr)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("conformance: %s %s: expected 200, got %d — body: %s",
				p.method, p.path, resp.StatusCode, body)
			continue
		}

		// 5b. FindRoute: match the URL path against the spec.
		//
		// kin-openapi's gorillamux router resolves the path against the spec's
		// server base URL ("/api/v1"), so we supply the full path here.
		specReq, _ := http.NewRequest(p.method, p.path, nil)
		specReq.Header.Set("Authorization", "Bearer "+adminToken)

		route, pathParams, findErr := router.FindRoute(specReq)
		if findErr != nil {
			t.Errorf("conformance: FindRoute(%s %s): %v — route may be absent from spec; check that the path exists under /api/v1 in pulse-api.yaml",
				p.method, p.path, findErr)
			continue
		}

		// 5c. ValidateResponse — the schema guard.
		//
		// SetBodyBytes supplies the actual body; ExcludeResponseBody is NOT set,
		// so kin-openapi validates the JSON against the schema declared in the
		// spec's 200 response. A missing required field or a wrong-typed value
		// will cause ValidateResponse to return a non-nil error.
		//
		// ExcludeRequestBody: true is set only to skip validating the (empty)
		// GET request body, which is irrelevant here.
		resp.Header.Set("Content-Type", "application/json") // ensure MIME matches spec
		input := &openapi3filter.ResponseValidationInput{
			RequestValidationInput: &openapi3filter.RequestValidationInput{
				Request:     specReq,
				PathParams:  pathParams,
				Route:       route,
				QueryParams: specReq.URL.Query(),
			},
			Status: resp.StatusCode,
			Header: resp.Header,
			Options: &openapi3filter.Options{
				AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
				ExcludeRequestBody: true,
				// ExcludeResponseBody intentionally absent — we WANT body validation.
			},
		}
		input.SetBodyBytes(body)
		// Restore Body so the validator can re-read it if needed.
		resp.Body = io.NopCloser(bytes.NewReader(body))

		if valErr := openapi3filter.ValidateResponse(context.Background(), input); valErr != nil {
			t.Errorf("conformance FAIL: %s %s (%s) — response body does not conform to spec schema: %v\nbody: %s",
				p.method, p.path, p.desc, valErr, body)
			continue
		}

		validated++
		t.Logf("conformance PASS: %s %s (%s) → 200, body conforms to spec", p.method, p.path, p.desc)
	}

	// ── Guard (b): at least 3 endpoints must have been found-and-validated ────
	const minValidated = 3
	if validated < minValidated {
		t.Errorf("conformance: only %d/%d endpoints were found-in-spec AND ValidateResponse'd (need >= %d); "+
			"check that probes are not all skipping FindRoute",
			validated, len(probes), minValidated)
	} else {
		t.Logf("conformance: %d/%d endpoints validated against OpenAPI spec", validated, len(probes))
	}
}
