/**
 * Test data caches for specific components
 * These are temporary caches used during testing workflows
 */ 

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