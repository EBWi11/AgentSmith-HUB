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