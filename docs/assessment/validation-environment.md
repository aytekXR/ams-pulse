# Validation Environment — Real-AMS Harness Design

**Phase:** 2 — Test Environment
**Produced:** S16 close (2026-07-11)
**Implements in:** S17

This document specifies the reusable test harness for running validation
scenarios against the real Ant Media Server (AMS 3.0.3 Enterprise,
`http://161.97.172.146:5080`) and the production Pulse deployment
(`https://beyondkaira.com`). The harness is designed for repeatability:
any scenario can be rerun from scratch in a new session without manual
state setup.

---

## 1. Topology

```
┌──────────────────────────────────────────────────────────────────┐
│  VPS: 161.97.172.146                                             │
│                                                                  │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │  pulse-prod (5-overlay compose)                             ││
│  │    caddy  :443 → beyondkaira.com / pulse.beyondkaira.com    ││
│  │    pulse  :8090 API+UI  :8091 beacon  :8092 webhook         ││
│  │    clickhouse  pulse-migrate  backup-sidecar                ││
│  └─────────────────────────────────────────────────────────────┘│
│                                                                  │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │  AMS (container: antmedia, --network host)                  ││
│  │    :5080 REST API + HLS + WebSocket (WebRTC signaling)      ││
│  │    :1935 RTMP                                               ││
│  │    :4200 SRT                                                ││
│  └─────────────────────────────────────────────────────────────┘│
│                                                                  │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │  ams-teststream (container)                                 ││
│  │    ffmpeg RTMP 2 Mbps test-pattern → AMS LiveApp/teststream ││
│  └─────────────────────────────────────────────────────────────┘│
│                                                                  │
│  ┌──────────────────────────────────────┐                       │
│  │  pulse-realams (isolated test stack) │  ← validation target  │
│  │    pulse :18090  (base + real-ams)   │                       │
│  │    No caddy, no backup sidecar       │                       │
│  └──────────────────────────────────────┘                       │
└──────────────────────────────────────────────────────────────────┘
```

The validation harness targets either:
- **pulse-prod** — for evidence that mirrors the real operator experience
- **pulse-realams** — for intrusive test runs (restart, license swap,
  stress) that must not disturb prod

All harness scripts check which target is active via the `PULSE_TARGET`
environment variable (`prod` or `realams`, default `realams`).

---

## 2. Compose Stack for Isolated Testing

### 2.1 Existing overlay (pulse-realams)

Already available: base + real-ams overlays + a `realams-test` overlay.
Pulse is exposed on `127.0.0.1:18090`.

Start command:
```bash
sg docker -c 'docker compose -p pulse-realams \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.real-ams.yml \
  --env-file deploy/.env \
  up -d'
```

Stop:
```bash
sg docker -c 'docker compose -p pulse-realams \
  -f deploy/docker-compose.yml \
  -f deploy/docker-compose.real-ams.yml \
  down'
```

### 2.2 Harness script location

```
qa/realams/
  harness/
    env.sh          — load PULSE_URL, AMS_URL, TOKEN, COOKIE_FILE
    auth.sh         — AMS cookie-session auth (one-call, not retried)
    assert.sh       — comparison helpers: assert_eq, assert_approx,
                      assert_gte, assert_lte
    capture.sh      — timestamped snapshot capture to evidence/
    publisher.sh    — ffmpeg RTMP publisher control (start/stop/kill)
    viewer-sim.sh   — HLS viewer simulation (curl loop)
    failures.sh     — failure injection helpers
  scenarios/        — one script per TC-* scenario
  evidence/         — GITIGNORED; timestamped JSON+screenshot packages
  Makefile          — targets: validate-all, validate-<phase>, validate-<tc>
```

### 2.3 env.sh

```bash
# qa/realams/harness/env.sh
PULSE_TARGET=${PULSE_TARGET:-realams}
if [ "$PULSE_TARGET" = "prod" ]; then
  PULSE_URL="https://beyondkaira.com/api/v1"
  PULSE_WS="wss://beyondkaira.com/api/v1/live/ws"
else
  PULSE_URL="http://127.0.0.1:18090/api/v1"
  PULSE_WS="ws://127.0.0.1:18090/api/v1/live/ws"
fi
AMS_URL="http://161.97.172.146:5080"
AMS_COOKIE_FILE="$(dirname "$0")/../evidence/.ams-cookie"
# Token from oguz-testing.md line 159 / deploy/.env
PULSE_TOKEN="${PULSE_TOKEN:-$(grep PULSE_ADMIN_TOKEN deploy/.env | cut -d= -f2)}"
EVIDENCE_DIR="qa/realams/evidence/$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$EVIDENCE_DIR"
```

