# Real-AMS ground truth — `test.antmedia.io` (curl-verified 2026-06-21 from the VPS)

Every fact below was verified by live `curl` from the VPS (`161.97.172.146`), not inferred.
Authoritative real JSON captures: `agents/handoffs/real-ams-captures/*.json`.

## Server
- **AMS 3.0.3 Enterprise Edition** (`GET /rest/v2/version` → `{"versionName":"3.0.3","versionType":"Enterprise Edition",...}`).
- Standalone (no cluster). `server-settings`: `jwtServerControlEnabled=false`, empty `jwtServerSecretKey`.

## Auth = cookie-session (NOT a static Bearer token)
- `POST /rest/v2/users/authenticate` with JSON body `{"email":"...","password":"..."}`
  → **HTTP 200** + `Set-Cookie: JSESSIONID=...` and body `{"success":true,"message":"system/ADMIN",...}`.
- **Login success is in the JSON body** (`"success": true|false`), NOT the HTTP status:
  a wrong password returns **HTTP 200** with `{"success":false,...}`.
- The `JSESSIONID` cookie unlocks the root management API.
- There is **no JWT path** (jwtServerControlEnabled=false) → the only option is login + cookie + refresh.

## REST path layout (per-application context, path params)
| Purpose | REAL working path | amsclient CURRENT (broken) |
|---|---|---|
| App discovery (root, needs cookie) | `GET /rest/v2/applications` → `{"applications":["LiveApp","live",...]}` **array of strings** | same path, decodes `[{"name":...}]` → fails |
| Broadcast list (per-app, path params) | `GET /{app}/rest/v2/broadcasts/list/{offset}/{size}` | `GET /rest/v2/broadcasts/{app}/list?offset=&size=` → **404** |
| Broadcast count | `GET /{app}/rest/v2/broadcasts/count` → `{"number":N}` | n/a |
| Active live count | `GET /{app}/rest/v2/broadcasts/active-live-stream-count` → `{"number":N}` | n/a |
| Per-stream stats | `GET /{app}/rest/v2/broadcasts/{id}/broadcast-statistics` → `{"totalRTMPWatchersCount":-1,"totalHLSWatchersCount":0,"totalWebRTCWatchersCount":0,"totalDASHWatchersCount":0}` | `/rest/v2/broadcasts/{app}/{id}/statistics` (wrong path + wrong DTO fields) |
| WebRTC peer stats | `GET /{app}/rest/v2/broadcasts/{id}/webrtc-client-stats/0/100` → `[]` (when no peers) | `/rest/v2/broadcasts/{app}/{id}/webrtc-client-stats/0/100` → 404 |
| System info | `GET /rest/v2/system-status` → `{"osName":"Linux","osArch":"amd64","javaVersion":"17","processorCount":8}` | `/rest/v2/system/stats` → **404** |
| Cluster nodes | `GET /rest/v2/cluster/nodes` → **404** (standalone, no cluster) | same path → 404 (currently errors) |

Notes:
- `BroadcastStatistics`, `SystemStats`, `NodeInfo` are **never called in production** (collector map) —
  fix their paths/DTOs for correctness + tests, but they carry zero runtime risk.
- `ListBroadcastsPaged` (`restpoller.go:151`) is the **only critical-path** call. Broken path ⇒
  `/api/v1/live/overview` shows 0 viewers/publishers.
- `ClusterNodes` 404 is silently ignored in the restpoller, but `cluster.Discovery` (`discovery.go:124`)
  logs a `Warn` every 30 s ⇒ make `ClusterNodes`/`NodeInfo` **404-tolerant** (return empty, nil err).

## Per-app IP allow-list + auth interaction
- From the VPS IP, **8 of 16 apps return 403 "Not allowed IP"**: Icomms, TEST, VsMediaTesting,
  WebRTCAppEE, amartest, drmtest, live, ll-hls.
- **Open apps (200):** 24x7test, Conference, **LiveApp** (16 broadcasts, **1 live: `test123` broadcasting**,
  bitrate 622312, 0 viewers right now), LiveShopping, PetarTest2 (3), clipcreator (1), demo (4), meet (1).
- **Per-app broadcast endpoints need NO auth from an allowed IP** (`/LiveApp/rest/v2/broadcasts/count`
  → 200 with no cookie at all). The cookie is required for `/rest/v2/applications` (discovery) + root/system.
- Both "session expired/unauth" and "IP-blocked" return **403** (cannot distinguish by status alone).
  ⇒ the refresh logic must re-login + retry **once**, **throttled**, so a permanent IP-block 403 can
  never cause a login storm.

## Validation target
`PULSE_AMS_APPLICATIONS` = the open apps (e.g. `LiveApp,demo,PetarTest2,clipcreator,meet,24x7test,Conference,LiveShopping`).
With LiveApp's `test123` live, `/api/v1/live/overview` must show **`total_publishers` ≥ 1** (viewers may be 0).
