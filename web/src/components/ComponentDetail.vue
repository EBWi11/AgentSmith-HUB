<template>
  <div v-if="loading" class="flex items-center justify-center h-full text-gray-400 text-lg">Loading...</div>
  <div v-else-if="error" class="flex items-center justify-center h-full text-red-400 text-lg">{{ error }}</div>
  
  <!-- Create Mode -->
  <div v-else-if="props.item && props.item.isNew" class="h-full flex flex-col">
    <MonacoEditor v-model:value="editorValue" :language="props.item.type === 'rulesets' ? 'xml' : (props.item.type === 'plugins' ? 'go' : 'yaml')" :read-only="false" :error-lines="errorLines" class="flex-1" @save="saveNew" />
    <div class="flex justify-end mt-4 px-4 space-x-2 border-t pt-4 pb-3">
      <!-- Test Buttons -->
      <button 
        v-if="isRuleset"
        @click="showTestModal = true" 
        class="btn btn-test-ruleset btn-md"
      >
        <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
        </svg>
        Test Ruleset
      </button>
      <button 
        v-if="isProject"
        @click="showProjectTestModal = true" 
        class="btn btn-test-project btn-md"
      >
        <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
        </svg>
        Test Project
      </button>
      <button 
        v-if="isPlugin"
        @click="showPluginTestModal = true" 
        class="btn btn-test-plugin btn-md"
      >
        <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
        </svg>
        Test Plugin
      </button>
      
      <!-- Verify Buttons -->
      <button 
        v-if="isRuleset"
        @click="verifyRuleset" 
        class="btn btn-verify btn-md"
        :disabled="verifyLoading"
      >
        <span v-if="verifyLoading" class="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin mr-2"></span>
        <svg v-else class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        {{ verifyLoading ? 'Verifying...' : 'Verify' }}
      </button>
      <button 
        v-if="isOutput"
        @click="verifyOutput" 
        class="btn btn-verify btn-md"
        :disabled="verifyLoading"
      >
        <span v-if="verifyLoading" class="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin mr-2"></span>
        <svg v-else class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        {{ verifyLoading ? 'Verifying...' : 'Verify' }}
      </button>
      <button 
        v-if="isInput"
        @click="verifyInput" 
        class="btn btn-verify btn-md"
        :disabled="verifyLoading"
      >
        <span v-if="verifyLoading" class="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin mr-2"></span>
        <svg v-else class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        {{ verifyLoading ? 'Verifying...' : 'Verify' }}
      </button>
      
      <!-- Connect Check Button -->
      <button 
        v-if="supportsConnectCheck"
        @click="connectCheck" 
        class="btn btn-connect btn-md"
        :disabled="connectCheckLoading"
      >
        <span v-if="connectCheckLoading" class="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin mr-2"></span>
        <svg v-else class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" />
        </svg>
        {{ connectCheckLoading ? 'Checking...' : 'Connect Check' }}
      </button>
      
      <!-- Save Button -->
      <button 
        @click="saveNew" 
        class="btn btn-primary btn-md"
        :disabled="saving"
      >
        <span v-if="saving" class="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin mr-2"></span>
        <svg v-else class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7H5a2 2 0 00-2 2v9a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-3m-1 4l-3 3m0 0l-3-3m3 3V4" />
        </svg>
        {{ saving ? 'Saving...' : 'Create' }}
      </button>
    </div>
    <div v-if="saveError" class="text-xs text-red-500 mt-2">{{ saveError }}</div>
  </div>

  <!-- Edit Mode -->
  <div v-else-if="props.item && props.item.isEdit && detail" class="h-full flex flex-col relative">
    <!-- Floating Validation Status (for Rulesets, Plugins, Outputs, and Inputs) -->
    <div v-if="(isRuleset || isPlugin || isOutput || isInput) && (validationResult.errors.length > 0 || validationResult.warnings.length > 0) && showValidationPanel" 
         class="absolute top-4 right-4 z-50 max-w-md bg-white/95 border border-gray-200/60 rounded-xl shadow-2xl backdrop-blur-md">
      <!-- Validation Errors -->
      <div v-if="validationResult.errors.length > 0" class="validation-errors p-4 bg-red-50/60 border-l-4 border-red-400/70 text-red-800 rounded-t-xl backdrop-blur-sm">
        <div class="flex justify-between items-start mb-3">
          <h3 class="font-semibold text-sm text-red-900">{{ isPlugin ? 'Compilation' : (isOutput ? 'Output Validation' : (isInput ? 'Input Validation' : 'Validation')) }} Errors</h3>
          <button @click="showValidationPanel = false" class="text-red-400 hover:text-red-600 ml-2 transition-colors duration-150">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
            </svg>
          </button>
        </div>
        <ul class="text-xs space-y-1">
          <li v-for="(error, index) in validationResult.errors" :key="index" class="flex flex-col">
            <span class="font-medium text-red-900">Line {{ error.line }}:</span> 
            <span class="text-red-700 ml-1">{{ error.message }}</span>
            <span v-if="error.detail" class="text-red-600 text-xs mt-1 ml-4 italic opacity-80">{{ error.detail }}</span>
          </li>
        </ul>
      </div>

      <!-- Validation Warnings -->
      <div v-if="validationResult.warnings.length > 0" 
           class="validation-warnings p-4 bg-amber-50/60 border-l-4 border-amber-400/70 text-amber-800 backdrop-blur-sm"
           :class="{ 'rounded-t-xl': validationResult.errors.length === 0, 'rounded-b-xl': true }">
        <div v-if="validationResult.errors.length === 0" class="flex justify-between items-start mb-3">
          <h3 class="font-semibold text-sm text-amber-900">{{ isPlugin ? 'Compilation' : (isOutput ? 'Output Validation' : (isInput ? 'Input Validation' : 'Validation')) }} Warnings</h3>
          <button @click="showValidationPanel = false" class="text-amber-400 hover:text-amber-600 ml-2 transition-colors duration-150">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
            </svg>
          </button>
        </div>
        <h3 v-else class="font-semibold text-sm mb-3 text-amber-900">{{ isPlugin ? 'Compilation' : (isOutput ? 'Output Validation' : (isInput ? 'Input Validation' : 'Validation')) }} Warnings</h3>
        <ul class="text-xs space-y-1">
          <li v-for="(warning, index) in validationResult.warnings" :key="index" class="flex flex-col">
            <span class="font-medium text-amber-900">Line {{ warning.line }}:</span> 
            <span class="text-amber-700 ml-1">{{ warning.message }}</span>
            <span v-if="warning.detail" class="text-amber-600 text-xs mt-1 ml-4 italic opacity-80">{{ warning.detail }}</span>
          </li>
        </ul>
      </div>
    </div>

    <!-- Validation Status Indicator -->
    <div v-if="(isRuleset || isPlugin || isOutput || isInput) && (validationResult.errors.length > 0 || validationResult.warnings.length > 0) && !showValidationPanel"
         class="absolute top-4 right-4 z-50">
      <button @click="showValidationPanel = true" 
              class="flex items-center space-x-1 px-2 py-1 rounded-full text-white text-xs shadow-lg transition-all hover:scale-105"
              :class="validationResult.errors.length > 0 ? 'bg-gradient-to-r from-red-500 to-red-600 hover:from-red-600 hover:to-red-700' : 'bg-gradient-to-r from-amber-500 to-orange-500 hover:from-amber-600 hover:to-orange-600'">
        <svg class="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
        </svg>
        <span>{{ validationResult.errors.length > 0 ? `${validationResult.errors.length} Error${validationResult.errors.length > 1 ? 's' : ''}` : `${validationResult.warnings.length} Warning${validationResult.warnings.length > 1 ? 's' : ''}` }}</span>
      </button>
    </div>
    
    <MonacoEditor v-model:value="editorValue" :language="props.item.type === 'rulesets' ? 'xml' : (props.item.type === 'plugins' ? 'go' : 'yaml')" :read-only="false" :error-lines="errorLines" class="flex-1" @save="saveEdit" />
    <div class="flex justify-end mt-4 px-4 space-x-2 border-t pt-4 pb-3">
      <!-- Cancel Button -->
      <button 
        @click="cancelEdit" 
        class="btn btn-secondary btn-md"
      >
        <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
        </svg>
        Cancel
      </button>
      
      <!-- Test Buttons -->
      <button 
        v-if="isRuleset"
        @click="showTestModal = true" 
        class="btn btn-test-ruleset btn-md"
      >
        <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
        </svg>
        Test Ruleset
      </button>
      <button 
        v-if="isProject"
        @click="showProjectTestModal = true" 
        class="btn btn-test-project btn-md"
      >
        <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
        </svg>
        Test Project
      </button>
      <button 
        v-if="isPlugin"
        @click="showPluginTestModal = true" 
        class="btn btn-test-plugin btn-md"
      >
        <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
        </svg>
        Test Plugin
      </button>
      
      <!-- Verify Buttons -->
      <button 
        v-if="isRuleset"
        @click="verifyRuleset" 
        class="btn btn-verify btn-md"
        :disabled="verifyLoading"
      >
        <span v-if="verifyLoading" class="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin mr-2"></span>
        <svg v-else class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        {{ verifyLoading ? 'Verifying...' : 'Verify' }}
      </button>
      <button 
        v-if="isProject"
        @click="verifyProject" 
        class="btn btn-verify btn-md"
        :disabled="verifyLoading"
      >
        <span v-if="verifyLoading" class="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin mr-2"></span>
        <svg v-else class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        {{ verifyLoading ? 'Verifying...' : 'Verify' }}
      </button>
      <button 
        v-if="isPlugin"
        @click="verifyPlugin" 
        class="btn btn-verify btn-md"
        :disabled="verifyLoading"
      >
        <span v-if="verifyLoading" class="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin mr-2"></span>
        <svg v-else class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        {{ verifyLoading ? 'Verifying...' : 'Verify' }}
      </button>
      <button 
        v-if="isOutput"
        @click="verifyOutput" 
        class="btn btn-verify btn-md"
        :disabled="verifyLoading"
      >
        <span v-if="verifyLoading" class="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin mr-2"></span>
        <svg v-else class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        {{ verifyLoading ? 'Verifying...' : 'Verify' }}
      </button>
      <button 
        v-if="isInput"
        @click="verifyInput" 
        class="btn btn-verify btn-md"
        :disabled="verifyLoading"
      >
        <span v-if="verifyLoading" class="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin mr-2"></span>
        <svg v-else class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        {{ verifyLoading ? 'Verifying...' : 'Verify' }}
      </button>
      
      <!-- Connect Check Button -->
      <button 
        v-if="supportsConnectCheck"
        @click="connectCheck" 
        class="btn btn-connect btn-md"
        :disabled="connectCheckLoading"
      >
        <span v-if="connectCheckLoading" class="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin mr-2"></span>
        <svg v-else class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" />
        </svg>
        {{ connectCheckLoading ? 'Checking...' : 'Connect Check' }}
      </button>
      
      <!-- Save Button -->
      <button 
        @click="saveEdit" 
        class="btn btn-primary btn-md"
        :disabled="saving"
      >
        <span v-if="saving" class="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin mr-2"></span>
        <svg v-else class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7H5a2 2 0 00-2 2v9a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-3m-1 4l-3 3m0 0l-3-3m3 3V4" />
        </svg>
        {{ saving ? 'Saving...' : 'Update' }}
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
            class="btn btn-start btn-sm"
            :disabled="projectOperationLoading"
          >
            <span v-if="projectOperationLoading" class="w-3 h-3 border-1.5 border-current border-t-transparent rounded-full animate-spin mr-1"></span>
            <svg v-else class="w-4 h-4 mr-1.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
            Start Project
          </button>
          
          <button 
            v-if="detail.status === 'running'"
            @click="stopProject"
            class="btn btn-stop btn-sm"
            :disabled="projectOperationLoading"
          >
            <span v-if="projectOperationLoading" class="w-3 h-3 border-1.5 border-current border-t-transparent rounded-full animate-spin mr-1"></span>
            <svg v-else class="w-4 h-4 mr-1.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 10a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1h-4a1 1 0 01-1-1v-4z" />
            </svg>
            Stop Project
          </button>
          
          <button 
            v-if="detail.status === 'running'"
            @click="restartProject"
            class="btn btn-restart btn-sm"
            :disabled="projectOperationLoading"
          >
            <span v-if="projectOperationLoading" class="w-3 h-3 border-1.5 border-current border-t-transparent rounded-full animate-spin mr-1"></span>
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
        <!-- Keep only Verify button for projects -->
        <div v-if="isProject" class="flex">
          <button 
            @click="verifyProject"
            class="btn btn-verify btn-sm"
            :disabled="verifyLoading"
          >
            <span v-if="verifyLoading" class="w-3 h-3 border-1.5 border-current border-t-transparent rounded-full animate-spin mr-1"></span>
            <svg v-else xmlns="http://www.w3.org/2000/svg" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
            <span>{{ verifyLoading ? 'Verifying...' : 'Verify' }}</span>
          </button>
        </div>
      </div>
    </div>
    <MonacoEditor :value="detail.raw" :language="props.item.type === 'rulesets' ? 'xml' : (props.item.type === 'plugins' ? 'go' : 'yaml')" :read-only="true" class="flex-1" />
  </div>

  <!-- Test Modal -->
  <RulesetTestModal 
    v-if="props.item && props.item.type === 'rulesets'" 
    :show="showTestModal" 
    :rulesetId="props.item?.originalId || props.item?.id" 
    :rulesetContent="props.item?.isEdit ? editorValue : null"
    @close="showTestModal = false" 
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
        <button @click="closeProjectWarningModal" class="btn btn-secondary btn-sm">
          Cancel
        </button>
        <button @click="continueProjectOperation" class="btn btn-warning btn-sm" :disabled="projectOperationLoading">
          <span v-if="projectOperationLoading" class="w-3 h-3 border-1.5 border-current border-t-transparent rounded-full animate-spin mr-1"></span>
          Continue Anyway
        </button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, watch, inject, computed, onMounted, onBeforeUnmount } from 'vue'
