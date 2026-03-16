/**
 * Runtime Configuration System
 * Supports both build-time environment variables and runtime configuration files
 */

// Default configuration
const defaultConfig = {
  // API Configuration
  apiBaseUrl: '/api',
  apiTimeout: 30000 // 30 seconds
};

// Environment-based configuration (build-time)
const envConfig = {
  // Use Vite's environment variables
  apiBaseUrl: import.meta.env.VITE_API_BASE_URL || 
              (import.meta.env.DEV 
                ? 'http://10.20.103.140/api'
                : '/api'),
  apiTimeout: import.meta.env.VITE_API_TIMEOUT ? parseInt(import.meta.env.VITE_API_TIMEOUT) : 30000
};

// Runtime configuration (loaded from external file)
let runtimeConfig = {};

// Merged configuration object that preserves reference for consumers
const config = {
  ...defaultConfig,
  ...envConfig
};

function applyMergedConfig() {
  Object.assign(config, getConfig());
}

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
      const loadedConfig = await response.json();
      runtimeConfig = loadedConfig || {};
      applyMergedConfig();
      return config;
    }
  } catch (error) {
    console.debug('No runtime configuration file found, using default configuration');
  }
  
  applyMergedConfig();
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
  if (!configPromise) {
    configPromise = (async () => {
      await loadRuntimeConfig();
      return getConfig();
    })();
  }

  try {
    const merged = await configPromise;
    applyMergedConfig();
    return merged;
  } catch (error) {
    applyMergedConfig();
    throw error;
  }
}

// Ensure config has initial merged values
applyMergedConfig();

// Configuration hot reload removed - use smart refresh system for config updates

export { initializeConfig, loadRuntimeConfig };
export default config;
