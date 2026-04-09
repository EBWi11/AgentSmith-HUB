<template>
  <div class="flex flex-col h-full min-h-0 overflow-hidden bg-gray-50">
    <Header />
    <div class="flex flex-1 min-h-0 overflow-hidden">
      <Sidebar 
        :selected="selected" 
        :collapsed="sidebarCollapsed"
        @select-item="onSelectItem" 
        @open-editor="onOpenEditor" 
        @item-deleted="handleItemDeleted"
        @open-pending-changes="onOpenPendingChanges"
        @test-ruleset="onTestRuleset"
        @test-output="onTestOutput"
        @test-project="onTestProject"
        @toggle-collapse="toggleSidebarCollapse"
        ref="sidebarRef"
      />
      <main class="main-scroll-region flex-1 min-h-0 bg-gray-50 transition-all duration-300 overflow-hidden overscroll-contain">
        <router-view v-if="!selected || selected.type === 'home'" />
        <ComponentDetail 
          v-else-if="selected && selected.type !== 'cluster' && selected.type !== 'pending-changes' && selected.type !== 'load-local-components' && selected.type !== 'operations-history' && selected.type !== 'error-logs' && selected.type !== 'agent-logs' && selected.type !== 'settings' && selected.type !== 'tutorial'" 
          :item="selected" 
          @cancel-edit="handleCancelEdit"
          @updated="handleUpdated"
          @created="handleCreated"
          ref="componentDetailRef"
        />
        <ClusterStatus v-else-if="selected && selected.type === 'cluster'" />
        <PendingChanges 
          v-else-if="selected && selected.type === 'pending-changes'" 
          @refresh-list="handleRefreshList"
        />
        <LoadLocalComponents 
          v-else-if="selected && selected.type === 'load-local-components'" 
          @refresh-list="handleRefreshList"
        />
        <OperationsHistory v-else-if="selected && selected.type === 'operations-history'" />
        <ErrorLogs v-else-if="selected && selected.type === 'error-logs'" />
        <AgentToolsLogs v-else-if="selected && selected.type === 'agent-logs'" />
        <router-view v-else-if="selected && selected.type === 'tutorial'" />
        <!-- Fallback: render any unmatched child route (e.g., tutorial before selected is set) -->
        <router-view v-else />
      </main>
    </div>
    
    <!-- Test Ruleset / Agent Modal -->
    <RulesetTestModal 
      :show="showTestRulesetModal"
      :rulesetId="testRulesetId"
      :rulesetContent="testRulesetContent"
      :componentType="testComponentType"
      @close="closeTestRulesetModal"
    />
    
    <!-- Test Output Modal -->
    <OutputTestModal 
      :show="showTestOutputModal"
      :outputId="testOutputId"
      @close="closeTestOutputModal"
    />
    
    <!-- Test Project Modal -->
    <ProjectTestModal 
      :show="showTestProjectModal"
      :projectId="testProjectId"
      @close="closeTestProjectModal"
    />
  </div>
</template>

<script setup>
import { ref, onBeforeUnmount, onMounted, watch, defineAsyncComponent } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Header from '../components/Header.vue'
import Sidebar from '../components/Sidebar/Sidebar.vue'
const ComponentDetail = defineAsyncComponent(() => import('../components/ComponentDetail.vue'))
const ClusterStatus = defineAsyncComponent(() => import('../components/ClusterStatus.vue'))
const PendingChanges = defineAsyncComponent(() => import('../components/PendingChanges.vue'))
const LoadLocalComponents = defineAsyncComponent(() => import('../components/LoadLocalComponents.vue'))
const OperationsHistory = defineAsyncComponent(() => import('../components/OperationsHistory.vue'))
const ErrorLogs = defineAsyncComponent(() => import('../views/ErrorLogs.vue'))
const AgentToolsLogs = defineAsyncComponent(() => import('../views/AgentToolsLogs.vue'))
const RulesetTestModal = defineAsyncComponent(() => import('../components/RulesetTestModal.vue'))
const OutputTestModal = defineAsyncComponent(() => import('../components/OutputTestModal.vue'))
const ProjectTestModal = defineAsyncComponent(() => import('../components/ProjectTestModal.vue'))
// Test caches are now integrated into DataCache store
import { useDataCacheStore } from '../stores/dataCache'

// State
const selected = ref(null)
const sidebarRef = ref(null)
const componentDetailRef = ref(null)
const sidebarCollapsed = ref(false)
const showTestRulesetModal = ref(false)
const testRulesetId = ref('')
const testRulesetContent = ref('')
const testComponentType = ref('rulesets')
const showTestOutputModal = ref(false)
const testOutputId = ref('')
const showTestProjectModal = ref(false)
const testProjectId = ref('')

// Get route and router
const route = useRoute()
const router = useRouter()
const dataCache = useDataCacheStore()

// Helper: update global CSS variable for sidebar width
function updateSidebarWidth () {
  const width = sidebarCollapsed.value ? 64 : 288 // keep in sync with Sidebar.vue
  document.documentElement.style.setProperty('--sidebar-width', width + 'px')
}