import { hubApi } from '../api'
import MonacoEditor from '@/components/MonacoEditor.vue'
import ProjectWorkflow from './Visualization/ProjectWorkflow.vue'
import RulesetTestModal from './RulesetTestModal.vue'
import PluginTestModal from './PluginTestModal.vue'
import ProjectTestModal from './ProjectTestModal.vue'
import { useStore } from 'vuex'
import { useRouter } from 'vue-router'

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
const connectCheckLoading = ref(false)
const showValidationPanel = ref(true) // Show validation panel by default when there are errors/warnings
const pluginVerifyTimeout = ref(null) // Timeout for plugin auto-verification
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
const isInput = computed(() => {
  return props.item?.type === 'inputs'
})

// Check if component supports connect check (excludes print output)
const supportsConnectCheck = computed(() => {
  if (isInput.value) {
    return true // All input types support connect check
  }
  if (isOutput.value && detail.value?.raw) {
    // Parse output config to check if it's print type
    try {
      const yamlContent = detail.value.raw
      // Simple check for print type in YAML
      if (yamlContent.includes('type: print') || yamlContent.includes('type: "print"') || yamlContent.includes("type: 'print'")) {
        return false // Print output doesn't need connect check
      }
      return true // Other output types support connect check
    } catch (e) {
      return true // Default to supporting connect check if parse fails
    }
  }
  return false
})

