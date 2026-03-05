# Semdragons UI — Frontend Targets
#
# Usage:
#   make test          — Run unit tests (vitest, no Docker needed)
#   make check         — Run type-check + lint
#   make dev           — Start Vite dev server
#
# For E2E tests (Playwright + Docker), use the root Makefile:
#   make e2e           — Full E2E lifecycle
#   make e2e-cloud     — Cloud LLM E2E (Gemini/Anthropic/OpenAI)
#   make e2e-ollama    — Ollama E2E (local LLM)

.PHONY: test check check-schema dev

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
