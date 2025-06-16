<template>
  <div v-if="loading" class="flex items-center justify-center h-full text-gray-400 text-lg">Loading...</div>
  <div v-else-if="error" class="flex items-center justify-center h-full text-red-400 text-lg">{{ error }}</div>
  
  <!-- Create Mode -->
  <div v-else-if="props.item && props.item.isNew" class="h-full flex flex-col">
    <MonacoEditor v-model:value="editorValue" :language="props.item.type === 'rulesets' ? 'xml' : (props.item.type === 'plugins' ? 'go' : 'yaml')" :read-only="false" :error-lines="errorLines" class="flex-1" @save="content => saveNew(content)" />
    <div class="flex justify-end mt-4 px-4 space-x-3 border-t pt-4 pb-3">
      <button 
        v-if="isRuleset"
        @click="showTestModal = true" 
        class="px-3 py-1.5 bg-indigo-500 text-white text-sm rounded hover:bg-indigo-600 transition-colors duration-150 focus:outline-none focus:ring-1 focus:ring-indigo-500 flex items-center space-x-1.5"
      >
        <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
        </svg>
        <span>Test</span>
      </button>
      <button 
        v-if="isOutput"
        @click="showOutputTestModal = true" 
        class="px-3 py-1.5 bg-indigo-500 text-white text-sm rounded hover:bg-indigo-600 transition-colors duration-150 focus:outline-none focus:ring-1 focus:ring-indigo-500 flex items-center space-x-1.5"
      >
        <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
        </svg>
        <span>Test</span>
      </button>
      <button 
        v-if="isProject"
        @click="showProjectTestModal = true" 
        class="px-3 py-1.5 bg-indigo-500 text-white text-sm rounded hover:bg-indigo-600 transition-colors duration-150 focus:outline-none focus:ring-1 focus:ring-indigo-500 flex items-center space-x-1.5"
      >
        <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
        </svg>
        <span>Test</span>
      </button>
      <button 
        @click="() => saveNew()" 
        class="px-3 py-1.5 bg-blue-500 text-white text-sm rounded hover:bg-blue-600 transition-colors duration-150 focus:outline-none focus:ring-1 focus:ring-blue-500 flex items-center space-x-1.5"
        :disabled="saving"
      >
        <span v-if="saving" class="w-3 h-3 border-1.5 border-white border-t-transparent rounded-full animate-spin"></span>
        <span>{{ saving ? 'Saving...' : 'Save' }}</span>
      </button>
    </div>
    <div v-if="saveError" class="text-xs text-red-500 mt-2">{{ saveError }}</div>
  </div>

  <!-- Edit Mode -->
  <div v-else-if="props.item && props.item.isEdit && detail" class="h-full flex flex-col relative">
    <!-- Floating Validation Status -->
    <div v-if="isRuleset && (validationResult.errors.length > 0 || validationResult.warnings.length > 0) && showValidationPanel" 
         class="absolute top-4 right-4 z-50 max-w-md bg-white border border-gray-200 rounded-lg shadow-lg">
      <!-- Validation Errors -->
      <div v-if="validationResult.errors.length > 0" class="validation-errors p-3 bg-red-50 border-l-4 border-red-500 text-red-700 rounded-t-lg">
        <div class="flex justify-between items-start mb-2">
          <h3 class="font-bold text-sm">Validation Errors</h3>
          <button @click="showValidationPanel = false" class="text-red-400 hover:text-red-600 ml-2">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
            </svg>
          </button>
        </div>
        <ul class="text-xs">
          <li v-for="(error, index) in validationResult.errors" :key="index" class="mb-1">
            <span class="font-semibold">Line {{ error.line }}:</span> 
            {{ error.message }}
            <span v-if="error.detail" class="block ml-4 text-red-600 italic">{{ error.detail }}</span>
          </li>
        </ul>
      </div>

      <!-- Validation Warnings -->
      <div v-if="validationResult.warnings.length > 0" 
           class="validation-warnings p-3 bg-yellow-50 border-l-4 border-yellow-500 text-yellow-700"
           :class="{ 'rounded-t-lg': validationResult.errors.length === 0, 'rounded-b-lg': true }">
        <div v-if="validationResult.errors.length === 0" class="flex justify-between items-start mb-2">
          <h3 class="font-bold text-sm">Validation Warnings</h3>
          <button @click="showValidationPanel = false" class="text-yellow-400 hover:text-yellow-600 ml-2">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
            </svg>
          </button>
        </div>
        <h3 v-else class="font-bold text-sm mb-2">Validation Warnings</h3>
        <ul class="text-xs">
          <li v-for="(warning, index) in validationResult.warnings" :key="index" class="mb-1">
            <span class="font-semibold">Line {{ warning.line }}:</span> 
            {{ warning.message }}
            <span v-if="warning.detail" class="block ml-4 text-yellow-600 italic">{{ warning.detail }}</span>
          </li>
        </ul>
      </div>
    </div>

    <!-- Validation Status Indicator -->
    <div v-if="isRuleset && (validationResult.errors.length > 0 || validationResult.warnings.length > 0) && !showValidationPanel"
         class="absolute top-4 right-4 z-50">
      <button @click="showValidationPanel = true" 
              class="flex items-center space-x-1 px-2 py-1 rounded-full text-white text-xs shadow-lg transition-all hover:scale-105"
              :class="validationResult.errors.length > 0 ? 'bg-red-500 hover:bg-red-600' : 'bg-yellow-500 hover:bg-yellow-600'">
        <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
        </svg>
        <span>{{ validationResult.errors.length > 0 ? `${validationResult.errors.length} Error${validationResult.errors.length > 1 ? 's' : ''}` : `${validationResult.warnings.length} Warning${validationResult.warnings.length > 1 ? 's' : ''}` }}</span>
      </button>
    </div>
    
    <MonacoEditor v-model:value="editorValue" :language="props.item.type === 'rulesets' ? 'xml' : (props.item.type === 'plugins' ? 'go' : 'yaml')" :read-only="false" :error-lines="errorLines" class="flex-1" @save="content => saveEdit(content)" />
    <div class="flex justify-end mt-4 px-4 space-x-3 border-t pt-4 pb-3">
      <button 
        @click="cancelEdit" 
        class="px-3 py-1.5 border border-gray-300 text-gray-700 text-sm rounded hover:bg-gray-50 transition-colors duration-150 focus:outline-none focus:ring-1 focus:ring-gray-300"
      >
        Cancel
      </button>
      <button 
        v-if="isRuleset"
        @click="showTestModal = true" 
        class="px-3 py-1.5 bg-indigo-500 text-white text-sm rounded hover:bg-indigo-600 transition-colors duration-150 focus:outline-none focus:ring-1 focus:ring-indigo-500 flex items-center space-x-1.5"
      >
        <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
        </svg>
        <span>Test</span>
      </button>
      <button 
        v-if="isOutput"
        @click="showOutputTestModal = true" 
        class="px-3 py-1.5 bg-indigo-500 text-white text-sm rounded hover:bg-indigo-600 transition-colors duration-150 focus:outline-none focus:ring-1 focus:ring-indigo-500 flex items-center space-x-1.5"
      >
        <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
        </svg>
        <span>Test</span>
      </button>
      <button 
        v-if="isProject"
        @click="showProjectTestModal = true" 
        class="px-3 py-1.5 bg-indigo-500 text-white text-sm rounded hover:bg-indigo-600 transition-colors duration-150 focus:outline-none focus:ring-1 focus:ring-indigo-500 flex items-center space-x-1.5"
      >
        <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
        </svg>
        <span>Test</span>
      </button>
      <button 
        v-if="isProject"
        @click="verifyProject" 
        class="px-3 py-1.5 bg-green-500 text-white text-sm rounded hover:bg-green-600 transition-colors duration-150 focus:outline-none focus:ring-1 focus:ring-green-500 flex items-center space-x-1.5 mr-2"
        :disabled="verifyLoading"
      >
        <span v-if="verifyLoading" class="w-3 h-3 border-1.5 border-white border-t-transparent rounded-full animate-spin"></span>
        <svg v-else xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        <span>{{ verifyLoading ? 'Verifying...' : 'Verify' }}</span>
      </button>
      <button 
        @click="() => saveEdit()" 
        class="px-3 py-1.5 bg-blue-500 text-white text-sm rounded hover:bg-blue-600 transition-colors duration-150 focus:outline-none focus:ring-1 focus:ring-blue-500 flex items-center space-x-1.5"
        :disabled="saving"
      >
        <span v-if="saving" class="w-3 h-3 border-1.5 border-white border-t-transparent rounded-full animate-spin"></span>
        <span>{{ saving ? 'Saving...' : 'Save' }}</span>
      </button>
    </div>
    <div v-if="saveError" class="text-xs text-red-500 mt-2 px-4 mb-3">{{ saveError }}</div>
  </div>

  <!-- Special layout for projects -->
  <div v-else-if="props.item && props.item.type === 'projects' && detail && detail.raw" class="flex h-full">
    <div class="w-1/2 h-full">
       <MonacoEditor :value="detail.raw" :language="props.item.type === 'rulesets' ? 'xml' : (props.item.type === 'plugins' ? 'go' : 'yaml')" :read-only="true" class="h-full" />
    </div>
    <div class="w-1/2 h-full border-l border-gray-200">
      <ProjectWorkflow :projectContent="detail.raw" />
    </div>
  </div>

  <!-- Default layout for other components -->
  <div v-else-if="detail && detail.raw" class="h-full flex flex-col">
    <div class="flex justify-between px-4 py-2 bg-gray-50 border-b">
      <div class="flex items-center">
        <span v-if="detail.isTemporary" class="text-xs bg-blue-100 text-blue-800 px-2 py-1 rounded-md mr-2">
          Temporary Version
        </span>
        <span v-if="isPlugin && detail.type === 'local'" class="text-xs bg-gray-100 text-gray-800 px-2 py-1 rounded-md mr-2">
          Built-in Plugin
        </span>
        
        <!-- Project control buttons -->
        <div v-if="isProject && !detail.isTemporary" class="flex space-x-2">
          <button 
            v-if="detail.status === 'stopped'"
            @click="startProject"
            class="px-3 py-1.5 bg-green-500 text-white text-sm rounded hover:bg-green-600 transition-colors duration-150 focus:outline-none focus:ring-1 focus:ring-green-500 flex items-center space-x-1.5"
            :disabled="projectOperationLoading"
          >
            <span v-if="projectOperationLoading" class="w-3 h-3 border-1.5 border-white border-t-transparent rounded-full animate-spin"></span>
            <svg v-else class="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
            <span>Start</span>
          </button>
          
          <button 
            v-if="detail.status === 'running'"
            @click="stopProject"
            class="px-3 py-1.5 bg-red-500 text-white text-sm rounded hover:bg-red-600 transition-colors duration-150 focus:outline-none focus:ring-1 focus:ring-red-500 flex items-center space-x-1.5"
            :disabled="projectOperationLoading"
          >
            <span v-if="projectOperationLoading" class="w-3 h-3 border-1.5 border-white border-t-transparent rounded-full animate-spin"></span>
            <svg v-else class="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 10a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1h-4a1 1 0 01-1-1v-4z" />
            </svg>
            <span>Stop</span>
          </button>
          
          <button 
            v-if="detail.status === 'running'"
            @click="restartProject"
            class="px-3 py-1.5 bg-yellow-500 text-white text-sm rounded hover:bg-yellow-600 transition-colors duration-150 focus:outline-none focus:ring-1 focus:ring-yellow-500 flex items-center space-x-1.5"
            :disabled="projectOperationLoading"
          >
            <span v-if="projectOperationLoading" class="w-3 h-3 border-1.5 border-white border-t-transparent rounded-full animate-spin"></span>
            <svg v-else class="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
            <span>Restart</span>
          </button>
        </div>
        
        <!-- Temporary project warning -->
        <div v-if="isProject && detail.isTemporary" class="flex items-center text-yellow-600">
          <svg class="w-4 h-4 mr-1" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
          </svg>
          <span class="text-xs">Project operations unavailable for temporary version</span>
        </div>
      </div>
      <div class="flex items-center">
        <!-- Refresh button -->
        <button 
          @click="refreshComponent"
          class="px-2 py-1 mr-2 text-gray-600 hover:text-gray-800 hover:bg-gray-100 rounded transition-colors duration-150 focus:outline-none"
          :disabled="loading"
          title="Refresh"
        >
          <svg class="w-4 h-4" :class="{ 'animate-spin': loading }" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
        </button>
        
        <div v-if="isRuleset || isOutput || isPlugin || isProject" class="flex">
          <button 
            v-if="isRuleset"
            @click="showTestModal = true"
            class="px-3 py-1.5 bg-indigo-500 text-white text-sm rounded hover:bg-indigo-600 transition-colors duration-150 focus:outline-none focus:ring-1 focus:ring-indigo-500 flex items-center space-x-1.5"
          >
            <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
            </svg>
            <span>Test Ruleset</span>
          </button>
          <button 
            v-if="isOutput"
            @click="showOutputTestModal = true"
            class="px-3 py-1.5 bg-indigo-500 text-white text-sm rounded hover:bg-indigo-600 transition-colors duration-150 focus:outline-none focus:ring-1 focus:ring-indigo-500 flex items-center space-x-1.5"
          >
            <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
            </svg>
            <span>Test Output</span>
          </button>
          <button 
            v-if="isPlugin"
            @click="showPluginTestModal = true"
            class="px-3 py-1.5 bg-indigo-500 text-white text-sm rounded hover:bg-indigo-600 transition-colors duration-150 focus:outline-none focus:ring-1 focus:ring-indigo-500 flex items-center space-x-1.5"
          >
            <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
            </svg>
            <span>Test Plugin</span>
          </button>
          <button 
            v-if="isProject"
            @click="verifyProject"
            class="px-3 py-1.5 bg-green-500 text-white text-sm rounded hover:bg-green-600 transition-colors duration-150 focus:outline-none focus:ring-1 focus:ring-green-500 flex items-center space-x-1.5 mr-2"
            :disabled="verifyLoading"
          >
            <span v-if="verifyLoading" class="w-3 h-3 border-1.5 border-white border-t-transparent rounded-full animate-spin"></span>
            <svg v-else xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
            <span>{{ verifyLoading ? 'Verifying...' : 'Verify' }}</span>
          </button>
          <button 
            v-if="isProject"
            @click="showProjectTestModal = true"
            class="px-3 py-1.5 bg-indigo-500 text-white text-sm rounded hover:bg-indigo-600 transition-colors duration-150 focus:outline-none focus:ring-1 focus:ring-indigo-500 flex items-center space-x-1.5"
          >
            <svg xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
            </svg>
            <span>Test Project</span>
          </button>
        </div>
      </div>
    </div>
    <MonacoEditor :value="detail.raw" :language="props.item.type === 'rulesets' ? 'xml' : (props.item.type === 'plugins' ? 'go' : 'yaml')" :read-only="true" class="flex-1" />
  </div>

  <div v-else class="flex items-center justify-center h-full text-gray-400 text-lg">
    No content available
  </div>

  <!-- Test Modal -->
  <RulesetTestModal 
    v-if="props.item && props.item.type === 'rulesets'" 
    :show="showTestModal" 
    :rulesetId="props.item?.originalId || props.item?.id" 
    :rulesetContent="props.item?.isEdit ? editorValue : null"
    @close="showTestModal = false" 
  />
  <OutputTestModal
    v-if="props.item && props.item.type === 'outputs'"
    :show="showOutputTestModal"
    :outputId="props.item?.id"
    @close="showOutputTestModal = false"
  />
  <PluginTestModal
    v-if="props.item && props.item.type === 'plugins'"
    :show="showPluginTestModal"
    :pluginId="props.item?.id"
    @close="showPluginTestModal = false"
  />
  <ProjectTestModal
    v-if="props.item && props.item.type === 'projects'"
    :show="showProjectTestModal"
    :projectId="props.item?.id"
    @close="showProjectTestModal = false"
  />

  <!-- Project Operation Warning Modal -->
  <div v-if="projectWarningModal" class="fixed inset-0 bg-black bg-opacity-30 flex items-center justify-center z-50">
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
</template>

