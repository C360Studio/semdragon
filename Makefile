.PHONY: build test lint fmt tidy check

# Build all packages
build:
	go build ./...

# Run all tests
test:
	go test -v ./...

# Run tests with race detection
test-race:
	go test -race -v ./...

# Run a specific test
# Usage: make test-one TEST=TestFunctionName
test-one:
	go test -v -run $(TEST) ./...

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

# Full check: format, tidy, lint, test
check: fmt tidy lint test

# Coverage report
coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
