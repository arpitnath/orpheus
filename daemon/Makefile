.PHONY: all build build-runtime build-daemon build-daemon-linux-arm64 build-daemon-linux-amd64 build-daemon-all build-cli test test-go test-integration clean install dev-setup

# Variables
BINARY_NAME=orpheus-runtime
DAEMON_BINARY=orpheusd
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.Version=$(VERSION)"

all: build

# Build Go runtime binary
build-runtime:
	@echo "Building runtime..."
	@go build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/orpheus-runtime

# Build daemon (macOS/current platform)
build-daemon:
	@echo "Building daemon for current platform..."
	@go build $(LDFLAGS) -o bin/$(DAEMON_BINARY) ./cmd/orpheusd

# Build daemon for Linux ARM64 (Apple Silicon Lima VM)
# Note: Uses pure-Go SQLite (modernc.org/sqlite) for cross-compilation
build-daemon-linux-arm64:
	@echo "Building daemon for Linux ARM64..."
	@GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build $(LDFLAGS) -o bin/$(DAEMON_BINARY)-linux-arm64 ./cmd/orpheusd

# Build daemon for Linux AMD64 (Intel Lima VM)
# Note: Uses pure-Go SQLite (modernc.org/sqlite) for cross-compilation
build-daemon-linux-amd64:
	@echo "Building daemon for Linux AMD64..."
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(LDFLAGS) -o bin/$(DAEMON_BINARY)-linux-amd64 ./cmd/orpheusd

# Build all daemon binaries (current platform + Linux ARM64 + AMD64)
build-daemon-all: build-daemon build-daemon-linux-arm64 build-daemon-linux-amd64
	@echo "All daemon binaries built successfully"

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
	@mkdir -p ~/.orpheus/bin
	@cp bin/$(BINARY_NAME) ~/.orpheus/bin/
	@cd cli && pip install .
	@echo "Installed to ~/.orpheus/bin/"

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
