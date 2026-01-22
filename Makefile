# =============================================================================
# Orpheus Makefile
# =============================================================================
#
# Build and deployment automation for Orpheus - Infrastructure for AI Agents
#
# Usage:
#   make help              Show all available commands
#   make build             Build for current platform
#   make build-linux-amd64 Cross-compile for Linux x86_64
#   make deploy            Deploy to server
#   make test              Run tests
#
# =============================================================================

# Configuration
BINARY_NAME := orpheusd
CLI_NAME := orpheus
BUILD_DIR := bin
CORE_DIR := core
GO_VERSION := 1.21

# Version (from git tag or commit)
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

# Platform detection
UNAME_S := $(shell uname -s)
UNAME_M := $(shell uname -m)

# Server configuration (set via environment or command line)
SERVER ?=
SSH_KEY ?=
ARCH ?= amd64

# SSH options
ifdef SSH_KEY
  SSH_OPTS := -i $(SSH_KEY) -o StrictHostKeyChecking=no
else
  SSH_OPTS := -o StrictHostKeyChecking=no
endif

# Colors for output
CYAN := \033[36m
GREEN := \033[32m
YELLOW := \033[33m
RED := \033[31m
RESET := \033[0m

# =============================================================================
# Help
# =============================================================================

.PHONY: help
help:
	@echo ""
	@echo "$(CYAN)Orpheus Build System$(RESET)"
	@echo "$(CYAN)=====================$(RESET)"
	@echo ""
	@echo "$(GREEN)Local Development:$(RESET)"
	@echo "  make build              Build daemon for current platform"
	@echo "  make build-cli          Build CLI (requires Node.js)"
	@echo "  make test               Run all tests"
	@echo "  make test-core          Run Go tests only"
	@echo "  make clean              Remove build artifacts"
	@echo ""
	@echo "$(GREEN)Cross-Compilation:$(RESET)"
	@echo "  make build-linux-amd64  Build for Linux x86_64 (EC2, most VPS)"
	@echo "  make build-linux-arm64  Build for Linux ARM64 (Graviton, ARM VPS)"
	@echo "  make build-darwin-arm64 Build for macOS Apple Silicon"
	@echo "  make build-all          Build for all platforms"
	@echo ""
	@echo "$(GREEN)Deployment:$(RESET)"
	@echo "  make deploy SERVER=user@host [SSH_KEY=~/.ssh/key.pem] [ARCH=arm64]"
	@echo "                          Deploy daemon binary to server"
	@echo "  make deploy-systemd     Deploy + install systemd service"
	@echo "  make logs SERVER=...    View daemon logs on server"
	@echo ""
	@echo "$(GREEN)Server Setup (run ON the server):$(RESET)"
	@echo "  make server-deps        Install podman, runc"
	@echo "  make server-start       Start daemon (foreground)"
	@echo "  make server-status      Check daemon health"
	@echo ""
	@echo "$(GREEN)Examples:$(RESET)"
	@echo "  # Build and deploy to EC2"
	@echo "  make build-linux-amd64"
	@echo "  make deploy SERVER=ubuntu@44.221.80.248 SSH_KEY=~/.ssh/key.pem"
	@echo ""
	@echo "  # Check server health"
	@echo "  make server-status SERVER=ubuntu@44.221.80.248"
	@echo ""

# =============================================================================
# Local Development
# =============================================================================

$(BUILD_DIR):
	@mkdir -p $(BUILD_DIR)

.PHONY: build
build: $(BUILD_DIR)
	@echo "$(CYAN)Building $(BINARY_NAME) for $(UNAME_S)/$(UNAME_M)...$(RESET)"
	cd $(CORE_DIR) && go build $(LDFLAGS) -o ../$(BUILD_DIR)/$(BINARY_NAME) ./cmd/orpheusd
	@echo "$(GREEN)Built: $(BUILD_DIR)/$(BINARY_NAME)$(RESET)"

.PHONY: build-cli
build-cli:
	@echo "$(CYAN)Building CLI...$(RESET)"
	cd cli && npm install && npm run build
	@echo "$(GREEN)CLI built$(RESET)"

.PHONY: test
test: test-core
	@echo "$(GREEN)All tests passed$(RESET)"

.PHONY: test-core
test-core:
	@echo "$(CYAN)Running Go tests...$(RESET)"
	cd $(CORE_DIR) && go test -v ./...

.PHONY: clean
clean:
	@echo "$(CYAN)Cleaning build artifacts...$(RESET)"
	rm -rf $(BUILD_DIR)
	@echo "$(GREEN)Clean$(RESET)"

# =============================================================================
# Cross-Compilation (using Podman for consistent builds)
# =============================================================================

.PHONY: build-linux-amd64
build-linux-amd64: $(BUILD_DIR)
	@echo "$(CYAN)Building for Linux x86_64 (using Podman)...$(RESET)"
	@if ! command -v podman &> /dev/null; then \
		echo "$(YELLOW)Podman not found, using native Go cross-compile$(RESET)"; \
		cd $(CORE_DIR) && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o ../$(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/orpheusd; \
	else \
		podman run --rm \
			-v $(PWD)/$(CORE_DIR):/src:Z \
			-w /src \
			docker.io/golang:$(GO_VERSION) \
			sh -c "CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags '-X main.Version=$(VERSION)' -o $(BINARY_NAME) ./cmd/orpheusd"; \
		mv $(CORE_DIR)/$(BINARY_NAME) $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64; \
	fi
	@echo "$(GREEN)Built: $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64$(RESET)"

.PHONY: build-linux-arm64
build-linux-arm64: $(BUILD_DIR)
	@echo "$(CYAN)Building for Linux ARM64 (using Podman)...$(RESET)"
	@if ! command -v podman &> /dev/null; then \
		echo "$(YELLOW)Podman not found, using native Go cross-compile$(RESET)"; \
		cd $(CORE_DIR) && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o ../$(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/orpheusd; \
	else \
		podman run --rm \
			-v $(PWD)/$(CORE_DIR):/src:Z \
			-w /src \
			docker.io/golang:$(GO_VERSION) \
			sh -c "CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags '-X main.Version=$(VERSION)' -o $(BINARY_NAME) ./cmd/orpheusd"; \
		mv $(CORE_DIR)/$(BINARY_NAME) $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64; \
	fi
	@echo "$(GREEN)Built: $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64$(RESET)"

.PHONY: build-darwin-arm64
build-darwin-arm64: $(BUILD_DIR)
	@echo "$(CYAN)Building for macOS ARM64...$(RESET)"
	cd $(CORE_DIR) && CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o ../$(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/orpheusd
	@echo "$(GREEN)Built: $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64$(RESET)"

.PHONY: build-darwin-amd64
build-darwin-amd64: $(BUILD_DIR)
	@echo "$(CYAN)Building for macOS x86_64...$(RESET)"
	cd $(CORE_DIR) && CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o ../$(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/orpheusd
	@echo "$(GREEN)Built: $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64$(RESET)"

.PHONY: build-all
build-all: build-linux-amd64 build-linux-arm64 build-darwin-arm64 build-darwin-amd64
	@echo ""
	@echo "$(GREEN)All platforms built:$(RESET)"
	@ls -lh $(BUILD_DIR)/

# =============================================================================
# Deployment
# =============================================================================

.PHONY: deploy
deploy: _check-server
	@BINARY=$(BUILD_DIR)/$(BINARY_NAME)-linux-$(ARCH); \
	if [ ! -f "$$BINARY" ]; then \
		echo "$(YELLOW)Binary not found. Building first...$(RESET)"; \
		$(MAKE) build-linux-$(ARCH); \
	fi; \
	echo "$(CYAN)Deploying to $(SERVER)...$(RESET)"; \
	scp $(SSH_OPTS) $$BINARY $(SERVER):/tmp/orpheusd; \
	ssh $(SSH_OPTS) $(SERVER) "sudo mv /tmp/orpheusd /usr/local/bin/orpheusd && sudo chmod +x /usr/local/bin/orpheusd"; \
	echo "$(GREEN)Deployed!$(RESET)"; \
	echo ""; \
	echo "$(CYAN)Next steps:$(RESET)"; \
	echo "  1. SSH to server: ssh $(SERVER)"; \
	echo "  2. Install deps (first time): sudo apt install -y podman runc"; \
	echo "  3. Start daemon: orpheusd --tcp-bind 0.0.0.0:7777"; \
	echo ""; \
	echo "  Or use systemd: make deploy-systemd SERVER=$(SERVER)"

.PHONY: deploy-systemd
deploy-systemd: deploy
	@echo "$(CYAN)Installing systemd service...$(RESET)"
	scp $(SSH_OPTS) scripts/orpheusd.service $(SERVER):/tmp/
	ssh $(SSH_OPTS) $(SERVER) "sudo mv /tmp/orpheusd.service /etc/systemd/system/ && sudo systemctl daemon-reload && sudo systemctl enable orpheusd"
	@echo "$(GREEN)Systemd service installed$(RESET)"
	@echo ""
	@echo "$(CYAN)Commands:$(RESET)"
	@echo "  Start:   ssh $(SERVER) 'sudo systemctl start orpheusd'"
	@echo "  Stop:    ssh $(SERVER) 'sudo systemctl stop orpheusd'"
	@echo "  Status:  ssh $(SERVER) 'sudo systemctl status orpheusd'"
	@echo "  Logs:    ssh $(SERVER) 'sudo journalctl -u orpheusd -f'"

.PHONY: restart
restart: _check-server
	@echo "$(CYAN)Restarting daemon on $(SERVER)...$(RESET)"
	ssh $(SSH_OPTS) $(SERVER) "sudo systemctl restart orpheusd"
	@sleep 2
	@$(MAKE) server-status SERVER=$(SERVER)

.PHONY: logs
logs: _check-server
	@echo "$(CYAN)Streaming logs from $(SERVER)...$(RESET)"
	ssh $(SSH_OPTS) $(SERVER) "sudo journalctl -u orpheusd -f"

# =============================================================================
# Server Setup (run ON the server)
# =============================================================================

.PHONY: server-deps
server-deps:
	@echo "$(CYAN)Installing server dependencies...$(RESET)"
	sudo apt-get update
	sudo apt-get install -y podman runc
	@echo "$(GREEN)Dependencies installed$(RESET)"

.PHONY: server-start
server-start:
	@echo "$(CYAN)Starting Orpheus daemon...$(RESET)"
	orpheusd --tcp-bind 0.0.0.0:7777

.PHONY: server-status
server-status:
ifdef SERVER
	@echo "$(CYAN)Checking $(SERVER) health...$(RESET)"
	@ssh $(SSH_OPTS) $(SERVER) "curl -s http://localhost:7777/v1/health" | jq . || echo "$(RED)Daemon not responding$(RESET)"
else
	@echo "$(CYAN)Checking local daemon health...$(RESET)"
	@curl -s http://localhost:7777/v1/health | jq . || echo "$(RED)Daemon not responding$(RESET)"
endif

# =============================================================================
# Utilities
# =============================================================================

.PHONY: _check-server
_check-server:
ifndef SERVER
	$(error $(RED)SERVER is required. Usage: make deploy SERVER=user@host [SSH_KEY=~/.ssh/key.pem] [ARCH=arm64]$(RESET))
endif

.PHONY: version
version:
	@echo "Version: $(VERSION)"
	@echo "Build Time: $(BUILD_TIME)"

# =============================================================================
# Development Shortcuts
# =============================================================================

.PHONY: dev
dev: build
	@echo "$(CYAN)Starting daemon in development mode...$(RESET)"
	$(BUILD_DIR)/$(BINARY_NAME) --tcp-bind 127.0.0.1:7777

.PHONY: fmt
fmt:
	@echo "$(CYAN)Formatting Go code...$(RESET)"
	cd $(CORE_DIR) && go fmt ./...

.PHONY: lint
lint:
	@echo "$(CYAN)Linting Go code...$(RESET)"
	cd $(CORE_DIR) && go vet ./...
