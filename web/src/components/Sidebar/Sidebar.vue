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
            <!-- Add section icon -->
            <div class="w-4 h-4 mr-1.5 text-gray-600" v-html="section.icon"></div>
            <span class="truncate">{{ section.title }}</span>
          </button>
          <div class="relative">
            <button v-if="!section.children" @click="openAddModal(type)" class="p-1 rounded-full hover:bg-primary/10 text-primary transition flex items-center justify-center w-6 h-6">
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6"></path></svg>
            </button>
          </div>
        </div>
        <div v-if="!collapsed[type]" class="space-y-0.5">
          <div v-if="section.children">
            <!-- 已经将Push Changes整合到section.children中，不再需要单独的部分 -->
            <div v-for="child in section.children" :key="child.type"
                 class="flex items-center justify-between py-1 px-3 rounded-md group cursor-pointer transition-all hover:bg-gray-100"
                 :class="{ 'bg-blue-50': selected && selected.type === child.type }"
                 @click="$emit('select-item', { type: child.type })">
              <div class="flex items-center min-w-0 flex-1">
                <!-- 移除所有子组件的图标 -->
                <span class="text-sm truncate">{{ child.title }}</span>
              </div>
            </div>
          </div>
          <div v-else-if="!loading[type] && !error[type]">
            <div v-for="item in filteredItems(type)" :key="item.id" 
                 class="flex items-center justify-between py-1 px-3 hover:bg-gray-100 rounded-md cursor-pointer group"
                 :class="{ 'bg-blue-50': selected && selected.id === item.id && selected.type === type }"
                 @click="handleItemClick(type, item)">
              <div class="flex items-center min-w-0 flex-1">
                <span class="text-sm truncate">{{ item.id }}</span>
                <!-- Plugin type badge -->
                <span v-if="type === 'plugins' && item.type === 'local'" 
                      class="ml-2 text-xs bg-gray-100 text-gray-800 w-5 h-5 flex items-center justify-center rounded-full cursor-help"
                      @mouseenter="showTooltip($event, 'Built-in Plugin')"
                      @mouseleave="hideTooltip">
                  L
                </span>
                <!-- Temporary file badge -->
                <span v-if="item.hasTemp" 
                      class="ml-2 text-xs bg-blue-100 text-blue-800 w-5 h-5 flex items-center justify-center rounded-full cursor-help"
                      @mouseenter="showTooltip($event, 'Temporary Version')"
                      @mouseleave="hideTooltip">
                  T
                </span>
                <!-- Project status badge -->
                <span v-if="type === 'projects' && item.status" 
                      class="ml-2 text-xs w-5 h-5 flex items-center justify-center rounded-full cursor-help"
                      :class="{
                        'bg-green-100 text-green-800': item.status === 'running',
                        'bg-gray-100 text-gray-800': item.status === 'stopped',
                        'bg-red-100 text-red-800': item.status === 'error'
                      }"
                      @mouseenter="showTooltip($event, getStatusTitle(item))"
                      @mouseleave="hideTooltip">
                  {{ getStatusLabel(item.status) }}
                </span>
              </div>
              
              <!-- Actions menu -->
              <div class="relative">
                <button class="p-1 rounded-full text-gray-400 hover:text-gray-600 hover:bg-gray-200 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity menu-toggle-button w-6 h-6 flex items-center justify-center"
                        @click.stop="toggleMenu(item)">
                  <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 5v.01M12 12v.01M12 19v.01M12 6a1 1 0 110-2 1 1 0 010 2zm0 7a1 1 0 110-2 1 1 0 010 2zm0 7a1 1 0 110-2 1 1 0 010 2z"></path>
                  </svg>
                </button>
                <!-- Dropdown menu -->
                <div v-if="item.menuOpen" 
                     class="absolute right-0 mt-1 w-48 bg-white rounded-md shadow-lg z-10 dropdown-menu"
                     @click.stop>
                  <div class="py-1">
                    <!-- Edit action -->
                    <a v-if="!(type === 'plugins' && item.type === 'local')" 
                       href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                       @click.prevent.stop="closeAllMenus(); $emit('open-editor', { type, id: item.id, isEdit: true })">
                      <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"></path>
                      </svg>
                      Edit
                    </a>
                    
                    <!-- Project specific actions -->
                    <template v-if="type === 'projects'">
                      <!-- Start action -->
                      <a v-if="item.status === 'stopped' && !item.hasTemp" href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                         @click.prevent.stop="startProject(item)">
                        <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                        </svg>
                        Start
                      </a>
                      
                      <!-- Stop action -->
                      <a v-if="item.status === 'running' && !item.hasTemp" href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                         @click.prevent.stop="stopProject(item)">
                        <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 10a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1h-4a1 1 0 01-1-1v-4z" />
                        </svg>
                        Stop
                      </a>
                      
                      <!-- Restart action -->
                      <a v-if="item.status === 'running' && !item.hasTemp" href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                         @click.prevent.stop="restartProject(item)">
                        <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                        </svg>
                        Restart
                      </a>
                    </template>
                    
                    <!-- Test actions for different component types -->
                    <a v-if="type === 'inputs' || type === 'outputs'" href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                       @click.prevent.stop="checkConnection(type, item)">
                      <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
                      </svg>
                      Connect Check
                    </a>
                    
                    <!-- 添加查看使用情况选项，仅对input、output和ruleset类型显示 -->
                    <a v-if="type === 'inputs' || type === 'outputs' || type === 'rulesets'" href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                       @click.prevent.stop="openUsageModal(type, item)">
                      <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" />
                      </svg>
                      View Usage
                    </a>
                    
                    <a v-if="type === 'plugins'" href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                       @click.prevent.stop="openTestPlugin(item)">
                      <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                      </svg>
                      Test Plugin
                    </a>
                    
                    <a v-if="type === 'rulesets'" href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                       @click.prevent.stop="openTestRuleset(item)">
                      <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                      </svg>
                      Test Ruleset
                    </a>
                    
                    <a v-if="type === 'outputs'" href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                       @click.prevent.stop="openTestOutput(item)">
                      <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                      </svg>
                      Test Output
                    </a>
                    
                    <a v-if="type === 'projects'" href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                       @click.prevent.stop="openTestProject(item)">
                      <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                      </svg>
                      Test Project
                    </a>
                    
                    <!-- Copy name action -->
                    <a href="#" class="flex items-center px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" 
                       @click.prevent.stop="copyName(item)">
                      <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 5H6a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2v-1M8 5a2 2 0 002 2h2a2 2 0 002-2M8 5a2 2 0 012-2h2a2 2 0 012 2m0 0h2a2 2 0 012 2v3m2 4H10m0 0l3-3m-3 3l3 3"></path>
                      </svg>
                      Copy Name
                    </a>
                    
                    <!-- Delete action -->
                    <div v-if="!(type === 'plugins' && item.type === 'local')" class="border-t border-gray-100 my-1"></div>
                    <a v-if="!(type === 'plugins' && item.type === 'local')" 
                       href="#" class="flex items-center px-4 py-2 text-sm text-red-600 hover:bg-red-50" 
                       @click.prevent.stop="openDeleteModal(type, item)">
                      <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1-1v3M4 7h16" />
                      </svg>
                      Delete
                    </a>
                  </div>
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
          <input 
            type="text" 
            v-model="addName" 
            @keyup.enter="confirmAddName"
            class="w-full px-3 py-2 border border-gray-300 rounded-md focus:outline-none focus:ring-1 focus:ring-blue-500" 
            placeholder="Enter name" 
            ref="addNameInput"
          />
        </div>
        <div class="flex justify-end space-x-3">
          <button @click="closeAddModal" class="px-3 py-1 text-sm text-gray-500">Cancel</button>
          <button 
            @click="confirmAddName" 
            :disabled="!addName || !addName.trim()"
            class="px-3 py-1 bg-blue-500 text-white text-sm rounded disabled:bg-gray-300 disabled:cursor-not-allowed transition-colors"
          >
            Create
          </button>
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

    <!-- Test Plugin Modal -->
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

    <!-- Project Warning Modal -->
    <div v-if="showProjectWarningModal" class="fixed inset-0 bg-black bg-opacity-30 flex items-center justify-center z-50">
      <div class="bg-white rounded-lg shadow-xl w-96 p-6">
        <div class="flex items-center mb-4 text-yellow-600">
          <svg class="w-6 h-6 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
          </svg>
          <h3 class="text-lg font-medium">Warning</h3>
        </div>
        
        <p class="mb-4 text-sm text-gray-600">{{ projectWarningMessage }}</p>
        
        <div class="flex justify-end space-x-3">
          <button @click="closeProjectWarningModal" class="px-3 py-1.5 border border-gray-300 text-gray-700 text-sm rounded hover:bg-gray-50">
            Cancel
          </button>
          <button @click="continueProjectOperation" class="px-3 py-1.5 bg-yellow-500 text-white text-sm rounded hover:bg-yellow-600" :disabled="projectOperationLoading">
            <span v-if="projectOperationLoading" class="w-3 h-3 border-1.5 border-white border-t-transparent rounded-full animate-spin mr-1"></span>
            Continue Anyway
          </button>
        </div>
      </div>
    </div>

    <!-- Tooltip component -->
    <div v-if="tooltip.show" 
         class="absolute z-50 bg-gray-800 text-white text-xs rounded py-1 px-2 max-w-xs"
         :style="{
           top: tooltip.y + 'px',
           left: tooltip.x + 'px',
           transform: 'translate(-50%, -100%)',
           marginTop: '-8px'
         }">
      {{ tooltip.text }}
    </div>

    <!-- 添加组件使用情况模态框 -->
    <div v-if="showUsageModal" class="fixed inset-0 bg-black bg-opacity-30 flex items-center justify-center z-50">
      <div class="bg-white rounded shadow-lg p-6 w-96 max-h-[80vh] overflow-y-auto">
        <div class="flex justify-between items-center mb-4">
          <h3 class="font-bold">Component Usage</h3>
          <button @click="closeUsageModal" class="text-gray-400 hover:text-gray-600">
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
            </svg>
          </button>
        </div>
        
        <div v-if="usageLoading" class="flex justify-center items-center py-8">
          <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
        </div>
        
        <div v-else-if="usageError" class="bg-red-50 border-l-4 border-red-500 p-4 mb-4">
          <div class="flex">
            <div class="flex-shrink-0">
              <svg class="h-5 w-5 text-red-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
              </svg>
            </div>
            <div class="ml-3">
              <p class="text-sm text-red-700">{{ usageError }}</p>
            </div>
          </div>
        </div>
        
        <div v-else>
          <div class="mb-3">
            <div class="text-sm text-gray-600 mb-1">Component Type:</div>
            <div class="font-medium">{{ usageComponentType }}</div>
          </div>
          
          <div class="mb-3">
            <div class="text-sm text-gray-600 mb-1">Component ID:</div>
            <div class="font-medium">{{ usageComponentId }}</div>
          </div>
          
          <div class="mb-3">
            <div class="text-sm text-gray-600 mb-1">Projects using this component:</div>
            <div v-if="usageProjects.length === 0" class="text-gray-500 italic">
              No projects are using this component
            </div>
            <div v-else class="mt-2 space-y-2">
              <div v-for="project in usageProjects" :key="project.id" 
                   class="p-2 border rounded-md flex items-center justify-between cursor-pointer hover:bg-gray-50 transition-colors"
                   @click="navigateToProject(project.id)">
                <div class="flex items-center">
                  <span class="w-2 h-2 rounded-full mr-2"
                        :class="{
                          'bg-green-500': project.status === 'running',
                          'bg-gray-500': project.status === 'stopped',
                          'bg-red-500': project.status === 'error'
                        }"></span>
                  <span>{{ project.id }}</span>
                </div>
                <div>
                  <span class="text-xs px-2 py-0.5 rounded-full"
                        :class="{
                          'bg-green-100 text-green-800': project.status === 'running',
                          'bg-gray-100 text-gray-800': project.status === 'stopped',
                          'bg-red-100 text-red-800': project.status === 'error'
                        }">
                    {{ project.status }}
                  </span>
                </div>
              </div>
            </div>
          </div>
        </div>
        
        <div class="flex justify-end mt-4">
          <button @click="closeUsageModal" class="px-4 py-2 bg-gray-100 hover:bg-gray-200 rounded text-sm transition">Close</button>
        </div>
      </div>
    </div>
  </aside>
