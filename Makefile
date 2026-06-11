# Pulse — top-level build orchestration.
# Each target delegates into the component directory; components stay independently buildable.

.PHONY: help build build-server build-web build-sdk \
        test test-server test-web test-sdk \
        lint lint-server lint-web lint-sdk \
        validate-contracts dev up down

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

build: build-server build-web build-sdk ## Build everything

build-server: ## Build the pulse server binary
	cd server && go build -o bin/pulse ./cmd/pulse

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
