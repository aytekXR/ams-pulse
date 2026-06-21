# A1 — amsclient: cookie-session auth + per-app REST paths + real fixtures

**Scope (single writer):** `server/pkg/amsclient/client.go`, `server/pkg/amsclient/client_test.go`,
`server/pkg/amsclient/testdata/*`. Do NOT touch any other package. **Author only — do NOT `git add`/commit.**

Read first: `agents/handoffs/wave-realams/GROUND-TRUTH.md` and the real captures in
`agents/handoffs/real-ams-captures/`. Architecture rule: AMS wire formats live ONLY here.

## 1. Cookie-session login + refresh (the crux)

Add to `Config`:
```go
LoginEmail    string // AMS console email for cookie-session auth (optional)
LoginPassword string // AMS console password (optional)
```

Add login state to `Client` (use `sync.Mutex` + `time.Time`; import `bytes`, `errors`,
`net/http/cookiejar`, `sync`):
```go
loginEmail    string
loginPassword string
loginMu       sync.Mutex
lastLogin     time.Time
loggedIn      bool
```

In `New`: when `cfg.LoginEmail != ""`, attach a cookie jar so the JSESSIONID is carried automatically:
```go
hc := &http.Client{Timeout: timeout}
if cfg.LoginEmail != "" {
    jar, _ := cookiejar.New(nil) // never errors in practice
    hc.Jar = jar
}
```
Keep the existing static-Bearer behaviour intact (`authHeader` from `AuthToken`) — both can coexist.

`login(ctx)`: POST `/rest/v2/users/authenticate` with JSON `{"email":..,"password":..}` and
`Content-Type: application/json`. Non-2xx → error. Decode `{"success":bool}`; `success==false` →
`fmt.Errorf("amsclient: login failed (check PULSE_AMS_LOGIN_EMAIL/PASSWORD)")`. On success the jar
already holds the cookie.

`ensureLogin(ctx, force bool) error` — no-op when `loginEmail==""`. Mutex-guarded. Logs in when not
yet logged in, or when `force` and the last login is older than a throttle window
(`const minLoginInterval = 3 * time.Second`). The throttle is critical: a permanent IP-block 403 must
NOT cause a login storm — if `force` but we re-logged-in within `minLoginInterval`, return nil (reuse).

## 2. getJSON: login + single throttled retry on 401/403

Refactor `getJSON` to: best-effort `ensureLogin(ctx,false)` first (ignore its error — per-app endpoints
work without auth); split the actual GET into a `doGet(ctx,path,query) (*http.Response,error)` helper
that sets the Bearer header (if any) + `Accept: application/json`. After the first GET, if the status is
401 or 403 **and** `loginEmail != ""`: close the body, `ensureLogin(ctx,true)`, and if that succeeds
retry the GET **once**. Then decode 2xx (keep the `io.LimitReader(.., 10<<20)` 10 MB cap) or return the
error.

Replace the inline non-2xx error with a typed error so callers can branch on status:
```go
type httpStatusError struct{ Path string; Status int; Body string }
func (e *httpStatusError) Error() string {
    return fmt.Sprintf("amsclient: GET %s: HTTP %d: %s", e.Path, e.Status, e.Body)
}
```
Return `&httpStatusError{path, resp.StatusCode, string(body)}` on non-2xx (body capped at 4096).
(Existing `TestListBroadcasts_Non2xx_ReturnsError` still passes — `Error()` contains "503".)

## 3. Per-app REST paths (the path drift)

- `ListBroadcasts(ctx, app, offset, size)`: if `size<=0 { size = 200 }`; path
  `fmt.Sprintf("/%s/rest/v2/broadcasts/list/%d/%d", app, offset, size)`; **no query params**;
  backfill `AppName=app`.
- `ListBroadcastsPaged(ctx, app)`: loop `pageSize=200`, path
  `fmt.Sprintf("/%s/rest/v2/broadcasts/list/%d/%d", app, offset, pageSize)`; backfill AppName; stop when
  a page has `< pageSize` entries.
- `BroadcastStatistics(ctx, app, streamID)`: path
  `fmt.Sprintf("/%s/rest/v2/broadcasts/%s/broadcast-statistics", app, streamID)`. Rewrite
  `BroadcastStatisticsDTO` to the REAL fields:
  ```go
  type BroadcastStatisticsDTO struct {
      TotalRTMPWatchersCount  int `json:"totalRTMPWatchersCount"`
      TotalHLSWatchersCount   int `json:"totalHLSWatchersCount"`
      TotalWebRTCWatchersCount int `json:"totalWebRTCWatchersCount"`
      TotalDASHWatchersCount  int `json:"totalDASHWatchersCount"`
  }
  ```
