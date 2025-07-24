# AgentSmith-HUB Dockerfile
# Unified image for both leader and follower modes

FROM alpine:3.19

# Install runtime dependencies
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

# Copy and extract the deployment archive
ARG TARGETARCH
COPY agentsmith-hub-*.tar.gz ./
RUN echo "=== STEP 1: Check tar.gz files ===" && \
    ls -la *.tar.gz && \
    echo "=== STEP 2: Extract tar.gz ===" && \
    tar -xzf agentsmith-hub-${TARGETARCH}.tar.gz && \
    echo "=== STEP 3: Check extracted structure ===" && \
    ls -la && \
    echo "=== STEP 4: Check agentsmith-hub directory ===" && \
    ls -la agentsmith-hub/ && \
    echo "=== STEP 5: Remove tar.gz ===" && \
    rm agentsmith-hub-*.tar.gz && \
    echo "=== STEP 6: Copy files ===" && \
    cp -r agentsmith-hub/* . && \
    echo "=== STEP 7: Check copied files ===" && \
    ls -la && \
    echo "=== STEP 8: Remove directory ===" && \
    rm -rf agentsmith-hub && \
    echo "=== STEP 9: Final check ===" && \
    ls -la && \
    echo "=== STEP 10: Set permissions ===" && \
    chmod +x ./agentsmith-hub && \
    echo "=== STEP 11: Verify binary ===" && \
    ls -la ./agentsmith-hub

# Ensure startup scripts are executable
RUN echo "=== STEP 12: Check scripts exist ===" && \
    ls -la *.sh && \
    echo "=== STEP 13: Set script permissions ===" && \
    chmod +x ./start.sh ./stop.sh && \
    echo "=== STEP 14: Verify script permissions ===" && \
    ls -la *.sh

# Create docker-entrypoint.sh with frontend and backend
RUN echo '#!/bin/bash' > ./docker-entrypoint.sh && \
    echo 'set -e' >> ./docker-entrypoint.sh && \
    echo '' >> ./docker-entrypoint.sh && \
    echo '# Start nginx for frontend' >> ./docker-entrypoint.sh && \
    echo 'nginx -g "daemon off;" &' >> ./docker-entrypoint.sh && \
    echo '' >> ./docker-entrypoint.sh && \
    echo '# Start backend' >> ./docker-entrypoint.sh && \
    echo 'if [ "$MODE" = "follower" ]; then' >> ./docker-entrypoint.sh && \
    echo '  exec ./start.sh --follower' >> ./docker-entrypoint.sh && \
    echo 'else' >> ./docker-entrypoint.sh && \
    echo '  exec ./start.sh' >> ./docker-entrypoint.sh && \
    echo 'fi' >> ./docker-entrypoint.sh && \
    chmod +x ./docker-entrypoint.sh

# Create necessary directories
RUN echo "=== STEP 18: Create directories ===" && \
    mkdir -p /tmp/hub_logs /var/lib/nginx/html /var/log/nginx && \
    echo "=== STEP 19: Check created directories ===" && \
    ls -la /tmp/hub_logs && \
    ls -la /var/lib/nginx/html && \
    ls -la /var/log/nginx

# Configure nginx (nginx.conf is now available from the extracted archive)
RUN echo "=== STEP 15: Check nginx directory ===" && \
    ls -la nginx/ && \
    echo "=== STEP 16: Copy nginx config ===" && \
    cp nginx/nginx.conf /etc/nginx/nginx.conf && \
    echo "=== STEP 17: Verify nginx config ===" && \
    ls -la /etc/nginx/nginx.conf

# Set proper ownership
RUN echo "=== STEP 20: Set ownership ===" && \
    chown -R agentsmith:agentsmith /opt/agentsmith-hub /tmp/hub_logs /var/lib/nginx /var/log/nginx /etc/nginx/nginx.conf && \
    echo "=== STEP 21: Verify ownership ===" && \
    ls -la /opt/agentsmith-hub/ && \
    ls -la /tmp/hub_logs && \
    ls -la /etc/nginx/nginx.conf

# Set environment variables
ENV CONFIG_ROOT=/opt/agentsmith-hub/config
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

# Default command
ENTRYPOINT ["./docker-entrypoint.sh"]
