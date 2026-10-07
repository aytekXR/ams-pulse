#!/usr/bin/env bash
# Stop the marketplace demo stack and delete its volumes and local state.
# Scoped to compose project `pulse-mktdemo` only — prod/realams/quickstart are untouched.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_ARGS=()
[[ -f "$HERE/.env" ]] && ENV_ARGS=(--env-file "$HERE/.env")

docker compose -p pulse-mktdemo -f "$HERE/docker-compose.demo.yml" "${ENV_ARGS[@]}" down -v --remove-orphans
rm -rf "$HERE/.state"
rm -f "$HERE/.env"
echo "[demo-stack] removed project pulse-mktdemo, its volumes, .env and .state/"