</template>

<script setup>
import { ref, reactive, onMounted, onBeforeUnmount, inject, nextTick } from 'vue'
import { hubApi } from '@/api'
import { useRouter } from 'vue-router'

// 获取路由器实例
const router = useRouter()

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

const items = reactive({
  inputs: [],
  outputs: [],
  rulesets: [],
  plugins: [],
  projects: [],
  cluster: []
})

const collapsed = reactive({
  inputs: true,
  outputs: true,
  rulesets: true,
  plugins: true,
  projects: true,
  settings: true
})

const sections = reactive({
  inputs: { 
    title: 'Input', 
    icon: '<svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"/></svg>' 
  },
  outputs: { 
    title: 'Output', 
    icon: '<svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M16 15l-3-3 3-3m-5 3h8M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"/></svg>' 
  },
  rulesets: { 
    title: 'Ruleset', 
    icon: '<svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-3 7h3m-3 4h3m-6-4h.01M9 16h.01"/></svg>' 
  },
  plugins: { 
    title: 'Plugin', 
    icon: '<svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M11 4a2 2 0 114 0v1a1 1 0 001 1h3a1 1 0 011 1v3a1 1 0 01-1 1h-1a2 2 0 100 4h1a1 1 0 011 1v3a1 1 0 01-1 1h-3a1 1 0 01-1-1v-1a2 2 0 10-4 0v1a1 1 0 01-1 1H7a1 1 0 01-1-1v-3a1 1 0 00-1-1H4a2 2 0 110-4h1a1 1 0 001-1V7a1 1 0 011-1h3a1 1 0 001-1V4z"/></svg>' 
  },
  projects: { 
    title: 'Project', 
    icon: '<svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z"/></svg>' 
  },
  settings: { 
    title: 'Setting', 
    icon: '<svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"></path><path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"></path></svg>',
    children: [
      { type: 'pending-changes', title: 'Push Changes' },
      { type: 'load-local-components', title: 'Load Local Components' },
      { type: 'cluster', title: 'Cluster', icon: '<svg class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" d="M5 12a7 7 0 1114 0 7 7 0 01-14 0zM12 8v4l3 3"></path></svg>' }
    ]
  }
})

