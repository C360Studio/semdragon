.PHONY: build test test-integration test-all lint fmt tidy check e2e e2e-setup e2e-teardown e2e-headed e2e-clean

# Build all packages
build:
	go build ./...

# Run unit tests only (fast, no Docker required)
test:
	go test -v ./...

# Run integration tests only (requires Docker for NATS)
test-integration:
	go test -v -tags=integration ./...

# Run all tests (unit + integration)
test-all:
	go test -v -tags=integration ./...

# Run tests with race detection (includes integration tests)
test-race:
	go test -race -v -tags=integration ./...

# Run a specific test
# Usage: make test-one TEST=TestFunctionName
test-one:
	go test -v -run $(TEST) ./...

# Run a specific test with integration tag
# Usage: make test-one-integration TEST=TestFunctionName
test-one-integration:
	go test -v -tags=integration -run $(TEST) ./...

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
	go test -tags=integration -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# =============================================================================
# E2E Testing (Playwright)
# =============================================================================

# Run E2E tests (starts services, runs tests, tears down)
e2e: e2e-setup
	cd ui && npm run test:e2e
	$(MAKE) e2e-teardown

# Start services with E2E seeding enabled
e2e-setup:
	cd ui && SEED_E2E=true docker compose up -d
	cd ui && npx wait-on http://localhost:8080/health -t 60000
	cd ui && npx wait-on http://localhost:5173 -t 60000

# Stop services and remove volumes for clean state
e2e-teardown:
	cd ui && docker compose down -v

# Run E2E tests in headed mode (shows browser)
e2e-headed: e2e-setup
	cd ui && npm run test:e2e:headed
	$(MAKE) e2e-teardown

# Run E2E tests with UI mode (interactive debugging)
e2e-ui: e2e-setup
	cd ui && npm run test:e2e:ui

# Force clean slate (removes volumes)
e2e-clean:
	cd ui && docker compose down -v

# View E2E test report
e2e-report:
	cd ui && npm run test:e2e:report

# Install Playwright browsers
e2e-install:
	cd ui && npx playwright install
