<template>
  <aside class="w-72 h-full bg-white shadow-sm flex flex-col px-3 pt-5 pb-3 font-sans">
    <div class="mb-4">
      <div class="relative">
        <input
          type="text"
          placeholder="Search"
          v-model="search"
          class="w-full pl-7 pr-3 py-1.5 rounded-lg bg-gray-50 border border-gray-100 text-sm focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary transition"
        />
        <svg class="absolute left-2.5 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-300" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"></path></svg>
      </div>
    </div>
    <div class="flex-1 overflow-y-auto custom-scrollbar">
      <div v-for="(section, type) in sections" :key="type" class="mb-4">
        <div class="flex items-center justify-between mb-1.5">
          <button
            @click="toggleCollapse(type)"
            class="flex items-center text-[13px] font-bold text-gray-900 tracking-wide uppercase focus:outline-none group"
            style="min-width:0;"
          >
            <svg
              class="w-4 h-4 mr-1.5 transition-transform duration-200"
              :class="{ 'rotate-90': !collapsed[type] }"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/>
            </svg>
            <span class="truncate">{{ section.title }}</span>
          </button>
          <button v-if="!section.children" @click="openAddModal(type)" class="rounded-full p-0.5 hover:bg-primary/10 text-primary transition flex items-center justify-center">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"></path></svg>
          </button>
        </div>
        <div v-if="!collapsed[type]" class="space-y-0.5">
          <div v-if="section.children">
            <!-- Add Push button (only shown in Settings section) -->
            <div v-if="type === 'settings'" class="flex items-center px-1.5 py-1 rounded-md group cursor-pointer transition-all hover:bg-gray-100 mb-1"
                 @click="openPendingChanges">
              <svg class="w-4 h-4 mr-1.5 text-gray-400 group-hover:text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16a4 4 0 01-.88-7.903A5 5 0 1115.9 6L16 6a5 5 0 011 9.9M15 13l-3-3m0 0l-3 3m3-3v12"></path>
              </svg>
              <span class="flex-1 truncate">Push Changes</span>
            </div>
            
            <div v-for="child in section.children" :key="child.type"
                 class="flex items-center px-1.5 py-1 rounded-md group cursor-pointer transition-all hover:bg-gray-100"
                 :class="{ 'bg-primary/10 text-primary font-semibold border-l-2 border-primary': selected && selected.type === child.type, 'text-gray-800': !(selected && selected.type === child.type) }"
                 @click="$emit('select-item', { type: child.type })">
              <svg v-html="child.icon" class="w-4 h-4 mr-1.5 text-gray-400 group-hover:text-primary"></svg>
              <span class="flex-1 truncate">{{ child.title }}</span>
            </div>
          </div>
          <div v-else-if="!loading[type] && !error[type]">
            <div v-for="item in filteredItems(type)" :key="item.id || item.name"
                 :class="['flex items-center px-1.5 py-1 rounded-md group cursor-pointer transition-all', selected && selected.type === type && selected.id === (item.id || item.name) ? 'bg-primary/10 text-primary font-semibold border-l-2 border-primary' : 'text-gray-800 hover:bg-gray-100']"
                 @click="handleItemClick(type, item)">
              <div v-if="type === 'projects'" class="relative mr-1.5">
                <div class="w-2 h-2 rounded-full"
                     :class="{
                       'bg-green-500 animate-pulse': item.status === 'running',
                       'bg-red-500': item.status === 'stopped',
                       'bg-yellow-500': item.status === 'error'
                     }">
                </div>
              </div>
              <svg v-html="section.icon" class="w-4 h-4 mr-1.5 text-gray-400 group-hover:text-primary"></svg>
              <span class="flex-1 truncate">{{ item.id || item.name }}</span>
              <div class="relative ml-0.5 flex items-center" @click.stop>
                <button @click="item.menuOpen = !item.menuOpen"
                  class="p-0.5 rounded-full focus:outline-none opacity-0 group-hover:opacity-100 transition-opacity duration-150 text-gray-300 hover:text-gray-500 flex items-center justify-center"
                  style="transform: scale(0.7); transform-origin: right;">
                  <svg width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
                    <circle cx="12" cy="5" r="1.5"/>
                    <circle cx="12" cy="12" r="1.5"/>
                    <circle cx="12" cy="19" r="1.5"/>
                  </svg>
                </button>
                <div v-if="item.menuOpen" class="absolute right-0 z-50 mt-1 w-32 bg-white border border-gray-200 rounded shadow-lg py-1 select-none"
                  @mouseleave="item.menuOpen = false">
                  <div class="px-3 py-1 text-xs hover:bg-gray-100 cursor-pointer" @click.stop="copyName(item)">Copy name</div>
                  <div class="px-3 py-1 text-xs hover:bg-gray-100 cursor-pointer" @click.stop="$emit('open-editor', { type, id: item.id || item.name, isEdit: true })">Edit</div>
                  <div v-if="type === 'inputs' || type === 'outputs'" class="px-3 py-1 text-xs hover:bg-gray-100 cursor-pointer" @click.stop="checkConnection(type, item)">Connect check</div>
                  <div v-if="type === 'plugins'" class="px-3 py-1 text-xs hover:bg-gray-100 cursor-pointer" @click.stop="openTestPlugin(item)">Test plugin</div>
                  <div v-if="type === 'rulesets'" class="px-3 py-1 text-xs hover:bg-gray-100 cursor-pointer" @click.stop="openTestRuleset(item)">Test ruleset</div>
                  <div v-if="type === 'outputs'" class="px-3 py-1 text-xs hover:bg-gray-100 cursor-pointer" @click.stop="openTestOutput(item)">Test output</div>
                  <div v-if="type === 'projects'" class="px-3 py-1 text-xs hover:bg-gray-100 cursor-pointer" @click.stop="openTestProject(item)">Test project</div>
                  <div class="border-t border-gray-100 my-1"></div>
                  <div class="px-3 py-1 text-xs text-red-600 hover:bg-red-50 cursor-pointer" @click.stop="deleteItem(type, item)">Delete</div>
                </div>
              </div>
            </div>
          </div>
          <div v-if="loading[type]" class="py-1 text-center text-gray-400">
            <div class="animate-spin rounded-full h-4 w-4 border-b-2 border-gray-900 mx-auto"></div>
          </div>
          <div v-else-if="error[type]" class="text-red-500 text-xs py-1">
            {{ error[type] }}
          </div>
        </div>
      </div>
    </div>

    <!-- Create New Modal -->
    <div v-if="showAddModal" class="fixed inset-0 bg-black bg-opacity-30 flex items-center justify-center z-50">
      <div class="bg-white rounded-lg shadow-xl w-96 p-6">
        <h3 class="text-lg font-medium text-gray-900 mb-4">Add {{ addType ? addType.slice(0, -1) : 'Component' }}</h3>
        <div class="mb-4">
          <label class="block text-sm font-medium text-gray-700 mb-1">Name</label>
          <input type="text" v-model="addName" class="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-1 focus:ring-blue-500" placeholder="Enter name" />
        </div>
        <div class="flex justify-end space-x-3">
          <button @click="closeAddModal" class="px-3 py-1 text-sm text-gray-500">Cancel</button>
          <button @click="confirmAddName" class="px-3 py-1 bg-blue-500 text-white text-sm rounded">Create</button>
        </div>
        <div v-if="addError" class="mt-3 text-sm text-red-500">{{ addError }}</div>
      </div>
    </div>

    <!-- Connection Modal -->
    <div v-if="showConnectionModal" class="fixed inset-0 bg-black bg-opacity-30 flex items-center justify-center z-50">
      <div class="bg-white rounded shadow-lg p-6 w-96 max-h-[80vh] overflow-y-auto">
        <div class="flex justify-between items-center mb-4">
          <h3 class="font-bold">Client Connection Status</h3>
          <button @click="closeConnectionModal" class="text-gray-400 hover:text-gray-600">
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
            </svg>
          </button>
        </div>
        
        <div v-if="connectionLoading" class="flex justify-center items-center py-8">
          <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
        </div>
        
        <div v-else-if="connectionError" class="bg-red-50 border-l-4 border-red-500 p-4 mb-4">
          <div class="flex">
            <div class="flex-shrink-0">
              <svg class="h-5 w-5 text-red-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
              </svg>
            </div>
            <div class="ml-3">
              <p class="text-sm text-red-700">{{ connectionError }}</p>
            </div>
          </div>
        </div>
        
        <div v-else-if="connectionResult">
          <!-- Status Badge -->
          <div class="mb-4 flex items-center">
            <div class="px-2.5 py-0.5 rounded-full text-xs font-medium"
                 :class="{
                   'bg-green-100 text-green-800': connectionResult.status === 'success',
                   'bg-yellow-100 text-yellow-800': connectionResult.status === 'warning',
                   'bg-red-100 text-red-800': connectionResult.status === 'error'
                 }">
              {{ connectionResult.status }}
            </div>
            <span class="ml-2 text-sm text-gray-600">{{ connectionResult.message }}</span>
          </div>
          
          <!-- Client Type -->
          <div v-if="connectionResult.details.client_type" class="mb-4">
            <h4 class="text-sm font-medium text-gray-700 mb-2">Client Type:</h4>
            <div class="p-2 bg-gray-50 rounded-md text-sm">
              {{ connectionResult.details.client_type }}
            </div>
          </div>
          
          <!-- Connection Status -->
          <div v-if="connectionResult.details.connection_status" class="mb-4">
            <h4 class="text-sm font-medium text-gray-700 mb-2">Connection Status:</h4>
            <div class="flex items-center p-2 border rounded-md"
                 :class="{
                   'border-green-200 bg-green-50': ['active', 'connected', 'always_connected'].includes(connectionResult.details.connection_status),
                   'border-yellow-200 bg-yellow-50': connectionResult.details.connection_status === 'idle',
                   'border-red-200 bg-red-50': ['not_configured', 'unsupported'].includes(connectionResult.details.connection_status)
                 }">
              <span class="w-2 h-2 rounded-full mr-2" 
                    :class="{
                      'bg-green-500': ['active', 'connected', 'always_connected'].includes(connectionResult.details.connection_status),
                      'bg-yellow-500': connectionResult.details.connection_status === 'idle',
                      'bg-red-500': ['not_configured', 'unsupported'].includes(connectionResult.details.connection_status),
                      'bg-gray-400': connectionResult.details.connection_status === 'unknown'
                    }"></span>
              <span class="text-sm">{{ connectionResult.details.connection_status }}</span>
            </div>
          </div>
          
          <!-- Connection Info -->
          <div v-if="connectionResult.details.connection_info && Object.keys(connectionResult.details.connection_info).length > 0" class="mb-4">
            <h4 class="text-sm font-medium text-gray-700 mb-2">Connection Info:</h4>
            <div class="p-3 bg-gray-50 rounded-md text-sm overflow-x-auto">
              <div v-for="(value, key) in connectionResult.details.connection_info" :key="key" class="mb-1 flex">
                <span class="font-medium text-gray-600 mr-2">{{ key }}:</span>
                <span v-if="Array.isArray(value)" class="text-gray-800">{{ value.join(', ') }}</span>
                <span v-else class="text-gray-800">{{ value }}</span>
              </div>
            </div>
          </div>
          
          <!-- Metrics -->
          <div v-if="connectionResult.details.metrics" class="mb-4">
            <h4 class="text-sm font-medium text-gray-700 mb-2">Metrics:</h4>
            <div class="p-3 bg-gray-50 rounded-md text-sm">
              <div v-for="(value, key) in connectionResult.details.metrics" :key="key" class="mb-1 flex">
                <span class="font-medium text-gray-600 mr-2">{{ key }}:</span>
                <span class="text-gray-800">{{ value }}</span>
              </div>
            </div>
          </div>
          
          <!-- Connection Errors -->
          <div v-if="connectionResult.details.connection_errors && connectionResult.details.connection_errors.length > 0" class="mb-4">
            <h4 class="text-sm font-medium text-gray-700 mb-2">Connection Issues:</h4>
            <ul class="space-y-2">
              <li v-for="(error, index) in connectionResult.details.connection_errors" :key="index" 
                  class="p-2 border rounded-md"
                  :class="{
                    'border-red-200 bg-red-50': error.severity === 'error',
                    'border-yellow-200 bg-yellow-50': error.severity === 'warning',
                    'border-blue-200 bg-blue-50': error.severity === 'info'
                  }">
                <div class="flex items-center">
                  <span class="w-2 h-2 rounded-full mr-2" 
                        :class="{
                          'bg-red-500': error.severity === 'error',
                          'bg-yellow-500': error.severity === 'warning',
                          'bg-blue-500': error.severity === 'info'
                        }"></span>
                  <span class="text-xs text-gray-500">{{ error.severity }}</span>
                </div>
                <p class="text-sm mt-1">{{ error.message }}</p>
              </li>
            </ul>
          </div>
          
          <!-- No Connection Info -->
          <div v-if="!connectionResult.details.client_type && 
                    !connectionResult.details.connection_status && 
                    (!connectionResult.details.connection_info || Object.keys(connectionResult.details.connection_info).length === 0)"
               class="text-center py-4 text-gray-500">
            No connection information available
          </div>
        </div>
        
        <div class="flex justify-end mt-4">
          <button @click="closeConnectionModal" class="px-4 py-2 bg-gray-100 hover:bg-gray-200 rounded text-sm transition">Close</button>
        </div>
      </div>
    </div>

    <!-- 改进测试插件弹窗 -->
    <div v-if="showTestPluginModal" class="fixed inset-0 bg-black bg-opacity-30 flex items-center justify-center z-50">
      <div class="bg-white rounded shadow-lg p-6 w-[500px] max-h-[80vh] overflow-y-auto">
        <div class="flex justify-between items-center mb-4">
          <h3 class="font-bold">Test Plugin: {{ testPluginName }}</h3>
          <button @click="closeTestPluginModal" class="text-gray-400 hover:text-gray-600">
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
            </svg>
          </button>
        </div>
        
        <!-- Plugin Arguments -->
        <div class="mb-4">
          <h4 class="text-sm font-medium text-gray-700 mb-2">Arguments:</h4>
          <div class="space-y-2">
            <div v-for="(arg, index) in testPluginArgs" :key="index" class="flex items-center space-x-2">
              <div class="flex-1">
                <input 
                  v-model="arg.value" 
                  :placeholder="`Argument ${index + 1} (string)`"
                  class="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
                />
                <div class="text-xs text-gray-500 mt-0.5">
                  {{ getArgumentTypeHint(testPluginName, index) }}
                </div>
              </div>
              <button @click="removePluginArg(index)" class="p-1 rounded-full bg-red-50 text-red-500 hover:bg-red-100">
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"></path>
                </svg>
              </button>
            </div>
          </div>
          <button @click="addPluginArg" class="mt-2 flex items-center text-sm text-primary hover:text-primary-dark">
            <svg class="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"></path>
            </svg>
            Add Argument
          </button>
        </div>
        
        <!-- Test Button -->
        <div class="mb-4">
          <button 
            @click="testPlugin" 
            class="w-full py-2 bg-primary text-white rounded-md hover:bg-primary-dark transition-colors flex items-center justify-center"
            :disabled="testPluginLoading"
          >
            <span v-if="!testPluginLoading">Test Plugin</span>
            <div v-else class="animate-spin rounded-full h-4 w-4 border-2 border-white"></div>
          </button>
        </div>
        
        <!-- Test Results -->
        <div v-if="testPluginResult !== null" class="mb-4">
          <h4 class="text-sm font-medium text-gray-700 mb-2">Result:</h4>
          <div class="p-3 rounded-md overflow-x-auto" :class="testPluginResult.success ? 'bg-green-50 border border-green-100' : 'bg-red-50 border border-red-100'">
            <div v-if="testPluginError" class="text-red-600 text-sm mb-2 font-medium">
              Error: {{ testPluginError }}
            </div>
            <div class="text-sm">
              <div v-if="testPluginResult.result !== null" class="mt-2">
                <div class="font-medium text-gray-700">Result value:</div>
                <pre class="whitespace-pre-wrap mt-1 text-gray-800">{{ JSON.stringify(testPluginResult.result, null, 2) }}</pre>
              </div>
              <div v-else class="text-gray-500 italic">
                No result value returned
              </div>
            </div>
          </div>
        </div>
        
        <div class="flex justify-end mt-4">
          <button @click="closeTestPluginModal" class="px-4 py-2 bg-gray-100 hover:bg-gray-200 rounded text-sm transition">Close</button>
        </div>
      </div>
    </div>

    <!-- Delete Confirmation Modal -->
    <div v-if="showDeleteModal" class="fixed inset-0 bg-black bg-opacity-30 flex items-center justify-center z-50">
      <div class="bg-white rounded-lg shadow-xl w-96 p-6">
        <div class="flex justify-between items-center mb-4">
          <h3 class="text-lg font-medium text-gray-900">Confirm Delete</h3>
          <button @click="closeDeleteModal" class="text-gray-400 hover:text-gray-600">
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
            </svg>
          </button>
        </div>
        
        <div class="mb-6">
          <p class="text-sm text-gray-600 mb-2">
            You are about to delete <span class="font-semibold">{{ itemToDelete?.item?.id || itemToDelete?.item?.name }}</span>.
            This action cannot be undone.
          </p>
          <p class="text-sm text-gray-600 mb-4">
            Type <span class="font-bold text-red-600">delete</span> to confirm.
          </p>
          
          <input 
            type="text" 
            v-model="deleteConfirmText" 
            class="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-1 focus:ring-red-500" 
            placeholder="Type 'delete' to confirm"
            @keyup.enter="confirmDelete"
          />
        </div>
        
        <div v-if="deleteError" class="mb-4 text-sm text-red-600">{{ deleteError }}</div>
        
        <div class="flex justify-end space-x-3">
          <button 
            @click="closeDeleteModal" 
            class="px-3 py-1.5 border border-gray-300 text-gray-700 text-sm rounded hover:bg-gray-50 transition-colors"
          >
            Cancel
          </button>
          <button 
            @click="confirmDelete" 
            class="px-3 py-1.5 bg-red-600 text-white text-sm rounded hover:bg-red-700 transition-colors"
            :disabled="deleteConfirmText !== 'delete'"
          >
            Delete
          </button>
        </div>
      </div>
    </div>
  </aside>
