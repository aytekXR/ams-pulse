package api_test

// webui_static_test.go — D-076b regression tests for mountWebUI static serving.
//
// Found live during the v0.3.0 browser-accept: index.html references root-level
// build assets (favicon.svg, favicon-16/32.png, /icons/*, /logo/*,
// site.webmanifest) but mountWebUI only routed /assets/* — everything else fell
// into the SPA fallback and returned index.html as text/html, so the browser
// rendered a broken tab icon. The fix serves any real file under WebDir before
// falling back to the SPA index.

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pulse-analytics/pulse/server/internal/api"
	"github.com/pulse-analytics/pulse/server/internal/license"
	"github.com/pulse-analytics/pulse/server/internal/query"
	"github.com/pulse-analytics/pulse/server/internal/store/meta"
)

func newWebUITestServer(t *testing.T, webDir string) *httptest.Server {
	t.Helper()
	ctx := context.Background()
	ddl, err := readMetaDDL(t)
	if err != nil {
		t.Fatalf("meta DDL not found (broken test mount, D-028): %v", err)
	}
	store, err := meta.New(ctx, "sqlite", ":memory:", "webui-test-secret-key")
	if err != nil {
		t.Fatalf("meta.New: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.MigrateEmbedded(ctx, string(ddl)); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	lic, _ := license.New("", "")
	live := &fakeLiveProvider{}
	qsvc := query.New(live, nil, lic)
	srv := api.New(api.Config{ListenAddr: ":0", WebDir: webDir}, store, live, qsvc, lic, nil)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestMountWebUI_ServesRootStaticAssets(t *testing.T) {
	webDir := t.TempDir()
	indexBody := "<!doctype html><html><body>spa-index</body></html>"
	svgBody := `<svg xmlns="http://www.w3.org/2000/svg"></svg>`
	pngBody := "\x89PNG-fake-bytes"
	manifestBody := `{"name":"Pulse"}`
	mustWrite := func(rel, body string) {
		t.Helper()
		p := filepath.Join(webDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("index.html", indexBody)
	mustWrite("favicon.svg", svgBody)
	mustWrite("icons/app-icon-192.png", pngBody)
	mustWrite("site.webmanifest", manifestBody)

	ts := newWebUITestServer(t, webDir)

	get := func(path string) (int, string, string) {
		t.Helper()
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b), resp.Header.Get("Content-Type")
	}

	// Real root-level files must be served as themselves, never as the SPA index.
	for _, tc := range []struct {
		path, want, wantCTFrag string
	}{
		{"/favicon.svg", svgBody, "svg"},
		{"/icons/app-icon-192.png", pngBody, "png"},
		{"/site.webmanifest", manifestBody, ""}, // CT for .webmanifest varies; body is the contract
	} {
		code, body, ct := get(tc.path)
		if code != http.StatusOK {
			t.Errorf("%s: status %d, want 200", tc.path, code)
		}
		if body != tc.want {
			t.Errorf("%s: served wrong body (got %q — the SPA fallback bug serves index.html here)", tc.path, body[:min(40, len(body))])
		}
		if strings.Contains(ct, "text/html") {
			t.Errorf("%s: content-type %q is text/html — browser renders a broken asset", tc.path, ct)
		}
		if tc.wantCTFrag != "" && !strings.Contains(ct, tc.wantCTFrag) {
			t.Errorf("%s: content-type %q, want fragment %q", tc.path, ct, tc.wantCTFrag)
		}
	}

	// SPA fallback must stay intact for client-side routes (no such file).
	if code, body, _ := get("/dashboard"); code != http.StatusOK || body != indexBody {
		t.Errorf("/dashboard: got %d %q, want 200 + SPA index", code, body[:min(40, len(body))])
	}
	// Nonexistent asset-looking paths also fall back to the index (pre-existing contract).
	if code, body, _ := get("/nope.png"); code != http.StatusOK || body != indexBody {
		t.Errorf("/nope.png: got %d %q, want 200 + SPA index", code, body[:min(40, len(body))])
	}
	// API/ops guards preserved.
	if code, _, _ := get("/api/v1/definitely-not-a-route"); code == http.StatusOK {
		t.Errorf("/api/v1/definitely-not-a-route: got 200, want non-200 (must not serve SPA index)")
	}
	// A directory must not be served as a file listing or index (falls back to SPA).
	if code, body, _ := get("/icons"); code != http.StatusOK || body != indexBody {
		t.Errorf("/icons (directory): got %d %q, want SPA index fallback", code, body[:min(40, len(body))])
	}
}
