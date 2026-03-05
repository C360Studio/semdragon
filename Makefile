.PHONY: build test test-integration test-all lint fmt tidy check schema check-schema openapi check-openapi mockllm \
       up down up-cloud up-ollama \
       e2e e2e-up e2e-down e2e-run e2e-wait e2e-install e2e-chromium e2e-headed e2e-ui e2e-clean e2e-report \
       e2e-cloud e2e-cloud-up e2e-cloud-run e2e-cloud-down \
       e2e-cloud-tiered e2e-cloud-tiered-up e2e-cloud-tiered-run e2e-cloud-tiered-down \
       e2e-ollama e2e-ollama-up e2e-ollama-run e2e-ollama-down \
       ui-test ui-check

# Build all packages
build:
	go build ./...

# Run the mock LLM server locally (OpenAI-compatible stub on :9090)
mockllm:
	go run ./cmd/mockllm

# Run unit tests only (fast, no Docker required)
test:
	go test -race -v ./...

# Run integration tests only (requires Docker for NATS)
test-integration:
	go test -race -v -tags=integration ./...

# Run all tests (unit + integration)
test-all:
	go test -race -v -tags=integration ./...

# Run a specific test
# Usage: make test-one TEST=TestFunctionName
test-one:
	go test -race -v -run $(TEST) ./...

# Run a specific test with integration tag
# Usage: make test-one-integration TEST=TestFunctionName
test-one-integration:
	go test -race -v -tags=integration -run $(TEST) ./...

# Run linter
lint:
	revive -config revive.toml -formatter friendly ./...
	go vet ./...

# Format code
fmt:
	go fmt ./...
	goimports -w .

# Tidy dependencies
tidy:
	go mod tidy

# Full check: format, tidy, lint, test (unit + integration)
check: fmt tidy lint test-all

# Coverage report (includes integration tests)
coverage:
	go test -race -tags=integration -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Frontend unit tests (vitest)
ui-test:
	$(MAKE) -C ui test

# Frontend type-check + lint
ui-check:
	$(MAKE) -C ui check

# =============================================================================
# Entity Schema Codegen
# =============================================================================

# Generate entity schema JSON from Go Triples() implementations
schema:
	go run ./cmd/gen-entity-schema > ui/src/lib/services/entity-schema.generated.json

# Check that committed schema matches current Go code (ignoring generated_at timestamp)
check-schema:
	@go run ./cmd/gen-entity-schema | grep -v generated_at > /tmp/schema-check-new.json
	@grep -v generated_at ui/src/lib/services/entity-schema.generated.json > /tmp/schema-check-old.json
	@diff /tmp/schema-check-new.json /tmp/schema-check-old.json && echo "Schema is up to date."

# =============================================================================
# OpenAPI Spec Codegen
# =============================================================================

# Generate OpenAPI 3.0 JSON spec from Go struct reflection
openapi:
	go run ./cmd/openapi-gen > ui/static/openapi.json

# Check that committed spec matches current Go code
check-openapi:
	@go run ./cmd/openapi-gen > /tmp/openapi-check.json
	@diff /tmp/openapi-check.json ui/static/openapi.json && echo "OpenAPI spec is up to date."

# =============================================================================
# Docker Compose — Quick Start
# =============================================================================

CLOUD_COMPOSE  = -f docker-compose.yml -f docker-compose.cloud.yml
OLLAMA_COMPOSE = -f docker-compose.yml -f docker-compose.ollama.yml

# Start with mock LLM (no API key needed)
up:
	docker compose up -d --build --wait
	@echo "Stack is up. Dashboard: http://localhost:5173  API: http://localhost:8080"

# Start with cloud LLM (set GEMINI_API_KEY, ANTHROPIC_API_KEY, or OPENAI_API_KEY in .env)
up-cloud:
	docker compose $(CLOUD_COMPOSE) up -d --build --wait
	@echo "Cloud stack is up. Dashboard: http://localhost:5173  API: http://localhost:8080"

# Start with local Ollama (requires: ollama serve && ollama pull qwen2.5-coder:7b)
up-ollama:
	docker compose $(OLLAMA_COMPOSE) up -d --build --wait
	@echo "Ollama stack is up. Dashboard: http://localhost:5173  API: http://localhost:8080"

# Stop the stack
down:
	docker compose down -v
	@echo "Stack stopped."

# =============================================================================
# E2E Testing (Playwright + Docker Compose)
# =============================================================================

# ─── Default E2E (mock LLM) ─────────────────────────────────────────

# Full lifecycle: start stack, run tests, tear down
e2e: e2e-install e2e-up e2e-wait e2e-run e2e-down