</template>

<script setup>
import { ref, reactive, onMounted, onBeforeUnmount, inject } from 'vue'
import { hubApi } from '@/api'

// Props
const props = defineProps({
  selected: Object
})

// Emits
const emit = defineEmits([
  'select-item',
  'open-editor',
  'item-deleted',
  'open-pending-changes',
  'test-ruleset',
  'test-output',
  'test-project'
])

// Global message component
const $message = inject('$message', window?.$toast)

// Reactive state
const search = ref('')
const items = reactive({
  inputs: [],
  outputs: [],
  rulesets: [],
  plugins: [],
  projects: [],
  cluster: []
})
const loading = reactive({
  inputs: false,
  outputs: false,
  rulesets: false,
  plugins: false,
  projects: false,
  cluster: false
})
const error = reactive({
  inputs: null,
  outputs: null,
  rulesets: null,
  plugins: null,
  projects: null,
  cluster: null
})
const collapsed = reactive({
  inputs: true,
  outputs: true,
  rulesets: true,
  plugins: true,
  projects: true,
  cluster: true,
  settings: true
})

const sections = reactive({
  inputs: { 
    title: 'Input', 
    icon: '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4.5 1C4.22386 1 4 1.22386 4 1.5C4 1.77614 4.22386 2 4.5 2H12V13H4.5C4.22386 13 4 13.2239 4 13.5C4 13.7761 4.22386 14 4.5 14H12C12.5523 14 13 13.5523 13 13V2C13 1.44772 12.5523 1 12 1H4.5ZM6.60355 4.89645C6.40829 4.70118 6.09171 4.70118 5.89645 4.89645C5.70118 5.09171 5.70118 5.40829 5.89645 5.60355L7.29289 7H0.5C0.223858 7 0 7.22386 0 7.5C0 7.77614 0.223858 8 0.5 8H7.29289L5.89645 9.39645C5.70118 9.59171 5.70118 9.90829 5.89645 10.1036C6.09171 10.2988 6.40829 10.2988 6.60355 10.1036L8.85355 7.85355C9.04882 7.65829 9.04882 7.34171 8.85355 7.14645L6.60355 4.89645Z"></path>' 
  },
  outputs: { 
    title: 'Output', 
    icon: '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 1C2.44771 1 2 1.44772 2 2V13C2 13.5523 2.44772 14 3 14H10.5C10.7761 14 11 13.7761 11 13.5C11 13.2239 10.7761 13 10.5 13H3V2L10.5 2C10.7761 2 11 1.77614 11 1.5C11 1.22386 10.7761 1 10.5 1H3ZM12.6036 4.89645C12.4083 4.70118 12.0917 4.70118 11.8964 4.89645C11.7012 5.09171 11.7012 5.40829 11.8964 5.60355L13.2929 7H6.5C6.22386 7 6 7.22386 6 7.5C6 7.77614 6.22386 8 6.5 8H13.2929L11.8964 9.39645C11.7012 9.59171 11.7012 9.90829 11.8964 10.1036C12.0917 10.2988 12.4083 10.2988 12.6036 10.1036L14.8536 7.85355C15.0488 7.65829 15.0488 7.34171 14.8536 7.14645L12.6036 4.89645Z"></path>' 
  },
  rulesets: { 
    title: 'Ruleset', 
    icon: '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4.2 1H4.17741H4.1774C3.86936 0.999988 3.60368 0.999978 3.38609 1.02067C3.15576 1.04257 2.92825 1.09113 2.71625 1.22104C2.51442 1.34472 2.34473 1.51442 2.22104 1.71625C2.09113 1.92825 2.04257 2.15576 2.02067 2.38609C1.99998 2.60367 1.99999 2.86935 2 3.17738V3.1774V3.2V11.8V11.8226V11.8226C1.99999 12.1307 1.99998 12.3963 2.02067 12.6139C2.04257 12.8442 2.09113 13.0717 2.22104 13.2837C2.34473 13.4856 2.51442 13.6553 2.71625 13.779C2.92825 13.9089 3.15576 13.9574 3.38609 13.9793C3.60368 14 3.86937 14 4.17741 14H4.2H10.8H10.8226C11.1306 14 11.3963 14 11.6139 13.9793C11.8442 13.9574 12.0717 13.9089 12.2837 13.779C12.4856 13.6553 12.6553 13.4856 12.779 13.2837C12.9089 13.0717 12.9574 12.8442 12.9793 12.6139C13 12.3963 13 12.1306 13 11.8226V11.8V3.2V3.17741C13 2.86936 13 2.60368 12.9793 2.38609C12.9574 2.15576 12.9089 1.92825 12.779 1.71625C12.6553 1.51442 12.4856 1.34472 12.2837 1.22104C12.0717 1.09113 11.8442 1.04257 11.6139 1.02067C11.3963 0.999978 11.1306 0.999988 10.8226 1H10.8H4.2ZM3.23875 2.07368C3.26722 2.05623 3.32362 2.03112 3.48075 2.01618C3.64532 2.00053 3.86298 2 4.2 2H10.8C11.137 2 11.3547 2.00053 11.5193 2.01618C11.6764 2.03112 11.7328 2.05623 11.7613 2.07368C11.8285 2.11491 11.8851 2.17147 11.9263 2.23875C11.9438 2.26722 11.9689 2.32362 11.9838 2.48075C11.9995 2.64532 12 2.86298 12 3.2V11.8C12 12.137 11.9995 12.3547 11.9838 12.5193C11.9689 12.6764 11.9438 12.7328 11.9263 12.7613C11.8851 12.8285 11.8285 12.8851 11.7613 12.9263C11.7328 12.9438 11.6764 12.9689 11.5193 12.9838C11.3547 12.9995 11.137 13 10.8 13H4.2C3.86298 13 3.64532 12.9995 3.48075 12.9838C3.32362 12.9689 3.26722 12.9438 3.23875 12.9263C3.17147 12.8851 3.11491 12.8285 3.07368 12.7613C3.05624 12.7328 3.03112 12.6764 3.01618 12.5193C3.00053 12.3547 3 12.137 3 11.8V3.2C3 2.86298 3.00053 2.64532 3.01618 2.48075C3.03112 2.32362 3.05624 2.26722 3.07368 2.23875C3.11491 2.17147 3.17147 2.11491 3.23875 2.07368ZM5 10C4.72386 10 4.5 10.2239 4.5 10.5C4.5 10.7761 4.72386 11 5 11H8C8.27614 11 8.5 10.7761 8.5 10.5C8.5 10.2239 8.27614 10 8 10H5ZM4.5 7.5C4.5 7.22386 4.72386 7 5 7H10C10.2761 7 10.5 7.22386 10.5 7.5C10.5 7.77614 10.2761 8 10 8H5C4.72386 8 4.5 7.77614 4.5 7.5ZM5 4C4.72386 4 4.5 4.22386 4.5 4.5C4.5 4.77614 4.72386 5 5 5H10C10.2761 5 10.5 4.77614 10.5 4.5C10.5 4.22386 10.2761 4 10 4H5Z"></path>' 
  },
  plugins: { 
    title: 'Plugin', 
    icon: '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.14921 3.99996C2.14921 2.97778 2.97784 2.14915 4.00002 2.14915C5.02219 2.14915 5.85083 2.97778 5.85083 3.99996C5.85083 5.02213 5.02219 5.85077 4.00002 5.85077C2.97784 5.85077 2.14921 5.02213 2.14921 3.99996ZM4.00002 1.24915C2.48079 1.24915 1.24921 2.48073 1.24921 3.99996C1.24921 5.51919 2.48079 6.75077 4.00002 6.75077C5.51925 6.75077 6.75083 5.51919 6.75083 3.99996C6.75083 2.48073 5.51925 1.24915 4.00002 1.24915ZM5.82034 11.0001L2.49998 12.8369V9.16331L5.82034 11.0001ZM2.63883 8.21159C2.17228 7.9535 1.59998 8.29093 1.59998 8.82411V13.1761C1.59998 13.7093 2.17228 14.0467 2.63883 13.7886L6.57235 11.6126C7.05389 11.3462 7.05389 10.654 6.57235 10.3876L2.63883 8.21159ZM8.30001 9.00003C8.30001 8.61343 8.61341 8.30003 9.00001 8.30003H13C13.3866 8.30003 13.7 8.61343 13.7 9.00003V13C13.7 13.3866 13.3866 13.7 13 13.7H9.00001C8.61341 13.7 8.30001 13.3866 8.30001 13V9.00003ZM9.20001 9.20003V12.8H12.8V9.20003H9.20001ZM13.4432 2.19311C13.6189 2.01737 13.6189 1.73245 13.4432 1.55671C13.2675 1.38098 12.9826 1.38098 12.8068 1.55671L11 3.36353L9.19321 1.55674C9.01748 1.381 8.73255 1.381 8.55682 1.55674C8.38108 1.73247 8.38108 2.0174 8.55682 2.19313L10.3636 3.99992L8.55682 5.80671C8.38108 5.98245 8.38108 6.26737 8.55682 6.44311C8.73255 6.61885 9.01748 6.61885 9.19321 6.44311L11 4.63632L12.8068 6.44314C12.9826 6.61887 13.2675 6.61887 13.4432 6.44314C13.6189 6.2674 13.6189 5.98247 13.4432 5.80674L11.6364 3.99992L13.4432 2.19311Z"></path>' 
  },
  projects: { 
    title: 'Project', 
    icon: '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M0.900024 7.50002C0.900024 3.85495 3.85495 0.900024 7.50002 0.900024C11.1451 0.900024 14.1 3.85495 14.1 7.50002C14.1 11.1451 11.1451 14.1 7.50002 14.1C3.85495 14.1 0.900024 11.1451 0.900024 7.50002ZM7.50002 1.80002C4.35201 1.80002 1.80002 4.35201 1.80002 7.50002C1.80002 10.648 4.35201 13.2 7.50002 13.2C10.648 13.2 13.2 10.648 13.2 7.50002C13.2 4.35201 10.648 1.80002 7.50002 1.80002ZM3.07504 7.50002C3.07504 5.05617 5.05618 3.07502 7.50004 3.07502C9.94388 3.07502 11.925 5.05617 11.925 7.50002C11.925 9.94386 9.94388 11.925 7.50004 11.925C5.05618 11.925 3.07504 9.94386 3.07504 7.50002ZM7.50004 3.92502C5.52562 3.92502 3.92504 5.52561 3.92504 7.50002C3.92504 9.47442 5.52563 11.075 7.50004 11.075C9.47444 11.075 11.075 9.47442 11.075 7.50002C11.075 5.52561 9.47444 3.92502 7.50004 3.92502ZM7.50004 5.25002C6.2574 5.25002 5.25004 6.25739 5.25004 7.50002C5.25004 8.74266 6.2574 9.75002 7.50004 9.75002C8.74267 9.75002 9.75004 8.74266 9.75004 7.50002C9.75004 6.25738 8.74267 5.25002 7.50004 5.25002ZM6.05004 7.50002C6.05004 6.69921 6.69923 6.05002 7.50004 6.05002C8.30084 6.05002 8.95004 6.69921 8.95004 7.50002C8.95004 8.30083 8.30084 8.95002 7.50004 8.95002C6.69923 8.95002 6.05004 8.30083 6.05004 7.50002Z"></path>' 
  },
  settings: { 
    title: 'Setting', 
    icon: '<svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M12 15.5A3.5 3.5 0 1 0 12 8.5a3.5 3.5 0 0 0 0 7zm7.94-2.06a1 1 0 0 0 .26-1.09l-1.43-4.14a1 1 0 0 0-.76-.65l-4.14-1.43a1 1 0 0 0-1.09.26l-2.83 2.83a1 1 0 0 0-.26 1.09l1.43 4.14a1 1 0 0 0 .76.65l4.14 1.43a1 1 0 0 0 1.09-.26l2.83-2.83z"/></svg>',
    children: [
      { type: 'cluster', title: 'Cluster', icon: '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4.5 7.5a3 3 0 1 1 6 0a3 3 0 0 1-6 0zm3-6a6 6 0 1 0 0 12a6 6 0 0 0 0-12z"></path>' }
    ]
  }
})

