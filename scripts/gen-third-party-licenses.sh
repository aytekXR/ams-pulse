#!/usr/bin/env bash
# Generate THIRD-PARTY-LICENSES.md from the licence files of the dependencies that are
# ACTUALLY REDISTRIBUTED in the shipped artifacts.
#
# Why generated rather than hand-maintained: an attribution file is a legal document, and a
# hand-written one drifts the moment a dependency changes. This reads the real LICENSE text
# out of the Go module cache and node_modules, so the file cannot claim a licence the
# dependency does not carry.
#
# Scope — deliberately narrow, and the narrowness is the point:
#   INCLUDED: modules linked into the pulse binary (go list -deps ./cmd/pulse) and npm
#             packages whose code is bundled into the built web UI (npm --omit=dev).
#   EXCLUDED: build- and test-only dependencies (vite, eslint, typescript, vitest,
#             playwright, gcc, the golang builder image). Their code is not in the artifact,
#             so no distribution obligation attaches. A reviewer who greps for "vite" and
#             does not find it should learn why from the document itself.
#   EXCLUDED: third-party container images (ClickHouse, Caddy, busybox, Postgres). Pulse
#             pins image references; the operator's Docker or Kubernetes pulls them from
#             their own upstream registries. Pulse never rebundles them, so it is not the
#             distributor. They are listed by name and licence for completeness.
#
# Usage:  ./scripts/gen-third-party-licenses.sh          (writes THIRD-PARTY-LICENSES.md)
#         ./scripts/gen-third-party-licenses.sh --check   (fails if the file is stale)
#
# Requires Docker (Go is not installed on the build host) and a populated web/node_modules.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

OUT="THIRD-PARTY-LICENSES.md"
CHECK_MODE=0
[ "${1:-}" = "--check" ] && CHECK_MODE=1

DOCKER="docker"
command -v sg >/dev/null 2>&1 && groups | grep -qv '\bdocker\b' && DOCKER="sg docker -c"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "==> collecting Go modules linked into the pulse binary" >&2
# Emit "module<TAB>version<TAB>licence-file-contents" for every non-stdlib module in the
# binary. Done inside the container because the module cache lives there.
if [ "$DOCKER" = "docker" ]; then
  docker run --rm -v "$REPO_ROOT":/repo -w /repo/server -e GOFLAGS=-buildvcs=false \
    golang:1.25 sh -c '
      go list -deps -f "{{if not .Standard}}{{.Module.Path}} {{.Module.Version}}{{end}}" ./cmd/pulse 2>/dev/null \
        | grep -v "^github.com/aytekXR/ams-pulse/server" | sort -u | grep -v "^$" |
      while read -r mod ver; do
        [ -z "$mod" ] && continue
        dir="/go/pkg/mod/$(echo "$mod" | sed "s/\([A-Z]\)/!\l\1/g")@${ver}"
        lic=""
        for cand in LICENSE LICENSE.txt LICENSE.md COPYING LICENCE license; do
          [ -f "$dir/$cand" ] && lic="$dir/$cand" && break
        done
        printf "===MOD===\t%s\t%s\n" "$mod" "$ver"
        if [ -n "$lic" ]; then cat "$lic"; else echo "NO_LICENSE_FILE_FOUND"; fi
        printf "\n===ENDMOD===\n"
      done' > "$TMP/go-licenses.txt"