const showAddModal = ref(false)
const addType = ref('')
const addName = ref('')
const addRaw = ref('')
const addError = ref('')
const projectRefreshInterval = ref(null)

// Connection check related reactive variables
const showConnectionModal = ref(false)
const connectionResult = ref(null)
const connectionLoading = ref(false)
const connectionError = ref(null)

// Plugin testing related reactive variables
const showTestPluginModal = ref(false)
const testPluginName = ref('')
const testPluginArgs = ref([{ value: '' }])
const testPluginLoading = ref(false)
const testPluginResult = ref(null)
const testPluginError = ref(null)

// Delete confirmation related reactive variables
const showDeleteModal = ref(false)
const deleteConfirmText = ref('')
const itemToDelete = ref(null)
const deleteError = ref('')

// Project operation states
const projectOperationLoading = ref(false)
const showProjectWarningModal = ref(false)
const projectWarningMessage = ref('')
const projectOperationItem = ref(null)
const projectOperationType = ref('') // 'start', 'stop', 'restart'

// Flag variable to track if ESC key listener is added
const escKeyListenerAdded = ref(false)

// Search
const search = ref('')

// Centralized modal management
const activeModal = ref(null) // Tracks which modal is currently active

// Tooltip state
const tooltip = reactive({
  show: false,
  text: '',
  x: 0,
  y: 0
})