const showAddModal = ref(false)
const addType = ref('')
const addName = ref('')
const addRaw = ref('')
const addError = ref('')
const projectRefreshInterval = ref(null)

// 添加连接检查相关的响应式变量
const showConnectionModal = ref(false)
const connectionResult = ref(null)
const connectionLoading = ref(false)
const connectionError = ref(null)

// 添加测试插件相关的响应式变量
const showTestPluginModal = ref(false)
const testPluginName = ref('')
const testPluginArgs = ref([{ value: '' }])
const testPluginLoading = ref(false)
const testPluginResult = ref(null)
const testPluginError = ref(null)

// 添加删除确认相关的响应式变量
const showDeleteModal = ref(false)
const deleteConfirmText = ref('')
const itemToDelete = ref(null)
const deleteError = ref('')

// Lifecycle hooks
onMounted(async () => {
  await fetchAllItems()
  startProjectPolling()
})

onBeforeUnmount(() => {
  // Clear polling timer
  if (projectRefreshInterval.value) {
    clearInterval(projectRefreshInterval.value)
  }
  
  // 移除ESC键监听
  document.removeEventListener('keydown', handleEscKey)
})

// Methods
function startProjectPolling() {
  // Refresh project status every 5 seconds
  projectRefreshInterval.value = setInterval(async () => {
    if (!collapsed.projects) {
      await fetchItems('projects')
    }
  }, 5000)
}

