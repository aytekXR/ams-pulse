# Real-AMS Go-Live Runbook

> **HISTORICAL — this runbook describes the D-031/D-062 go-live (2026-06-21) that
> swapped `beyondkaira.com` from MOCK-AMS + seeded demo data to the live
> `test.antmedia.io`. That swap is complete (SESSION-05, D-062). For current
> production operations, use the 5-overlay commands in
> `docs/runbooks/productionize.md`. Sections 0–7 below are kept for provenance.**
>
> **Note on cross-references:** this file has sections 0–7 only. References to
> "§8" (post-swap smoke) should be read as §4; "§14" (5-overlay DC command) should
> be read as section 3 below or the Quick Reference in `docs/runbooks/productionize.md`.

**Target:** swap `beyondkaira.com` from MOCK-AMS + seeded demo data to the live
`test.antmedia.io` (AMS 3.0.3 Enterprise).
**Branch:** `ams-integration` (contains the D-030 wire fixes + D-031 deploy-prep fixes).
**Authored:** 2026-06-21 (D-031, from the `realams-deploy-readiness` workflow).
**Verdict:** ready-with-mitigations. The mechanics are validated; the items below
must be handled at deploy time. ⛔ **Founder-visible — execute only on operator GO.**

---

## 0. What the founder will actually see (set expectations FIRST)

`test.antmedia.io` is a **single-node** AMS with **one** live stream right now
(`LiveApp/test123`, an RTSP camera at ~624 kbps, 0 viewers). After the swap the
dashboard honestly shows:

| Surface | Real-AMS result | Is it a bug? |
|---|---|---|
| Live overview | `publishers: 1`, `viewers: 0` | No — real state |
| `test123` bitrate | ~624 kbps (was 624016 before D-030) | No — fixed |
| `test123` health | "Warning" at default target 2000 kbps; "Good" if target lowered (see §2 decision) | No — honest, tunable |
| Fleet / nodes | empty ("no nodes") | No — standalone AMS, `cluster/nodes` 404 |
| CPU / RAM cards | absent | No — AMS standalone exposes no per-node sys metrics |
| Viewer QoE page | empty state | No — needs beacon-js SDK in the player (next step) |
| WebRTC viewer stats | empty even with viewers | Known deferred gap (D-031 backlog) |

These empty states are **correct**, not failures — but prepare talking points (§6).

---

## 1. Phase 0 — code fixes (✅ DONE this session, verify in build)

Both shipped on `ams-integration` and are included automatically by `--build pulse`:

- **`maskDSN` no-op → ClickHouse password leaked to logs.** Fixed in
  `server/cmd/pulse/migrate.go` (uses `url.URL.Redacted()` → password renders as
  `xxxxx`). Regression test `TestMaskDSN` in `server/cmd/pulse/migrate_test.go`.
- **Broken SDK-docs CTA** (`your-org` placeholder 404). Fixed in
  `web/src/features/qoe/QoePage.tsx` → `https://github.com/aytekXR/ams-pulse#sdk-setup`.

No further code changes are required for the swap.

---

## 2. Operator decisions (make these BEFORE running §3)

1. **Wipe ClickHouse? (recommended: YES).** Prod ClickHouse holds ~1.05M seeded
   demo rows (fake stream IDs: `concert-mainstage`, `sports-hd-soccer`, …). The
   audience/QoE/geo analytics endpoints have **no node filter**, so they would
   aggregate fake + real data and show e.g. "thousands of viewers" next to a real
   1-publisher/0-viewer stream. Rollup tables are already empty (no populated
   historical charts to lose). Wiping = honest history from t=0. Commands in §3-C.
2. **Bitrate health target.** `test123` is ~624 kbps. With the default
   `PULSE_INGEST_TARGET_BITRATE_KBPS=2000` it scores "Warning"; set it to `600`
   and it reads "Good" (100/100). This is a per-deployment threshold, not a
   formula override — choose the value that honestly reflects the source profile
   (an RTSP camera pull is legitimately low-bitrate). Set it in `deploy/.env` (§3-A).
3. **Demo viewer (optional).** Open `https://test.antmedia.io/LiveApp/streams/test123.m3u8`
   in a tab before the call to populate the protocol-mix donut with 1 HLS viewer.
