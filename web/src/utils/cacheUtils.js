/**
 * Test data caches for specific components
 * These are temporary caches used during testing workflows
 */ 

import { useDataCacheStore } from '../stores/dataCache'
import { 
  saveSidebarState, 
  saveComponentDetailState, 
  saveProjectWorkflowState 
} from './stateManager'

// 延迟缓存清理函数 - 用于项目操作和组件变更后的缓存重置
export function clearCacheWithDelay(delay = 1000, reason = 'operation') {
  const dataCache = useDataCacheStore()
  
  console.log(`[CacheUtils] Saving UI states before cache clear due to: ${reason}`)
  
  // 保存UI状态，防止界面重置
  saveUIStates()
  
  console.log(`[CacheUtils] Immediate cache clear + scheduling delayed clear in ${delay}ms due to: ${reason}`)
  
  // 立即清理一次，确保后续API调用不会使用旧缓存
  dataCache.clearAll()
  
  // 发出缓存清理事件，通知组件立即刷新
  window.dispatchEvent(new CustomEvent('cacheCleared', { 
    detail: { reason, timestamp: Date.now(), shouldRestoreState: true }
  }))
  
  // 延迟再清理一次，确保任何中间缓存也被清理
  setTimeout(() => {
    console.log(`[CacheUtils] Delayed cache clear due to: ${reason}`)
    dataCache.clearAll()
    
    // 再次发出事件，但不需要恢复状态（避免重复）
    window.dispatchEvent(new CustomEvent('cacheCleared', { 
      detail: { reason: `${reason} (delayed)`, timestamp: Date.now(), shouldRestoreState: false }
    }))
  }, delay)
}

// 保存所有组件的UI状态
function saveUIStates() {
  try {
    // 查找并保存 Sidebar 状态
    const sidebarElements = document.querySelectorAll('[data-component="sidebar"]')
    sidebarElements.forEach((element) => {
      // 尝试多种方式访问 Vue 组件实例
      const vueInstance = element.__vueParentComponent || element.__vue__ || element._vnode?.component
      if (vueInstance?.exposed || vueInstance?.setupState) {
        const exposed = vueInstance.exposed || vueInstance.setupState
        saveSidebarState(exposed)
      }
    })

    // 查找并保存 ComponentDetail 状态
    const detailElements = document.querySelectorAll('[data-component="component-detail"]')
    detailElements.forEach((element) => {
      const vueInstance = element.__vueParentComponent || element.__vue__ || element._vnode?.component
      if (vueInstance?.exposed || vueInstance?.setupState) {
        const exposed = vueInstance.exposed || vueInstance.setupState
        saveComponentDetailState(exposed)
      }
    })

    // 查找并保存 ProjectWorkflow 状态
    const workflowElements = document.querySelectorAll('[data-component="project-workflow"]')
    workflowElements.forEach((element) => {
      const vueInstance = element.__vueParentComponent || element.__vue__ || element._vnode?.component
      if (vueInstance?.exposed || vueInstance?.setupState) {
        const exposed = vueInstance.exposed || vueInstance.setupState
        saveProjectWorkflowState(exposed)
      }
    })

    console.log('[CacheUtils] UI states saved successfully')
  } catch (error) {
    console.warn('[CacheUtils] Failed to save UI states:', error)
  }
}

// 立即清理缓存函数
export function clearCacheImmediate(reason = 'operation') {
  const dataCache = useDataCacheStore()
  
  console.log(`[CacheUtils] Clearing all cache immediately due to: ${reason}`)
  dataCache.clearAll()
}

// Ruleset test data cache
const rulesetTestDataCache = new Map();

export const RulesetTestCache = {
  // Get test data for a specific ruleset
  get(rulesetId) {
    return rulesetTestDataCache.get(rulesetId) || null;
  },
  
  // Save test data for a specific ruleset
  set(rulesetId, data) {
    rulesetTestDataCache.set(rulesetId, data);
  },
  
  // Clear test data for a specific ruleset
  clear(rulesetId) {
    rulesetTestDataCache.delete(rulesetId);
  },
  
  // Clear all test data
  clearAll() {
    rulesetTestDataCache.clear();
  }
};

// Project test data cache
const projectTestDataCache = new Map();

export const ProjectTestCache = {
  // Get test data for a specific project
  get(projectId) {
    return projectTestDataCache.get(projectId) || null;
  },
  
  // Save test data for a specific project
  set(projectId, data) {
    projectTestDataCache.set(projectId, data);
  },
  
  // Clear test data for a specific project
  clear(projectId) {
    projectTestDataCache.delete(projectId);
  },
  
  // Clear all test data
  clearAll() {
    projectTestDataCache.clear();
  }
}; 