// 处理ESC键按下
function handleEscKey(event) {
  if (event.key === 'Escape') {
    if (showAddModal.value) {
      showAddModal.value = false
    }
    if (showConnectionModal.value) {
      showConnectionModal.value = false
    }
    if (showTestPluginModal.value) {
      showTestPluginModal.value = false
    }
    if (showDeleteModal.value) {
      closeDeleteModal()
    }
  }
}

function openAddModal(type) {
  addType.value = type
  addName.value = ''
  addError.value = ''
  showAddModal.value = true
  
  // 添加ESC键监听
  document.addEventListener('keydown', handleEscKey)
}

function closeAddModal() {
  showAddModal.value = false
  
  // 移除ESC键监听
  document.removeEventListener('keydown', handleEscKey)
}

async function toggleCollapse(type) {
  collapsed[type] = !collapsed[type]
  // If expanding, refresh the list
  if (!collapsed[type]) {
    await fetchItems(type)
  }
}

function openPendingChanges() {
  emit('open-pending-changes')
}

function filteredItems(type) {
  if (!items[type] || !Array.isArray(items[type])) return []
  if (!search.value) return items[type]
  
  return items[type].filter(item => {
    const id = item.id || item.name || ''
    return id.toLowerCase().includes(search.value.toLowerCase())
  })
}

