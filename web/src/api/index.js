import axios from 'axios';
import config from '../config';

const api = axios.create({
  baseURL: config.apiBaseUrl,
  timeout: config.apiTimeout,
  headers: {
    'Content-Type': 'application/json',
  },
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

// Add request interceptor to add token to all requests
api.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('auth_token');
    if (token) {
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
      localStorage.removeItem('auth_token');
      console.error('Authentication failed: Token invalid or expired');
      
      // 安全地跳转到登录页面
      if (typeof window !== 'undefined') {
        // 检查当前路径，如果不是登录页则跳转
        const currentPath = window.location.pathname;
        const isLoginPage = currentPath === '/' || currentPath === '/login' || 
                           currentPath.startsWith('/#/') || currentPath.includes('/login');
        
        if (!isLoginPage) {
          if (window.router) {
            // 如果在Vue Router环境中，使用路由跳转
            try {
              window.router.push({ name: 'Login' });
            } catch (routerError) {
              // 路由跳转失败时使用location跳转
              window.location.replace('/');
            }
          } else {
            // 使用replace避免在历史记录中留下记录
            window.location.replace('/');
          }
        }
      }
    }
    if (typeof window !== 'undefined' && window.$toast) {
      let msg = error.response?.data?.error || error.message || 'Unknown error';
      window.$toast.show(msg, 'error');
    }
    return Promise.reject(error);
  }
);

