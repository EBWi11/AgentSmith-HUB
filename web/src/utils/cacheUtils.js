/**
 * Test data caches for specific components
 * These are temporary caches used during testing workflows
 */ 

import { useDataCacheStore } from '../stores/dataCache'

// 延迟缓存清理函数 - 用于项目操作和组件变更后的缓存重置
export function clearCacheWithDelay(delay = 1000, reason = 'operation') {
  const dataCache = useDataCacheStore()
  
  console.log(`[CacheUtils] Scheduling cache clear in ${delay}ms due to: ${reason}`)
  
  setTimeout(() => {
    console.log(`[CacheUtils] Clearing all cache due to: ${reason}`)
    dataCache.clearAll()
  }, delay)
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