// Handle route changes
onMounted(() => {
  updateSidebarWidth()
  // Check if we have component type in the route
  const { params, meta } = route
  if (meta.componentType) {
    if (meta.componentType === 'home') {
      // For home page, show dashboard
      selected.value = {
        type: 'home',
        _timestamp: Date.now()
      }
    } else if (meta.componentType === 'cluster' || meta.componentType === 'pending-changes' || meta.componentType === 'load-local-components' || meta.componentType === 'operations-history' || meta.componentType === 'error-logs' || meta.componentType === 'agent-logs' || meta.componentType === 'tutorial') {
      // For cluster, pending-changes, load-local-components, operations-history, error-logs, agent-logs, no ID needed
      selected.value = {
        type: meta.componentType,
        _timestamp: Date.now()
      }
    } else if (params.id) {
      // For regular components, need ID
      selected.value = {
        type: meta.componentType,
        id: params.id,
        isEdit: false,
        _timestamp: Date.now()
      }
    }
  }
})

// Watch for route changes
watch(
  () => [route.params, route.meta],
  ([newParams, newMeta], [oldParams, oldMeta]) => {
    const { id } = newParams
    const componentType = newMeta.componentType
    const oldId = oldParams?.id
    const oldComponentType = oldMeta?.componentType
    
    // Test cache has TTL and will expire automatically when switching components
    
    if (componentType) {
      if (componentType === 'home') {
        // For home page, show dashboard
        if (!selected.value || selected.value.type !== componentType) {
          selected.value = {
            type: 'home',
            _timestamp: Date.now()
          }
        }
      } else if (componentType === 'cluster' || componentType === 'pending-changes' || componentType === 'load-local-components' || componentType === 'operations-history' || componentType === 'error-logs' || componentType === 'agent-logs' || componentType === 'tutorial') {
        // For cluster, pending-changes, load-local-components, operations-history, error-logs, agent-logs, no ID needed
        if (!selected.value || selected.value.type !== componentType) {
          selected.value = {
            type: componentType,
            _timestamp: Date.now()
          }
        }
      } else if (id && (!selected.value || selected.value.id !== id || selected.value.type !== componentType)) {
        // For regular components, need ID
        selected.value = {
          type: componentType,
          id,
          isEdit: false,
          _timestamp: Date.now()
        }
      }
    }
  }
)

// Update URL when selected component changes
watch(
  () => selected.value,
  (newVal) => {
    if (newVal && newVal.type && !newVal.isNew) {
      const currentPath = router.currentRoute.value.path
      let expectedPath
      
      if (newVal.type === 'home') {
        // For home page, use base app path
        expectedPath = '/app'
      } else if (newVal.type === 'cluster' || newVal.type === 'pending-changes' || newVal.type === 'load-local-components' || newVal.type === 'operations-history' || newVal.type === 'error-logs' || newVal.type === 'agent-logs') {
        // For cluster, pending-changes, load-local-components, operations-history, error-logs, agent-logs, no ID in URL
        expectedPath = `/app/${newVal.type}`
      } else if (newVal.id) {
        // For regular components, include ID in URL
        expectedPath = `/app/${newVal.type}/${newVal.id}`
      }
      
      if (expectedPath && currentPath !== expectedPath) {
        router.push(expectedPath)
      }
    }
  },
  { deep: true }
)

// Watch for sidebar collapsed state
watch(sidebarCollapsed, () => {
  updateSidebarWidth()
})

// Methods
function onSelectItem(item) {
  // Use router navigation for better URL management
  if (item.type === 'home') {
    router.push('/app')
  } else if (item.type === 'cluster') {
    router.push('/app/cluster')
  } else if (item.type === 'pending-changes') {
    router.push('/app/pending-changes')
  } else if (item.type === 'load-local-components') {
    router.push('/app/load-local-components')
  } else if (item.type === 'operations-history') {
    router.push('/app/operations-history')
  } else if (item.type === 'error-logs') {
    router.push('/app/error-logs')
  } else if (item.type === 'agent-logs') {
    router.push('/app/agent-logs')
  } else if (item.id) {
    router.push(`/app/${item.type}/${item.id}`)
  } else {
    // Fallback for direct selection (e.g., new components)
    selected.value = item
  }
}

async function onOpenEditor(payload) {
  try {
    // If in edit mode and not a new component, create a temporary file first
    if (payload.isEdit && !payload.isNew) {
      // We shouldn't use createTempFile here, as this API would submit changes directly
      // We should first get the component content, then open it in the editor
      // Let the user submit changes only when they click the save button
      
      // Set edit state first, let the component detail page handle getting content
      selected.value = payload;
    } else {
      // For new components, set directly
      selected.value = payload;
    }
  } catch (e) {
    // Only log error, don't show notification
  }
}

function handleCancelEdit(item) {
  // Exit edit mode, return to view mode
  selected.value = {
    ...item,
    isEdit: false
  }
}

