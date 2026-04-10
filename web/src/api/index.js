import axios from 'axios';
import config, { initializeConfig } from '../config';

const api = axios.create({
  baseURL: config.apiBaseUrl,
  timeout: config.apiTimeout,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Create a separate axios instance for public APIs (no token required)
const publicApi = axios.create({
  baseURL: config.apiBaseUrl,
  timeout: config.apiTimeout,
  headers: {
    'Content-Type': 'application/json',
  },
});

initializeConfig()
  .then((resolvedConfig) => {
    if (!resolvedConfig) {
      return;
    }

    api.defaults.baseURL = resolvedConfig.apiBaseUrl;
    api.defaults.timeout = resolvedConfig.apiTimeout;
    publicApi.defaults.baseURL = resolvedConfig.apiBaseUrl;
    publicApi.defaults.timeout = resolvedConfig.apiTimeout;
  })
  .catch((error) => {
    console.warn('Failed to apply runtime configuration to API clients:', error);
  });

/**
 * Handles API errors consistently
 * @param {Error} error - The error object
 * @param {string} defaultMessage - Default message if error details aren't available
 * @param {boolean} returnEmptyArray - Whether to return an empty array instead of throwing
 * @returns {Array|void} - Empty array for list endpoints or throws error
 */
const handleApiError = (error, defaultMessage, returnEmptyArray = false) => {
  console.error(defaultMessage, error);
  if (returnEmptyArray) return [];
  throw error;
};

const COMPONENT_ENDPOINTS = {
  inputs: '/inputs',
  outputs: '/outputs',
  rulesets: '/rulesets',
  projects: '/projects',
  plugins: '/plugins',
  agents: '/agents',
  skills: '/skills'
};

const COMPONENT_GETTERS = {
  inputs: 'getInput',
  outputs: 'getOutput',
  rulesets: 'getRuleset',
  projects: 'getProject',
  plugins: 'getPlugin',
  agents: 'getAgent',
  skills: 'getSkill'
};

const PROJECT_OPERATION_ENDPOINTS = {
  start: '/start-project',
  stop: '/stop-project',
  restart: '/restart-project'
};

const normalizeComponentType = (type = '', target = 'plural') => {
  const normalized = String(type || '').trim().toLowerCase();
  if (!normalized) return normalized;

  if (target === 'plural') {
    switch (normalized) {
      case 'input':
        return 'inputs';
      case 'output':
        return 'outputs';
      case 'ruleset':
        return 'rulesets';
      case 'project':
        return 'projects';
      case 'plugin':
        return 'plugins';
      case 'agent':
        return 'agents';
      case 'skill':
        return 'skills';
      default:
        return normalized.endsWith('s') ? normalized : `${normalized}s`;
    }
  }

  switch (normalized) {
    case 'inputs':
      return 'input';
    case 'outputs':
      return 'output';
    case 'rulesets':
      return 'ruleset';
    case 'projects':
      return 'project';
    case 'plugins':
      return 'plugin';
    case 'agents':
      return 'agent';
    case 'skills':
      return 'skill';
    default:
      return normalized.endsWith('s') ? normalized.slice(0, -1) : normalized;
  }
};

const getComponentEndpoint = (type, id = '') => {
  const pluralType = normalizeComponentType(type, 'plural');
  const endpoint = COMPONENT_ENDPOINTS[pluralType];
  if (!endpoint) {
    throw new Error(`Unsupported component type: ${type}`);
  }
  return id ? `${endpoint}/${id}` : endpoint;
};

const normalizeRawContent = (raw) => {
  return typeof raw === 'object' ? JSON.stringify(raw) : String(raw || '');
};

const dispatchComponentChanged = (action, type, id) => {
  if (typeof window === 'undefined') {
    return;
  }

  window.dispatchEvent(new CustomEvent('componentChanged', {
    detail: { action, type, id, timestamp: Date.now() }
  }));
};

const buildVerifyFailure = (message) => ({
  data: {
    valid: false,
    error: message
  }
});

const buildResultError = (error, fallbackMessage, fallbackPayload = {}) => ({
  success: false,
  error: error.response?.data?.error || error.message || fallbackMessage,
  ...fallbackPayload
});

const normalizeParams = (params = {}) => {
  if (params instanceof URLSearchParams) {
    return Object.fromEntries(params.entries());
  }

  if (typeof params === 'string') {
    return Object.fromEntries(new URLSearchParams(params));
  }

  return params || {};
};

// Add request interceptor to add token or bearer to all requests
api.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('auth_token');
    const bearer = localStorage.getItem('auth_bearer');
    if (bearer) {
      config.headers.Authorization = `Bearer ${bearer}`;
    } else if (token) {
      config.headers.token = token;
    }
    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);

// Add response interceptor to handle token expiration
api.interceptors.response.use(
  (response) => {
    return response;
  },
  (error) => {
    if (error.response?.status === 401) {
      try {
        localStorage.removeItem('auth_token');
        localStorage.removeItem('auth_bearer');
        delete api.defaults.headers.token;
        delete api.defaults.headers.Authorization;
      } catch (e) {}
      console.error('Authentication failed: Token invalid or expired');
      
      // Safe redirect to login page
      if (typeof window !== 'undefined') {
        // Check current path, redirect if not on login page
        const currentPath = window.location.pathname;
        const isLoginPage = currentPath === '/' || currentPath === '/login' || 
                           currentPath.startsWith('/#/') || currentPath.includes('/login');
        
        if (!isLoginPage) {
          if (window.router) {
            // If in Vue Router environment, use router navigation
            try {
              window.router.push({ name: 'Login' });
            } catch (routerError) {
              // Fall back to location redirect if router navigation fails
              window.location.replace('/');
            }
          } else {
            // Use replace to avoid leaving records in browser history
            window.location.replace('/');
          }
        }
      }
    }
    if (typeof window !== 'undefined' && window.$toast) {
      let msg = error.response?.data?.error || error.message || 'Unknown error';
      
      try {
        if (typeof window.$toast.show === 'function') {
          window.$toast.show(msg, 'error');
        }
      } catch (error) {
        // Silently ignore toast errors to prevent breaking the API functionality
        console.warn('Toast notification failed:', error)
      }
    }
    return Promise.reject(error);
  }
);

/**
 * Generic function to fetch components by type
 * @param {string} type - Component type
 * @returns {Promise<Array>} - Array of components with temp file info
 */
// Will be defined after hubApi is declared
let fetchComponentsByType;

export const hubApi = {
  setToken(token) {
    localStorage.setItem('auth_token', token);
    api.defaults.headers.token = token;
  },

  clearToken() {
    localStorage.removeItem('auth_token');
    localStorage.removeItem('auth_bearer');
    delete api.defaults.headers.token;
    delete api.defaults.headers.Authorization;
  },

  async verifyToken() {
    try {
      const response = await api.get('/token-check');
      return response.data;
    } catch (error) {
      // Clear token to avoid infinite refresh
      this.clearToken();
      throw error;
    }
  },

  async getAuthConfig() {
    const response = await publicApi.get('/auth/config');
    return response.data;
  },

  async getFeatures() {
    const response = await publicApi.get('/features');
    return response.data;
  },

  async getComponent(type, id) {
    const response = await api.get(getComponentEndpoint(type, id));
    return response.data;
  },

  async createComponent(type, id, raw, extraPayload = {}) {
    const response = await api.post(getComponentEndpoint(type), {
      id,
      raw,
      ...extraPayload
    });
    return response.data;
  },

  async updateComponent(type, id, raw) {
    const response = await api.put(getComponentEndpoint(type, id), {
      raw: normalizeRawContent(raw)
    });
    return response.data;
  },

  async deleteAndDispatch(type, id) {
    const response = await this.deleteComponent(type, id);
    dispatchComponentChanged('deleted', normalizeComponentType(type, 'plural'), id);
    return response;
  },

  async getComponentRaw(type, id) {
    const getterName = COMPONENT_GETTERS[normalizeComponentType(type, 'plural')];
    if (!getterName || typeof this[getterName] !== 'function') {
      throw new Error(`Unsupported component type: ${type}`);
    }
    return this[getterName](id);
  },

  async resolveTempState(type, id, options = {}) {
    if (typeof options?.hasTemp === 'boolean') {
      return options.hasTemp;
    }

    const tempInfo = await this.checkTemporaryFile(type, id);
    return tempInfo.hasTemp === true;
  },

  async runProjectOperation(operation, id, options = {}) {
    const endpoint = PROJECT_OPERATION_ENDPOINTS[operation];
    if (!endpoint) {
      throw new Error(`Unsupported project operation: ${operation}`);
    }

    const hasTemp = await this.resolveTempState('projects', id, options);
    if (hasTemp) {
      try {
        await this.applySingleChange('projects', id);
      } catch (applyError) {
        console.error(`Failed to apply changes for project ${id} before ${operation}:`, applyError);
        throw new Error(`Failed to apply changes before ${operation}: ${applyError.message}`);
      }
    }

    const response = await api.post(endpoint, { project_id: id });
    return response.data;
  },

  setBearer(idToken) {
    localStorage.setItem('auth_bearer', idToken);
    api.defaults.headers.Authorization = `Bearer ${idToken}`;
  },

  /**
   * Fetch components with temporary file information (unified interface)
   * @param {string} type - Component type (inputs, outputs, rulesets, plugins, projects)
   * @returns {Array} - Components with hasTemp flag
   */
  async fetchComponentsWithTempInfo(type) {
    try {
      // Direct API call instead of using deprecated fetch methods
      let response;
      switch (type) {
        case 'inputs':
        case 'outputs':
        case 'rulesets':
        case 'plugins':
        case 'projects':
        case 'agents':
        case 'skills':
          response = await fetchComponentsByType(type);
          break;
        case 'cluster':
          response = await this.fetchClusterInfo();
          break;
        default:
          return [];
      }
      
      // Ensure each component has correct hasTemp property and belongs to correct component type
      if (Array.isArray(response)) {
        // Filter out potentially incorrect components due to ID conflicts
        response = response.filter(item => {
          // For plugins, check if has name field; for other components, check if has id field
          if (type === 'plugins' && !item.name && item.id) {
            // console.warn(`Filtered out invalid plugin item:`, item);
            return false;
          } else if (type !== 'plugins' && !item.id) {
            // console.warn(`Filtered out invalid ${type} item:`, item);
            return false;
          }
          return true;
        });
        
        // Ensure all components have hasTemp property - directly from backend response
        for (const item of response) {
          // hasTemp should be set by backend, but ensure it exists
          if (item.hasTemp === undefined) {
            item.hasTemp = false;
          }
          // Don't override backend hasTemp value with path checking here
          // The backend hasTemp is authoritative as it checks memory state
        }
      }
      
      return response;
    } catch (error) {
      return handleApiError(error, `Error fetching ${type}:`, true);
    }
  },

  // Legacy fetch methods removed - use fetchComponentsWithTempInfo instead

  async getInput(id) {
    return this.getComponent('inputs', id);
  },

  async getOutput(id) {
    return this.getComponent('outputs', id);
  },

  async getRuleset(id) {
    return this.getComponent('rulesets', id);
  },

  async getProject(id) {
    try {
      const response = await api.get(`/projects/${id}`);
      // Don't automatically fetch error details to prevent spamming the backend
      // Error details should be fetched explicitly when needed
      return response.data;
    } catch (error) {
      console.error(`Error fetching project ${id}:`, error);
      throw error;
    }
  },

  async getPlugin(id) {
    try {
      const response = await api.get(`/plugins/${id}`);
      return response.data;
    } catch (error) {
      if (error.response && error.response.status === 404) {
        throw new Error(`Plugin ${id} not found`);
      }
      throw new Error(error.message || 'Failed to get plugin');
    }
  },

  async createInput(id, raw) {
    return this.createComponent('inputs', id, raw);
  },

  async createOutput(id, raw) {
    return this.createComponent('outputs', id, raw);
  },

  async createRuleset(id, raw, folder = '') {
    return this.createComponent('rulesets', id, raw, { folder: folder || '' });
  },

  // Ruleset folder management
  async getRulesetFolders() {
    try {
      const response = await api.get('/ruleset-folders');
      return response.data || [];
    } catch (error) {
      return handleApiError(error, 'Error fetching ruleset folders:', true);
    }
  },

  async createRulesetFolder(name) {
    const response = await api.post('/ruleset-folders', { name });
    return response.data;
  },

  async renameRulesetFolder(oldName, newName) {
    const response = await api.put(`/ruleset-folders/${oldName}`, { new_name: newName });
    return response.data;
  },

  async deleteRulesetFolder(name) {
    const response = await api.delete(`/ruleset-folders/${name}`);
    return response.data;
  },

  async moveRuleset(id, folder) {
    const response = await api.put(`/rulesets/${id}/move`, { folder: folder || '' });
    return response.data;
  },

  async createProject(id, raw) {
    return this.createComponent('projects', id, raw);
  },

  async createPlugin(id, raw) {
    return this.createComponent('plugins', id, raw);
  },

  // Generic component deletion function
  async deleteComponent(type, id) {
    const response = await api.delete(getComponentEndpoint(type, id));
    return response.data;
  },

  async deleteInput(id) {
    return this.deleteAndDispatch('inputs', id);
  },

  async deleteOutput(id) {
    return this.deleteAndDispatch('outputs', id);
  },

  async deleteRuleset(id) {
    return this.deleteAndDispatch('rulesets', id);
  },

  async deleteProject(id) {
    return this.deleteAndDispatch('projects', id);
  },

  async deletePlugin(id) {
    return this.deleteAndDispatch('plugins', id);
  },

  // Agent CRUD
  async getAgent(id) {
    return this.getComponent('agents', id);
  },

  async createAgent(id, raw) {
    return this.createComponent('agents', id, raw);
  },

  async updateAgent(id, raw) {
    return this.updateComponent('agents', id, raw);
  },

  async deleteAgent(id) {
    return this.deleteAndDispatch('agents', id);
  },

  // Skill CRUD
  async getSkill(id) {
    return this.getComponent('skills', id);
  },

  async createSkill(id, raw) {
    return this.createComponent('skills', id, raw);
  },

  async updateSkill(id, raw) {
    return this.updateComponent('skills', id, raw);
  },

  async deleteSkill(id) {
    return this.deleteAndDispatch('skills', id);
  },

  async startProject(id, options = {}) {
    try {
      return await this.runProjectOperation('start', id, options);
    } catch (error) {
      console.error(`Error starting project ${id}:`, error);
      throw error;
    }
  },

  async stopProject(id, options = {}) {
    try {
      return await this.runProjectOperation('stop', id, options);
    } catch (error) {
      console.error(`Error stopping project ${id}:`, error);
      throw error;
    }
  },

  async updatePlugin(id, raw) {
    try {
      return await this.updateComponent('plugins', id, raw);
    } catch (error) {
      if (error.response && error.response.data && error.response.data.error) {
        throw new Error(error.response.data.error);
      }
      throw new Error(error.message || 'Failed to update plugin');
    }
  },

  async updateInput(id, raw) {
    return this.updateComponent('inputs', id, raw);
  },

  async updateOutput(id, raw) {
    return this.updateComponent('outputs', id, raw);
  },

  async updateRuleset(id, raw) {
    return this.updateComponent('rulesets', id, raw);
  },

  async updateProject(id, raw) {
    return this.updateComponent('projects', id, raw);
  },

  // Get enhanced pending changes with status information
  async fetchEnhancedPendingChanges() {
    try {
      const response = await api.get('/pending-changes/enhanced');
      return response.data || [];
    } catch (error) {
      return handleApiError(error, 'Error fetching enhanced pending changes:', true);
    }
  },

  // Verify all pending changes without applying them
  async verifyPendingChanges() {
    try {
      const response = await api.post('/verify-changes');
      return response.data;
    } catch (error) {
      console.error('Error verifying pending changes:', error);
      throw error;
    }
  },


  // Cancel a single pending change
  async cancelPendingChange(type, id) {
    try {
      const response = await api.delete(`/cancel-change/${type}/${id}`);
      return response.data;
    } catch (error) {
      console.error('Error cancelling pending change:', error);
      throw error;
    }
  },

  // Cancel all pending changes
  async cancelAllPendingChanges() {
    try {
      const response = await api.delete('/cancel-all-changes');
      return response.data;
    } catch (error) {
      console.error('Error cancelling all pending changes:', error);
      throw error;
    }
  },
  
  // Apply a single pending change
  async applySingleChange(type, id) {
    try {
      const response = await api.post('/apply-single-change', { type, id });
      return response.data;
    } catch (error) {
      if (error.response && error.response.data && error.response.data.error &&
          error.response.data.error.includes('verification failed')) {
        throw {
          message: error.response.data.error,
          isVerificationError: true
        };
      }
      throw error;
    }
  },
  
  // Restart a specific project
  async restartProject(id, options = {}) {
    try {
      return await this.runProjectOperation('restart', id, options);
    } catch (error) {
      console.error(`Error restarting project ${id}:`, error);
      throw error;
    }
  },
  
  // Verify component configuration
  async verifyComponent(type, id, raw) {
    try {
      if (!type || !id) {
        return buildVerifyFailure('Missing component type or ID');
      }
      
      if (raw !== undefined) {
        const response = await api.post(`/verify/${type}/${id}`, { raw });
        return response;
      }

      const componentData = await this.getComponentRaw(type, id);
      if (!componentData || !componentData.raw) {
        return buildVerifyFailure(`Component not found or has no content: ${id}`);
      }

      const response = await api.post(`/verify/${type}/${id}`, { raw: componentData.raw });
      return response;
    } catch (error) {
      console.error('Verification API error:', error);
      
      // If this is an HTTP error with response data, return it as-is to preserve structure
      if (error.response && error.response.data) {
        return error.response;
      }
      
      return buildVerifyFailure(error.message || 'Unknown verification error');
    }
  },

  // Add saveEdit function
  async saveEdit(type, id, raw) {
    const pluralType = normalizeComponentType(type, 'plural');
    const updaterName = `update${pluralType.slice(0, -1).replace(/^\w/, c => c.toUpperCase())}`;
    if (typeof this[updaterName] !== 'function') {
      throw new Error('Unsupported component type');
    }

    const response = await this[updaterName](id, raw);
    dispatchComponentChanged('updated', type, id);
    
    return response;
  },

  // Add saveNew function
  async saveNew(type, id, raw) {
    const pluralType = normalizeComponentType(type, 'plural');
    const creatorName = `create${pluralType.slice(0, -1).replace(/^\w/, c => c.toUpperCase())}`;
    if (typeof this[creatorName] !== 'function') {
      throw new Error('Unsupported component type');
    }

    const response = await this[creatorName](id, raw);
    dispatchComponentChanged('created', type, id);
    
    return response;
  },

  // Function to get all available plugins (simple format for testing)
  async getAvailablePlugins() {
    try {
      // Use the unified plugins API with parameters for simple format
      const response = await api.get('/plugins', {
        params: {
          detailed: 'false',
          include_temp: 'false',
          type: 'yaegi'
        }
      });
      return response.data || [];
    } catch (error) {
      console.error('Error fetching available plugins:', error);
      return [];
    }
  },
  
  // Add connection check function
  async connectCheck(type, id) {
    try {
      const componentType = normalizeComponentType(type, 'singular');
      
      // Basic validation
      if (!componentType || !id) {
        throw new Error('Component type and ID are required');
      }
      
      // Only input and output components support connection check
      if (componentType !== 'input' && componentType !== 'output') {
        return {
          success: false,
          error: 'Connection check is only supported for input and output components'
        };
      }
      
      // Send connection check request
      const response = await api.get(`/connect-check/${componentType}/${id}`);
      return response.data;
    } catch (error) {
      return buildResultError(error, `Failed to check connection for ${type} ${id}`);
    }
  },

  // Add connection check function with custom configuration
  async connectCheckWithConfig(type, id, configContent) {
    try {
      const componentType = normalizeComponentType(type, 'singular');
      
      // Basic validation
      if (!componentType || !id || !configContent) {
        throw new Error('Component type, ID, and configuration content are required');
      }
      
      // Only input and output components support connection check
      if (componentType !== 'input' && componentType !== 'output') {
        return {
          success: false,
          error: 'Connection check is only supported for input and output components'
        };
      }
      
      // Send connection check request with configuration
      const response = await api.post(`/connect-check/${componentType}/${id}`, { 
        raw: configContent 
      });
      return response.data;
    } catch (error) {
      return buildResultError(error, `Failed to check connection for ${type} ${id}`);
    }
  },
  
  // Test plugin component
  async testPlugin(id, data) {
    try {
      // Basic validation
      if (!id) {
        throw new Error('Plugin ID is required');
      }
      
      if (!Array.isArray(data)) {
        throw new Error('Test data must be an array');
      }
      
      // Convert array to object format expected by backend
      // For plugins, we need to create an object with indexed keys
      const pluginData = {};
      data.forEach((value, index) => {
        pluginData[index.toString()] = value;
      });
      
      // Use API instance to send request
      const response = await api.post(`/test-plugin/${id}`, { data: pluginData });
      return response.data;
    } catch (error) {
      return buildResultError(error, 'Failed to test plugin', { result: null });
    }
  },

  // Test ruleset component
  async testRuleset(id, data) {
    try {
      // Basic validation
      if (!id) {
        throw new Error('Ruleset ID is required');
      }
      
      const isObject = data && typeof data === 'object' && !Array.isArray(data);
      const isArray = Array.isArray(data);
      if (!isObject && !isArray) {
        throw new Error('Test data must be an object or array');
      }
      
      // Use API instance to send request
      const response = await api.post(`/test-ruleset/${id}`, { data });
      return response.data;
    } catch (error) {
      return buildResultError(error, 'Failed to test ruleset', { results: [] });
    }
  },

  // Test ruleset content
  async testRulesetContent(content, data) {
    try {
      // Basic validation
      if (!content) {
        throw new Error('Ruleset content is required');
      }
      
      const isObject = data && typeof data === 'object' && !Array.isArray(data);
      const isArray = Array.isArray(data);
      if (!isObject && !isArray) {
        throw new Error('Test data must be an object or array');
      }
      
      // Use API instance to send request
      const response = await api.post('/test-ruleset-content', { content, data });
      return response.data;
    } catch (error) {
      return buildResultError(error, 'Failed to test ruleset content', { results: [] });
    }
  },

  async testAgent(id, data) {
    try {
      if (!id) throw new Error('Agent ID is required');
      if (!data || typeof data !== 'object') throw new Error('Test data must be an object');
      const response = await api.post(`/test-agent/${id}`, { data });
      return response.data;
    } catch (error) {
      return buildResultError(error, 'Failed to test agent', { results: [] });
    }
  },

  async testAgentContent(content, data) {
    try {
      if (!content) throw new Error('Agent content is required');
      if (!data || typeof data !== 'object') throw new Error('Test data must be an object');
      const response = await api.post('/test-agent-content', { content, data });
      return response.data;
    } catch (error) {
      return buildResultError(error, 'Failed to test agent content', { results: [] });
    }
  },

  // Test plugin content
  async testPluginContent(content, data) {
    try {
      // Basic validation
      if (!content) {
        throw new Error('Plugin content is required');
      }
      
      if (!data || typeof data !== 'object') {
        throw new Error('Test data must be an object');
      }
      
      // Use API instance to send request
      const response = await api.post('/test-plugin-content', { content, data });
      return response.data;
    } catch (error) {
      return buildResultError(error, 'Failed to test plugin content', { result: null });
    }
  },

  // Test project content
  async testProjectContent(content, inputNode, data) {
    try {
      // Basic validation
      if (!content) {
        throw new Error('Project content is required');
      }
      
      if (!inputNode) {
        throw new Error('Input node is required');
      }
      
      if (!data || typeof data !== 'object') {
        throw new Error('Test data must be an object');
      }
      
      // Use API instance to send request
      const response = await api.post(`/test-project-content/${inputNode}`, { content, data });
      return response.data;
    } catch (error) {
      return buildResultError(error, 'Failed to test project content', { outputs: {} });
    }
  },

  // Test output component
  async testOutput(id, data) {
    try {
      // Basic validation
      if (!id) {
        throw new Error('Output ID is required');
      }
      
      if (!data || typeof data !== 'object') {
        throw new Error('Test data must be an object');
      }
      
      // Use API instance to send request
      const response = await api.post(`/test-output/${id}`, { data });
      return response.data;
    } catch (error) {
      return buildResultError(error, 'Failed to test output', { metrics: {} });
    }
  },

  // Test project component
  async testProject(id, inputNode, data) {
    try {
      const response = await api.post(`/test-project/${id}`, {
        input_node: inputNode,
        data: data
      });
      return response.data;
    } catch (error) {
      return buildResultError(error, 'Failed to test project', { outputs: {} });
    }
  },
  
  // Get project input nodes list
  async getProjectInputs(id) {
    try {
      // Basic validation
      if (!id) {
        throw new Error('Project ID is required');
      }
      
      // Use API instance to send request
      const response = await api.get(`/project-inputs/${id}`);
      return response.data;
    } catch (error) {
      return buildResultError(error, 'Failed to get project inputs', { inputs: [] });
    }
  },

  // Get cluster project states (leader only)
  async getClusterProjectStates() {
    try {
      const response = await api.get('/cluster-project-states');
      return response.data;
    } catch (error) {
      throw new Error(error.response?.data?.error || 'Failed to get cluster project states');
    }
  },

  // Get cluster information
  async fetchClusterInfo() {
    try {
      const response = await publicApi.get('/cluster-status');
      return response.data;
    } catch (error) {
      console.error('Error fetching cluster info:', error);
      throw new Error(error.response?.data?.error || 'Failed to get cluster info');
    }
  },

  // Get project components (inputs, outputs, rulesets)
  async getProjectComponents(id) {
    try {
      // Basic validation
      if (!id) {
        throw new Error('Project ID is required');
      }
      
      // Use API instance to send request
      const response = await api.get(`/project-components/${id}`);
      return response.data;
    } catch (error) {
      // If HTTP error, return error message
      if (error.response && error.response.data) {
        return {
          success: false,
          error: error.response.data.error || 'Failed to get project components',
          totalComponents: 0,
          componentCounts: { inputs: 0, outputs: 0, rulesets: 0 }
        };
      }
      // If network error or other error
      return {
        success: false,
        error: error.message || 'Network error or server not responding',
        totalComponents: 0,
        componentCounts: { inputs: 0, outputs: 0, rulesets: 0 }
      };
    }
  },

  // Add a method to check if component has temporary files
  async checkTemporaryFile(type, id) {
    try {
      if (!id) {
        return { hasTemp: false };
      }
      
      let data;
      
      // Retrieve component information directly from the API
      try {
        const response = await api.get(getComponentEndpoint(type, id));
        data = response.data;
        
        // Verify that the returned data indeed belongs to the requested component type
        // All components should now have an ID field
        if (!data.id) {
          console.error(`Invalid ${type} data for ${id}:`, data);
          return { hasTemp: false };
        }
        
        // Check if the returned data contains path information and if it's a temporary file
        return {
          hasTemp: data && data.path && data.path.endsWith('.new'),
          data: data
        };
      } catch (error) {
        // If the API returns 404, it means that the component does not exist
        if (error.response && error.response.status === 404) {
          // console.debug(`${type} ${id} not found`);
        } else {
          console.error(`Error fetching ${type} ${id}:`, error);
        }
        return { hasTemp: false };
      }
    } catch (error) {
      console.error('Error checking temporary file:', error);
      return { hasTemp: false };
    }
  },

  // Obtain which projects are using the component
  async getComponentUsage(type, id) {
    try {
      // The backend API expects complex component types and directly uses the passed type
      const response = await api.get(`/component-usage/${type}/${id}`);
      return response.data;
    } catch (error) {
      return handleApiError(error, `Error fetching usage for ${type} ${id}:`, true);
    }
  },

  // Load Local Components API functions
  async fetchLocalChanges() {
    try {
      const response = await api.get('/local-changes');
      return response.data || [];
    } catch (error) {
      console.error('Error fetching local changes:', error);
      throw error;
    }
  },

  // Lightweight local changes count for badges
  async fetchLocalChangesCount() {
    try {
      const response = await api.get('/local-changes/count');
      return response.data?.count || 0;
    } catch (error) {
      console.error('Error fetching local changes count:', error);
      return 0;
    }
  },

  async loadLocalChanges() {
    try {
      const response = await api.post('/load-local-changes');
      return response.data;
    } catch (error) {
      console.error('Error loading local changes:', error);
      throw error;
    }
  },

  async loadSingleLocalChange(type, id) {
    try {
      const response = await api.post('/load-single-local-change', {
        type: type,
        id: id
      });
      return response.data;
    } catch (error) {
      console.error(`Error loading single local change for ${type}/${id}:`, error);
      throw error;
    }
  },

  async getSamplerData(componentName, projectNodeSequence) {
    try {
      const params = {
        name: componentName,
        projectNodeSequence: projectNodeSequence
      };
      const response = await api.get('/samplers/data', { params });
      return response.data;
    } catch (error) {
      return handleApiError(error, 'Error fetching sampler data:', true);
    }
  },

  async getRulesetFields(id) {
    try {
      const response = await api.get(`/ruleset-fields/${id}`);
      return response.data;
    } catch (error) {
              // console.warn(`Failed to fetch ruleset fields for ${id}:`, error);
      return { fieldKeys: [], sampleCount: 0 };
    }
  },

  async getPluginParameters(id) {
    try {
      const response = await api.get(`/plugin-parameters/${id}`);
      return response.data;
    } catch (error) {
      console.error(`Error fetching plugin parameters for ${id}:`, error);
      throw error;
    }
  },

    async getProjectDailyMessages(projectId, extraParams = {}) {
    try {
      const response = await publicApi.get('/daily-messages', { 
        params: { project_id: projectId, ...extraParams } 
      });
      return response.data;
    } catch (error) {
      console.error(`Error fetching daily messages for project ${projectId}:`, error);
      throw error;
    }
  },

  // Get project component sequences - returns each component's projectNodeSequence list in current project
  async getProjectComponentSequences(projectId, extraParams = {}) {
    try {
      const response = await api.get(`/project-component-sequences/${projectId}`, {
        params: extraParams
      });
      return response.data;
    } catch (error) {
      console.error(`Error fetching component sequences for project ${projectId}:`, error);
      throw error;
    }
  },

  // Get plugin usage information
  async getPluginUsage(pluginId) {
    try {
      const response = await api.get(`/plugins/${pluginId}/usage`);
      return response.data;
    } catch (error) {
      console.error(`Error fetching plugin usage for ${pluginId}:`, error);
      throw error;
    }
  },

  async getAggregatedDailyMessages() {
    try {
      const response = await publicApi.get('/daily-messages', { 
        params: { aggregated: true } 
      });
      return response.data;
    } catch (error) {
      console.error('Error fetching aggregated daily messages:', error);
      throw error;
    }
  },

  async getAllNodeDailyMessages() {
    try {
      const response = await publicApi.get('/daily-messages', { 
        params: { by_node: true } 
      });
      return response.data;
    } catch (error) {
      console.error('Error fetching daily messages for all nodes:', error);
      throw error;
    }
  },

  async getCurrentSystemMetrics() {
    try {
      const response = await publicApi.get('/system-metrics', { params: { current: true } });
      return response.data;
    } catch (error) {
      console.error('Error fetching current system metrics:', error);
      throw error;
    }
  },

  // Cluster System Metrics APIs (now available from any node)
  // Use publicApi for statistics - no token required
  async getClusterSystemMetrics(nodeId = null) {
    try {
      const params = {};
      if (nodeId) {
        params.node_id = nodeId;
      }
      const response = await publicApi.get('/cluster-system-metrics', { params });
      return response.data;
    } catch (error) {
      console.error('Error fetching cluster system metrics:', error);
      throw error;
    }
  },

  // Error log endpoints
  async getErrorLogs(params = {}) {
    try {
      const response = await api.get('/error-logs', { params });
      return response.data;
    } catch (error) {
      console.error('Error fetching error logs:', error);
      throw new Error(error.response?.data?.error || error.message || 'Failed to fetch error logs');
    }
  },

  // Agent logs endpoint
  async getAgentLogs(params = {}) {
    try {
      const response = await api.get('/agent-logs', { params });
      return response.data;
    } catch (error) {
      console.error('Error fetching agent logs:', error);
      throw new Error(error.response?.data?.error || error.message || 'Failed to fetch agent logs');
    }
  },

  async generateAgentMemoryFromLog(agentId, logId, opts = {}) {
    try {
      const body = { log_id: logId };
      if (opts.comment != null && String(opts.comment).trim() !== '') {
        body.comment = String(opts.comment).trim();
        if (opts.tag != null && String(opts.tag).trim() !== '') {
          body.tag = String(opts.tag).trim();
        }
      }
      const response = await api.post(`/agents/${encodeURIComponent(agentId)}/memory-notes/generate-from-log`, body);
      return response.data;
    } catch (error) {
      console.error('Error generating agent memory from log:', error);
      const msg =
        error.response?.data?.error || error.message || 'Failed to generate memory from log';
      const err = new Error(msg);
      err.status = error.response?.status;
      err.data = error.response?.data;
      throw err;
    }
  },

  async getErrorLogNodes() {
    try {
      const response = await api.get('/error-logs/nodes');
      return response.data;
    } catch (error) {
      console.error('Error fetching known nodes for error logs:', error);
      throw new Error(error.response?.data?.error || error.message || 'Failed to fetch known nodes');
    }
  },

  // Search components configuration
  async searchComponents(query) {
    try {
      const response = await api.get('/search-components', { 
        params: { q: query } 
      });
      return response.data;
    } catch (error) {
      console.error('Error searching components:', error);
      throw error;
    }
  },

  // Operations History API functions
  async getOperationsHistory(params = '') {
    try {
      const response = await api.get('/operations-history', {
        params: normalizeParams(params)
      });
      return response.data;
    } catch (error) {
      console.error('Error fetching operations history:', error);
      throw error;
    }
  },

  async getOperationsHistoryNodes() {
    try {
      const response = await api.get('/operations-history/nodes');
      return response.data;
    } catch (error) {
      console.error('Error fetching known nodes for operations history:', error);
      throw new Error(error.response?.data?.error || error.message || 'Failed to fetch known nodes');
    }
  },

  async getPluginStats(params = {}) {
    try {
      const response = await api.get('/plugin-stats', { params });
      return response.data;
    } catch (error) {
      return handleApiError(error, 'Error fetching plugin stats:', true);
    }
  },
};

/**
 * Generic function to fetch components by type
 * @param {string} type - Component type
 * @param {string} endpoint - API endpoint
 * @returns {Promise<Array>} - Array of components with temp file info
 */
fetchComponentsByType = async (type) => {
  try {
    let apiEndpoint;
    switch(type) {
      case 'inputs':
        apiEndpoint = '/inputs';
        break;
      case 'outputs':
        apiEndpoint = '/outputs';
        break;
      case 'rulesets':
        apiEndpoint = '/rulesets';
        break;
      case 'plugins':
        apiEndpoint = '/plugins?detailed=true';
        break;
      case 'projects':
        apiEndpoint = '/projects';
        break;
      case 'agents':
        apiEndpoint = '/agents';
        break;
      case 'skills':
        apiEndpoint = '/skills';
        break;
      default:
        throw new Error(`Unsupported component type: ${type}`);
    }
    
    const response = await api.get(apiEndpoint);
    const items = response.data || [];
    
    // Create a map to track unique components by ID
    const uniqueItems = new Map();
    
    // Process each item without additional temp file checking
    for (const item of items) {
      // Get component ID (for plugins, use name as ID)
      const id = item.id || item.name;
      if (!id) continue;
      
      // Backend already provides hasTemp property based on memory state
      // This is more reliable than checking file existence
      if (item.hasTemp === undefined) {
        item.hasTemp = false;
      }
      
      // Store in Map, ensuring that each ID has only one component
      // If there is already a component with the same ID, prefer the one with hasTemp=true
      if (!uniqueItems.has(id) || item.hasTemp) {
        uniqueItems.set(id, item);
      }
    }
    
    // Convert back to array and sort
    const result = Array.from(uniqueItems.values());
    result.sort((a, b) => {
      const idA = a.id || a.name || '';
      const idB = b.id || b.name || '';
      return idA.localeCompare(idB);
    });
    return result;
  } catch (error) {
    return handleApiError(error, `Error fetching ${type}:`, true);
  }
}; 