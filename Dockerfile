# AgentSmith-HUB Dockerfile
# Unified image for both leader and follower modes

FROM alpine:3.19

# Install runtime dependencies including web server
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    libc6-compat \
    nginx \
    bash \
    curl \
    wget

# Create non-root user
RUN addgroup -g 1000 agentsmith && \
    adduser -D -s /bin/bash -u 1000 -G agentsmith agentsmith

# Set working directory
WORKDIR /opt/agentsmith-hub

# Copy pre-built binary based on target architecture
# The binary is expected to be in the root of the build context
ARG TARGETARCH
COPY agentsmith-hub-${TARGETARCH} ./agentsmith-hub
RUN chmod +x ./agentsmith-hub

# Copy libraries based on target architecture
COPY lib/linux/${TARGETARCH}/ ./lib/

# Copy configuration files
COPY config/ ./config/
COPY mcp_config/ ./mcp_config/

# Copy web frontend
# The web/dist directory is expected to be in the build context
COPY web/dist/ ./web/dist/

# Copy startup scripts
COPY scripts/docker/leader-start.sh ./leader-start.sh
COPY scripts/docker/follower-start.sh ./follower-start.sh
COPY scripts/docker/docker-entrypoint.sh ./docker-entrypoint.sh
RUN chmod +x ./leader-start.sh ./follower-start.sh ./docker-entrypoint.sh

# Create necessary directories
RUN mkdir -p /tmp/hub_logs /opt/lib /opt/mcp_config /opt/config /var/lib/nginx/html /var/log/nginx && \
    chown -R agentsmith:agentsmith /opt/agentsmith-hub /tmp/hub_logs /opt/config /var/lib/nginx /var/log/nginx

# Configure nginx for web frontend
COPY scripts/docker/nginx.conf /etc/nginx/http.d/default.conf

# Set environment variables
ENV CONFIG_ROOT=/opt/config
ENV LOG_LEVEL=info
ENV NODE_ID=default
ENV MODE=leader

# Expose ports
EXPOSE 8080 80

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/ping || exit 1

# Switch to non-root user
USER agentsmith

# Default command - use environment variable to determine mode
ENTRYPOINT ["./docker-entrypoint.sh"]