<script setup>
import { ref, watch, inject, computed, onMounted, onBeforeUnmount } from 'vue'
import { hubApi, verifyComponent } from '../api'
import MonacoEditor from '@/components/MonacoEditor.vue'
import ProjectWorkflow from './Visualization/ProjectWorkflow.vue'
import RulesetTestModal from './RulesetTestModal.vue'
import OutputTestModal from './OutputTestModal.vue'
import PluginTestModal from './PluginTestModal.vue'
import ProjectTestModal from './ProjectTestModal.vue'
import { useStore } from 'vuex'
import { useRouter } from 'vue-router'
import { validateRulesetXml } from '../utils/rulesetValidator'
import { getDefaultTemplate } from '../utils/templateGenerator'

// Props
const props = defineProps({
  item: Object
})

// Emits
const emit = defineEmits(['created', 'updated', 'cancel-edit'])

// Reactive state
const loading = ref(false)
const error = ref(null)
const detail = ref(null)
const editorValue = ref('')
const saveError = ref('')
const saving = ref(false)
const originalContent = ref('') // Save original content for restoring when canceling edit
const errorLines = ref([]) // Array of error lines
const preventRefetch = ref(false) // Flag to prevent unnecessary re-fetching
const validationResult = ref({
  isValid: true,
  errors: [],
  warnings: []
})
const verifyLoading = ref(false)
const showValidationPanel = ref(true) // Show validation panel by default when there are errors/warnings
const isRuleset = computed(() => {
  return props.item?.type === 'rulesets'
})
const isOutput = computed(() => {
  return props.item?.type === 'outputs'
})
const isPlugin = computed(() => {
  return props.item?.type === 'plugins'
})
const isProject = computed(() => {
  return props.item?.type === 'projects'
})