else
  sg docker -c "docker run --rm -v $REPO_ROOT:/repo -w /repo/server -e GOFLAGS=-buildvcs=false \
    golang:1.25 sh -c 'go list -deps -f \"{{if not .Standard}}{{.Module.Path}} {{.Module.Version}}{{end}}\" ./cmd/pulse 2>/dev/null \
      | grep -v \"^github.com/aytekXR/ams-pulse/server\" | sort -u | grep -v \"^\$\" |
    while read -r mod ver; do
      [ -z \"\$mod\" ] && continue
      dir=\"/go/pkg/mod/\$(echo \"\$mod\" | sed \"s/\\([A-Z]\\)/!\\l\\1/g\")@\${ver}\"
      lic=\"\"
      for cand in LICENSE LICENSE.txt LICENSE.md COPYING LICENCE license; do
        [ -f \"\$dir/\$cand\" ] && lic=\"\$dir/\$cand\" && break
      done
      printf \"===MOD===\\t%s\\t%s\\n\" \"\$mod\" \"\$ver\"
      if [ -n \"\$lic\" ]; then cat \"\$lic\"; else echo NO_LICENSE_FILE_FOUND; fi
      printf \"\\n===ENDMOD===\\n\"
    done'" > "$TMP/go-licenses.txt"
fi

echo "==> collecting npm runtime packages bundled into the web UI" >&2
if [ -d web/node_modules ]; then
  ( cd web && npm ls --omit=dev --all --parseable 2>/dev/null || true ) \
    | grep node_modules | sort -u > "$TMP/npm-paths.txt" || true
else
  : > "$TMP/npm-paths.txt"
  echo "WARN: web/node_modules absent — npm section will be empty" >&2
fi

python3 - "$TMP" "$OUT" "$CHECK_MODE" <<'PY'
import json, os, re, sys, hashlib, subprocess
tmp, out_path, check_mode = sys.argv[1], sys.argv[2], sys.argv[3] == "1"

# ── Go ────────────────────────────────────────────────────────────────────────
go = []
raw = open(os.path.join(tmp, "go-licenses.txt"), errors="replace").read()
for chunk in raw.split("===MOD===")[1:]:
    head, _, rest = chunk.partition("\n")
    parts = head.strip().split("\t")
    if len(parts) < 2:
        continue
    mod, ver = parts[0].strip(), parts[1].strip()
    text = rest.split("===ENDMOD===")[0].strip()
    go.append((mod, ver, text))

# ── npm ───────────────────────────────────────────────────────────────────────
npm = []
seen = set()
for line in open(os.path.join(tmp, "npm-paths.txt"), errors="replace"):
    d = line.strip()
    if not d or not os.path.isdir(d):
        continue
    pj = os.path.join(d, "package.json")
    if not os.path.isfile(pj):
        continue
    try:
        meta = json.load(open(pj))
    except Exception:
        continue
    name, ver = meta.get("name"), meta.get("version")
    if not name or (name, ver) in seen:
        continue
    seen.add((name, ver))
    lic = meta.get("license")
    if isinstance(lic, dict):
        lic = lic.get("type")
    if not lic and isinstance(meta.get("licenses"), list) and meta["licenses"]:
        lic = meta["licenses"][0].get("type")
    text = ""
    for cand in ("LICENSE", "LICENSE.txt", "LICENSE.md", "LICENCE", "COPYING", "license", "license.md"):
        p = os.path.join(d, cand)
        if os.path.isfile(p):
            text = open(p, errors="replace").read().strip()
            break
    npm.append((name, ver, lic or "UNKNOWN", text))
npm.sort()

def spdx_of(text, declared=None):
    """Identify the licence from its own text. Never guess from the project's reputation."""
    if declared and declared != "UNKNOWN":
        return declared
    t = " ".join(text.split()).lower()
    if not t or t == "no_license_file_found":
        return "UNKNOWN"
    if "apache license" in t and "version 2.0" in t:
        return "Apache-2.0"
    if "permission is hereby granted, free of charge" in t:
        return "MIT"
    if "redistributions of source code must retain" in t:
        return "BSD-3-Clause" if "name of" in t and "endorse or promote" in t else "BSD-2-Clause"
    # ISC ships in two wordings — "and/or distribute" and "and distribute". Matching only the
    # first left nhooyr.io/websocket reported as UNKNOWN when its LICENSE.txt is plainly ISC.
    if ("permission to use, copy, modify, and/or distribute this software" in t
            or "permission to use, copy, modify, and distribute this software" in t):
        return "ISC"
    if "mozilla public license" in t:
        return "MPL-2.0"
    if "gnu general public license" in t:
        return "GPL (see text)"
    return "UNKNOWN"

def copyright_of(text):
    for line in text.splitlines():
        s = line.strip()
        if re.match(r"(?i)copyright\s", s) and len(s) < 200:
            return s
    return ""

go_rows = [(m, v, spdx_of(t), copyright_of(t), t) for m, v, t in go]
npm_rows = [(n, v, spdx_of(t, d), copyright_of(t), t) for n, v, d, t in npm]

COPYLEFT = ("GPL", "AGPL", "LGPL", "SSPL")
flagged = [r for r in go_rows + npm_rows if any(c in r[2] for c in COPYLEFT)]
unknown = [r for r in go_rows + npm_rows if r[2] == "UNKNOWN"]

# Distinct licence texts, reproduced once each in the appendix.
texts = {}
for _, _, spdx, _, t in go_rows + npm_rows:
    if not t or t == "NO_LICENSE_FILE_FOUND":
        continue
    key = hashlib.sha256(" ".join(t.split()).encode()).hexdigest()[:12]
    texts.setdefault(key, (spdx, t))

L = []
w = L.append
w("# Third-party licences")
w("")
w("Pulse redistributes open-source code. This file is the attribution notice for it.")
w("")
w("**It is generated, not written.** `scripts/gen-third-party-licenses.sh` reads the licence")
w("text out of the Go module cache and `web/node_modules` and emits this file, so an entry can")
w("never claim a licence its dependency does not carry. Regenerate it whenever dependencies")
w("change; `--check` mode fails if it is stale.")
w("")
w("## What is in scope, and what is deliberately not")
w("")
w("Listed here is the code that ends up **inside a shipped artifact**: the Go modules linked")
w("into the `pulse` binary, and the npm packages bundled into the built web UI.")
w("")
w("**Build- and test-only dependencies are excluded on purpose.** Vite, ESLint, TypeScript,")
w("Vitest, Playwright, the Go toolchain and the builder images never reach the artifact, so no")
w("distribution obligation attaches to them. If you grep this file for `vite` and find nothing,")
w("that is the reason, not an omission.")
w("")
w("**Third-party container images are also excluded from the attribution tables below**, for a")
w("different reason: Pulse *pins image references*, it does not rebundle images. Your Docker or")
w("Kubernetes pulls ClickHouse, Caddy, Postgres and busybox from their own upstream registries,")
w("so Pulse is not their distributor. They are named in the last section for completeness.")
w("")
w("## Pulse's own licensing")
w("")
w("| Component | Licence |")
w("|---|---|")
w("| Pulse server and web UI | PolyForm Noncommercial 1.0.0 for the Free tier; a commercial subscription is required for paid production use. See `LICENSE` and `docs/licensing-public.md`. |")
w("| `sdk/beacon-js` (npm `ams-pulse-beacon`) | MIT — embeddable in any player, including commercial products |")
w("| `sdk/beacon-swift` (`PulseBeacon`) | MIT |")
w("")
w("Both SDKs ship with **zero runtime dependencies**, so embedding either one adds no")
w("third-party obligations of its own. That is a verified property, not an aspiration:")
w("`sdk/beacon-js/package.json` declares no `dependencies` (only build tooling under")
w("`devDependencies`) and its published bundle is self-contained; `sdk/beacon-swift/Package.swift`")
w("and `ios/PulseKit/Package.swift` both declare no package dependencies.")
w("")
w("## Summary")
w("")
w(f"- Go modules linked into the `pulse` binary: **{len(go_rows)}**")
w(f"- npm packages bundled into the web UI: **{len(npm_rows)}**")
if flagged:
    w(f"- ⚠ Copyleft-licensed redistributed dependencies: **{len(flagged)}** — see below")
else:
    w("- Copyleft (GPL / AGPL / LGPL / SSPL) among redistributed dependencies: **none**")
if unknown:
    w(f"- ⚠ Dependencies whose licence could not be determined from their own files: **{len(unknown)}**")
else:
    w("- Dependencies with an undetermined licence: **none**")
w("")
if flagged:
    w("### Copyleft dependencies requiring review")
    w("")
    w("| Dependency | Version | Licence |")
    w("|---|---|---|")
    for n, v, s, _, _ in flagged:
        w(f"| `{n}` | {v} | {s} |")
    w("")
if unknown:
    w("### Undetermined licences")
    w("")
    w("Listed as UNKNOWN rather than guessed. Resolve before relying on this file for a")
    w("compliance sign-off.")
    w("")
    w("| Dependency | Version |")
    w("|---|---|")
    for n, v, _, _, _ in unknown:
        w(f"| `{n}` | {v} |")
    w("")

w("## Go modules (linked into the `pulse` binary)")
w("")
w("| Module | Version | Licence | Copyright |")
w("|---|---|---|---|")
for n, v, s, c, _ in sorted(go_rows):
    w(f"| `{n}` | {v} | {s} | {c.replace('|', '/')} |")
w("")
w("## npm packages (bundled into the web UI)")
w("")
if npm_rows:
    w("| Package | Version | Licence | Copyright |")
    w("|---|---|---|---|")
    for n, v, s, c, _ in npm_rows:
        w(f"| `{n}` | {v} | {s} | {c.replace('|', '/')} |")
else:
    w("_Not enumerated in this run: `web/node_modules` was absent. Run `npm ci` in `web/` and regenerate._")
w("")
w("## Container images referenced (not redistributed by Pulse)")
w("")
w("Pulse pins these by tag or digest in `deploy/`; your own container runtime pulls them from")
w("their upstream registries. Pulse does not modify or rebundle them.")
w("")
w("| Image | Licence of the software inside | Role |")
w("|---|---|---|")
w("| `clickhouse/clickhouse-server` | Apache-2.0 | Metrics store. Apache-2.0, **not** SSPL — worth stating plainly, because ClickHouse is often assumed to be source-available-restricted and it is not. |")
w("| `caddy` | Apache-2.0 | Optional TLS reverse proxy |")
w("| `postgres` | PostgreSQL Licence (permissive, BSD-like) | Optional external meta store |")
w("| `busybox` | GPL-2.0-only | Ephemeral Helm `initContainer` that waits for ClickHouse. Pulled by your cluster, never bundled or modified by Pulse, so no GPL distribution obligation arises for this product. |")
w("| `alpine` | MIT (base system); individual packages carry their own licences | Runtime base of the Pulse image |")
w("")
w("## Appendix — full licence texts")
w("")
w("Permissive licences require their text to travel with the distribution, so each distinct")
w("licence text found among the dependencies above is reproduced once here, verbatim from the")
w("dependency that supplied it.")
w("")
for key, (spdx, t) in sorted(texts.items(), key=lambda kv: (kv[1][0], kv[0])):
    users = [n for n, _, s, _, tt in go_rows + npm_rows
             if tt and hashlib.sha256(" ".join(tt.split()).encode()).hexdigest()[:12] == key]
    w(f"### {spdx} — text `{key}`")
    w("")
    w(f"Applies to: {', '.join('`%s`' % u for u in sorted(users)[:40])}"
      + (f" _(and {len(users) - 40} more)_" if len(users) > 40 else ""))
    w("")
    w("```text")
    for line in t.splitlines():
        w(line.rstrip())
    w("```")
    w("")

body = "\n".join(L).rstrip() + "\n"

if check_mode:
    existing = open(out_path).read() if os.path.exists(out_path) else ""
    def norm(s):
        return "\n".join(l.rstrip() for l in s.splitlines()).strip()
    if norm(existing) != norm(body):
        print(f"FAIL: {out_path} is stale — regenerate with scripts/gen-third-party-licenses.sh")
        sys.exit(1)
    print(f"OK: {out_path} is current ({len(go_rows)} Go modules, {len(npm_rows)} npm packages)")
else:
    open(out_path, "w").write(body)
    print(f"wrote {out_path}: {len(go_rows)} Go modules, {len(npm_rows)} npm packages, "
          f"{len(texts)} distinct licence texts, {len(flagged)} copyleft, {len(unknown)} unknown")
PY
