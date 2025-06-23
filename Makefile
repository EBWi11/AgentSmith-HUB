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
	@echo ""
	@echo "Build targets:"
	@echo "  build            - Build for current platform"
	@echo "  build-darwin     - Build for macOS (darwin/amd64)"
	@echo "  build-darwin-arm64 - Build for macOS (darwin/arm64)"
	@echo "  build-linux      - Build for Linux (linux/amd64)"
	@echo "  build-linux-arm64 - Build for Linux (linux/arm64)"
	@echo "  build-all        - Build for all supported platforms"
	@echo "  build-main       - Build for main platforms (darwin/amd64, linux/amd64)"
	@echo ""
	@echo "Package targets:"
	@echo "  package          - Create package for current platform (binary + libs + run.sh)"
	@echo "  package-linux    - Create Linux package"
	@echo "  package-darwin-arm64 - Create macOS ARM64 package"
	@echo "  package-darwin   - Create macOS Intel package"
	@echo "  package-all      - Create packages for all platforms"
	@echo "  archive          - Create tar.gz archive from current platform package"
	@echo "  archive-all      - Create archives for all platform packages"
	@echo ""
	@echo "Development targets:"
	@echo "  dev              - Build development version with debug info"
	@echo "  clean            - Clean build artifacts"
	@echo "  test             - Run tests"
	@echo "  test-coverage    - Run tests with coverage"
	@echo "  lint             - Run linter"
	@echo "  fmt              - Format code"
	@echo "  deps             - Install/update dependencies"
	@echo ""
	@echo "Utility targets:"
	@echo "  release          - Create release archives for all platforms (deprecated, use archive-all)"
	@echo "  check-libs       - Check if required libraries exist"
	@echo "  check-available  - Check what platforms can be built on current system"
	@echo "  info             - Show build information"
	@echo "  help             - Show this help message"

# === PACKAGE TARGETS ===

# Package for current platform
package:
	@echo "Creating package for current platform..."
	@$(MAKE) package-current

# Detect current platform and create package
package-current:
	@UNAME_S=$$(uname -s); \
	UNAME_M=$$(uname -m); \
	if [ "$$UNAME_S" = "Linux" ]; then \
		if [ "$$UNAME_M" = "aarch64" ] || [ "$$UNAME_M" = "arm64" ]; then \
			$(MAKE) package-linux-arm64; \
		else \
			$(MAKE) package-linux; \
		fi; \
	elif [ "$$UNAME_S" = "Darwin" ]; then \
		if [ "$$UNAME_M" = "arm64" ]; then \
			$(MAKE) package-darwin-arm64; \
		else \
			$(MAKE) package-darwin; \
		fi; \
	else \
		echo "Unsupported platform: $$UNAME_S"; \
		exit 1; \
	fi

# Package for Linux
package-linux: build-linux
	@echo "Creating Linux package..."
	@$(MAKE) create-package PLATFORM=linux ARCH=amd64

# Package for macOS ARM64
package-darwin-arm64: build-darwin-arm64
	@echo "Creating macOS ARM64 package..."
	@$(MAKE) create-package PLATFORM=darwin ARCH=arm64

# Package for macOS Intel
package-darwin: build-darwin
	@echo "Creating macOS Intel package..."
	@$(MAKE) create-package PLATFORM=darwin ARCH=amd64

# Package for Linux ARM64
package-linux-arm64: build-linux-arm64
	@echo "Creating Linux ARM64 package..."
	@$(MAKE) create-package PLATFORM=linux ARCH=arm64