async function fetchAllItems() {
  const types = ['inputs', 'outputs', 'rulesets', 'plugins', 'projects', 'cluster']
  await Promise.all(types.map(type => fetchItems(type)))
}

async function fetchItems(type) {
  loading[type] = true
  error[type] = null
  try {
    let response
    console.log(`Fetching ${type}...`)
    switch (type) {
      case 'inputs':
        response = await hubApi.fetchInputs()
        break
      case 'outputs':
        response = await hubApi.fetchOutputs()
        break
      case 'rulesets':
        response = await hubApi.fetchRulesets()
        break
      case 'plugins':
        response = await hubApi.fetchPlugins()
        break
      case 'projects':
        response = await hubApi.fetchProjects()
        break
      case 'cluster':
        response = await hubApi.fetchClusterInfo()
        break
      default:
        response = []
    }
    console.log(`${type} response:`, response)
    
    // Transform response data to match expected format
    if (Array.isArray(response)) {
      items[type] = response.map(item => {
        if (type === 'plugins') {
          return {
            id: item.name,
            type: item.type
          }
        } else {
          return {
            id: item.id,
            type: item.type,
            status: item.status
          }
        }
      })
    } else {
      items[type] = []
    }
  } catch (err) {
    console.error(`Error fetching ${type}:`, err)
    console.error('Error details:', {
      message: err.message,
      response: err.response?.data,
      status: err.response?.status
    })
    error[type] = `Failed to load ${type}: ${err.message}`
  } finally {
    loading[type] = false
  }
}