- `WebRTCClientStats(ctx, app, streamID)`: path
  `fmt.Sprintf("/%s/rest/v2/broadcasts/%s/webrtc-client-stats/0/100", app, streamID)`.
- `SystemStats(ctx)`: path `/rest/v2/system-status` (still returns `map[string]any`, tolerant).
- `ListApplications(ctx)`: decode `applications` as `[]json.RawMessage`, then per element: if it begins
  with `"` decode as a plain string; else decode as `ApplicationDTO{Name}`. This tolerates BOTH the real
  v3 string array AND the older `[{"name":...}]` form. Skip empties.
- `ClusterNodes(ctx)` and `NodeInfo(ctx,id)`: make **404-tolerant** — on a `*httpStatusError` with
  `Status==404`, return `nil, nil` (ClusterNodes) / `&ClusterNodeDTO{}, nil` (NodeInfo). Use
  `errors.As`. Rationale: standalone AMS has no cluster endpoint; today this spams a Warn every 30 s.

## 4. Tests + fixtures (real captures)

Update `testdata/applications.json` to the real string form:
`{"applications":["LiveApp","WebRTCApp","live","vod"]}` (keep the existing test's `want` list ordering).
Add `testdata/applications_object_form.json` = `{"applications":[{"name":"LiveApp"},{"name":"live"}]}`
and a test `TestListApplications_ObjectFormStillDecodes` (cross-version tolerance).

Copy `agents/handoffs/real-ams-captures/LiveApp_list.json` → `testdata/broadcasts_real_liveapp.json` and
`agents/handoffs/real-ams-captures/broadcast-statistics_test123.json` →
`testdata/broadcast_statistics_real.json`. Add tests:
- `TestListBroadcasts_RealLiveAppCapture`: decode the real list, assert 16 entries, find the
  `broadcasting` stream `test123` with `BitRate==622312`, `Status=="broadcasting"`, viewer counts decode.
- `TestListBroadcasts_UsesPerAppPathParams`: httptest handler that **asserts**
  `r.URL.Path == "/LiveApp/rest/v2/broadcasts/list/0/200"` for `ListBroadcasts(ctx,"LiveApp",0,200)`
  (locks the corrected path; fail the test if the path is wrong).
- `TestBroadcastStatistics_RealFields`: decode `broadcast_statistics_real.json`, assert
  `TotalHLSWatchersCount==0`, `TotalRTMPWatchersCount==-1`, and that the request path ends with
  `/broadcast-statistics`.

Auth tests (use an `httptest.Server` with a stateful handler + `atomic.Int64` login counter):
- `TestLogin_AttachesCookieAndAuthorizes`: a protected GET returns 403 unless the JSESSIONID set by
  `/rest/v2/users/authenticate` is present. Client built with `LoginEmail/LoginPassword` → first call
  logs in, sends the cookie, gets 200.
- `TestLogin_WrongPasswordReturnsError`: authenticate handler returns 200 `{"success":false}` →
  `ListApplications` returns an error containing "login failed".
- `TestSessionExpiry_RelogsInAndRetriesOnce`: handler returns 403 for the first protected GET (stale
  cookie), 200 after a second login; assert the call ultimately succeeds and the login endpoint was hit
  exactly twice (initial + one refresh).
- `TestPersistent403_DoesNotStormLogins`: protected GET always 403; assert `ListApplications` errors and
  the login endpoint is hit a small bounded number of times (≤ 2) — proves the throttle/single-retry.
- `TestClusterNodes_404ReturnsEmptyNoError`: server 404s `/rest/v2/cluster/nodes`; `ClusterNodes`
  returns `(nil, nil)`.

Keep all existing tests passing. Existing broadcast tests call `ListBroadcasts(ctx,"LiveApp",0,200)`
against path-agnostic mocks — they remain valid.

## 5. Self-check (optional but reduces fix-loop rounds)

`go` is only in Docker on this VPS. You MAY self-check compile with:
`sg docker -c "docker run --rm -v /home/aytek/repo/ams-pulse:/repo -w /repo/server -e GOFLAGS=-buildvcs=false golang:1.25 sh -c 'go vet ./pkg/amsclient/ && go test ./pkg/amsclient/ -count=1'"`
(no `-race` needed for the quick check). Do not run the full suite — that is the Verify phase's job.

Return: a concise summary of every path/DTO/auth change and the list of new/updated test + fixture files.
