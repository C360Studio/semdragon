# Semdragons UI — Build & Test Targets
#
# Usage:
#   make test          — Run unit tests (vitest, no Docker needed)
#   make check         — Run type-check + lint
#   make e2e           — Full E2E: start stack, run Playwright, tear down
#   make e2e-up        — Start Docker E2E stack (nats + backend + ui)
#   make e2e-down      — Stop Docker E2E stack
#   make e2e-run       — Run Playwright tests (assumes stack is already up)
#   make e2e-chromium  — Run Playwright on chromium only (fast iteration)
#   make e2e-ollama    — Full Ollama E2E: start stack, run tests, tear down
#   make e2e-ollama-up — Start Ollama E2E stack (requires host Ollama)
#   make e2e-ollama-run — Run Ollama integration spec only
#   make e2e-ollama-down — Stop Ollama E2E stack

.PHONY: test check check-schema e2e e2e-up e2e-down e2e-run e2e-chromium e2e-install dev \
       e2e-ollama e2e-ollama-up e2e-ollama-run e2e-ollama-down

# ─── Unit Tests ───────────────────────────────────────────────────────

test:
	npm run test -- --run

check:
	npm run check

# Verify entity schema matches Go code (delegates to root Makefile)
check-schema:
	cd .. && make check-schema

# ─── Dev Server ───────────────────────────────────────────────────────

dev:
	npm run dev

# ─── E2E: Full Lifecycle ─────────────────────────────────────────────

e2e: e2e-install e2e-up e2e-wait e2e-run e2e-down

# Start the Docker stack (nats + backend + dev UI)
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

# Run Playwright tests (frontend must be running)
e2e-run:
	npx playwright test

# Run on chromium only (fast iteration during development)
e2e-chromium:
	npx playwright test --project=chromium

# Stop the Docker stack
e2e-down:
	docker compose down -v
	@echo "E2E stack stopped."

# Install Playwright browsers (first-time setup)
e2e-install:
	npx playwright install --with-deps chromium

# ─── E2E: Ollama Integration (real LLM) ────────────────────────────

OLLAMA_COMPOSE = -f docker-compose.yml -f docker-compose.ollama.yml

e2e-ollama: e2e-install e2e-ollama-up e2e-wait e2e-ollama-run e2e-ollama-down

# Start stack with Ollama config (no mockllm, points at host Ollama)
e2e-ollama-up:
	SEED_E2E=true docker compose $(OLLAMA_COMPOSE) up -d --build --wait
	@echo "Ollama E2E stack is up. Backend: http://localhost:8080  UI: http://localhost:5173"

# Run only the Ollama integration spec (chromium only — API-level tests)
e2e-ollama-run:
	E2E_OLLAMA=true npx playwright test ollama-integration --project=chromium

# Stop the Ollama stack
e2e-ollama-down:
	docker compose $(OLLAMA_COMPOSE) down -v
	@echo "Ollama E2E stack stopped."
