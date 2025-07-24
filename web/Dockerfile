# AgentSmith-HUB Frontend Dockerfile
# Multi-stage build for optimized image size

# Build stage
FROM node:20-alpine AS builder

# Set working directory
WORKDIR /app

# Copy package files
COPY package*.json ./

# Install dependencies
RUN npm ci

# Copy source code
COPY . .

# Build arguments
ARG VERSION=unknown
ARG GIT_COMMIT=unknown

# Set environment variables for build
ENV VITE_APP_VERSION=${VERSION}
ENV VITE_APP_COMMIT=${GIT_COMMIT}

# Build the application
RUN npm run build

# Production stage
FROM nginx:alpine

# Install envsubst (part of gettext package)
RUN apk add --no-cache gettext

# Copy built files
COPY --from=builder /app/dist /usr/share/nginx/html

# Copy configuration template
COPY --from=builder /app/config.template.json /usr/share/nginx/html/config.template.json

# Copy nginx configuration
COPY nginx.conf /etc/nginx/nginx.conf

# Create startup script
RUN echo '#!/bin/sh' > /docker-entrypoint.sh && \
    echo 'set -e' >> /docker-entrypoint.sh && \
    echo '' >> /docker-entrypoint.sh && \
    echo '# Default values' >> /docker-entrypoint.sh && \
    echo 'export API_BASE_URL=${API_BASE_URL:-"http://localhost:8080"}' >> /docker-entrypoint.sh && \
    echo 'export API_TIMEOUT=${API_TIMEOUT:-30000}' >> /docker-entrypoint.sh && \
    echo 'export DEBUG_MODE=${DEBUG_MODE:-false}' >> /docker-entrypoint.sh && \
    echo 'export CLUSTER_MODE=${CLUSTER_MODE:-true}' >> /docker-entrypoint.sh && \
    echo 'export THEME=${THEME:-dark}' >> /docker-entrypoint.sh && \
    echo 'export LANGUAGE=${LANGUAGE:-zh-CN}' >> /docker-entrypoint.sh && \
    echo '' >> /docker-entrypoint.sh && \
    echo '# Generate configuration file from template' >> /docker-entrypoint.sh && \
    echo 'echo "🔧 Generating configuration..."' >> /docker-entrypoint.sh && \
    echo 'envsubst < /usr/share/nginx/html/config.template.json > /usr/share/nginx/html/config.json' >> /docker-entrypoint.sh && \
    echo '' >> /docker-entrypoint.sh && \
    echo '# Show generated configuration' >> /docker-entrypoint.sh && \
    echo 'echo "📋 Configuration generated:"' >> /docker-entrypoint.sh && \
    echo 'cat /usr/share/nginx/html/config.json' >> /docker-entrypoint.sh && \
    echo '' >> /docker-entrypoint.sh && \
    echo '# Remove template file' >> /docker-entrypoint.sh && \
    echo 'rm -f /usr/share/nginx/html/config.template.json' >> /docker-entrypoint.sh && \
    echo '' >> /docker-entrypoint.sh && \
    echo '# Start nginx' >> /docker-entrypoint.sh && \
    echo 'echo "🚀 Starting nginx..."' >> /docker-entrypoint.sh && \
    echo 'exec nginx -g "daemon off;"' >> /docker-entrypoint.sh

# Make startup script executable
RUN chmod +x /docker-entrypoint.sh

# Expose port
EXPOSE 80

# Set default environment variables
ENV API_BASE_URL=http://localhost:8080
ENV API_TIMEOUT=30000
ENV DEBUG_MODE=false
ENV CLUSTER_MODE=true
ENV THEME=dark
ENV LANGUAGE=zh-CN

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost/ || exit 1

# Use custom entrypoint
ENTRYPOINT ["/docker-entrypoint.sh"] 