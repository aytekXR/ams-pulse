#!/usr/bin/env bash
#
# build-api-docs.sh — regenerate docs/api/index.html from the OpenAPI contract.
#
# WHY THIS SCRIPT EXISTS INSTEAD OF A BARE `redocly build-docs` CALL
# -------------------------------------------------------------------
# `redocly build-docs` emits HTML whose <script src> points at
# cdn.redocly.com. docs/api-guide.md advertises this file as a "self-contained"
# reference, it is listed in the marketplace submission package as the API
# reference a reviewer opens, and this repo has a standing rule against external
# CDNs (the public website has a check enforcing zero external requests). All
# three were quietly false: opening the API reference pinged a third-party CDN,
# and opening it offline produced a blank page.
#
# So this script builds the docs and then INLINES the ReDoc bundle, pinned to an
# exact version. Anyone who regenerates with a bare redocly call will silently
# reintroduce the CDN reference — the verification step below is what catches
# that, so keep using this script.
#
# ReDoc (community edition) is MIT-licensed. The bundle's own licence banner is
# preserved verbatim in the inlined output.
#
# USAGE:  scripts/build-api-docs.sh [--check]
#           --check  verify the committed file is current and CDN-free; make no
#                    changes. Exits non-zero if it is stale or references a CDN.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

SPEC="contracts/openapi/pulse-api.yaml"
OUT="docs/api/index.html"
# Pinned deliberately: an unpinned bundle would make the generated file
# non-reproducible and could change the docs without a contract change.
REDOC_VERSION="v2.5.3"
REDOC_URL="https://cdn.redocly.com/redoc/${REDOC_VERSION}/bundles/redoc.standalone.js"

CHECK_ONLY=0
[ "${1:-}" = "--check" ] && CHECK_ONLY=1

[ -f "$SPEC" ] || {
	echo "ERROR: spec not found: $SPEC" >&2
	exit 1
}

TMPDIR_BUILD="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_BUILD"' EXIT

echo "==> building ReDoc HTML from $SPEC"
# --disableGoogleFont matters: without it the emitted page pulls Montserrat and
# Roboto from fonts.googleapis.com. This repo self-hosts fonts and never uses a
# font CDN, and a customer opening our API reference should not be reported to
# Google.
npx --yes @redocly/cli build-docs "$SPEC" --disableGoogleFont -o "$TMPDIR_BUILD/raw.html" >/dev/null

echo "==> fetching pinned ReDoc bundle ($REDOC_VERSION)"
curl -fsS -m 120 -o "$TMPDIR_BUILD/redoc.js" "$REDOC_URL"

echo "==> inlining the bundle (verified against redocly's own SRI hash)"
python3 - "$TMPDIR_BUILD/raw.html" "$TMPDIR_BUILD/redoc.js" "$TMPDIR_BUILD/out.html" "$REDOC_URL" <<'PY'
import base64
import hashlib
import re
import sys

raw_path, js_path, out_path, url = sys.argv[1:5]
html = open(raw_path, encoding="utf-8").read()
js_bytes = open(js_path, "rb").read()

# Match the whole <script ...></script> element regardless of attribute order:
# redocly emits src, integrity and crossorigin, and that set has changed before.
pattern = re.compile(
    r'<script\b[^>]*\bsrc="%s"[^>]*>\s*</script>' % re.escape(url)
)
m = pattern.search(html)
if not m:
    sys.exit(
        "ERROR: could not find the ReDoc <script src> tag to replace.\n"
        "redocly's output template changed — inspect it and update this script;\n"
        "do NOT ship the file with the CDN reference intact."
    )

tag = m.group(0)

# Supply-chain check: redocly puts an SRI hash on the tag it generates. Verify
# the bytes we downloaded against it, so we inline exactly the bundle redocly
# intended and a compromised or truncated CDN response cannot be baked in.
sri = re.search(r'integrity="sha(256|384|512)-([A-Za-z0-9+/=]+)"', tag)
if sri:
    algo, expected = "sha" + sri.group(1), sri.group(2)
    actual = base64.b64encode(hashlib.new(algo, js_bytes).digest()).decode()
    if actual != expected:
        sys.exit(
            "ERROR: downloaded ReDoc bundle FAILS the SRI hash redocly emitted.\n"
            "  expected %s-%s\n  actual   %s-%s\n"
            "Refusing to inline it." % (algo, expected, algo, actual)
        )
    print("    SRI %s verified (%d bytes)" % (algo, len(js_bytes)))
