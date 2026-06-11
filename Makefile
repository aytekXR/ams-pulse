# Pulse — top-level build orchestration.
# Each target delegates into the component directory; components stay independently buildable.

.PHONY: help build build-server build-web build-sdk test test-server test-web test-sdk lint dev up down

help: ## List targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-14s %s\n", $$1, $$2}'

build: build-server build-web build-sdk ## Build everything

build-server: ## Build the pulse server binary
	cd server && go build -o bin/pulse ./cmd/pulse

build-web: ## Build the web dashboard
	cd web && npm run build

build-sdk: ## Build the beacon SDK
	cd sdk/beacon-js && npm run build

test: test-server test-web test-sdk ## Run all tests

test-server:
	cd server && go test ./...

test-web:
	cd web && npm test

test-sdk:
	cd sdk/beacon-js && npm test

lint: ## Lint all components
	cd server && go vet ./...
	cd web && npm run lint
	cd sdk/beacon-js && npm run lint

up: ## Start the full stack locally (Docker Compose)
	cd deploy && docker compose up -d

down: ## Stop the local stack
	cd deploy && docker compose down