function getDefaultConfig(type) {
  const timestamp = Date.now()
  const id = addName.value || `new_${type.slice(0, -1)}_${timestamp}`
  switch (type) {
    case 'inputs':
      return { id, raw: addRaw.value || `name: "${id}"
type: "file"
file:
  path: "/path/to/input.json"
  format: "json"` }
    case 'outputs':
      return { id, raw: addRaw.value || `type: kafka
kafka:
  brokers:
    - 127.0.0.1:9092
  topic: test-topic
  group: test` }
    case 'rulesets':
      return { id, raw: addRaw.value || `<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<root type=\"DETECTION\" />` }
    case 'projects':
      return { id, raw: addRaw.value || `name: "${id}"
flow:
  - from: "input.default"
    to: "ruleset.default"
  - from: "ruleset.default"
    to: "output.default"` }
    case 'plugins':
      return { id, raw: addRaw.value || `// New plugin code` }
    default:
      return { id: '', raw: '' }
  }
}

async function confirmAddName() {
  if (!addName.value || addName.value.trim() === '') {
    addError.value = 'Name cannot be empty'
    return
  }
  
  // 规范化名称，移除空格
  addName.value = addName.value.trim()
  
  // Check for duplicates
  if (items[addType.value].some(item => 
    (item.id && item.id.toLowerCase() === addName.value.toLowerCase()) || 
    (item.name && item.name.toLowerCase() === addName.value.toLowerCase())
  )) {
    addError.value = 'Name already exists'
    return
  }

  try {
    const raw = ''
    switch (addType.value) {
      case 'inputs':
        await hubApi.createInput(addName.value, raw)
        break
      case 'outputs':
        await hubApi.createOutput(addName.value, raw)
        break
      case 'rulesets':
        await hubApi.createRuleset(addName.value, raw)
        break
      case 'projects':
        await hubApi.createProject(addName.value, raw)
        break
      case 'plugins':
        await hubApi.createPlugin(addName.value, raw)
        break
      default:
        throw new Error('Unsupported type')
    }
    
    // Refresh the list
    await fetchItems(addType.value)
    
    // Close the modal
    showAddModal.value = false
    
    // Directly open edit mode
    emit('open-editor', { 
      type: addType.value, 
      id: addName.value, 
      isEdit: true 
    })
  } catch (e) {
    addError.value = 'Creation failed: ' + (e?.message || 'Unknown error')
  }
}