// Add ESC key listener
function addEscKeyListener() {
  if (!escKeyListenerAdded.value) {
    document.addEventListener('keydown', handleEscKey)
    escKeyListenerAdded.value = true
  }
}

// Remove ESC key listener
function removeEscKeyListener() {
  if (escKeyListenerAdded.value) {
    document.removeEventListener('keydown', handleEscKey)
    escKeyListenerAdded.value = false
  }
}

// Handle ESC key press
function handleEscKey(event) {
  if (event.key === 'Escape' && activeModal.value) {
    closeActiveModal()
  }
}

// Close currently active modal
function closeActiveModal() {
  switch (activeModal.value) {
    case 'delete':
      closeDeleteModal()
      break
    case 'add':
      closeAddModal()
      break
    case 'connection':
      closeConnectionModal()
      break
    case 'testPlugin':
      closeTestPluginModal()
      break
    case 'testRuleset':
      closeTestRulesetModal()
      break
    case 'testOutput':
      closeTestOutputModal()
      break
    case 'testProject':
      closeTestProjectModal()
      break
    case 'projectWarning':
      closeProjectWarningModal()
      break
    case 'usage':
      closeUsageModal()
      break
  }
  
  activeModal.value = null
}

// Lifecycle hooks
onMounted(async () => {
  await fetchAllItems()
  startProjectPolling()
  
  // Add click event listener to close menus when clicking outside
  document.addEventListener('click', handleOutsideClick)
})

