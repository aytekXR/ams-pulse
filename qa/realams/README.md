# qa/realams — Real-AMS Validation Harness

Runs E2E validation scenarios against the real Ant Media Server (AMS 3.0.3
Enterprise) and either the isolated `pulse-realams` stack (default) or the
production `beyondkaira.com` deployment.

Design spec: `docs/assessment/validation-environment.md`
Scenario matrix: `docs/assessment/scenario-matrix.md`

---

## Directory Layout

```
qa/realams/
  harness/
    env.sh          — exports PULSE_URL, AMS_URL, PULSE_TOKEN, AMS_COOKIE_FILE,
                      EVIDENCE_ROOT.  Source this file first.
    auth.sh         — idempotent AMS cookie-session auth (one attempt, never loops)
    assert.sh       — assert_eq / assert_approx / assert_gte / assert_lte /
                      assert_within / scenario_verdict
    capture.sh      — capture_ams / capture_pulse / compare_viewer_count
    publisher.sh    — start/stop/kill ffmpeg RTMP publishers
    viewer-sim.sh   — HLS and WebRTC viewer simulation
  scenarios/        — one script per TC-* scenario
  evidence/         — GITIGNORED; timestamped JSON + screenshot packages
```

`evidence/` is gitignored. Small reference fixtures (<50 KB) may be
committed to `agents/handoffs/real-ams-captures/` instead.

---

## Prerequisites

1. VPS access with `sg docker` permissions.
2. `deploy/.env` contains:
   - `PULSE_AMS_URL` — AMS base URL (e.g. `http://161.97.172.146:5080`)
   - `PULSE_AMS_LOGIN_EMAIL` — AMS admin email for harness polling account
   - `PULSE_AMS_LOGIN_PASSWORD` — Plaintext password (AMS REST accepts plaintext)
3. For the `realams` target: `pulse-realams-pulse-1` container is running.
4. For the `prod` target: `PULSE_TOKEN` must be set in your shell environment
   (get from `oguz-testing.md` line 159). **NEVER store the token value in any
   committed file.**

---

## Token Handling

### realams target (default)

The token is printed once in the container logs at startup. `env.sh` auto-extracts
it at source time:

```bash
sg docker -c "docker logs pulse-realams-pulse-1 2>&1" | grep -oE 'plt_[a-f0-9]+' | head -1
```

No manual step required.

### prod target

The production Pulse token is **never stored in files**. Retrieve it from
`oguz-testing.md` line 159, set it in your environment, then run:

```bash
PULSE_TOKEN=plt_<redacted> PULSE_TARGET=prod bash qa/realams/scenarios/TC-H-02-healthz.sh
```

If `PULSE_TOKEN` is not set when `PULSE_TARGET=prod`, `env.sh` exits with instructions.

---

## Running a Single Scenario

```bash
cd /home/aytek/repo/ams-pulse

# Default target: realams (isolated stack on 127.0.0.1:18090)
bash qa/realams/scenarios/TC-L-01-broadcast-lifecycle.sh

# Prod target (requires PULSE_TOKEN in env)
PULSE_TOKEN=plt_... PULSE_TARGET=prod bash qa/realams/scenarios/TC-H-02-healthz.sh
```

Each scenario script:
- Sources harness files automatically
- Creates its own timestamped `EVIDENCE_DIR` under `EVIDENCE_ROOT`
- Exits 0 (PASS), 1 (FAIL), or 77 (SKIP — precondition unmet)
- Writes `${EVIDENCE_DIR}/verdict.txt` and `${EVIDENCE_DIR}/timeline.txt`
- Cleans up every container/viewer it created (via `trap cleanup EXIT`)

---

## Running the P0 Suite

```bash
make validate-realams-p0
```

Or manually, in dependency order:

