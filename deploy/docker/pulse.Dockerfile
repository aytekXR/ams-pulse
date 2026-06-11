# Pulse all-in-one image: multi-stage build producing one small container that
# serves the API, the built web UI, and the beacon ingest endpoint.
# TODO(INFRA-01): pin base image digests; add non-root user; healthcheck.

# --- web UI ---
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json* ./
RUN npm ci || npm install
COPY web/ ./
RUN npm run build

# --- server ---
FROM golang:1.24-alpine AS server
WORKDIR /src/server
COPY server/go.mod server/go.sum* ./
RUN go mod download || true
COPY server/ ./
COPY contracts/ /src/contracts/
RUN CGO_ENABLED=0 go build -o /out/pulse ./cmd/pulse

# --- runtime ---
FROM alpine:3.21
RUN adduser -D -H pulse
COPY --from=server /out/pulse /usr/local/bin/pulse
COPY --from=web /src/web/dist /usr/share/pulse/web
USER pulse
EXPOSE 8090 8091
ENTRYPOINT ["pulse", "serve"]
