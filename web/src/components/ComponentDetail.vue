<template>
  <div v-if="loading" class="flex items-center justify-center h-full text-gray-400 text-lg">Loading...</div>
  <div v-else-if="error" class="flex items-center justify-center h-full text-red-400 text-lg">{{ error }}</div>
  
  <!-- Create Mode -->
  <div v-else-if="props.item && props.item.isNew" class="h-full flex flex-col">
    <!-- Special layout for projects: Split view with live preview -->
    <div v-if="isProject" class="flex h-full">
      <div class="w-1/2 h-full">
        <MonacoEditor v-model:value="editorValue" :language="props.item.type === 'rulesets' ? 'xml' : (props.item.type === 'plugins' ? 'go' : 'yaml')" :read-only="false" :error-lines="errorLines" class="h-full" @save="saveNew" @line-change="handleLineChange" :component-id="props.item?.id" :component-type="props.item?.type" />
      </div>
      <div class="w-1/2 h-full border-l border-gray-200">
        <div class="p-3 bg-gray-50 border-b border-gray-200">
          <h3 class="text-sm font-medium text-gray-700 flex items-center">
            <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
            </svg>
            Live Preview
          </h3>
        </div>
        <ProjectWorkflow :projectContent="editorValue" :projectId="props.item?.id" :enableMessages="false" />
      </div>
    </div>
    <!-- Default full-screen editor for other component types -->
          <MonacoEditor v-else v-model:value="editorValue" :language="props.item.type === 'rulesets' ? 'xml' : (props.item.type === 'plugins' ? 'go' : 'yaml')" :read-only="false" :error-lines="errorLines" class="flex-1" @save="saveNew" @line-change="handleLineChange" :component-id="props.item?.id" :component-type="props.item?.type" />
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
        @click="verifyCurrentComponent" 
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
        @click="verifyCurrentComponent" 
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
        @click="verifyCurrentComponent" 
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
        @click="() => saveNew()" 
        class="btn btn-primary btn-md"
        :disabled="saving"
      >
        <span v-if="saving" class="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin mr-2"></span>
        <svg v-else class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8.71,7.71,11,5.41V15a1,1,0,0,0,2,0V5.41l2.29,2.3a1,1,0,0,0,1.42,0,1,1,0,0,0,0-1.42l-4-4a1,1,0,0,0-.33-.21,1,1,0,0,0-.76,0,1,1,0,0,0-.33.21l-4,4A1,1,0,1,0,8.71,7.71ZM21,12a1,1,0,0,0-1,1v6a1,1,0,0,1-1,1H5a1,1,0,0,1-1-1V13a1,1,0,0,0-2,0v6a3,3,0,0,0,3,3H19a3,3,0,0,0,3-3V13A1,1,0,0,0,21,12Z" />
        </svg>
        {{ saving ? 'Saving...' : 'Create' }}
      </button>
    </div>
    <div v-if="saveError" class="text-xs text-red-500 mt-2">{{ saveError }}</div>
  </div>

  <!-- Edit Mode -->
  <div v-else-if="props.item && props.item.isEdit && detail" class="h-full flex flex-col relative">
    <!-- Floating Validation Status (for Rulesets, Projects, Plugins, Outputs, and Inputs) -->
    <div v-if="(isRuleset || isProject || isPlugin || isOutput || isInput) && (validationResult.errors.length > 0 || validationResult.warnings.length > 0) && showValidationPanel" 
         class="absolute top-4 right-4 z-50 max-w-lg bg-white/95 border border-gray-200/60 rounded-xl shadow-2xl backdrop-blur-md">
      <!-- Validation Errors -->
      <div v-if="validationResult.errors.length > 0" class="validation-errors p-4 bg-red-50/60 border-l-4 border-red-400/70 text-red-800 rounded-t-xl backdrop-blur-sm">
        <div class="flex justify-between items-start mb-3">
          <h3 class="font-semibold text-sm text-red-900">{{ isPlugin ? 'Compilation' : (isOutput ? 'Output Validation' : (isInput ? 'Input Validation' : (isProject ? 'Project Validation' : 'Validation'))) }} Errors</h3>
          <button @click="showValidationPanel = false" class="text-red-400 hover:text-red-600 ml-2 transition-colors duration-150">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
            </svg>
          </button>
        </div>
        <ul class="text-xs space-y-2">
          <li v-for="(error, index) in validationResult.errors" :key="index" class="flex flex-col">
            <span class="font-medium text-red-900">Line {{ error.line }}:</span> 
            <span class="text-red-700 ml-1 break-words leading-relaxed">{{ error.message }}</span>
            <span v-if="error.detail" class="text-red-600 text-xs mt-1 ml-4 italic opacity-80 break-words">{{ error.detail }}</span>
          </li>
        </ul>
      </div>

      <!-- Validation Warnings -->
      <div v-if="validationResult.warnings.length > 0" 
           class="validation-warnings p-4 bg-amber-50/60 border-l-4 border-amber-400/70 text-amber-800 backdrop-blur-sm"
           :class="{ 'rounded-t-xl': validationResult.errors.length === 0, 'rounded-b-xl': true }">
        <div v-if="validationResult.errors.length === 0" class="flex justify-between items-start mb-3">
          <h3 class="font-semibold text-sm text-amber-900">{{ isPlugin ? 'Compilation' : (isOutput ? 'Output Validation' : (isInput ? 'Input Validation' : (isProject ? 'Project Validation' : 'Validation'))) }} Warnings</h3>
          <button @click="showValidationPanel = false" class="text-amber-400 hover:text-amber-600 ml-2 transition-colors duration-150">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path>
            </svg>
          </button>
        </div>
        <h3 v-else class="font-semibold text-sm mb-3 text-amber-900">{{ isPlugin ? 'Compilation' : (isOutput ? 'Output Validation' : (isInput ? 'Input Validation' : (isProject ? 'Project Validation' : 'Validation'))) }} Warnings</h3>
        <ul class="text-xs space-y-2">
          <li v-for="(warning, index) in validationResult.warnings" :key="index" class="flex flex-col">
            <span class="font-medium text-amber-900">Line {{ warning.line }}:</span> 
            <span class="text-amber-700 ml-1 break-words leading-relaxed">{{ warning.message }}</span>
            <span v-if="warning.detail" class="text-amber-600 text-xs mt-1 ml-4 italic opacity-80 break-words">{{ warning.detail }}</span>
          </li>
        </ul>
      </div>
    </div>

    <!-- Validation Status Indicator -->
    <div v-if="(isRuleset || isProject || isPlugin || isOutput || isInput) && (validationResult.errors.length > 0 || validationResult.warnings.length > 0) && !showValidationPanel"
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
    
         <!-- Special layout for projects: Split view with live preview -->
     <div v-if="isProject" class="flex h-full">
       <div class="w-1/2 h-full">
         <MonacoEditor v-model:value="editorValue" :language="props.item.type === 'rulesets' ? 'xml' : (props.item.type === 'plugins' ? 'go' : 'yaml')" :read-only="false" :error-lines="errorLines" class="h-full" @save="saveEdit" @line-change="handleLineChange" :component-id="props.item?.id" :component-type="props.item?.type" />
       </div>
       <div class="w-1/2 h-full border-l border-gray-200">
        <div class="p-3 bg-gray-50 border-b border-gray-200">
          <h3 class="text-sm font-medium text-gray-700 flex items-center">
            <svg class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                             <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
            </svg>
            Live Preview
          </h3>
        </div>
        <ProjectWorkflow :projectContent="editorValue" :projectId="props.item?.id" :enableMessages="false" />
      </div>
    </div>
    <!-- Default full-screen editor for other component types -->
           <MonacoEditor v-else v-model:value="editorValue" :language="props.item.type === 'rulesets' ? 'xml' : (props.item.type === 'plugins' ? 'go' : 'yaml')" :read-only="false" :error-lines="errorLines" class="flex-1" @save="saveEdit" @line-change="handleLineChange" :component-id="props.item?.id" :component-type="props.item?.type" />
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
        @click="verifyCurrentComponent" 
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
        @click="verifyCurrentComponent" 
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
        @click="verifyCurrentComponent" 
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
        @click="verifyCurrentComponent" 
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
        @click="verifyCurrentComponent" 
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
        @click="() => saveEdit()" 
        class="btn btn-primary btn-md"
        :disabled="saving"
      >
        <span v-if="saving" class="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin mr-2"></span>
        <svg v-else class="w-4 h-4 mr-2" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8.71,7.71,11,5.41V15a1,1,0,0,0,2,0V5.41l2.29,2.3a1,1,0,0,0,1.42,0,1,1,0,0,0,0-1.42l-4-4a1,1,0,0,0-.33-.21,1,1,0,0,0-.76,0,1,1,0,0,0-.33.21l-4,4A1,1,0,1,0,8.71,7.71ZM21,12a1,1,0,0,0-1,1v6a1,1,0,0,1-1,1H5a1,1,0,0,1-1-1V13a1,1,0,0,0-2,0v6a3,3,0,0,0,3,3H19a3,3,0,0,0,3-3V13A1,1,0,0,0,21,12Z" />
        </svg>
        {{ saving ? 'Saving...' : 'Update' }}
      </button>
    </div>
    <div v-if="saveError" class="text-xs text-red-500 mt-2 px-4 mb-3">{{ saveError }}</div>
  </div>

  <!-- Special layout for projects -->
  <div v-else-if="props.item && props.item.type === 'projects' && detail && detail.raw" class="flex h-full">
    <div class="w-1/2 h-full">
       <MonacoEditor :value="detail.raw" :language="props.item.type === 'rulesets' ? 'xml' : (props.item.type === 'plugins' ? 'go' : 'yaml')" :read-only="true" class="h-full" :component-id="props.item?.id" :component-type="props.item?.type" />
    </div>
    <div class="w-1/2 h-full border-l border-gray-200">
      <ProjectWorkflow :projectContent="detail.raw" :projectId="props.item?.id || detail.id" :enableMessages="detail.status === 'running'" />
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
            v-if="detail.status === 'stopped' || detail.status === 'error'"
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
          
          <!-- Starting status display -->
          <div v-if="detail.status === 'starting'" class="flex items-center text-blue-600 bg-blue-50 px-3 py-1 rounded-md">
            <div class="w-3 h-3 border-1.5 border-current border-t-transparent rounded-full animate-spin mr-2"></div>
            <span class="text-sm font-medium">Starting...</span>
          </div>
          
          <!-- Stopping status display -->
          <div v-if="detail.status === 'stopping'" class="flex items-center text-orange-600 bg-orange-50 px-3 py-1 rounded-md">
            <div class="w-3 h-3 border-1.5 border-current border-t-transparent rounded-full animate-spin mr-2"></div>
            <span class="text-sm font-medium">Stopping...</span>
          </div>
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
            @click="verifyCurrentComponent"
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
    <MonacoEditor :value="detail.raw" :language="props.item.type === 'rulesets' ? 'xml' : (props.item.type === 'plugins' ? 'go' : 'yaml')" :read-only="true" class="flex-1" :component-id="props.item?.id" :component-type="props.item?.type" />
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
    :pluginContent="props.item?.isEdit ? editorValue : null"
    @close="showPluginTestModal = false"
  />
  <ProjectTestModal
    v-if="props.item && props.item.type === 'projects'"
    :show="showProjectTestModal"
    :projectId="props.item?.id"
    :projectContent="props.item?.isEdit ? editorValue : null"
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
import { ref, watch, inject, computed, onMounted, onBeforeUnmount, watchEffect } from 'vue'
import { hubApi } from '../api'
import { useDataCacheStore } from '../stores/dataCache'
import MonacoEditor from '@/components/MonacoEditor.vue'
import ProjectWorkflow from './Visualization/ProjectWorkflow.vue'
import RulesetTestModal from './RulesetTestModal.vue'
import PluginTestModal from './PluginTestModal.vue'
import ProjectTestModal from './ProjectTestModal.vue'
import { useStore } from 'vuex'
import { useRouter } from 'vue-router'
import { useComponentValidation } from '../composables/useComponentValidation'
import { useComponentSave } from '../composables/useComponentSave'
import { extractLineNumber } from '../utils/common'
import { RulesetTestCache, ProjectTestCache } from '../utils/cacheUtils'