```bash
# Infrastructure checks first
bash qa/realams/scenarios/TC-H-02-healthz.sh
bash qa/realams/scenarios/TC-FL-02-ams-version.sh

# Lifecycle
bash qa/realams/scenarios/TC-L-01-broadcast-lifecycle.sh
bash qa/realams/scenarios/TC-L-02-concurrent-broadcasts.sh
bash qa/realams/scenarios/TC-L-03-publisher-crash.sh

# Viewer counts
bash qa/realams/scenarios/TC-V-01-hls-viewer-single.sh
bash qa/realams/scenarios/TC-V-04-rtmp-viewer-clamp.sh

# Ingest health
bash qa/realams/scenarios/TC-I-01-normal-ingest.sh
bash qa/realams/scenarios/TC-I-02-bitrate-conversion.sh

# Failure scenarios
bash qa/realams/scenarios/TC-F-01-graceful-stop.sh
bash qa/realams/scenarios/TC-F-07-ip-blocked-app.sh
```

---

## Evidence Layout

```
qa/realams/evidence/
  S17-TC-L-01-20260712T143022Z/
    ams-pre-baseline-143022.json       AMS state before action
    ams-pre-baseline-143022.json.headers
    ams-broadcasting-143045.json       AMS state after action
    pulse-pre-baseline-143022.json     Pulse state before action
    pulse-broadcasting-143050.json     Pulse state after action
    compare-vc-val-stream-143055.json  Viewer count parity snapshot
    checks.txt                         PASS/FAIL log from assert.sh
    timeline.txt                       Human-readable event log
    verdict.txt                        PASS/FAIL + failed-check list
  .ams-cookie                          AMS session cookie (never committed)
```

`evidence/` is gitignored at the repo level. If you accidentally see evidence
files staged for commit, run:

```bash
git reset HEAD qa/realams/evidence/
```

---

## AMS Lockout Warning

**AMS allows only 2 failed login attempts before locking the account for 5
minutes, keyed by email address (not IP).** The `admin@` account is also used
by Pulse's production poller — a lockout breaks prod polling.

Rules enforced by `auth.sh`:
- Attempts login **exactly once** per invocation
- Prints a lockout warning and exits 1 on failure
- **NEVER call `auth.sh` in a retry loop**
- Reuses the existing cookie if it is still valid (idempotent)
- Use `admin@` account for harness; use `aytek@` for human console sessions

---

## Cleanup of Root-Owned Evidence Files

Containers sometimes write root-owned files to mounted directories. Because
the VPS user has no `sudo`, use an alpine container to delete them:

```bash
sg docker -c "docker run --rm \
  -v /home/aytek/repo/ams-pulse/qa/realams/evidence:/s \
  alpine rm -rf /s/<target-subdir>"
```

---

## Adding Evidence to gitignore

`qa/realams/evidence/` should be gitignored. If it is not yet in the root
`.gitignore`, add:

```
qa/realams/evidence/
```

Small reference fixtures (<50 KB) — e.g. representative AMS API snapshots —
may be committed to `agents/handoffs/real-ams-captures/` for documentation.

---

## Authoring New Scenarios

1. Copy an existing scenario as a template.
2. Set `SCENARIO="TC-X-NN"` at the top.
3. Source harness files: `env.sh`, `auth.sh`, `assert.sh`, `capture.sh`
   (plus `publisher.sh` / `viewer-sim.sh` as needed).
4. Set `EVIDENCE_DIR="${EVIDENCE_ROOT}/S17-${SCENARIO}-$(date -u +%Y%m%dT%H%M%SZ)"`.
5. Create `EVIDENCE_DIR` with `mkdir -p`.
6. Register a `trap cleanup EXIT` that removes every container / viewer
   the script creates — use unique IDs prefixed `val-` or `pulse-pub-val-`.
7. Use unique stream IDs: `val-<tc>-$(date +%s)`.
8. NEVER assert exact global counts — other streams (e.g. `teststream`) exist.
   Assert per-stream values or before/after deltas.
9. Write `${EVIDENCE_DIR}/timeline.txt` with timestamped events.
10. Call `scenario_verdict` at the end — it writes `verdict.txt` and sets exit
    code 0 (PASS) or 1 (FAIL). Exit 77 for SKIP with a message in `verdict.txt`.
