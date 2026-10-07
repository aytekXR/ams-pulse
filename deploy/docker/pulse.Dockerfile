# Pulse all-in-one image: multi-stage build producing one small container that
# serves the API, the built web UI, and the beacon ingest endpoint.
# Base image digests pinned 2026-06-11 via registry HTTP API (no Docker daemon).
# To refresh: auth.docker.io token → registry-1.docker.io manifests HEAD with
#   Accept: application/vnd.oci.image.index.v1+json,...manifest.list.v2+json

# --- web UI ---
# node:22-alpine
FROM node@sha256:926d6cafec97f338577041890465522f70fe74aa6fe4b021a4fd7f87a5996b25 AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci --legacy-peer-deps || npm install --legacy-peer-deps
COPY web/ ./
RUN npm run build

# --- server ---
# golang:1.26-alpine (multi-arch index) — digest pinned 2026-10-07; Go: go1.26.8. The previous
# pin was labelled 1.25 but was go1.26.5, whose stdlib CVEs blocked the v0.5.0 Trivy gate.
# To refresh: docker buildx imagetools inspect golang:1.26-alpine  (use the index Digest)
FROM golang@sha256:8ac98ca534ac3f51e1f420a1dd2c15e74c75cfa0f23f3ad27eb5d7236c349a0c AS server
WORKDIR /src/server
COPY server/go.mod server/go.sum* ./
RUN go mod download || true
COPY server/ ./
COPY contracts/ /src/contracts/
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 go build \
      -ldflags "-X main.Version=${VERSION} -X main.GitCommit=${COMMIT} -X main.BuildDate=${BUILD_DATE}" \
      -o /out/pulse ./cmd/pulse

# --- runtime ---
# alpine:3.24.2 (multi-arch index) — digest pinned 2026-10-07; carries OpenSSL 3.5.8-r0
# (CVE-2026-14456 fixed). Refresh: docker buildx imagetools inspect alpine:3.24
FROM alpine@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6
# Create the meta-store/secret-key dir owned by the non-root pulse user so a fresh
# pulse-data named volume inherits pulse:pulse ownership (else SQLITE_CANTOPEN at /var/lib/pulse).
RUN adduser -D -H pulse && mkdir -p /var/lib/pulse && chown pulse:pulse /var/lib/pulse
COPY --from=server /out/pulse /usr/local/bin/pulse
COPY --from=web /src/web/dist /usr/share/pulse/web
# Bake ClickHouse migration SQL into the image so single-image deployments (quickstart,
# released ghcr.io image) do not need a repo clone to apply schema.
# Source: contracts/db/clickhouse/ is copied into /src/contracts/ in the server build stage (line 25).
COPY --from=server /src/contracts/db/clickhouse /usr/share/pulse/migrations
ENV PULSE_MIGRATIONS_DIR=/usr/share/pulse/migrations
USER pulse
EXPOSE 8090 8091 8092
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:8090/healthz || exit 1
ENTRYPOINT ["pulse", "serve"]
