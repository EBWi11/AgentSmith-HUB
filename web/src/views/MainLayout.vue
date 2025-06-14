<template>
  <div class="flex flex-col h-screen bg-white">
    <Header />
    <div class="flex flex-1 overflow-hidden">
      <Sidebar 
        :selected="selected" 
        @select-item="onSelectItem" 
        @open-editor="onOpenEditor" 
        @item-deleted="handleItemDeleted"
        @open-pending-changes="onOpenPendingChanges"
        @test-ruleset="onTestRuleset"
        @test-output="onTestOutput"
        @test-project="onTestProject"
        ref="sidebarRef"
      />
      <main class="flex-1 bg-gray-50">
        <ComponentDetail 
          v-if="selected && selected.type !== 'cluster' && selected.type !== 'pending-changes'" 
          :item="selected" 
          @cancel-edit="handleCancelEdit"
          @updated="handleUpdated"
        />
        <ClusterStatus v-else-if="selected && selected.type === 'cluster'" />
        <PendingChanges v-else-if="selected && selected.type === 'pending-changes'" />
      </main>
    </div>
    
    <!-- Test Ruleset Modal -->
    <RulesetTestModal 
      :show="showTestRulesetModal"
      :rulesetId="testRulesetId"
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
import { ref, onBeforeUnmount } from 'vue'
import Header from '../components/Header.vue'
import Sidebar from '../components/Sidebar/Sidebar.vue'
import ComponentDetail from '../components/ComponentDetail.vue'
import ClusterStatus from '../components/ClusterStatus.vue'
import PendingChanges from '../components/PendingChanges.vue'
import RulesetTestModal from '../components/RulesetTestModal.vue'
import OutputTestModal from '../components/OutputTestModal.vue'
import ProjectTestModal from '../components/ProjectTestModal.vue'

// State
const selected = ref(null)
const sidebarRef = ref(null)
const showTestRulesetModal = ref(false)
const testRulesetId = ref('')
const showTestOutputModal = ref(false)
const testOutputId = ref('')
const showTestProjectModal = ref(false)
const testProjectId = ref('')

// Methods
function onSelectItem(item) {
  selected.value = item
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
    console.error('Failed to open editor:', e);
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
  // Handle post-update logic
  selected.value = {
    ...item,
    isEdit: false
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
}

// Open the pending changes view
function onOpenPendingChanges() {
  selected.value = { type: 'pending-changes' }
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

// 组件卸载前移除事件监听
onBeforeUnmount(() => {
  document.removeEventListener('keydown', handleEscKey);
});

// Open the ruleset test modal
function onTestRuleset(payload) {
  console.log('MainLayout: onTestRuleset called with payload', payload);
  testRulesetId.value = payload.id;
  showTestRulesetModal.value = true;
  console.log('MainLayout: showTestRulesetModal set to', showTestRulesetModal.value);
  
  // 添加ESC键监听
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
  console.log('MainLayout: onTestOutput called with payload', payload);
  testOutputId.value = payload.id;
  showTestOutputModal.value = true;
  console.log('MainLayout: showTestOutputModal set to', showTestOutputModal.value);
  
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
  console.log('MainLayout: onTestProject called with payload', payload);
  testProjectId.value = payload.id;
  showTestProjectModal.value = true;
  console.log('MainLayout: showTestProjectModal set to', showTestProjectModal.value);
  
  // 添加ESC键监听
  document.addEventListener('keydown', handleEscKey);
}

// Close the project test modal
function closeTestProjectModal() {
  showTestProjectModal.value = false;
  
  // 移除ESC键监听
  document.removeEventListener('keydown', handleEscKey);
}
</script> 