function handleUpdated(item) {
  // Check if we should exit to view mode after save
  if (item.exitToViewMode) {
    // Exit to view mode
    selected.value = {
      type: item.type,
      id: item.id,
      isEdit: false,
      // Add timestamp to trigger data refresh
      _timestamp: Date.now()
    }
  } else {
    // Keep edit mode, don't switch to view mode
    selected.value = {
      type: item.type,
      id: item.id,
      isEdit: true,
      // Add timestamp to trigger data refresh
      _timestamp: Date.now()
    }
  }
  
  // Refresh sidebar list
  refreshSidebar(item.type)
}

// Handle component creation completed event
function handleCreated(item) {
  // Check if we should exit to view mode after save
  if (item.exitToViewMode) {
    // Exit to view mode
    selected.value = {
      type: item.type,
      id: item.id,
      isEdit: false,
      // Add timestamp to trigger data refresh
      _timestamp: Date.now()
    }
  } else {
    // Keep edit mode for newly created components
    selected.value = {
      type: item.type,
      id: item.id,
      isEdit: true,
      // Add timestamp to trigger data refresh
      _timestamp: Date.now()
    }
  }
  
  // Refresh sidebar list
  refreshSidebar(item.type)
}

// Handle delete event
function handleItemDeleted({ type, id }) {
  // If the currently selected item is the one being deleted, clear the selection
  if (selected.value && selected.value.id === id && selected.value.type === type) {
    selected.value = null
  }
  
  // Refresh sidebar list
  refreshSidebar(type)
}

// Refresh a specific type of list in the sidebar
function refreshSidebar(type) {
  if (sidebarRef.value && typeof sidebarRef.value.fetchItems === 'function') {
    sidebarRef.value.fetchItems(type)
  }
  
  // HIGHEST PRIORITY: Clear cache and force immediate refresh
  const dataCache = useDataCacheStore()
  dataCache.clearComponentCache(type)
  setTimeout(() => {
    dataCache.fetchComponents(type, true, true) // isPriorityRefresh = true
  }, 150)
}

// Open the pending changes view
function onOpenPendingChanges() {
  router.push('/app/pending-changes')
}

// 处理ESC键按下
function handleEscKey(event) {
  if (event.key === 'Escape') {
    if (showTestRulesetModal.value) {
      closeTestRulesetModal();
    }
    if (showTestOutputModal.value) {
      closeTestOutputModal();
    }
    if (showTestProjectModal.value) {
      closeTestProjectModal();
    }
  }
}

onBeforeUnmount(() => {
  document.removeEventListener('keydown', handleEscKey);
  
  // Test caches have TTL and will expire automatically
});

// Open the ruleset test modal
function onTestRuleset(payload) {
  testRulesetId.value = payload.id;
  testComponentType.value = payload.type || 'rulesets';

  // Sidebar menu can pass temp content directly; editor content still wins if open.
  testRulesetContent.value = payload.content || '';
  if (selected.value && 
      selected.value.type === 'rulesets' && 
      selected.value.id === payload.id && 
      componentDetailRef.value) {
    // Get editor content from ComponentDetail
    const editorContent = componentDetailRef.value.getEditorContent?.();
    if (editorContent !== undefined) {
      testRulesetContent.value = editorContent;
    }
  }
  
  showTestRulesetModal.value = true;
  document.addEventListener('keydown', handleEscKey);
}

// Close the ruleset test modal
function closeTestRulesetModal() {
  showTestRulesetModal.value = false;
  
  // 移除ESC键监听
  document.removeEventListener('keydown', handleEscKey);
}

// Open the output test modal
function onTestOutput(payload) {
  testOutputId.value = payload.id;
  showTestOutputModal.value = true;

  // 添加ESC键监听
  document.addEventListener('keydown', handleEscKey);
}

// Close the output test modal
function closeTestOutputModal() {
  showTestOutputModal.value = false;
  
  // 移除ESC键监听
  document.removeEventListener('keydown', handleEscKey);
}

// Open the project test modal
function onTestProject(payload) {
  testProjectId.value = payload.id;
  showTestProjectModal.value = true;

  // 添加ESC键监听
  document.addEventListener('keydown', handleEscKey);
}

// Close the project test modal
function closeTestProjectModal() {
  showTestProjectModal.value = false;
  
  // 移除ESC键监听
  document.removeEventListener('keydown', handleEscKey);
}

function handleRefreshList(type) {
  // Refresh component list of specified type
  refreshSidebar(type)
}

// Toggle sidebar collapse
function toggleSidebarCollapse() {
  sidebarCollapsed.value = !sidebarCollapsed.value
}
</script> 

<style scoped>
.main-scroll-region {
  /* Keep scrollbar space stable to reduce layout jitter */
  scrollbar-gutter: stable;
  /* Smoother momentum scrolling on WebKit touch devices */
  -webkit-overflow-scrolling: touch;
}
</style>