onBeforeUnmount(() => {
  // Clear polling timer
  if (projectRefreshInterval.value) {
    clearInterval(projectRefreshInterval.value)
  }
  
  // Remove ESC key listener
  removeEscKeyListener()
  
  // Remove click event listener
  document.removeEventListener('click', handleOutsideClick)
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

function openAddModal(type) {
  addType.value = type
  addName.value = ''
  addError.value = ''
  showAddModal.value = true
  activeModal.value = 'add'
  
  addEscKeyListener()
  
  // Auto focus on input field when modal opens
  nextTick(() => {
    const inputElement = document.querySelector('input[ref="addNameInput"]') || 
                        document.querySelector('.bg-white input[type="text"]')
    if (inputElement) {
      inputElement.focus()
    }
  })
}

function closeAddModal() {
  showAddModal.value = false
  activeModal.value = null
  
  if (!isAnyModalOpen()) {
    removeEscKeyListener()
  }
}

async function toggleCollapse(type) {
  collapsed[type] = !collapsed[type]
  // If expanding, refresh the list
  if (!collapsed[type]) {
    await fetchItems(type)
  }
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
    // Use new API method to get components with temporary file information
    response = await hubApi.fetchComponentsWithTempInfo(type);

    // Transform response data to match expected format
    if (Array.isArray(response)) {
      items[type] = response.map(item => {
        // 确保只处理属于当前类型的组件
        if (type === 'plugins') {
          // 插件必须有name字段
          if (!item.name) {
            console.warn(`Skipping invalid plugin item:`, item);
            return null;
          }
          return {
            id: item.name,
            type: item.type,
            hasTemp: item.hasTemp
          }
        } else {
          // 其他组件必须有id字段
          if (!item.id) {
            console.warn(`Skipping invalid ${type} item:`, item);
            return null;
          }
          
          // 如果是项目且状态为error，获取错误信息
          if (type === 'projects' && item.status === 'error') {
            // 异步获取错误信息，但不等待
            (async () => {
              try {
                const projectDetails = await hubApi.getProject(item.id);
                if (projectDetails && projectDetails.errorMessage) {
                  // 更新项目的错误信息
                  const index = items[type].findIndex(p => p.id === item.id);
                  if (index !== -1) {
                    items[type][index].errorMessage = projectDetails.errorMessage;
                  }
                }
              } catch (err) {
                console.error(`Failed to fetch error details for project ${item.id}:`, err);
              }
            })();
          }
          
          return {
            id: item.id,
            type: item.type,
            status: item.status,
            hasTemp: item.hasTemp,
            errorMessage: item.errorMessage || ''
          }
        }
      }).filter(Boolean) // 过滤掉null项
      
      // 对列表按照ID排序
      items[type].sort((a, b) => {
        const idA = a.id || a.name || ''
        const idB = b.id || b.name || ''
        return idA.localeCompare(idB)
      })
    } else {
      items[type] = []
    }
  } catch (err) {
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
  
  // Normalize name by removing whitespace
  addName.value = addName.value.trim()

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

// Open delete confirmation modal
function openDeleteModal(type, item) {
  closeAllMenus()
  itemToDelete.value = { type, item }
  deleteConfirmText.value = ''
  deleteError.value = ''
  showDeleteModal.value = true
  activeModal.value = 'delete'
  
  addEscKeyListener()
}

// Close delete confirmation modal
function closeDeleteModal() {
  showDeleteModal.value = false
  itemToDelete.value = null
  deleteConfirmText.value = ''
  deleteError.value = ''
  activeModal.value = null
  
  if (!isAnyModalOpen()) {
    removeEscKeyListener()
  }
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
    
    // If the currently selected item is the one being deleted, clear selection
    if (props.selected && props.selected.type === type && props.selected.id === item.id) {
      emit('select-item', { type: null, id: null })
    }
    
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
  // Close all dropdown menus
  Object.keys(items).forEach(type => {
    if (Array.isArray(items[type])) {
      items[type].forEach(item => {
        if (item.menuOpen) {
          item.menuOpen = false
        }
      })
    }
  })
}

// Implement connection check function
async function checkConnection(type, item) {
  closeAllMenus()
  try {
    connectionLoading.value = true
    connectionError.value = null
    showConnectionModal.value = true
    activeModal.value = 'connection'
    
    addEscKeyListener()
    
    const id = item.id || item.name
    const result = await hubApi.connectCheck(type, id)
    connectionResult.value = result
  } catch (error) {
    connectionError.value = error.message || 'Failed to check connection'
  } finally {
    connectionLoading.value = false
  }
}

// Close connection check modal
function closeConnectionModal() {
  showConnectionModal.value = false
  activeModal.value = null
  
  if (!isAnyModalOpen()) {
    removeEscKeyListener()
  }
}

// Open test plugin modal
function openTestPlugin(item) {
  closeAllMenus()
  testPluginName.value = item.name || item.id
  testPluginArgs.value = [{ value: '' }]
  testPluginResult.value = null
  testPluginError.value = null
  showTestPluginModal.value = true
  activeModal.value = 'testPlugin'
  
  addEscKeyListener()
}

// Close test plugin modal
function closeTestPluginModal() {
  showTestPluginModal.value = false
  activeModal.value = null
  
  if (!isAnyModalOpen()) {
    removeEscKeyListener()
  }
}

// Open test ruleset modal
function openTestRuleset(item) {
  const payload = {
    type: 'rulesets', 
    id: item.id || item.name
  };
  emit('test-ruleset', payload);
  // Ensure menus are closed
  closeAllMenus();
}

// Open test output modal
function openTestOutput(item) {
  const payload = {
    type: 'outputs', 
    id: item.id || item.name
  };
  emit('test-output', payload);
  // Ensure menus are closed
  closeAllMenus();
}

// Open test project modal
function openTestProject(item) {
  const payload = {
    type: 'projects', 
    id: item.id || item.name
  };
  emit('test-project', payload);
  // Ensure menus are closed
  closeAllMenus();
}

// Add plugin parameter
function addPluginArg() {
  testPluginArgs.value.push({ value: '' })
}

// Remove plugin parameter
function removePluginArg(index) {
  testPluginArgs.value.splice(index, 1)
  if (testPluginArgs.value.length === 0) {
    testPluginArgs.value.push({ value: '' })
  }
}

// Test plugin
async function testPlugin() {
  try {
    testPluginLoading.value = true
    testPluginResult.value = null
    testPluginError.value = null
    
    // Process parameter values, try to convert to appropriate types
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
    
    // Handle error message
    if (result.error) {
      testPluginError.value = result.error
    }
  } catch (error) {
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

// Get parameter type hint
function getArgumentTypeHint() {
  // Default hint
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

// Project operations
async function startProject(item) {
  closeAllMenus()
  projectOperationLoading.value = true
  
  try {
    const result = await hubApi.startProject(item.id)
    
    if (result.warning) {
      // If there are warnings (e.g., temporary files exist), show warning modal
      projectWarningMessage.value = result.message
      projectOperationItem.value = item
      projectOperationType.value = 'start'
      showProjectWarningModal.value = true
      activeModal.value = 'projectWarning'
      addEscKeyListener()
    } else if (result.success) {
      // Project started successfully
      $message?.success?.('Project started successfully')
      // Update project status
      item.status = 'running'
      // Refresh project list
      await fetchItems('projects')
    } else if (result.error) {
      // Start failed
      $message?.error?.('Failed to start project: ' + result.error)
    }
  } catch (error) {
    $message?.error?.('Error starting project: ' + (error.message || 'Unknown error'))
  } finally {
    projectOperationLoading.value = false
  }
}

async function stopProject(item) {
  closeAllMenus()
  projectOperationLoading.value = true
  
  try {
    const result = await hubApi.stopProject(item.id)
    
    if (result.warning) {
      // If there are warnings (e.g., temporary files exist), show warning modal
      projectWarningMessage.value = result.message
      projectOperationItem.value = item
      projectOperationType.value = 'stop'
      showProjectWarningModal.value = true
      activeModal.value = 'projectWarning'
      addEscKeyListener()
    } else if (result.success) {
      // Project stopped successfully
      $message?.success?.('Project stopped successfully')
      // Update project status
      item.status = 'stopped'
      // Refresh project list
      await fetchItems('projects')
    } else if (result.error) {
      // Stop failed
      $message?.error?.('Failed to stop project: ' + result.error)
    }
  } catch (error) {
    $message?.error?.('Error stopping project: ' + (error.message || 'Unknown error'))
  } finally {
    projectOperationLoading.value = false
  }
}

async function restartProject(item) {
  closeAllMenus()
  projectOperationLoading.value = true
  
  try {
    const result = await hubApi.restartProject(item.id)
    
    if (result.warning) {
      // If there are warnings (e.g., temporary files exist), show warning modal
      projectWarningMessage.value = result.message
      projectOperationItem.value = item
      projectOperationType.value = 'restart'
      showProjectWarningModal.value = true
      activeModal.value = 'projectWarning'
      addEscKeyListener()
    } else if (result.success) {
      // Project restarted successfully
      $message?.success?.('Project restarted successfully')
      // Update project status
      item.status = 'running'
      // Refresh project list
      await fetchItems('projects')
    } else if (result.error) {
      // Restart failed
      $message?.error?.('Failed to restart project: ' + result.error)
    }
  } catch (error) {
    $message?.error?.('Error restarting project: ' + (error.message || 'Unknown error'))
  } finally {
    projectOperationLoading.value = false
  }
}

// Close project operation warning modal
function closeProjectWarningModal() {
  showProjectWarningModal.value = false
  projectWarningMessage.value = ''
  projectOperationItem.value = null
  projectOperationType.value = ''
  activeModal.value = null
  
  if (!isAnyModalOpen()) {
    removeEscKeyListener()
  }
}

// Continue project operation (based on original project, not temporary files)
async function continueProjectOperation() {
  if (!projectOperationItem.value || !projectOperationType.value) {
    closeProjectWarningModal()
    return
  }
  
  const item = projectOperationItem.value
  const operationType = projectOperationType.value
  
  closeProjectWarningModal()
  projectOperationLoading.value = true
  
  try {
    let result
    
    // Perform operations on the original project
    if (operationType === 'start') {
      // Start using original project ID
      const response = await hubApi.startProject(item.id)
      result = { success: true, data: response.data }
    } else if (operationType === 'stop') {
      // Stop using original project ID
      const response = await hubApi.stopProject(item.id)
      result = { success: true, data: response.data }
    } else if (operationType === 'restart') {
      // Stop first, then start
      await hubApi.stopProject(item.id)
      const response = await hubApi.startProject(item.id)
      result = { success: true, data: response.data }
    }
    
    if (result && result.success) {
      // Operation executed successfully
      $message?.success?.(`Project ${operationType}ed successfully`)
      // Update project status
      if (operationType === 'start' || operationType === 'restart') {
        item.status = 'running'
      } else if (operationType === 'stop') {
        item.status = 'stopped'
      }
      
      // Refresh project list
      await fetchItems('projects')
    }
  } catch (error) {
    $message?.error?.(`Error ${operationType}ing project: ` + (error.message || 'Unknown error'))
  } finally {
    projectOperationLoading.value = false
  }
}

// Check if any modal is open
function isAnyModalOpen() {
  // Simply check if there's an active modal
  return activeModal.value !== null;
}

// Handle clicks outside the menu
function handleOutsideClick(event) {
  // Check if the click is inside a dropdown menu or on a menu toggle button
  const isMenuClick = event.target.closest('.dropdown-menu')
  const isToggleClick = event.target.closest('.menu-toggle-button')
  
  // If clicking inside menu or on toggle button, don't close
  if (isMenuClick || isToggleClick) {
    return
  }
  
  // Close all menus
  closeAllMenus()
}

// Get status title based on project status
function getStatusTitle(item) {
  switch (item.status) {
    case 'running':
      return 'Running'
    case 'stopped':
      return 'Stopped'
    case 'error':
      // 如果有错误信息，则显示错误信息
      return item.errorMessage ? `Error: ${item.errorMessage}` : 'Error'
    default:
      return 'Unknown'
  }
}

// Get status label based on project status
function getStatusLabel(status) {
  switch (status) {
    case 'running':
      return 'R'
    case 'stopped':
      return 'S'
    case 'error':
      return 'E'
    default:
      return '?'
  }
}

// Show tooltip
function showTooltip(event, text) {
  tooltip.text = text
  tooltip.x = event.clientX
  tooltip.y = event.clientY
  tooltip.show = true
}

// Hide tooltip
function hideTooltip() {
  tooltip.show = false
}

// Variables related to component usage
const showUsageModal = ref(false)
const usageLoading = ref(false)
const usageError = ref(null)
const usageComponentType = ref('')
const usageComponentId = ref('')
const usageProjects = ref([])

// Add the "View Usage" option to the three-point menu
function openUsageModal(type, item) {
  closeAllMenus()
  usageLoading.value = true
  usageError.value = null
  usageComponentType.value = type
  usageComponentId.value = item.id || item.name
  usageProjects.value = []
  showUsageModal.value = true
  activeModal.value = 'usage'
  
  addEscKeyListener()
  
  // Obtain component usage status
  fetchComponentUsage(type, item.id || item.name)
}

// Close the usage mode box
function closeUsageModal() {
  showUsageModal.value = false
  activeModal.value = null
  
  if (!isAnyModalOpen()) {
    removeEscKeyListener()
  }
}

// Obtain component usage status
async function fetchComponentUsage(type, id) {
  try {
    const result = await hubApi.getComponentUsage(type, id)
    usageProjects.value = result.usage || []
  } catch (error) {
    usageError.value = error.message || 'Failed to fetch component usage'
  } finally {
    usageLoading.value = false
  }
}

// Jump to the project details page
function navigateToProject(projectId) {
  closeUsageModal()
  
  router.push(`/app/projects/${projectId}`)
  
  // Notify the parent component that a project has been selected
  emit('select-item', {
    type: 'projects',
    id: projectId,
    isEdit: false,
    _timestamp: Date.now()
  })
}

// Toggle menu for a specific item
function toggleMenu(item) {
  const wasOpen = item.menuOpen
  
  // Close all menus first
  closeAllMenus()
  
  // If the menu wasn't open, open it
  if (!wasOpen) {
    item.menuOpen = true
  }
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