---

## 3. AMS Authentication

**Mechanism:** Cookie-session via
`POST /rest/v2/users/authenticate {email, password}`
(client.go:250, AMS jwtServerControlEnabled=false confirmed from
server-settings.json)

**Critical constraints:**
- ALLOWED_LOGIN_ATTEMPTS = 2; any third failed attempt locks the account
  for 5 minutes, keyed by email (not IP). Do NOT retry on failure.
- Use `admin@` account for harness scripts (Pulse machine account).
  Use `aytek@` for human console sessions. Never share the lockout counter.
- Passwords are MD5-hashed in browser but AMS accepts plaintext from
  curl/Go (AMS checks `MD5(submitted) == stored_md5`). Harness uses
  plaintext passwords from `deploy/.env`.

### 3.1 auth.sh

```bash
# qa/realams/harness/auth.sh
# MUST be called once per harness session; never called in a loop
set -euo pipefail
source "$(dirname "$0")/env.sh"

AMS_EMAIL="${AMS_EMAIL:-$(grep PULSE_AMS_USER deploy/.env | cut -d= -f2)}"
AMS_PASS="${AMS_PASS:-$(grep PULSE_AMS_PASS deploy/.env | cut -d= -f2)}"

echo "[auth] authenticating to AMS as $AMS_EMAIL"
RESP=$(curl -s -c "$AMS_COOKIE_FILE" -X POST \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"$AMS_EMAIL\",\"password\":\"$AMS_PASS\"}" \
  "$AMS_URL/rest/v2/users/authenticate")

SUCCESS=$(echo "$RESP" | jq -r '.success // false')
if [ "$SUCCESS" != "true" ]; then
  echo "[auth] FAIL: $RESP"
  echo "[auth] If this is a password error, wait 5 minutes before retrying"
  exit 1
fi
echo "[auth] OK"
```

---

## 4. Publisher Control

### 4.1 Synthetic RTMP Publisher (ffmpeg)

The existing `ams-teststream` container runs a continuous 2 Mbps
test-pattern feed into `LiveApp/teststream`. The harness can control it:

```bash
# publisher.sh
start_publisher() {
  local STREAM_ID="${1:-teststream}"
  local APP="${2:-LiveApp}"
  local BITRATE_KBPS="${3:-2000}"
  docker run -d --rm --name "pulse-pub-${STREAM_ID}" \
    linuxserver/ffmpeg \
    -re -f lavfi -i "testsrc2=size=1280x720:rate=30" \
    -f lavfi -i "sine=frequency=440" \
    -c:v libx264 -b:v "${BITRATE_KBPS}k" -g 60 \
    -c:a aac -b:a 128k \
    -f flv "rtmp://161.97.172.146:1935/${APP}/${STREAM_ID}"
}

stop_publisher() {
  local STREAM_ID="${1:-teststream}"
  docker rm -f "pulse-pub-${STREAM_ID}" 2>/dev/null || true
}

kill_publisher() {
  # Abrupt kill — simulates encoder crash for terminated_unexpectedly test
  local STREAM_ID="${1:-teststream}"
  docker kill "pulse-pub-${STREAM_ID}" 2>/dev/null || true
}
```

**Cleanup:** Docker root artifacts can appear in mounted volumes.
Remove via:
```bash
docker run --rm -v <dir>:/s alpine rm -rf /s/<target>
```
(Memory note: Docker root artifacts cleanup — host `rm` fails without
sudo; use alpine container.)

### 4.2 Multi-Stream Bulk Publisher

For concurrent-stream scenarios:
```bash
start_bulk_publishers() {
  local COUNT="${1:-5}"
  local APP="${2:-LiveApp}"
  local PREFIX="${3:-valtest}"
  for i in $(seq 1 "$COUNT"); do
    local ID
    ID=$(printf "%s%04d" "$PREFIX" "$i")
    start_publisher "$ID" "$APP" 500 &
  done
  wait
  echo "[publisher] Started $COUNT streams with prefix $PREFIX"
}
```

### 4.3 AMS API Stream Control (fallback)

