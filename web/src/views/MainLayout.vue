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
  </div>
</template>

<script setup>
import { ref } from 'vue'
import Header from '../components/Header.vue'
import Sidebar from '../components/Sidebar/Sidebar.vue'
import ComponentDetail from '../components/ComponentDetail.vue'
import ClusterStatus from '../components/ClusterStatus.vue'
import PendingChanges from '../components/PendingChanges.vue'

// State
const selected = ref(null)
const sidebarRef = ref(null)

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
</script> 