// Test modal state
const showTestModal = ref(false)
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
      validationResult.value = { isValid: true, errors: [], warnings: [] };
      showValidationPanel.value = false;
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
      validationResult.value = { isValid: true, errors: [], warnings: [] };
      showValidationPanel.value = false;
    } else if (newVal && newVal.isEdit) {
      fetchDetail(newVal, true);
      errorLines.value = [];
      validationResult.value = { isValid: true, errors: [], warnings: [] };
      showValidationPanel.value = false;
    } else if (newVal && (typeChanged || idChanged || timestampChanged || editModeChanged)) {
      // If component ID, type, timestamp or edit mode changes, refresh details
      fetchDetail(newVal);
      errorLines.value = [];
      validationResult.value = { isValid: true, errors: [], warnings: [] };
      showValidationPanel.value = false;
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
    
    // 如果是ruleset，进行后端验证（初始加载时静默验证）
    if (item.type === 'rulesets' && data.raw) {
      try {
        const response = await hubApi.verifyComponent(item.type, item.id, data.raw);
        if (response.data && response.data.errors && Array.isArray(response.data.errors)) {
          // Backend returned structured validation result
          validationResult.value = {
            isValid: response.data.valid || false,
            errors: response.data.errors || [],
            warnings: response.data.warnings || []
          };
          
          // Extract error lines for highlighting
          errorLines.value = response.data.errors.map(err => err.line).filter(Boolean);
          
          // Show validation panel if there are errors or warnings
          if (response.data.errors.length > 0 || (response.data.warnings && response.data.warnings.length > 0)) {
            showValidationPanel.value = true;
          } else {
            showValidationPanel.value = false;
          }
        } else {
          // Clear validation if no structured response
          validationResult.value = { isValid: true, errors: [], warnings: [] };
          errorLines.value = [];
          showValidationPanel.value = false;
        }
      } catch (verifyError) {
        console.warn('Initial ruleset verification failed:', verifyError);
        // Don't show errors on initial load, just clear validation
        validationResult.value = { isValid: true, errors: [], warnings: [] };
        errorLines.value = [];
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

// Real-time validation function (no messages, silent)
const validateRulesetRealtime = async () => {
  if (isRuleset.value && editorValue.value && props.item?.id) {
    try {
      const response = await hubApi.verifyComponent(props.item.type, props.item.id, editorValue.value);
      
      if (response.data && response.data.errors && Array.isArray(response.data.errors)) {
        // Backend returned structured validation result
        validationResult.value = {
          isValid: response.data.valid || false,
          errors: response.data.errors || [],
          warnings: response.data.warnings || []
        };
        
        // Update error line highlights
        errorLines.value = response.data.errors.map(error => error.line).filter(Boolean);
        
        // Show/hide validation panel based on results
        if (response.data.errors.length > 0 || (response.data.warnings && response.data.warnings.length > 0)) {
          showValidationPanel.value = true;
        } else {
          showValidationPanel.value = false;
        }
        

        return response.data.valid || false;
      } else if (response.data && response.data.hasOwnProperty('valid')) {
        // Handle simple valid/invalid response
        if (response.data.valid) {
          validationResult.value = { isValid: true, errors: [], warnings: [] };
          errorLines.value = [];
          showValidationPanel.value = false;
        } else {
          // For invalid responses without detailed errors, show generic error
          const errorMessage = response.data.error || 'Validation failed';
          validationResult.value = {
            isValid: false,
            errors: [{ line: 'Unknown', message: errorMessage }],
            warnings: []
          };
          errorLines.value = [];
          showValidationPanel.value = true;
        }
        return response.data.valid;
      } else {
        // Clear validation
        validationResult.value = { isValid: true, errors: [], warnings: [] };
        errorLines.value = [];
        showValidationPanel.value = false;
        return true;
      }
    } catch (error) {
      // Clear validation errors when validation request fails
      validationResult.value = { isValid: true, errors: [], warnings: [] };
      errorLines.value = [];
      showValidationPanel.value = false;
      return true;
    }
  }
  return true;
}

// Watch for changes in editor content and perform real-time validation  
const rulesetValidationTimeout = ref(null);

watch(editorValue, (newContent) => {
  if (isRuleset.value && newContent) {
    // Debounce ruleset validation to avoid excessive API calls
    clearTimeout(rulesetValidationTimeout.value);
    rulesetValidationTimeout.value = setTimeout(async () => {
      await validateRulesetRealtime();
    }, 800); // Wait 800ms after user stops typing for faster feedback
  } else if (isPlugin.value && newContent && props.item?.isEdit) {
    // Auto-verify plugin code changes, but with debouncing to avoid excessive API calls
    clearTimeout(pluginVerifyTimeout.value)
    pluginVerifyTimeout.value = setTimeout(() => {
      autoVerifyPlugin()
    }, 2000) // Wait 2 seconds after user stops typing
  }
}, { deep: true })



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

// Verify plugin function
async function verifyPlugin() {
  if (!isPlugin.value) return;
  
  verifyLoading.value = true;
  
  try {
    const contentToVerify = props.item?.isEdit ? editorValue.value : detail.value?.raw;
    
    if (!contentToVerify) {
      $message?.warning?.('No content to verify');
      return;
    }
    
    const response = await hubApi.verifyComponent(props.item.type, props.item.id, contentToVerify);
    
    if (response.data && response.data.valid) {
      $message?.success?.('Plugin code is valid');
      // Clear validation errors on successful verification
      validationResult.value = {
        isValid: true,
        errors: [],
        warnings: []
      };
      errorLines.value = [];
    } else {
      const errorMessage = response.data?.error || 'Unknown verification error';
      $message?.error?.('Verification failed: ' + errorMessage);
      
      // Extract line number from error message for highlighting
      const lineNumber = extractLineNumber(errorMessage);
      if (lineNumber) {
        errorLines.value = [lineNumber];
        
        // Add to validation result for display in the panel
        validationResult.value = {
          isValid: false,
          errors: [{
            line: lineNumber,
            message: errorMessage,
            detail: response.data?.detail || null
          }],
          warnings: []
        };
      } else {
        // Add general error without line number
        validationResult.value = {
          isValid: false,
          errors: [{
            line: 'Unknown',
            message: errorMessage,
            detail: response.data?.detail || null
          }],
          warnings: []
        };
      }
      showValidationPanel.value = true;
    }
  } catch (error) {
    const errorMessage = error.response?.data?.error || error.message || 'Unknown verification error';
    $message?.error?.('Verification error: ' + errorMessage);
    
    // Extract line number from error message for highlighting
    const lineNumber = extractLineNumber(errorMessage);
    if (lineNumber) {
      errorLines.value = [lineNumber];
      
      // Add to validation result for display in the panel
      validationResult.value = {
        isValid: false,
        errors: [{
          line: lineNumber,
          message: errorMessage,
          detail: error.response?.data?.detail || null
        }],
        warnings: []
      };
    } else {
      // Add general error without line number
      validationResult.value = {
        isValid: false,
        errors: [{
          line: 'Unknown',
          message: errorMessage,
          detail: error.response?.data?.detail || null
        }],
        warnings: []
      };
    }
    showValidationPanel.value = true;
  } finally {
    verifyLoading.value = false;
  }
}

// Verify ruleset function
async function verifyRuleset() {
  if (!isRuleset.value) return;
  
  verifyLoading.value = true;
  
  try {
    const contentToVerify = props.item?.isEdit ? editorValue.value : detail.value?.raw;
    
    if (!contentToVerify) {
      $message?.warning?.('No content to verify');
      return;
    }
    
    const response = await hubApi.verifyComponent(props.item.type, props.item.id, contentToVerify);
    
    if (response.data && response.data.valid) {
      $message?.success?.('Ruleset XML is valid');
      // Clear validation errors on successful verification
      validationResult.value = {
        isValid: true,
        errors: [],
        warnings: []
      };
      errorLines.value = [];
      showValidationPanel.value = false;
    } else {
      // Handle structured response from backend
      if (response.data && response.data.errors && Array.isArray(response.data.errors)) {
        // Backend returned structured validation result
        validationResult.value = {
          isValid: response.data.valid || false,
          errors: response.data.errors || [],
          warnings: response.data.warnings || []
        };
        
        // Extract error lines for highlighting
        errorLines.value = response.data.errors.map(err => err.line).filter(Boolean);
        
        const errorCount = response.data.errors.length;
        const warningCount = (response.data.warnings || []).length;
        
        if (errorCount > 0) {
          $message?.error?.(`Verification failed: ${errorCount} error${errorCount > 1 ? 's' : ''} found`);
        } else if (warningCount > 0) {
          $message?.warning?.(`Verification completed with ${warningCount} warning${warningCount > 1 ? 's' : ''}`);
        }
        
        showValidationPanel.value = true;
      } else {
        // Fallback to old format
        const errorMessage = response.data?.error || 'Unknown verification error';
        $message?.error?.('Verification failed: ' + errorMessage);
        
        const lineNumber = extractLineNumber(errorMessage);
        errorLines.value = lineNumber ? [lineNumber] : [];
        
        validationResult.value = {
          isValid: false,
          errors: [{
            line: lineNumber || 'Unknown',
            message: errorMessage,
            detail: response.data?.detail || null
          }],
          warnings: []
        };
        showValidationPanel.value = true;
      }
    }
  } catch (error) {
    const errorMessage = error.response?.data?.error || error.message || 'Unknown verification error';
    $message?.error?.('Verification error: ' + errorMessage);
    
    const lineNumber = extractLineNumber(errorMessage);
    errorLines.value = lineNumber ? [lineNumber] : [];
    
    validationResult.value = {
      isValid: false,
      errors: [{
        line: lineNumber || 'Unknown',
        message: errorMessage,
        detail: error.response?.data?.detail || null
      }],
      warnings: []
    };
    showValidationPanel.value = true;
  } finally {
    verifyLoading.value = false;
  }
}

// Verify output function
async function verifyOutput() {
  if (!isOutput.value) return;
  
  verifyLoading.value = true;
  
  try {
    const contentToVerify = props.item?.isEdit ? editorValue.value : detail.value?.raw;
    
    if (!contentToVerify) {
      $message?.warning?.('No content to verify');
      return;
    }
    
    const response = await hubApi.verifyComponent(props.item.type, props.item.id, contentToVerify);
    
    if (response.data && response.data.valid) {
      $message?.success?.('Output configuration is valid');
      // Clear validation errors on successful verification
      validationResult.value = {
        isValid: true,
        errors: [],
        warnings: []
      };
      errorLines.value = [];
    } else {
      const errorMessage = response.data?.error || 'Unknown verification error';
      $message?.error?.('Verification failed: ' + errorMessage);
      
      // Extract line number from error message for highlighting
      const lineNumber = extractLineNumber(errorMessage);
      if (lineNumber) {
        errorLines.value = [lineNumber];
        
        // Add to validation result for display in the panel
        validationResult.value = {
          isValid: false,
          errors: [{
            line: lineNumber,
            message: errorMessage,
            detail: response.data?.detail || null
          }],
          warnings: []
        };
      } else {
        // Add general error without line number
        validationResult.value = {
          isValid: false,
          errors: [{
            line: 'Unknown',
            message: errorMessage,
            detail: response.data?.detail || null
          }],
          warnings: []
        };
      }
      showValidationPanel.value = true;
    }
  } catch (error) {
    const errorMessage = error.response?.data?.error || error.message || 'Unknown verification error';
    $message?.error?.('Verification error: ' + errorMessage);
    
    // Extract line number from error message for highlighting
    const lineNumber = extractLineNumber(errorMessage);
    if (lineNumber) {
      errorLines.value = [lineNumber];
      
      // Add to validation result for display in the panel
      validationResult.value = {
        isValid: false,
        errors: [{
          line: lineNumber,
          message: errorMessage,
          detail: error.response?.data?.detail || null
        }],
        warnings: []
      };
    } else {
      // Add general error without line number
      validationResult.value = {
        isValid: false,
        errors: [{
          line: 'Unknown',
          message: errorMessage,
          detail: error.response?.data?.detail || null
        }],
        warnings: []
      };
    }
    showValidationPanel.value = true;
  } finally {
    verifyLoading.value = false;
  }
}

// Verify input function
async function verifyInput() {
  if (!isInput.value) return;
  
  verifyLoading.value = true;
  
  try {
    const contentToVerify = props.item?.isEdit ? editorValue.value : detail.value?.raw;
    
    if (!contentToVerify) {
      $message?.warning?.('No content to verify');
      return;
    }
    
    const response = await hubApi.verifyComponent(props.item.type, props.item.id, contentToVerify);
    
    if (response.data && response.data.valid) {
      $message?.success?.('Input configuration is valid');
      // Clear validation errors on successful verification
      validationResult.value = {
        isValid: true,
        errors: [],
        warnings: []
      };
      errorLines.value = [];
    } else {
      const errorMessage = response.data?.error || 'Unknown verification error';
      $message?.error?.('Verification failed: ' + errorMessage);
      
      // Extract line number from error message for highlighting
      const lineNumber = extractLineNumber(errorMessage);
      if (lineNumber) {
        errorLines.value = [lineNumber];
        
        // Add to validation result for display in the panel
        validationResult.value = {
          isValid: false,
          errors: [{
            line: lineNumber,
            message: errorMessage,
            detail: response.data?.detail || null
          }],
          warnings: []
        };
      } else {
        // Add general error without line number
        validationResult.value = {
          isValid: false,
          errors: [{
            line: 'Unknown',
            message: errorMessage,
            detail: response.data?.detail || null
          }],
          warnings: []
        };
      }
      showValidationPanel.value = true;
    }
  } catch (error) {
    const errorMessage = error.response?.data?.error || error.message || 'Unknown verification error';
    $message?.error?.('Verification error: ' + errorMessage);
    
    // Extract line number from error message for highlighting
    const lineNumber = extractLineNumber(errorMessage);
    if (lineNumber) {
      errorLines.value = [lineNumber];
      
      // Add to validation result for display in the panel
      validationResult.value = {
        isValid: false,
        errors: [{
          line: lineNumber,
          message: errorMessage,
          detail: error.response?.data?.detail || null
        }],
        warnings: []
      };
    } else {
      // Add general error without line number
      validationResult.value = {
        isValid: false,
        errors: [{
          line: 'Unknown',
          message: errorMessage,
          detail: error.response?.data?.detail || null
        }],
        warnings: []
      };
    }
    showValidationPanel.value = true;
  } finally {
    verifyLoading.value = false;
  }
}

// Connect check function for both input and output components
async function connectCheck() {
  if (!isInput.value && !isOutput.value) return;
  
  connectCheckLoading.value = true;
  
  try {
    // Determine component type
    const componentType = isInput.value ? 'inputs' : 'outputs';
    const componentName = isInput.value ? 'Input' : 'Output';
    
    // Call connect check API
    const response = await hubApi.connectCheck(componentType, props.item.id);
    
    if (response.status === 'success') {
      $message?.success?.(response.message || `${componentName} connection check passed`);
    } else if (response.status === 'warning') {
      $message?.warning?.(response.message || `${componentName} connection check has warnings`);
    } else {
      $message?.error?.(response.message || `${componentName} connection check failed`);
    }
  } catch (error) {
    const errorMessage = error.response?.data?.error || error.message || 'Connection check error';
    $message?.error?.('Connection check error: ' + errorMessage);
  } finally {
    connectCheckLoading.value = false;
  }
}

// Auto-verify plugin function (called by debounced watch)
async function autoVerifyPlugin() {
  if (!isPlugin.value || !props.item?.isEdit) return;
  
  try {
    const contentToVerify = editorValue.value;
    
    if (!contentToVerify || contentToVerify.trim() === '') {
      // Clear validation errors if content is empty
      validationResult.value = {
        isValid: true,
        errors: [],
        warnings: []
      };
      errorLines.value = [];
      return;
    }
    
    const response = await hubApi.verifyComponent(props.item.type, props.item.id, contentToVerify);
    
    if (response.data && response.data.valid) {
      // Clear validation errors on successful verification
      validationResult.value = {
        isValid: true,
        errors: [],
        warnings: []
      };
      errorLines.value = [];
      showValidationPanel.value = false;
    } else {
      const errorMessage = response.data?.error || 'Unknown verification error';
      
      // Extract line number from error message for highlighting
      const lineNumber = extractLineNumber(errorMessage);
      if (lineNumber) {
        errorLines.value = [lineNumber];
        
        // Add to validation result for display in the panel
        validationResult.value = {
          isValid: false,
          errors: [{
            line: lineNumber,
            message: errorMessage,
            detail: response.data?.detail || null
          }],
          warnings: []
        };
      } else {
        // Add general error without line number
        validationResult.value = {
          isValid: false,
          errors: [{
            line: 'Unknown',
            message: errorMessage,
            detail: response.data?.detail || null
          }],
          warnings: []
        };
      }
      showValidationPanel.value = true;
    }
  } catch (error) {
    const errorMessage = error.response?.data?.error || error.message || 'Unknown verification error';
    
    // Extract line number from error message for highlighting
    const lineNumber = extractLineNumber(errorMessage);
    if (lineNumber) {
      errorLines.value = [lineNumber];
      
      // Add to validation result for display in the panel
      validationResult.value = {
        isValid: false,
        errors: [{
          line: lineNumber,
          message: errorMessage,
          detail: error.response?.data?.detail || null
        }],
        warnings: []
      };
    } else {
      // Add general error without line number
      validationResult.value = {
        isValid: false,
        errors: [{
          line: 'Unknown',
          message: errorMessage,
          detail: error.response?.data?.detail || null
        }],
        warnings: []
      };
    }
    showValidationPanel.value = true;
  }
}

// Perform initial validation when component is mounted
onMounted(async () => {
  // Clear any previous validation state first
  validationResult.value = { isValid: true, errors: [], warnings: [] };
  errorLines.value = [];
  showValidationPanel.value = false;
  
  if (isRuleset.value && editorValue.value) {
    await validateRulesetRealtime()
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
    const isValid = await validateRulesetRealtime()
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
    const isValid = await validateRulesetRealtime()
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
    
    // Create new component
    const response = await hubApi.createComponent(currentItem.type, currentItem.id, contentToSave)
    
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
  // Clear plugin verification timeout
  if (pluginVerifyTimeout.value) {
    clearTimeout(pluginVerifyTimeout.value);
  }
});



// Mount hook for initial setup
onMounted(async () => {
  if (props.item) {
    if (props.item.isNew) {
      detail.value = null;
      editorValue.value = getTemplateForComponent(props.item.type, props.item.id);
    } else if (props.item.isEdit || !props.item.isEdit) {
      await fetchDetail(props.item, props.item.isEdit);
      
      if (isRuleset.value && editorValue.value) {
        await validateRulesetRealtime()
      }
    }
    
    if (isProject.value) {
      setupStatusRefresh();
    }
  }
});
</script> 

<style scoped>


/* Test Ruleset Button - Minimal Style */
.btn.btn-test-ruleset {
  background: transparent !important;
  border: 1px solid #d1d5db !important;
  color: #6b7280 !important;
  transition: all 0.15s ease !important;
  box-shadow: none !important;
  transform: none !important;
}

.btn.btn-test-ruleset:hover:not(:disabled) {
  border-color: #9ca3af !important;
  color: #374151 !important;
  background: rgba(249, 250, 251, 0.5) !important;
  box-shadow: none !important;
  transform: none !important;
}


/* Test Project Button - Minimal Style */
.btn.btn-test-project {
  background: transparent !important;
  border: 1px solid #d1d5db !important;
  color: #6b7280 !important;
  transition: all 0.15s ease !important;
  box-shadow: none !important;
  transform: none !important;
}

.btn.btn-test-project:hover:not(:disabled) {
  border-color: #0891b2 !important;
  color: #0891b2 !important;
  background: rgba(236, 254, 255, 0.3) !important;
  box-shadow: none !important;
  transform: none !important;
}

/* Test Plugin Button - Minimal Style */
.btn.btn-test-plugin {
  background: transparent !important;
  border: 1px solid #d1d5db !important;
  color: #6b7280 !important;
  transition: all 0.15s ease !important;
  box-shadow: none !important;
  transform: none !important;
}

.btn.btn-test-plugin:hover:not(:disabled) {
  border-color: #6366f1 !important;
  color: #6366f1 !important;
  background: rgba(238, 242, 255, 0.3) !important;
  box-shadow: none !important;
  transform: none !important;
}

/* Verify Buttons - Minimal Style */
.btn.btn-verify {
  background: transparent !important;
  border: 1px solid #d1d5db !important;
  color: #6b7280 !important;
  transition: all 0.15s ease !important;
  box-shadow: none !important;
  transform: none !important;
}

.btn.btn-verify:hover:not(:disabled) {
  border-color: #059669 !important;
  color: #059669 !important;
  background: rgba(236, 253, 245, 0.3) !important;
  box-shadow: none !important;
  transform: none !important;
}

/* Connect Check Button - Minimal Style */
.btn.btn-connect {
  background: transparent !important;
  border: 1px solid #d1d5db !important;
  color: #6b7280 !important;
  transition: all 0.15s ease !important;
  box-shadow: none !important;
  transform: none !important;
}

.btn.btn-connect:hover:not(:disabled) {
  border-color: #8b5cf6 !important;
  color: #8b5cf6 !important;
  background: rgba(250, 245, 255, 0.3) !important;
  box-shadow: none !important;
  transform: none !important;
}

/* Primary Buttons (Save/Create/Update) - Minimal Style */
.btn.btn-primary {
  background: transparent !important;
  border: 1px solid #3b82f6 !important;
  color: #3b82f6 !important;
  transition: all 0.15s ease !important;
  box-shadow: none !important;
  transform: none !important;
}

.btn.btn-primary:hover:not(:disabled) {
  border-color: #2563eb !important;
  color: #2563eb !important;
  background: rgba(59, 130, 246, 0.05) !important;
  box-shadow: none !important;
  transform: none !important;
}

.btn.btn-primary:disabled {
  border-color: #d1d5db !important;
  color: #9ca3af !important;
  background: transparent !important;
  box-shadow: none !important;
  transform: none !important;
}

/* Secondary Buttons (Cancel) - Minimal Style */
.btn.btn-secondary {
  background: transparent !important;
  border: 1px solid #d1d5db !important;
  color: #6b7280 !important;
  transition: all 0.15s ease !important;
  box-shadow: none !important;
  transform: none !important;
}

.btn.btn-secondary:hover:not(:disabled) {
  border-color: #9ca3af !important;
  color: #374151 !important;
  background: rgba(249, 250, 251, 0.5) !important;
  box-shadow: none !important;
  transform: none !important;
}

/* Project Control Buttons - Minimal Style */
.btn.btn-start {
  background: transparent !important;
  border: 1px solid #d1d5db !important;
  color: #6b7280 !important;
  transition: all 0.15s ease !important;
  box-shadow: none !important;
  transform: none !important;
}

.btn.btn-start:hover:not(:disabled) {
  border-color: #059669 !important;
  color: #059669 !important;
  background: rgba(236, 253, 245, 0.3) !important;
  box-shadow: none !important;
  transform: none !important;
}

.btn.btn-stop {
  background: transparent !important;
  border: 1px solid #d1d5db !important;
  color: #6b7280 !important;
  transition: all 0.15s ease !important;
  box-shadow: none !important;
  transform: none !important;
}

.btn.btn-stop:hover:not(:disabled) {
  border-color: #dc2626 !important;
  color: #dc2626 !important;
  background: rgba(254, 242, 242, 0.3) !important;
  box-shadow: none !important;
  transform: none !important;
}

.btn.btn-restart {
  background: transparent !important;
  border: 1px solid #d1d5db !important;
  color: #6b7280 !important;
  transition: all 0.15s ease !important;
  box-shadow: none !important;
  transform: none !important;
}

.btn.btn-restart:hover:not(:disabled) {
  border-color: #f59e0b !important;
  color: #f59e0b !important;
  background: rgba(255, 251, 235, 0.3) !important;
  box-shadow: none !important;
  transform: none !important;
}

/* Warning Buttons - Minimal Style */
.btn.btn-warning {
  background: transparent !important;
  border: 1px solid #f59e0b !important;
  color: #f59e0b !important;
  transition: all 0.15s ease !important;
  box-shadow: none !important;
  transform: none !important;
}

.btn.btn-warning:hover:not(:disabled) {
  border-color: #d97706 !important;
  color: #d97706 !important;
  background: rgba(255, 251, 235, 0.3) !important;
  box-shadow: none !important;
  transform: none !important;
}

/* Icon Buttons - Minimal Style */
.btn.btn-icon {
  background: transparent !important;
  border: 1px solid transparent !important;
  color: #6b7280 !important;
  transition: all 0.15s ease !important;
  padding: 0.5rem !important;
  box-shadow: none !important;
  transform: none !important;
}

.btn.btn-icon:hover:not(:disabled) {
  border-color: #d1d5db !important;
  color: #374151 !important;
  background: rgba(249, 250, 251, 0.5) !important;
  box-shadow: none !important;
  transform: none !important;
}

/* Ghost Button Variants */
.btn.btn-secondary-ghost {
  background: transparent !important;
  border: 1px solid transparent !important;
  color: #6b7280 !important;
  transition: all 0.15s ease !important;
  box-shadow: none !important;
  transform: none !important;
}

.btn.btn-secondary-ghost:hover:not(:disabled) {
  border-color: #d1d5db !important;
  color: #374151 !important;
  background: rgba(249, 250, 251, 0.5) !important;
  box-shadow: none !important;
  transform: none !important;
}

/* General Button Styles - Minimal Tech Theme */
button {
  transition: all 0.15s ease !important;
}

/* Disabled button states */
button:disabled {
  opacity: 0.5 !important;
  cursor: not-allowed !important;
}

/* Enhanced focus states for accessibility */
button:focus {
  outline: 2px solid #3b82f6 !important;
  outline-offset: 2px !important;
}

/* Validation Styles - Minimal Tech Theme */
.validation-errors, .validation-warnings {
  border-radius: 6px;
}

.validation-errors {
  background-color: rgba(239, 68, 68, 0.05);
  border-left: 3px solid #ef4444;
}

.validation-warnings {
  background-color: rgba(245, 158, 11, 0.05);
  border-left: 3px solid #f59e0b;
}

.validation-errors h3, .validation-warnings h3 {
  margin-top: 0;
  font-size: 14px;
  font-weight: 600;
  letter-spacing: 0.025em;
}

</style> 