When ffmpeg is not available or for large-scale count injection:
```bash
# Create a stream object without a real ingest (status: created)
create_ams_stream() {
  local STREAM_ID="$1"
  local APP="${2:-LiveApp}"
  curl -s -b "$AMS_COOKIE_FILE" -X POST \
    -H "Content-Type: application/json" \
    -d "{\"streamId\":\"$STREAM_ID\",\"name\":\"$STREAM_ID\"}" \
    "$AMS_URL/$APP/rest/v2/broadcasts/create"
}
```

Note: AMS REST-created streams start in `created` status (no ingest).
Only real ffmpeg/encoder publish changes status to `broadcasting`.

---

## 5. Viewer Simulation

### 5.1 HLS Viewer Loop (curl)

Simulates HLS viewer activity by polling the playlist and segments:
```bash
start_hls_viewer() {
  local STREAM_ID="${1:-teststream}"
  local APP="${2:-LiveApp}"
  local VIEWER_ID="${3:-viewer-001}"
  (
    PLAYLIST="http://161.97.172.146:5080/$APP/streams/$STREAM_ID/playlist.m3u8"
    while true; do
      SEGMENTS=$(curl -s "$PLAYLIST" | grep "\.ts" | head -3)
      for SEG in $SEGMENTS; do
        BASE=$(echo "$PLAYLIST" | rev | cut -d/ -f2- | rev)
        curl -s -o /dev/null "$BASE/$SEG"
      done
      sleep 2
    done
  ) &
  echo $! > "/tmp/hls-viewer-$VIEWER_ID.pid"
}

stop_hls_viewer() {
  local VIEWER_ID="${1:-viewer-001}"
  kill "$(cat /tmp/hls-viewer-$VIEWER_ID.pid)" 2>/dev/null || true
}
```

### 5.2 WebRTC Viewer (Playwright headless)

For real WebRTC viewer sessions (counts viewable in
`webRTCViewerCount`, stats in `webrtc-client-stats`):

```bash
# HOST CONSTRAINT: Playwright runs ONLY in docker on this VPS (no browser
# libs on the host, no sudo — same rule as the CI e2e gate; see §9). Image
# already pulled: mcr.microsoft.com/playwright:v1.61.1-noble — keep it in
# lockstep with the pinned @playwright/test version in web/package.json.
# --network host so the container reaches AMS on 161.97.172.146:5080;
# web/ is mounted so `require('playwright')` resolves from node_modules.
start_webrtc_viewer() {
  local STREAM_ID="${1:-teststream}"
  local APP="${2:-LiveApp}"
  local NAME="webrtc-viewer-${STREAM_ID}-$$"
  sg docker -c "docker run -d --rm --name '$NAME' --ipc=host --network host \
    -w /work -v /home/aytek/repo/ams-pulse/web:/work \
    mcr.microsoft.com/playwright:v1.61.1-noble node -e \"
const { chromium } = require('playwright');
(async () => {
  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();
  // AMS sample player page (adjust path to match actual AMS player URL)
  await page.goto('http://161.97.172.146:5080/$APP/player.html?id=$STREAM_ID');
  await page.waitForTimeout(120000);  // hold 2 min for stats collection
  await browser.close();
})();
\""
  echo "$NAME" > "/tmp/webrtc-viewer-$STREAM_ID.cid"
}

stop_webrtc_viewer() {
  local STREAM_ID="${1:-teststream}"
  sg docker -c "docker stop \"\$(cat /tmp/webrtc-viewer-$STREAM_ID.cid)\"" 2>/dev/null || true
}
```

Note: Playwright headless WebRTC viewer creates a real peer connection.
The AMS `webrtc-client-stats` endpoint will return a non-empty array
only when this viewer is active and the stream is `broadcasting`.

### 5.3 Viewer Ramp Profile

For anomaly detection scenarios requiring a gradual ramp followed by a
spike:
```bash
ramp_hls_viewers() {
  local STREAM_ID="$1"
  local TARGET="$2"      # target viewer count
  local STEP="${3:-5}"   # viewers to add per iteration
  local INTERVAL="${4:-10}"  # seconds between steps
  for i in $(seq $STEP $STEP $TARGET); do
    for j in $(seq 1 $STEP); do
      start_hls_viewer "$STREAM_ID" "LiveApp" "viewer-ramp-$i-$j"
    done
    echo "[ramp] $i/$TARGET HLS viewers active"
    sleep "$INTERVAL"
  done
}
```

---

## 6. Failure Injection

### 6.1 Publisher Disconnect