import { getDefaultTemplate } from '../utils/templateGenerator'

// Props
const props = defineProps({
  item: Object
})

// Emits
const emit = defineEmits(['created', 'updated', 'cancel-edit'])

// Use composables for validation and save operations
const {
  validationResult,
  errorLines,
  showValidationPanel,
  verifyLoading,
  clearValidation,
  validateRealtime,
  verifyComponent,
  validateBeforeSave,
  verifyAfterSave
} = useComponentValidation()

const {
  saving,
  saveError,
  preventRefetch,
  saveEdit: saveEditComponent,
  saveNew: saveNewComponent
} = useComponentSave()

// Reactive state
const loading = ref(false)
const error = ref(null)
const detail = ref(null)
const editorValue = ref('')
const originalContent = ref('') // Save original content for restoring when canceling edit
const connectCheckLoading = ref(false)
const projectValidationTimeout = ref(null) // Timeout for project auto-verification
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
  if (isOutput.value && detail.value) {
    // Use the type information returned by backend API
    if (detail.value.type) {
      const outputType = detail.value.type.toLowerCase()
      if (outputType === 'print') {
        return false // Print output doesn't need connect check
      }
      // For other types (kafka, elasticsearch, aliyun_sls), support connect check
      return true
    }
    // Fallback: if no type info, don't support connect check (safer)
    return false
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
const dataCache = useDataCacheStore()

// Project operation state
const projectOperationLoading = ref(false)
const projectWarningModal = ref(false)
const projectWarningMessage = ref('')
const projectOperationType = ref('') // 'start', 'stop', 'restart'

// Project status refresh
const statusRefreshInterval = ref(null)
const lastProjectOperation = ref(0) // 记录最近项目操作时间
const currentStatusInterval = ref(300000) // 记录当前刷新间隔

// Watch for item changes
watch(
  () => props.item,
  (newVal, oldVal) => {
    // Skip if we're preventing refetch (during save operations)
    if (preventRefetch.value) {
      return
    }
    
    if (!newVal) {
      detail.value = null;
      clearValidation();
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
      clearValidation();
    } else if (newVal && newVal.isEdit) {
      fetchDetail(newVal, true);
      clearValidation();
    } else if (newVal && (typeChanged || idChanged || timestampChanged || editModeChanged)) {
      // If component ID, type, timestamp or edit mode changes, refresh details
      fetchDetail(newVal);
      clearValidation();
    }
  },
  { immediate: true, deep: true }
)

// Functions now imported from utils/common.js and composables

// Methods
async function fetchDetail(item, forEdit = false) {
  detail.value = null
  error.value = null
  if (!item || !item.id) {
    return;
  }
  loading.value = true
  try {
    let data
    let tempInfo = null
    
    // If in edit mode, check for temporary file
    if (forEdit) {
      tempInfo = await hubApi.checkTemporaryFile(item.type, item.id);
      
      // Don't automatically create temporary file - let the save operation handle it
      // This prevents creating unnecessary .new files when content is identical
    }
    
    // Get details using unified dataCache
    data = await dataCache.fetchComponentDetail(item.type, item.id, forEdit)
    
    // Check if this is a temporary file
    if (data && data.path) {
      data.isTemporary = data.path.endsWith('.new');
    }
    
    // Ensure we have content
    if (!data || (!data.raw && data.raw !== '')) {
      // console.warn(`No content received for ${item.type} ${item.id}:`, data);
      // Try to fetch again without temporary file logic
      if (forEdit && tempInfo && tempInfo.hasTemp) {
        return await fetchDetail(item, false);
      }
    }
    
    detail.value = data;
    
    if (forEdit) {
      editorValue.value = data.raw || '';
      originalContent.value = data.raw || '';
    }
    
    // Perform initial validation for rulesets (silent)
    if (item.type === 'rulesets' && data.raw) {
      try {
        await validateRealtime(item.type, item.id, data.raw);
      } catch (verifyError) {
        // console.warn('Initial ruleset verification failed:', verifyError);
        // Don't show errors on initial load, just clear validation
        clearValidation();
      }
    }
  } catch (e) {
    error.value = `Failed to load ${item.type}: ${e.message || 'Unknown error'}`;
    console.error(`Error fetching ${item.type} detail:`, e);
  } finally {
    loading.value = false;
  }
}

// Real-time validation functions are now centralized in useComponentValidation composable

// Watch for changes in editor content and perform real-time validation  
const rulesetValidationTimeout = ref(null);

// Real-time validation for all component types
watch(editorValue, (newContent) => {
  if (props.item?.type && props.item?.id && newContent) {
    if (isRuleset.value) {
      // Debounce ruleset validation to avoid excessive API calls
      clearTimeout(rulesetValidationTimeout.value);
      rulesetValidationTimeout.value = setTimeout(async () => {
        await validateRealtime(props.item.type, props.item.id, newContent);
      }, 800); // Wait 800ms after user stops typing for faster feedback
    } else if (isProject.value) {
      // Debounce project validation for better responsiveness
      clearTimeout(projectValidationTimeout.value);
      projectValidationTimeout.value = setTimeout(async () => {
        await validateRealtime(props.item.type, props.item.id, newContent);
      }, 1000); // Wait 1s after user stops typing
    }
  }
}, { deep: true })

// Track last cursor line for validation
const lastCursorLine = ref(1)

// Timeout variables for validation
const inputValidationTimeout = ref(null)
const outputValidationTimeout = ref(null)
const pluginValidationTimeout = ref(null)

// Handle line change for real-time validation (project, input, output, plugin)
function handleLineChange(newLineNumber) {
  if (newLineNumber !== lastCursorLine.value && props.item?.type && props.item?.id && editorValue.value) {
    if (isProject.value) {
      // User moved to a different line in project, validate the project
      clearTimeout(projectValidationTimeout.value);
      projectValidationTimeout.value = setTimeout(async () => {
        await validateRealtime(props.item.type, props.item.id, editorValue.value);
      }, 300); // Quick validation when changing lines
    } else if (isInput.value) {
      // User moved to a different line in input, validate the input
      clearTimeout(inputValidationTimeout.value);
      inputValidationTimeout.value = setTimeout(async () => {
        await validateRealtime(props.item.type, props.item.id, editorValue.value);
      }, 300); // Quick validation when changing lines
    } else if (isOutput.value) {
      // User moved to a different line in output, validate the output
      clearTimeout(outputValidationTimeout.value);
      outputValidationTimeout.value = setTimeout(async () => {
        await validateRealtime(props.item.type, props.item.id, editorValue.value);
      }, 300); // Quick validation when changing lines
    } else if (isPlugin.value) {
      // User moved to a different line in plugin, validate the plugin
      clearTimeout(pluginValidationTimeout.value);
      pluginValidationTimeout.value = setTimeout(async () => {
        await validateRealtime(props.item.type, props.item.id, editorValue.value);
      }, 300); // Quick validation when changing lines
    }
    
    lastCursorLine.value = newLineNumber;
  }
}



// Generic verify function using composable
async function verifyCurrentComponent() {
  if (!props.item?.type || !props.item?.id) return;
  
  const contentToVerify = props.item?.isEdit ? editorValue.value : detail.value?.raw;
  await verifyComponent(props.item.type, props.item.id, contentToVerify);
}

// All specific verification functions replaced with generic verifyCurrentComponent()

// Connect check function for both input and output components
async function connectCheck() {
  if (!isInput.value && !isOutput.value) return;
  
  connectCheckLoading.value = true;
  
  try {
    // Determine component type
    const componentType = isInput.value ? 'inputs' : 'outputs';
    const componentName = isInput.value ? 'Input' : 'Output';
    
    // Get content to test (use editor value if editing, otherwise use saved config)
    const contentToTest = props.item?.isEdit ? editorValue.value : detail.value?.raw;
    
    if (!contentToTest) {
      $message?.warning?.('No content to test');
      return;
    }
    
    // Step 1: Verify configuration first
    let verifyResponse;
    try {
      verifyResponse = await hubApi.verifyComponent(props.item.type, props.item.id, contentToTest);
      
      if (!verifyResponse.data || !verifyResponse.data.valid) {
        const errorMessage = verifyResponse.data?.error || 'Configuration verification failed';
        $message?.error?.(`Verification failed: ${errorMessage}. Please fix configuration before testing connection.`);
        return;
      }
      
      // Show success message for verification
      if (props.item?.isEdit) {
        $message?.success?.('Configuration verified successfully, proceeding with connection test...');
      }
    } catch (verifyError) {
      const errorMessage = verifyError.response?.data?.error || verifyError.message || 'Configuration verification error';
      $message?.error?.(`Verification error: ${errorMessage}. Please fix configuration before testing connection.`);
      return;
    }
    
    // Step 2: Perform connection check with the verified configuration
    const response = await hubApi.connectCheckWithConfig(componentType, props.item.id, contentToTest);
    
    // Helper function to format message with edit suffix
    const formatMessage = (message) => {
      // If not in edit mode, return original message
      if (!props.item?.isEdit) {
        return message;
      }
      
      // If message already contains test-related suffix, don't add duplicate
      if (message.includes('(tested with') || message.includes('(using current')) {
        return message;
      }
      
      return `${message} (tested with current editor content)`;
    };

    if (response.status === 'success') {
      const message = response.message || `${componentName} connection check passed`;
      $message?.success?.(formatMessage(message));
    } else if (response.status === 'warning') {
      const message = response.message || `${componentName} connection check has warnings`;
      $message?.warning?.(formatMessage(message));
    } else {
      // Try to get detailed error information
      let message = response.message || `${componentName} connection check failed`;
      
      // Check if detailed connection error information is available
      if (response.details && response.details.connection_errors && response.details.connection_errors.length > 0) {
        const detailError = response.details.connection_errors[0].message;
        if (detailError && detailError !== message) {
          message = `${message}: ${detailError}`;
        }
      }
      
      $message?.error?.(formatMessage(message));
    }
  } catch (error) {
    // 网络请求异常或其他JavaScript错误
    const errorMessage = error.response?.data?.error || error.response?.data?.message || error.message || 'Connection check error';
    $message?.error?.('Connection check error: ' + errorMessage);
  } finally {
    connectCheckLoading.value = false;
  }
}

// validatePluginRealtime function removed - now handled by composable



// Perform initial validation when component is mounted
onMounted(async () => {
  // Clear any previous validation state first
  clearValidation();
  
  if (props.item?.type && props.item?.id && editorValue.value) {
    await validateRealtime(props.item.type, props.item.id, editorValue.value);
  }
  
  // If component type is project, fetch all components list
  if (props.item && props.item.type === 'projects') {
    store.dispatch('fetchAllComponents')
  }
  
  // Set up periodic validation for projects (every 3 seconds)
  if (isProject.value) {
    const periodicValidation = setInterval(async () => {
      if (props.item?.isEdit && props.item?.type && props.item?.id && editorValue.value) {
        await validateRealtime(props.item.type, props.item.id, editorValue.value);
      }
    }, 3000);
    
    // Clean up interval on component unmount
    onBeforeUnmount(() => {
      clearInterval(periodicValidation);
    });
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
  
  // Use the new composable for save operation
  const success = await saveEditComponent(currentItem.type, currentItem.id, contentToSave, {
    validateBeforeSave,
    verifyAfterSave,
    fetchDetail,
    onSuccess: (item) => {
      // Clear test cache after successful save
      if (currentItem.type === 'rulesets') {
        RulesetTestCache.clear(currentItem.id);
      } else if (currentItem.type === 'projects') {
        ProjectTestCache.clear(currentItem.id);
      }
      emit('updated', item)
    }
  })
  
  if (success) {
    // Force refresh by clearing current detail first and reloading
    detail.value = null
    editorValue.value = ''
    
    // Refresh component content after successful save but stay in edit mode
    await fetchDetail(currentItem, true)
    
    // If still no content, try fetching the original file
    if (!detail.value || !detail.value.raw) {
      await fetchDetail(currentItem, false)
      if (detail.value && detail.value.raw) {
        editorValue.value = detail.value.raw
        originalContent.value = detail.value.raw
      }
    }
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
  
  // Use the new composable for save operation
  await saveNewComponent(currentItem.type, currentItem.id, contentToSave, {
    validateBeforeSave,
    verifyAfterSave,
    onSuccess: (item) => {
      // Clear test cache after successful save
      if (currentItem.type === 'rulesets') {
        RulesetTestCache.clear(currentItem.id);
      } else if (currentItem.type === 'projects') {
        ProjectTestCache.clear(currentItem.id);
      }
      emit('created', item)
    }
  })
}

function cancelEdit() {
  // Restore original content
  editorValue.value = originalContent.value
  if (detail.value) detail.value.raw = originalContent.value
  // Clear error messages
  saveError.value = ''
  errorLines.value = [] // 清空错误行
  
  // Clear test cache when canceling edit
  if (props.item?.id) {
    if (isRuleset.value) {
      RulesetTestCache.clear(props.item.id);
    } else if (isProject.value) {
      ProjectTestCache.clear(props.item.id);
    }
  }
  
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

// 发送全局项目操作事件
function emitProjectOperation(operationType) {
  const timestamp = Date.now()
  lastProjectOperation.value = timestamp
  
  // 发送全局事件通知其他组件
  window.dispatchEvent(new CustomEvent('projectOperation', {
    detail: {
      projectId: props.item?.id,
      operationType,
      timestamp
    }
  }))
}

// Project operations
async function startProject() {
  if (!props.item || !props.item.id) return
  
  // 记录操作时间并通知其他组件
  emitProjectOperation('start')
  
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
      // 不要立即修改状态，让刷新机制去更新状态确保同步
      // 操作后会触发快速刷新来获取真实状态
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
  
  // 记录操作时间并通知其他组件
  emitProjectOperation('stop')
  
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
      // 不要立即修改状态，让刷新机制去更新状态确保同步
      // 操作后会触发快速刷新来获取真实状态
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
  
  // 记录操作时间并通知其他组件
  emitProjectOperation('restart')
  
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
      // 不要立即修改状态，让刷新机制去更新状态确保同步
      // 操作后会触发快速刷新来获取真实状态
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
            // 不要立即修改状态，让刷新机制去更新状态确保同步
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
            // 不要立即修改状态，让刷新机制去更新状态确保同步
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
            // 不要立即修改状态，让刷新机制去更新状态确保同步
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
    // 动态调整刷新频率：过渡状态1秒，稳定状态60秒
    const getRefreshInterval = () => {
      const currentStatus = detail.value?.status;
      // 过渡状态（starting/stopping）使用更快的刷新频率
      if (currentStatus === 'starting' || currentStatus === 'stopping') {
        return 1000; // 1秒
      }
      // 检查是否在最近操作后的30秒内
      const recentOperation = Date.now() - lastProjectOperation.value < 30000;
      if (recentOperation) {
        return 1000; // 1秒 - 最近操作后快速刷新
      }
      // 稳定状态使用正常频率
      return 300000; // 5分钟
    };
    
    const refreshStatus = async () => {
      if (detail.value && !detail.value.isTemporary) {
        try {
          // Use dataCache with force refresh for real-time status updates
          const clusterStatus = await dataCache.fetchClusterInfo(true);
          if (clusterStatus && clusterStatus.projects) {
            const projectStatus = clusterStatus.projects.find(p => p.id === props.item.id);
            if (projectStatus && detail.value) {
              const oldStatus = detail.value.status;
              detail.value.status = projectStatus.status || 'stopped';
              
              // 检查是否需要调整刷新间隔
              const newInterval = getRefreshInterval();
              
              if (newInterval !== currentStatusInterval.value) {
                clearInterval(statusRefreshInterval.value);
                currentStatusInterval.value = newInterval;
                statusRefreshInterval.value = setInterval(refreshStatus, newInterval);
                // console.log(`ComponentDetail refresh interval adjusted to ${newInterval}ms for project ${props.item.id}`);
              }
            }
          }
        } catch (error) {
          console.error('Failed to refresh project status:', error);
        }
      }
    };
    
    // 初始设置
    const initialInterval = getRefreshInterval();
    currentStatusInterval.value = initialInterval;
    statusRefreshInterval.value = setInterval(refreshStatus, initialInterval);
    // console.log(`ComponentDetail refresh started with ${initialInterval}ms for project ${props.item?.id}`);
    
    // 立即执行一次刷新
    refreshStatus();
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
    
    // Clear test cache when switching between different components
    if (oldVal && oldVal !== newVal) {
      if (props.item?.type === 'rulesets') {
        RulesetTestCache.clear(oldVal);
        // console.log(`[ComponentDetail] Cleared test cache for previous ruleset: ${oldVal}`);
      } else if (props.item?.type === 'projects') {
        ProjectTestCache.clear(oldVal);
        // console.log(`[ComponentDetail] Cleared test cache for previous project: ${oldVal}`);
      }
    }
    
    // Clear any existing validation errors when switching components
    validationResult.value = { isValid: true, errors: [], warnings: [] };
    errorLines.value = [];
    showValidationPanel.value = false;
  }
});

// 页面可见性变化处理
function handleComponentVisibilityChange() {
  if (document.hidden) {
    // 页面隐藏时暂停刷新
    clearStatusRefresh();
  } else if (isProject.value && props.item?.id) {
    // 页面重新可见时恢复刷新
    setupStatusRefresh();
  }
}

// 组件卸载时清除定时刷新
onBeforeUnmount(() => {
  clearStatusRefresh();
  
  // Remove page visibility change listener
  document.removeEventListener('visibilitychange', handleComponentVisibilityChange);
  
  // Clear validation timeouts
  if (rulesetValidationTimeout.value) {
    clearTimeout(rulesetValidationTimeout.value);
  }
  if (projectValidationTimeout.value) {
    clearTimeout(projectValidationTimeout.value);
  }
  if (inputValidationTimeout.value) {
    clearTimeout(inputValidationTimeout.value);
  }
  if (outputValidationTimeout.value) {
    clearTimeout(outputValidationTimeout.value);
  }
  if (pluginValidationTimeout.value) {
    clearTimeout(pluginValidationTimeout.value);
  }
  
  // Clear test cache when leaving component interface
  if (props.item?.id) {
    if (isRuleset.value) {
      RulesetTestCache.clear(props.item.id);
      // console.log(`[ComponentDetail] Cleared test cache for ruleset: ${props.item.id}`);
    } else if (isProject.value) {
      ProjectTestCache.clear(props.item.id);
      // console.log(`[ComponentDetail] Cleared test cache for project: ${props.item.id}`);
    }
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
  
  // Add page visibility change listener for projects
  if (isProject.value) {
    document.addEventListener('visibilitychange', handleComponentVisibilityChange);
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