function copyName(item) {
  const text = item.id || item.name
  if (navigator.clipboard) {
    navigator.clipboard.writeText(text)
  } else {
    const input = document.createElement('input')
    input.value = text
    document.body.appendChild(input)
    input.select()
    document.execCommand('copy')
    document.body.removeChild(input)
  }
  // Removed copy success notification to reduce unnecessary alerts
  // Close all menus
  closeAllMenus()
}

// 打开删除确认模态框
function openDeleteModal(type, item) {
  closeAllMenus()
  itemToDelete.value = { type, item }
  deleteConfirmText.value = ''
  deleteError.value = ''
  showDeleteModal.value = true
  
  // 添加ESC键监听
  document.addEventListener('keydown', handleEscKey)
}

// 关闭删除确认模态框
function closeDeleteModal() {
  showDeleteModal.value = false
  itemToDelete.value = null
  deleteConfirmText.value = ''
  deleteError.value = ''
  
  // 移除ESC键监听
  document.removeEventListener('keydown', handleEscKey)
}

// 确认删除
async function confirmDelete() {
  if (deleteConfirmText.value !== 'delete') {
    deleteError.value = 'Please type "delete" to confirm'
    return
  }
  
  if (!itemToDelete.value) {
    closeDeleteModal()
    return
  }
  
  const { type, item } = itemToDelete.value
  
  try {
    if (type === 'inputs') await hubApi.deleteInput(item.id)
    else if (type === 'outputs') await hubApi.deleteOutput(item.id)
    else if (type === 'rulesets') await hubApi.deleteRuleset(item.id)
    else if (type === 'projects') await hubApi.deleteProject(item.id)
    else if (type === 'plugins') await hubApi.deletePlugin(item.id)
    
    // Refresh the list
    await fetchItems(type)
    
    // Show success message
    $message?.success?.('Deleted successfully!')
    
    // Emit delete event to notify parent component
    emit('item-deleted', { type, id: item.id })
    
    // Close modal
    closeDeleteModal()
  } catch (e) {
    deleteError.value = 'Delete failed: ' + (e?.message || 'Unknown error')
  }
}