```bash
# Simulates encoder crash (terminated_unexpectedly)
inject_publisher_kill() {
  kill_publisher "$1"
  echo "[failure] Publisher killed at $(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
```

After killing: poll AMS for `terminated_unexpectedly` status; poll Pulse
for `publish_end` event. Measure the latency.

### 6.2 Invalid Stream Key

AMS enforces publish tokens only if token control is enabled
(`publishTokenControl=true`, which it is not on the test AMS per
`server-settings.json` where `jwtServerControlEnabled=false`). Invalid
stream key scenarios require enabling token enforcement on a test app.

For token-based scenarios:
```bash
# Publish with wrong stream key (token mismatch)
inject_invalid_stream_key() {
  local APP="$1"
  local WRONG_KEY="invalid-key-$(date +%s)"
  docker run --rm linuxserver/ffmpeg \
    -re -f lavfi -i "testsrc2=size=320x180:rate=5" \
    -c:v libx264 -b:v 100k \
    -f flv "rtmp://161.97.172.146:1935/$APP/$WRONG_KEY" \
    2>&1 | head -20 &
  sleep 5
  docker kill "$(docker ps -qf ancestor=linuxserver/ffmpeg)" 2>/dev/null || true
}
```

### 6.3 Network Interruption (Docker network disconnect)

If the publisher is a Docker container:
```bash
inject_network_disconnect() {
  local CONTAINER="${1:-pulse-pub-teststream}"
  docker network disconnect bridge "$CONTAINER"
  echo "[failure] Network disconnected for $CONTAINER at $(date -u)"
  sleep 10
  docker network connect bridge "$CONTAINER"
  echo "[failure] Network reconnected at $(date -u)"
}
```

For host-network containers (AMS uses `--network host`), `tc` or
`iptables` rules are needed. On the VPS, `iptables` is available but
`sudo` is required. Add `tc` rules if running as root or with
`CAP_NET_ADMIN`:
```bash
# Block RTMP ingest for 15 seconds (requires root on VPS)
inject_tc_loss() {
  local IFACE="${1:-eth0}"
  local DURATION="${2:-15}"
  tc qdisc add dev "$IFACE" root netem loss 100% delay 1000ms
  sleep "$DURATION"
  tc qdisc del dev "$IFACE" root
}
```

Note: VPS does not have `sudo` for the harness user. If `iptables`/`tc`
is unavailable, use Docker container-level disconnect only.

### 6.4 AMS Restart

```bash
inject_ams_restart() {
  docker restart antmedia
  echo "[failure] AMS restart triggered at $(date -u)"
  # Wait for AMS to become ready (up to 60 s)
  for i in $(seq 1 30); do
    if curl -s --max-time 2 "$AMS_URL/rest/v2/version" > /dev/null 2>&1; then
      echo "[failure] AMS ready after ${i}×2 s"
      return 0
    fi
    sleep 2
  done
  echo "[failure] AMS did not recover within 60 s"
  return 1
}
```

After restart: measure how long Pulse takes to re-establish the poll
session (requires new cookie-session auth, throttled to ≥3 s between
re-login attempts per client.go:250 comment).

### 6.5 Pulse Restart

```bash
inject_pulse_restart() {
  sg docker -c 'docker compose -p pulse-realams restart pulse'
  # Wait for Pulse to become healthy
  for i in $(seq 1 30); do
    if curl -s --max-time 2 http://127.0.0.1:18090/api/v1/healthz | \
       jq -e '.status == "ok"' > /dev/null 2>&1; then
      echo "[failure] Pulse ready after ${i}×2 s"
      return 0
    fi
    sleep 2
  done
  echo "[failure] Pulse did not recover within 60 s"
  return 1
}
```

After restart: in-memory live snapshot is rebuilt from the ClickHouse
history on startup (or empty, depending on in-memory vs. persistent
design). Validate whether live streams visible before restart reappear.

### 6.6 Expired Token Scenario

AMS token expiry (relevant when token control is enabled):
```bash
inject_expired_token_publish() {
  local STREAM_ID="$1"
  local APP="${2:-LiveApp}"
  # Create a token that expires 1 second from now
  EXPIRE_TS=$(( $(date +%s%3N) + 1000 ))
  TOKEN=$(curl -s -b "$AMS_COOKIE_FILE" \
    "$AMS_URL/$APP/rest/v2/broadcasts/$STREAM_ID/token?expireDate=$EXPIRE_TS&type=publish" \
    | jq -r '.tokenId')
  sleep 2  # Token already expired
  docker run --rm linuxserver/ffmpeg \
    -re -f lavfi -i "testsrc2=size=320x180:rate=5" \
    -c:v libx264 -b:v 100k \
    -f flv "rtmp://161.97.172.146:1935/$APP/$STREAM_ID?token=$TOKEN" \
    2>&1 | head -10 || true
}
```