// Test modal state
const showTestModal = ref(false)
const showOutputTestModal = ref(false)
const showPluginTestModal = ref(false)
const showProjectTestModal = ref(false)

// Global message component
const $message = inject('$message', window?.$toast)
const store = useStore()
const router = useRouter()

// Project operation state
const projectOperationLoading = ref(false)
const projectWarningModal = ref(false)
const projectWarningMessage = ref('')
const projectOperationType = ref('') // 'start', 'stop', 'restart'

// Project status refresh
const statusRefreshInterval = ref(null)

// Watch for item changes
watch(
  () => props.item,
  (newVal, oldVal) => {
    console.log('Item watch triggered:', { newVal, oldVal, preventRefetch: preventRefetch.value })
    
    // Skip if we're preventing refetch (during save operations)
    if (preventRefetch.value) {
      console.log('Skipping refetch due to preventRefetch flag')
      return
    }
    
    if (!newVal) {
      detail.value = null;
      errorLines.value = [];
      return;
    }
    
    // Detect changes in timestamp or other properties
    const timestampChanged = newVal._timestamp !== oldVal?._timestamp;
    const typeChanged = newVal.type !== oldVal?.type;
    const idChanged = newVal.id !== oldVal?.id;
    const editModeChanged = newVal.isEdit !== oldVal?.isEdit;
    
    if (newVal && newVal.isNew) {
      detail.value = null;
      editorValue.value = getTemplateForComponent(newVal.type, newVal.id);
      errorLines.value = [];
    } else if (newVal && newVal.isEdit) {
      fetchDetail(newVal, true);
      errorLines.value = [];
    } else if (newVal && (typeChanged || idChanged || timestampChanged || editModeChanged)) {
      // If component ID, type, timestamp or edit mode changes, refresh details
      fetchDetail(newVal);
      errorLines.value = [];
    }
  },
  { immediate: true, deep: true }
)

