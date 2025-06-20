# AgentSmith-HUB Makefile
#
# Cross-platform build support for Go application with CGO dependencies
# 
# Notes:
# - This project requires librure library for different platforms
# - Currently available: lib/darwin/librure.a (ARM64), lib/linux/librure.so
# - For cross-compilation, you need the corresponding library for target platform
# - Use 'make check-available' to see what you can build on current system
#
# Quick start:
# - make build          # Build for current platform
# - make build-all      # Build for all platforms (requires all libraries)
# - make check-libs     # Check library availability

# Variables
BINARY_NAME=agentsmith-hub
BUILD_DIR=bin
SRC_DIR=src
LIB_DIR=lib
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME = $(shell date +%Y-%m-%dT%H:%M:%S)
LDFLAGS = -X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)

# Cross-compilation targets
.PHONY: all build build-darwin build-linux build-all clean test lint fmt deps dev help
.PHONY: build-nocgo build-darwin-nocgo build-linux-nocgo build-all-nocgo

# Default target
all: build

# Build for current platform
build:
	@echo "Building $(BINARY_NAME) for current platform..."
	@$(MAKE) build-current

# Detect current platform and build
build-current:
	@UNAME_S=$$(uname -s); \
	UNAME_M=$$(uname -m); \
	if [ "$$UNAME_S" = "Linux" ]; then \
		if [ "$$UNAME_M" = "aarch64" ] || [ "$$UNAME_M" = "arm64" ]; then \
			$(MAKE) build-linux-arm64; \
		else \
			$(MAKE) build-linux; \
		fi; \
	elif [ "$$UNAME_S" = "Darwin" ]; then \
		if [ "$$UNAME_M" = "arm64" ]; then \
			$(MAKE) build-darwin-arm64; \
		else \
			$(MAKE) build-darwin; \
		fi; \
	else \
		echo "Unsupported platform: $$UNAME_S"; \
		exit 1; \
	fi