# Create package directory structure
create-package:
	@echo "Creating package for $(PLATFORM)/$(ARCH)..."
	@mkdir -p $(BUILD_DIR)/release/$(BINARY_NAME)-$(PLATFORM)-$(ARCH)
	@PACKAGE_DIR=$(BUILD_DIR)/release/$(BINARY_NAME)-$(PLATFORM)-$(ARCH); \
	\
	echo "Copying binary..."; \
	cp $(BUILD_DIR)/$(BINARY_NAME)-$(PLATFORM)-$(ARCH) $$PACKAGE_DIR/; \
	\
	echo "Creating lib directory..."; \
	mkdir -p $$PACKAGE_DIR/lib/$(PLATFORM); \
	if [ "$(PLATFORM)" = "linux" ]; then \
		cp $(LIB_DIR)/linux/librure.so $$PACKAGE_DIR/lib/linux/; \
	else \
		cp $(LIB_DIR)/darwin/librure.a $$PACKAGE_DIR/lib/darwin/; \
	fi; \
	\
	echo "Creating run.sh script..."; \
	cat > $$PACKAGE_DIR/run.sh << 'EOF'; \
	#!/bin/bash; \
	; \
	# AgentSmith-HUB Runner Script; \
	# Auto-generated by Makefile; \
	; \
	set -e; \
	; \
	# Determine the directory where this script is located; \
	SCRIPT_DIR="$$(cd "$$(dirname "$${BASH_SOURCE[0]}")" && pwd)"; \
	; \
	# Set library path and binary name based on OS; \
	case "$$(uname -s)" in; \
	    Linux*); \
	        export LD_LIBRARY_PATH="$$SCRIPT_DIR/lib/linux:$${LD_LIBRARY_PATH:-}"; \
	        BINARY_NAME="$(BINARY_NAME)-linux-$(ARCH)"; \
	        ;; \
	    Darwin*); \
	        export DYLD_LIBRARY_PATH="$$SCRIPT_DIR/lib/darwin:$${DYLD_LIBRARY_PATH:-}"; \
	        BINARY_NAME="$(BINARY_NAME)-darwin-$(ARCH)"; \
	        ;; \
	    *); \
	        echo "Unsupported operating system: $$(uname -s)"; \
	        exit 1; \
	        ;; \
	esac; \
	; \
	# Check if binary exists; \
	BINARY_PATH="$$SCRIPT_DIR/$$BINARY_NAME"; \
	if [ ! -f "$$BINARY_PATH" ]; then; \
	    echo "Binary not found: $$BINARY_PATH"; \
	    echo "Available binaries:"; \
	    ls -la "$$SCRIPT_DIR"/$(BINARY_NAME)-* 2>/dev/null || echo "No binaries found"; \
	    exit 1; \
	fi; \
	; \
	# Make sure binary is executable; \
	chmod +x "$$BINARY_PATH"; \
	; \
	echo "Starting AgentSmith-HUB ($$(uname -s)/$$(uname -m))..."; \
	echo "Binary: $$BINARY_NAME"; \
	echo "Library path set for: $$(uname -s)"; \
	; \
	# Run the binary with all passed arguments; \
	exec "$$BINARY_PATH" "$$@"; \
	EOF; \
	chmod +x $$PACKAGE_DIR/run.sh; \
	\
	echo "Copying config directory..."; \
	if [ -d "config" ]; then \
		cp -r config $$PACKAGE_DIR/; \
	fi; \
	\
	echo "Creating README..."; \
	cat > $$PACKAGE_DIR/README.md << EOF; \
	# AgentSmith-HUB - $(PLATFORM)/$(ARCH); \
	; \
	## Quick Start; \
	; \
	\`\`\`bash; \
	# Make the runner executable; \
	chmod +x run.sh; \
	; \
	# Run the application; \
	./run.sh; \
	\`\`\`; \
	; \
	## Files; \
	; \
	- \`$(BINARY_NAME)-$(PLATFORM)-$(ARCH)\`: Main binary; \
	- \`run.sh\`: Platform-aware runner script (recommended); \
	- \`lib/$(PLATFORM)/\`: Required libraries; \
	- \`config/\`: Configuration templates (if available); \
	; \
	## Build Information; \
	; \
	- Version: $(VERSION); \
	- Build Time: $(BUILD_TIME); \
	- Platform: $(PLATFORM)/$(ARCH); \
	; \
	## Manual Execution; \
	; \
	If you prefer to run the binary directly:; \
	; \
	\`\`\`bash; \
	# Linux; \
	export LD_LIBRARY_PATH="\$$PWD/lib/linux:\$$LD_LIBRARY_PATH"; \
	./$(BINARY_NAME)-$(PLATFORM)-$(ARCH); \
	; \
	# macOS; \
	export DYLD_LIBRARY_PATH="\$$PWD/lib/darwin:\$$DYLD_LIBRARY_PATH"; \
	./$(BINARY_NAME)-$(PLATFORM)-$(ARCH); \
	\`\`\`; \
	EOF; \
	\
	echo "Package created: $$PACKAGE_DIR"; \
	ls -la $$PACKAGE_DIR

# Package all platforms
package-all: package-linux package-darwin-arm64 package-linux-arm64 package-darwin
	@echo "All packages created!"
	@ls -la $(BUILD_DIR)/release/

# Create archive from package
archive: package
	@echo "Creating archive..."
	@UNAME_S=$$(uname -s); \
	UNAME_M=$$(uname -m); \
	if [ "$$UNAME_S" = "Linux" ]; then \
		if [ "$$UNAME_M" = "aarch64" ] || [ "$$UNAME_M" = "arm64" ]; then \
			PLATFORM=linux; ARCH=arm64; \
		else \
			PLATFORM=linux; ARCH=amd64; \
		fi; \
	elif [ "$$UNAME_S" = "Darwin" ]; then \
		if [ "$$UNAME_M" = "arm64" ]; then \
			PLATFORM=darwin; ARCH=arm64; \
		else \
			PLATFORM=darwin; ARCH=amd64; \
		fi; \
	fi; \
	cd $(BUILD_DIR)/release && \
	tar -czf $(BINARY_NAME)-$(VERSION)-$$PLATFORM-$$ARCH.tar.gz $(BINARY_NAME)-$$PLATFORM-$$ARCH; \
	echo "Archive created: $(BUILD_DIR)/release/$(BINARY_NAME)-$(VERSION)-$$PLATFORM-$$ARCH.tar.gz"

# Create archives for all packages
archive-all: package-all
	@echo "Creating archives for all packages..."
	@cd $(BUILD_DIR)/release && \
	for dir in $(BINARY_NAME)-*; do \
		if [ -d "$$dir" ]; then \
			tar -czf "$$dir-$(VERSION).tar.gz" "$$dir"; \
			echo "Archive created: $$dir-$(VERSION).tar.gz"; \
		fi; \
	done
	@echo "All archives created!"
	@ls -la $(BUILD_DIR)/release/*.tar.gz 