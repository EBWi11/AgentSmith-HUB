import { ref, reactive } from 'vue'
import { hubApi } from '../api'

/**
 * 通用API操作composable
 * 用于减少重复的CRUD操作代码
 */
export function useApiOperations() {
  const loading = reactive({
    fetch: false,
    save: false,
    delete: false,
    verify: false,
    connect: false
  })
  
  const error = ref(null)
  
  // 通用获取操作
  const fetchData = async (apiMethod, ...args) => {
    loading.fetch = true
    error.value = null
    try {
      const result = await apiMethod(...args)
      return result
    } catch (e) {
      error.value = e.message || 'Failed to fetch data'
      throw e
    } finally {
      loading.fetch = false
    }
  }
  
  // 通用保存操作
  const saveData = async (apiMethod, ...args) => {
    loading.save = true
    error.value = null
    try {
      const result = await apiMethod(...args)
      return result
    } catch (e) {
      error.value = e.message || 'Failed to save data'
      throw e
    } finally {
      loading.save = false
    }
  }
  
  // 通用删除操作
  const deleteData = async (apiMethod, ...args) => {
    loading.delete = true
    error.value = null
    try {
      const result = await apiMethod(...args)
      return result
    } catch (e) {
      error.value = e.message || 'Failed to delete data'
      throw e
    } finally {
      loading.delete = false
    }
  }
  
  // 通用验证操作
  const verifyData = async (apiMethod, ...args) => {
    loading.verify = true
    error.value = null
    try {
      const result = await apiMethod(...args)
      return result
    } catch (e) {
      error.value = e.message || 'Verification failed'
      throw e
    } finally {
      loading.verify = false
    }
  }
  
  // 通用连接检查操作
  const connectCheck = async (apiMethod, ...args) => {
    loading.connect = true
    error.value = null
    try {
      const result = await apiMethod(...args)
      return result
    } catch (e) {
      error.value = e.message || 'Connection check failed'
      throw e
    } finally {
      loading.connect = false
    }
  }
  
  return {
    loading,
    error,
    fetchData,
    saveData,
    deleteData,
    verifyData,
    connectCheck
  }
}

/**
 * 组件类型操作composable
 * 统一处理不同组件类型的API调用
 */
export function useComponentOperations(componentType) {
  const { loading, error, fetchData, saveData, deleteData, verifyData, connectCheck } = useApiOperations()
  
  // API方法映射
  const apiMethods = {
    inputs: {
      fetch: hubApi.fetchInputs,
      get: hubApi.getInput,
      create: hubApi.createInput,
      update: hubApi.updateInput,
      delete: hubApi.deleteInput,
      verify: (id, content) => hubApi.verifyComponent('inputs', id, content),
      connectCheck: (id, config) => hubApi.connectCheck('inputs', id)
    },
    outputs: {
      fetch: hubApi.fetchOutputs,
      get: hubApi.getOutput,
      create: hubApi.createOutput,
      update: hubApi.updateOutput,
      delete: hubApi.deleteOutput,
      verify: (id, content) => hubApi.verifyComponent('outputs', id, content),
      connectCheck: (id, config) => hubApi.connectCheck('outputs', id)
    },
    rulesets: {
      fetch: hubApi.fetchRulesets,
      get: hubApi.getRuleset,
      create: hubApi.createRuleset,
      update: hubApi.updateRuleset,
      delete: hubApi.deleteRuleset,
      verify: (id, content) => hubApi.verifyComponent('rulesets', id, content)
    },
    plugins: {
      fetch: hubApi.fetchPlugins,
      get: hubApi.getPlugin,
      create: hubApi.createPlugin,
      update: hubApi.updatePlugin,
      delete: hubApi.deletePlugin,
      verify: (id, content) => hubApi.verifyComponent('plugins', id, content)
    },
    projects: {
      fetch: hubApi.fetchProjects,
      get: hubApi.getProject,
      create: hubApi.createProject,
      update: hubApi.updateProject,
      delete: hubApi.deleteProject,
      verify: (id, content) => hubApi.verifyComponent('projects', id, content),
      start: hubApi.startProject,
      stop: hubApi.stopProject,
      restart: hubApi.restartProject
    }
  }
  
  const methods = apiMethods[componentType] || {}
  
  // 通用操作函数
  const fetchComponents = () => fetchData(methods.fetch)
  const getComponent = (id) => fetchData(methods.get, id)
  const createComponent = (id, content) => saveData(methods.create, id, content)
  const updateComponent = (id, content) => saveData(methods.update, id, content)
  const deleteComponent = (id) => deleteData(methods.delete, id)
  const verifyComponent = (id, content) => verifyData(methods.verify, id, content)
  const checkConnection = (id, config) => connectCheck(methods.connectCheck, id, config)
  
  // 项目特有操作
  const startProject = (id) => saveData(methods.start, id)
  const stopProject = (id) => saveData(methods.stop, id)
  const restartProject = (id) => saveData(methods.restart, id)
  
  return {
    loading,
    error,
    fetchComponents,
    getComponent,
    createComponent,
    updateComponent,
    deleteComponent,
    verifyComponent,
    checkConnection,
    startProject,
    stopProject,
    restartProject
  }
} 