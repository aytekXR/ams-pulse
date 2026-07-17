#!/usr/bin/env bash
# qa/tools/ams-drift-watch.sh — gap G-26.
#
# Fail (exit 1) when an Ant Media Server release tag newer than the pinned
# version appears, so the AMS-integration findings (publishType/bitrate units,
# webhook actions, REST paths, DTO semantics) can be re-verified against the new
# source before Pulse claims compatibility with it.
#
# Pinned version is the AMS release Pulse is validated against. Bump PINNED after
# re-verifying findings against the newer tag.
#
# Exit: 0 = no newer tag (or pinned is newest); 1 = drift detected / lookup failed.
# Intended for a nightly CI job (network egress to github.com required).
set -euo pipefail

PINNED="${AMS_PINNED_TAG:-ams-v3.0.3}"
REPO_URL="https://github.com/ant-media/Ant-Media-Server.git"

# Newest ams-v* tag by version sort, excluding dereferenced ^{} entries.
LATEST="$(git ls-remote --tags "$REPO_URL" 2>/dev/null \
  | sed 's|.*refs/tags/||' \
  | grep -E '^ams-v[0-9]' \
  | grep -v '\^{}' \
  | sort -V \
  | tail -1)"

if [ -z "$LATEST" ]; then
  echo "ams-drift-watch: could not read tags from $REPO_URL (network?)" >&2
  exit 1
fi

if [ "$LATEST" = "$PINNED" ]; then
  echo "ams-drift-watch: OK — pinned=$PINNED is the newest ams-v* tag"
  exit 0
fi

# sort -V puts the newer tag last; if PINNED is already newest, LATEST==PINNED
# above. Reaching here means LATEST differs — confirm it is actually newer.
NEWEST="$(printf '%s\n%s\n' "$PINNED" "$LATEST" | sort -V | tail -1)"
if [ "$NEWEST" = "$PINNED" ]; then
  echo "ams-drift-watch: OK — pinned=$PINNED is >= latest observed $LATEST"
  exit 0
fi

cat >&2 <<EOF
ams-drift-watch: DRIFT — a newer Ant Media Server tag exists.
  pinned = $PINNED
  latest = $LATEST
Action: re-verify the AMS-integration findings (webhook actions/signing/content-type,
REST paths Pulse consumes, BroadcastDTO/VoD field semantics) against $LATEST, run the
AMS version-matrix tests, then bump AMS_PINNED_TAG (and the test-design docs) if clean.
EOF
exit 1
