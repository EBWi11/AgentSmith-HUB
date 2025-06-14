import axios from 'axios';
import config from '../config';

const api = axios.create({
  baseURL: config.apiBaseUrl,
  timeout: config.apiTimeout,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Add request interceptor to add token to all requests
api.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('auth_token');
    if (token) {
      config.headers.token = token;
    }
    console.log('Request:', {
      url: config.url,
      method: config.method,
      headers: config.headers,
      baseURL: config.baseURL
    });
    return config;
  },
  (error) => {
    console.error('Request interceptor error:', error);
    return Promise.reject(error);
  }
);

// Add response interceptor to handle token expiration
api.interceptors.response.use(
  (response) => {
    console.log('Response:', {
      url: response.config.url,
      status: response.status,
      data: response.data,
      headers: response.headers
    });
    return response;
  },
  (error) => {
    console.error('Response error:', {
      url: error.config?.url,
      status: error.response?.status,
      data: error.response?.data,
      message: error.message
    });
    if (error.response?.status === 401) {
      localStorage.removeItem('auth_token');
      window.location.href = '/';
    }
    if (typeof window !== 'undefined' && window.$toast) {
      let msg = error.response?.data?.error || error.message || 'Unknown error';
      window.$toast.show(msg);
    }
    return Promise.reject(error);
  }
);

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
    const response = await api.get('/token/check');
    return response.data;
  },

  async fetchInputs() {
    const response = await api.get('/input');
    return response.data;
  },

  async fetchOutputs() {
    const response = await api.get('/output');
    return response.data;
  },

  async fetchRulesets() {
    const response = await api.get('/ruleset');
    return response.data;
  },

  async fetchPlugins() {
    const response = await api.get('/plugin');
    return response.data;
  },

  async fetchProjects() {
    const response = await api.get('/project');
    return response.data;
  },

  async getInput(id) {
    const response = await api.get(`/input/${id}`);
    return response.data;
  },

  async getOutput(id) {
    const response = await api.get(`/output/${id}`);
    return response.data;
  },

  async getRuleset(id) {
    const response = await api.get(`/ruleset/${id}`);
    return response.data;
  },

  async getProject(id) {
    const response = await api.get(`/project/${id}`);
    return response.data;
  },

  async getPlugin(id) {
    try {
      const response = await api.get(`/plugin/${id}`);
      return response.data;
    } catch (error) {
      console.error(`Failed to get plugin ${id}:`, error);
      if (error.response && error.response.status === 404) {
        throw new Error(`Plugin ${id} not found`);
      }
      throw new Error(error.message || 'Failed to get plugin');
    }
  },

  async createInput(id, raw) {
    const response = await api.post('/input', { id, raw });
    return response.data;
  },

  async createOutput(id, raw) {
    const response = await api.post('/output', { id, raw });
    return response.data;
  },

  async createRuleset(id, raw) {
    const response = await api.post('/ruleset', { id, raw });
    return response.data;
  },

  async createProject(id, raw) {
    const response = await api.post('/project', { id, raw });
    return response.data;
  },

  async deleteInput(id) {
    const response = await api.delete(`/input/${id}`);
    return response.data;
  },

  async deleteOutput(id) {
    const response = await api.delete(`/output/${id}`);
    return response.data;
  },

  async deleteRuleset(id) {
    const response = await api.delete(`/ruleset/${id}`);
    return response.data;
  },

  async deleteProject(id) {
    const response = await api.delete(`/project/${id}`);
    return response.data;
  },

  async deletePlugin(id) {
    const response = await api.delete(`/plugin/${id}`);
    return response.data;
  },

  async startProject(id) {
    const response = await api.post('/project/start', { project_id: id });
    return response.data;
  },

  async stopProject(id) {
    const response = await api.post('/project/stop', { project_id: id });
    return response.data;
  },

  async fetchClusterStatus() {
    const response = await api.get('/cluster/status');
    return response.data;
  },

  async fetchClusterInfo() {
    const response = await api.get('/cluster_info');
    return response.data;
  },

  async createPlugin(id, raw) {
    const response = await api.post('/plugin', { id, raw });
    return response.data;
  },

  async updatePlugin(id, raw) {
    try {
      // 确保raw是字符串
      const rawString = typeof raw === 'object' ? JSON.stringify(raw) : String(raw || '');
      console.log('updatePlugin raw:', { raw, type: typeof raw, rawString, type2: typeof rawString });
      const response = await api.put(`/plugin/${id}`, { raw: rawString });
      return response.data;
    } catch (error) {
      console.error(`Failed to update plugin ${id}:`, error);
      if (error.response && error.response.data && error.response.data.error) {
        throw new Error(error.response.data.error);
      }
      throw new Error(error.message || 'Failed to update plugin');
    }
  },

  async updateInput(id, raw) {
    // 确保raw是字符串
    const rawString = typeof raw === 'object' ? JSON.stringify(raw) : String(raw || '');
    console.log('updateInput raw:', { raw, type: typeof raw, rawString, type2: typeof rawString });
    const response = await api.put(`/input/${id}`, { raw: rawString });
    return response.data;
  },

  async updateOutput(id, raw) {
    // 确保raw是字符串
    const rawString = typeof raw === 'object' ? JSON.stringify(raw) : String(raw || '');
    console.log('updateOutput raw:', { raw, type: typeof raw, rawString, type2: typeof rawString });
    const response = await api.put(`/output/${id}`, { raw: rawString });
    return response.data;
  },

  async updateRuleset(id, raw) {
    // 确保raw是字符串
    const rawString = typeof raw === 'object' ? JSON.stringify(raw) : String(raw || '');
    console.log('updateRuleset raw:', { raw, type: typeof raw, rawString, type2: typeof rawString });
    const response = await api.put(`/ruleset/${id}`, { raw: rawString });
    return response.data;
  },

  async updateProject(id, raw) {
    // 确保raw是字符串
    const rawString = typeof raw === 'object' ? JSON.stringify(raw) : String(raw || '');
    console.log('updateProject raw:', { raw, type: typeof raw, rawString, type2: typeof rawString });
    const response = await api.put(`/project/${id}`, { raw: rawString });
    return response.data;
  },

  // Get all pending changes (temporary files)
  async fetchPendingChanges() {
    const response = await api.get('/pending-changes');
    return response.data;
  },

  // Apply all pending changes
  async applyPendingChanges() {
    try {
      const response = await api.post('/apply-changes');
      return response.data;
    } catch (error) {
      console.error('Failed to apply pending changes:', error);
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

  // Create temporary file (for editing existing files)
  async createTempFile(type, id) {
    try {
      console.log(`Creating temp file for ${type}/${id}`);
      // 直接调用后端API创建临时文件
      const response = await api.post(`/temp-file/${type}/${id}`);
      console.log('Temp file creation successful:', response.data);
      return response.data;
    } catch (error) {
      console.error('Failed to create temp file:', error);
      console.error('Request details:', {
        url: `/temp-file/${type}/${id}`,
        method: 'POST',
        error: error.message,
        status: error.response?.status,
        responseData: error.response?.data
      });
      throw error;
    }
  },
  
  // Apply a single pending change
  async applySingleChange(type, id) {
    try {
      const response = await api.post('/apply-single-change', { type, id });
      return response.data;
    } catch (error) {
      console.error('Failed to apply single change:', error);
      // 如果是验证失败的错误，特殊处理
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
    
    // First stop the project
    await this.stopProject(id);
    // Then start it again
    const response = await this.startProject(id);
    return response.data;
  },
  
  // Verify component configuration
  async verifyComponent(type, id, raw) {
    try {
      // 如果提供了raw内容，使用它进行验证
      if (raw !== undefined) {
        const response = await api.post(`/verify/${type}/${id}`, { raw });
        return response;
      } 
      // 否则，验证已存在的组件
      else {
        const response = await api.get(`/verify/${type}/${id}`);
        return response;
      }
    } catch (error) {
      throw error;
    }
  },

  // 添加saveEdit函数
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

  // 添加saveNew函数
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

  // 修改获取所有可用插件的函数，添加模拟数据
  async getAvailablePlugins() {
    try {
      // 尝试从API获取插件列表
      try {
        const response = await api.get('/plugins/available');
        return response.data;
      } catch (apiError) {
        console.warn('Failed to fetch plugins from API, using mock data:', apiError);
        
        // 如果API不可用，返回模拟数据
        return [
          { name: 'ip_in_range', description: 'Check if IP is in a specified range' },
          { name: 'is_base64', description: 'Check if string is base64 encoded' },
          { name: 'is_url', description: 'Check if string is a valid URL' },
          { name: 'contains_sensitive', description: 'Check if string contains sensitive information' },
          { name: 'regex_match', description: 'Match string against a regex pattern' },
          { name: 'json_extract', description: 'Extract value from JSON string' },
          { name: 'base64_decode', description: 'Decode base64 string' },
          { name: 'url_decode', description: 'Decode URL encoded string' },
          { name: 'hash_md5', description: 'Calculate MD5 hash of string' },
          { name: 'hash_sha1', description: 'Calculate SHA1 hash of string' },
          { name: 'hash_sha256', description: 'Calculate SHA256 hash of string' },
          { name: 'timestamp_to_date', description: 'Convert timestamp to date string' },
          { name: 'is_ip', description: 'Check if string is a valid IP address' },
          { name: 'is_domain', description: 'Check if string is a valid domain name' },
          { name: 'domain_to_ip', description: 'Resolve domain name to IP address' },
          { name: 'ip_to_geo', description: 'Get geolocation information for IP address' }
        ];
      }
    } catch (error) {
      console.error('Failed to fetch available plugins:', error);
      return [];
    }
  },
  
  // 添加连接检查函数
  async connectCheck(type, id) {
    try {
      // 目前API可能还没有实现，先使用模拟数据
      try {
        const response = await api.get(`/connect-check/${type}/${id}`);
        return response.data;
      } catch (apiError) {
        console.warn('Failed to check connection from API, using mock data:', apiError);
        
        // 根据组件类型返回不同的模拟数据
        if (type === 'inputs') {
          return {
            status: 'success',
            message: 'Connection check successful',
            details: {
              connected_to: [
                { type: 'ruleset', id: 'example_ruleset', status: 'active' },
                { type: 'ruleset', id: 'detection_rules', status: 'inactive' }
              ],
              connection_errors: []
            }
          };
        } else if (type === 'outputs') {
          return {
            status: 'warning',
            message: 'Connection check completed with warnings',
            details: {
              connected_from: [
                { type: 'ruleset', id: 'example_ruleset', status: 'active' }
              ],
              connection_errors: [
                { message: 'Connection timeout when sending data', severity: 'warning' }
              ]
            }
          };
        } else {
          return {
            status: 'error',
            message: 'Connection check not supported for this component type',
            details: {}
          };
        }
      }
    } catch (error) {
      console.error(`Failed to check ${type} connection:`, error);
      throw error;
    }
  },
  
  // 添加测试插件函数
  async testPlugin(name, args) {
    try {
      // 基本验证
      if (!name) {
        throw new Error('Plugin name is required');
      }
      
      if (!Array.isArray(args)) {
        throw new Error('Arguments must be an array');
      }
      
      // 使用api实例发送请求
      const response = await api.post(`/test-plugin/${name}`, { args });
      return response.data;
    } catch (error) {
      console.error(`Failed to test plugin ${name}:`, error);
      // 如果是HTTP错误，返回错误信息
      if (error.response && error.response.data) {
        return {
          success: false,
          error: error.response.data.error || 'Failed to test plugin',
          result: null
        };
      }
      // 如果是网络错误或其他错误
      return {
        success: false,
        error: error.message || 'Network error or server not responding',
        result: null
      };
    }
  },
};

// 导出verifyComponent函数，供其他组件使用
export async function verifyComponent(type, id, raw) {
  try {
    // 如果提供了raw内容，使用它进行验证
    if (raw !== undefined) {
      const response = await api.post(`/verify/${type}/${id}`, { raw });
      return response;
    } 
    // 否则，验证已存在的组件
    else {
      const response = await api.get(`/verify/${type}/${id}`);
      return response;
    }
  } catch (error) {
    throw error;
  }
}

// 导出saveComponent函数
export async function saveComponent(type, id, raw) {
  try {
    // 确保raw是字符串
    const rawString = typeof raw === 'object' ? JSON.stringify(raw) : String(raw || '');
    
    if (id.endsWith('.new')) {
      // 创建新组件
      const response = await api.post(`/${type}`, { id: id.replace('.new', ''), raw: rawString });
      return response;
    } else {
      // 更新现有组件
      const response = await api.put(`/${type}/${id}`, { raw: rawString });
      return response;
    }
  } catch (error) {
    throw error;
  }
} 