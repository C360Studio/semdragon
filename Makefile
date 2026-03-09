.PHONY: build test test-integration test-all lint fmt tidy check schema check-schema openapi check-openapi mockllm \
       up down up-cloud up-ollama \
       e2e e2e-up e2e-down e2e-run e2e-wait e2e-install e2e-chromium e2e-headed e2e-ui e2e-clean e2e-report \
       e2e-gemini e2e-anthropic e2e-openai e2e-ollama e2e-spec e2e-nats-clean e2e-logs e2e-help \
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
BACKEND_PORT   ?= 8081

# Start with mock LLM (no API key needed)
up:
	BACKEND_PORT=$(BACKEND_PORT) docker compose --profile mock up -d --build --wait
	@echo "Stack is up. Dashboard: http://localhost:5173  API: http://localhost:$(BACKEND_PORT)"

# Start with cloud LLM (set GEMINI_API_KEY, ANTHROPIC_API_KEY, or OPENAI_API_KEY in .env)
up-cloud:
	BACKEND_PORT=$(BACKEND_PORT) docker compose $(CLOUD_COMPOSE) up -d --build --wait
	@echo "Cloud stack is up. Dashboard: http://localhost:5173  API: http://localhost:$(BACKEND_PORT)"

# Start with local Ollama (requires: ollama serve && ollama pull qwen2.5-coder:7b)
up-ollama:
	BACKEND_PORT=$(BACKEND_PORT) docker compose $(OLLAMA_COMPOSE) up -d --build --wait
	@echo "Ollama stack is up. Dashboard: http://localhost:5173  API: http://localhost:$(BACKEND_PORT)"

# Stop the stack (--remove-orphans handles any profile/override combination)
down:
	docker compose --profile mock down -v --remove-orphans
	@echo "Stack stopped."

# =============================================================================
# E2E Testing (Playwright + Docker Compose)
# =============================================================================
#
# Env vars (unified):
#   SEMDRAGONS_E2E_CONFIG  — config file name inside /etc/semdragons/ (set by Makefile per provider)
#   E2E_LLM_MODE           — tells Playwright which LLM mode: mock|gemini|anthropic|openai|ollama
#   BACKEND_PORT            — host port for the backend (default 8081)
#   SPEC                    — run a single spec file (e.g. SPEC=quest-lifecycle)
#
# Quick reference:
#   make e2e                  — mock LLM, all specs
#   make e2e-gemini           — Gemini cloud, all specs
#   make e2e-anthropic        — Anthropic cloud, all specs
#   make e2e-openai           — OpenAI cloud, all specs
#   make e2e-ollama           — local Ollama, all specs
#   make e2e-help             — print full usage guide

# Default spec — empty means "all specs"
SPEC ?=
_pw_spec = $(if $(SPEC),$(SPEC),)
_pw_args = $(_pw_spec) --project=chromium

# ─── Mock LLM (default, no API key needed) ────────────────────────

e2e: e2e-install e2e-up e2e-wait e2e-run e2e-down

e2e-up:
	SEED_E2E=true BACKEND_PORT=$(BACKEND_PORT) docker compose --profile mock up -d --build --wait
	@echo "E2E stack is up. Backend: http://localhost:$(BACKEND_PORT)  UI: http://localhost:5173"

e2e-wait:
	@echo "Waiting for backend health..."
	@for i in $$(seq 1 30); do \
		if curl -sf http://localhost:$(BACKEND_PORT)/health > /dev/null 2>&1; then \
			echo "Backend healthy after $${i}s"; \
			break; \
		fi; \
		sleep 1; \
	done

e2e-run:
	cd ui && E2E_LLM_MODE=mock BACKEND_PORT=$(BACKEND_PORT) npx playwright test $(_pw_args)

e2e-chromium:
	cd ui && E2E_LLM_MODE=mock BACKEND_PORT=$(BACKEND_PORT) npx playwright test --project=chromium

e2e-headed: e2e-install e2e-up e2e-wait
	cd ui && E2E_LLM_MODE=mock BACKEND_PORT=$(BACKEND_PORT) npx playwright test --headed
	$(MAKE) e2e-down

e2e-ui: e2e-install e2e-up e2e-wait
	cd ui && E2E_LLM_MODE=mock BACKEND_PORT=$(BACKEND_PORT) npx playwright test --ui

e2e-down:
	docker compose --profile mock down -v --remove-orphans
	@echo "E2E stack stopped."

e2e-clean: e2e-nats-clean

e2e-report:
	cd ui && npx playwright show-report

e2e-install:
	cd ui && npx playwright install --with-deps chromium

# ─── Cloud Providers (one command, full lifecycle) ─────────────────
#
# Each target: clean NATS → start stack with provider config → run tests → tear down.
# API keys are read from .env or the environment.
#
# Usage:
#   make e2e-gemini                              # all specs
#   make e2e-gemini SPEC=party-quest-dag-e2e     # single spec
#   make e2e-anthropic SPEC=dm-chat-integration
#   make e2e-openai SPEC=quest-lifecycle

