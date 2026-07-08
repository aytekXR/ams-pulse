# Pulse all-in-one image: multi-stage build producing one small container that
# serves the API, the built web UI, and the beacon ingest endpoint.
# Base image digests pinned 2026-06-11 via registry HTTP API (no Docker daemon).
# To refresh: auth.docker.io token → registry-1.docker.io manifests HEAD with
#   Accept: application/vnd.oci.image.index.v1+json,...manifest.list.v2+json

# --- web UI ---
# node:22-alpine
FROM node@sha256:9385cd9f3001dfc3431e8ead12c43e9e1f87cc1b9b5c6cfd0f73865d405b27c4 AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci --legacy-peer-deps || npm install --legacy-peer-deps
COPY web/ ./
RUN npm run build

# --- server ---
# golang:1.25-alpine — digest pinned 2026-07-08 via `docker image inspect golang:1.25-alpine --format '{{index .RepoDigests 0}}'`
# Tag: golang:1.25-alpine  Go: go1.25.12
# To refresh: docker pull golang:1.25-alpine && docker image inspect golang:1.25-alpine --format '{{index .RepoDigests 0}}'
FROM golang@sha256:b900de91b15b2e2953d930ece1d0ecff0a1590ab2006088d20dcf0f56f1e979f AS server
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
# alpine:3.21
FROM alpine@sha256:48b0309ca019d89d40f670aa1bc06e426dc0931948452e8491e3d65087abc07d
# Create the meta-store/secret-key dir owned by the non-root pulse user so a fresh
# pulse-data named volume inherits pulse:pulse ownership (else SQLITE_CANTOPEN at /var/lib/pulse).
RUN adduser -D -H pulse && mkdir -p /var/lib/pulse && chown pulse:pulse /var/lib/pulse
COPY --from=server /out/pulse /usr/local/bin/pulse
COPY --from=web /src/web/dist /usr/share/pulse/web
USER pulse
EXPOSE 8090 8091
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:8090/healthz || exit 1
ENTRYPOINT ["pulse", "serve"]
