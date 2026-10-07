package amsclient

// S125 (D-194): login backoff. Found live while reinstalling AMS 3.1.0: the
// collector polls every 5 s and re-tried a rejected login on every poll. AMS
// locks an account for 300 s after two failed logins, so a wrong password — or
// a Pulse that starts before the AMS admin user exists — kept the account locked
// indefinitely, locking humans out of the AMS console too (the installer asks for
// the admin account). A rejected login must now back off; a session that expires
// must still recover at once; and a permanent 403 (the per-app REST IP filter)
// must not trigger a fresh login on every poll.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeClock is a manually advanced clock for the client's login timing.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (f *fakeClock) now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.t
}

func (f *fakeClock) advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.t = f.t.Add(d)
}

// amsStub serves the authenticate endpoint with a switchable reply and answers
// /rest/v2/applications with 403 unless the request carries a session cookie.
type amsStub struct {
	logins    atomic.Int64
	mu        sync.Mutex
	loginBody string // JSON reply of POST /rest/v2/users/authenticate
	loginCode int    // HTTP status of that reply
	appsCode  int    // 0 = 200 with a session cookie, 403 without; else always this
}

func (s *amsStub) setLogin(code int, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loginCode, s.loginBody = code, body
}

func (s *amsStub) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		code, body, appsCode := s.loginCode, s.loginBody, s.appsCode
		s.mu.Unlock()
		switch r.URL.Path {
		case "/rest/v2/users/authenticate":
			s.logins.Add(1)
			if strings.Contains(body, `"success":true`) {
				http.SetCookie(w, &http.Cookie{Name: "JSESSIONID", Value: fmt.Sprintf("s%d", s.logins.Load())})
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			fmt.Fprint(w, body)
		case "/rest/v2/applications":
			if appsCode != 0 {
				w.WriteHeader(appsCode)
				fmt.Fprint(w, `{"error":"not allowed IP"}`)
				return
			}
			if _, err := r.Cookie("JSESSIONID"); err != nil {
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `{"error":"Not authenticated user"}`)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"applications":["LiveApp"]}`)
		default:
			http.NotFound(w, r)
		}
	})
}

const (
	loginOK       = `{"success":true,"message":"system/ADMIN"}`
	loginRejected = `{"success":false,"message":""}`
	loginLocked   = `{"success":false,"message":"Too many login attempts. User is blocked for 300 secs"}`
)

func newBackoffClient(t *testing.T, stub *amsStub) (*Client, *fakeClock) {
	t.Helper()
	srv := httptest.NewServer(stub.handler())
	t.Cleanup(srv.Close)
	c := New(Config{BaseURL: srv.URL, LoginEmail: "admin@example.com", LoginPassword: "pw"})
	clk := &fakeClock{t: time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)}
	c.now = clk.now
	return c, clk
}

// poll simulates one collector poll.
func poll(c *Client) error {
	_, err := c.ListApplications(context.Background())
	return err
}

func TestLoginBackoff_RejectedLoginIsNotRetriedEveryPoll(t *testing.T) {
	stub := &amsStub{}
	stub.setLogin(http.StatusOK, loginRejected)
	c, clk := newBackoffClient(t, stub)

	for i := 0; i < 12; i++ { // one minute of 5 s polls
		err := poll(c)
		if err == nil || !strings.Contains(err.Error(), "login failed") {
			t.Fatalf("poll %d: want a 'login failed' error, got %v", i, err)
		}
		clk.advance(5 * time.Second)
	}
	if n := stub.logins.Load(); n != 1 {
		t.Fatalf("a rejected login was retried %d times in a minute; want exactly 1 "+
			"(AMS locks the account after 2 failures)", n)
	}
}

func TestLoginBackoff_GrowsExponentiallyAndCaps(t *testing.T) {
	stub := &amsStub{}
	stub.setLogin(http.StatusOK, loginRejected)
	c, clk := newBackoffClient(t, stub)

	// Each wait is the full backoff after the previous rejection: 1, 2, 4, 8, 15 (cap), 15 min.
	waits := []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 15 * time.Minute, 15 * time.Minute}
	_ = poll(c) // rejection #1
	for i, w := range waits {
		before := stub.logins.Load()
		clk.advance(w - time.Second)
		_ = poll(c)
		if stub.logins.Load() != before {
			t.Fatalf("step %d: retried before the %v backoff elapsed", i, w)
		}
		clk.advance(time.Second)
		_ = poll(c)
		if stub.logins.Load() != before+1 {
			t.Fatalf("step %d: did not retry once the %v backoff elapsed", i, w)
		}
	}
}

func TestLoginBackoff_HonoursTheLockDurationAMSReports(t *testing.T) {
	stub := &amsStub{}
	stub.setLogin(http.StatusOK, loginLocked)
	c, clk := newBackoffClient(t, stub)

	_ = poll(c)
	clk.advance(299 * time.Second)
	_ = poll(c)
	if n := stub.logins.Load(); n != 1 {
		t.Fatalf("retried %d times inside the 300 s lock AMS reported; want 1", n)
	}
	clk.advance(15 * time.Second) // past 300 s plus the safety margin
	stub.setLogin(http.StatusOK, loginOK)
	if err := poll(c); err != nil {
		t.Fatalf("after the lock expired the poll should succeed: %v", err)
	}
	if n := stub.logins.Load(); n != 2 {
		t.Fatalf("logins = %d, want 2", n)
	}
}

func TestLoginBackoff_SuccessResetsTheBackoff(t *testing.T) {
	stub := &amsStub{}
	stub.setLogin(http.StatusOK, loginRejected)
	c, clk := newBackoffClient(t, stub)

	_ = poll(c) // rejected → 1 min backoff
	clk.advance(time.Minute)
	stub.setLogin(http.StatusOK, loginOK)
	if err := poll(c); err != nil { // the admin user now exists → recovers
		t.Fatalf("poll after fixing credentials: %v", err)
	}
	// A later rejection starts again from the first step (1 min), not from where it was.
	stub.setLogin(http.StatusOK, loginRejected)
	c.invalidateSession()
	_ = poll(c)
	before := stub.logins.Load()
	clk.advance(time.Minute)
	_ = poll(c)
	if stub.logins.Load() != before+1 {
		t.Fatal("after a success the backoff must restart at 1 minute")
	}
}

func TestLoginBackoff_AuthenticateHTTP401IsARejection(t *testing.T) {
	stub := &amsStub{}
	stub.setLogin(http.StatusUnauthorized, `{"success":false}`)
	c, clk := newBackoffClient(t, stub)
	for i := 0; i < 6; i++ {
		_ = poll(c)
		clk.advance(5 * time.Second)
	}
	if n := stub.logins.Load(); n != 1 {
		t.Fatalf("HTTP 401 from authenticate retried %d times in 30 s; want 1", n)
	}
}

func TestLoginBackoff_ServerErrorsAreRetriedEveryPoll(t *testing.T) {
	// A 5xx is AMS being unwell, not a credential problem — it cannot lock the
	// account, so the collector should keep trying at its normal cadence.
	stub := &amsStub{}
	stub.setLogin(http.StatusInternalServerError, `{}`)
	c, clk := newBackoffClient(t, stub)
	for i := 0; i < 3; i++ {
		_ = poll(c)
		clk.advance(5 * time.Second)
	}
	if n := stub.logins.Load(); n < 3 {
		t.Fatalf("a 5xx login reply was retried only %d times in 3 polls; want at least 3", n)
	}
}

func TestPersistent403_DoesNotReloginEveryPoll(t *testing.T) {
	// The per-app REST IP filter answers 403 to every request, session or not.
	// A fresh login cannot fix that, so polls must not log in again each time.
	stub := &amsStub{appsCode: http.StatusForbidden}
	stub.setLogin(http.StatusOK, loginOK)
	c, clk := newBackoffClient(t, stub)

	for i := 0; i < 12; i++ { // one minute of 5 s polls
		if err := poll(c); err == nil {
			t.Fatalf("poll %d: want the 403 surfaced as an error", i)
		}
		clk.advance(5 * time.Second)
	}
	if n := stub.logins.Load(); n > 3 {
		t.Fatalf("a permanent 403 caused %d logins in a minute; want ≤ 3", n)
	}
}

func TestExpiredSession_StillRecoversOnTheFirst403(t *testing.T) {
	stub := &amsStub{}
	stub.setLogin(http.StatusOK, loginOK)
	c, _ := newBackoffClient(t, stub)

	if err := poll(c); err != nil {
		t.Fatalf("first poll: %v", err)
	}
	// AMS restarts: the session cookie is no longer valid. Simulate it by clearing
	// the jar, so the next request carries no session and gets 403.
	c.httpClient.Jar = newSimpleCookieJar()
	if err := poll(c); err != nil {
		t.Fatalf("an expired session must be renewed on the first 403: %v", err)
	}
	if n := stub.logins.Load(); n != 2 {
		t.Fatalf("logins = %d, want 2 (initial + one renewal)", n)
	}
}
