#!/usr/bin/env bash
# Assemble the Ant Media Marketplace submission ZIP from the repository.
#
#   bash qa/marketplace/build-submission-zip.sh             # full package (for the developer)
#   bash qa/marketplace/build-submission-zip.sh ant-media   # what to send Ant Media
#
# Output (dist/ is gitignored):
#   full       dist/ams-pulse-antmedia-marketplace-submission.zip — everything, including
#              internal/, operator-expected.md and the internal submission notes
#   ant-media  dist/pulse-for-ant-media-server-marketplace-materials.zip — only the shareable
#              material (README-ant-media.md as README.md, page copy, answers, draft review,
#              guides, assets, PDFs, public reference docs); fails if anything internal leaks in
#
# Sources:
#   docs/marketplace/antmedia-submission/   README, operator-expected, marketplace/, assets/, internal/
#   website/                                public site snapshot (tests/ and tools/ excluded)
#   docs/*.md (selected)                    reference documents → documentation/other/
# Generated here:
#   documentation/pdf/                      PDFs rendered by qa/marketplace/tools/*.mjs
#   MANIFEST.sha256                         checksum of every file in the archive
#
# Fails closed on anything that looks like a secret or a dev artifact (see scan_stage).
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MODE="${1:-full}"
case "$MODE" in
  full)      NAME="ams-pulse-antmedia-marketplace-submission" ;;
  ant-media) NAME="pulse-for-ant-media-server-marketplace-materials" ;;
  *) printf '[zip] ERROR: unknown mode %s (full | ant-media)\n' "$MODE" >&2; exit 2 ;;
esac
SRC="$REPO/docs/marketplace/antmedia-submission"
DIST="$REPO/dist"
STAGE="$DIST/stage/$NAME"
ZIP="$DIST/$NAME.zip"
TOOLS="$REPO/qa/marketplace/tools"

log() { printf '[zip] %s\n' "$*"; }
die() { printf '[zip] ERROR: %s\n' "$*" >&2; exit 1; }

[[ -d "$SRC" ]] || die "missing $SRC"
[[ -d "$TOOLS/node_modules/marked" ]] || (cd "$TOOLS" && npm ci --no-audit --no-fund >/dev/null)

rm -rf "$DIST/stage" "$ZIP"
mkdir -p "$STAGE/documentation/pdf" "$STAGE/documentation/other"

# ── 1. Package content ─────────────────────────────────────────────────────────
log "copying package content ($MODE)"
if [[ "$MODE" == full ]]; then
  cp "$SRC/README.md" "$SRC/README-ant-media.md" "$SRC/operator-expected.md" "$STAGE/"
  cp -r "$SRC/marketplace" "$SRC/assets" "$SRC/internal" "$STAGE/"
else
  cp "$SRC/README-ant-media.md" "$STAGE/README.md"
  cp -r "$SRC/marketplace" "$SRC/assets" "$STAGE/"
  # Internal to the developer: test notes with defect detail and the to-do checklist.
  rm -f "$STAGE/marketplace/submission-notes.md" "$STAGE/marketplace/submission-checklist.md"
fi

# ── 2. Website snapshot (no test harness, no generator tooling) — full only ─────
if [[ "$MODE" == full ]]; then
  log "copying website snapshot"
  mkdir -p "$STAGE/website"
  ( cd "$REPO/website" && tar --exclude=./tests --exclude=./tools -cf - . ) | tar -C "$STAGE/website" -xf -
fi

# ── 3. Reference documents from the repository ─────────────────────────────────
REF_DOCS=(docs/known-limitations.md docs/licensing-public.md docs/support.md docs/beacon-sdk.md
          docs/compatibility.md docs/user-guide.md SECURITY.md)
for f in "${REF_DOCS[@]}"; do
  cp "$REPO/$f" "$STAGE/documentation/other/"