// Extract line number from error message
function extractLineNumber(errorMessage) {
  if (!errorMessage) return null;
  
  // Try to extract line number from error message
  const lineMatches = errorMessage.match(/line\s*(\d+)/i) || 
                      errorMessage.match(/line:\s*(\d+)/i) ||
                      errorMessage.match(/location:.*line\s*(\d+)/i);
  
  if (lineMatches && lineMatches[1]) {
    return parseInt(lineMatches[1]);
  }
  
  return null;
}

// Methods
async function fetchDetail(item, forEdit = false) {
  console.log('fetchDetail called with:', { item, forEdit, itemId: item?.id, itemType: item?.type })
  
  detail.value = null
  error.value = null
  if (!item || !item.id) {
    console.log('fetchDetail: No item or item.id provided', item);
    return;
  }
  loading.value = true
  try {
    let data
    let tempInfo = null
    
    console.log(`Fetching ${item.type} ${item.id}, forEdit: ${forEdit}`)
    
    // If in edit mode, check for temporary file
    if (forEdit) {
      tempInfo = await hubApi.checkTemporaryFile(item.type, item.id);
      console.log('Temporary file info:', tempInfo)
      
      // Don't automatically create temporary file - let the save operation handle it
      // This prevents creating unnecessary .new files when content is identical
    }
    
    // Get details based on component type
    switch (item.type) {
      case 'inputs':
        data = await hubApi.getInput(item.id);
        break
      case 'outputs':
        data = await hubApi.getOutput(item.id);
        break
      case 'rulesets':
        data = await hubApi.getRuleset(item.id);
        break
      case 'projects':
        data = await hubApi.getProject(item.id);
        // Get project status
        try {
          const clusterStatus = await hubApi.fetchClusterStatus();
          if (clusterStatus && clusterStatus.projects) {
            const projectStatus = clusterStatus.projects.find(p => p.id === item.id);
            if (projectStatus) {
              data.status = projectStatus.status || 'stopped';
            } else {
              data.status = 'stopped'; // Default to stopped state
            }
          }
        } catch (statusError) {
          console.error('Failed to fetch project status:', statusError);
          data.status = 'unknown';
        }
        break
      case 'plugins':
        data = await hubApi.getPlugin(item.id);
        break
      default:
        throw new Error(`Unsupported component type: ${item.type}`);
    }
    
    console.log('Fetched data:', { 
      hasData: !!data, 
      hasRaw: !!data?.raw, 
      rawLength: data?.raw?.length || 0,
      path: data?.path,
      isTemporary: data?.path?.endsWith('.new')
    })
    
    // Check if this is a temporary file
    if (data && data.path) {
      data.isTemporary = data.path.endsWith('.new');
    }
    
    // Ensure we have content
    if (!data || (!data.raw && data.raw !== '')) {
      console.warn(`No content received for ${item.type} ${item.id}:`, data);
      // Try to fetch again without temporary file logic
      if (forEdit && tempInfo && tempInfo.hasTemp) {
        console.log('Retrying fetch without temporary file logic...');
        return await fetchDetail(item, false);
      }
    }
    
    detail.value = data;
    
    if (forEdit) {
      editorValue.value = data.raw || '';
      originalContent.value = data.raw || '';
      console.log('Set editor values:', { 
        editorValueLength: editorValue.value.length,
        originalContentLength: originalContent.value.length 
      })
    }
    
    // 如果是ruleset，验证XML
    if (item.type === 'rulesets' && data.raw) {
      validationResult.value = validateRulesetXml(data.raw);
      if (!validationResult.value.isValid) {
        // 提取错误行号
        errorLines.value = validationResult.value.errors.map(err => extractLineNumber(err)).filter(Boolean);
        // Show validation panel when there are errors/warnings
        showValidationPanel.value = true;
      } else {
        errorLines.value = [];
        // Hide validation panel when there are no errors/warnings
        showValidationPanel.value = false;
      }
    }
    
    console.log('fetchDetail completed successfully:', {
      hasDetail: !!detail.value,
      hasRaw: !!detail.value?.raw,
      editorValueLength: editorValue.value.length
    })
  } catch (e) {
    error.value = `Failed to load ${item.type}: ${e.message || 'Unknown error'}`;
    console.error(`Error fetching ${item.type} detail:`, e);
  } finally {
    loading.value = false;
  }
}