function deleteItem(type, item) {
  openDeleteModal(type, item)
}

function closeAllMenus() {
  Object.values(items).forEach(arr => {
    if (Array.isArray(arr)) {
      arr.forEach(i => { if (i.menuOpen) i.menuOpen = false })
    }
  })
}

// 添加连接检查函数实现
async function checkConnection(type, item) {
  try {
    connectionLoading.value = true
    connectionError.value = null
    showConnectionModal.value = true
    
    // 添加ESC键监听
    document.addEventListener('keydown', handleEscKey)
    
    const id = item.id || item.name
    const result = await hubApi.connectCheck(type, id)
    connectionResult.value = result
  } catch (error) {
    console.error('Connection check failed:', error)
    connectionError.value = error.message || 'Failed to check connection'
  } finally {
    connectionLoading.value = false
  }
}

// 关闭连接检查模态框
function closeConnectionModal() {
  showConnectionModal.value = false
  
  // 移除ESC键监听
  document.removeEventListener('keydown', handleEscKey)
}

// 打开测试插件弹窗
function openTestPlugin(item) {
  testPluginName.value = item.name || item.id
  testPluginArgs.value = [{ value: '' }]
  testPluginResult.value = null
  testPluginError.value = null
  showTestPluginModal.value = true
  
  // 添加ESC键监听
  document.addEventListener('keydown', handleEscKey)
}

// 关闭测试插件模态框
function closeTestPluginModal() {
  showTestPluginModal.value = false
  
  // 移除ESC键监听
  document.removeEventListener('keydown', handleEscKey)
}

// 打开测试ruleset弹窗
function openTestRuleset(item) {
  console.log('Sidebar: openTestRuleset called with item', item);
  const payload = { 
    type: 'rulesets', 
    id: item.id || item.name
  };
  console.log('Sidebar: emitting test-ruleset event with payload', payload);
  emit('test-ruleset', payload);
  // 确保菜单关闭
  closeAllMenus();
}

// 打开测试output弹窗
function openTestOutput(item) {
  console.log('Sidebar: openTestOutput called with item', item);
  const payload = { 
    type: 'outputs', 
    id: item.id || item.name
  };
  console.log('Sidebar: emitting test-output event with payload', payload);
  emit('test-output', payload);
  // 确保菜单关闭
  closeAllMenus();
}

// 打开测试project弹窗
function openTestProject(item) {
  console.log('Sidebar: openTestProject called with item', item);
  const payload = { 
    type: 'projects', 
    id: item.id || item.name
  };
  console.log('Sidebar: emitting test-project event with payload', payload);
  emit('test-project', payload);
  // 确保菜单关闭
  closeAllMenus();
}

// 添加插件参数
function addPluginArg() {
  testPluginArgs.value.push({ value: '' })
}

// 移除插件参数
function removePluginArg(index) {
  testPluginArgs.value.splice(index, 1)
  if (testPluginArgs.value.length === 0) {
    testPluginArgs.value.push({ value: '' })
  }
}

// 测试插件
async function testPlugin() {
  try {
    testPluginLoading.value = true
    testPluginResult.value = null
    testPluginError.value = null
    
    // 处理参数值，尝试转换为合适的类型
    const args = testPluginArgs.value.map(arg => {
      const value = arg.value.trim()
      if (value === '') return null
      if (value === 'true') return true
      if (value === 'false') return false
      if (!isNaN(value)) return Number(value)
      return value
    })
    
    const result = await hubApi.testPlugin(testPluginName.value, args)
    testPluginResult.value = result
    
    // 处理错误信息
    if (result.error) {
      testPluginError.value = result.error
    }
  } catch (error) {
    console.error('Plugin test failed:', error)
    testPluginError.value = error.message || 'Failed to test plugin'
    testPluginResult.value = { 
      success: false, 
      result: null,
      error: error.message || 'Unknown error occurred'
    }
  } finally {
    testPluginLoading.value = false
  }
}

// 获取参数类型提示
function getArgumentTypeHint() {
  // 默认提示
  return 'String, number, or boolean value'
}

// Expose methods to parent component
defineExpose({
  fetchItems,
  fetchAllItems
})

function handleItemClick(type, item) {
  const id = item.id || item.name;
  emit('select-item', { type, id });
}
</script>

<style>
.custom-scrollbar::-webkit-scrollbar {
  width: 6px;
  background: transparent;
}
.custom-scrollbar::-webkit-scrollbar-thumb {
  background: #e5e7eb;
  border-radius: 3px;
}
</style> 