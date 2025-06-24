# AgentSmith-HUB Makefile
BINARY_NAME=agentsmith-hub
BUILD_DIR=build
DIST_DIR=dist
FRONTEND_DIR=web
BACKEND_DIR=src

# Version information
VERSION=$(shell cat VERSION 2>/dev/null || echo "unknown")
BUILD_TIME=$(shell date -u '+%Y-%m-%d %H:%M:%S UTC')
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS=-ldflags "-s -w -X 'main.Version=$(VERSION)' -X 'main.BuildTime=$(BUILD_TIME)' -X 'main.GitCommit=$(GIT_COMMIT)'"

# Build configuration - always target Linux
UNAME_S := $(shell uname -s)
TARGET_GOOS=linux
TARGET_GOARCH=amd64
LIB_PATH=lib/linux

.PHONY: all clean backend backend-docker frontend package deploy install-deps help

# Default: build for Linux (production)
all: clean backend frontend package

install-deps:
	@echo "Installing dependencies..."
	cd $(BACKEND_DIR) && go mod download
	cd $(FRONTEND_DIR) && npm install

# Build backend for Linux (smart build based on host platform)
backend:
	@echo "Building backend for Linux..."
	mkdir -p $(BUILD_DIR)
	@if [ "$(UNAME_S)" = "Darwin" ]; then \
		echo "Cross-compiling from macOS to Linux..."; \
		echo "Attempting CGO cross-compilation (may fail, use 'make backend-docker' if needed)..."; \
		cd $(BACKEND_DIR) && \
		CGO_ENABLED=0 GOOS=$(TARGET_GOOS) GOARCH=$(TARGET_GOARCH) \
		go build $(LDFLAGS) -o ../$(BUILD_DIR)/$(BINARY_NAME) . || \
		(echo "Cross-compilation failed. Please use: make backend-docker" && exit 1); \
	else \
		echo "Building on Linux natively..."; \
		cd $(BACKEND_DIR) && \
		CGO_ENABLED=1 \
		CGO_LDFLAGS="-L$(PWD)/$(LIB_PATH) -lrure" \
		GOOS=$(TARGET_GOOS) GOARCH=$(TARGET_GOARCH) \
		go build $(LDFLAGS) -o ../$(BUILD_DIR)/$(BINARY_NAME) .; \
	fi

# Build backend using Docker (recommended for macOS)
backend-docker:
	@echo "Building backend in Docker (Linux target)..."
	@if ! command -v docker >/dev/null 2>&1; then \
		echo "Docker is not installed. Please install Docker."; \
		exit 1; \
	fi
	mkdir -p $(BUILD_DIR)
	docker run --rm -v "$(PWD):/workspace" -w /workspace/$(BACKEND_DIR) \
		-e CGO_ENABLED=1 \
		-e GOOS=linux \
		-e GOARCH=amd64 \
		-e CGO_LDFLAGS="-L/workspace/lib/linux -lrure" \
		golang:1.24.3 \
		sh -c "apt-get update && apt-get install -y build-essential && go build -ldflags \"-s -w -X 'main.Version=$(VERSION)' -X 'main.BuildTime=$(BUILD_TIME)' -X 'main.GitCommit=$(GIT_COMMIT)'\" -o ../$(BUILD_DIR)/$(BINARY_NAME) ."

frontend:
	@echo "Building frontend..."
	cd $(FRONTEND_DIR) && npm run build
	mkdir -p $(BUILD_DIR)/web
	cp -r $(FRONTEND_DIR)/dist/* $(BUILD_DIR)/web/

# Package everything for Linux deployment
package: backend frontend
	@echo "Packaging for Linux deployment..."
	mkdir -p $(DIST_DIR)
	@echo "Copying backend binary..."
	cp $(BUILD_DIR)/$(BINARY_NAME) $(DIST_DIR)/
	@echo "Copying frontend files..."
	cp -r $(BUILD_DIR)/web $(DIST_DIR)/
	@echo "Copying required libraries (Linux .so)..."
	mkdir -p $(DIST_DIR)/lib
	cp -r $(LIB_PATH)/* $(DIST_DIR)/lib/
	@echo "Copying config directory..."
	cp -r config $(DIST_DIR)/
	@echo "Creating scripts..."
	./script/create_scripts.sh $(DIST_DIR)
	@echo ""
	@echo "=== Linux Package Complete ==="
	@echo "Deployment files are ready in: $(DIST_DIR)/"
	@echo "- Backend binary: $(BINARY_NAME) (Linux amd64)"
	@echo "- Frontend files: web/"
	@echo "- Libraries: lib/ (Linux .so files)"
	@echo "- Configuration: config/"
	@echo "- Scripts: start.sh, stop.sh"
	@echo ""
	@echo "To deploy:"
	@echo "1. Copy $(DIST_DIR)/ to Linux target server"
	@echo "2. Run ./start.sh to start services"
	@echo "3. Run ./stop.sh to stop services"

deploy: package
	@echo "Creating deployment archive..."
	cd $(DIST_DIR) && tar --create --gzip --file=../agentsmith-hub-deployment.tar.gz .
	@echo "Deployment archive created: agentsmith-hub-deployment.tar.gz"

clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	rm -rf $(DIST_DIR)
	rm -f agentsmith-hub-deployment.tar.gz

dev-backend:
	@echo "Starting backend in development mode (current platform)..."
	cd $(BACKEND_DIR) && go run $(LDFLAGS) . -config_root ../config

dev-frontend:
	@echo "Starting frontend in development mode..."
	cd $(FRONTEND_DIR) && npm run dev

dev: install-deps
	@echo "Development setup complete."
	@echo "Run 'make dev-backend' and 'make dev-frontend' in separate terminals"

help:
	@echo "AgentSmith-HUB Build System"
	@echo ""
	@echo "Main Targets:"
	@echo "  all              - Build everything for Linux deployment"
	@echo "  backend          - Build backend for Linux (smart: cross-compile or native)"
	@echo "  backend-docker   - Build backend using Docker (recommended for macOS)"
	@echo "  frontend         - Build frontend for production"
	@echo "  package          - Package everything for Linux deployment"
	@echo "  deploy           - Create deployment archive"
	@echo ""
	@echo "Development:"
	@echo "  install-deps     - Install dependencies"
	@echo "  dev-backend      - Run backend in development mode"
	@echo "  dev-frontend     - Run frontend in development mode"
	@echo "  clean            - Clean build artifacts"
	@echo ""
	@echo "Platform Support:"
	@echo "  - Build Host: macOS or Linux"
	@echo "  - Build Target: Linux amd64 (always)"
	@echo "  - macOS: Uses cross-compilation (or Docker if CGO fails)"
	@echo "  - Linux: Uses native compilation"
	@echo ""
	@echo "Quick Start:"
	@echo "  make all         # Build everything for Linux"
	@echo "  make deploy      # Create deployment archive"
	@echo ""
	@echo "Deployment:"
	@echo "  1. Run 'make all' to build for Linux"
	@echo "  2. Copy $(DIST_DIR)/ to Linux target server"
	@echo "  3. Run './start.sh' to start services"
	@echo "  4. Run './stop.sh' to stop services" 