// Add validation function
const validateRuleset = () => {
  if (isRuleset.value && editorValue.value) {
    const result = validateRulesetXml(editorValue.value)
    validationResult.value = result
    
    // Update error line highlights
    errorLines.value = result.errors.map(error => error.line)
    
    // Show/hide validation panel based on results
    if (result.errors.length > 0 || result.warnings.length > 0) {
      showValidationPanel.value = true;
    } else {
      showValidationPanel.value = false;
    }
    
    return result.isValid
  }
  return true
}

// Watch for changes in editor content and perform real-time validation
watch(editorValue, (newContent) => {
  if (isRuleset.value && newContent) {
    validateRuleset()
  }
}, { deep: true })

// Add a method to manually clear error lines (for debugging)
function clearErrorLines() {
  errorLines.value = []
  console.log('Error lines cleared manually')
}

// Expose clearErrorLines to window for debugging
if (typeof window !== 'undefined') {
  window.clearErrorLines = clearErrorLines
}

// Verify project function
async function verifyProject() {
  if (!isProject.value) return;
  
  verifyLoading.value = true;
  
  try {
    const contentToVerify = props.item?.isEdit ? editorValue.value : detail.value?.raw;
    
    if (!contentToVerify) {
      $message?.warning?.('No content to verify');
      return;
    }
    
    const response = await hubApi.verifyComponent(props.item.type, props.item.id, contentToVerify);
    
    if (response.data && response.data.valid) {
      $message?.success?.('Project configuration is valid');
    } else {
      const errorMessage = response.data?.error || 'Unknown verification error';
      $message?.error?.('Verification failed: ' + errorMessage);
    }
  } catch (error) {
    const errorMessage = error.response?.data?.error || error.message || 'Unknown verification error';
    $message?.error?.('Verification error: ' + errorMessage);
  } finally {
    verifyLoading.value = false;
  }
}