4. **Merge `ams-integration` → `main`.** Not required (the deploy builds from the
   checked-out tree), but good hygiene — operator's call on timing.

---

## 3. Phase 1–3 — deploy

```bash
cd /home/aytek/repo/ams-pulse
# Real-AMS compose set:
export DC="-p pulse-prod -f deploy/docker-compose.yml -f deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.real-ams.yml -f deploy/docker-compose.backup.yml --env-file deploy/.env"
# Mock-ams (rollback) compose set — omits real-ams overlay but keeps backup sidecar:
export DC_MOCK="-p pulse-prod -f deploy/docker-compose.yml -f deploy/docker-compose.hardened.yml -f deploy/docker-compose.prod-tls.yml -f deploy/docker-compose.backup.yml --env-file deploy/.env"
```

### 3-A. Pre-flight (all must pass)

```bash
git branch --show-current                              # expect: ams-integration
git status -s                                          # expect: clean
file deploy/.env | grep -c CRLF                        # expect: 0  (else: dos2unix deploy/.env)
grep -E 'PULSE_AMS_URL|PULSE_INGEST_TARGET_BITRATE_KBPS' deploy/.env   # add target per §2.2 if desired
sg docker -c "docker compose $DC config -q" && echo CONFIG_OK         # must exit 0
# Prove the VPS can cookie-login to the real server:
EMAIL=$(grep '^PULSE_AMS_LOGIN_EMAIL=' deploy/.env | cut -d= -f2-)
PASS=$(grep '^PULSE_AMS_LOGIN_PASSWORD=' deploy/.env | cut -d= -f2-)
curl -s -m 15 -X POST https://test.antmedia.io/rest/v2/users/authenticate \
  -H 'Content-Type: application/json' \
  -d "$(printf '{"email":"%s","password":"%s"}' "$EMAIL" "$PASS")" | grep -q '"success":true' \
  && echo LOGIN_OK || echo "LOGIN_FAILED — abort"
```

Record the existing admin token (it survives the swap — persisted in the
`pulse-prod_pulse-data` volume): `sg docker -c "docker compose $DC_MOCK logs pulse" | grep -oE 'plt_[A-Za-z0-9]+' | tail -1`.
The exact value + the demo sidecar details are in `oguz-testing.md` (gitignored).
**Never run `down -v` on `pulse-prod_pulse-data`** — that destroys the token.

### 3-B. Stop the seeded-demo liveness sidecar (CRITICAL — not Compose-managed)

```bash
sg docker -c "docker stop pulse-demo-liveness && docker rm pulse-demo-liveness"
sg docker -c "docker ps --filter name=pulse-demo-liveness"   # expect: empty
```

### 3-C. Wipe ClickHouse (only if §2.1 = YES)

```bash
sg docker -c "docker compose $DC_MOCK stop pulse mock-ams clickhouse"
sg docker -c "docker volume rm pulse-prod_clickhouse-data"      # destroys seeded analytics ONLY
sg docker -c "docker compose $DC_MOCK up -d clickhouse"
# wait for healthy:
sg docker -c "docker compose $DC_MOCK ps clickhouse"            # expect: healthy
sg docker -c "docker compose $DC run --rm pulse-migrate"        # applies 0001_init / 0002_concurrency_rollup / 0003_probe_segment_ttfb
```

### 3-D. Deploy pulse wired to real AMS

Stamped-build procedure (D-058): build with explicit ARGs first, then `up -d` WITHOUT
`--build`. Mixing `--build` into `up -d` rebuilds in-place and loses the
`VERSION`/`COMMIT`/`BUILD_DATE` stamps.

```bash
# Build with explicit stamps FIRST:
sg docker -c "docker compose $DC build \
  --build-arg VERSION=$(git describe --tags --always) \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  --build-arg BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  pulse"
# Then start WITHOUT --build — uses the pre-built stamped image:
sg docker -c "docker compose $DC up -d pulse"
```

Profiles out mock-ams, wires the `PULSE_AMS_*` env. ClickHouse, Caddy and the
`pulse-prod_pulse-data` volume (token, alert rules) are untouched.

---