# Start the Docker stack (nats + mockllm + backend + ui)
e2e-up:
	SEED_E2E=true docker compose up -d --build --wait
	@echo "E2E stack is up. Backend: http://localhost:8080  UI: http://localhost:5173"

# Wait for backend health (with retries)
e2e-wait:
	@echo "Waiting for backend health..."
	@for i in $$(seq 1 30); do \
		if curl -sf http://localhost:8080/health > /dev/null 2>&1; then \
			echo "Backend healthy after $${i}s"; \
			break; \
		fi; \
		sleep 1; \
	done

# Run Playwright tests (stack must be running)
e2e-run:
	cd ui && npx playwright test

# Run on chromium only (fast iteration)
e2e-chromium:
	cd ui && npx playwright test --project=chromium

# Run E2E tests in headed mode (shows browser)
e2e-headed: e2e-install e2e-up e2e-wait
	cd ui && npx playwright test --headed
	$(MAKE) e2e-down

# Run E2E tests with UI mode (interactive debugging)
e2e-ui: e2e-install e2e-up e2e-wait
	cd ui && npx playwright test --ui

# Stop the Docker stack
e2e-down:
	docker compose down -v
	@echo "E2E stack stopped."

# Force clean slate (removes volumes)
e2e-clean:
	docker compose down -v

# View E2E test report
e2e-report:
	cd ui && npx playwright show-report

# Install Playwright browsers (first-time setup)
e2e-install:
	cd ui && npx playwright install --with-deps chromium

# ─── Cloud LLM E2E (Gemini, Anthropic, OpenAI) ──────────────────────
#
# Usage:
#   GEMINI_API_KEY=$GEMINI_API_KEY make e2e-cloud
#   SEMDRAGONS_CONFIG=/etc/semdragons/semdragons-e2e-anthropic.json ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY make e2e-cloud
#   SEMDRAGONS_CONFIG=/etc/semdragons/semdragons-e2e-openai.json OPENAI_API_KEY=$OPENAI_API_KEY make e2e-cloud

e2e-cloud: e2e-install e2e-cloud-up e2e-wait e2e-cloud-run e2e-cloud-down

# Start stack with cloud config (no mockllm, uses cloud API keys)
e2e-cloud-up:
	SEED_E2E=true docker compose $(CLOUD_COMPOSE) up -d --build --wait
	@echo "Cloud E2E stack is up. Backend: http://localhost:8080  UI: http://localhost:5173"

# Run DM chat integration tests (chromium only — real LLM)
e2e-cloud-run:
	cd ui && E2E_REAL_LLM=true npx playwright test dm-chat-integration --project=chromium

# Stop the cloud stack
e2e-cloud-down:
	docker compose $(CLOUD_COMPOSE) down -v
	@echo "Cloud E2E stack stopped."

# ─── Cloud LLM E2E — Tiered (multi-model Gemini) ─────────────────────
#
# Usage:
#   GEMINI_API_KEY=$GEMINI_API_KEY make e2e-cloud-tiered

TIERED_CONFIG = /etc/semdragons/semdragons-e2e-gemini-tiered.json

e2e-cloud-tiered: e2e-install e2e-cloud-tiered-up e2e-wait e2e-cloud-tiered-run e2e-cloud-tiered-down

# Start stack with tiered config
e2e-cloud-tiered-up:
	SEED_E2E=true SEMDRAGONS_CONFIG=$(TIERED_CONFIG) docker compose $(CLOUD_COMPOSE) up -d --build --wait
	@echo "Tiered E2E stack is up. Backend: http://localhost:8080  UI: http://localhost:5173"

# Run model registry spec (chromium only — real LLM)
e2e-cloud-tiered-run:
	cd ui && E2E_REAL_LLM=true npx playwright test model-registry --project=chromium

# Stop the tiered stack
e2e-cloud-tiered-down:
	docker compose $(CLOUD_COMPOSE) down -v
	@echo "Tiered E2E stack stopped."

# ─── Ollama E2E (local LLM) ─────────────────────────────────────────

e2e-ollama: e2e-install e2e-ollama-up e2e-wait e2e-ollama-run e2e-ollama-down

# Start stack with Ollama config (no mockllm, points at host Ollama)
e2e-ollama-up:
	SEED_E2E=true docker compose $(OLLAMA_COMPOSE) up -d --build --wait
	@echo "Ollama E2E stack is up. Backend: http://localhost:8080  UI: http://localhost:5173"

# Run Ollama integration spec (chromium only)
e2e-ollama-run:
	cd ui && E2E_OLLAMA=true npx playwright test ollama-integration --project=chromium

# Stop the Ollama stack
e2e-ollama-down:
	docker compose $(OLLAMA_COMPOSE) down -v
	@echo "Ollama E2E stack stopped."