// Perform initial validation when component is mounted
onMounted(() => {
  if (isRuleset.value && editorValue.value) {
    validateRuleset()
  }
  
  // If component type is project, fetch all components list
  if (props.item && props.item.type === 'projects') {
    store.dispatch('fetchAllComponents')
  }
})

async function saveEdit(content) {
  // If called directly from MonacoEditor's @save event, content will have a value
  // If called from button click, content will be undefined
  const contentToSave = content !== undefined ? content : editorValue.value
  
  // Preserve the current item reference
  const currentItem = props.item
  if (!currentItem || !currentItem.id) {
    console.error('saveEdit: No valid item to save', currentItem)
    saveError.value = 'Invalid item to save'
    return
  }
  
  // Validate ruleset using XML validator
  if (isRuleset.value) {
    const isValid = validateRuleset()
    if (!isValid && !confirm('Ruleset contains validation errors. Save anyway?')) {
      return
    }
  }
  
  saveError.value = ''
  saving.value = true
  
  try {
    // Set flag to prevent unnecessary re-fetching during save
    preventRefetch.value = true
    
    // Save component directly - the backend will handle whether to create .new file or not
    // based on content comparison
    
    // Pre-save verification for all component types
    try {
      const verifyRes = await hubApi.verifyComponent(currentItem.type, currentItem.id, contentToSave)

      // If verification failed, ask user if they want to proceed
      if (verifyRes.data && !verifyRes.data.valid) {
        const errorMessage = verifyRes.data?.error || 'Unknown verification error'
        if (!confirm(`Verification failed: ${errorMessage}\n\nSave anyway?`)) {
          saving.value = false
          return
        }
      }
    } catch (verifyErr) {
      const errorMessage = verifyErr.response?.data?.error || verifyErr.message || 'Unknown verification error'
      if (!confirm(`Verification error: ${errorMessage}\n\nSave anyway?`)) {
        saving.value = false
        return
      }
    }
    
    // Save component
    const response = await hubApi.saveEdit(currentItem.type, currentItem.id, contentToSave)
    
    // Add a small delay to ensure backend has processed the save
    await new Promise(resolve => setTimeout(resolve, 200))
    
    // Force refresh by clearing current detail first
    detail.value = null
    editorValue.value = ''
    
    // Refresh component content after successful save but stay in edit mode
    await fetchDetail(currentItem, true)
    
    // If still no content, try fetching the original file
    if (!detail.value || !detail.value.raw) {
      console.log('No content after edit mode fetch, trying view mode...')
      await fetchDetail(currentItem, false)
      if (detail.value && detail.value.raw) {
        editorValue.value = detail.value.raw
        originalContent.value = detail.value.raw
      }
    }
    
    // Post-save verification
    try {
      const verifyRes = await hubApi.verifyComponent(currentItem.type, currentItem.id)
      if (verifyRes.data && verifyRes.data.valid) {
        $message?.success?.('Saved and verified successfully')
      } else {
        const errorMessage = verifyRes.data?.error || 'Unknown verification error'
        $message?.warning?.('Saved but verification failed: ' + errorMessage)
        
        // Extract line number from error message
        const lineNumber = extractLineNumber(errorMessage)
        if (lineNumber) {
          errorLines.value = [lineNumber]
        }
      }
    } catch (verifyErr) {
      const errorMessage = verifyErr.response?.data?.error || verifyErr.message || 'Unknown verification error'
      $message?.warning?.('Saved but verification failed: ' + errorMessage)
      
      const lineNumber = extractLineNumber(errorMessage)
      if (lineNumber) {
        errorLines.value = [lineNumber]
      }
    }
    
    // Update component list (but don't emit immediately to avoid re-render issues)
    setTimeout(() => {
      emit('updated', currentItem)
      // Clear the prevent refetch flag after a delay
      setTimeout(() => {
        preventRefetch.value = false
        console.log('Re-enabled refetch after save completion')
      }, 500)
    }, 100)
  } catch (err) {
    saveError.value = err.response?.data?.error || err.message || 'Failed to save'
    $message?.error?.('Error: ' + saveError.value)
  } finally {
    saving.value = false
    // Don't clear the flag here, let the timeout handle it
  }
}

