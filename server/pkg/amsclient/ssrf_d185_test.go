package amsclient_test

// ssrf_d185_test.go — SSRF guard and health-surface sanitation on the AMS poll client.
//
// External review round 11 (N-01) filed the AMS poll client as the last outbound
// client without ssrfguard on its own dialer. The ledger's reachability claim was
// wrong (an API-created ams_sources row is never polled — only PULSE_AMS_URL
// reaches this client), but the guard gap was real, and the ledger's own bounding
// argument — "the poll loop only ever appends fixed AMS paths, so the IMDS
// credential path is unreachable" — is refuted by TestClient_RefusesRedirectToIMDS
// below: the redirect target is chosen by the responder, not by us.
//
// Each test here fails on the pre-D-185 client: without Control on the dialer the
// errors are dial timeouts rather than guard refusals, and without HealthSafeError
// the upstream body is present in the health string.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aytekXR/ams-pulse/server/pkg/amsclient"
)

// TestClient_RefusesIMDSDirect proves ssrfguard.DialControl is installed on the
// poll client's transport: a base URL pointing at the IMDSv4 address must fail
// with the guard's refusal, not with a dial timeout.
func TestClient_RefusesIMDSDirect(t *testing.T) {
	c := amsclient.New(amsclient.Config{BaseURL: "http://169.254.169.254", Timeout: 2 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := c.GetVersion(ctx)
	if err == nil {
		t.Fatal("expected error dialing 169.254.169.254, got nil")
	}
	if !strings.Contains(err.Error(), "ssrfguard: refusing to dial restricted") {
		t.Fatalf("guard not wired — the dial was actually attempted: %v", err)
	}
}

// TestClient_RefusesRedirectToIMDS is the finding that matters. CheckRedirect
// follows up to 10 hops, so a hostile or compromised AMS — a separate trust
// domain from Pulse — chooses the next URL, including its PATH. The guard must
// re-run on the redirect hop, at the socket, after DNS resolution.
func TestClient_RefusesRedirectToIMDS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/iam/security-credentials/pulse", http.StatusFound)
	}))
	defer srv.Close()

	c := amsclient.New(amsclient.Config{BaseURL: srv.URL, Timeout: 2 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := c.GetVersion(ctx)
	if err == nil {
		t.Fatal("expected the redirect hop to 169.254.169.254 to be refused, got nil")
	}
	if !strings.Contains(err.Error(), "ssrfguard: refusing to dial restricted") {
		t.Fatalf("redirect hop was not guarded: %v", err)
	}
}

// TestClient_AllowsLoopbackAndPrivate pins the other half of the policy: a real
// AMS node routinely lives on loopback or an RFC-1918 network, so the guard must
// NOT reject those. An over-broad guard would take the product down.
func TestClient_AllowsLoopbackAndPrivate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"versionName":"3.0.3","versionType":"Enterprise Edition"}`))
	}))
	defer srv.Close()

	c := amsclient.New(amsclient.Config{BaseURL: srv.URL, Timeout: 2 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	v, err := c.GetVersion(ctx)
	if err != nil {
		t.Fatalf("loopback AMS must remain reachable: %v", err)
	}
	if v == nil || v.VersionName != "3.0.3" {
		t.Fatalf("unexpected version payload: %+v", v)
	}
}

// TestHealthSafeError_StripsUpstreamBody covers the disclosure the guard does not
// close on its own: getJSON copies up to 4 KB of the upstream RESPONSE BODY into
// its error, the restpoller stores that as its last-error, and /healthz — which
// api/server.go registers as "Operational (unauthenticated)" — republishes it.
func TestHealthSafeError_StripsUpstreamBody(t *testing.T) {
	const secret = "SECRET-ACCESS-KEY-aws-imds-credential-blob"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(secret))
	}))
	defer srv.Close()

	c := amsclient.New(amsclient.Config{BaseURL: srv.URL, Timeout: 2 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := c.GetVersion(ctx)
	if err == nil {
		t.Fatal("expected an error from the 500 response")
	}
	// The operator-facing error keeps the body: logs are where it belongs.
	if !strings.Contains(err.Error(), secret) {
		t.Fatalf("the full error should still carry the body for logs, got: %v", err)
	}
	// The health-facing rendering must not.
	safe := amsclient.HealthSafeError(err)
	if strings.Contains(safe, secret) {
		t.Fatalf("HealthSafeError leaked the upstream body: %q", safe)
	}
	if !strings.Contains(safe, "HTTP 500") {
		t.Fatalf("HealthSafeError dropped the diagnosis as well as the body: %q", safe)
	}
}

// TestHealthSafeError_StripsLoginBody covers the SECOND route by which an
// upstream body reaches /healthz: a failed cookie-session login. getJSON surfaces
// loginErr directly on the re-login path, so sanitizing only getJSON's own error
// would have missed this one.
func TestHealthSafeError_StripsLoginBody(t *testing.T) {
	const secret = "LOGIN-BODY-SECRET-do-not-publish"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/users/authenticate") {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(secret))
			return
		}
		// Force the re-login path: 401 on the data request too.
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("nope"))
	}))
	defer srv.Close()

	c := amsclient.New(amsclient.Config{
		BaseURL:       srv.URL,
		LoginEmail:    "admin@example.com",
		LoginPassword: "hunter2",
		Timeout:       2 * time.Second,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := c.ListApplications(ctx)
	if err == nil {
		t.Fatal("expected an error when login returns 401")
	}
	if safe := amsclient.HealthSafeError(err); strings.Contains(safe, secret) {
		t.Fatalf("HealthSafeError leaked the login response body: %q", safe)
	}
}

// TestHealthSafeError_Bounded pins the length cap and the nil case.
func TestHealthSafeError_Bounded(t *testing.T) {
	if got := amsclient.HealthSafeError(nil); got != "" {
		t.Fatalf("nil error should render empty, got %q", got)
	}
	long := strings.Repeat("ä", 5000) // multi-byte on purpose: no split runes
	safe := amsclient.HealthSafeError(&stubErr{msg: long})
	if len([]rune(safe)) > 201 {
		t.Fatalf("HealthSafeError did not bound the message: %d runes", len([]rune(safe)))
	}
	if !strings.HasSuffix(safe, "…") {
		t.Fatalf("expected an ellipsis marker on a truncated message, got %q", safe)
	}
}

type stubErr struct{ msg string }

func (e *stubErr) Error() string { return e.msg }

// TestAppNameIsPathEscaped covers the D-185 escaping fix. The app segment is
// interpolated into every per-app request path, and unlike its sibling streamID
// — escaped in the same Sprintf since an earlier round — it was raw. The value
// does not come from the operator: it is whatever the REMOTE server returned
// from its application list, so it is exactly as trustworthy as the server the
// poller is pointed at.
//
// The assertion is on the escaped path the server receives, not on the string we
// build, because the escaping only matters if it survives to the wire.
func TestAppNameIsPathEscaped(t *testing.T) {
	cases := []struct {
		name, app, wantPath string
	}{
		{
			// Traversal attempt: must stay one path segment.
			name:     "slashes_are_escaped",
			app:      "live/../../latest/meta-data",
			wantPath: "/live%2F..%2F..%2Flatest%2Fmeta-data/rest/v2/broadcasts/list/0/200",
		},
		{
			name:     "space_is_escaped",
			app:      "my app",
			wantPath: "/my%20app/rest/v2/broadcasts/list/0/200",
		},
		{
			// Positive control: an ordinary AMS app name is byte-identical after
			// escaping, so the common path is unchanged.
			name:     "normal_app_unchanged",
			app:      "LiveApp",
			wantPath: "/LiveApp/rest/v2/broadcasts/list/0/200",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotPath := make(chan string, 4)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case gotPath <- r.URL.EscapedPath():
				default:
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[]`))
			}))
			defer srv.Close()

			c := amsclient.New(amsclient.Config{BaseURL: srv.URL, Timeout: 2 * time.Second})
			if _, err := c.ListBroadcasts(context.Background(), tc.app, 0, 200); err != nil {
				t.Fatalf("ListBroadcasts: %v", err)
			}
			select {
			case got := <-gotPath:
				if got != tc.wantPath {
					t.Errorf("escaped request path = %q, want %q\n(app %q must be url.PathEscape'd so it cannot leave its path segment)",
						got, tc.wantPath, tc.app)
				}
			default:
				t.Fatal("server received no request")
			}
		})
	}
}