e2e-gemini: e2e-install e2e-nats-clean
	SEED_E2E=true BACKEND_PORT=$(BACKEND_PORT) \
		SEMDRAGONS_E2E_CONFIG=$${SEMDRAGONS_E2E_CONFIG:-semdragons-e2e-gemini.json} \
		docker compose $(CLOUD_COMPOSE) up -d --build --wait
	@$(MAKE) e2e-wait
	cd ui && E2E_LLM_MODE=gemini BACKEND_PORT=$(BACKEND_PORT) npx playwright test $(_pw_args) || true
	@$(MAKE) _e2e-cloud-down

e2e-anthropic: e2e-install e2e-nats-clean
	SEED_E2E=true BACKEND_PORT=$(BACKEND_PORT) \
		SEMDRAGONS_E2E_CONFIG=semdragons-e2e-anthropic.json \
		docker compose $(CLOUD_COMPOSE) up -d --build --wait
	@$(MAKE) e2e-wait
	cd ui && E2E_LLM_MODE=anthropic BACKEND_PORT=$(BACKEND_PORT) npx playwright test $(_pw_args) || true
	@$(MAKE) _e2e-cloud-down

e2e-openai: e2e-install e2e-nats-clean
	SEED_E2E=true BACKEND_PORT=$(BACKEND_PORT) \
		SEMDRAGONS_E2E_CONFIG=semdragons-e2e-openai.json \
		docker compose $(CLOUD_COMPOSE) up -d --build --wait
	@$(MAKE) e2e-wait
	cd ui && E2E_LLM_MODE=openai BACKEND_PORT=$(BACKEND_PORT) npx playwright test $(_pw_args) || true
	@$(MAKE) _e2e-cloud-down

# ─── Ollama (local LLM) ───────────────────────────────────────────

e2e-ollama: e2e-install e2e-nats-clean
	SEED_E2E=true BACKEND_PORT=$(BACKEND_PORT) \
		docker compose $(OLLAMA_COMPOSE) up -d --build --wait
	@$(MAKE) e2e-wait
	cd ui && E2E_LLM_MODE=ollama BACKEND_PORT=$(BACKEND_PORT) npx playwright test $(_pw_args) || true
	@$(MAKE) _e2e-ollama-down

# ─── Single Spec Runner (against running stack) ───────────────────
#
# Usage:
#   make e2e-spec SPEC=party-quest-dag-e2e             # mock LLM
#   make e2e-spec SPEC=dm-chat-integration MODE=gemini  # real LLM

MODE ?= mock
e2e-spec:
ifndef SPEC
	$(error SPEC is required. Usage: make e2e-spec SPEC=party-quest-dag-e2e)
endif
	cd ui && E2E_LLM_MODE=$(MODE) BACKEND_PORT=$(BACKEND_PORT) npx playwright test $(SPEC) --project=chromium

# ─── Utilities ─────────────────────────────────────────────────────

e2e-nats-clean:
	@docker compose --profile mock down -v --remove-orphans 2>/dev/null || true
	@docker compose $(CLOUD_COMPOSE) down -v --remove-orphans 2>/dev/null || true
	@echo "NATS volumes wiped."

_e2e-cloud-down:
	docker compose $(CLOUD_COMPOSE) down -v --remove-orphans
	@echo "Cloud E2E stack stopped."

_e2e-ollama-down:
	docker compose $(OLLAMA_COMPOSE) down -v --remove-orphans
	@echo "Ollama E2E stack stopped."

e2e-logs:
	docker compose logs -f backend

e2e-logs-all:
	docker compose logs -f

e2e-help:
	@echo ""
	@echo "E2E Test Targets"
	@echo "================"
	@echo ""
	@echo "  make e2e                    Mock LLM, all specs (no API key needed)"
	@echo "  make e2e-gemini             Gemini cloud, all specs"
	@echo "  make e2e-anthropic          Anthropic cloud, all specs"
	@echo "  make e2e-openai             OpenAI cloud, all specs"
	@echo "  make e2e-ollama             Local Ollama, all specs"
	@echo ""
	@echo "  Add SPEC=<name> to run a single spec:"
	@echo "    make e2e-gemini SPEC=quest-lifecycle"
	@echo "    make e2e-anthropic SPEC=dm-chat-integration"
	@echo ""
	@echo "  Manual stack control:"
	@echo "    make e2e-up               Start mock stack"
	@echo "    make e2e-down             Stop mock stack"
	@echo "    make e2e-spec SPEC=x      Run spec against running stack"
	@echo "    make e2e-spec SPEC=x MODE=gemini"
	@echo ""
	@echo "  Utilities:"
	@echo "    make e2e-nats-clean        Wipe NATS volumes"
	@echo "    make e2e-logs              Tail backend logs"
	@echo "    make e2e-report            Open Playwright report"
	@echo ""
	@echo "  API keys: set in .env or environment"
	@echo "    GEMINI_API_KEY, ANTHROPIC_API_KEY, OPENAI_API_KEY"
	@echo ""