done
# The copies keep their repository-relative links, which are dead ends in the ZIP (README.md,
# LICENSE, runbooks …). Point them at the same files on GitHub; links between the copied
# documents stay local.
node - "$REPO" "$STAGE/documentation/other" "${REF_DOCS[@]}" <<'NODE'
const fs = require("fs"), path = require("path");
const [repo, outDir, ...docs] = process.argv.slice(2);
const local = new Map(docs.map((d) => [d, path.basename(d)]));
const BASE = "https://github.com/aytekXR/ams-pulse";
const rewrite = (from, target) => {
  if (/^([a-z][a-z0-9+.-]*:|#)/i.test(target)) return target;          // URL, mailto:, anchor
  const [p, ...rest] = target.split("#");
  const anchor = rest.length ? "#" + rest.join("#") : "";
  const resolved = path.posix.normalize(path.posix.join(path.posix.dirname(from), p));
  if (resolved.startsWith("..")) return target;                         // outside the repo
  if (local.has(resolved)) return local.get(resolved) + anchor;
  if (/\.(png|jpe?g|gif|webp)$/i.test(resolved))                     // images must stay images
    return `https://raw.githubusercontent.com/aytekXR/ams-pulse/main/${resolved}`;
  const isDir = fs.existsSync(path.join(repo, resolved)) && fs.statSync(path.join(repo, resolved)).isDirectory();
  return `${BASE}/${isDir ? "tree" : "blob"}/main/${resolved.replace(/\/$/, "")}${anchor}`;
};
for (const d of docs) {
  const file = path.join(outDir, path.basename(d));
  const src = fs.readFileSync(file, "utf8");
  const out = src
    .replace(/\]\(([^)\s]+)(\s+"[^"]*")?\)/g, (_, t, title = "") => `](${rewrite(d, t)}${title})`)
    .replace(/^(\[[^\]]+\]:\s+)(\S+)/gm, (_, lead, t) => lead + rewrite(d, t));
  fs.writeFileSync(file, out);
}
NODE

# ── 4. PDFs ────────────────────────────────────────────────────────────────────
log "rendering PDFs"
PDFTMP="$DIST/stage/pdf"
mkdir -p "$PDFTMP"
node "$TOOLS/render-docs.mjs" "$PDFTMP" \
  "$SRC/marketplace/product-overview.md=Pulse — Product Overview" \
  "$SRC/marketplace/installation-guide.md=Pulse — Installation Guide" \
  "$SRC/marketplace/answers-for-ant-media.md=Pulse — Answers to Ant Media's open items" \
  "$SRC/marketplace/marketplace-copy.md=Pulse — Marketplace page copy" >/dev/null
cp "$PDFTMP/product-overview.pdf"       "$STAGE/documentation/pdf/pulse-product-overview.pdf"
cp "$PDFTMP/installation-guide.pdf"     "$STAGE/documentation/pdf/pulse-installation-guide.pdf"
cp "$PDFTMP/answers-for-ant-media.pdf"  "$STAGE/documentation/pdf/pulse-answers-for-ant-media.pdf"
cp "$PDFTMP/marketplace-copy.pdf"       "$STAGE/documentation/pdf/pulse-marketplace-page-copy.pdf"
node "$TOOLS/build-asset-sheet.mjs" "$SRC" "$STAGE/documentation/pdf/pulse-marketplace-asset-sheet.pdf" >/dev/null

# ── 5. Fail-closed hygiene scan ────────────────────────────────────────────────
scan_stage() {
  local bad=0
  # Files that must never ship.
  if find "$STAGE" \( -name '.env' -o -name '.env.*' -o -name '*.pem' -o -name '*.key' -o -name '*.p8' \
       -o -name 'node_modules' -o -name '.git' -o -name '.state' -o -name '*.db' -o -name '.DS_Store' \) \
       -print | grep -q .; then
    find "$STAGE" \( -name '.env' -o -name '.env.*' -o -name '*.pem' -o -name '*.key' -o -name '*.p8' \
         -o -name 'node_modules' -o -name '.git' -o -name '.state' -o -name '*.db' -o -name '.DS_Store' \) -print >&2
    bad=1
  fi
  # Secret-shaped content in text files: Pulse tokens, 64-hex keys, private keys, license keys
  # (Pulse's and Ant Media's: AMS + 30 hex), the demo stack's literal password.
  local pattern='plt_[a-f0-9]{16,}|(PULSE_SECRET_KEY|PULSE_LICENSE_KEY|WEBHOOK_SECRET)=[A-Za-z0-9+/=]{16,}|-----BEGIN [A-Z ]*PRIVATE KEY-----|ghp_[A-Za-z0-9]{20,}|AKIA[0-9A-Z]{16}|AMS[0-9a-f]{28,}|demo-password'
  if grep -rIlE "$pattern" "$STAGE" >/dev/null 2>&1; then
    grep -rInE "$pattern" "$STAGE" | cut -c1-200 >&2
    bad=1
  fi
  # Public IPv4 addresses (the build host's or anyone's). Loopback, RFC 1918, 0.0.0.0 and the
  # RFC 5737 documentation ranges are fine.
  local ips
  ips=$(grep -rIhoE '\b([0-9]{1,3}\.){3}[0-9]{1,3}\b' "$STAGE" \
    | grep -vE '^(127\.|10\.|192\.168\.|172\.(1[6-9]|2[0-9]|3[01])\.|0\.0\.0\.0$|192\.0\.2\.|198\.51\.100\.|203\.0\.113\.)' \
    | sort -u || true)
  if [[ -n "$ips" ]]; then
    while IFS= read -r ip; do
      printf 'public IPv4 address %s in:\n' "$ip" >&2
      grep -rIlF "$ip" "$STAGE" >&2 || true
    done <<< "$ips"
    bad=1
  fi
  # Unfilled template placeholders (a WEB_GATES_RESULT once shipped in the submission notes).
  # Allowed by name: TESTFLIGHT_PUBLIC_LINK_PLACEHOLDER, the documented fill-in point on the
  # website's /beta/ page that waits on Apple enrolment.
  local placeholder='\b(PENDING_[A-Z][A-Z_]{2,}|[A-Z]{2,}(_[A-Z]{2,})*_(RESULT|RESULTS|PLACEHOLDER))\b'
  local residue
  residue=$(grep -rInE "$placeholder" "$STAGE" 2>/dev/null | grep -v 'TESTFLIGHT_PUBLIC_LINK_PLACEHOLDER' || true)
  if [[ -n "$residue" ]]; then
    printf '%s\n' "$residue" | cut -c1-200 >&2
    bad=1
  fi
  # Absolute paths from the build host.
  if grep -rIl -e '/home/aytek' -e '/tmp/claude-' "$STAGE" >/dev/null 2>&1; then
    grep -rIn -e '/home/aytek' -e '/tmp/claude-' "$STAGE" | cut -c1-200 >&2
    bad=1
  fi
  # The Ant Media copy must carry nothing internal — no files, and no links to them
  # (a link to a file that is not in the ZIP is a dead end for the reader).
  if [[ "$MODE" == ant-media ]]; then
    if find "$STAGE" \( -name internal -o -name operator-expected.md -o -name submission-notes.md \
         -o -name submission-checklist.md -o -name website \) -print | grep -q .; then
      find "$STAGE" \( -name internal -o -name operator-expected.md -o -name submission-notes.md \
           -o -name submission-checklist.md -o -name website \) -print >&2
      bad=1
    fi
    if grep -rIlE 'operator-expected\.md|submission-notes\.md|submission-checklist\.md|internal/evidence' \
         "$STAGE" --include='*.md' >/dev/null 2>&1; then
      grep -rInE 'operator-expected\.md|submission-notes\.md|submission-checklist\.md|internal/evidence' \
        "$STAGE" --include='*.md' | cut -c1-200 >&2
      bad=1
    fi
  fi
  return "$bad"
}
log "scanning for secrets and dev artifacts"
scan_stage || die "hygiene scan failed (see above) — nothing was zipped"

# ── 6. Manifest + archive ──────────────────────────────────────────────────────
( cd "$STAGE" && find . -type f -print0 | sort -z | xargs -0 sha256sum ) > "$DIST/stage/MANIFEST.sha256"
mv "$DIST/stage/MANIFEST.sha256" "$STAGE/MANIFEST.sha256"
( cd "$DIST/stage" && zip -q -r -X "$ZIP" "$NAME" )
rm -rf "$DIST/stage"

log "wrote $ZIP ($(du -h "$ZIP" | cut -f1), $(unzip -Z1 "$ZIP" | grep -vc '/$') files)"
