.PHONY: all build build-runtime build-cli test test-go test-integration clean install dev-setup

# Variables
BINARY_NAME=agentscale-runtime
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.Version=$(VERSION)"

all: build

# Build Go runtime binary
build-runtime:
	@echo "Building runtime..."
	@go build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/agentscale-runtime

# Build Python CLI (install in editable mode)
build-cli:
	@echo "Building CLI..."
	@cd cli && pip install -e .

# Build all
build: build-runtime build-cli

# Run Go tests
test-go:
	@echo "Running Go tests..."
	@go test -v ./...

# Run integration tests
test-integration:
	@echo "Running integration tests..."
	@./scripts/test_integration.sh

# Run all tests
test: test-go test-integration

# Clean build artifacts
clean:
	@rm -rf bin/
	@find . -name "_entrypoint.py" -delete
	@find . -name "*.pyc" -delete
	@find . -name "__pycache__" -type d -exec rm -rf {} + 2>/dev/null || true

# Install locally
install: build
	@mkdir -p ~/.agentscale/bin
	@cp bin/$(BINARY_NAME) ~/.agentscale/bin/
	@cd cli && pip install .
	@echo "Installed to ~/.agentscale/bin/"

# Development setup
dev-setup:
	@go mod tidy
	@cd cli && pip install -e ".[dev]" 2>/dev/null || pip install -e .

# Run example (for testing)
example:
	@echo '{"query": "test"}' | ./bin/$(BINARY_NAME) run ./examples/simple-agent --no-isolate

# Format code
fmt:
	@go fmt ./...

# Check for issues
vet:
	@go vet ./...
