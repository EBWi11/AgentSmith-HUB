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
UNAME_M := $(shell uname -m)
TARGET_GOOS=linux

# Architecture detection and mapping
ifeq ($(UNAME_M),x86_64)
	HOST_ARCH=amd64
else ifeq ($(UNAME_M),aarch64)
	HOST_ARCH=arm64
else ifeq ($(UNAME_M),arm64)
	HOST_ARCH=arm64
else
	HOST_ARCH=amd64
endif

# Default target architecture (can be overridden)
TARGET_GOARCH ?= $(HOST_ARCH)
LIB_PATH=lib/linux/$(TARGET_GOARCH)

.PHONY: all clean backend backend-docker frontend package deploy install-deps help build-all-arch

# Default: build for current architecture
all: clean backend frontend package

# Build for all architectures
build-all-arch: clean frontend package-amd64 package-arm64

install-deps:
	@echo "Installing dependencies..."
	cd $(BACKEND_DIR) && go mod download
	cd $(FRONTEND_DIR) && npm install

# Build backend for specific architecture
backend:
	@echo "Building backend for Linux $(TARGET_GOARCH)..."
	mkdir -p $(BUILD_DIR)
	@if [ "$(UNAME_S)" = "Darwin" ]; then \
		echo "Cross-compiling from macOS to Linux $(TARGET_GOARCH)..."; \
		if [ "$(TARGET_GOARCH)" = "arm64" ]; then \
			echo "Cross-compiling to ARM64 (CGO disabled for compatibility)..."; \
			cd $(BACKEND_DIR) && \
			CGO_ENABLED=0 GOOS=$(TARGET_GOOS) GOARCH=$(TARGET_GOARCH) \
			go build $(LDFLAGS) -o ../$(BUILD_DIR)/$(BINARY_NAME)-$(TARGET_GOARCH) . || \
			(echo "Cross-compilation failed. Please use: make backend-docker TARGET_GOARCH=$(TARGET_GOARCH)" && exit 1); \
		else \
			echo "Cross-compiling to AMD64 (CGO disabled for compatibility)..."; \
			cd $(BACKEND_DIR) && \
			CGO_ENABLED=0 GOOS=$(TARGET_GOOS) GOARCH=$(TARGET_GOARCH) \
			go build $(LDFLAGS) -o ../$(BUILD_DIR)/$(BINARY_NAME)-$(TARGET_GOARCH) . || \
			(echo "Cross-compilation failed. Please use: make backend-docker TARGET_GOARCH=$(TARGET_GOARCH)" && exit 1); \
		fi; \
	else \
		echo "Building on Linux natively for $(TARGET_GOARCH)..."; \
		if [ "$(TARGET_GOARCH)" = "$(HOST_ARCH)" ]; then \
			echo "Native build for $(TARGET_GOARCH)..."; \
			cd $(BACKEND_DIR) && \
			CGO_ENABLED=1 \
			CGO_LDFLAGS="-L$(PWD)/$(LIB_PATH) -lrure" \
			GOOS=$(TARGET_GOOS) GOARCH=$(TARGET_GOARCH) \
			go build $(LDFLAGS) -o ../$(BUILD_DIR)/$(BINARY_NAME)-$(TARGET_GOARCH) .; \
		else \
			echo "Cross-compiling on Linux from $(HOST_ARCH) to $(TARGET_GOARCH)..."; \
			if [ "$(TARGET_GOARCH)" = "arm64" ]; then \
				cd $(BACKEND_DIR) && \
				CC=aarch64-linux-gnu-gcc \
				CGO_ENABLED=1 \
				CGO_LDFLAGS="-L$(PWD)/$(LIB_PATH) -lrure" \
				GOOS=$(TARGET_GOOS) GOARCH=$(TARGET_GOARCH) \
				go build $(LDFLAGS) -o ../$(BUILD_DIR)/$(BINARY_NAME)-$(TARGET_GOARCH) .; \
			else \
				cd $(BACKEND_DIR) && \
				CGO_ENABLED=1 \
				CGO_LDFLAGS="-L$(PWD)/$(LIB_PATH) -lrure" \
				GOOS=$(TARGET_GOOS) GOARCH=$(TARGET_GOARCH) \
				go build $(LDFLAGS) -o ../$(BUILD_DIR)/$(BINARY_NAME)-$(TARGET_GOARCH) .; \
			fi; \
		fi; \
	fi