# Build for macOS (Darwin)
build-darwin:
	@echo "Building $(BINARY_NAME) for macOS (darwin/amd64)..."
	@mkdir -p $(BUILD_DIR)
	cd $(SRC_DIR) && \
	CGO_ENABLED=1 \
	GOOS=darwin \
	GOARCH=amd64 \
	CGO_LDFLAGS="-L$(PWD)/$(LIB_DIR)/darwin -lrure" \
	go build -ldflags="$(LDFLAGS)" -o ../$(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 main.go
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64"

# Build for macOS ARM64 (Apple Silicon)
build-darwin-arm64:
	@echo "Building $(BINARY_NAME) for macOS (darwin/arm64)..."
	@mkdir -p $(BUILD_DIR)
	cd $(SRC_DIR) && \
	CGO_ENABLED=1 \
	GOOS=darwin \
	GOARCH=arm64 \
	CGO_LDFLAGS="-L$(PWD)/$(LIB_DIR)/darwin -lrure" \
	go build -ldflags="$(LDFLAGS)" -o ../$(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 main.go
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64"

# Build for Linux
build-linux:
	@echo "Building $(BINARY_NAME) for Linux (linux/amd64)..."
	@mkdir -p $(BUILD_DIR)
	cd $(SRC_DIR) && \
	CGO_ENABLED=1 \
	GOOS=linux \
	GOARCH=amd64 \
	CGO_LDFLAGS="-L$(PWD)/$(LIB_DIR)/linux -lrure" \
	LD_LIBRARY_PATH="$(PWD)/$(LIB_DIR)/linux:$$LD_LIBRARY_PATH" \
	go build -ldflags="$(LDFLAGS)" -o ../$(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 main.go
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64"

# Build for Linux ARM64
build-linux-arm64:
	@echo "Building $(BINARY_NAME) for Linux (linux/arm64)..."
	@mkdir -p $(BUILD_DIR)
	cd $(SRC_DIR) && \
	CGO_ENABLED=1 \
	GOOS=linux \
	GOARCH=arm64 \
	CGO_LDFLAGS="-L$(PWD)/$(LIB_DIR)/linux -lrure" \
	LD_LIBRARY_PATH="$(PWD)/$(LIB_DIR)/linux:$$LD_LIBRARY_PATH" \
	go build -ldflags="$(LDFLAGS)" -o ../$(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 main.go
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64"

# Build all supported platforms
build-all: build-darwin build-darwin-arm64 build-linux build-linux-arm64
	@echo "All builds complete!"
	@ls -la $(BUILD_DIR)/

# Build only main platforms (Intel/AMD64)
build-main: build-darwin build-linux
	@echo "Main platform builds complete!"
	@ls -la $(BUILD_DIR)/

# === NO-CGO BUILDS (Cross-platform compatible) ===

# Build for current platform without CGO
build-nocgo:
	@echo "Building $(BINARY_NAME) for current platform (no CGO)..."
	@$(MAKE) build-current-nocgo

# Detect current platform and build without CGO
build-current-nocgo:
	@UNAME_S=$$(uname -s); \
	UNAME_M=$$(uname -m); \
	if [ "$$UNAME_S" = "Linux" ]; then \
		if [ "$$UNAME_M" = "aarch64" ] || [ "$$UNAME_M" = "arm64" ]; then \
			$(MAKE) build-linux-arm64-nocgo; \
		else \
			$(MAKE) build-linux-nocgo; \
		fi; \
	elif [ "$$UNAME_S" = "Darwin" ]; then \
		if [ "$$UNAME_M" = "arm64" ]; then \
			$(MAKE) build-darwin-arm64-nocgo; \
		else \
			$(MAKE) build-darwin-nocgo; \
		fi; \
	else \
		echo "Unsupported platform: $$UNAME_S"; \
		exit 1; \
	fi

# Build for macOS (Darwin) without CGO
build-darwin-nocgo:
	@echo "Building $(BINARY_NAME) for macOS (darwin/amd64) - no CGO..."
	@mkdir -p $(BUILD_DIR)
	cd $(SRC_DIR) && \
	CGO_ENABLED=0 \
	GOOS=darwin \
	GOARCH=amd64 \
	go build -ldflags="$(LDFLAGS)" -tags nocgo -o ../$(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64-nocgo main.go
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64-nocgo"

# Build for macOS ARM64 (Apple Silicon) without CGO
build-darwin-arm64-nocgo:
	@echo "Building $(BINARY_NAME) for macOS (darwin/arm64) - no CGO..."
	@mkdir -p $(BUILD_DIR)
	cd $(SRC_DIR) && \
	CGO_ENABLED=0 \
	GOOS=darwin \
	GOARCH=arm64 \
	go build -ldflags="$(LDFLAGS)" -tags nocgo -o ../$(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64-nocgo main.go
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64-nocgo"

# Build for Linux without CGO
build-linux-nocgo:
	@echo "Building $(BINARY_NAME) for Linux (linux/amd64) - no CGO..."
	@mkdir -p $(BUILD_DIR)
	cd $(SRC_DIR) && \
	CGO_ENABLED=0 \
	GOOS=linux \
	GOARCH=amd64 \
	go build -ldflags="$(LDFLAGS)" -tags nocgo -o ../$(BUILD_DIR)/$(BINARY_NAME)-linux-amd64-nocgo main.go
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64-nocgo"

# Build for Linux ARM64 without CGO
build-linux-arm64-nocgo:
	@echo "Building $(BINARY_NAME) for Linux (linux/arm64) - no CGO..."
	@mkdir -p $(BUILD_DIR)
	cd $(SRC_DIR) && \
	CGO_ENABLED=0 \
	GOOS=linux \
	GOARCH=arm64 \
	go build -ldflags="$(LDFLAGS)" -tags nocgo -o ../$(BUILD_DIR)/$(BINARY_NAME)-linux-arm64-nocgo main.go
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64-nocgo"

# Build for Windows without CGO
build-windows-nocgo:
	@echo "Building $(BINARY_NAME) for Windows (windows/amd64) - no CGO..."
	@mkdir -p $(BUILD_DIR)
	cd $(SRC_DIR) && \
	CGO_ENABLED=0 \
	GOOS=windows \
	GOARCH=amd64 \
	go build -ldflags="$(LDFLAGS)" -tags nocgo -o ../$(BUILD_DIR)/$(BINARY_NAME)-windows-amd64-nocgo.exe main.go
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64-nocgo.exe"

# Build all supported platforms without CGO
build-all-nocgo: build-darwin-nocgo build-darwin-arm64-nocgo build-linux-nocgo build-linux-arm64-nocgo build-windows-nocgo
	@echo "All no-CGO builds complete!"
	@ls -la $(BUILD_DIR)/

# Build main platforms without CGO  
build-main-nocgo: build-darwin-nocgo build-linux-nocgo
	@echo "Main platform no-CGO builds complete!"
	@ls -la $(BUILD_DIR)/

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	@echo "Clean complete"

# Run tests
test:
	@echo "Running tests..."
	cd $(SRC_DIR) && go test ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	cd $(SRC_DIR) && go test -cover ./...

# Run linter
lint:
	@echo "Running linter..."
	cd $(SRC_DIR) && go vet ./...

# Format code
fmt:
	@echo "Formatting code..."
	cd $(SRC_DIR) && go fmt ./...

# Install dependencies
deps:
	@echo "Installing dependencies..."
	cd $(SRC_DIR) && go mod tidy
	cd $(SRC_DIR) && go mod download

# Development build (with debug info) for current platform
dev:
	@echo "Building development version..."
	@mkdir -p $(BUILD_DIR)
	@UNAME_S=$$(uname -s); \
	UNAME_M=$$(uname -m); \
	if [ "$$UNAME_S" = "Linux" ]; then \
		cd $(SRC_DIR) && \
		CGO_ENABLED=1 \
		CGO_LDFLAGS="-L$(PWD)/$(LIB_DIR)/linux -lrure" \
		LD_LIBRARY_PATH="$(PWD)/$(LIB_DIR)/linux:$$LD_LIBRARY_PATH" \
		go build -gcflags="all=-N -l" -ldflags="$(LDFLAGS)" -o ../$(BUILD_DIR)/$(BINARY_NAME)-dev main.go; \
	elif [ "$$UNAME_S" = "Darwin" ]; then \
		cd $(SRC_DIR) && \
		CGO_ENABLED=1 \
		CGO_LDFLAGS="-L$(PWD)/$(LIB_DIR)/darwin -lrure" \
		go build -gcflags="all=-N -l" -ldflags="$(LDFLAGS)" -o ../$(BUILD_DIR)/$(BINARY_NAME)-dev main.go; \
	else \
		echo "Unsupported platform for development build: $$UNAME_S"; \
		exit 1; \
	fi
	@echo "Development build complete: $(BUILD_DIR)/$(BINARY_NAME)-dev"

# Create release archives
release: build-all
	@echo "Creating release archives..."
	@mkdir -p $(BUILD_DIR)/release
	cd $(BUILD_DIR) && tar -czf release/$(BINARY_NAME)-$(VERSION)-darwin-amd64.tar.gz $(BINARY_NAME)-darwin-amd64
	cd $(BUILD_DIR) && tar -czf release/$(BINARY_NAME)-$(VERSION)-darwin-arm64.tar.gz $(BINARY_NAME)-darwin-arm64
	cd $(BUILD_DIR) && tar -czf release/$(BINARY_NAME)-$(VERSION)-linux-amd64.tar.gz $(BINARY_NAME)-linux-amd64
	cd $(BUILD_DIR) && tar -czf release/$(BINARY_NAME)-$(VERSION)-linux-arm64.tar.gz $(BINARY_NAME)-linux-arm64
	@echo "Release archives created in $(BUILD_DIR)/release/"
	@ls -la $(BUILD_DIR)/release/

# Check if required libraries exist
check-libs:
	@echo "Checking required libraries..."
	@if [ ! -f "$(LIB_DIR)/darwin/librure.a" ]; then \
		echo "Error: $(LIB_DIR)/darwin/librure.a not found"; \
		exit 1; \
	fi
	@if [ ! -f "$(LIB_DIR)/linux/librure.so" ]; then \
		echo "Error: $(LIB_DIR)/linux/librure.so not found"; \
		exit 1; \
	fi
	@echo "All required libraries found!"

# Show build information
info:
	@echo "Build Information:"
	@echo "  Version: $(VERSION)"
	@echo "  Build Time: $(BUILD_TIME)"
	@echo "  Binary Name: $(BINARY_NAME)"
	@echo "  Build Directory: $(BUILD_DIR)"
	@echo "  Source Directory: $(SRC_DIR)"
	@echo "  Library Directory: $(LIB_DIR)"

# Check what platforms can be built on current system
check-available:
	@echo "Checking available build targets on current system..."
	@echo ""
	@echo "Current system: $$(uname -s)/$$(uname -m)"
	@echo ""
	@echo "Library availability:"
	@if [ -f "$(LIB_DIR)/darwin/librure.a" ]; then \
		ARCH=$$(lipo -info $(LIB_DIR)/darwin/librure.a 2>/dev/null | grep "architecture" | sed 's/.*architecture: //'); \
		echo "  ✓ Darwin: $$ARCH"; \
	else \
		echo "  ✗ Darwin: librure.a not found"; \
	fi
	@if [ -f "$(LIB_DIR)/linux/librure.so" ]; then \
		echo "  ✓ Linux: librure.so available"; \
	else \
		echo "  ✗ Linux: librure.so not found"; \
	fi
	@echo ""
	@echo "Recommended build targets for this system:"
	@UNAME_S=$$(uname -s); UNAME_M=$$(uname -m); \
	if [ "$$UNAME_S" = "Darwin" ] && [ "$$UNAME_M" = "arm64" ]; then \
		echo "  - make build                    # Current platform (darwin/arm64)"; \
		echo "  - make build-darwin-arm64       # Explicit darwin/arm64"; \
		if [ -f "$(LIB_DIR)/linux/librure.so" ]; then \
			echo "  - make build-linux              # Cross-compile to linux (may require setup)"; \
			echo "  - make build-linux-arm64        # Cross-compile to linux/arm64 (may require setup)"; \
		fi; \
	elif [ "$$UNAME_S" = "Darwin" ] && [ "$$UNAME_M" = "x86_64" ]; then \
		echo "  - make build                    # Current platform (darwin/amd64)"; \
		echo "  - make build-darwin             # Explicit darwin/amd64"; \
		if [ -f "$(LIB_DIR)/linux/librure.so" ]; then \
			echo "  - make build-linux              # Cross-compile to linux"; \
		fi; \
	elif [ "$$UNAME_S" = "Linux" ]; then \
		echo "  - make build                    # Current platform (linux)"; \
		echo "  - make build-linux              # Explicit linux build"; \
	fi
	@echo ""
	@echo "Note: Cross-compilation may require additional setup (cross-compiler, etc.)"

# Show help
help:
	@echo "Available targets:"
	@echo "  build            - Build for current platform"
	@echo "  build-darwin     - Build for macOS (darwin/amd64)"
	@echo "  build-darwin-arm64 - Build for macOS (darwin/arm64)"
	@echo "  build-linux      - Build for Linux (linux/amd64)"
	@echo "  build-linux-arm64 - Build for Linux (linux/arm64)"
	@echo "  build-all        - Build for all supported platforms"
	@echo "  build-main       - Build for main platforms (darwin/amd64, linux/amd64)"
	@echo "  clean            - Clean build artifacts"
	@echo "  test             - Run tests"
	@echo "  test-coverage    - Run tests with coverage"
	@echo "  lint             - Run linter"
	@echo "  fmt              - Format code"
	@echo "  deps             - Install/update dependencies"
	@echo "  dev              - Build development version with debug info"
	@echo "  release          - Create release archives for all platforms"
	@echo "  check-libs       - Check if required libraries exist"
	@echo "  check-available  - Check what platforms can be built on current system"
	@echo "  info             - Show build information"
	@echo "  help             - Show this help message" 