async function saveNew(content) {
  // If called directly from MonacoEditor's @save event, content will have a value
  // If called from button click, content will be undefined
  const contentToSave = content !== undefined ? content : editorValue.value
  
  // Preserve the current item reference
  const currentItem = props.item
  if (!currentItem || !currentItem.id) {
    console.error('saveNew: No valid item to save', currentItem)
    saveError.value = 'Invalid item to save'
    return
  }
  
  // Validate ruleset using XML validator
  if (isRuleset.value) {
    const isValid = validateRuleset()
    if (!isValid && !confirm('Ruleset contains validation errors. Create anyway?')) {
      return
    }
  }
  
  saveError.value = ''
  saving.value = true
  
  try {
    // Set flag to prevent unnecessary re-fetching during save
    preventRefetch.value = true
    
    // Pre-save verification for all component types
    try {
      const verifyRes = await hubApi.verifyComponent(currentItem.type, currentItem.id, contentToSave)

      // If verification failed, ask user if they want to proceed
      if (verifyRes.data && !verifyRes.data.valid) {
        const errorMessage = verifyRes.data?.error || 'Unknown verification error'
        if (!confirm(`Verification failed: ${errorMessage}\n\nCreate anyway?`)) {
          saving.value = false
          return
        }
      }
    } catch (verifyErr) {
      const errorMessage = verifyErr.response?.data?.error || verifyErr.message || 'Unknown verification error'
      if (!confirm(`Verification error: ${errorMessage}\n\nCreate anyway?`)) {
        saving.value = false
        return
      }
    }
    
    // Save new component
    const response = await hubApi.saveNew(currentItem.type, currentItem.id, contentToSave)
    
    // Add a small delay to ensure backend has processed the save
    await new Promise(resolve => setTimeout(resolve, 200))
    
    // Force refresh by clearing current detail first
    detail.value = null
    editorValue.value = ''
    
    // Refresh component content after successful save but stay in edit mode
    await fetchDetail(currentItem, true)
    
    // If still no content, try fetching the original file
    if (!detail.value || !detail.value.raw) {
      console.log('No content after edit mode fetch, trying view mode...')
      await fetchDetail(currentItem, false)
      if (detail.value && detail.value.raw) {
        editorValue.value = detail.value.raw
        originalContent.value = detail.value.raw
      }
    }
    
    // Post-save verification
    try {
      const verifyRes = await hubApi.verifyComponent(currentItem.type, currentItem.id)
      if (verifyRes.data && verifyRes.data.valid) {
        $message?.success?.('Created and verified successfully')
      } else {
        const errorMessage = verifyRes.data?.error || 'Unknown verification error'
        $message?.warning?.('Created but verification failed: ' + errorMessage)
        
        // Extract line number from error message
        const lineNumber = extractLineNumber(errorMessage)
        if (lineNumber) {
          errorLines.value = [lineNumber]
        }
      }
    } catch (verifyErr) {
      const errorMessage = verifyErr.response?.data?.error || verifyErr.message || 'Unknown verification error'
      $message?.warning?.('Created but verification failed: ' + errorMessage)
      
      const lineNumber = extractLineNumber(errorMessage)
      if (lineNumber) {
        errorLines.value = [lineNumber]
      }
    }
    
    // Notify parent component of successful creation
    setTimeout(() => {
      emit('created', currentItem)
      // Clear the prevent refetch flag after a delay
      setTimeout(() => {
        preventRefetch.value = false
        console.log('Re-enabled refetch after save completion')
      }, 500)
    }, 100)
  } catch (err) {
    saveError.value = err.response?.data?.error || err.message || 'Failed to create'
    $message?.error?.('Error: ' + saveError.value)
  } finally {
    saving.value = false
    // Don't clear the flag here, let the timeout handle it
  }
}

function cancelEdit() {
  // Restore original content
  editorValue.value = originalContent.value
  if (detail.value) detail.value.raw = originalContent.value
  // Clear error messages
  saveError.value = ''
  errorLines.value = [] // 清空错误行
  // Exit edit mode
  emit('cancel-edit', props.item)
}

function getLanguage(type) {
  switch (type) {
    case 'rulesets':
      return 'xml'
    case 'plugins':
      return 'go'
    case 'yaml':
      return 'yaml'
    default:
      return 'yaml'
  }
}

function getTemplateForComponent(type, id) {
  // 传递store参数，特别是对于项目类型
  return getDefaultTemplate(type, id, store);
}

// Project operations
async function startProject() {
  if (!props.item || !props.item.id) return
  
  projectOperationLoading.value = true
  
  try {
    const result = await hubApi.startProject(props.item.id)
    
    if (result.warning) {
      // 如果有警告（例如存在临时文件），显示警告模态框
      projectWarningMessage.value = result.message
      projectOperationType.value = 'start'
      projectWarningModal.value = true
    } else if (result.success) {
      // 成功启动项目
      $message?.success?.('Project started successfully')
      // 更新项目状态
      if (detail.value) {
        detail.value.status = 'running'
      }
    } else if (result.error) {
      // 启动失败
      $message?.error?.('Failed to start project: ' + result.error)
    }
  } catch (error) {
    $message?.error?.('Error starting project: ' + (error.message || 'Unknown error'))
  } finally {
    projectOperationLoading.value = false
  }
}

async function stopProject() {
  if (!props.item || !props.item.id) return
  
  projectOperationLoading.value = true
  
  try {
    const result = await hubApi.stopProject(props.item.id)
    
    if (result.warning) {
      // 如果有警告（例如存在临时文件），显示警告模态框
      projectWarningMessage.value = result.message
      projectOperationType.value = 'stop'
      projectWarningModal.value = true
    } else if (result.success) {
      // 成功停止项目
      $message?.success?.('Project stopped successfully')
      // 更新项目状态
      if (detail.value) {
        detail.value.status = 'stopped'
      }
    } else if (result.error) {
      // 停止失败
      $message?.error?.('Failed to stop project: ' + result.error)
    }
  } catch (error) {
    $message?.error?.('Error stopping project: ' + (error.message || 'Unknown error'))
  } finally {
    projectOperationLoading.value = false
  }
}