## 4. Phase 4 — post-swap verification (run within ~1 min; allow 2 poll cycles)

```bash
TOK=$(sg docker -c "docker compose $DC logs pulse" | grep -oE 'plt_[A-Za-z0-9]+' | tail -1)
# Health (VPS local DNS is stale — always --resolve):
curl -sS --resolve beyondkaira.com:443:161.97.172.146 https://beyondkaira.com/healthz   # {"status":"ok"}
# Real publishers:
curl -s --resolve beyondkaira.com:443:161.97.172.146 https://beyondkaira.com/api/v1/live/overview -H "Authorization: Bearer $TOK"
#   expect: total_publishers:1, apps:[{app:LiveApp,...}]
# Bitrate sane (NOT 624016):
curl -s --resolve beyondkaira.com:443:161.97.172.146 https://beyondkaira.com/api/v1/live/streams -H "Authorization: Bearer $TOK"
#   expect items[].bitrate_kbps ≈ 624   (if 624016 → D-030 not in the image; rebuild)
# No auth/decode errors:
sg docker -c "docker compose $DC logs pulse --tail 80" | grep -iE '401|403|decode|unmarshal|login failed'   # expect: empty
# Password masked in migrate logs (Phase-0 maskDSN fix):
sg docker -c "docker compose $DC logs pulse-migrate" | grep -E 'clickhouse://' | grep -c xxxxx   # expect: ≥1, and NO plaintext password
```

Browser smoke (private window, `https://beyondkaira.com`): Publishers=1; one stream
row `test123`; Fleet shows the empty state (correct for standalone); Viewer-QoE
empty-state link points at `aytekXR/ams-pulse` (not `your-org`).

---

## 5. Rollback (fast, no volume loss)

```bash
# Re-point pulse at mock-ams (drop the real-ams overlay; NEVER add -v).
# Build without --build mixed into up -d (D-058 stamped-build):
sg docker -c "docker compose $DC_MOCK build \
  --build-arg VERSION=$(git describe --tags --always) \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  --build-arg BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  pulse"
sg docker -c "docker compose $DC_MOCK up -d pulse"
# Restore the seeded-demo liveness sidecar — exact run command is in oguz-testing.md (gitignored).
# Verify demo data returns within ~15 s:
curl -s --resolve beyondkaira.com:443:161.97.172.146 https://beyondkaira.com/api/v1/live/overview -H "Authorization: Bearer $TOK"
#   expect: total_viewers ~13, total_publishers ~8
```

Note: if ClickHouse was wiped (§3-C), the seeded *history* is gone; the liveness
sidecar regenerates live demo numbers but historical charts restart from t=0.

---

## 6. Talking points for the known empty states

| What Oğuz sees | Honest explanation |
|---|---|
| Fleet "no nodes" / no CPU·RAM cards | `test.antmedia.io` is a single-node AMS; cluster + per-node sys metrics require AMS cluster mode (`cluster/nodes` returns 404) |
| Viewer QoE empty | beacon-js SDK must be embedded in the player — that's the next integration step |
| WebRTC viewer stats empty even with viewers | Known deferred gap — aggregator has no `EventWebRTCClientStats` case yet (D-031 backlog) |

---

## 7. Post-demo follow-up (D-031 backlog)

1. **Standalone node card** — on `cluster/nodes` 404, render a synthetic single-node
   card from `/rest/v2/system-status` (`processorCount`, `osName`) so CPU/RAM/Fleet
   aren't blank for single-node AMS.
2. **`EventWebRTCClientStats` aggregator case** — add the missing `case` in
   `server/internal/collector/aggregator/aggregator.go` so per-viewer RTT/jitter/loss
   surface (decide viewer-side vs ingest-side field ownership).
3. **Surface AMS version** (`3.0.3 Enterprise`) on the source/fleet view.
4. **Merge `ams-integration` → `main`** + remove the vestigial `AMS_LOGIN_EMAIL`/
   `AMS_LOGIN_PASSWORD` lines from `deploy/.env` (not read by any compose service).
5. **B7** (per-source webhook secret, contract CR) + **B3** (Docker secrets) — also
   add a Caddy `/webhook/*` route if low-latency webhook ingest is wanted (polling
   works without it).