Note: Token enforcement requires `publishTokenControl=true` on the AMS
app. The test AMS has token control disabled. Enable on `pulse-test` app
only, not on `LiveApp`, to avoid disrupting live streams.

### 6.7 AMS Unavailable (Pulse behavior under AMS downtime)

```bash
inject_ams_stop() {
  docker stop antmedia
  echo "[failure] AMS stopped at $(date -u)"
}

inject_ams_start() {
  docker start antmedia
  # Re-authenticate after recovery
  sleep 30  # AMS startup time
  bash qa/realams/harness/auth.sh
}
```

Pulse should: log repeated poll errors, mark all streams as offline after
`StaleTimeout`, surface degraded health in `GET /api/v1/healthz`.

---

## 7. Evidence Capture

### 7.1 capture.sh

```bash
# qa/realams/harness/capture.sh
# Usage: capture_ams <endpoint> <label>
capture_ams() {
  local ENDPOINT="$1"
  local LABEL="$2"
  local OUTFILE="$EVIDENCE_DIR/ams-${LABEL}-$(date -u +%H%M%S).json"
  curl -s -b "$AMS_COOKIE_FILE" \
    -D "${OUTFILE}.headers" \
    "$AMS_URL$ENDPOINT" | jq . > "$OUTFILE"
  echo "[capture] AMS $LABEL → $OUTFILE"
}

# Usage: capture_pulse <endpoint> <label>
capture_pulse() {
  local ENDPOINT="$1"
  local LABEL="$2"
  local OUTFILE="$EVIDENCE_DIR/pulse-${LABEL}-$(date -u +%H%M%S).json"
  curl -s \
    -H "Authorization: Bearer $PULSE_TOKEN" \
    -D "${OUTFILE}.headers" \
    "$PULSE_URL$ENDPOINT" | jq . > "$OUTFILE"
  echo "[capture] Pulse $LABEL → $OUTFILE"
}

# Usage: compare_viewer_count <stream_id> <app>
compare_viewer_count() {
  local STREAM_ID="$1"
  local APP="${2:-LiveApp}"
  # AMS ground truth (inline broadcast count)
  AMS_VC=$(curl -s -b "$AMS_COOKIE_FILE" \
    "$AMS_URL/$APP/rest/v2/broadcasts/$STREAM_ID" | \
    jq '(.hlsViewerCount // 0) + (.webRTCViewerCount // 0) +
        (if .rtmpViewerCount < 0 then 0 else .rtmpViewerCount end) +
        (.dashViewerCount // 0)')
  # Pulse assertion
  PULSE_VC=$(curl -s -H "Authorization: Bearer $PULSE_TOKEN" \
    "$PULSE_URL/live/streams" | \
    jq --arg id "$STREAM_ID" \
    '.streams[] | select(.stream_id == $id) | .viewer_count // 0')
  echo "[compare] AMS viewer_count=$AMS_VC  Pulse viewer_count=$PULSE_VC"
  if [ "$AMS_VC" = "$PULSE_VC" ]; then
    echo "[compare] PASS: counts match"
  else
    echo "[compare] FAIL: delta=$(( PULSE_VC - AMS_VC ))"
  fi
}
```

### 7.2 Evidence Package Structure

```
qa/realams/evidence/20260712T143022Z-TC-L-01/
  ams-broadcasts-before-143022.json     AMS state before action
  ams-broadcasts-after-143045.json      AMS state after action
  pulse-live-overview-before-143022.json
  pulse-live-overview-after-143050.json
  timeline.txt                          human-readable event log
  verdict.txt                           PASS/FAIL + numeric deltas
  screenshot-*.png                      Playwright screenshots (if used)
```

---

## 8. Scenario Runner Design

All scenario scripts follow this structure:

```bash
#!/usr/bin/env bash
# qa/realams/scenarios/TC-L-01-broadcast-lifecycle.sh
set -euo pipefail
source "$(dirname "$0")/../harness/env.sh"
source "$(dirname "$0")/../harness/auth.sh"
source "$(dirname "$0")/../harness/assert.sh"
source "$(dirname "$0")/../harness/capture.sh"
source "$(dirname "$0")/../harness/publisher.sh"

SCENARIO="TC-L-01"
echo "=== $SCENARIO: Broadcast Lifecycle ==="

# Setup
STREAM_ID="val-tc-l01-$(date +%s)"
capture_ams "/LiveApp/rest/v2/broadcasts/list/0/10" "pre-baseline"
capture_pulse "/live/overview" "pre-baseline"

# Action
start_publisher "$STREAM_ID" "LiveApp" 1000
echo "Waiting 15 s for poll convergence..."
sleep 15

# Assertion
capture_ams "/LiveApp/rest/v2/broadcasts/$STREAM_ID" "broadcasting"
capture_pulse "/live/streams" "broadcasting"

AMS_STATUS=$(jq -r '.status' "$EVIDENCE_DIR/ams-broadcasting"*".json")
assert_eq "$AMS_STATUS" "broadcasting" "AMS status after publish"
compare_viewer_count "$STREAM_ID" "LiveApp"

# Cleanup
stop_publisher "$STREAM_ID"
sleep 15
capture_ams "/LiveApp/rest/v2/broadcasts/$STREAM_ID" "finished"
AMS_STATUS_AFTER=$(jq -r '.status' "$EVIDENCE_DIR/ams-finished"*".json")
assert_eq "$AMS_STATUS_AFTER" "finished" "AMS status after stop"

echo "=== $SCENARIO: COMPLETE ==="
```

---

## 9. Known Constraints and Mitigations

| Constraint | Source | Mitigation |
|-----------|--------|------------|
| AMS MD5 console login (plaintext REST works) | `oguz-testing.md` lines 106–119 | Harness uses plaintext password via curl; no console login |
| AMS brute-force lockout (2 attempts, 5 min, by email) | `oguz-testing.md` lines 121–126 | `auth.sh` called once per session; no retry loop; `admin@` only for harness |
| AMS 3.0.3 webhooks unsigned (fail-closed) | O3 decision, `decisions.md:2404` | Webhook scenarios validated via poll-path timing only |
| Playwright must run in Docker — NOT installed on the VPS host (no browser libs, no sudo) | CI e2e gate rule (D-055), memory note | Every Playwright use (WebRTC viewers §5.2, UI checks) via `mcr.microsoft.com/playwright:v1.61.1-noble` with `--network host` |
| Docker root artifacts in mounted dirs | Memory note | Use `alpine rm -rf` container for cleanup |
| No `sudo` on VPS | Scout B known constraints | `tc`/`iptables` failure injection requires alternative (Docker disconnect) |
| PULSE_AMS_APPLICATIONS may filter apps | Scout B `open_ams_apps` | Harness uses only the 8 open apps; note 403s in verdict |
| per-app `remoteAllowedCIDR` blocks Pulse polling | Scout B note | `LiveApp` and `pulse-test` confirmed open; others may 403 |
| AMS trial license expires 2026-07-12T12:09Z | `local_env.ams_trial_license_expiry` | Operator-waived; observe and report only; note blocked features |
| Max 2 git pushes per session | Memory note | Harness scripts are local; commit evidence separately |
| Docker root artifacts | Memory note | Clean up via `docker run --rm -v <dir>:/s alpine rm -rf /s/<target>` |

---

## 10. Reusability and Regression

To re-run any scenario in a future session:

1. `cd /home/aytek/repo/ams-pulse`
2. Confirm AMS is up: `curl -s http://161.97.172.146:5080/rest/v2/version | jq .`
3. Confirm Pulse is up: `curl -s https://beyondkaira.com/api/v1/healthz`
4. `source qa/realams/harness/env.sh && bash qa/realams/harness/auth.sh`
5. `bash qa/realams/scenarios/<TC-*>.sh`
6. Review `qa/realams/evidence/<timestamp>/verdict.txt`

For full regression:
```bash
make validate-realams        # runs all TC-* scenarios sequentially
make validate-realams-quick  # subset: lifecycle, viewer counts, alerts
```

Makefile targets are to be defined in S17. Evidence packages are
gitignored (`qa/realams/evidence/` in `.gitignore`). Small reference
fixtures (e.g., representative AMS API snapshots, <50 KB) may be
committed to `agents/handoffs/real-ams-captures/` for documentation.
