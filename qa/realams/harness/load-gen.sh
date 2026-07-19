#!/usr/bin/env bash
# qa/realams/harness/load-gen.sh
#
# Load-generator abstraction for the opt-in load lane.
# SOURCE after load-env.sh + publisher.sh.
#
#   LOAD_GENERATOR=native   (default) — repo primitives (start_bulk_publishers);
#                                       zero downloads, synthetic testsrc2/sine.
#   LOAD_GENERATOR=official           — Ant Media's official Scripts
#                                       (load-testing/rtmp_publisher.sh); speaks
#                                       Ant Media's toolkit vocabulary for the
#                                       marketplace submission. Needs LOAD_MEDIA
#                                       and network access to GitHub.
#
# Both modes give every stream an ID that begins with the RUN prefix
#   native:   ${RUN}0001 .. ${RUN}NNNN
#   official: ${RUN}_1   .. ${RUN}_N
# so parity assertions can count by startswith(RUN) regardless of mode.
#
[[ -n "${_LOAD_GEN_SH_LOADED:-}" ]] && return 0
_LOAD_GEN_SH_LOADED=1
set -euo pipefail

_LOAD_TOOLS_DIR="${REPO_ROOT}/qa/realams/load/tools"

# ── _lg_fetch_official NAME → prints path, non-zero on failure ────────────────
_lg_fetch_official() {
  local name="$1"
  local dest="${_LOAD_TOOLS_DIR}/${name}"
  mkdir -p "${_LOAD_TOOLS_DIR}"
  if [ ! -x "${dest}" ]; then
    curl -fsSL -o "${dest}" \
      "https://raw.githubusercontent.com/ant-media/Scripts/master/load-testing/${name}" \
      2>/dev/null || return 1
    chmod +x "${dest}"
  fi
  printf '%s' "${dest}"
}

# ── load_start_publishers RUN COUNT ───────────────────────────────────────────
# Starts COUNT detached publishers. Returns non-zero (3) if the official
# generator cannot start (missing media/tool) so the caller can SKIP(77).
load_start_publishers() {
  local RUN="$1" COUNT="$2"
  if [ "${LOAD_GENERATOR:-native}" = "official" ]; then
    [ -f "${LOAD_MEDIA}" ] || { echo "[load-gen] LOAD_MEDIA missing (${LOAD_MEDIA})" >&2; return 3; }
    local pub
    pub="$(_lg_fetch_official rtmp_publisher.sh)" \
      || { echo "[load-gen] cannot fetch official rtmp_publisher.sh" >&2; return 3; }
    echo "[load-gen] official rtmp_publisher.sh ×${COUNT} → rtmp://${LOAD_AMS_HOST}:1935/${LOAD_APP}/${RUN}" >&2
    bash "${pub}" "${LOAD_MEDIA}" "rtmp://${LOAD_AMS_HOST}:1935/${LOAD_APP}/${RUN}" "${COUNT}" \
      >/dev/null 2>&1 &
  else
    echo "[load-gen] native start_bulk_publishers ×${COUNT} (prefix=${RUN}, ${LOAD_PUB_KBPS}k)" >&2
    start_bulk_publishers "${COUNT}" "${LOAD_APP}" "${RUN}" "${LOAD_PUB_KBPS}"
  fi
}

# ── load_stop_publishers RUN COUNT ────────────────────────────────────────────
load_stop_publishers() {
  local RUN="$1" COUNT="${2:-0}" i
  if [ "${LOAD_GENERATOR:-native}" = "official" ]; then
    # Match on the unique, regex-metachar-free RUN token (val-load-<hex>) only — NOT the
    # full rtmp URL: a dotted IP in LOAD_AMS_HOST or a metachar in LOAD_APP would be treated
    # as ERE wildcards by pkill -f and over-match unrelated processes.
    pkill -f "${RUN}" 2>/dev/null || true
  else
    for i in $(seq 1 "${COUNT}"); do
      stop_publisher "$(printf '%s%04d' "${RUN}" "${i}")" 2>/dev/null || true
    done
  fi
}

# ── load_count_ams_broadcasting RUN → count of RUN* streams broadcasting ──────
load_count_ams_broadcasting() {
  local RUN="$1"
  curl -s -m 20 -b "${AMS_COOKIE_FILE}" \
    "${AMS_URL}/${LOAD_APP}/rest/v2/broadcasts/list/0/500" 2>/dev/null \
    | jq --arg p "${RUN}" \
      '[.[] | select(.streamId | startswith($p)) | select(.status == "broadcasting")] | length' \
      2>/dev/null || echo 0
}
