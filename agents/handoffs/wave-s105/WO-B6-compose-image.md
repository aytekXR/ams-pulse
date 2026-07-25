# WO-B6 (S105) — evaluator compose path defaults to the signed GHCR image

**Review finding (Issue 10, P1, verified):** `deploy/docker-compose.yml` still carries
`TODO(INFRA-01 wave-1): switch to published image once first release ships` (four releases have
shipped) and a `build:` block; README's documented production path makes evaluators compile from
source, bypassing the cosign-signed, Trivy-gated, SBOM-attached GHCR image. Only quickstart uses
the published image.

**Hard constraints (S100/D-164 lesson — the VPS prod deploy flow is load-bearing):**
- Do NOT change `deploy/nginx/deployment.sh`.
- Do NOT change the functional content of `deploy/docker-compose.prod.yml` (the VPS canonical
  path deliberately builds stamped source; you may add a clarifying comment header ONLY).
- Never set both `image: ghcr.io/...` and `build:` on the same service in a file combination
  that a documented `--build` command uses — a local build would masquerade under the signed
  tag name.

## Scope — you may ONLY edit these files
- `deploy/docker-compose.yml`
- `deploy/docker-compose.build.yml` (create)
- `deploy/docker-compose.prod.yml` (comment header only)
- `README.md` — deployment/production sections ONLY (another agent edits the License section
  concurrently; keep edits scoped)
- `docs/runbooks/install.md`
- `Makefile` (only compose invocations, if they rely on build:)
- `.github/workflows/*.yml` (ONLY compose invocations that relied on `deploy/docker-compose.yml`
  having a build: block — CI must keep building from source; do NOT touch release.yml, another
  agent owns it)
- `qa/**` scripts IF they invoke `deploy/docker-compose.yml` expecting build: (grep first)

No git commands.

## Work items

1. **`deploy/docker-compose.yml`:** pulse service (and the one-shot migrate service if it shares
   the build) → `image: ${PULSE_IMAGE:-ghcr.io/aytekxr/ams-pulse:0.4.1}`; REMOVE the build: block
   and the stale TODO comment. Add a comment: default is the signed published image (cosign/SBOM
   — see README); to build from source add `-f deploy/docker-compose.build.yml --build`.

2. **`deploy/docker-compose.build.yml` (new):** overlay adding the original `build:` block(s)
   (context/dockerfile/args exactly as they were) for the same service names. Building via
   overlay must tag the local build as something that CANNOT be confused with the signed GHCR
   tag — set `image: ams-pulse:local-build` (or similar) in the overlay so the local build never
   masquerades as `ghcr.io/aytekxr/ams-pulse:0.4.1`.

3. **Callers:** grep Makefile, .github/workflows (except release.yml), qa/ and docs for
   invocations of `deploy/docker-compose.yml` that expect source builds (`--build` or implicit).
   Update each to include `-f deploy/docker-compose.build.yml`. CI e2e jobs MUST keep building
   the current commit from source — verify by reading the workflow steps you touch.

4. **`README.md` deployment sections:** the evaluator/production path becomes: quickstart
   installer OR compose with the signed image (show `docker compose -f deploy/docker-compose.yml
   up -d` + the cosign verify snippet reference); building from source is the documented
   alternative (`-f deploy/docker-compose.yml -f deploy/docker-compose.build.yml up -d --build`).

5. **`docs/runbooks/install.md` Path A:** flip to image-default, keep the source-build variant as
   an explicitly labeled alternative. Fix the borderline stale comment at ~line 221 ("v0.4.0
   publishes the image tag 0.4.0") to use 0.4.1 in its example while keeping the convention
   explanation.

6. **`deploy/docker-compose.prod.yml`:** add a 3-4 line comment header: this file is the VPS
   canonical path and deliberately builds a stamped source image via deploy/nginx/deployment.sh
   (see deploy/runbooks/upgrade-rollback.md); evaluators should use the quickstart or the base
   compose + GHCR image.

7. **Validate:** `docker compose -f deploy/docker-compose.yml config -q` AND
   `docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.build.yml config -q`
   both succeed (run from repo root; use env defaults — if required env vars block `config`,
   pass a minimal `--env-file` the repo already provides for CI, or document why not runnable).

## Definition of done
Stale TODO gone; default path = signed image; CI still builds from source; both compose configs
validate. Return: files changed, every caller you updated, validation output, anything that
still references build-from-source as the default (report).
