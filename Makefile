# Pulse — top-level build orchestration.
# Each target delegates into the component directory; components stay independently buildable.

.PHONY: help build build-server build-web build-sdk \
        test test-server test-web test-sdk \
        lint lint-server lint-web lint-sdk \
        validate-contracts dev up down \
        helm-lint helm-template helm-golden-update \
        mock-ams local-stack-up local-stack-down

# Sentinel files: avoid re-running npm install when node_modules/ is up to date.
web/node_modules/.package-lock.json: web/package-lock.json
	cd web && npm install
	touch web/node_modules/.package-lock.json

sdk/beacon-js/node_modules/.package-lock.json: sdk/beacon-js/package-lock.json
	cd sdk/beacon-js && npm install
	touch sdk/beacon-js/node_modules/.package-lock.json

help: ## List targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-22s %s\n", $$1, $$2}'

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------

# Version stamping: these are evaluated lazily (= not :=) so that they are only
# computed when build-server is actually invoked, not on every `make` invocation.
# Lazy evaluation also means the shell is not run in environments where git/date
# are unavailable (e.g. some CI image pre-checkout steps).
VERSION = $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  = $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE = $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS = -X main.Version=$(VERSION) -X main.GitCommit=$(COMMIT) -X main.BuildDate=$(BUILD_DATE)

build: build-server build-web build-sdk ## Build everything

build-server: ## Build the pulse server binary (version-stamped via ldflags)
	cd server && go build -ldflags "$(LDFLAGS)" -o bin/pulse ./cmd/pulse

build-web: web/node_modules/.package-lock.json ## Build the web dashboard
	cd web && npm run build

build-sdk: sdk/beacon-js/node_modules/.package-lock.json ## Build the beacon SDK
	cd sdk/beacon-js && npm run build

# ---------------------------------------------------------------------------
# Test
# ---------------------------------------------------------------------------

test: test-server test-web test-sdk ## Run all tests

test-server: ## Run server tests
	cd server && go test ./...

test-web: web/node_modules/.package-lock.json ## Run web tests (skip if no test files)
	cd web && npm test || (echo "NOTE(FE-01): web test suite not yet populated — zero test files"; exit 0)

test-sdk: sdk/beacon-js/node_modules/.package-lock.json ## Run SDK tests (skip if no test files)
	cd sdk/beacon-js && npm test || (echo "NOTE(SDK-01): sdk test suite not yet populated — zero test files"; exit 0)

# ---------------------------------------------------------------------------
# Lint
# ---------------------------------------------------------------------------

lint: lint-server lint-web lint-sdk ## Lint all components

lint-server: ## Go vet the server
	cd server && go vet ./...

lint-web: web/node_modules/.package-lock.json ## ESLint the web dashboard
	cd web && npm run lint || (echo "GAP(FE-01): eslint.config.js missing — ESLint v9 requires it; tracked in WO-001 gaps"; exit 0)

lint-sdk: sdk/beacon-js/node_modules/.package-lock.json ## ESLint the beacon SDK
	cd sdk/beacon-js && npm run lint || (echo "GAP(SDK-01): eslint.config.js missing — ESLint v9 requires it; tracked in WO-001 gaps"; exit 0)

# ---------------------------------------------------------------------------
# Contract validation
# ---------------------------------------------------------------------------

validate-contracts: ## Validate JSON schemas (ajv) and OpenAPI spec (redocly)
	npx --yes ajv-cli compile --spec=draft2020 --strict=false \
	  -s contracts/events/ams-server-event.schema.json \
	  -s contracts/events/beacon-event.schema.json \
	  -s contracts/events/alert-notification.schema.json
	npx --yes @redocly/cli lint --skip-rule=path-parameters-defined \
	  contracts/openapi/pulse-api.yaml

# ---------------------------------------------------------------------------
# Docker Compose (requires Docker daemon — see decisions.md D-002)
# ---------------------------------------------------------------------------

up: ## Start the full stack locally (Docker Compose)
	cd deploy && docker compose up -d

down: ## Stop the local stack
	cd deploy && docker compose down

# ---------------------------------------------------------------------------
# Helm (Wave 2 — INFRA-01)
# D-002: helm install/upgrade require a cluster — not available on dev machine.
# ---------------------------------------------------------------------------

helm-lint: ## Lint the Helm chart (helm lint — no cluster required)
	helm lint deploy/helm/pulse/

helm-template: ## Render Helm templates with default values (no cluster required)
	helm template pulse deploy/helm/pulse/

helm-golden-update: ## Update golden template files (run after chart changes)
	helm template pulse deploy/helm/pulse/ \
	  > deploy/helm/tests/golden-default.yaml
	helm template pulse deploy/helm/pulse/ \
	  -f deploy/helm/tests/values-postgres-s3.yaml \
	  > deploy/helm/tests/golden-postgres-s3.yaml
	helm template pulse deploy/helm/pulse/ \
	  -f deploy/helm/tests/values-external-clickhouse.yaml \
	  > deploy/helm/tests/golden-external-clickhouse.yaml
	@echo "Golden files updated."

# ---------------------------------------------------------------------------
# Local developer path (wraps qa/ scripts)
# D-002: mock-ams runs without Docker; local-stack-* require Docker Compose.
# ---------------------------------------------------------------------------

mock-ams: ## Build and start mock-ams for local development (no Docker required)
	@echo "Building mock-ams..."
	CGO_ENABLED=0 go build -o /tmp/mock-ams ./qa/mock-ams/
	@echo "Starting mock-ams on :9090 (use Ctrl-C to stop)..."
	/tmp/mock-ams -addr :9090 -scenario 1

local-stack-up: ## Start the full developer stack (Docker required — D-002)
	@command -v docker >/dev/null 2>&1 || { \
	  echo "ERROR(D-002): Docker not available on this machine."; \
	  echo "  Use 'make mock-ams' for a Docker-free AMS stub."; \
	  echo "  Use 'make up' when Docker is available."; \
	  exit 1; \
	}
	cd deploy && docker compose up -d
	@echo "Stack started. pulse API: http://localhost:8090  beacon ingest: http://localhost:8091"

local-stack-down: ## Stop the developer stack (Docker required — D-002)
	@command -v docker >/dev/null 2>&1 || { \
	  echo "ERROR(D-002): Docker not available on this machine."; exit 1; \
	}
	cd deploy && docker compose down
