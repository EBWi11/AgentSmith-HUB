# Configuration Guide

AgentSmith-HUB frontend supports multiple configuration methods, including build-time and runtime configuration, to adapt to different deployment environments.

## Configuration Priority

Configuration is merged according to the following priority:

1. **Runtime Configuration** (Highest Priority) - `/config.json`
2. **Environment Variables** (Medium Priority) - `.env` files or build-time environment variables
3. **Default Configuration** (Lowest Priority) - Built-in default values

## Configuration Methods

### 1. Runtime Configuration (Recommended for Production)

Create a `/config.json` file in the deployed web root directory, allowing configuration changes without recompilation:

```json
{
  "apiBaseUrl": "https://api.example.com:8080",
  "apiTimeout": 60000,
  "enableDebugMode": false,
  "enableClusterMode": true,
  "theme": "light",
  "language": "en"
}
```

**Advantages:**
- No recompilation needed for configuration changes
- Supports dynamic configuration updates
- Suitable for containerized deployments

**Deployment Examples:**
```bash
# Copy configuration file during deployment
cp config.production.json /path/to/web/config.json

# Or generate configuration file using environment variables
envsubst < config.template.json > /path/to/web/config.json
```

### 2. Environment Variables (Recommended for Build Time)

Set configuration through environment variables during build:

```bash
# Set environment variables during build
VITE_API_BASE_URL=https://api.example.com:8080 \
VITE_API_TIMEOUT=60000 \
VITE_DEBUG_MODE=false \
npm run build
```

Or create a `.env` file:

```env
VITE_API_BASE_URL=https://api.example.com:8080
VITE_API_TIMEOUT=60000
VITE_DEBUG_MODE=false
VITE_CLUSTER_MODE=true
VITE_THEME=light
VITE_LANGUAGE=en
```

**Advantages:**
- Easy integration with CI/CD systems
- Supports builds for different environments
- Configuration is fixed at build time

### 3. Default Configuration

If no other configuration is provided, the system will use built-in default configuration:

```javascript
{
  apiBaseUrl: 'http://localhost:8080',
  apiTimeout: 30000,
  enableDebugMode: false,
  enableClusterMode: true,
  theme: 'light',
  language: 'en'
}
```

## Configuration Options

### API Configuration

- `apiBaseUrl`: Backend API address
  - Development default: `http://localhost:8080`
  - Production default: `${window.location.protocol}//${window.location.hostname}:8080`
- `apiTimeout`: API request timeout (milliseconds)

### Feature Flags

- `enableDebugMode`: Whether to enable debug mode
- `enableClusterMode`: Whether to enable cluster mode

### UI Configuration

- `theme`: Theme setting (`light` | `dark`)
- `language`: Language setting (`en` | `zh`)

### Authentication (OIDC)

The frontend initializes OIDC based on backend runtime config from `GET /auth/config`. Usually no build-time values are needed. If you need static values at build time (optional), set:

```env
# VITE_OIDC_ISSUER=https://your-idp/.well-known/openid-configuration
# VITE_OIDC_CLIENT_ID=agentsmith-web
```

At runtime, the backend must be configured with:

- `OIDC_ENABLED`, `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_REDIRECT_URI` (required when enabled)
- Optional: `OIDC_USERNAME_CLAIM`, `OIDC_ALLOWED_USERS`, `OIDC_SCOPE`

The login page shows an SSO button when OIDC is enabled. Default callback route is `/oidc/callback`.

## Deployment Scenarios

### Scenario 1: Containerized Deployment

```dockerfile
# Dockerfile
FROM nginx:alpine

# Copy built files
COPY dist/ /usr/share/nginx/html/

# Copy configuration file template
COPY config.template.json /usr/share/nginx/html/config.json

# Replace configuration during container startup
CMD envsubst < /usr/share/nginx/html/config.json.template > /usr/share/nginx/html/config.json && nginx -g 'daemon off;'
```

### Scenario 2: Traditional Deployment

```bash
# Build application
npm run build

# Copy to web server
cp -r dist/* /var/www/html/

# Create runtime configuration
echo '{"apiBaseUrl": "https://api.example.com:8080"}' > /var/www/html/config.json
```

### Scenario 3: CDN Deployment

```bash
# Build with environment variables
VITE_API_BASE_URL=https://api.example.com:8080 npm run build

# Upload to CDN
aws s3 sync dist/ s3://your-bucket/
```

## Development Mode

In development mode, the configuration system will:

1. Check for configuration file updates every 10 seconds
2. Automatically reload configuration
3. Trigger `configurationChanged` event

```javascript
// Listen for configuration changes
window.addEventListener('configurationChanged', (event) => {
  console.log('Configuration updated:', event.detail);
});
```

## Troubleshooting

### Configuration Not Taking Effect

1. Check if configuration file path is correct (`/config.json`)
2. Confirm JSON format is correct
3. Check browser developer tools console logs
4. Verify environment variables start with `VITE_`

### Configuration Priority Issues

Use browser developer tools to view the actual loaded configuration:

```javascript
// Execute in console
import config from './src/config/index.js';
console.log(config);
```

### Network Request Failures

Check if `config.json` file is accessible:

```bash
curl https://your-domain.com/config.json
```

## Best Practices

1. **Use runtime configuration for production environments** for easier operations management
2. **Use environment variables for development environments** for easier debugging
3. **Manage sensitive information through environment variables**, don't write to configuration files
4. **Use version control for configuration files** for easy rollbacks
5. **Regularly check configuration file validity** 