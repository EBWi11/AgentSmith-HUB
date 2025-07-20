/**
 * Runtime Configuration System
 * Supports both build-time environment variables and runtime configuration files
 */

// Default configuration
const defaultConfig = {
  // API Configuration
  apiBaseUrl: '/api',
  apiTimeout: 30000, // 30 seconds
  
  // Feature flags
  enableDebugMode: false,
  enableClusterMode: true,
  
  // UI Configuration
  theme: 'light',
  language: 'en'
};

// Environment-based configuration (build-time)
const envConfig = {
  // Use Vite's environment variables
  apiBaseUrl: import.meta.env.VITE_API_BASE_URL || 
              (import.meta.env.DEV 
                ? 'http://localhost:8080' 
                : '/api'),
  apiTimeout: import.meta.env.VITE_API_TIMEOUT ? parseInt(import.meta.env.VITE_API_TIMEOUT) : 30000,
  enableDebugMode: import.meta.env.VITE_DEBUG_MODE === 'true',
  enableClusterMode: import.meta.env.VITE_CLUSTER_MODE !== 'false',
  theme: import.meta.env.VITE_THEME || 'light',
  language: import.meta.env.VITE_LANGUAGE || 'en'
};

// Runtime configuration (loaded from external file)
let runtimeConfig = {};

/**
 * Load runtime configuration from external file
 * This allows configuration changes without recompilation
 */
async function loadRuntimeConfig() {
  try {
    // Try to load configuration from /config.json
    const response = await fetch('/config.json', {
      cache: 'no-cache',
      headers: {
        'Cache-Control': 'no-cache'
      }
    });
    
    if (response.ok) {
      const config = await response.json();
      runtimeConfig = config;
      console.log('Runtime configuration loaded successfully');
      return config;
    }
  } catch (error) {
    console.debug('No runtime configuration file found, using default configuration');
  }
  
  return {};
}

/**
 * Get merged configuration
 * Priority: runtime config > environment config > default config
 */
function getConfig() {
  return {
    ...defaultConfig,
    ...envConfig,
    ...runtimeConfig
  };
}

// Initialize configuration
let configPromise = null;

/**
 * Initialize configuration asynchronously
 */
async function initializeConfig() {
  if (configPromise) {
    return configPromise;
  }
  
  configPromise = loadRuntimeConfig();
  await configPromise;
  
  return getConfig();
}

// Export the configuration
const config = getConfig();

// Configuration hot reload removed - use smart refresh system for config updates

export { initializeConfig, loadRuntimeConfig };
export default config; 