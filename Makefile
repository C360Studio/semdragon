.PHONY: build test test-integration test-all lint fmt tidy check

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
	golangci-lint run

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
