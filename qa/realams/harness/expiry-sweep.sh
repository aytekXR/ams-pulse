#!/usr/bin/env bash
# qa/realams/harness/expiry-sweep.sh — phase-labeled read-only AMS + Pulse sweep.
#
# Purpose (S21/D-083): capture an identical, diff-friendly snapshot of AMS and
# Pulse health before and after the AMS trial-license lapse (2026-07-12T12:09Z),
# so the post-expiry delta is a plain diff of two stable.txt files.
#
#   usage: bash qa/realams/harness/expiry-sweep.sh <phase>
#          phase: preexpiry | postexpiry | any label (goes into the evidence dir name)
#
#   diff:  diff <pre-dir>/stable.txt <post-dir>/stable.txt
#
# S21's pre-expiry baseline: qa/realams/evidence/S21-sweep-preexpiry-20260712T014135Z
# (evidence/ is gitignored; the dir persists on this VPS).
#
# READ-ONLY against AMS and Pulse. Lockout-safe: auth.sh reuses the cookie and
# attempts login at most ONCE (2 failed tries = 5-min email-keyed lock, and
# admin@ is prod's polling account — NEVER retry).
set -euo pipefail

PHASE="${1:?usage: expiry-sweep.sh <phase-label>}"
HARNESS="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TS="$(date -u +%Y%m%dT%H%M%SZ)"

# shellcheck source=./env.sh
source "${HARNESS}/env.sh"
# shellcheck source=./auth.sh
source "${HARNESS}/auth.sh"

EV="${EVIDENCE_ROOT}/S21-sweep-${PHASE}-${TS}"
mkdir -p "$EV"
STABLE="${EV}/stable.txt"

note() { echo "$1" | tee -a "$STABLE"; }
# authed GET: writes body to $2, echoes http code
aget() {
  curl -s -m 15 -b "$AMS_COOKIE_FILE" -o "$2" -w "%{http_code}" "$1" || echo "000"
}

echo "# S21 sweep phase=${PHASE} ts=${TS} (header file excluded from diff)" > "${EV}/header.txt"

# ── 1. Server-scope authed endpoints ─────────────────────────────────────────
C="$(aget "${AMS_URL}/rest/v2/version" "${EV}/version.json")"
note "version.http=${C}"
note "version.body=$(cat "${EV}/version.json")"

C="$(aget "${AMS_URL}/rest/v2/applications" "${EV}/applications.json")"
note "applications.http=${C}"
note "applications.body=$(cat "${EV}/applications.json")"

C="$(aget "${AMS_URL}/rest/v2/licence-status" "${EV}/licence-status.json")"
note "licence-status.http=${C}"
note "licence-status.body=$(head -c 400 "${EV}/licence-status.json")"

C="$(aget "${AMS_URL}/rest/v2/cluster/nodes" "${EV}/cluster-nodes.json")"
note "cluster-nodes.http=${C}"

C="$(aget "${AMS_URL}/rest/v2/system-status" "${EV}/system-status.json")"
note "system-status.http=${C}"

# ── 2. Per-app settings + app-scope REST ─────────────────────────────────────
for APP in LiveApp WebRTCAppEE live pulse-test; do
  C="$(aget "${AMS_URL}/rest/v2/applications/settings/${APP}" "${EV}/settings-${APP}.json")"
  CIDR="$(jq -r '.remoteAllowedCIDR // "n/a"' "${EV}/settings-${APP}.json" 2>/dev/null || echo parse-err)"
  note "settings.${APP}.http=${C} remoteAllowedCIDR=${CIDR}"

  C="$(aget "${AMS_URL}/${APP}/rest/v2/broadcasts/count" "${EV}/bcount-${APP}.json")"
  note "broadcasts-count.${APP}.http=${C} body=$(head -c 200 "${EV}/bcount-${APP}.json")"
done

# ── 3. Active stream + HLS serving ───────────────────────────────────────────
C="$(aget "${AMS_URL}/LiveApp/rest/v2/broadcasts/list/0/50" "${EV}/blist-LiveApp.json")"
note "broadcasts-list.LiveApp.http=${C}"
SID="$(jq -r '[.[] | select(.status=="broadcasting")][0].streamId // empty' "${EV}/blist-LiveApp.json" 2>/dev/null || true)"
if [ -n "$SID" ]; then
  # The stream id is volatile — record only presence in stable.txt.
  HC="$(curl -s -m 15 -o "${EV}/hls-manifest.m3u8" -w "%{http_code}" "${AMS_URL}/LiveApp/streams/${SID}.m3u8" || echo "000")"
  note "hls-live-manifest.http=${HC} (a broadcasting stream existed)"
  echo "hls stream id: ${SID}" >> "${EV}/header.txt"
else
  note "hls-live-manifest.http=SKIP (no broadcasting stream on LiveApp)"
fi

# ── 4. Pulse side: prod health + realams polling liveness ────────────────────
PRODH="$(curl -s -m 10 https://beyondkaira.com/healthz | jq -c '{status, ch: .components.clickhouse.status, col: .components.collector.status, meta: .components.meta_store.status}' 2>/dev/null || echo unreachable)"
note "pulse-prod.healthz=${PRODH}"

# PULSE_TOKEN + PULSE_URL come from env.sh (realams auto-extract by default).
OV="$(curl -s -m 10 -H "Authorization: Bearer ${PULSE_TOKEN}" "${PULSE_URL}/live/overview" 2>/dev/null || echo unreachable)"
echo "$OV" > "${EV}/realams-overview.json"
PUBS="$(echo "$OV" | jq -r '.total_publishers // "parse-err"' 2>/dev/null || echo parse-err)"
note "pulse-realams.overview.total_publishers=${PUBS}"

# Prod poll errors in the last 15 min (count only — a stable-ish signal).
PERR="$(sg docker -c "docker logs --since 15m pulse-prod-pulse-1 2>&1" | grep -ciE "poll.*(error|fail)|401|403" || true)"
note "pulse-prod.poll-errlines-15m=${PERR}"

echo "---"
echo "evidence: ${EV}"
echo "stable summary:"
cat "$STABLE"