# Build backend using Docker (recommended for macOS and cross-compilation)
backend-docker:
	@echo "Building backend in Docker (Linux $(TARGET_GOARCH))..."
	@if ! command -v docker >/dev/null 2>&1; then \
		echo "Docker is not installed. Please install Docker."; \
		exit 1; \
	fi
	mkdir -p $(BUILD_DIR)
	@if [ "$(TARGET_GOARCH)" = "arm64" ]; then \
		echo "Building for ARM64 architecture..."; \
		docker run --rm -v "$(PWD):/workspace" -w /workspace/$(BACKEND_DIR) \
			-e CGO_ENABLED=1 \
			-e CC=aarch64-linux-gnu-gcc \
			-e GOOS=linux \
			-e GOARCH=arm64 \
			-e CGO_LDFLAGS="-L/workspace/lib/linux/arm64 -lrure" \
			golang:1.21 \
			sh -c "apt-get update && apt-get install -y build-essential gcc-aarch64-linux-gnu && go build -ldflags \"-s -w -X 'main.Version=$(VERSION)' -X 'main.BuildTime=$(BUILD_TIME)' -X 'main.GitCommit=$(GIT_COMMIT)'\" -o ../$(BUILD_DIR)/$(BINARY_NAME)-arm64 ."; \
	else \
		echo "Building for AMD64 architecture..."; \
		docker run --rm -v "$(PWD):/workspace" -w /workspace/$(BACKEND_DIR) \
			-e CGO_ENABLED=1 \
			-e GOOS=linux \
			-e GOARCH=amd64 \
			-e CGO_LDFLAGS="-L/workspace/lib/linux/amd64 -lrure" \
			golang:1.21 \
			sh -c "apt-get update && apt-get install -y build-essential && go build -ldflags \"-s -w -X 'main.Version=$(VERSION)' -X 'main.BuildTime=$(BUILD_TIME)' -X 'main.GitCommit=$(GIT_COMMIT)'\" -o ../$(BUILD_DIR)/$(BINARY_NAME)-amd64 ."; \
	fi

# Build for specific architectures
backend-amd64:
	@$(MAKE) backend TARGET_GOARCH=amd64

backend-arm64:
	@$(MAKE) backend TARGET_GOARCH=arm64

backend-docker-amd64:
	@$(MAKE) backend-docker TARGET_GOARCH=amd64

backend-docker-arm64:
	@$(MAKE) backend-docker TARGET_GOARCH=arm64