async function restartProject() {
  if (!props.item || !props.item.id) return
  
  projectOperationLoading.value = true
  
  try {
    const result = await hubApi.restartProject(props.item.id)
    
    if (result.warning) {
      // 如果有警告（例如存在临时文件），显示警告模态框
      projectWarningMessage.value = result.message
      projectOperationType.value = 'restart'
      projectWarningModal.value = true
    } else if (result.success) {
      // 成功重启项目
      $message?.success?.('Project restarted successfully')
      // 更新项目状态
      if (detail.value) {
        detail.value.status = 'running'
      }
    } else if (result.error) {
      // 重启失败
      $message?.error?.('Failed to restart project: ' + result.error)
    }
  } catch (error) {
    $message?.error?.('Error restarting project: ' + (error.message || 'Unknown error'))
  } finally {
    projectOperationLoading.value = false
  }
}

function closeProjectWarningModal() {
  projectWarningModal.value = false
}

function continueProjectOperation() {
  closeProjectWarningModal()
  
  if (!props.item || !props.item.id || !projectOperationType.value) return
  
  projectOperationLoading.value = true
  
  try {
    const id = props.item.id
    
    // 使用原始项目进行操作
    if (projectOperationType.value === 'start') {
      // 直接调用API启动项目
      hubApi.startProject(id)
        .then(result => {
          if (result.success) {
            $message?.success?.('Project started successfully')
            if (detail.value) {
              detail.value.status = 'running'
            }
          } else if (result.error) {
            $message?.error?.('Failed to start project: ' + result.error)
          }
        })
        .catch(error => {
          $message?.error?.('Failed to start project: ' + (error.message || 'Unknown error'))
        })
        .finally(() => {
          projectOperationLoading.value = false
        })
    } else if (projectOperationType.value === 'stop') {
      // 直接调用API停止项目
      hubApi.stopProject(id)
        .then(result => {
          if (result.success) {
            $message?.success?.('Project stopped successfully')
            if (detail.value) {
              detail.value.status = 'stopped'
            }
          } else if (result.error) {
            $message?.error?.('Failed to stop project: ' + result.error)
          }
        })
        .catch(error => {
          $message?.error?.('Failed to stop project: ' + (error.message || 'Unknown error'))
        })
        .finally(() => {
          projectOperationLoading.value = false
        })
    } else if (projectOperationType.value === 'restart') {
      // 先停止，再启动
      hubApi.restartProject(id)
        .then(result => {
          if (result.success) {
            $message?.success?.('Project restarted successfully')
            if (detail.value) {
              detail.value.status = 'running'
            }
          } else if (result.error) {
            $message?.error?.('Failed to restart project: ' + result.error)
          }
        })
        .catch(error => {
          $message?.error?.('Failed to restart project: ' + (error.message || 'Unknown error'))
        })
        .finally(() => {
          projectOperationLoading.value = false
        })
    }
  } catch (error) {
    $message?.error?.('Error with project operation: ' + (error.message || 'Unknown error'))
    projectOperationLoading.value = false
  }
}

// 设置定时刷新项目状态
function setupStatusRefresh() {
  if (isProject.value && props.item && props.item.id && !statusRefreshInterval.value) {
    // 每5秒刷新一次项目状态
    statusRefreshInterval.value = setInterval(async () => {
      if (detail.value && !detail.value.isTemporary) {
        try {
          const clusterStatus = await hubApi.fetchClusterStatus();
          if (clusterStatus && clusterStatus.projects) {
            const projectStatus = clusterStatus.projects.find(p => p.id === props.item.id);
            if (projectStatus && detail.value) {
              detail.value.status = projectStatus.status || 'stopped';
            }
          }
        } catch (error) {
          console.error('Failed to refresh project status:', error);
        }
      }
    }, 5000);
  }
}

// 清除定时刷新
function clearStatusRefresh() {
  if (statusRefreshInterval.value) {
    clearInterval(statusRefreshInterval.value);
    statusRefreshInterval.value = null;
  }
}

// 监听项目类型变化，设置或清除定时刷新
watch(isProject, (newVal) => {
  if (newVal) {
    setupStatusRefresh();
  } else {
    clearStatusRefresh();
  }
});

// 监听项目ID变化，重置定时刷新
watch(() => props.item?.id, (newVal, oldVal) => {
  if (newVal !== oldVal) {
    clearStatusRefresh();
    if (isProject.value) {
      setupStatusRefresh();
    }
  }
});

// 组件卸载时清除定时刷新
onBeforeUnmount(() => {
  clearStatusRefresh();
});

// Refresh component details
async function refreshComponent() {
  if (!props.item || !props.item.id) return
  await fetchDetail(props.item)
  $message?.success?.('Component refreshed')
}
</script> 

<style scoped>

/* 验证错误和警告样式 */
.validation-errors, .validation-warnings {
  margin-bottom: 15px;
  padding: 10px;
  border-radius: 4px;
}

.validation-errors {
  background-color: rgba(244, 67, 54, 0.1);
  border-left: 4px solid #f44336;
}

.validation-warnings {
  background-color: rgba(255, 152, 0, 0.1);
  border-left: 4px solid #ff9800;
}

.validation-errors h3, .validation-warnings h3 {
  margin-top: 0;
  font-size: 16px;
  font-weight: bold;
}

</style> 