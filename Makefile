# AgentSmith-HUB Makefile

# Variables
BINARY_NAME=agentsmith-hub
BUILD_DIR=bin
SRC_DIR=src
LIB_DIR=lib

# Detect OS
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Linux)
    LIB_PLATFORM=linux
endif
ifeq ($(UNAME_S),Darwin)
    LIB_PLATFORM=darwin
endif

# CGO flags
CGO_LDFLAGS=-L$(PWD)/$(LIB_DIR)/$(LIB_PLATFORM) -lrure

# Default target
.PHONY: all
all: build

# Build the application
.PHONY: build
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	cd $(SRC_DIR) && CGO_LDFLAGS="$(CGO_LDFLAGS)" go build -o ../$(BUILD_DIR)/$(BINARY_NAME) main.go
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	@echo "Clean complete"

# Run tests
.PHONY: test
test:
	@echo "Running tests..."
	cd $(SRC_DIR) && go test ./...

# Run linter
.PHONY: lint
lint:
	@echo "Running linter..."
	cd $(SRC_DIR) && go vet ./...

# Format code
.PHONY: fmt
fmt:
	@echo "Formatting code..."
	cd $(SRC_DIR) && go fmt ./...

# Install dependencies
.PHONY: deps
deps:
	@echo "Installing dependencies..."
	cd $(SRC_DIR) && go mod tidy

# Development build (with debug info)
.PHONY: dev
dev:
	@echo "Building development version..."
	@mkdir -p $(BUILD_DIR)
	cd $(SRC_DIR) && CGO_LDFLAGS="$(CGO_LDFLAGS)" go build -gcflags="all=-N -l" -o ../$(BUILD_DIR)/$(BINARY_NAME)-dev main.go
	@echo "Development build complete: $(BUILD_DIR)/$(BINARY_NAME)-dev"

# Show help
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build        - Build the application"
	@echo "  clean        - Clean build artifacts"
	@echo "  test         - Run tests"
	@echo "  lint         - Run linter"
	@echo "  fmt          - Format code"
	@echo "  deps         - Install dependencies"
	@echo "  dev          - Build development version with debug info"
	@echo "  help         - Show this help message" 