else:
    # No integrity attribute to check against: fall back to a size floor so a
    # CDN error page cannot be inlined silently.
    if len(js_bytes) < 500000:
        sys.exit(
            "ERROR: no SRI attribute and the bundle is only %d bytes — "
            "refusing to inline a probably-truncated file." % len(js_bytes)
        )
    print("    WARNING: no integrity attribute on the tag; inlined on size check only")

js = js_bytes.decode("utf-8")

# The bundle hardcodes the Redocly attribution logo as a CDN URL and injects it
# as an <img> at RUNTIME, so it is invisible to any check that greps the HTML
# for tags — the static check passed while the browser still fetched it. Replace
# the URL with an inline data: URI of the same mark.
#
# The SRI verification above has already run against the untouched bytes, so
# this edit cannot mask a compromised download.
LOGO_URL = "https://cdn.redoc.ly/redoc/logo-mini.svg"
if LOGO_URL in js:
    logo_svg = (
        '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 25 22" width="25" height="22">'
        '<path fill="#1f3d5c" d="M12.5 0 25 6.4v9.2L12.5 22 0 15.6V6.4z"/>'
        '<path fill="#fff" d="M12.5 4.6 20 8.5v5L12.5 17.4 5 13.5v-5z"/>'
        "</svg>"
    )
    data_uri = "data:image/svg+xml;base64," + base64.b64encode(
        logo_svg.encode("utf-8")
    ).decode()
    js = js.replace(LOGO_URL, data_uri)
    print("    inlined the Redocly attribution logo (was a runtime CDN fetch)")

# Guard against the bundle containing a literal closing script tag, which would
# terminate the inline block early and produce a silently broken page.
js = js.replace("</script>", "<\\/script>")
html = html[: m.start()] + "<script>\n" + js + "\n</script>" + html[m.end() :]

open(out_path, "w", encoding="utf-8").write(html)
PY

# Verify the result really is self-contained before it is allowed anywhere near
# the tree. This is the check that would have caught the original defect.
# Scope note: this looks for SUBRESOURCES the browser fetches automatically —
# script src, stylesheet/font/preload link href, img/iframe src. Ordinary
# navigational <a href="https://..."> links are fine and expected (the page
# links to the repo and to redocly.com).
assert_self_contained() {
	local f="$1" label="$2" bad
	bad=$(grep -oE '<(script|img|iframe)\b[^>]*\bsrc="https?://[^"]+"' "$f" || true)
	bad="${bad}$(grep -oE '<link\b[^>]*\brel="(stylesheet|preload|preconnect|dns-prefetch)"[^>]*\bhref="https?://[^"]+"' "$f" || true)"
	bad="${bad}$(grep -oE '<link\b[^>]*\bhref="https?://[^"]+"[^>]*\brel="(stylesheet|preload|preconnect|dns-prefetch)"' "$f" || true)"
	bad="${bad}$(grep -oE '@import\s+url\(["'"'"']?https?://' "$f" || true)"
	# Tag-shaped checks are NOT sufficient on their own. The ReDoc bundle injects
	# its attribution logo at runtime from a URL held as a plain string in the
	# minified JS, so a tag grep passed while the browser still hit the CDN —
	# caught only by loading the page with --network none. So also refuse any
	# literal asset URL on a known CDN host, wherever it appears in the file.
	bad="${bad}$(grep -oE 'https?://(cdn\.redoc\.ly|cdn\.redocly\.com|fonts\.googleapis\.com|fonts\.gstatic\.com)/[^"'"'"' )]*' "$f" || true)"
	if [ -n "$bad" ]; then
		echo "ERROR: $label still fetches external subresources:" >&2
		printf '%s\n' "$bad" | head -5 >&2
		return 1
	fi
	return 0
}

assert_self_contained "$TMPDIR_BUILD/out.html" "generated output" || {
	echo "Refusing to write it." >&2
	exit 1
}

if [ "$CHECK_ONLY" = "1" ]; then
	if ! assert_self_contained "$OUT" "$OUT"; then
		echo "FAIL: regenerate with scripts/build-api-docs.sh"
		exit 1
	fi
	if ! cmp -s "$TMPDIR_BUILD/out.html" "$OUT"; then
		echo "FAIL: $OUT is stale relative to $SPEC — regenerate with scripts/build-api-docs.sh"
		exit 1
	fi
	echo "OK: $OUT is current and self-contained"
	exit 0
fi

cp "$TMPDIR_BUILD/out.html" "$OUT"
OUT_BYTES=$(wc -c <"$OUT")
echo "wrote $OUT (${OUT_BYTES} bytes, ReDoc ${REDOC_VERSION} inlined, zero external requests)"