/**
 * Generic function to fetch components by type
 * @param {string} type - Component type
 * @param {string} endpoint - API endpoint
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
    delete api.defaults.headers.token;
  },

  async verifyToken() {
    try {
      const response = await api.get('/token-check');
      return response.data;
    } catch (error) {
      // 清除token，避免无限刷新
      this.clearToken();
      throw error;
    }
  },

  /**
   * Fetch components with temporary file information
   * @param {string} type - Component type (inputs, outputs, rulesets, plugins, projects)
   * @returns {Array} - Components with hasTemp flag
   */
  async fetchComponentsWithTempInfo(type) {
    try {
      let response;
      switch (type) {
        case 'inputs':
          response = await this.fetchInputs();
          break;
        case 'outputs':
          response = await this.fetchOutputs();
          break;
        case 'rulesets':
          response = await this.fetchRulesets();
          break;
        case 'plugins':
          response = await this.fetchPlugins();
          break;
        case 'projects':
          response = await this.fetchProjects();
          break;
        case 'cluster':
          response = await this.fetchClusterInfo();
          break;
        default:
          return [];
      }
      
      // 确保每个组件都有正确的hasTemp属性，并且属于正确的组件类型
      if (Array.isArray(response)) {
        // 过滤掉可能由于ID冲突导致错误添加的其他类型组件
        response = response.filter(item => {
          // 对于插件，检查是否有name字段；对于其他组件，检查是否有id字段
          if (type === 'plugins' && !item.name && item.id) {
            console.warn(`Filtered out invalid plugin item:`, item);
            return false;
          } else if (type !== 'plugins' && !item.id) {
            console.warn(`Filtered out invalid ${type} item:`, item);
            return false;
          }
          return true;
        });
        
        // 确保所有组件都有hasTemp属性
        for (const item of response) {
          if (item.hasTemp === undefined) {
            item.hasTemp = false;
          }
        }
      }
      
      return response;
    } catch (error) {
      return handleApiError(error, `Error fetching ${type}:`, true);
    }
  },

  async fetchInputs() {
    return fetchComponentsByType('inputs', '/inputs');
  },

  async fetchOutputs() {
    return fetchComponentsByType('outputs', '/outputs');
  },

  async fetchRulesets() {
    return fetchComponentsByType('rulesets', '/rulesets');
  },

  async fetchPlugins() {
    return fetchComponentsByType('plugins', '/plugins');
  },

  async fetchProjects() {
    return fetchComponentsByType('projects', '/projects');
  },

  async getInput(id) {
    const response = await api.get(`/inputs/${id}`);
    return response.data;
  },

  async getOutput(id) {
    const response = await api.get(`/outputs/${id}`);
    return response.data;
  },

  async getRuleset(id) {
    const response = await api.get(`/rulesets/${id}`);
    return response.data;
  },

  async getProject(id) {
    try {
      const response = await api.get(`/projects/${id}`);
      // 如果项目状态为error，尝试获取错误信息
      if (response.data && response.data.status === 'error') {
        try {
          // 尝试获取项目的错误信息
          const errorResponse = await api.get(`/project-error/${id}`);
          if (errorResponse.data && errorResponse.data.error) {
            response.data.errorMessage = errorResponse.data.error;
          }
        } catch (errorFetchError) {
          console.warn(`Failed to fetch error details for project ${id}:`, errorFetchError);
          // 设置一个默认的错误信息
          response.data.errorMessage = "Unknown error occurred";
        }
      }
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
    const response = await api.post('/inputs', { id, raw });
    return response.data;
  },

  async createOutput(id, raw) {
    const response = await api.post('/outputs', { id, raw });
    return response.data;
  },

  async createRuleset(id, raw) {
    const response = await api.post('/rulesets', { id, raw });
    return response.data;
  },

  async createProject(id, raw) {
    const response = await api.post('/projects', { id, raw });
    return response.data;
  },

  async createPlugin(id, raw) {
    const response = await api.post('/plugins', { id, raw });
    return response.data;
  },

  // Generic component deletion function
  async deleteComponent(type, id) {
    try {
      // Ensure type is plural for API call
      let componentType = type;
      if (!componentType.endsWith('s')) {
        componentType = componentType + 's';
      }
      
      const response = await api.delete(`/${componentType}/${id}`);
      return response.data;
    } catch (error) {
      throw error;
    }
  },

  async deleteInput(id) {
    return this.deleteComponent('inputs', id);
  },

  async deleteOutput(id) {
    return this.deleteComponent('outputs', id);
  },

  async deleteRuleset(id) {
    return this.deleteComponent('rulesets', id);
  },

  async deleteProject(id) {
    return this.deleteComponent('projects', id);
  },

  async deletePlugin(id) {
    return this.deleteComponent('plugins', id);
  },

  async startProject(id) {
    try {
      // Check if temporary file exists
      const tempInfo = await this.checkTemporaryFile('projects', id);
      
      // If temporary file exists, apply the changes first
      if (tempInfo.hasTemp) {
        try {
          await this.applySingleChange('projects', id);
        } catch (applyError) {
          console.error(`Failed to apply changes for project ${id} before starting:`, applyError);
          throw new Error(`Failed to apply changes before starting: ${applyError.message}`);
        }
      }
      
      // Start the project
      const response = await api.post('/start-project', { project_id: id });
      return response.data;
    } catch (error) {
      console.error(`Error starting project ${id}:`, error);
      throw error;
    }
  },

  async stopProject(id) {
    try {
      // Check if temporary file exists
      const tempInfo = await this.checkTemporaryFile('projects', id);
      
      // If temporary file exists, apply the changes first
      if (tempInfo.hasTemp) {
        try {
          await this.applySingleChange('projects', id);
        } catch (applyError) {
          console.error(`Failed to apply changes for project ${id} before stopping:`, applyError);
          throw new Error(`Failed to apply changes before stopping: ${applyError.message}`);
        }
      }
      
      // Stop the project
      const response = await api.post('/stop-project', { project_id: id });
      return response.data;
    } catch (error) {
      console.error(`Error stopping project ${id}:`, error);
      throw error;
    }
  },

  async fetchClusterStatus() {
    const response = await api.get('/cluster-status');
    return response.data;
  },

  async fetchClusterInfo() {
    const response = await api.get('/cluster-status');
    return response.data;
  },

  async updatePlugin(id, raw) {
    try {
      // Ensure raw is a string
      const rawString = typeof raw === 'object' ? JSON.stringify(raw) : String(raw || '');
      const response = await api.put(`/plugins/${id}`, { raw: rawString });
      return response.data;
    } catch (error) {
      if (error.response && error.response.data && error.response.data.error) {
        throw new Error(error.response.data.error);
      }
      throw new Error(error.message || 'Failed to update plugin');
    }
  },

  async updateInput(id, raw) {
    // Ensure raw is a string
    const rawString = typeof raw === 'object' ? JSON.stringify(raw) : String(raw || '');
    const response = await api.put(`/inputs/${id}`, { raw: rawString });
    return response.data;
  },

  async updateOutput(id, raw) {
    // Ensure raw is a string
    const rawString = typeof raw === 'object' ? JSON.stringify(raw) : String(raw || '');
    const response = await api.put(`/outputs/${id}`, { raw: rawString });
    return response.data;
  },

  async updateRuleset(id, raw) {
    // Ensure raw is a string
    const rawString = typeof raw === 'object' ? JSON.stringify(raw) : String(raw || '');
    const response = await api.put(`/rulesets/${id}`, { raw: rawString });
    return response.data;
  },

  async updateProject(id, raw) {
    // Ensure raw is a string
    const rawString = typeof raw === 'object' ? JSON.stringify(raw) : String(raw || '');
    const response = await api.put(`/projects/${id}`, { raw: rawString });
    return response.data;
  },

  // Get all pending changes (temporary files) - Legacy API
  async fetchPendingChanges() {
    const response = await api.get('/pending-changes');
    return response.data;
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

  // Apply all pending changes - Legacy API
  async applyPendingChanges() {
    try {
      const response = await api.post('/apply-changes');
      return response.data;
    } catch (error) {
      // 如果是验证失败的错误，特殊处理
      if (error.response && error.response.data && error.response.data.verify_failures) {
        throw {
          message: error.response.data.error,
          verifyFailures: error.response.data.verify_failures,
          successCount: error.response.data.success_count,
          failureCount: error.response.data.failure_count
        };
      }
      throw error;
    }
  },

  // Apply all pending changes with enhanced transaction support
  async applyPendingChangesEnhanced() {
    try {
      const response = await api.post('/apply-changes/enhanced');
      return response.data;
    } catch (error) {
      console.error('Error applying pending changes (enhanced):', error);
      throw error;
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

  // Verify a single pending change
  async verifySinglePendingChange(type, id) {
    try {
      const response = await api.post(`/verify-change/${type}/${id}`);
      return response.data;
    } catch (error) {
      console.error('Error verifying single pending change:', error);
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

  // Create temporary file (for editing existing files)
  async createTempFile(type, id) {
    try {
      // Directly call backend API to create temporary file
      const response = await api.post(`/temp-file/${type}/${id}`);
      return response.data;
    } catch (error) {
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
  
  // Restart all projects
  async restartAllProjects() {
    const response = await api.post('/restart-all-projects');
    return response.data;
  },
  
  // Restart a specific project
  async restartProject(id) {
    if (id === 'all') {
      return this.restartAllProjects();
    }
    
    try {
      // First check if the project has temporary files
      const tempInfo = await this.checkTemporaryFile('projects', id);
      if (tempInfo.hasTemp) {
        // Apply the changes first
        try {
          await this.applySingleChange('projects', id);
        } catch (applyError) {
          console.error(`Failed to apply changes for project ${id} before restarting:`, applyError);
          return {
            success: false,
            error: `Failed to apply changes before restarting: ${applyError.message}`
          };
        }
      }
      
      // Use the dedicated restart endpoint
      const response = await api.post('/restart-project', { project_id: id });
      return response.data;
    } catch (error) {
      console.error(`Error restarting project ${id}:`, error);
      return {
        success: false,
        error: error.response?.data?.error || error.message || 'Unknown error'
      };
    }
  },
  
  // Verify component configuration
  async verifyComponent(type, id, raw) {
    try {
      if (!type || !id) {
        return {
          data: {
            valid: false,
            error: 'Missing component type or ID'
          }
        };
      }
      
      if (raw !== undefined) {
        const response = await api.post(`/verify/${type}/${id}`, { raw });
        // Return the complete response data to preserve detailed error information
        return response;
      } else {
        // If raw is not provided, get component and validate
        let componentData;
        switch (type) {
          case 'inputs':
            componentData = await this.getInput(id);
            break;
          case 'outputs':
            componentData = await this.getOutput(id);
            break;
          case 'rulesets':
            componentData = await this.getRuleset(id);
            break;
          case 'projects':
            componentData = await this.getProject(id);
            break;
          case 'plugins':
            componentData = await this.getPlugin(id);
            break;
          default:
            return {
              data: {
                valid: false,
                error: `Unsupported component type: ${type}`
              }
            };
        }
        
        if (!componentData || !componentData.raw) {
          return {
            data: {
              valid: false,
              error: `Component not found or has no content: ${id}`
            }
          };
        }
        
        const response = await api.post(`/verify/${type}/${id}`, { raw: componentData.raw });
        // Return the complete response data to preserve detailed error information
        return response;
      }
    } catch (error) {
      console.error('Verification API error:', error);
      
      // If this is an HTTP error with response data, return it as-is to preserve structure
      if (error.response && error.response.data) {
        return error.response;
      }
      
      // For other errors, return a simple error structure
      return {
        data: {
          valid: false,
          error: error.message || 'Unknown verification error'
        }
      };
    }
  },

  // Add saveEdit function
  async saveEdit(type, id, raw) {
    let response;
    switch (type) {
      case 'inputs':
        response = await this.updateInput(id, raw);
        break;
      case 'outputs':
        response = await this.updateOutput(id, raw);
        break;
      case 'rulesets':
        response = await this.updateRuleset(id, raw);
        break;
      case 'projects':
        response = await this.updateProject(id, raw);
        break;
      case 'plugins':
        response = await this.updatePlugin(id, raw);
        break;
      default:
        throw new Error('Unsupported component type');
    }
    return response;
  },

  // Add saveNew function
  async saveNew(type, id, raw) {
    let response;
    switch (type) {
      case 'inputs':
        response = await this.createInput(id, raw);
        break;
      case 'outputs':
        response = await this.createOutput(id, raw);
        break;
      case 'rulesets':
        response = await this.createRuleset(id, raw);
        break;
      case 'projects':
        response = await this.createProject(id, raw);
        break;
      case 'plugins':
        response = await this.createPlugin(id, raw);
        break;
      default:
        throw new Error('Unsupported component type');
    }
    return response;
  },

  // Function to get all available plugins
  async getAvailablePlugins() {
    try {
      const response = await api.get('/available-plugins');
      return response.data || [];
    } catch (error) {
      console.error('Error fetching available plugins:', error);
      return [];
    }
  },
  
  // Add connection check function
  async connectCheck(type, id) {
    try {
      // Normalize component type (remove trailing 's' if present)
      let componentType = type;
      if (componentType.endsWith('s')) {
        componentType = componentType.slice(0, -1);
      }
      
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
      // If HTTP error, return error message with details
      if (error.response && error.response.data) {
        return {
          success: false,
          error: error.response.data.error || `Failed to check connection for ${type} ${id}`
        };
      }
      
      // If network error or other error
      return {
        success: false,
        error: error.message || 'Network error or server not responding'
      };
    }
  },

  // Add connection check function with custom configuration
  async connectCheckWithConfig(type, id, configContent) {
    try {
      // Normalize component type (remove trailing 's' if present)
      let componentType = type;
      if (componentType.endsWith('s')) {
        componentType = componentType.slice(0, -1);
      }
      
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
      // If HTTP error, return error message with details
      if (error.response && error.response.data) {
        return {
          success: false,
          error: error.response.data.error || `Failed to check connection for ${type} ${id}`
        };
      }
      
      // If network error or other error
      return {
        success: false,
        error: error.message || 'Network error or server not responding'
      };
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
      // If HTTP error, return error message
      if (error.response && error.response.data) {
        return {
          success: false,
          error: error.response.data.error || 'Failed to test plugin',
          result: null
        };
      }
      // If network error or other error
      return {
        success: false,
        error: error.message || 'Network error or server not responding',
        result: null
      };
    }
  },

  // Test ruleset component
  async testRuleset(id, data) {
    try {
      // Basic validation
      if (!id) {
        throw new Error('Ruleset ID is required');
      }
      
      if (!data || typeof data !== 'object') {
        throw new Error('Test data must be an object');
      }
      
      // Use API instance to send request
      const response = await api.post(`/test-ruleset/${id}`, { data });
      return response.data;
    } catch (error) {
      // If HTTP error, return error message
      if (error.response && error.response.data) {
        return {
          success: false,
          error: error.response.data.error || 'Failed to test ruleset',
          results: []
        };
      }
      // If network error or other error
      return {
        success: false,
        error: error.message || 'Network error or server not responding',
        results: []
      };
    }
  },

  // Test ruleset content
  async testRulesetContent(content, data) {
    try {
      // Basic validation
      if (!content) {
        throw new Error('Ruleset content is required');
      }
      
      if (!data || typeof data !== 'object') {
        throw new Error('Test data must be an object');
      }
      
      // Use API instance to send request
      const response = await api.post('/test-ruleset-content', { content, data });
      return response.data;
    } catch (error) {
      // If HTTP error, return error message
      if (error.response && error.response.data) {
        return {
          success: false,
          error: error.response.data.error || 'Failed to test ruleset content',
          results: []
        };
      }
      // If network error or other error
      return {
        success: false,
        error: error.message || 'Network error or server not responding',
        results: []
      };
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
      // If HTTP error, return error message
      if (error.response && error.response.data) {
        return {
          success: false,
          error: error.response.data.error || 'Failed to test plugin content',
          result: null
        };
      }
      // If network error or other error
      return {
        success: false,
        error: error.message || 'Network error or server not responding',
        result: null
      };
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
      // If HTTP error, return error message
      if (error.response && error.response.data) {
        return {
          success: false,
          error: error.response.data.error || 'Failed to test project content',
          outputs: {}
        };
      }
      // If network error or other error
      return {
        success: false,
        error: error.message || 'Network error or server not responding',
        outputs: {}
      };
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
      // If HTTP error, return error message
      if (error.response && error.response.data) {
        return {
          success: false,
          error: error.response.data.error || 'Failed to test output',
          metrics: {}
        };
      }
      // If network error or other error
      return {
        success: false,
        error: error.message || 'Network error or server not responding',
        metrics: {}
      };
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
      return handleApiError(error, 'Error testing project:');
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
      // If HTTP error, return error message
      if (error.response && error.response.data) {
        return {
          success: false,
          error: error.response.data.error || 'Failed to get project inputs',
          inputs: []
        };
      }
      // If network error or other error
      return {
        success: false,
        error: error.message || 'Network error or server not responding',
        inputs: []
      };
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
      
      // Get component based on type
      let data;
      let endpoint;
      
      switch (type) {
        case 'inputs':
          endpoint = `/inputs/${id}`;
          break;
        case 'outputs':
          endpoint = `/outputs/${id}`;
          break;
        case 'rulesets':
          endpoint = `/rulesets/${id}`;
          break;
        case 'projects':
          endpoint = `/projects/${id}`;
          break;
        case 'plugins':
          endpoint = `/plugins/${id}`;
          break;
        default:
          return { hasTemp: false };
      }
      
      // Retrieve component information directly from the API
      try {
        const response = await api.get(endpoint);
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
          console.debug(`${type} ${id} not found`);
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

  // Cancel upgrade for a component
  async cancelUpgrade(type, id) {
    try {
      // Ensure type is plural for API call
      let componentType = type;
      if (!componentType.endsWith('s')) {
        componentType = componentType + 's';
      }
      
      const response = await api.post(`/cancel-upgrade/${componentType}/${id}`);
      return response.data;
    } catch (error) {
      if (error.response && error.response.data) {
        throw new Error(error.response.data.error || 'Failed to cancel upgrade');
      }
      throw new Error(error.message || 'Network error or server not responding');
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

  async getSamplerData(params) {
    try {
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
      console.warn(`Failed to fetch ruleset fields for ${id}:`, error);
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

  // Component Data APIs (backend still uses QPS endpoints for component enumeration)
  async getQPSData(params = {}) {
    try {
      const response = await api.get('/qps-data', { params });
      return response.data;
    } catch (error) {
      console.error('Error fetching component data:', error);
      throw error;
    }
  },

  async getQPSStats() {
    try {
      const response = await api.get('/qps-stats');
      return response.data;
    } catch (error) {
      console.error('Error fetching component stats:', error);
      throw error;
    }
  },

  async getProjectQPS(projectId, aggregated = false) {
    try {
      const params = { project_id: projectId };
      if (aggregated) {
        params.aggregated = true;
      }
      const response = await api.get('/qps-data', { params });
      return response.data;
    } catch (error) {
      console.error(`Error fetching component data for project ${projectId}:`, error);
      throw error;
    }
  },

  // Daily Messages APIs (real message counts for today from 00:00)
  async getDailyMessages(projectId = null, aggregated = false) {
    try {
      const params = {};
      if (projectId) {
        params.project_id = projectId;
      }
      if (aggregated) {
        params.aggregated = true;
      }
      const response = await api.get('/daily-messages', { params });
      return response.data;
    } catch (error) {
      console.error('Error fetching daily messages:', error);
      throw error;
    }
  },

  async getProjectDailyMessages(projectId) {
    try {
      const response = await api.get('/daily-messages', { 
        params: { project_id: projectId } 
      });
      return response.data;
    } catch (error) {
      console.error(`Error fetching daily messages for project ${projectId}:`, error);
      throw error;
    }
  },

  async getAggregatedDailyMessages() {
    try {
      const response = await api.get('/daily-messages', { 
        params: { aggregated: true } 
      });
      return response.data;
    } catch (error) {
      console.error('Error fetching aggregated daily messages:', error);
      throw error;
    }
  },

  async getNodeDailyMessages(nodeId) {
    try {
      const response = await api.get('/daily-messages', { 
        params: { by_node: true, node_id: nodeId } 
      });
      return response.data;
    } catch (error) {
      console.error(`Error fetching daily messages for node ${nodeId}:`, error);
      throw error;
    }
  },

  async getAllNodeDailyMessages() {
    try {
      const response = await api.get('/daily-messages', { 
        params: { by_node: true } 
      });
      return response.data;
    } catch (error) {
      console.error('Error fetching daily messages for all nodes:', error);
      throw error;
    }
  },

  // System Metrics APIs
  async getSystemMetrics(params = {}) {
    try {
      const response = await api.get('/system-metrics', { params });
      return response.data;
    } catch (error) {
      console.error('Error fetching system metrics:', error);
      throw error;
    }
  },

  async getCurrentSystemMetrics() {
    try {
      const response = await api.get('/system-metrics', { params: { current: true } });
      return response.data;
    } catch (error) {
      console.error('Error fetching current system metrics:', error);
      throw error;
    }
  },

  async getSystemStats() {
    try {
      const response = await api.get('/system-stats');
      return response.data;
    } catch (error) {
      console.error('Error fetching system stats:', error);
      throw error;
    }
  },

  async getHistoricalSystemMetrics(since) {
    try {
      const params = {};
      if (since) {
        params.since = since;
      }
      const response = await api.get('/system-metrics', { params });
      return response.data;
    } catch (error) {
      console.error('Error fetching historical system metrics:', error);
      throw error;
    }
  },

  // Cluster System Metrics APIs (only available from leader)
  async getClusterSystemMetrics(nodeId = null) {
    try {
      const params = {};
      if (nodeId) {
        params.node_id = nodeId;
      }
      const response = await api.get('/cluster-system-metrics', { params });
      return response.data;
    } catch (error) {
      console.error('Error fetching cluster system metrics:', error);
      throw error;
    }
  },

  async getClusterSystemStats() {
    try {
      const response = await api.get('/cluster-system-stats');
      return response.data;
    } catch (error) {
      console.error('Error fetching cluster system stats:', error);
      throw error;
    }
  },

  async getAggregatedSystemMetrics() {
    try {
      const response = await api.get('/cluster-system-metrics', { params: { aggregated: true } });
      return response.data;
    } catch (error) {
      console.error('Error fetching aggregated system metrics:', error);
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

  async getClusterErrorLogs(params = {}) {
    try {
      const response = await api.get('/cluster-error-logs', { params });
      return response.data;
    } catch (error) {
      console.error('Error fetching cluster error logs:', error);
      throw new Error(error.response?.data?.error || error.message || 'Failed to fetch cluster error logs');
    }
  }
};

/**
 * Generic function to fetch components by type
 * @param {string} type - Component type
 * @param {string} endpoint - API endpoint
 * @returns {Promise<Array>} - Array of components with temp file info
 */
fetchComponentsByType = async (type, endpoint) => {
  try {
    // Fix endpoint paths to match backend API routes
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
        apiEndpoint = '/plugins';
        break;
      case 'projects':
        apiEndpoint = '/projects';
        break;
      default:
        apiEndpoint = endpoint;
    }
    
    const response = await api.get(apiEndpoint);
    const items = response.data || [];
    
    // Create a map to track unique components by ID
    const uniqueItems = new Map();
    
    // Add temporary file information to each item
    for (const item of items) {
      // Get component ID (for plugins, use name as ID)
      const id = item.id || item.name;
      if (!id) continue;
      
      // Check for temporary files and ensure that only components of the current type are checked
      const tempInfo = await hubApi.checkTemporaryFile(type, id);
      
      // Set hasTemp property
      item.hasTemp = tempInfo.hasTemp;
      
      // Store in Map, ensuring that each ID has only one component
      // If there is already a component with the same ID, it should only be replaced when the new component is a temporary file
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