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
    const response = await api.get(`/plugin/${id}`);
    return response.data;
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
    const response = await api.put(`/plugin/${id}`, { raw });
    return response.data;
  }
}; 