frontend:
	@echo "Building frontend..."
	cd $(FRONTEND_DIR) && npm run build
	mkdir -p $(BUILD_DIR)/web
	cp -r $(FRONTEND_DIR)/dist/* $(BUILD_DIR)/web/

# Package for specific architecture
package-arch:
	@echo "Packaging for Linux $(TARGET_GOARCH) deployment..."
	mkdir -p $(DIST_DIR)/$(TARGET_GOARCH)
	@echo "Copying backend binary..."
	cp $(BUILD_DIR)/$(BINARY_NAME)-$(TARGET_GOARCH) $(DIST_DIR)/$(TARGET_GOARCH)/$(BINARY_NAME)
	chmod +x $(DIST_DIR)/$(TARGET_GOARCH)/$(BINARY_NAME)
	@echo "Copying frontend files..."
	cp -r $(BUILD_DIR)/web $(DIST_DIR)/$(TARGET_GOARCH)/
	@echo "Copying required libraries (Linux $(TARGET_GOARCH))..."
	mkdir -p $(DIST_DIR)/$(TARGET_GOARCH)/lib
	cp -r $(LIB_PATH)/* $(DIST_DIR)/$(TARGET_GOARCH)/lib/
	@echo "Copying config directory..."
	cp -r config $(DIST_DIR)/$(TARGET_GOARCH)/
	@echo "Copying MCP config directory..."
	cp -r mcp_config $(DIST_DIR)/$(TARGET_GOARCH)/
	@echo "Creating scripts..."
	./script/create_scripts.sh $(DIST_DIR)/$(TARGET_GOARCH) $(TARGET_GOARCH)
	@echo ""
	@echo "=== Linux $(TARGET_GOARCH) Package Complete ==="
	@echo "Deployment files are ready in: $(DIST_DIR)/$(TARGET_GOARCH)/"
	@echo "- Backend binary: $(BINARY_NAME) (Linux $(TARGET_GOARCH))"
	@echo "- Frontend files: web/"
	@echo "- Libraries: lib/ (Linux $(TARGET_GOARCH) .so files)"
	@echo "- Configuration: config/"
	@echo "- Scripts: start.sh, stop.sh"
	@echo ""

# Package for current architecture (backward compatibility)
package: backend frontend
	@$(MAKE) package-arch TARGET_GOARCH=$(TARGET_GOARCH)

# Package for specific architectures
package-amd64: backend-amd64 frontend
	@$(MAKE) package-arch TARGET_GOARCH=amd64

package-arm64: backend-arm64 frontend
	@$(MAKE) package-arch TARGET_GOARCH=arm64

deploy: package
	@echo "Creating deployment archive for $(TARGET_GOARCH)..."
	mkdir -p agentsmith-hub-$(TARGET_GOARCH)
	cp -r $(DIST_DIR)/$(TARGET_GOARCH)/* agentsmith-hub-$(TARGET_GOARCH)/
	tar czf agentsmith-hub-$(TARGET_GOARCH)-deployment.tar.gz agentsmith-hub-$(TARGET_GOARCH)
	rm -rf agentsmith-hub-$(TARGET_GOARCH)
	@echo "Deployment archive created: agentsmith-hub-$(TARGET_GOARCH)-deployment.tar.gz"

deploy-all: build-all-arch
	@echo "Creating deployment archives for all architectures..."
	@for arch in amd64 arm64; do \
		echo "Creating archive for $$arch..."; \
		mkdir -p agentsmith-hub-$$arch; \
		cp -r $(DIST_DIR)/$$arch/* agentsmith-hub-$$arch/; \
		tar czf agentsmith-hub-$$arch-deployment.tar.gz agentsmith-hub-$$arch; \
		rm -rf agentsmith-hub-$$arch; \
		echo "Archive created: agentsmith-hub-$$arch-deployment.tar.gz"; \
	done
	@echo "All deployment archives created successfully!"

clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	rm -rf $(DIST_DIR)
	rm -f agentsmith-hub-*-deployment.tar.gz

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
	@echo "  all              - Build everything for current architecture"
	@echo "  build-all-arch   - Build for all architectures (amd64 and arm64)"
	@echo "  backend          - Build backend for current architecture"
	@echo "  backend-amd64    - Build backend for AMD64"
	@echo "  backend-arm64    - Build backend for ARM64"
	@echo "  backend-docker   - Build backend using Docker for current architecture"
	@echo "  backend-docker-amd64 - Build backend using Docker for AMD64"
	@echo "  backend-docker-arm64 - Build backend using Docker for ARM64"
	@echo "  frontend         - Build frontend for production"
	@echo "  package          - Package everything for current architecture"
	@echo "  package-amd64    - Package for AMD64"
	@echo "  package-arm64    - Package for ARM64"
	@echo "  deploy           - Create deployment archive for current architecture"
	@echo "  deploy-all       - Create deployment archives for all architectures"
	@echo ""
	@echo "Development:"
	@echo "  install-deps     - Install dependencies"
	@echo "  dev-backend      - Run backend in development mode"
	@echo "  dev-frontend     - Run frontend in development mode"
	@echo "  clean            - Clean build artifacts"
	@echo ""
	@echo "Architecture Support:"
	@echo "  - Build Host: macOS or Linux"
	@echo "  - Build Target: Linux amd64/arm64"
	@echo "  - Current Host: $(UNAME_S) $(UNAME_M) ($(HOST_ARCH))"
	@echo "  - Default Target: $(TARGET_GOARCH)"
	@echo "  - Override with: make TARGET_GOARCH=arm64 <target>"
	@echo ""
	@echo "Cross-compilation:"
	@echo "  - macOS: Uses CGO_ENABLED=0 (or Docker for CGO)"
	@echo "  - Linux: Uses native or cross-compilation with CGO"
	@echo ""
	@echo "Quick Start:"
	@echo "  make all                    # Build for current architecture"
	@echo "  make build-all-arch         # Build for all architectures"
	@echo "  make deploy-all             # Create all deployment archives"
	@echo ""
	@echo "Deployment:"
	@echo "  1. Run 'make package-amd64' or 'make package-arm64'"
	@echo "  2. Copy $(DIST_DIR)/<arch>/ to Linux target server"
	@echo "  3. Run './start.sh' to start services"
	@echo "  4. Run './stop.sh' to stop services" 