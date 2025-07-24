# AgentSmith-HUB Dockerfile
# Multi-stage build for optimized image size

# Build stage
FROM golang:1.24.5-alpine AS builder

# Install build dependencies
RUN apk add --no-cache \
    build-base \
    git \
    ca-certificates

# Set working directory
WORKDIR /app

# Copy go mod files
COPY src/go.mod src/go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY src/ ./

# Copy libraries
COPY lib/ ./lib/

# Build arguments
ARG VERSION=unknown
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown
ARG TARGETARCH

# Set build environment
ENV CGO_ENABLED=1
ENV GOOS=linux

# Set architecture-specific variables and build
RUN if [ "$TARGETARCH" = "arm64" ]; then \
        GOARCH=arm64 \
        CC=aarch64-linux-gnu-gcc \
        LIB_PATH="/app/lib/linux/arm64" \
        CGO_LDFLAGS="-L${LIB_PATH} -lrure -Wl,-rpath,${LIB_PATH}" \
        LD_LIBRARY_PATH="${LIB_PATH}:$LD_LIBRARY_PATH" \
        LDFLAGS="-s -w -X 'main.Version=${VERSION}' -X 'main.BuildTime=${BUILD_TIME}' -X 'main.GitCommit=${GIT_COMMIT}'" \
        go build -ldflags "$LDFLAGS" -o agentsmith-hub .; \
    else \
        GOARCH=amd64 \
        LIB_PATH="/app/lib/linux/amd64" \
        CGO_LDFLAGS="-L${LIB_PATH} -lrure -Wl,-rpath,${LIB_PATH}" \
        LD_LIBRARY_PATH="${LIB_PATH}:$LD_LIBRARY_PATH" \
        LDFLAGS="-s -w -X 'main.Version=${VERSION}' -X 'main.BuildTime=${BUILD_TIME}' -X 'main.GitCommit=${GIT_COMMIT}'" \
        go build -ldflags "$LDFLAGS" -o agentsmith-hub .; \
    fi

# Runtime stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    libc6-compat

# Create non-root user
RUN addgroup -g 1000 agentsmith && \
    adduser -D -s /bin/sh -u 1000 -G agentsmith agentsmith

# Set working directory
WORKDIR /opt/agentsmith-hub

# Copy binary from builder
COPY --from=builder /app/agentsmith-hub .

# Copy libraries
COPY --from=builder /app/lib/ ./lib/

# Copy configuration files
COPY config/ ./config/
COPY mcp_config/ ./mcp_config/

# Create necessary directories
RUN mkdir -p /tmp/hub_logs /opt/lib /opt/mcp_config && \
    chown -R agentsmith:agentsmith /opt/agentsmith-hub /tmp/hub_logs

# Set environment variables
ENV CONFIG_ROOT=/opt/agentsmith-hub/config
ENV LOG_LEVEL=info
ENV NODE_ID=default

# Expose port
EXPOSE 8080

# Switch to non-root user
USER agentsmith

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/ping || exit 1

# Default command
ENTRYPOINT ["./agentsmith-hub"]

# Default arguments (can be overridden)
CMD ["--config_root", "/opt/agentsmith-hub/config", "--port", "8080"] 